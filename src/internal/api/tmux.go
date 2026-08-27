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
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	osuser "os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/chrote/server/internal/core"
)

// TmuxHandler handles tmux-related API endpoints
type TmuxHandler struct {
	cache          *sessionsCache
	colorRegex     *regexp.Regexp
	socket         string
	workDir        string
	sessionDropSem chan struct{}
}

type sessionsCache struct {
	mu        sync.RWMutex
	data      *SessionsResponse
	timestamp time.Time
	ttl       time.Duration
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
	Partial bool `json:"partial,omitempty"`
}

// CreateSessionRequest is the request body for creating a session
type CreateSessionRequest struct {
	Name        string `json:"name"`
	UnixUser    string `json:"unixUser,omitempty"`
	MouseScroll *bool  `json:"mouseScroll,omitempty"`
}

// RenameSessionRequest is the request body for renaming a session
type RenameSessionRequest struct {
	NewName string `json:"newName"`
}

// MouseModeRequest is the request body for toggling tmux mouse mode.
type MouseModeRequest struct {
	Enabled *bool `json:"enabled"`
}

// AppearanceRequest is the request body for tmux appearance settings
type AppearanceRequest struct {
	StatusBg           string `json:"statusBg"`
	StatusFg           string `json:"statusFg"`
	PaneBorderActive   string `json:"paneBorderActive"`
	PaneBorderInactive string `json:"paneBorderInactive"`
	ModeStyleBg        string `json:"modeStyleBg"`
	ModeStyleFg        string `json:"modeStyleFg"`
}

func newSessionDropSemaphore() chan struct{} {
	sem := make(chan struct{}, 1)
	sem <- struct{}{}
	return sem
}

// NewTmuxHandler creates the default tmux handler. By default it uses
// TMUX_TMPDIR; CHROTE_DEFAULT_TMUX_SOCKET pins the same /api/tmux route to an
// explicit socket without changing the dashboard UI.
func NewTmuxHandler() *TmuxHandler {
	return &TmuxHandler{
		cache: &sessionsCache{
			ttl: time.Second,
		},
		colorRegex:     regexp.MustCompile(`^#[0-9A-Fa-f]{3,6}$|^[a-zA-Z]+$|^default$`),
		socket:         strings.TrimSpace(os.Getenv("CHROTE_DEFAULT_TMUX_SOCKET")),
		workDir:        strings.TrimSpace(os.Getenv("CHROTE_DEFAULT_TMUX_WORKDIR")),
		sessionDropSem: newSessionDropSemaphore(),
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
	mux.HandleFunc("POST /api/tmux/appearance", h.ApplyAppearance)
	mux.HandleFunc("POST /api/tmux/mouse", h.SetMouseMode)
}

// RunTmux satisfies the teams.TmuxRunner interface.
func (h *TmuxHandler) RunTmux(args ...string) (string, error) {
	return h.runTmux(args...)
}

type tmuxTarget struct {
	socket   string
	workDir  string
	unixUser string
}

var (
	tmuxCurrentUser = osuser.Current
	tmuxLookupUser  = osuser.Lookup
)

type tmuxUserLookupResult struct {
	account *osuser.User
	err     error
}

type tmuxUserLookupCall struct {
	done   chan struct{}
	result tmuxUserLookupResult
}

var tmuxUserLookupFlights = struct {
	sync.Mutex
	calls map[string]*tmuxUserLookupCall
}{calls: map[string]*tmuxUserLookupCall{}}

func resolveTmuxUserContext(ctx context.Context, key string, lookup func() (*osuser.User, error)) (*osuser.User, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	tmuxUserLookupFlights.Lock()
	call := tmuxUserLookupFlights.calls[key]
	if call == nil {
		call = &tmuxUserLookupCall{done: make(chan struct{})}
		tmuxUserLookupFlights.calls[key] = call
		go func() {
			call.result.account, call.result.err = lookup()
			tmuxUserLookupFlights.Lock()
			delete(tmuxUserLookupFlights.calls, key)
			close(call.done)
			tmuxUserLookupFlights.Unlock()
		}()
	}
	tmuxUserLookupFlights.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-call.done:
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return call.result.account, call.result.err
	}
}

func currentTmuxUserContext(ctx context.Context) (*osuser.User, error) {
	return resolveTmuxUserContext(ctx, "current", tmuxCurrentUser)
}

