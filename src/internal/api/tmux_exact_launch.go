package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chrote/server/internal/core"
)

const (
	exactLaunchCreationIDEnv = "CHROTE_EXACT_LAUNCH_ID"
	exactLaunchDigestEnv     = "CHROTE_EXACT_LAUNCH_DIGEST"
	exactLaunchMaxArgs       = 64
	exactLaunchMaxArgBytes   = 16 << 10
)

var errExactLaunchEnvironmentAbsent = errors.New("exact launch session environment marker is absent")

type ExactLaunchRequest struct {
	SourceID         string   `json:"sourceId"`
	SourceGeneration string   `json:"sourceGeneration"`
	UnixUser         string   `json:"unixUser,omitempty"`
	Name             string   `json:"name"`
	CWD              string   `json:"cwd"`
	Argv             []string `json:"argv"`
}

type ExactLaunchReceipt struct {
	Success          bool   `json:"success"`
	State            string `json:"state"`
	LaunchID         string `json:"launchId"`
	SourceID         string `json:"sourceId"`
	SourceGeneration string `json:"sourceGeneration"`
	UnixUser         string `json:"unixUser,omitempty"`
	SessionID        string `json:"sessionId"`
	SessionName      string `json:"sessionName"`
	PaneID           string `json:"paneId"`
	PanePID          int    `json:"panePid"`
	ProcessStart     string `json:"processStart"`
	CWD              string `json:"cwd"`
	ArgvSHA256       string `json:"argvSha256"`
	Timestamp        string `json:"timestamp"`
}

type exactLaunchError struct {
	Status  int
	Code    string
	Message string
}

func (e *exactLaunchError) Error() string {
	if e == nil {
		return "exact launch failed"
	}
	return e.Message
}

type ownedExactLaunch struct {
	Session ownedTmuxSession
	PaneID  string
	PanePID int
}

var exactLaunchProcessStartTime = readProcessStartTime

func readProcessStartTime(pid int) (string, error) {
	if pid <= 0 {
		return "", fmt.Errorf("invalid process pid %d", pid)
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(raw))
	closeParen := strings.LastIndex(text, ")")
	if closeParen < 0 {
		return "", fmt.Errorf("process stat is malformed")
	}
	fields := strings.Fields(text[closeParen+1:])
	if len(fields) <= 19 {
		return "", fmt.Errorf("process stat is missing start time")
	}
	if _, err := strconv.ParseUint(fields[19], 10, 64); err != nil {
		return "", fmt.Errorf("process start time is malformed")
	}
	return fields[19], nil
}

func exactLaunchDigest(sourceID, unixUser, name, cwd string, argv []string) string {
	raw, _ := json.Marshal(struct {
		SourceID string   `json:"sourceId"`
		UnixUser string   `json:"unixUser"`
		Name     string   `json:"name"`
		CWD      string   `json:"cwd"`
		Argv     []string `json:"argv"`
	}{sourceID, unixUser, name, cwd, argv})
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func exactLaunchCWD(raw string) (string, *exactLaunchError) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !filepath.IsAbs(raw) {
		return "", &exactLaunchError{Status: http.StatusBadRequest, Code: "EXACT_LAUNCH_CWD_INVALID", Message: "exact launch cwd must be an absolute existing directory"}
	}
	resolved, err := filepath.EvalSymlinks(raw)
	if err != nil {
		return "", &exactLaunchError{Status: http.StatusBadRequest, Code: "EXACT_LAUNCH_CWD_INVALID", Message: "exact launch cwd could not be resolved"}
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", &exactLaunchError{Status: http.StatusBadRequest, Code: "EXACT_LAUNCH_CWD_INVALID", Message: "exact launch cwd is invalid"}
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", &exactLaunchError{Status: http.StatusBadRequest, Code: "EXACT_LAUNCH_CWD_INVALID", Message: "exact launch cwd must be an existing directory"}
	}
	allowed := false
	for _, root := range core.GetAllowedRoots() {
		canonicalRoot, rootErr := filepath.EvalSymlinks(root)
		if rootErr != nil {
			continue
		}
		if core.IsPathUnderRoot(resolved, canonicalRoot) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", &exactLaunchError{Status: http.StatusForbidden, Code: "EXACT_LAUNCH_CWD_FORBIDDEN", Message: "exact launch cwd is outside configured roots"}
	}
	return filepath.Clean(resolved), nil
}

