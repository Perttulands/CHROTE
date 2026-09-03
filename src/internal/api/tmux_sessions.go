// Package api provides HTTP handlers for the API
package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chrote/server/internal/core"
)

// TmuxHandler handles tmux-related API endpoints
type TmuxHandler struct {
	// proc locates the process filesystem used to recognise the ptys this
	// server spawned. Its zero value owns no pty, so a handler built without
	// it reports every attached client as foreign rather than as CHROTE's.
	proc procSource
	// launch is the only place the harness ids accepted on session creation
	// are defined, and the only place their commands live.
	launch LaunchConfig
	// hooks is how a launched harness is told to report its completion. Its
	// zero value installs nothing.
	hooks AgentHooks
	// events holds the last event each session's harness reported.
	events *agentEventStore
}

// SessionsResponse is the response for listing sessions
type SessionsResponse struct {
	Sessions      []core.Session            `json:"sessions"`
	Grouped       map[string][]core.Session `json:"grouped"`
	TerminalUsers []string                  `json:"terminalUsers"`
	Timestamp     string                    `json:"timestamp"`
	Error         string                    `json:"error,omitempty"`
	// Partial is true only when configured-user tmux collection is the sole
	// source of errors and at least one configured user succeeded.
	Partial         bool     `json:"partial,omitempty"`
	SuccessfulUsers []string `json:"successfulUsers,omitempty"`
	FailedUsers     []string `json:"failedUsers,omitempty"`
}

// CreateSessionRequest is the request body for creating a session
type CreateSessionRequest struct {
	Name        string `json:"name"`
	UnixUser    string `json:"unixUser,omitempty"`
	MouseScroll *bool  `json:"mouseScroll,omitempty"`
	// Cwd is where the session starts: absolute, or the home token for the
	// target Unix user's home. Empty keeps that user's configured workdir.
	Cwd string `json:"cwd,omitempty"`
	// Harness names a command from the launch configuration to start in the
	// new session. Empty and "shell" start nothing.
	Harness string `json:"harness,omitempty"`
	// Flags is the line typed after the harness's binary. Absent means the
	// harness's configured default flags; an empty string means none.
	Flags *string `json:"flags,omitempty"`
	// Notify asks for the harness's completion hooks to be installed, so the
	// session reports when its agent finishes or needs input. Absent means yes.
	Notify *bool `json:"notify,omitempty"`
}

// RenameSessionRequest is the request body for renaming a session
type RenameSessionRequest struct {
	NewName string `json:"newName"`
}

const reservedInternalSessionPrefix = "chrote-probe-"

func isReservedInternalSessionName(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), reservedInternalSessionPrefix)
}

// NewTmuxHandler creates the default tmux handler, which offers only the
// shell harness.
func NewTmuxHandler() *TmuxHandler {
	return NewTmuxHandlerWithLaunchConfig(LaunchConfig{})
}

// NewTmuxHandlerWithLaunchConfig creates a tmux handler that can start the
// harnesses the operator configured, without completion hooks.
func NewTmuxHandlerWithLaunchConfig(launch LaunchConfig) *TmuxHandler {
	return NewTmuxHandlerWithLaunch(launch, AgentHooks{})
}

// NewTmuxHandlerWithLaunch creates a tmux handler that can start the
// configured harnesses and wire their completion hooks to this server.
func NewTmuxHandlerWithLaunch(launch LaunchConfig, hooks AgentHooks) *TmuxHandler {
	return &TmuxHandler{
		proc:   systemProcSource(os.Getpid()),
		launch: withLaunchDefaults(launch),
		hooks:  hooks,
		events: newAgentEventStore(),
	}
}

// RegisterRoutes registers the tmux routes on the given mux
func (h *TmuxHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tmux/sessions", h.ListSessions)
	mux.HandleFunc("POST /api/tmux/sessions", h.CreateSession)
	mux.HandleFunc("GET /api/tmux/sessions/{name}/panes", h.ListSessionPanes)
	mux.HandleFunc("POST /api/tmux/sessions/{name}/send", h.SendToSession)
	mux.HandleFunc("DELETE /api/tmux/sessions/all", h.DeleteAllSessions)
	mux.HandleFunc("DELETE /api/tmux/sessions/{name}", h.DeleteSession)
	mux.HandleFunc("PATCH /api/tmux/sessions/{name}", h.RenameSession)
	mux.HandleFunc("GET /api/tmux/sessions/{name}/capture", h.CapturePane)
	mux.HandleFunc("POST /api/tmux/mouse", h.SetMouseMode)
}