func lookupTmuxUserContext(ctx context.Context, username string) (*osuser.User, error) {
	lookupUser := tmuxLookupUser
	return resolveTmuxUserContext(ctx, "lookup:"+username, func() (*osuser.User, error) {
		return lookupUser(username)
	})
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

// terminalUserMapEnvVars are the CHROTE_TERMINAL_USER_* CSV maps parsed by
// parseUserValueMap.
var terminalUserMapEnvVars = []string{
	"CHROTE_TERMINAL_USER_SOCKETS",
	"CHROTE_TERMINAL_USER_WORKDIRS",
}

// ValidateTerminalUserEnv rejects a Unix user appearing twice in any
// CHROTE_TERMINAL_USER_* map. parseUserValueMap is last-wins while
// terminal-launch.sh is first-wins, so a duplicate makes session listing and
// terminal attach resolve different tmux servers. Refusing to start is the only
// way both parsers stay in agreement.
func ValidateTerminalUserEnv() error {
	for _, name := range terminalUserMapEnvVars {
		if err := validateNoDuplicateUserKeys(name, os.Getenv(name)); err != nil {
			return err
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
	raw := strings.TrimSpace(os.Getenv("CHROTE_TERMINAL_USERS"))
	if raw == "" {
		return []string{}
	}
	users := []string{}
	seen := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item != "" && !seen[item] {
			users = append(users, item)
			seen[item] = true
		}
	}
	return users
}

func advertisedTerminalUsers() []string {
	return configuredTerminalUsers()
}

func allowedTerminalUsers() map[string]bool {
	allowed, _ := allowedTerminalUsersContext(context.Background())
	return allowed
}

func allowedTerminalUsersContext(ctx context.Context) (map[string]bool, error) {
	allowed := map[string]bool{}
	configured := configuredTerminalUsers()
	if len(configured) > 0 {
		for _, item := range configured {
			allowed[item] = true
		}
		return allowed, nil
	}
	current, err := currentTmuxUserContext(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return allowed, nil
	}
	if current != nil && current.Username != "" {
		allowed[current.Username] = true
	}
	return allowed, nil
}

func (h *TmuxHandler) targetForUnixUser(unixUser string) (tmuxTarget, error) {
	return h.resolveTargetForUnixUser(context.Background(), unixUser, false)
}

func (h *TmuxHandler) targetForUnixUserContext(ctx context.Context, unixUser string) (tmuxTarget, error) {
	return h.resolveTargetForUnixUser(ctx, unixUser, true)
}

func (h *TmuxHandler) resolveTargetForUnixUser(ctx context.Context, unixUser string, configuredFastPath bool) (tmuxTarget, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return tmuxTarget{}, err
	}
	unixUser = strings.TrimSpace(unixUser)
	if unixUser == "" {
		target := tmuxTarget{socket: h.socket, workDir: h.workDir}
		return target, nil
	}

	allowed, err := allowedTerminalUsersContext(ctx)
	if err != nil {
		return tmuxTarget{}, err
	}
	if !allowed[unixUser] {
		return tmuxTarget{}, fmt.Errorf("Unix user %q is not allowed for terminal launch", unixUser)
	}

	socketMap := parseUserValueMap(os.Getenv("CHROTE_TERMINAL_USER_SOCKETS"))
	workDirMap := parseUserValueMap(os.Getenv("CHROTE_TERMINAL_USER_WORKDIRS"))
	target := tmuxTarget{
		socket:   socketMap[unixUser],
		workDir:  workDirMap[unixUser],
		unixUser: unixUser,
	}
	if configuredFastPath && target.socket != "" && target.workDir != "" {
		return target, nil
	}

	account, err := lookupTmuxUserContext(ctx, unixUser)
	if err != nil {
		if target.socket != "" && target.workDir != "" && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			return target, nil
		}
		return tmuxTarget{}, fmt.Errorf("lookup Unix user %q: %w", unixUser, err)
	}
	currentUser := ""
	current, currentErr := currentTmuxUserContext(ctx)
	if currentErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return tmuxTarget{}, ctxErr
		}
	} else if current != nil {
		currentUser = current.Username
	}
	if target.workDir == "" {
		if currentUser == unixUser && h.workDir != "" {
			target.workDir = h.workDir
		} else {
			target.workDir = account.HomeDir
		}
	}
	if target.socket == "" {
		if currentUser == unixUser {
			target.socket = h.socket
		} else {
			target.socket = fmt.Sprintf("/tmp/tmux-%s/default", account.Uid)
		}
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
	if unixUser == "" {
		configured := configuredTerminalUsers()
		switch len(configured) {
		case 0:
			return h.targetForUnixUser("")
		case 1:
			unixUser = configured[0]
		default:
			return tmuxTarget{}, fmt.Errorf("Unix user is required when multiple terminal users are configured")
		}
	}
	return h.targetForUnixUser(unixUser)
}

const defaultSessionDropsDir = "/srv/data/chrote/session-drops"
const defaultSessionDropRetention = 7 * 24 * time.Hour
const defaultSessionDropMaintenanceInterval = time.Hour
const reservedInternalSessionPrefix = "chrote-probe-"

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

func isReservedInternalSessionName(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), reservedInternalSessionPrefix)
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

func isTmuxNoServerError(errStr string) bool {
	lower := strings.ToLower(errStr)
	return strings.Contains(lower, "no server running on ") ||
		(strings.Contains(lower, "error connecting to ") && strings.Contains(lower, "(no such file or directory)"))
}