func exactLaunchExecutableRoots(ownerHome string) []string {
	raw := strings.TrimSpace(os.Getenv("CHROTE_EXACT_LAUNCH_EXECUTABLE_ROOTS"))
	if raw == "" {
		raw = "/usr/bin,/bin,/usr/local/bin"
		if strings.TrimSpace(ownerHome) != "" {
			raw += "," + ownerHome
		}
	}
	roots := []string{}
	for _, candidate := range strings.Split(raw, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || !filepath.IsAbs(candidate) {
			continue
		}
		canonical, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		roots = append(roots, filepath.Clean(canonical))
	}
	return roots
}

type exactLaunchPathIdentity struct {
	Path     string
	Device   uint64
	Inode    uint64
	Mode     uint32
	UID      uint32
	GID      uint32
	Size     int64
	Modified syscall.Timespec
	Changed  syscall.Timespec
}

func readExactLaunchPathIdentity(path string) (exactLaunchPathIdentity, error) {
	info, err := os.Stat(path)
	if err != nil {
		return exactLaunchPathIdentity{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return exactLaunchPathIdentity{}, fmt.Errorf("path identity is unavailable")
	}
	return exactLaunchPathIdentity{
		Path: filepath.Clean(path), Device: uint64(stat.Dev), Inode: stat.Ino,
		Mode: stat.Mode, UID: stat.Uid, GID: stat.Gid, Size: stat.Size,
		Modified: stat.Mtim, Changed: stat.Ctim,
	}, nil
}

func exactLaunchArgv(raw []string, ownerHome string) ([]string, *exactLaunchError) {
	if len(raw) < 2 || len(raw) > exactLaunchMaxArgs {
		return nil, &exactLaunchError{Status: http.StatusBadRequest, Code: "EXACT_LAUNCH_ARGV_INVALID", Message: "exact launch argv must contain an executable and at least one argument, with at most 64 arguments"}
	}
	total := 0
	argv := make([]string, len(raw))
	for i, arg := range raw {
		if arg == "" && i == 0 {
			return nil, &exactLaunchError{Status: http.StatusBadRequest, Code: "EXACT_LAUNCH_ARGV_INVALID", Message: "exact launch executable is required"}
		}
		if strings.ContainsRune(arg, '\x00') {
			return nil, &exactLaunchError{Status: http.StatusBadRequest, Code: "EXACT_LAUNCH_ARGV_INVALID", Message: "exact launch argv contains a NUL byte"}
		}
		if arg == ";" || arg == "\\;" {
			return nil, &exactLaunchError{Status: http.StatusBadRequest, Code: "EXACT_LAUNCH_ARGV_INVALID", Message: "exact launch argv contains a tmux command separator"}
		}
		total += len(arg)
		if total > exactLaunchMaxArgBytes {
			return nil, &exactLaunchError{Status: http.StatusRequestEntityTooLarge, Code: "EXACT_LAUNCH_ARGV_TOO_LARGE", Message: "exact launch argv exceeds 16 KiB"}
		}
		argv[i] = arg
	}
	if !filepath.IsAbs(argv[0]) {
		return nil, &exactLaunchError{Status: http.StatusBadRequest, Code: "EXACT_LAUNCH_EXECUTABLE_INVALID", Message: "exact launch executable must be absolute"}
	}
	executable, err := filepath.EvalSymlinks(argv[0])
	if err != nil {
		return nil, &exactLaunchError{Status: http.StatusBadRequest, Code: "EXACT_LAUNCH_EXECUTABLE_INVALID", Message: "exact launch executable could not be resolved"}
	}
	info, err := os.Stat(executable)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 || info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
		return nil, &exactLaunchError{Status: http.StatusBadRequest, Code: "EXACT_LAUNCH_EXECUTABLE_INVALID", Message: "exact launch executable is not a non-setid executable regular file"}
	}
	allowed := false
	for _, root := range exactLaunchExecutableRoots(ownerHome) {
		if core.IsPathUnderRoot(executable, root) {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, &exactLaunchError{Status: http.StatusForbidden, Code: "EXACT_LAUNCH_EXECUTABLE_FORBIDDEN", Message: "exact launch executable is outside configured executable roots"}
	}
	argv[0] = filepath.Clean(executable)
	return argv, nil
}

func parseOwnedExactLaunch(output, name string, token string) (ownedExactLaunch, error) {
	parts := strings.Split(strings.TrimSpace(output), "\t")
	if len(parts) != 3 {
		return ownedExactLaunch{}, fmt.Errorf("tmux exact launch output should contain session ID, pane ID, and pane PID")
	}
	if !tmuxSessionIDPattern.MatchString(parts[0]) || !tmuxPaneIDPattern.MatchString(parts[1]) || !tmuxPIDPattern.MatchString(parts[2]) {
		return ownedExactLaunch{}, fmt.Errorf("tmux exact launch output contains invalid identities")
	}
	pid, _ := strconv.Atoi(parts[2])
	return ownedExactLaunch{
		Session: ownedTmuxSession{ID: parts[0], Name: name, Token: token},
		PaneID:  parts[1],
		PanePID: pid,
	}, nil
}

func (h *TmuxHandler) createOwnedExactLaunch(parent context.Context, target tmuxTarget, launchID, name, cwd, digest string, argv []string) (ownedExactLaunch, error) {
	token, err := newTmuxCreationToken()
	if err != nil {
		return ownedExactLaunch{}, fmt.Errorf("generate tmux creation token: %w", err)
	}
	owned := ownedTmuxSession{Name: name, Token: token}
	args := []string{
		"new-session", "-d", "-P", "-F", "#{session_id}\t#{pane_id}\t#{pane_pid}",
		"-e", tmuxCreationTokenEnv + "=" + token,
		"-e", exactLaunchCreationIDEnv + "=" + launchID,
		"-e", exactLaunchDigestEnv + "=" + digest,
		"-s", name, "-c", cwd, "--",
	}
	args = append(args, argv...)
	output, createErr := h.runTmuxOnSocketContext(parent, target.socket, args...)
	if createErr != nil {
		return ownedExactLaunch{}, h.cleanupOwnedTmuxSessionAfterError(target.socket, owned, createErr)
	}
	created, parseErr := parseOwnedExactLaunch(output, name, token)
	if parseErr != nil {
		return ownedExactLaunch{}, h.cleanupOwnedTmuxSessionAfterError(target.socket, owned, parseErr)
	}
	return created, nil
}

func exactLaunchSessionByName(sessions []core.Session, name string) (core.Session, bool) {
	for _, session := range sessions {
		if session.Name == name {
			return session, true
		}
	}
	return core.Session{}, false
}

func (h *TmuxHandler) exactLaunchSessionEnvironment(target tmuxTarget, sessionID, name string) (string, error) {
	output, err := h.runTmuxOnSocket(target.socket, "show-environment", "-t", sessionID, name)
	if err != nil {
		diagnostic := strings.TrimSpace(tmuxErrorDiagnostic(err))
		diagnostic = strings.TrimPrefix(diagnostic, "exit status 1: ")
		if diagnostic == "unknown variable: "+name {
			return "", errExactLaunchEnvironmentAbsent
		}
		return "", err
	}
	prefix := name + "="
	line := strings.TrimSpace(output)
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("tmux session environment %s is malformed", name)
	}
	return strings.TrimPrefix(line, prefix), nil
}

