package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chrote/server/internal/core"
)

const (
	exactLaunchCreationIDEnv = "CHROTE_EXACT_LAUNCH_ID"
	exactLaunchDigestEnv     = "CHROTE_EXACT_LAUNCH_DIGEST"
	exactLaunchMaxArgs       = 64
	exactLaunchMaxArgBytes   = 16 << 10
)

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

func exactLaunchArgv(raw []string) ([]string, *exactLaunchError) {
	if len(raw) == 0 || len(raw) > exactLaunchMaxArgs {
		return nil, &exactLaunchError{Status: http.StatusBadRequest, Code: "EXACT_LAUNCH_ARGV_INVALID", Message: "exact launch argv must contain between 1 and 64 arguments"}
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
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return nil, &exactLaunchError{Status: http.StatusBadRequest, Code: "EXACT_LAUNCH_EXECUTABLE_INVALID", Message: "exact launch executable is not an executable regular file"}
	}
	argv[0] = executable
	if len(argv) == 1 {
		envBin, err := exec.LookPath("env")
		if err != nil {
			return nil, &exactLaunchError{Status: http.StatusNotImplemented, Code: "EXACT_LAUNCH_ARGV_UNSUPPORTED", Message: "single-argument direct launch requires env"}
		}
		envBin, err = filepath.Abs(envBin)
		if err != nil {
			return nil, &exactLaunchError{Status: http.StatusNotImplemented, Code: "EXACT_LAUNCH_ARGV_UNSUPPORTED", Message: "single-argument direct launch helper is invalid"}
		}
		argv = []string{envBin, "--", executable}
	}
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
		return "", err
	}
	prefix := name + "="
	line := strings.TrimSpace(output)
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("tmux session environment %s is absent", name)
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

// ExactLaunch handles PUT /api/tmux/recovery-launches/{launchId}.
func (h *TmuxHandler) ExactLaunch(w http.ResponseWriter, r *http.Request) {
	launchID := strings.TrimSpace(r.PathValue("launchId"))
	if !recoveryUUIDRegex.MatchString(launchID) {
		core.WriteError(w, http.StatusBadRequest, "EXACT_LAUNCH_ID_INVALID", "exact launch ID must be a UUID")
		return
	}
	var req ExactLaunchRequest
	if err := decodeOptionalJSONBodyLimited(w, r, &req, exactLaunchMaxArgBytes+4096); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid exact launch request")
		return
	}
	if valid, message := core.ValidateSessionName(strings.TrimSpace(req.Name), "session name"); !valid || isReservedInternalSessionName(req.Name) {
		if message == "" {
			message = "session name is reserved"
		}
		core.WriteError(w, http.StatusBadRequest, "EXACT_LAUNCH_NAME_INVALID", message)
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
	cwd, cwdErr := exactLaunchCWD(req.CWD)
	if cwdErr != nil {
		writeExactLaunchError(w, cwdErr)
		return
	}
	argv, argvErr := exactLaunchArgv(req.Argv)
	if argvErr != nil {
		writeExactLaunchError(w, argvErr)
		return
	}
	digest := exactLaunchDigest(req.SourceID, target.unixUser, req.Name, cwd, argv)

	h.recoveryMu.Lock()
	defer h.recoveryMu.Unlock()

	sessions, sourceErr := h.listSessionsForTarget(target)
	if sourceErr != "" {
		core.WriteError(w, http.StatusConflict, "TMUX_SOURCE_UNAVAILABLE", "exact launch source is not currently authoritative")
		return
	}
	currentGeneration := tmuxSourceGeneration(target.unixUser, sessions)
	if existing, exists := exactLaunchSessionByName(sessions, req.Name); exists {
		receipt, owned, replayErr := h.replayExactLaunch(target, existing, launchID, digest, cwd)
		if replayErr != nil {
			writeExactLaunchError(w, replayErr)
			return
		}
		if !owned {
			core.WriteError(w, http.StatusConflict, "EXACT_LAUNCH_COLLISION", "exact launch session name is already in use")
			return
		}
		receipt.Success = true
		receipt.State = "replayed"
		receipt.LaunchID = launchID
		receipt.SourceID = req.SourceID
		receipt.SourceGeneration = req.SourceGeneration
		receipt.UnixUser = target.unixUser
		receipt.ArgvSHA256 = digest
		receipt.Timestamp = time.Now().UTC().Format(time.RFC3339)
		core.WriteJSON(w, http.StatusOK, receipt)
		return
	}
	if strings.TrimSpace(req.SourceGeneration) == "" || req.SourceGeneration != currentGeneration {
		core.WriteError(w, http.StatusConflict, "TMUX_SOURCE_CHANGED", "exact launch source changed after inspection")
		return
	}
	ownerHome := ""
	if strings.TrimSpace(target.ownerHome) != "" {
		ownerHome, err = trustedSessionBankOwnerHome(target)
		if err != nil {
			core.WriteError(w, http.StatusBadRequest, "EXACT_LAUNCH_OWNER_INVALID", err.Error())
			return
		}
	}
	if err := h.ensureTmuxNameOwnershipAvailable(req.Name, target.unixUser, ownerHome); err != nil {
		writeRecoveryOwnershipError(w, "EXACT_LAUNCH_OWNERSHIP_CONFLICT", "SESSION_BANK_ERROR", err)
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