func effectiveTmuxSocket(socket string) string {
	if socket = strings.TrimSpace(socket); socket != "" {
		return filepath.Clean(socket)
	}
	return filepath.Join(core.GetTmuxTmpdir(), "default")
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

func appendSessionResponseError(existing, next string) string {
	if strings.TrimSpace(existing) == "" {
		return next
	}
	if strings.TrimSpace(next) == "" {
		return existing
	}
	return existing + "; " + next
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

// runTmux executes a tmux command with proper environment
func (h *TmuxHandler) runTmux(args ...string) (string, error) {
	return h.runTmuxOnSocket(h.socket, args...)
}

func (h *TmuxHandler) runTmuxOnSocket(socket string, args ...string) (string, error) {
	return h.runTmuxOnSocketContext(context.Background(), socket, args...)
}

func (h *TmuxHandler) runTmuxOnSocketContext(parent context.Context, socket string, args ...string) (string, error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	if socket != "" {
		args = append([]string{"-S", socket}, args...)
	}

	cmd := exec.CommandContext(ctx, core.TmuxBin(), args...)
	cmd.Env = core.GetTmuxEnv()
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

func parseSessionsOutput(output string, unixUser string) []core.Session {
	sessions := []core.Session{}
	lines := strings.Split(strings.TrimRight(output, "\r\n"), "\n")
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		sessionID := ""
		name := ""
		windowsText := ""
		attachedText := ""
		cwd := ""
		currentCommand := ""
		if parts := strings.SplitN(line, "	", 6); len(parts) == 6 {
			sessionID, name, windowsText, attachedText, cwd, currentCommand = parts[0], parts[1], parts[2], parts[3], parts[4], parts[5]
		} else if len(parts) == 5 {
			sessionID, name, windowsText, attachedText, cwd = parts[0], parts[1], parts[2], parts[3], parts[4]
		} else {
			parts := strings.Split(line, ":")
			if len(parts) < 3 {
				continue
			}
			nameIndex := 0
			if len(parts) >= 4 {
				sessionID = parts[0]
				nameIndex = 1
			}
			name = parts[nameIndex]
			windowsText = parts[nameIndex+1]
			attachedText = parts[nameIndex+2]
		}
		if isReservedInternalSessionName(name) {
			continue
		}
		windows, _ := strconv.Atoi(windowsText) //nolint:errcheck // defaults to 0 on parse failure, corrected to 1 below
		if windows == 0 {
			windows = 1
		}
		sessions = append(sessions, core.Session{
			ID:             sessionID,
			Name:           name,
			Windows:        windows,
			Attached:       attachedText == "1",
			Group:          core.CategorizeSession(name),
			UnixUser:       unixUser,
			CWD:            cwd,
			CurrentCommand: currentCommand,
		})
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

func (h *TmuxHandler) listSessionsForTarget(target tmuxTarget) ([]core.Session, string) {
	output, err := h.runTmuxOnSocket(target.socket, "list-sessions", "-F", "#{session_id}	#{session_name}	#{session_windows}	#{session_attached}	#{pane_current_path}	#{pane_current_command}")
	if err != nil {
		diagnostic := tmuxErrorDiagnostic(err)
		if isTmuxNoServerErrorForSocket(diagnostic, target.socket) {
			return []core.Session{}, ""
		}
		return []core.Session{}, publicTmuxSourceError(err)
	}
	return parseSessionsOutput(output, target.unixUser), ""
}

// ListSessions handles GET /api/tmux/sessions
func (h *TmuxHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	queryUnixUser := ""
	if r != nil {
		queryUnixUser = strings.TrimSpace(r.URL.Query().Get("unixUser"))
	}
	useConfiguredUsers := queryUnixUser == "" && len(configuredTerminalUsers()) > 0
	useCache := queryUnixUser == "" && !useConfiguredUsers

	// Check cache
	if useCache {
		h.cache.mu.RLock()
		if h.cache.data != nil && time.Since(h.cache.timestamp) < h.cache.ttl {
			data := h.cache.data
			h.cache.mu.RUnlock()
			core.WriteJSON(w, http.StatusOK, data)
			return
		}
		h.cache.mu.RUnlock()
	}

	response := &SessionsResponse{
		Sessions:      []core.Session{},
		Grouped:       make(map[string][]core.Session),
		TerminalUsers: advertisedTerminalUsers(),
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
	}
	var multiUserError string
	multiUserPartialCandidate := false

	if useConfiguredUsers {
		var errors []string
		successfulUsers := 0
		for _, unixUser := range configuredTerminalUsers() {
			target, targetErr := h.targetForUnixUser(unixUser)
			if targetErr != nil {
				publicError := "tmux source configuration is unavailable"
				errors = append(errors, fmt.Sprintf("%s: %s", unixUser, publicError))
				continue
			}
			sessions, errStr := h.listSessionsForTarget(target)
			if errStr != "" {
				errors = append(errors, fmt.Sprintf("%s: %s", unixUser, errStr))
				continue
			}
			successfulUsers++
			response.Sessions = append(response.Sessions, sessions...)
		}
		if len(errors) > 0 {
			multiUserError = strings.Join(errors, "; ")
			response.Error = appendSessionResponseError(response.Error, multiUserError)
			multiUserPartialCandidate = successfulUsers > 0
		}
	} else {
		target, targetErr := targetFromRequest(h, r, "")
		if targetErr != nil {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", targetErr.Error())
			return
		}
		sessions, errStr := h.listSessionsForTarget(target)
		response.Sessions = append(response.Sessions, sessions...)
		if errStr != "" {
			response.Error = errStr
		}
	}

	core.SortSessions(response.Sessions)
	response.Grouped = core.GroupSessions(response.Sessions)
	response.Partial = multiUserPartialCandidate && response.Error == multiUserError

	// Update cache
	if useCache {
		h.cache.mu.Lock()
		h.cache.data = response
		h.cache.timestamp = time.Now()
		h.cache.mu.Unlock()
	}

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

func (h *TmuxHandler) createOwnedTmuxSession(parent context.Context, socket, name, workDir string) (ownedTmuxSession, error) {
	return h.createOwnedTmuxSessionWithWindow(parent, socket, name, workDir, "")
}

func (h *TmuxHandler) createOwnedTmuxSessionWithWindow(parent context.Context, socket, name, workDir, windowName string) (ownedTmuxSession, error) {
	token, err := newTmuxCreationToken()
	if err != nil {
		return ownedTmuxSession{}, fmt.Errorf("generate tmux creation token: %w", err)
	}
	session := ownedTmuxSession{Name: name, Token: token}
	args := []string{
		"new-session", "-d", "-P", "-F", "#{session_id}",
		"-e", tmuxCreationTokenEnv + "=" + token,
		"-s", name, "-c", workDir,
	}
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
	return session, nil
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

type sendPaneTarget struct {
	SessionID      string `json:"sessionId"`
	Session        string `json:"session"`
	PaneID         string `json:"pane"`
	PanePID        string `json:"panePid"`
	ServerPID      string `json:"serverPid"`
	WindowID       string `json:"windowId,omitempty"`
	WindowName     string `json:"windowName,omitempty"`
	CurrentPath    string `json:"currentPath,omitempty"`
	CurrentCommand string `json:"currentCommand,omitempty"`
	Active         bool   `json:"active"`
}

type sendTargetError struct {
	Status  int
	Code    string
	Message string
}

func (e *sendTargetError) Error() string { return e.Message }

func parseSendPaneTargets(output string) []sendPaneTarget {
	targets := []sendPaneTarget{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		parts := strings.Split(line, "	")
		if len(parts) < 5 {
			continue
		}
		target := sendPaneTarget{
			SessionID: strings.TrimSpace(parts[0]),
			Session:   strings.TrimSpace(parts[1]),
			PaneID:    strings.TrimSpace(parts[2]),
			PanePID:   strings.TrimSpace(parts[3]),
			ServerPID: strings.TrimSpace(parts[4]),
		}
		if len(parts) >= 10 {
			target.WindowID = strings.TrimSpace(parts[5])
			target.WindowName = strings.TrimSpace(parts[6])
			target.CurrentPath = strings.TrimSpace(parts[7])
			target.CurrentCommand = strings.TrimSpace(parts[8])
			target.Active = strings.TrimSpace(parts[9]) == "1"
		}
		if !tmuxSessionIDPattern.MatchString(target.SessionID) ||
			!tmuxPaneIDPattern.MatchString(target.PaneID) ||
			!tmuxPIDPattern.MatchString(target.PanePID) ||
			!tmuxPIDPattern.MatchString(target.ServerPID) {
			continue
		}
		targets = append(targets, target)
	}
	return targets
}

func (h *TmuxHandler) listSendPanes(ctx context.Context, target tmuxTarget, sessionName string) ([]sendPaneTarget, error) {
	output, err := h.runTmuxOnSocketContext(ctx, target.socket, "list-panes", "-a", "-F", "#{session_id}	#{session_name}	#{pane_id}	#{pane_pid}	#{pid}	#{window_id}	#{window_name}	#{pane_current_path}	#{pane_current_command}	#{pane_active}")
	if err != nil {
		return nil, err
	}
	panes := []sendPaneTarget{}
	for _, pane := range parseSendPaneTargets(output) {
		if pane.Session == sessionName {
			panes = append(panes, pane)
		}
	}
	if len(panes) == 0 {
		return nil, &sendTargetError{Status: http.StatusNotFound, Code: "SESSION_NOT_FOUND", Message: fmt.Sprintf("tmux session %q was not found exactly", sessionName)}
	}
	return panes, nil
}

func (h *TmuxHandler) resolveSendPane(ctx context.Context, target tmuxTarget, sessionName, requestedPane string) (sendPaneTarget, error) {
	panes, err := h.listSendPanes(ctx, target, sessionName)
	if err != nil {
		return sendPaneTarget{}, err
	}
	requestedPane = strings.TrimSpace(requestedPane)
	if requestedPane == "" {
		if len(panes) != 1 {
			return sendPaneTarget{}, &sendTargetError{Status: http.StatusConflict, Code: "PANE_REQUIRED", Message: fmt.Sprintf("tmux session %q has %d panes; select an exact %%pane", sessionName, len(panes))}
		}
		return panes[0], nil
	}
	if !tmuxPaneIDPattern.MatchString(requestedPane) {
		return sendPaneTarget{}, &sendTargetError{Status: http.StatusBadRequest, Code: "BAD_REQUEST", Message: "pane must be an immutable tmux pane ID such as %7"}
	}
	for _, pane := range panes {
		if pane.PaneID == requestedPane {
			return pane, nil
		}
	}
	return sendPaneTarget{}, &sendTargetError{Status: http.StatusConflict, Code: "PANE_NOT_IN_SESSION", Message: fmt.Sprintf("pane %q does not belong to tmux session %q", requestedPane, sessionName)}
}

func sameSendPaneGeneration(expected, actual sendPaneTarget) bool {
	return expected.SessionID == actual.SessionID &&
		expected.Session == actual.Session &&
		expected.PaneID == actual.PaneID &&
		expected.PanePID == actual.PanePID &&
		expected.ServerPID == actual.ServerPID
}

const (
	atomicSendPastedMarker              = "CHROTE_SEND_PASTED"
	atomicSendSubmitKeyMarker           = "CHROTE_SEND_SUBMIT_KEY_DISPATCHED"
	atomicSendTargetChangedMark         = "CHROTE_SEND_TARGET_CHANGED"
	atomicSendSubmitTargetChangedMarker = "CHROTE_SEND_SUBMIT_TARGET_CHANGED"
	tmuxSendSubmitSettleDelay           = 1200 * time.Millisecond
	tmuxSendSubmitObservationDelay      = 400 * time.Millisecond
	tmuxSendSubmitObservationTimeout    = 2 * time.Second
)

var tmuxSendSleep = func(ctx context.Context, delay time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func atomicSendCondition(pane sendPaneTarget) string {
	return fmt.Sprintf("#{&&:#{==:#{session_id},%s},#{&&:#{==:#{pane_id},%s},#{&&:#{==:#{pane_pid},%s},#{==:#{pid},%s}}}}", pane.SessionID, pane.PaneID, pane.PanePID, pane.ServerPID)
}

func atomicRetrySendCondition(pane sendPaneTarget, harness tmuxSubmitHarness) string {
	return fmt.Sprintf("#{&&:%s,#{==:#{pane_current_command},%s}}", atomicSendCondition(pane), harness.currentCommand())
}

func atomicPasteCommand(bufferName string, pane sendPaneTarget) string {
	command := fmt.Sprintf("paste-buffer -p -d -b %s -t %s", bufferName, pane.PaneID)
	return command + " ; display-message -p " + atomicSendPastedMarker
}

func atomicSubmitCommand(pane sendPaneTarget) string {
	return fmt.Sprintf("send-keys -t %s Enter ; display-message -p %s", pane.PaneID, atomicSendSubmitKeyMarker)
}

type tmuxSubmitHarness int

const (
	tmuxSubmitHarnessUnknown tmuxSubmitHarness = iota
	tmuxSubmitHarnessCodex
	tmuxSubmitHarnessClaude
)

func (harness tmuxSubmitHarness) currentCommand() string {
	switch harness {
	case tmuxSubmitHarnessCodex:
		return "codex"
	case tmuxSubmitHarnessClaude:
		return "claude"
	default:
		return ""
	}
}

func submitHarnessForPane(pane sendPaneTarget) tmuxSubmitHarness {
	switch strings.ToLower(strings.TrimSpace(pane.CurrentCommand)) {
	case "codex":
		return tmuxSubmitHarnessCodex
	case "claude":
		return tmuxSubmitHarnessClaude
	default:
		return tmuxSubmitHarnessUnknown
	}
}

type submitPayloadEvidence struct {
	witness            string
	codexCollapsedTags []string
}

func buildSubmitPayloadEvidence(payload string) (submitPayloadEvidence, bool) {
	for _, line := range strings.Split(strings.ReplaceAll(payload, "\r\n", "\n"), "\n") {
		witness := strings.Join(strings.Fields(line), " ")
		if len([]rune(witness)) < 16 {
			continue
		}
		runes := []rune(witness)
		if len(runes) > 80 {
			witness = string(runes[:80])
		}
		evidence := submitPayloadEvidence{witness: witness}
		for _, size := range []int{len(payload), utf8.RuneCountInString(payload)} {
			tag := fmt.Sprintf("[Pasted Content %d chars]", size)
			if len(evidence.codexCollapsedTags) == 0 || evidence.codexCollapsedTags[len(evidence.codexCollapsedTags)-1] != tag {
				evidence.codexCollapsedTags = append(evidence.codexCollapsedTags, tag)
			}
		}
		return evidence, true
	}
	return submitPayloadEvidence{}, false
}

func activeSubmitComposer(harness tmuxSubmitHarness, capture string) (string, bool) {
	capture = strings.ReplaceAll(capture, "\r", "")
	lower := strings.ToLower(capture)
	if strings.Contains(lower, "esc to interrupt") ||
		strings.Contains(lower, "ctrl+c to interrupt") ||
		strings.Contains(lower, "quick safety check") ||
		strings.Contains(lower, "yes, i trust this folder") {
		return "", false
	}

	hasCodex := strings.Contains(capture, "OpenAI Codex")
	hasClaude := strings.Contains(capture, "Claude Code")
	if hasCodex == hasClaude {
		return "", false
	}
	prefix := ""
	footerMatches := func(string) bool { return false }
	switch harness {
	case tmuxSubmitHarnessCodex:
		if !hasCodex {
			return "", false
		}
		prefix = "›"
		footerMatches = func(line string) bool {
			line = strings.ToLower(strings.TrimSpace(line))
			return strings.Contains(line, "gpt-") || strings.Contains(line, "% left")
		}
	case tmuxSubmitHarnessClaude:
		if !hasClaude {
			return "", false
		}
		prefix = "❯"
		footerMatches = func(line string) bool {
			return strings.Contains(strings.ToLower(line), "? for shortcuts")
		}
	default:
		return "", false
	}

	lines := strings.Split(capture, "\n")
	last := len(lines) - 1
	for last >= 0 && strings.TrimSpace(lines[last]) == "" {
		last--
	}
	if last < 1 || !footerMatches(lines[last]) {
		return "", false
	}
	composerStart := -1
	for index := last - 1; index >= 0; index-- {
		if strings.HasPrefix(strings.TrimSpace(lines[index]), prefix) {
			composerStart = index
			break
		}
	}
	if composerStart < 0 {
		return "", false
	}
	for _, line := range lines[composerStart+1 : last] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "$ ") || strings.HasPrefix(trimmed, "# ") {
			return "", false
		}
	}
	return strings.Join(strings.Fields(strings.Join(lines[composerStart:last], "\n")), " "), true
}

func composerMatchesPayload(harness tmuxSubmitHarness, composer string, evidence submitPayloadEvidence) bool {
	if evidence.witness != "" && strings.Contains(composer, evidence.witness) {
		return true
	}
	if harness != tmuxSubmitHarnessCodex {
		return false
	}
	for _, tag := range evidence.codexCollapsedTags {
		if strings.Contains(composer, tag) {
			return true
		}
	}
	return false
}

func (h *TmuxHandler) captureSubmitComposer(
	ctx context.Context,
	target tmuxTarget,
	pane sendPaneTarget,
	harness tmuxSubmitHarness,
	evidence submitPayloadEvidence,
	requirePayload bool,
) (string, bool) {
	panes, err := h.listSendPanes(ctx, target, pane.Session)
	if err != nil {
		return "", false
	}
	live := sendPaneTarget{}
	found := false
	for _, candidate := range panes {
		if sameSendPaneGeneration(pane, candidate) {
			live = candidate
			found = true
			break
		}
	}
	if !found || submitHarnessForPane(live) != harness {
		return "", false
	}
	output, err := h.runTmuxOnSocketContext(ctx, target.socket, "capture-pane", "-p", "-J", "-t", pane.PaneID, "-S", "-200")
	if err != nil {
		return "", false
	}
	composer, ok := activeSubmitComposer(harness, output)
	if !ok || (requirePayload && !composerMatchesPayload(harness, composer, evidence)) {
		return "", false
	}
	return composer, true
}

func (h *TmuxHandler) shouldRetrySubmit(
	ctx context.Context,
	target tmuxTarget,
	pane sendPaneTarget,
	harness tmuxSubmitHarness,
	evidence submitPayloadEvidence,
	expectedComposer string,
) bool {
	if expectedComposer == "" {
		return false
	}
	observationCtx, cancel := context.WithTimeout(ctx, tmuxSendSubmitObservationTimeout)
	defer cancel()
	capture := func() (string, bool) {
		if err := tmuxSendSleep(observationCtx, tmuxSendSubmitObservationDelay); err != nil {
			return "", false
		}
		return h.captureSubmitComposer(observationCtx, target, pane, harness, evidence, true)
	}
	first, ok := capture()
	if !ok || first != expectedComposer {
		return false
	}
	second, ok := capture()
	return ok && second == expectedComposer
}

type paneSendKind int

const (
	// paneSendDelivered means tmux confirmed the paste against the pinned pane
	// generation. SubmitKeyDispatched separately records at least one guarded
	// Enter transport receipt; it never claims application acceptance.
	paneSendDelivered paneSendKind = iota
	// paneSendTargetChanged means the pane generation moved before the paste ran,
	// so nothing was delivered.
	paneSendTargetChanged
	// paneSendUnknown means tmux never confirmed the outcome; the payload may or
	// may not have landed and must not be retried blindly.
	paneSendUnknown
)

// paneSendResult reports what the guarded tmux paste did. Kind covers everything
// after the buffer is loaded; a load failure is returned as an error instead
// because nothing can have been delivered yet.
type paneSendResult struct {
	Kind                paneSendKind
	SubmitKeyDispatched bool
	BufferCleaned       bool
	Detail              string
	OperationErr        error
	CleanupErr          error
}

func reservePaneSendCleanup(ctx context.Context, reserve time.Duration) (context.Context, time.Duration, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	deadline, bounded := ctx.Deadline()
	if reserve <= 0 || !bounded {
		return ctx, 0, func() {}
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return ctx, 0, func() {}
	}
	if maximumReserve := remaining / 4; reserve > maximumReserve {
		reserve = maximumReserve
	}
	if reserve <= 0 {
		return ctx, 0, func() {}
	}
	sendCtx, cancel := context.WithDeadline(ctx, deadline.Add(-reserve))
	return sendCtx, reserve, cancel
}

func paneSendCleanupContext(operationCtx context.Context, budget time.Duration) (context.Context, context.CancelFunc) {
	if budget <= 0 {
		return operationCtx, func() {}
	}
	deadline := time.Now().Add(budget)
	if operationDeadline, bounded := operationCtx.Deadline(); bounded && operationDeadline.Before(deadline) {
		deadline = operationDeadline
	}
	return context.WithDeadline(context.WithoutCancel(operationCtx), deadline)
}

// sendBufferToPane is the single delivery path shared by Send to Session and
// scheduled tasks: load the payload into a private buffer, paste it only while
// the pinned pane generation still matches, optionally dispatch one guarded
// Enter, and leave no buffer behind. Interactive sends may additionally allow
// one evidence-gated guarded retry; unattended scheduled sends do not.
// Interactive sends pass a background operation
// context so request cancellation cannot tear down a half-applied operator action;
// scheduled sends pass their bounded delivery context end to end.
func (h *TmuxHandler) sendBufferToPane(loadCtx, operationCtx context.Context, cleanupReserve time.Duration, target tmuxTarget, pane sendPaneTarget, bufferName, payloadPath string, submit, retryPendingComposer bool, evidence submitPayloadEvidence) (paneSendResult, error) {
	if operationCtx == nil {
		operationCtx = context.Background()
	}
	sendCtx, cleanupBudget, cancelSend := reservePaneSendCleanup(operationCtx, cleanupReserve)
	defer cancelSend()
	if cleanupBudget > 0 {
		loadCtx = sendCtx
	}
	harness := submitHarnessForPane(pane)
	beforePasteComposer := ""
	beforePasteCaptured := false
	if retryPendingComposer && harness != tmuxSubmitHarnessUnknown && evidence.witness != "" {
		beforePasteComposer, beforePasteCaptured = h.captureSubmitComposer(sendCtx, target, pane, harness, evidence, false)
	}
	bufferDeleted := false
	deleteBuffer := func() error {
		if bufferDeleted {
			return nil
		}
		cleanupCtx, cancelCleanup := paneSendCleanupContext(operationCtx, cleanupBudget)
		defer cancelCleanup()
		_, err := h.runTmuxOnSocketContext(cleanupCtx, target.socket, "delete-buffer", "-b", bufferName)
		if err == nil {
			bufferDeleted = true
		}
		return err
	}

	if _, err := h.runTmuxOnSocketContext(loadCtx, target.socket, "load-buffer", "-b", bufferName, payloadPath); err != nil {
		if cleanupErr := deleteBuffer(); cleanupErr != nil {
			err = fmt.Errorf("%w; buffer cleanup failed: %v", err, cleanupErr)
		}
		return paneSendResult{Kind: paneSendUnknown, BufferCleaned: bufferDeleted}, err
	}

	output, err := h.runTmuxOnSocketContext(
		sendCtx,
		target.socket,
		"if-shell", "-F", "-t", pane.PaneID,
		atomicSendCondition(pane),
		atomicPasteCommand(bufferName, pane),
		"display-message -p "+atomicSendTargetChangedMark,
	)
	if err != nil {
		cleanupErr := deleteBuffer()
		return paneSendResult{Kind: paneSendUnknown, BufferCleaned: cleanupErr == nil, Detail: err.Error(), OperationErr: err, CleanupErr: cleanupErr}, nil
	}

	switch marker := strings.TrimSpace(output); marker {
	case atomicSendTargetChangedMark:
		cleanupErr := deleteBuffer()
		return paneSendResult{Kind: paneSendTargetChanged, BufferCleaned: cleanupErr == nil, CleanupErr: cleanupErr}, nil
	case atomicSendPastedMarker:
		// paste-buffer -d consumed the buffer on success.
		bufferDeleted = true
	default:
		cleanupErr := deleteBuffer()
		return paneSendResult{
			Kind:          paneSendUnknown,
			BufferCleaned: cleanupErr == nil,
			Detail:        fmt.Sprintf("unexpected guarded paste result %q", marker),
			CleanupErr:    cleanupErr,
		}, nil
	}

	if !submit {
		return paneSendResult{Kind: paneSendDelivered, BufferCleaned: true}, nil
	}

	// Agent TUIs can swallow a submit key delivered in the same burst as a large
	// bracketed paste. Let the paste settle, then guard the first Enter against the
	// exact pane generation again before dispatching it.
	if err := tmuxSendSleep(sendCtx, tmuxSendSubmitSettleDelay); err != nil {
		return paneSendResult{
			Kind:          paneSendDelivered,
			BufferCleaned: true,
			Detail:        "submit key was not dispatched: " + err.Error(),
			OperationErr:  err,
		}, nil
	}
	pastedComposer := ""
	retryEligible := false
	if beforePasteCaptured {
		var pastedCaptured bool
		pastedComposer, pastedCaptured = h.captureSubmitComposer(sendCtx, target, pane, harness, evidence, true)
		retryEligible = pastedCaptured && pastedComposer != beforePasteComposer
	}
	output, err = h.runTmuxOnSocketContext(
		sendCtx,
		target.socket,
		"if-shell", "-F", "-t", pane.PaneID,
		atomicSendCondition(pane),
		atomicSubmitCommand(pane),
		"display-message -p "+atomicSendSubmitTargetChangedMarker,
	)
	if err != nil {
		return paneSendResult{Kind: paneSendUnknown, BufferCleaned: true, Detail: err.Error(), OperationErr: err}, nil
	}
	switch marker := strings.TrimSpace(output); marker {
	case atomicSendSubmitKeyMarker:
		if !retryEligible {
			return paneSendResult{Kind: paneSendDelivered, SubmitKeyDispatched: true, BufferCleaned: true}, nil
		}
		// A second Enter is eligible only when two bounded captures positively
		// identify the same non-empty pending prompt in a recognized idle composer.
		// The retry itself uses the same immutable server-side generation guard.
		if !h.shouldRetrySubmit(sendCtx, target, pane, harness, evidence, pastedComposer) {
			return paneSendResult{Kind: paneSendDelivered, SubmitKeyDispatched: true, BufferCleaned: true}, nil
		}
		output, err = h.runTmuxOnSocketContext(
			sendCtx,
			target.socket,
			"if-shell", "-F", "-t", pane.PaneID,
			atomicRetrySendCondition(pane, harness),
			atomicSubmitCommand(pane),
			"display-message -p "+atomicSendSubmitTargetChangedMarker,
		)
		if err != nil {
			return paneSendResult{
				Kind:                paneSendDelivered,
				SubmitKeyDispatched: true,
				BufferCleaned:       true,
				Detail:              "optional submit retry was not confirmed: " + err.Error(),
				OperationErr:        err,
			}, nil
		}
		switch retryMarker := strings.TrimSpace(output); retryMarker {
		case atomicSendSubmitKeyMarker:
			return paneSendResult{Kind: paneSendDelivered, SubmitKeyDispatched: true, BufferCleaned: true}, nil
		case atomicSendSubmitTargetChangedMarker:
			return paneSendResult{
				Kind:                paneSendDelivered,
				SubmitKeyDispatched: true,
				BufferCleaned:       true,
				Detail:              "target changed before optional submit retry; retry was suppressed",
			}, nil
		default:
			return paneSendResult{
				Kind:                paneSendDelivered,
				SubmitKeyDispatched: true,
				BufferCleaned:       true,
				Detail:              fmt.Sprintf("unexpected optional submit retry result %q", retryMarker),
			}, nil
		}
	case atomicSendSubmitTargetChangedMarker:
		return paneSendResult{
			Kind:          paneSendDelivered,
			BufferCleaned: true,
			Detail:        "target changed after paste; submit key was not dispatched",
		}, nil
	default:
		return paneSendResult{
			Kind:          paneSendUnknown,
			BufferCleaned: true,
			Detail:        fmt.Sprintf("unexpected guarded submit result %q", marker),
		}, nil
	}
}

// resolveActiveSendPane picks the pane an unattended sender should use: the
// session's only pane, or its active pane. Interactive sends pin an exact pane
// instead; a scheduled task has no operator to disambiguate at fire time.
func (h *TmuxHandler) resolveActiveSendPane(ctx context.Context, target tmuxTarget, sessionName string) (sendPaneTarget, error) {
	panes, err := h.listSendPanes(ctx, target, sessionName)
	if err != nil {
		return sendPaneTarget{}, err
	}
	if len(panes) == 1 {
		return panes[0], nil
	}
	for _, pane := range panes {
		if pane.Active {
			return pane, nil
		}
	}
	return sendPaneTarget{}, &sendTargetError{
		Status:  http.StatusConflict,
		Code:    "PANE_REQUIRED",
		Message: fmt.Sprintf("tmux session %q has %d panes and no active pane", sessionName, len(panes)),
	}
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

func maintainSessionDrops(dropRoot string, now time.Time) error {
	return maintainSessionDropsContext(context.Background(), dropRoot, now)
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

// SendToSession handles POST /api/tmux/sessions/{name}/send. It pins exact
// immutable pane identity and pastes through an atomic guarded tmux queue.
func (h *TmuxHandler) SendToSession(w http.ResponseWriter, r *http.Request) {
	sessionName := strings.TrimSpace(r.PathValue("name"))
	valid, errMsg := core.ValidateSessionName(sessionName, "session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}
	if err := parseSessionDropForm(w, r); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	target, targetErr := sendTargetFromRequest(h, r, sessionDropFormValue(r, "unixUser"))
	if targetErr != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", targetErr.Error())
		return
	}

	pane, err := h.resolveSendPane(r.Context(), target, sessionName, sessionDropFormValue(r, "pane"))
	if err != nil {
		var targetFailure *sendTargetError
		if errors.As(err, &targetFailure) {
			core.WriteError(w, targetFailure.Status, targetFailure.Code, targetFailure.Message)
		} else {
			core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		}
		return
	}
	requestedPane := strings.TrimSpace(sessionDropFormValue(r, "pane"))
	if requestedPane != "" {
		expected := sendPaneTarget{
			SessionID: strings.TrimSpace(sessionDropFormValue(r, "sessionId")),
			Session:   sessionName,
			PaneID:    requestedPane,
			PanePID:   strings.TrimSpace(sessionDropFormValue(r, "panePid")),
			ServerPID: strings.TrimSpace(sessionDropFormValue(r, "serverPid")),
		}
		if !tmuxSessionIDPattern.MatchString(expected.SessionID) ||
			!tmuxPIDPattern.MatchString(expected.PanePID) ||
			!tmuxPIDPattern.MatchString(expected.ServerPID) {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "an explicit pane requires its sessionId, panePid, and serverPid generation tuple")
			return
		}
		if !sameSendPaneGeneration(expected, pane) {
			core.WriteError(w, http.StatusConflict, "TARGET_CHANGED", "the selected tmux pane generation changed; refresh the chooser before retrying")
			return
		}
	} else if strings.TrimSpace(sessionDropFormValue(r, "sessionId")) != "" || strings.TrimSpace(sessionDropFormValue(r, "panePid")) != "" || strings.TrimSpace(sessionDropFormValue(r, "serverPid")) != "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "pane generation fields require an explicit pane")
		return
	}

	if err := h.lockSessionDrops(r.Context()); err != nil {
		core.WriteError(w, http.StatusRequestTimeout, "REQUEST_CANCELLED", "send request was cancelled before persistence")
		return
	}
	defer h.unlockSessionDrops()
	manifest, err := writeSessionDrop(r, sessionName, target, pane)
	if err != nil {
		if errors.Is(err, errEmptySessionDrop) {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "SESSION_DROP_ERROR", err.Error())
		return
	}
	dropPath := filepath.Dir(manifest.Payload)
	retainDrop := false
	defer func() {
		if !retainDrop {
			_ = os.RemoveAll(dropPath)
		}
	}()

	bufferName := "chrote-send-" + manifest.ID
	submissionRequested := submitFormValue(sessionDropFormValue(r, "submit"))
	writeUnknownOutcome := func(detail string, bufferCleaned bool, cleanupErr error) {
		retainDrop = true
		warning := "tmux did not confirm whether delivery occurred; inspect the exact pane before retrying"
		if strings.TrimSpace(detail) != "" {
			warning += ": " + strings.TrimSpace(detail)
		}
		if cleanupErr != nil {
			warning += fmt.Sprintf("; buffer cleanup could not be confirmed: %v", cleanupErr)
		}
		core.WriteJSON(w, http.StatusAccepted, map[string]interface{}{
			"success":             false,
			"transport":           "unknown",
			"retryable":           false,
			"deliveryConfirmed":   false,
			"submissionRequested": submissionRequested,
			"submitKeyDispatched": false,
			"bufferCleaned":       bufferCleaned,
			"targetVerified":      false,
			"warning":             warning,
			"session":             sessionName,
			"sessionId":           pane.SessionID,
			"pane":                pane.PaneID,
			"panePid":             pane.PanePID,
			"serverPid":           pane.ServerPID,
			"unixUser":            target.unixUser,
			"dropId":              manifest.ID,
			"dropPath":            dropPath,
			"payload":             manifest.Payload,
			"files":               manifest.Files,
			"timestamp":           time.Now().UTC().Format(time.RFC3339),
		})
	}
	result, err := h.sendBufferToPane(r.Context(), context.Background(), 0, target, pane, bufferName, manifest.Payload, submissionRequested, true, manifest.submitEvidence)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		return
	}
	switch result.Kind {
	case paneSendUnknown:
		writeUnknownOutcome(result.Detail, result.BufferCleaned, result.CleanupErr)
		return
	case paneSendTargetChanged:
		if result.CleanupErr != nil {
			core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", fmt.Sprintf("target changed and buffer cleanup failed: %v", result.CleanupErr))
			return
		}
		core.WriteError(w, http.StatusConflict, "TARGET_CHANGED", "tmux session or pane changed while preparing the send; inspect and retry")
		return
	}
	retainDrop = true
	warnings := []string{}
	verifiedPane, verifyErr := h.resolveSendPane(r.Context(), target, sessionName, pane.PaneID)
	targetVerified := verifyErr == nil && sameSendPaneGeneration(pane, verifiedPane)
	if strings.TrimSpace(result.Detail) != "" {
		warnings = append(warnings, result.Detail)
	}
	if !targetVerified {
		warnings = append(warnings, "target changed before post-send verification")
	}
	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":             true,
		"transport":           "pasted",
		"submissionRequested": submissionRequested,
		"submitKeyDispatched": result.SubmitKeyDispatched,
		"bufferCleaned":       true,
		"targetVerified":      targetVerified,
		"warning":             strings.Join(warnings, "; "),
		"session":             sessionName,
		"sessionId":           pane.SessionID,
		"pane":                pane.PaneID,
		"panePid":             pane.PanePID,
		"serverPid":           pane.ServerPID,
		"unixUser":            target.unixUser,
		"dropId":              manifest.ID,
		"dropPath":            dropPath,
		"payload":             manifest.Payload,
		"files":               manifest.Files,
		"timestamp":           time.Now().UTC().Format(time.RFC3339),
	})
}

// invalidateCache clears the sessions cache
func (h *TmuxHandler) invalidateCache() {
	h.cache.mu.Lock()
	h.cache.data = nil
	h.cache.timestamp = time.Time{}
	h.cache.mu.Unlock()
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

	// Create the session (detached) with an ownership marker and immutable ID.
	session, err := h.createOwnedTmuxSession(r.Context(), target.socket, name, workDir)
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
	h.invalidateCache()

	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"session":   name,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
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

	h.invalidateCache()

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

	h.invalidateCache()

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

	h.invalidateCache()

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

func (h *TmuxHandler) appearanceTargets() []tmuxTarget {
	users := configuredTerminalUsers()
	if len(users) == 0 {
		return []tmuxTarget{{socket: h.socket, workDir: h.workDir}}
	}
	targets := make([]tmuxTarget, 0, len(users))
	seenSockets := map[string]bool{}
	for _, user := range users {
		target, err := h.targetForUnixUser(user)
		if err != nil {
			continue
		}
		key := target.socket
		if key == "" {
			key = "ambient:"
		}
		if seenSockets[key] {
			continue
		}
		seenSockets[key] = true
		targets = append(targets, target)
	}
	if len(targets) == 0 {
		return []tmuxTarget{{socket: h.socket, workDir: h.workDir}}
	}
	return targets
}

var tmuxRightClickMenuKeys = []string{
	"MouseDown3Pane",
	"MouseDown3Status",
	"MouseDown3StatusLeft",
	"M-MouseDown3Pane",
	"M-MouseDown3Status",
	"M-MouseDown3StatusLeft",
}

func (h *TmuxHandler) removeTmuxRightClickMenus(parent context.Context, socket string) error {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	for _, key := range tmuxRightClickMenuKeys {
		// -q makes an already-absent binding successful; any remaining error is real.
		if _, err := h.runTmuxOnSocketContext(ctx, socket, "unbind-key", "-q", "-n", key); err != nil {
			return fmt.Errorf("remove tmux right-click binding %q: %w", key, err)
		}
	}
	return nil
}

func (h *TmuxHandler) applyMouseMode(parent context.Context, socket, value string) error {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	if _, err := h.runTmuxOnSocketContext(ctx, socket, "set-option", "-g", "mouse", value); err != nil {
		return err
	}
	return h.removeTmuxRightClickMenus(ctx, socket)
}

// SetMouseMode handles POST /api/tmux/mouse. It toggles tmux's global mouse
// option across all configured CHROTE terminal sockets.
func (h *TmuxHandler) SetMouseMode(w http.ResponseWriter, r *http.Request) {
	var req MouseModeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}
	if req.Enabled == nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "enabled must be a boolean")
		return
	}

	value := "off"
	if *req.Enabled {
		value = "on"
	}

	applied := 0
	targets := h.appearanceTargets()
	for _, target := range targets {
		if err := h.applyMouseMode(r.Context(), target.socket, value); err == nil {
			applied++
		}
		// A tmux server/profile may not be running yet; applied/success report that truthfully.
	}

	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":   applied == len(targets),
		"mouse":     value,
		"applied":   applied,
		"total":     len(targets),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// ApplyAppearance handles POST /api/tmux/appearance
func (h *TmuxHandler) ApplyAppearance(w http.ResponseWriter, r *http.Request) {
	var req AppearanceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}

	// Validate colors
	colors := map[string]string{
		"statusBg":           req.StatusBg,
		"statusFg":           req.StatusFg,
		"paneBorderActive":   req.PaneBorderActive,
		"paneBorderInactive": req.PaneBorderInactive,
		"modeStyleBg":        req.ModeStyleBg,
		"modeStyleFg":        req.ModeStyleFg,
	}

	for key, val := range colors {
		if val != "" && !h.colorRegex.MatchString(val) {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", fmt.Sprintf("Invalid color for %s: %s", key, val))
			return
		}
	}

	// Build tmux set commands
	var commands [][]string
	if req.StatusBg != "" && req.StatusFg != "" {
		commands = append(commands, []string{"set", "-g", "status-style", fmt.Sprintf("bg=%s,fg=%s", req.StatusBg, req.StatusFg)})
	}
	if req.PaneBorderActive != "" {
		commands = append(commands, []string{"set", "-g", "pane-active-border-style", fmt.Sprintf("fg=%s", req.PaneBorderActive)})
	}
	if req.PaneBorderInactive != "" {
		commands = append(commands, []string{"set", "-g", "pane-border-style", fmt.Sprintf("fg=%s", req.PaneBorderInactive)})
	}
	if req.ModeStyleBg != "" && req.ModeStyleFg != "" {
		commands = append(commands, []string{"set", "-g", "mode-style", fmt.Sprintf("bg=%s,fg=%s", req.ModeStyleBg, req.ModeStyleFg)})
	}

	applied := 0
	targets := h.appearanceTargets()
	for _, target := range targets {
		for _, args := range commands {
			_, err := h.runTmuxOnSocket(target.socket, args...)
			if err == nil {
				applied++
			}
			// Ignore errors for appearance - a tmux server/profile might not be running.
		}
	}

	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"applied":   applied,
		"total":     len(commands) * len(targets),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