type tmuxTarget struct {
	socket   string
	workDir  string
	unixUser string
}

func parseUserValueMap(raw string) map[string]string {
	result := map[string]string{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			continue
		}
		user := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if user != "" && value != "" {
			result[user] = value
		}
	}
	return result
}

// terminalUserMapEnvVars are the CSV maps parsed by parseUserValueMap.
var terminalUserMapEnvVars = []string{
	"CHROTE_TMUX_SOCKET",
	"CHROTE_TERMINAL_USER_WORKDIRS",
}

// ValidateTerminalUserEnv rejects a Unix user appearing twice in any
// terminal map and requires every configured socket to be explicit.
func ValidateTerminalUserEnv() error {
	for _, name := range terminalUserMapEnvVars {
		if err := validateNoDuplicateUserKeys(name, os.Getenv(name)); err != nil {
			return err
		}
	}
	for _, item := range strings.Split(os.Getenv("CHROTE_TMUX_SOCKET"), ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return fmt.Errorf("CHROTE_TMUX_SOCKET entry %q must be unixUser=/absolute/socket", item)
		}
		socket := strings.TrimSpace(parts[1])
		if !filepath.IsAbs(socket) {
			return fmt.Errorf("CHROTE_TMUX_SOCKET for Unix user %q must be an absolute path: %q", strings.TrimSpace(parts[0]), socket)
		}
		if filepath.Clean(socket) != socket {
			return fmt.Errorf("CHROTE_TMUX_SOCKET for Unix user %q must be canonical: %q", strings.TrimSpace(parts[0]), socket)
		}
	}
	return nil
}

func validateNoDuplicateUserKeys(envName, raw string) error {
	seen := map[string]string{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			continue
		}
		user := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if user == "" || value == "" {
			continue
		}
		if previous, duplicate := seen[user]; duplicate {
			return fmt.Errorf("%s has duplicate entries for Unix user %q (%q and %q); keep exactly one entry per user so terminal listing and terminal attach resolve the same socket", envName, user, previous, value)
		}
		seen[user] = value
	}
	return nil
}

func configuredTerminalUsers() []string {
	raw := strings.TrimSpace(os.Getenv("CHROTE_TMUX_SOCKET"))
	users := []string{}
	seen := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		parts := strings.SplitN(strings.TrimSpace(item), "=", 2)
		if len(parts) != 2 {
			continue
		}
		user := strings.TrimSpace(parts[0])
		socket := strings.TrimSpace(parts[1])
		if user != "" && socket != "" && !seen[user] {
			users = append(users, user)
			seen[user] = true
		}
	}
	return users
}

func advertisedTerminalUsers() []string {
	return configuredTerminalUsers()
}

func (h *TmuxHandler) targetForUnixUser(unixUser string) (tmuxTarget, error) {
	return h.targetForUnixUserContext(context.Background(), unixUser)
}

func (h *TmuxHandler) targetForUnixUserContext(ctx context.Context, unixUser string) (tmuxTarget, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return tmuxTarget{}, err
	}
	unixUser = strings.TrimSpace(unixUser)
	if unixUser == "" {
		users := configuredTerminalUsers()
		switch len(users) {
		case 0:
			return tmuxTarget{}, fmt.Errorf("no tmux sockets are configured")
		case 1:
			unixUser = users[0]
		default:
			return tmuxTarget{}, fmt.Errorf("Unix user is required when multiple terminal users are configured")
		}
	}

	socketMap := parseUserValueMap(os.Getenv("CHROTE_TMUX_SOCKET"))
	workDirMap := parseUserValueMap(os.Getenv("CHROTE_TERMINAL_USER_WORKDIRS"))
	target := tmuxTarget{
		socket:   filepath.Clean(socketMap[unixUser]),
		workDir:  workDirMap[unixUser],
		unixUser: unixUser,
	}
	if strings.TrimSpace(socketMap[unixUser]) == "" {
		return tmuxTarget{}, fmt.Errorf("Unix user %q is not allowed for terminal launch", unixUser)
	}
	if target.workDir == "" {
		target.workDir = core.GetWorkDir()
	}
	return target, nil
}

