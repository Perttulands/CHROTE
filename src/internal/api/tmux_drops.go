package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

func newSessionDropSemaphore() chan struct{} {
	sem := make(chan struct{}, 1)
	sem <- struct{}{}
	return sem
}

const defaultSessionDropsDir = "/srv/data/chrote/session-drops"
const defaultSessionDropRetention = 7 * 24 * time.Hour
const defaultSessionDropMaintenanceInterval = time.Hour

var sessionDropIDPattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z-[0-9a-f]{24}$`)
var sessionDropUnixUserPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*[$]?$`)
var errEmptySessionDrop = errors.New("send text or at least one file")

func validSessionDropID(name string) bool {
	if !sessionDropIDPattern.MatchString(name) {
		return false
	}
	_, err := time.Parse("20060102T150405Z", strings.SplitN(name, "-", 2)[0])
	return err == nil
}

func defaultSessionDropsPath() string {
	if override := strings.TrimSpace(os.Getenv("CHROTE_SESSION_DROPS_DIR")); override != "" {
		return override
	}
	return defaultSessionDropsDir
}

type sessionDropFile struct {
	Name        string `json:"name"`
	Original    string `json:"original"`
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	ContentType string `json:"contentType,omitempty"`
}

type sessionDropManifest struct {
	ID             string                `json:"id"`
	Session        string                `json:"session"`
	UnixUser       string                `json:"unixUser,omitempty"`
	PaneID         string                `json:"paneId"`
	PanePID        string                `json:"panePid"`
	ServerPID      string                `json:"serverPid"`
	CreatedAt      string                `json:"createdAt"`
	TextPath       string                `json:"textPath,omitempty"`
	Payload        string                `json:"payload"`
	Files          []sessionDropFile     `json:"files"`
	submitEvidence submitPayloadEvidence `json:"-"`
}

func newSessionDropID() (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return time.Now().UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(raw), nil
}

func sanitizeDropFileName(name string, fallback string) string {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	if name == "." || name == string(filepath.Separator) {
		name = ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		keep := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if keep {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	cleaned := strings.Trim(b.String(), ".-_")
	if cleaned == "" {
		cleaned = fallback
	}
	return cleaned
}

func uniqueDropFileName(used map[string]int, name string) string {
	if used[name] == 0 {
		used[name] = 1
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if base == "" {
		base = "file"
	}
	for i := used[name] + 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if used[candidate] == 0 {
			used[name] = i
			used[candidate] = 1
			return candidate
		}
	}
}

func parseSessionDropForm(w http.ResponseWriter, r *http.Request) error {
	const maxDropBytes = 256 << 20
	if r == nil {
		return fmt.Errorf("request is missing")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxDropBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return fmt.Errorf("invalid multipart body: %w", err)
	}
	return nil
}

func sessionDropRetention() (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv("CHROTE_SESSION_DROPS_RETENTION"))
	if raw == "" {
		return defaultSessionDropRetention, nil
	}
	retention, err := time.ParseDuration(raw)
	if err != nil || retention < 0 {
		return 0, fmt.Errorf("invalid CHROTE_SESSION_DROPS_RETENTION %q", raw)
	}
	return retention, nil
}

func sessionDropMaintenanceInterval() (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv("CHROTE_SESSION_DROPS_MAINTENANCE_INTERVAL"))
	if raw == "" {
		return defaultSessionDropMaintenanceInterval, nil
	}
	interval, err := time.ParseDuration(raw)
	if err != nil || interval <= 0 {
		return 0, fmt.Errorf("invalid CHROTE_SESSION_DROPS_MAINTENANCE_INTERVAL %q", raw)
	}
	return interval, nil
}

func (h *TmuxHandler) lockSessionDrops(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-h.sessionDropSem:
		return nil
	}
}

func (h *TmuxHandler) unlockSessionDrops() {
	h.sessionDropSem <- struct{}{}
}

func (h *TmuxHandler) maintainSessionDrops(ctx context.Context, now time.Time) error {
	if err := h.lockSessionDrops(ctx); err != nil {
		return err
	}
	defer h.unlockSessionDrops()
	return maintainSessionDropsContext(ctx, defaultSessionDropsPath(), now)
}

