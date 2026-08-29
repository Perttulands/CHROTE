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
	"strings"
	"time"
)

const defaultSessionDropsDir = "/srv/data/chrote/session-drops"

var sessionDropUnixUserPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*[$]?$`)
var errEmptySessionDrop = errors.New("send text or at least one file")

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
	ID        string            `json:"id"`
	Session   string            `json:"session"`
	UnixUser  string            `json:"unixUser,omitempty"`
	PaneID    string            `json:"paneId"`
	PanePID   string            `json:"panePid"`
	ServerPID string            `json:"serverPid"`
	CreatedAt string            `json:"createdAt"`
	TextPath  string            `json:"textPath,omitempty"`
	Payload   string            `json:"payload"`
	Files     []sessionDropFile `json:"files"`
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

func grantSessionDrop(path, unixUser string) error {
	unixUser = strings.TrimSpace(unixUser)
	if !sessionDropUnixUserPattern.MatchString(unixUser) {
		return fmt.Errorf("invalid session drop Unix user %q", unixUser)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "setfacl", "-P", "-R", "-m", "u:"+unixUser+":r-X", "--", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("grant session drop: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func ensureSessionDropRoot(dropRoot string) error {
	if strings.TrimSpace(dropRoot) == "" {
		return fmt.Errorf("session drops path is empty")
	}
	info, err := os.Lstat(dropRoot)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(dropRoot, 0o711); err != nil {
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
	// The root is traverse-only to callers who already know a random drop ID.
	// Fresh child directories and files remain 0700/0600 until the target user
	// receives the single recursive read/traverse grant after the write completes.
	if err := os.Chmod(dropRoot, 0o711); err != nil {
		return fmt.Errorf("set session drop root traversal mode: %w", err)
	}
	return nil
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
	if err := grantSessionDrop(dropPath, target.unixUser); err != nil {
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