func targetFromRequest(h *TmuxHandler, r *http.Request, bodyUnixUser string) (tmuxTarget, error) {
	unixUser := strings.TrimSpace(bodyUnixUser)
	if unixUser == "" && r != nil {
		unixUser = strings.TrimSpace(r.URL.Query().Get("unixUser"))
	}
	return h.targetForUnixUser(unixUser)
}

func sendTargetFromRequest(h *TmuxHandler, r *http.Request, bodyUnixUser string) (tmuxTarget, error) {
	bodyUnixUser = strings.TrimSpace(bodyUnixUser)
	queryUnixUser := ""
	if r != nil {
		queryUnixUser = strings.TrimSpace(r.URL.Query().Get("unixUser"))
	}
	if bodyUnixUser != "" && queryUnixUser != "" && bodyUnixUser != queryUnixUser {
		return tmuxTarget{}, fmt.Errorf("conflicting Unix users in query %q and request body %q", queryUnixUser, bodyUnixUser)
	}
	unixUser := bodyUnixUser
	if unixUser == "" {
		unixUser = queryUnixUser
	}
	return h.targetForUnixUser(unixUser)
}

func effectiveTmuxSocket(socket string) string {
	return filepath.Clean(strings.TrimSpace(socket))
}

func isTmuxNoServerErrorForSocket(errStr, socket string) bool {
	expected := effectiveTmuxSocket(socket)
	diagnostic := strings.TrimSpace(errStr)
	diagnostic = strings.TrimPrefix(diagnostic, "exit status 1: ")
	return diagnostic == "no server running on "+expected ||
		diagnostic == "error connecting to "+expected+" (No such file or directory)"
}

func isTmuxDuplicateSessionError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(tmuxErrorDiagnostic(err)), "duplicate session")
}

const tmuxCommandOutputLimit = 1 << 20

var errTmuxCommandOutputLimit = errors.New("tmux command output exceeded the 1 MiB limit")

type tmuxCommandError struct {
	cause      error
	diagnostic string
}

func (e *tmuxCommandError) Error() string {
	if e == nil || e.cause == nil {
		return "tmux command failed"
	}
	return "tmux command failed: " + e.cause.Error()
}

