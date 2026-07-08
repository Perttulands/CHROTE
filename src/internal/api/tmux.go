// Package api provides HTTP handlers for the API
package api

import (
	"context"
	"encoding/json"
	"fmt"
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
	"time"

	"github.com/chrote/server/internal/core"
)

// TmuxHandler handles tmux-related API endpoints
type TmuxHandler struct {
	cache      *sessionsCache
	colorRegex *regexp.Regexp
	socket     string
	workDir    string
	bank       *sessionBankStore
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
	Banked        []SessionBankEntry        `json:"banked"`
	TerminalUsers []string                  `json:"terminalUsers"`
	Timestamp     string                    `json:"timestamp"`
	Error         string                    `json:"error,omitempty"`
}

// SessionBankEntry is a durable reminder of a terminal session that CHROTE has
// seen. It survives CHROTE/tmux restarts so agent resume IDs stay visible.
type SessionBankEntry struct {
	ID            string `json:"id,omitempty"`
	Name          string `json:"name"`
	UnixUser      string `json:"unixUser,omitempty"`
	Group         string `json:"group"`
	Windows       int    `json:"windows"`
	Attached      bool   `json:"attached"`
	Live          bool   `json:"live"`
	FirstSeen     string `json:"firstSeen"`
	LastSeen      string `json:"lastSeen"`
	ResumeCommand string `json:"resumeCommand"`
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

// NewTmuxHandler creates the default tmux handler. By default it uses
// TMUX_TMPDIR; CHROTE_DEFAULT_TMUX_SOCKET pins the same /api/tmux route to an
// explicit socket without changing the dashboard UI.
func NewTmuxHandler() *TmuxHandler {
	return &TmuxHandler{
		cache: &sessionsCache{
			ttl: time.Second,
		},
		colorRegex: regexp.MustCompile(`^#[0-9A-Fa-f]{3,6}$|^[a-zA-Z]+$|^default$`),
		socket:     strings.TrimSpace(os.Getenv("CHROTE_DEFAULT_TMUX_SOCKET")),
		workDir:    strings.TrimSpace(os.Getenv("CHROTE_DEFAULT_TMUX_WORKDIR")),
		bank:       newSessionBankStore(defaultSessionBankPath()),
	}
}

// RegisterRoutes registers the tmux routes on the given mux
func (h *TmuxHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tmux/sessions", h.ListSessions)
	mux.HandleFunc("POST /api/tmux/sessions", h.CreateSession)
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
	allowed := map[string]bool{}
	configured := configuredTerminalUsers()
	if len(configured) > 0 {
		for _, item := range configured {
			allowed[item] = true
		}
		return allowed
	}
	if current, err := osuser.Current(); err == nil && current.Username != "" {
		allowed[current.Username] = true
	}
	return allowed
}

func (h *TmuxHandler) targetForUnixUser(unixUser string) (tmuxTarget, error) {
	unixUser = strings.TrimSpace(unixUser)
	if unixUser == "" {
		return tmuxTarget{socket: h.socket, workDir: h.workDir}, nil
	}

	allowed := allowedTerminalUsers()
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
	if target.socket != "" && target.workDir != "" {
		return target, nil
	}

	account, err := osuser.Lookup(unixUser)
	if err != nil {
		return tmuxTarget{}, fmt.Errorf("lookup Unix user %q: %w", unixUser, err)
	}
	currentUser := ""
	if current, err := osuser.Current(); err == nil {
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

const defaultSessionBankFile = "/srv/data/chrote/session-bank/sessions.json"

func defaultSessionBankPath() string {
	if override := strings.TrimSpace(os.Getenv("CHROTE_SESSION_BANK_PATH")); override != "" {
		return override
	}
	return defaultSessionBankFile
}

type sessionBankStore struct {
	path string
	mu   sync.Mutex
}

func newSessionBankStore(path string) *sessionBankStore {
	path = strings.TrimSpace(path)
	if path == "" {
		path = defaultSessionBankFile
	}
	return &sessionBankStore{path: path}
}

func sessionBankKey(name, unixUser string) string {
	return strings.TrimSpace(unixUser) + "\x00" + strings.TrimSpace(name)
}

func resumeCommandForSession(session core.Session) string {
	name := strings.TrimSpace(session.Name)
	if name == "" {
		return ""
	}
	return "/resume " + name
}

func (s *sessionBankStore) Read() ([]SessionBankEntry, error) {
	if s == nil {
		return []SessionBankEntry{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	return sanitizeSessionBankEntries(entries), nil
}

func (s *sessionBankStore) Snapshot(liveSessions []core.Session) ([]SessionBankEntry, error) {
	if s == nil {
		return []SessionBankEntry{}, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	entries = sanitizeSessionBankEntries(entries)
	byKey := map[string]SessionBankEntry{}
	for _, entry := range entries {
		if entry.Name == "" {
			continue
		}
		if entry.Group == "" {
			entry.Group = core.CategorizeSession(entry.Name)
		}
		entry.ResumeCommand = resumeCommandForSession(core.Session{Name: entry.Name})
		byKey[sessionBankKey(entry.Name, entry.UnixUser)] = entry
	}

	liveKeys := map[string]bool{}
	for _, session := range liveSessions {
		if session.Name == "" {
			continue
		}
		key := sessionBankKey(session.Name, session.UnixUser)
		liveKeys[key] = true
		entry := byKey[key]
		if entry.FirstSeen == "" {
			entry.FirstSeen = now
		}
		entry.Name = session.Name
		entry.ID = session.ID
		entry.UnixUser = session.UnixUser
		entry.Group = session.Group
		if entry.Group == "" {
			entry.Group = core.CategorizeSession(session.Name)
		}
		entry.Windows = session.Windows
		entry.Attached = session.Attached
		entry.Live = true
		entry.LastSeen = now
		entry.ResumeCommand = resumeCommandForSession(session)
		byKey[key] = entry
	}

	result := make([]SessionBankEntry, 0, len(byKey))
	for key, entry := range byKey {
		entry.Live = liveKeys[key]
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Live != result[j].Live {
			return result[i].Live
		}
		if result[i].LastSeen != result[j].LastSeen {
			return result[i].LastSeen > result[j].LastSeen
		}
		if result[i].UnixUser != result[j].UnixUser {
			return result[i].UnixUser < result[j].UnixUser
		}
		return result[i].Name < result[j].Name
	})
	if err := s.saveLocked(result); err != nil {
		return nil, err
	}
	return result, nil
}

func sanitizeSessionBankEntries(entries []SessionBankEntry) []SessionBankEntry {
	result := make([]SessionBankEntry, 0, len(entries))
	for _, entry := range entries {
		entry.Name = strings.TrimSpace(entry.Name)
		entry.UnixUser = strings.TrimSpace(entry.UnixUser)
		if valid, _ := core.ValidateSessionName(entry.Name, "session name"); !valid {
			continue
		}
		if entry.Group == "" {
			entry.Group = core.CategorizeSession(entry.Name)
		}
		entry.ResumeCommand = resumeCommandForSession(core.Session{Name: entry.Name})
		result = append(result, entry)
	}
	return result
}

func (s *sessionBankStore) loadLocked() ([]SessionBankEntry, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []SessionBankEntry{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return []SessionBankEntry{}, nil
	}
	var entries []SessionBankEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("read session bank %s: %w", s.path, err)
	}
	return entries, nil
}

func (s *sessionBankStore) saveLocked(entries []SessionBankEntry) error {
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o2770); err != nil {
		return err
	}
	_ = os.Chmod(dir, 0o2770)
	tmp, err := os.CreateTemp(dir, ".tmp-session-bank-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o660); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return err
	}
	_ = os.Chmod(s.path, 0o660)
	return nil
}

func isTmuxNoServerError(errStr string) bool {
	return strings.Contains(errStr, "no server running") ||
		strings.Contains(errStr, "No such file or directory") ||
		(strings.Contains(errStr, "error connecting to ") && strings.Contains(errStr, "(No such file or directory)"))
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

	cmd := exec.CommandContext(ctx, "tmux", args...)
	cmd.Env = core.GetTmuxEnv()

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%s: %s", err.Error(), string(exitErr.Stderr))
		}
		return "", err
	}
	return string(output), nil
}

func parseSessionsOutput(output string, unixUser string) []core.Session {
	sessions := []core.Session{}
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 3 {
			continue
		}
		sessionID := ""
		nameIndex := 0
		if len(parts) >= 4 {
			sessionID = parts[0]
			nameIndex = 1
		}
		name := parts[nameIndex]
		windows, _ := strconv.Atoi(parts[nameIndex+1]) //nolint:errcheck // defaults to 0 on parse failure, corrected to 1 below
		if windows == 0 {
			windows = 1
		}
		sessions = append(sessions, core.Session{
			ID:       sessionID,
			Name:     name,
			Windows:  windows,
			Attached: parts[nameIndex+2] == "1",
			Group:    core.CategorizeSession(name),
			UnixUser: unixUser,
		})
	}
	return sessions
}