func parseExactLaunchPane(output string) (ownedExactLaunch, string, error) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 || strings.TrimSpace(lines[0]) == "" {
		return ownedExactLaunch{}, "", fmt.Errorf("exact launch replay requires one pane")
	}
	parts := strings.Split(strings.TrimSpace(lines[0]), "	")
	if len(parts) != 5 || !tmuxSessionIDPattern.MatchString(parts[0]) || !tmuxPaneIDPattern.MatchString(parts[2]) || !tmuxPIDPattern.MatchString(parts[3]) {
		return ownedExactLaunch{}, "", fmt.Errorf("exact launch replay pane identities are invalid")
	}
	pid, _ := strconv.Atoi(parts[3])
	return ownedExactLaunch{
		Session: ownedTmuxSession{ID: parts[0], Name: parts[1]},
		PaneID:  parts[2],
		PanePID: pid,
	}, filepath.Clean(parts[4]), nil
}

func (h *TmuxHandler) replayExactLaunch(target tmuxTarget, existing core.Session, launchID, digest, cwd string) (ExactLaunchReceipt, bool, error) {
	storedLaunchID, err := h.exactLaunchSessionEnvironment(target, existing.ID, exactLaunchCreationIDEnv)
	if err != nil || storedLaunchID != launchID {
		return ExactLaunchReceipt{}, false, nil
	}
	storedDigest, err := h.exactLaunchSessionEnvironment(target, existing.ID, exactLaunchDigestEnv)
	if err != nil {
		return ExactLaunchReceipt{}, true, &exactLaunchError{Status: http.StatusConflict, Code: "EXACT_LAUNCH_REPLAY_UNVERIFIED", Message: "exact launch replay marker is incomplete"}
	}
	if storedDigest != digest {
		return ExactLaunchReceipt{}, true, &exactLaunchError{Status: http.StatusConflict, Code: "EXACT_LAUNCH_IDEMPOTENCY_CONFLICT", Message: "exact launch ID already owns a different launch specification"}
	}
	output, err := h.runTmuxOnSocket(target.socket, "list-panes", "-t", existing.ID, "-F", "#{session_id}	#{session_name}	#{pane_id}	#{pane_pid}	#{pane_current_path}")
	if err != nil {
		return ExactLaunchReceipt{}, true, &exactLaunchError{Status: http.StatusConflict, Code: "EXACT_LAUNCH_REPLAY_UNVERIFIED", Message: "exact launch target could not be re-read"}
	}
	owned, observedCWD, err := parseExactLaunchPane(output)
	if err != nil || owned.Session.ID != existing.ID || owned.Session.Name != existing.Name || observedCWD != cwd {
		return ExactLaunchReceipt{}, true, &exactLaunchError{Status: http.StatusConflict, Code: "EXACT_LAUNCH_REPLAY_UNVERIFIED", Message: "exact launch target changed after creation"}
	}
	start, err := exactLaunchProcessStartTime(owned.PanePID)
	if err != nil {
		return ExactLaunchReceipt{}, true, &exactLaunchError{Status: http.StatusConflict, Code: "EXACT_LAUNCH_REPLAY_UNVERIFIED", Message: "exact launch process identity is unavailable"}
	}
	return ExactLaunchReceipt{
		SessionID:    owned.Session.ID,
		SessionName:  owned.Session.Name,
		PaneID:       owned.PaneID,
		PanePID:      owned.PanePID,
		ProcessStart: start,
		CWD:          observedCWD,
	}, true, nil
}