func (e *tmuxCommandError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func tmuxErrorDiagnostic(err error) string {
	var commandErr *tmuxCommandError
	if errors.As(err, &commandErr) {
		return commandErr.diagnostic
	}
	if err == nil {
		return ""
	}
	return err.Error()
}

type tmuxCommandCapture struct {
	mu        sync.Mutex
	remaining int
	exceeded  bool
	stdout    bytes.Buffer
	stderr    bytes.Buffer
}

type tmuxCommandCaptureWriter struct {
	capture *tmuxCommandCapture
	stderr  bool
}

func (w tmuxCommandCaptureWriter) Write(p []byte) (int, error) {
	w.capture.mu.Lock()
	defer w.capture.mu.Unlock()
	if w.capture.remaining <= 0 {
		w.capture.exceeded = true
		return 0, errTmuxCommandOutputLimit
	}
	writeLen := len(p)
	if writeLen > w.capture.remaining {
		writeLen = w.capture.remaining
		w.capture.exceeded = true
	}
	var err error
	if w.stderr {
		_, err = w.capture.stderr.Write(p[:writeLen])
	} else {
		_, err = w.capture.stdout.Write(p[:writeLen])
	}
	w.capture.remaining -= writeLen
	if err != nil {
		return writeLen, err
	}
	if writeLen != len(p) {
		return writeLen, errTmuxCommandOutputLimit
	}
	return writeLen, nil
}

func (c *tmuxCommandCapture) values() (stdout, stderr string, exceeded bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stdout.String(), c.stderr.String(), c.exceeded
}

func (h *TmuxHandler) runTmuxOnSocket(socket string, args ...string) (string, error) {
	return h.runTmuxOnSocketContext(context.Background(), socket, args...)
}

func (h *TmuxHandler) runTmuxOnSocketContext(parent context.Context, socket string, args ...string) (string, error) {
	return h.runTmuxOnSocketInput(parent, socket, "", args...)
}

// runTmuxOnSocketInput runs one tmux command, feeding stdin from input. Only a
// control-mode client reads commands from stdin; every other caller passes "".
func (h *TmuxHandler) runTmuxOnSocketInput(parent context.Context, socket, input string, args ...string) (string, error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	socket = strings.TrimSpace(socket)
	if socket == "" {
		return "", fmt.Errorf("tmux socket is not configured")
	}
	args = append([]string{"-S", socket}, args...)

	cmd := exec.CommandContext(ctx, core.TmuxBin(), args...)
	cmd.Env = os.Environ()
	cmd.Stdin = strings.NewReader(input)
	capture := &tmuxCommandCapture{remaining: tmuxCommandOutputLimit}
	cmd.Stdout = tmuxCommandCaptureWriter{capture: capture}
	cmd.Stderr = tmuxCommandCaptureWriter{capture: capture, stderr: true}

	err := cmd.Run()
	output, stderr, exceeded := capture.values()
	if exceeded {
		return "", errTmuxCommandOutputLimit
	}
	if err != nil {
		cause := err
		if ctx.Err() != nil {
			cause = ctx.Err()
		}
		return "", &tmuxCommandError{cause: cause, diagnostic: strings.TrimSpace(stderr)}
	}
	return output, nil
}

// sessionInventoryFormat is the one command per socket that answers every
// question the session list asks. Facts that contradict a session's
// appearance are read here rather than by a follow-up call per fact.
const sessionInventoryFormat = "#{session_id}\t" +
	"#{session_name}\t" +
	"#{session_windows}\t" +
	"#{session_attached}\t" +
	"#{pane_current_path}\t" +
	"#{pane_current_command}\t" +
	"#{window_panes}\t" +
	"#{window_width}\t" +
	"#{window_height}\t" +
	"#{window-size}\t" +
	"#{mouse}\t" +
	"#{session_attached_list}"

const sessionInventoryFieldCount = 12

func parseSessionsOutput(output string, unixUser string, ownedPTYs map[string]bool) []core.Session {
	sessions := []core.Session{}
	lines := strings.Split(strings.TrimRight(output, "\r\n"), "\n")
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Every line answers the format string above, so a line that is not
		// exactly that many fields did not come from this inventory and is
		// not guessed at.
		parts := strings.SplitN(line, "\t", sessionInventoryFieldCount)
		if len(parts) != sessionInventoryFieldCount {
			continue
		}
		field := func(index int) string { return parts[index] }
		name := field(1)
		if isReservedInternalSessionName(name) {
			continue
		}
		windows, _ := strconv.Atoi(field(2)) //nolint:errcheck // defaults to 0 on parse failure, corrected to 1 below
		if windows == 0 {
			windows = 1
		}
		session := core.Session{
			ID:             field(0),
			Name:           name,
			Windows:        windows,
			Attached:       field(3) == "1",
			Group:          core.CategorizeSession(name),
			UnixUser:       unixUser,
			CWD:            field(4),
			CurrentCommand: field(5),
		}
		session.Panes, _ = strconv.Atoi(field(6))  //nolint:errcheck // an unparsable count claims nothing
		session.Width, _ = strconv.Atoi(field(7))  //nolint:errcheck // an unparsable size claims nothing
		session.Height, _ = strconv.Atoi(field(8)) //nolint:errcheck // an unparsable size claims nothing
		session.SizePinned = field(9) == "manual"
		if mouse := field(10); mouse == "0" || mouse == "1" {
			enabled := mouse == "1"
			session.MouseEnabled = &enabled
		}
		session.ForeignClients = foreignClientTTYs(field(11), ownedPTYs)
		session.Viewers = countAttachedClients(field(11))
		sessions = append(sessions, session)
	}
	return sessions
}

func publicTmuxSourceError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, errTmuxCommandOutputLimit) {
		return "tmux inventory exceeded the bounded output limit"
	}
	message := strings.ToLower(tmuxErrorDiagnostic(err))
	if strings.Contains(message, "permission denied") {
		return "tmux source permission denied"
	}
	if strings.Contains(message, "deadline exceeded") || strings.Contains(message, "signal: killed") {
		return "tmux inventory timed out"
	}
	return "tmux source unavailable"
}