// StartSessionDropJanitor hardens legacy drops synchronously before serving and
// removes expired drops periodically until ctx is cancelled. The returned
// channel closes after all janitor work has stopped.
func (h *TmuxHandler) StartSessionDropJanitor(ctx context.Context, report func(error)) (<-chan struct{}, error) {
	interval, err := sessionDropMaintenanceInterval()
	if err != nil {
		if report != nil {
			report(fmt.Errorf("invalid session drop maintenance interval; using %s: %w", defaultSessionDropMaintenanceInterval, err))
		}
		interval = defaultSessionDropMaintenanceInterval
	}
	done := make(chan struct{})
	initialErr := h.maintainSessionDrops(ctx, time.Now())
	if ctx.Err() != nil {
		close(done)
		return done, errors.Join(initialErr, ctx.Err())
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if err := h.maintainSessionDrops(ctx, now); err != nil && report != nil && !errors.Is(err, context.Canceled) {
					report(err)
				}
			}
		}
	}()
	return done, initialErr
}

func setSessionDropACLContext(parent context.Context, path, unixUser, permissions string, reset bool) error {
	unixUser = strings.TrimSpace(unixUser)
	if unixUser != "" && !sessionDropUnixUserPattern.MatchString(unixUser) {
		return fmt.Errorf("invalid session drop Unix user %q", unixUser)
	}
	args := []string{"-k"}
	if reset {
		args = []string{"-b", "-k"}
	}
	args = append(args, "-m", "g::---", "-m", "o::---")
	if unixUser != "" {
		args = append(args, "-m", "u:"+unixUser+":"+permissions)
	}
	args = append(args, "--", path)
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "setfacl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("set session drop ACL: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func rebuildSessionDropRootACLContext(parent context.Context, path string, unixUsers []string) error {
	if err := parent.Err(); err != nil {
		return err
	}
	args := []string{"-b", "-k", "-m", "g::---", "-m", "o::---"}
	for _, unixUser := range unixUsers {
		if !sessionDropUnixUserPattern.MatchString(unixUser) {
			return fmt.Errorf("invalid session drop Unix user %q", unixUser)
		}
		args = append(args, "-m", "u:"+unixUser+":--x")
	}
	args = append(args, "--", path)
	// setfacl computes the full access ACL before applying it, so cancellation
	// leaves either the prior ACL or this complete retained-user set.
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "setfacl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("rebuild session drop root ACL: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func ensureSessionDropRoot(dropRoot string) error {
	if strings.TrimSpace(dropRoot) == "" {
		return fmt.Errorf("session drops path is empty")
	}
	info, err := os.Lstat(dropRoot)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(dropRoot, 0o700); err != nil {
			return fmt.Errorf("create session drop root: %w", err)
		}
		info, err = os.Lstat(dropRoot)
	}
	if err != nil {
		return fmt.Errorf("inspect session drop root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("session drop root must be a real directory")
	}
	if err := os.Chmod(dropRoot, 0o700); err != nil {
		return fmt.Errorf("secure session drop root: %w", err)
	}
	return nil
}

func secureSessionDropTree(dropRoot, dropPath, unixUser string) error {
	return secureSessionDropTreeContext(context.Background(), dropRoot, dropPath, unixUser)
}

func secureSessionDropTreeContext(ctx context.Context, dropRoot, dropPath, unixUser string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ensureSessionDropRoot(dropRoot); err != nil {
		return err
	}
	if err := setSessionDropACLContext(ctx, dropRoot, unixUser, "--x", false); err != nil {
		return err
	}
	return secureSessionDropPathContext(ctx, dropPath, unixUser)
}

func secureSessionDropPathContext(ctx context.Context, dropPath, unixUser string) error {
	return filepath.Walk(dropPath, func(path string, info os.FileInfo, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("session drop contains symbolic link %q", path)
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("session drop contains non-regular file %q", path)
		}
		mode := os.FileMode(0o600)
		permissions := "r--"
		if info.IsDir() {
			mode = 0o700
			permissions = "r-x"
		}
		// Close the owning-group mask before rebuilding the ACL from a known base.
		if err := os.Chmod(path, mode); err != nil {
			return err
		}
		return setSessionDropACLContext(ctx, path, unixUser, permissions, true)
	})
}

func readSessionDropManifest(path string) (sessionDropManifest, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if os.IsNotExist(err) {
		return sessionDropManifest{}, nil
	}
	if err != nil {
		return sessionDropManifest{}, fmt.Errorf("open manifest without following links: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return sessionDropManifest{}, fmt.Errorf("inspect manifest: %w", err)
	}
	if !info.Mode().IsRegular() {
		return sessionDropManifest{}, fmt.Errorf("manifest must be a regular file")
	}
	manifest := sessionDropManifest{}
	if err := json.NewDecoder(io.LimitReader(file, 1<<20)).Decode(&manifest); err != nil {
		return sessionDropManifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	return manifest, nil
}

type sessionDropMaintenanceEntry struct {
	name     string
	path     string
	unixUser string
	expired  bool
	process  bool
}

func removeSessionDropTreeContext(ctx context.Context, root string) error {
	paths := []string{}
	if err := filepath.Walk(root, func(path string, _ os.FileInfo, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		return err
	}
	for index := len(paths) - 1; index >= 0; index-- {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.Remove(paths[index]); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func maintainSessionDropsContext(ctx context.Context, dropRoot string, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ensureSessionDropRoot(dropRoot); err != nil {
		return err
	}
	retention, retentionConfigErr := sessionDropRetention()
	if retentionConfigErr != nil {
		retention = defaultSessionDropRetention
	}
	maintenanceErrors := []error{}
	if retentionConfigErr != nil {
		maintenanceErrors = append(maintenanceErrors, retentionConfigErr)
	}
	entries, err := os.ReadDir(dropRoot)
	if err != nil {
		return fmt.Errorf("read session drops: %w", err)
	}

	inventory := make([]sessionDropMaintenanceEntry, 0, len(entries))
	retainedUsers := map[string]struct{}{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(maintenanceErrors, err)...)
		}
		record := sessionDropMaintenanceEntry{name: entry.Name(), path: filepath.Join(dropRoot, entry.Name())}
		info, infoErr := entry.Info()
		if infoErr != nil {
			maintenanceErrors = append(maintenanceErrors, fmt.Errorf("inspect session drop %q: %w", entry.Name(), infoErr))
			inventory = append(inventory, record)
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			maintenanceErrors = append(maintenanceErrors, fmt.Errorf("unsupported entry in session drop root %q", entry.Name()))
			inventory = append(inventory, record)
			continue
		}
		record.process = true
		record.expired = info.IsDir() && validSessionDropID(entry.Name()) && retention > 0 && now.Sub(info.ModTime()) > retention
		if info.IsDir() && !record.expired {
			manifest, manifestErr := readSessionDropManifest(filepath.Join(record.path, "manifest.json"))
			if manifestErr != nil {
				maintenanceErrors = append(maintenanceErrors, fmt.Errorf("read existing session drop %q manifest: %w", entry.Name(), manifestErr))
			} else if manifest.UnixUser != "" && !sessionDropUnixUserPattern.MatchString(manifest.UnixUser) {
				maintenanceErrors = append(maintenanceErrors, fmt.Errorf("invalid session drop Unix user %q in %q", manifest.UnixUser, entry.Name()))
			} else if manifest.UnixUser != "" {
				account, lookupErr := tmuxLookupUser(manifest.UnixUser)
				if lookupErr != nil || account == nil || strings.TrimSpace(account.Uid) == "" {
					if lookupErr == nil {
						lookupErr = fmt.Errorf("account has no numeric UID")
					}
					maintenanceErrors = append(maintenanceErrors, fmt.Errorf("resolve session drop Unix user %q in %q: %w", manifest.UnixUser, entry.Name(), lookupErr))
				} else {
					record.unixUser = manifest.UnixUser
					retainedUsers[record.unixUser] = struct{}{}
				}
			}
		}
		inventory = append(inventory, record)
	}

	users := make([]string, 0, len(retainedUsers))
	for unixUser := range retainedUsers {
		users = append(users, unixUser)
	}
	sort.Strings(users)
	if err := rebuildSessionDropRootACLContext(ctx, dropRoot, users); err != nil {
		return errors.Join(append(maintenanceErrors, err)...)
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(append(maintenanceErrors, err)...)
	}

	for _, record := range inventory {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(maintenanceErrors, err)...)
		}
		if !record.process {
			continue
		}
		if record.expired {
			if err := removeSessionDropTreeContext(ctx, record.path); err != nil {
				if ctx.Err() != nil {
					return errors.Join(append(maintenanceErrors, ctx.Err())...)
				}
				maintenanceErrors = append(maintenanceErrors, fmt.Errorf("remove expired session drop %q: %w", record.name, err))
			}
			continue
		}
		if err := secureSessionDropPathContext(ctx, record.path, record.unixUser); err != nil {
			if ctx.Err() != nil {
				return errors.Join(append(maintenanceErrors, ctx.Err())...)
			}
			maintenanceErrors = append(maintenanceErrors, fmt.Errorf("secure existing session drop %q: %w", record.name, err))
		}
	}
	return errors.Join(maintenanceErrors...)
}

func writeSessionDrop(r *http.Request, sessionName string, target tmuxTarget, pane sendPaneTarget) (manifest sessionDropManifest, err error) {
	if r == nil {
		return sessionDropManifest{}, fmt.Errorf("request is missing")
	}
	text := strings.TrimRight(strings.ReplaceAll(sessionDropFormValue(r, "text"), "\r\n", "\n"), "\x00")
	fileHeaders := []*multipart.FileHeader{}
	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		fileHeaders = append(fileHeaders, r.MultipartForm.File["files"]...)
		fileHeaders = append(fileHeaders, r.MultipartForm.File["file"]...)
	}
	if strings.TrimSpace(text) == "" && len(fileHeaders) == 0 {
		return sessionDropManifest{}, errEmptySessionDrop
	}

	dropID, err := newSessionDropID()
	if err != nil {
		return sessionDropManifest{}, fmt.Errorf("create drop id: %w", err)
	}
	dropRoot := defaultSessionDropsPath()
	if err := ensureSessionDropRoot(dropRoot); err != nil {
		return sessionDropManifest{}, err
	}
	dropPath := filepath.Join(dropRoot, dropID)
	filesDir := filepath.Join(dropPath, "files")
	if err := os.MkdirAll(filesDir, 0o700); err != nil {
		return sessionDropManifest{}, fmt.Errorf("create drop directory: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(dropPath)
		}
	}()

	manifest = sessionDropManifest{
		ID:        dropID,
		Session:   sessionName,
		UnixUser:  target.unixUser,
		PaneID:    pane.PaneID,
		PanePID:   pane.PanePID,
		ServerPID: pane.ServerPID,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Payload:   filepath.Join(dropPath, "payload.txt"),
		Files:     []sessionDropFile{},
	}
	if text != "" {
		manifest.TextPath = filepath.Join(dropPath, "text.txt")
		if err := os.WriteFile(manifest.TextPath, []byte(text), 0o600); err != nil {
			return sessionDropManifest{}, fmt.Errorf("write drop text: %w", err)
		}
	}

	usedNames := map[string]int{}
	for idx, header := range fileHeaders {
		if header == nil {
			continue
		}
		fallback := fmt.Sprintf("file-%d", idx+1)
		cleanName := uniqueDropFileName(usedNames, sanitizeDropFileName(header.Filename, fallback))
		destPath := filepath.Join(filesDir, cleanName)
		src, err := header.Open()
		if err != nil {
			return sessionDropManifest{}, fmt.Errorf("open uploaded file %q: %w", header.Filename, err)
		}
		dest, err := os.OpenFile(destPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			_ = src.Close()
			return sessionDropManifest{}, fmt.Errorf("create uploaded file %q: %w", cleanName, err)
		}
		written, copyErr := io.Copy(dest, src)
		closeDestErr := dest.Close()
		closeSrcErr := src.Close()
		if copyErr != nil {
			return sessionDropManifest{}, fmt.Errorf("write uploaded file %q: %w", cleanName, copyErr)
		}
		if closeDestErr != nil {
			return sessionDropManifest{}, fmt.Errorf("close uploaded file %q: %w", cleanName, closeDestErr)
		}
		if closeSrcErr != nil {
			return sessionDropManifest{}, fmt.Errorf("close uploaded source %q: %w", header.Filename, closeSrcErr)
		}
		manifest.Files = append(manifest.Files, sessionDropFile{
			Name:        cleanName,
			Original:    filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/")),
			Path:        destPath,
			Size:        written,
			ContentType: header.Header.Get("Content-Type"),
		})
	}

	sections := []string{}
	if trimmedText := strings.TrimRight(text, "\n"); trimmedText != "" {
		sections = append(sections, trimmedText)
	}
	sections = append(sections, "CHROTE stored this send at:\n- "+dropPath)
	if len(manifest.Files) > 0 {
		fileSection := "Files:\n"
		for _, file := range manifest.Files {
			fileSection += "- " + file.Path + "\n"
		}
		sections = append(sections, strings.TrimRight(fileSection, "\n"))
	}
	payload := strings.Join(sections, "\n\n")
	manifest.submitEvidence, _ = buildSubmitPayloadEvidence(payload)
	if err := os.WriteFile(manifest.Payload, []byte(payload), 0o600); err != nil {
		return sessionDropManifest{}, fmt.Errorf("write drop payload: %w", err)
	}

	manifestPath := filepath.Join(dropPath, "manifest.json")
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return sessionDropManifest{}, fmt.Errorf("marshal drop manifest: %w", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(manifestPath, raw, 0o600); err != nil {
		return sessionDropManifest{}, fmt.Errorf("write drop manifest: %w", err)
	}
	if err := secureSessionDropTree(dropRoot, dropPath, target.unixUser); err != nil {
		return sessionDropManifest{}, err
	}
	complete = true
	return manifest, nil
}

func sessionDropFormValue(r *http.Request, key string) string {
	if r == nil || r.MultipartForm == nil {
		return ""
	}
	values := r.MultipartForm.Value[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func submitFormValue(raw string) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}