func (h *TmuxHandler) listSessionsForTarget(target tmuxTarget) ([]core.Session, string) {
	output, err := h.runTmuxOnSocket(target.socket, "list-sessions", "-F", "#{session_id}:#{session_name}:#{session_windows}:#{session_attached}")
	if err != nil {
		errStr := err.Error()
		if isTmuxNoServerError(errStr) {
			return []core.Session{}, ""
		}
		return []core.Session{}, errStr
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
		Banked:        []SessionBankEntry{},
		TerminalUsers: advertisedTerminalUsers(),
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
	}

	if useConfiguredUsers {
		var errors []string
		for _, unixUser := range configuredTerminalUsers() {
			target, targetErr := h.targetForUnixUser(unixUser)
			if targetErr != nil {
				errors = append(errors, targetErr.Error())
				continue
			}
			sessions, errStr := h.listSessionsForTarget(target)
			if errStr != "" {
				errors = append(errors, fmt.Sprintf("%s: %s", unixUser, errStr))
				continue
			}
			response.Sessions = append(response.Sessions, sessions...)
		}
		if len(errors) > 0 {
			response.Error = strings.Join(errors, "; ")
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
	if h.bank != nil {
		var banked []SessionBankEntry
		var bankErr error
		if queryUnixUser == "" && response.Error == "" {
			banked, bankErr = h.bank.Snapshot(response.Sessions)
		} else {
			banked, bankErr = h.bank.Read()
		}
		if bankErr != nil {
			response.Error = appendSessionResponseError(response.Error, "session bank: "+bankErr.Error())
		} else {
			response.Banked = banked
		}
	}

	// Update cache
	if useCache {
		h.cache.mu.Lock()
		h.cache.data = response
		h.cache.timestamp = time.Now()
		h.cache.mu.Unlock()
	}

	core.WriteJSON(w, http.StatusOK, response)
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

	// Create the session (detached)
	_, err := h.runTmuxOnSocket(target.socket, "new-session", "-d", "-s", name, "-c", workDir)
	if err != nil {
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
	_, _ = h.runTmuxOnSocket(target.socket, "set-option", "-g", "mouse", mouseValue)
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

	// Get list of all sessions first
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
	} else if !isTmuxNoServerError(err.Error()) {
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
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
		_, err := h.runTmuxOnSocket(target.socket, "set-option", "-g", "mouse", value)
		if err == nil {
			applied++
		}
		// Ignore errors: a tmux server/profile may not be running yet.
	}

	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
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