func (h *TmuxHandler) listSessionsForTarget(target tmuxTarget, ownedPTYs map[string]bool) ([]core.Session, string) {
	output, err := h.runTmuxOnSocket(target.socket, "list-sessions", "-F", sessionInventoryFormat)
	if err != nil {
		diagnostic := tmuxErrorDiagnostic(err)
		if isTmuxNoServerErrorForSocket(diagnostic, target.socket) {
			return []core.Session{}, ""
		}
		return []core.Session{}, publicTmuxSourceError(err)
	}
	sessions := parseSessionsOutput(output, target.unixUser, ownedPTYs)
	h.events.attach(target.unixUser, sessions)
	return sessions, ""
}

// ListSessions handles GET /api/tmux/sessions
func (h *TmuxHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	queryUnixUser := ""
	if r != nil {
		queryUnixUser = strings.TrimSpace(r.URL.Query().Get("unixUser"))
	}

	response := &SessionsResponse{
		Sessions:      []core.Session{},
		Grouped:       make(map[string][]core.Session),
		TerminalUsers: advertisedTerminalUsers(),
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
	}
	// The ptys this server spawned are the same set for every socket, so the
	// walk happens once per listing rather than once per target.
	ownedPTYs := h.proc.ownedPTYs()
	if queryUnixUser == "" {
		var errors, successfulUsers, failedUsers []string
		for _, unixUser := range configuredTerminalUsers() {
			target, targetErr := h.targetForUnixUser(unixUser)
			if targetErr != nil {
				publicError := "tmux source configuration is unavailable"
				errors = append(errors, fmt.Sprintf("%s: %s", unixUser, publicError))
				failedUsers = append(failedUsers, unixUser)
				continue
			}
			sessions, errStr := h.listSessionsForTarget(target, ownedPTYs)
			if errStr != "" {
				errors = append(errors, fmt.Sprintf("%s: %s", unixUser, errStr))
				failedUsers = append(failedUsers, unixUser)
				continue
			}
			successfulUsers = append(successfulUsers, unixUser)
			response.Sessions = append(response.Sessions, sessions...)
		}
		if len(errors) > 0 {
			response.Error = strings.Join(errors, "; ")
			response.Partial = len(successfulUsers) > 0
			if response.Partial {
				response.SuccessfulUsers = successfulUsers
				response.FailedUsers = failedUsers
			}
		}
	} else {
		target, targetErr := targetFromRequest(h, r, "")
		if targetErr != nil {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", targetErr.Error())
			return
		}
		sessions, errStr := h.listSessionsForTarget(target, ownedPTYs)
		response.Sessions = append(response.Sessions, sessions...)
		if errStr != "" {
			response.Error = errStr
		}
	}

	core.SortSessions(response.Sessions)
	response.Grouped = core.GroupSessions(response.Sessions)

	core.WriteJSON(w, http.StatusOK, response)
}

const (
	tmuxCreationTokenEnv          = "CHROTE_CREATION_TOKEN"
	tmuxOwnershipMismatchResponse = "CHROTE_OWNERSHIP_MISMATCH"
)

var (
	tmuxSessionIDPattern     = regexp.MustCompile(`^\$[0-9]+$`)
	tmuxPaneIDPattern        = regexp.MustCompile(`^%[0-9]+$`)
	tmuxPIDPattern           = regexp.MustCompile(`^[1-9][0-9]*$`)
	tmuxCreationTokenPattern = regexp.MustCompile(`^[0-9a-f]{24}$`)
)

type ownedTmuxSession struct {
	ID    string
	Name  string
	Token string
}