func (h *TmuxHandler) sessionOwnedByExactLaunchID(target tmuxTarget, sessions []core.Session, launchID string) (core.Session, bool, error) {
	var matched core.Session
	found := false
	for _, session := range sessions {
		stored, err := h.exactLaunchSessionEnvironment(target, session.ID, exactLaunchCreationIDEnv)
		if errors.Is(err, errExactLaunchEnvironmentAbsent) {
			continue
		}
		if err != nil {
			return core.Session{}, false, &exactLaunchError{Status: http.StatusConflict, Code: "TMUX_SOURCE_UNAVAILABLE", Message: "exact launch ownership markers could not be read authoritatively"}
		}
		if stored != launchID {
			continue
		}
		if found {
			return core.Session{}, false, &exactLaunchError{Status: http.StatusConflict, Code: "EXACT_LAUNCH_IDEMPOTENCY_CONFLICT", Message: "exact launch ID is attached to multiple sessions"}
		}
		matched = session
		found = true
	}
	return matched, found, nil
}

func (h *TmuxHandler) exactLaunchIDExistsOnOtherSources(selected tmuxTarget, launchID string) (bool, error) {
	for _, unixUser := range configuredTerminalUsers() {
		if unixUser == selected.unixUser {
			continue
		}
		target, err := h.targetForUnixUser(unixUser)
		if err != nil {
			return false, &exactLaunchError{Status: http.StatusConflict, Code: "TMUX_SOURCE_UNAVAILABLE", Message: "exact launch ownership could not be verified across configured sources"}
		}
		sessions, sourceErr, _ := h.listSessionsForTarget(target)
		if sourceErr != "" {
			return false, &exactLaunchError{Status: http.StatusConflict, Code: "TMUX_SOURCE_UNAVAILABLE", Message: "exact launch ownership could not be verified across configured sources"}
		}
		_, found, err := h.sessionOwnedByExactLaunchID(target, sessions, launchID)
		if err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

func (h *TmuxHandler) verifyExactLaunch(parent context.Context, target tmuxTarget, created ownedExactLaunch, cwd string) (ExactLaunchReceipt, error) {
	output, err := h.runTmuxOnSocketContext(parent, target.socket,
		"display-message", "-p", "-t", created.PaneID,
		"#{session_id}\t#{session_name}\t#{pane_id}\t#{pane_pid}\t#{pane_current_path}",
	)
	if err != nil {
		return ExactLaunchReceipt{}, err
	}
	parts := strings.Split(strings.TrimSpace(output), "\t")
	if len(parts) != 5 || parts[0] != created.Session.ID || parts[1] != created.Session.Name || parts[2] != created.PaneID || parts[3] != strconv.Itoa(created.PanePID) || filepath.Clean(parts[4]) != cwd {
		return ExactLaunchReceipt{}, fmt.Errorf("tmux exact launch identities did not verify")
	}
	start, err := exactLaunchProcessStartTime(created.PanePID)
	if err != nil {
		return ExactLaunchReceipt{}, fmt.Errorf("verify launched process identity: %w", err)
	}
	return ExactLaunchReceipt{
		SessionID:    created.Session.ID,
		SessionName:  created.Session.Name,
		PaneID:       created.PaneID,
		PanePID:      created.PanePID,
		ProcessStart: start,
		CWD:          cwd,
	}, nil
}

func writeExactLaunchError(w http.ResponseWriter, err error) {
	var launchErr *exactLaunchError
	if errors.As(err, &launchErr) {
		core.WriteError(w, launchErr.Status, launchErr.Code, launchErr.Message)
		return
	}
	core.WriteError(w, http.StatusInternalServerError, "EXACT_LAUNCH_FAILED", err.Error())
}

func validateExactLaunchJSONValue(key string, raw json.RawMessage) error {
	trimmed := bytes.TrimSpace(raw)
	if key == "argv" {
		if len(trimmed) == 0 || trimmed[0] != '[' {
			return fmt.Errorf("exact launch argv must be an array of strings")
		}
		var values []json.RawMessage
		if err := json.Unmarshal(trimmed, &values); err != nil {
			return err
		}
		for _, value := range values {
			var decoded interface{}
			if err := json.Unmarshal(value, &decoded); err != nil {
				return err
			}
			if _, ok := decoded.(string); !ok {
				return fmt.Errorf("exact launch argv must contain only strings")
			}
		}
		return nil
	}

	var decoded interface{}
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		return err
	}
	if _, ok := decoded.(string); !ok {
		return fmt.Errorf("exact launch field must be a string")
	}
	return nil
}

func decodeExactLaunchRequest(w http.ResponseWriter, r *http.Request, dest *ExactLaunchRequest) error {
	if r == nil || r.Body == nil {
		return fmt.Errorf("request body is required")
	}
	reader := http.MaxBytesReader(w, r.Body, exactLaunchMaxArgBytes+4096)
	raw, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	allowed := map[string]bool{
		"sourceId": true, "sourceGeneration": true, "unixUser": true,
		"name": true, "cwd": true, "argv": true,
	}
	keys := json.NewDecoder(bytes.NewReader(raw))
	opening, err := keys.Token()
	if err != nil || opening != json.Delim('{') {
		return fmt.Errorf("request body must contain one JSON object")
	}
	seen := map[string]bool{}
	for keys.More() {
		token, err := keys.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok || !allowed[key] {
			return fmt.Errorf("unknown exact launch field")
		}
		if seen[key] {
			return fmt.Errorf("duplicate exact launch field")
		}
		seen[key] = true
		var value json.RawMessage
		if err := keys.Decode(&value); err != nil {
			return err
		}
		if err := validateExactLaunchJSONValue(key, value); err != nil {
			return err
		}
	}
	if closing, err := keys.Token(); err != nil || closing != json.Delim('}') {
		return fmt.Errorf("request body must contain one JSON object")
	}
	if token, err := keys.Token(); err != io.EOF || token != nil {
		return fmt.Errorf("request body must contain one JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(dest)
}

// ExactLaunch handles PUT /api/tmux/recovery-launches/{launchId}.
func (h *TmuxHandler) ExactLaunch(w http.ResponseWriter, r *http.Request) {
	launchID := strings.TrimSpace(r.PathValue("launchId"))
	if !recoveryUUIDRegex.MatchString(launchID) {
		core.WriteError(w, http.StatusBadRequest, "EXACT_LAUNCH_ID_INVALID", "exact launch ID must be a UUID")
		return
	}
	var req ExactLaunchRequest
	if err := decodeExactLaunchRequest(w, r, &req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid exact launch request")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if valid, message := core.ValidateSessionName(req.Name, "session name"); !valid || isReservedInternalSessionName(req.Name) {
		if message == "" {
			message = "session name is reserved"
		}
		core.WriteError(w, http.StatusBadRequest, "EXACT_LAUNCH_NAME_INVALID", message)
		return
	}

	req.UnixUser = strings.TrimSpace(req.UnixUser)
	if req.UnixUser == "" {
		core.WriteError(w, http.StatusBadRequest, "EXACT_LAUNCH_USER_REQUIRED", "exact launch Unix user is required")
		return
	}
	target, err := sendTargetFromRequest(h, r, req.UnixUser)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if strings.TrimSpace(req.SourceID) != tmuxSourceID(target.unixUser) {
		core.WriteError(w, http.StatusBadRequest, "EXACT_LAUNCH_SOURCE_INVALID", "exact launch source does not match the configured Unix user")
		return
	}
	ownerHome, err := trustedSessionBankOwnerHome(target)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "EXACT_LAUNCH_OWNER_INVALID", err.Error())
		return
	}
	cwd, cwdErr := exactLaunchCWD(req.CWD)
	if cwdErr != nil {
		writeExactLaunchError(w, cwdErr)
		return
	}
	argv, argvErr := exactLaunchArgv(req.Argv, ownerHome)
	if argvErr != nil {
		writeExactLaunchError(w, argvErr)
		return
	}
	cwdIdentity, cwdIdentityErr := readExactLaunchPathIdentity(cwd)
	executableIdentity, executableIdentityErr := readExactLaunchPathIdentity(argv[0])
	if cwdIdentityErr != nil || executableIdentityErr != nil {
		core.WriteError(w, http.StatusConflict, "EXACT_LAUNCH_PATH_CHANGED", "exact launch path identity could not be captured")
		return
	}
	digest := exactLaunchDigest(req.SourceID, target.unixUser, req.Name, cwd, argv)

	h.recoveryMu.Lock()
	defer h.recoveryMu.Unlock()

	sessions, sourceErr, serverIdentity := h.listSessionsForTarget(target)
	if sourceErr != "" {
		core.WriteError(w, http.StatusConflict, "TMUX_SOURCE_UNAVAILABLE", "exact launch source is not currently authoritative")
		return
	}
	currentGeneration := tmuxSourceGeneration(target.unixUser, sessions, serverIdentity)
	if strings.TrimSpace(req.SourceGeneration) == "" || req.SourceGeneration != currentGeneration {
		core.WriteError(w, http.StatusConflict, "TMUX_SOURCE_CHANGED", "exact launch source changed after inspection")
		return
	}
	ownedByLaunchID, launchIDExists, launchIDErr := h.sessionOwnedByExactLaunchID(target, sessions, launchID)
	if launchIDErr != nil {
		writeExactLaunchError(w, launchIDErr)
		return
	}
	foreignLaunchIDExists, launchIDErr := h.exactLaunchIDExistsOnOtherSources(target, launchID)
	if launchIDErr != nil {
		writeExactLaunchError(w, launchIDErr)
		return
	}
	if foreignLaunchIDExists {
		core.WriteError(w, http.StatusConflict, "EXACT_LAUNCH_IDEMPOTENCY_CONFLICT", "exact launch ID already exists on another configured source")
		return
	}
	if launchIDExists {
		if ownedByLaunchID.Name != req.Name {
			core.WriteError(w, http.StatusConflict, "EXACT_LAUNCH_IDEMPOTENCY_CONFLICT", "exact launch ID already owns another session name")
			return
		}
		receipt, owned, replayErr := h.replayExactLaunch(target, ownedByLaunchID, launchID, digest, cwd)
		if replayErr != nil {
			writeExactLaunchError(w, replayErr)
			return
		}
		if !owned {
			core.WriteError(w, http.StatusConflict, "EXACT_LAUNCH_REPLAY_UNVERIFIED", "exact launch marker changed after discovery")
			return
		}
		receipt.Success = true
		receipt.State = "replayed"
		receipt.LaunchID = launchID
		receipt.SourceID = req.SourceID
		receipt.SourceGeneration = currentGeneration
		receipt.UnixUser = target.unixUser
		receipt.ArgvSHA256 = digest
		receipt.Timestamp = time.Now().UTC().Format(time.RFC3339)
		core.WriteJSON(w, http.StatusOK, receipt)
		return
	}
	if _, exists := exactLaunchSessionByName(sessions, req.Name); exists {
		core.WriteError(w, http.StatusConflict, "EXACT_LAUNCH_COLLISION", "exact launch session name is already in use")
		return
	}
	if err := h.ensureTmuxNameOwnershipAvailable(req.Name, target.unixUser, ownerHome); err != nil {
		writeRecoveryOwnershipError(w, "EXACT_LAUNCH_OWNERSHIP_CONFLICT", "SESSION_BANK_ERROR", err)
		return
	}

	refreshedSessions, sourceErr, refreshedServerIdentity := h.listSessionsForTarget(target)
	if sourceErr != "" {
		core.WriteError(w, http.StatusConflict, "TMUX_SOURCE_UNAVAILABLE", "exact launch source is not currently authoritative")
		return
	}
	refreshedGeneration := tmuxSourceGeneration(target.unixUser, refreshedSessions, refreshedServerIdentity)
	if req.SourceGeneration != refreshedGeneration {
		core.WriteError(w, http.StatusConflict, "TMUX_SOURCE_CHANGED", "exact launch source changed before mutation")
		return
	}
	if _, exists := exactLaunchSessionByName(refreshedSessions, req.Name); exists {
		core.WriteError(w, http.StatusConflict, "EXACT_LAUNCH_COLLISION", "exact launch session name is already in use")
		return
	}
	if _, exists, err := h.sessionOwnedByExactLaunchID(target, refreshedSessions, launchID); err != nil || exists {
		if err != nil {
			writeExactLaunchError(w, err)
		} else {
			core.WriteError(w, http.StatusConflict, "EXACT_LAUNCH_IDEMPOTENCY_CONFLICT", "exact launch ID appeared before mutation")
		}
		return
	}
	freshCWD, freshCWDErr := exactLaunchCWD(req.CWD)
	freshArgv, freshArgvErr := exactLaunchArgv(req.Argv, ownerHome)
	freshCWDIdentity, freshCWDIdentityErr := readExactLaunchPathIdentity(freshCWD)
	freshExecutableIdentity, freshExecutableIdentityErr := exactLaunchPathIdentity{}, error(nil)
	if len(freshArgv) > 0 {
		freshExecutableIdentity, freshExecutableIdentityErr = readExactLaunchPathIdentity(freshArgv[0])
	}
	if freshCWDErr != nil || freshArgvErr != nil || freshCWDIdentityErr != nil || freshExecutableIdentityErr != nil ||
		freshCWD != cwd || !slices.Equal(freshArgv, argv) || freshCWDIdentity != cwdIdentity || freshExecutableIdentity != executableIdentity {
		core.WriteError(w, http.StatusConflict, "EXACT_LAUNCH_PATH_CHANGED", "exact launch path identity changed before mutation")
		return
	}

	finalSessions, sourceErr, finalServerIdentity := h.listSessionsForTarget(target)
	if sourceErr != "" || tmuxSourceGeneration(target.unixUser, finalSessions, finalServerIdentity) != req.SourceGeneration {
		core.WriteError(w, http.StatusConflict, "TMUX_SOURCE_CHANGED", "exact launch source changed immediately before mutation")
		return
	}
	if _, exists := exactLaunchSessionByName(finalSessions, req.Name); exists {
		core.WriteError(w, http.StatusConflict, "EXACT_LAUNCH_COLLISION", "exact launch session name appeared immediately before mutation")
		return
	}
	if _, exists, err := h.sessionOwnedByExactLaunchID(target, finalSessions, launchID); err != nil || exists {
		if err != nil {
			writeExactLaunchError(w, err)
		} else {
			core.WriteError(w, http.StatusConflict, "EXACT_LAUNCH_IDEMPOTENCY_CONFLICT", "exact launch ID appeared immediately before mutation")
		}
		return
	}

	created, err := h.createOwnedExactLaunch(r.Context(), target, launchID, req.Name, cwd, digest, argv)
	if err != nil {
		if isTmuxDuplicateSessionError(err) {
			core.WriteError(w, http.StatusConflict, "EXACT_LAUNCH_COLLISION", "exact launch session name is already in use")
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "EXACT_LAUNCH_FAILED", err.Error())
		return
	}
	receipt, err := h.verifyExactLaunch(r.Context(), target, created, cwd)
	if err != nil {
		err = h.cleanupOwnedTmuxSessionAfterError(target.socket, created.Session, err)
		core.WriteError(w, http.StatusInternalServerError, "EXACT_LAUNCH_VERIFY_FAILED", err.Error())
		return
	}
	receipt.Success = true
	receipt.State = "launched"
	receipt.LaunchID = launchID
	receipt.SourceID = req.SourceID
	receipt.SourceGeneration = currentGeneration
	receipt.UnixUser = target.unixUser
	receipt.ArgvSHA256 = digest
	receipt.Timestamp = time.Now().UTC().Format(time.RFC3339)
	h.invalidateCache()
	core.WriteJSON(w, http.StatusOK, receipt)
}