func newTmuxCreationToken() (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func isTmuxMissingTargetErrorForSocket(err error, socket, target string) bool {
	if err == nil {
		return false
	}
	diagnostic := tmuxErrorDiagnostic(err)
	if isTmuxNoServerErrorForSocket(diagnostic, socket) {
		return true
	}
	diagnostic = strings.TrimSpace(diagnostic)
	diagnostic = strings.TrimPrefix(diagnostic, "exit status 1: ")
	return diagnostic == "can't find session: "+target || diagnostic == "no such session: "+target
}

func (h *TmuxHandler) cleanupOwnedTmuxSession(parent context.Context, socket, target, token string) error {
	if strings.TrimSpace(target) == "" || !tmuxCreationTokenPattern.MatchString(token) {
		return fmt.Errorf("owned tmux cleanup requires a target and valid creation token")
	}
	if !tmuxSessionIDPattern.MatchString(target) {
		if valid, _ := core.ValidateSessionName(target, "cleanup target"); !valid {
			return fmt.Errorf("owned tmux cleanup target %q is invalid", target)
		}
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	condition := fmt.Sprintf("#{==:#{%s},%s}", tmuxCreationTokenEnv, token)
	killCommand := "kill-session -t " + target
	mismatchCommand := "display-message -p " + tmuxOwnershipMismatchResponse
	output, err := h.runTmuxOnSocketContext(ctx, socket,
		"if-shell", "-F", "-t", target,
		condition, killCommand, mismatchCommand,
	)
	if err != nil {
		if isTmuxMissingTargetErrorForSocket(err, socket, target) {
			return nil
		}
		return fmt.Errorf("atomically clean owned tmux session %q: %w", target, err)
	}
	switch strings.TrimSpace(output) {
	case "":
		return nil
	case tmuxOwnershipMismatchResponse:
		return fmt.Errorf("refusing to clean tmux session %q: creation token does not match", target)
	default:
		return fmt.Errorf("atomically clean owned tmux session %q: unexpected tmux response", target)
	}
}

func (h *TmuxHandler) cleanupOwnedTmuxSessionAfterError(socket string, session ownedTmuxSession, cause error) error {
	target := session.ID
	if target == "" {
		target = session.Name
	}
	if cleanupErr := h.cleanupOwnedTmuxSession(context.Background(), socket, target, session.Token); cleanupErr != nil {
		return errors.Join(cause, cleanupErr)
	}
	return cause
}

func (h *TmuxHandler) createOwnedTmuxSession(parent context.Context, socket, name, workDir string, env []string) (ownedTmuxSession, error) {
	return h.createOwnedTmuxSessionWithWindow(parent, socket, name, workDir, "", env)
}

// createOwnedTmuxSessionWithWindow creates the session with each KEY=VALUE in
// env set in it, alongside the creation token.
func (h *TmuxHandler) createOwnedTmuxSessionWithWindow(parent context.Context, socket, name, workDir, windowName string, env []string) (ownedTmuxSession, error) {
	token, err := newTmuxCreationToken()
	if err != nil {
		return ownedTmuxSession{}, fmt.Errorf("generate tmux creation token: %w", err)
	}
	session := ownedTmuxSession{Name: name, Token: token}
	args := []string{
		"new-session", "-d", "-P", "-F", "#{session_id}",
		"-e", tmuxCreationTokenEnv + "=" + token,
	}
	for _, entry := range env {
		args = append(args, "-e", entry)
	}
	args = append(args, "-s", name, "-c", workDir)
	if strings.TrimSpace(windowName) != "" {
		args = append(args, "-n", strings.TrimSpace(windowName))
	}
	output, createErr := h.runTmuxOnSocketContext(parent, socket, args...)
	if createErr != nil {
		return ownedTmuxSession{}, h.cleanupOwnedTmuxSessionAfterError(socket, session, createErr)
	}
	createdID := strings.TrimSpace(output)
	if !tmuxSessionIDPattern.MatchString(createdID) {
		parseErr := fmt.Errorf("tmux created session %q without a valid session ID", name)
		return ownedTmuxSession{}, h.cleanupOwnedTmuxSessionAfterError(socket, session, parseErr)
	}
	session.ID = createdID
	if sizeErr := h.sizeCreatedSession(parent, socket, session.ID); sizeErr != nil {
		return ownedTmuxSession{}, h.cleanupOwnedTmuxSessionAfterError(socket, session, sizeErr)
	}
	return session, nil
}

// CreateSession handles POST /api/tmux/sessions
func (h *TmuxHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && r.ContentLength > 0 {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	name := req.Name
	if name == "" {
		// Generate a name if not provided
		timestamp := strconv.FormatInt(time.Now().UnixMilli(), 36)
		name = "shell-" + timestamp
	} else {
		// Validate user-provided session name
		valid, errMsg := core.ValidateSessionName(name, "session name")
		if !valid {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
			return
		}
		if isReservedInternalSessionName(name) {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Session name is reserved for internal CHROTE use")
			return
		}
	}

	target, targetErr := targetFromRequest(h, r, req.UnixUser)
	if targetErr != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", targetErr.Error())
		return
	}

	workDir := target.workDir
	if workDir == "" {
		workDir = core.GetWorkDir()
	}
	// Both the folder and the harness are settled before anything is created,
	// so a request CHROTE cannot honour leaves no session behind.
	workDir, cwdErr := resolveLaunchCwd(req.Cwd, target.unixUser, workDir)
	if cwdErr != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", cwdErr.Error())
		return
	}
	launch, launchErr := h.launch.resolveHarness(req.Harness, req.Flags)
	if launchErr != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", launchErr.Error())
		return
	}
	// The hooks are settled before the session exists too: the command that
	// will be typed is final by the time anything is created.
	notify := req.Notify == nil || *req.Notify
	command, hookWarning := launch.command, ""
	if notify {
		command, hookWarning = h.hooks.commandFor(launch.harnessID, launch.command, target.unixUser, name)
	}
	notified := notify && hookWarning == "" && command != launch.command

	// Create the session (detached) with an ownership marker and immutable ID.
	session, err := h.createOwnedTmuxSession(r.Context(), target.socket, name, workDir, h.hooks.sessionEnv())
	if err != nil {
		if isTmuxDuplicateSessionError(err) {
			core.WriteError(w, http.StatusConflict, "SESSION_NAME_CONFLICT", fmt.Sprintf("tmux session name %q is already in use", name))
			return
		}
		core.WriteError(w, http.StatusBadRequest, "TMUX_ERROR", err.Error())
		return
	}
	mouseScroll := true
	if req.MouseScroll != nil {
		mouseScroll = *req.MouseScroll
	}
	mouseValue := "off"
	if mouseScroll {
		mouseValue = "on"
	}
	if err := h.applyMouseMode(r.Context(), target.socket, mouseValue); err != nil {
		err = h.cleanupOwnedTmuxSessionAfterError(target.socket, session, err)
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		return
	}
	// The harness is started last, after every step that would clean the
	// session up on failure. A session that already has an agent in it is
	// never a session CHROTE kills.
	response := map[string]interface{}{
		"success":   true,
		"session":   name,
		"cwd":       workDir,
		"harness":   launch.harnessID,
		"flags":     launch.flags,
		"notify":    notified,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	if hookWarning != "" {
		response["warning"] = hookWarning
	}
	if command != "" {
		if err := h.sendLaunchCommand(r.Context(), target.socket, session.ID, command); err != nil {
			// The session is a working login shell in the requested folder,
			// so it stays and the operator is told what did not start.
			response["warning"] = fmt.Sprintf("session created, but the %q command could not be started: %s", launch.harnessID, tmuxErrorDiagnostic(err))
		}
	}
	core.WriteJSON(w, http.StatusOK, response)
}

// sendLaunchCommand types a harness command into a session and submits it, so
// the harness runs under the login shell rather than replacing it.
func (h *TmuxHandler) sendLaunchCommand(ctx context.Context, socket, target, command string) error {
	if _, err := h.runTmuxOnSocketContext(ctx, socket, "send-keys", "-t", target, "-l", command); err != nil {
		return err
	}
	_, err := h.runTmuxOnSocketContext(ctx, socket, "send-keys", "-t", target, "Enter")
	return err
}

// DeleteSession handles DELETE /api/tmux/sessions/{name}
func (h *TmuxHandler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	sessionName := r.PathValue("name")

	valid, errMsg := core.ValidateSessionName(sessionName, "session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}

	target, targetErr := targetFromRequest(h, r, "")
	if targetErr != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", targetErr.Error())
		return
	}

	_, err := h.runTmuxOnSocket(target.socket, "kill-session", "-t", sessionName)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		return
	}

	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"killed":    sessionName,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// protectedSessions is the list of sessions that should not be killed by nuke
var protectedSessions = map[string]bool{}

// DeleteAllSessions handles DELETE /api/tmux/sessions/all
func (h *TmuxHandler) DeleteAllSessions(w http.ResponseWriter, r *http.Request) {
	// Verify the request came from the dashboard UI
	confirmHeader := r.Header.Get("X-Nuke-Confirm")
	if confirmHeader != "DASHBOARD-NUKE-CONFIRMED" {
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", "Nuke operation requires dashboard confirmation. Use the UI.")
		return
	}

	target, targetErr := targetFromRequest(h, r, "")
	if targetErr != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", targetErr.Error())
		return
	}
	// Get list of all sessions first.
	output, err := h.runTmuxOnSocket(target.socket, "list-sessions", "-F", "#{session_name}")
	var sessionNames []string
	var protectedNames []string
	if err == nil {
		lines := strings.Split(strings.TrimSpace(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				if protectedSessions[line] {
					protectedNames = append(protectedNames, line)
				} else {
					sessionNames = append(sessionNames, line)
				}
			}
		}
	} else if !isTmuxNoServerErrorForSocket(tmuxErrorDiagnostic(err), target.socket) {
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", "tmux source unavailable")
		return
	}

	if len(sessionNames) == 0 {
		core.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success":   true,
			"killed":    0,
			"protected": protectedNames,
			"message":   "No sessions to kill (protected sessions preserved)",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	// Kill each session individually instead of kill-server to preserve protected sessions
	var killed []string
	var errors []string
	for _, name := range sessionNames {
		_, err := h.runTmuxOnSocket(target.socket, "kill-session", "-t", name)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", name, err))
		} else {
			killed = append(killed, name)
		}
	}

	response := map[string]interface{}{
		"success":   len(errors) == 0,
		"killed":    len(killed),
		"sessions":  killed,
		"protected": protectedNames,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	if len(errors) > 0 {
		response["errors"] = errors
	}

	core.WriteJSON(w, http.StatusOK, response)
}

// RenameSession handles PATCH /api/tmux/sessions/{name}
func (h *TmuxHandler) RenameSession(w http.ResponseWriter, r *http.Request) {
	oldName := r.PathValue("name")

	valid, errMsg := core.ValidateSessionName(oldName, "current session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}
	if isReservedInternalSessionName(oldName) {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Session name is reserved for internal CHROTE use")
		return
	}

	var req RenameSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	valid, errMsg = core.ValidateSessionName(req.NewName, "new session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}
	if isReservedInternalSessionName(req.NewName) {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Session name is reserved for internal CHROTE use")
		return
	}

	target, targetErr := targetFromRequest(h, r, "")
	if targetErr != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", targetErr.Error())
		return
	}

	_, err := h.runTmuxOnSocket(target.socket, "rename-session", "-t", oldName, req.NewName)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		return
	}

	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"oldName":   oldName,
		"newName":   req.NewName,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// CapturePane handles GET /api/tmux/sessions/{name}/capture
func (h *TmuxHandler) CapturePane(w http.ResponseWriter, r *http.Request) {
	sessionName := r.PathValue("name")

	valid, errMsg := core.ValidateSessionName(sessionName, "session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}

	target, targetErr := targetFromRequest(h, r, "")
	if targetErr != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", targetErr.Error())
		return
	}

	// Build capture-pane command args
	args := []string{"capture-pane", "-t", sessionName, "-p"}

	// Support ?lines=N query param for scrollback
	if linesParam := r.URL.Query().Get("lines"); linesParam != "" {
		lines, err := strconv.Atoi(linesParam)
		if err != nil || lines < 0 {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid lines parameter: must be a non-negative integer")
			return
		}
		// -S -N means start from N lines before the current position
		args = append(args, "-S", fmt.Sprintf("-%d", lines))
	}

	output, err := h.runTmuxOnSocket(target.socket, args...)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		return
	}

	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"content": output,
		"session": sessionName,
	})
}

// ListSessionPanes handles GET /api/tmux/sessions/{name}/panes and exposes
// immutable pane IDs plus human-readable labels for an explicit safe choice.
func (h *TmuxHandler) ListSessionPanes(w http.ResponseWriter, r *http.Request) {
	sessionName := strings.TrimSpace(r.PathValue("name"))
	valid, errMsg := core.ValidateSessionName(sessionName, "session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}
	target, err := sendTargetFromRequest(h, r, "")
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	panes, err := h.listSendPanes(r.Context(), target, sessionName)
	if err != nil {
		var targetErr *sendTargetError
		if errors.As(err, &targetErr) {
			core.WriteError(w, targetErr.Status, targetErr.Code, targetErr.Message)
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"session":  sessionName,
		"unixUser": target.unixUser,
		"panes":    panes,
	})
}
