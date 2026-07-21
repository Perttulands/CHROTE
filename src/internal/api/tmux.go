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
	persistent *persistentAgentStore
	managed    *managedRecoveryStatusStore
	// recoveryMu serializes recovery-owner checks with the store or tmux
	// mutation that makes the winning claim observable.
	recoveryMu sync.Mutex
}

type recoveryOwnershipConflict struct {
	OwnerKind string
	OwnerRef  string
}

func (e *recoveryOwnershipConflict) Error() string {
	if e == nil {
		return "recovery ownership conflict"
	}
	return fmt.Sprintf("%s owns recovery: owner kind %q, ref %q", strings.ReplaceAll(e.OwnerKind, "_", " "), e.OwnerKind, e.OwnerRef)
}

func writeRecoveryOwnershipError(w http.ResponseWriter, conflictCode, failureCode string, err error) {
	var conflict *recoveryOwnershipConflict
	if errors.As(err, &conflict) {
		core.WriteError(w, http.StatusConflict, conflictCode, conflict.Error())
		return
	}
	core.WriteError(w, http.StatusInternalServerError, failureCode, err.Error())
}

type sessionsCache struct {
	mu        sync.RWMutex
	data      *SessionsResponse
	timestamp time.Time
	ttl       time.Duration
}

// SessionsResponse is the response for listing sessions
type SessionsResponse struct {
	Sessions      []core.Session               `json:"sessions"`
	Grouped       map[string][]core.Session    `json:"grouped"`
	Banked        []SessionBankEntry           `json:"banked"`
	Managed       []ManagedRecoveryStatusEntry `json:"managed"`
	TerminalUsers []string                     `json:"terminalUsers"`
	Timestamp     string                       `json:"timestamp"`
	Error         string                       `json:"error,omitempty"`
}

// SessionBankEntry is a durable reminder of a terminal session that CHROTE has
// seen. It survives CHROTE/tmux restarts so agent resume IDs stay visible.
type SessionBankEntry struct {
	ID                  string                       `json:"id,omitempty"`
	Name                string                       `json:"name"`
	UnixUser            string                       `json:"unixUser,omitempty"`
	Group               string                       `json:"group"`
	Windows             int                          `json:"windows"`
	Attached            bool                         `json:"attached"`
	Live                bool                         `json:"live"`
	FirstSeen           string                       `json:"firstSeen"`
	LastSeen            string                       `json:"lastSeen"`
	RecoveryKind        string                       `json:"recoveryKind,omitempty"`
	AgentKind           string                       `json:"agentKind,omitempty"`
	AgentSessionID      string                       `json:"agentSessionId,omitempty"`
	ResumeCommand       string                       `json:"resumeCommand"`
	CWD                 string                       `json:"cwd,omitempty"`
	TranscriptPath      string                       `json:"transcriptPath,omitempty"`
	RecoveryPlan        []WorkloadRecoveryDescriptor `json:"recoveryPlan,omitempty"`
	RecoveryPlanPresent bool                         `json:"-"`
}

// CreateSessionRequest is the request body for creating a session
type CreateSessionRequest struct {
	Name        string `json:"name"`
	UnixUser    string `json:"unixUser,omitempty"`
	MouseScroll *bool  `json:"mouseScroll,omitempty"`
}

// RecoverBankedSessionRequest is the request body for recovering a banked agent session.
type RecoverBankedSessionRequest struct {
	UnixUser     string `json:"unixUser,omitempty"`
	MouseScroll  *bool  `json:"mouseScroll,omitempty"`
	TopologyOnly bool   `json:"topologyOnly,omitempty"`
}

// UpdateBankedRecoveryRequest records agent recovery metadata for a banked session.
type UpdateBankedRecoveryRequest struct {
	UnixUser            string                       `json:"unixUser,omitempty"`
	AgentKind           string                       `json:"agentKind"`
	AgentSessionID      string                       `json:"agentSessionId"`
	CWD                 string                       `json:"cwd,omitempty"`
	TranscriptPath      string                       `json:"transcriptPath,omitempty"`
	RecoveryPlan        []WorkloadRecoveryDescriptor `json:"recoveryPlan,omitempty"`
	RecoveryPlanPresent bool                         `json:"-"`
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
		persistent: newPersistentAgentStore(defaultPersistentAgentsPath()),
		managed:    newManagedRecoveryStatusStore(defaultManagedRecoveryStatusPath()),
	}
}

func (entry *SessionBankEntry) UnmarshalJSON(raw []byte) error {
	type alias SessionBankEntry
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	var decoded alias
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	*entry = SessionBankEntry(decoded)
	_, entry.RecoveryPlanPresent = fields["recoveryPlan"]
	if entry.RecoveryPlanPresent && bytes.Equal(bytes.TrimSpace(fields["recoveryPlan"]), []byte("null")) {
		entry.RecoveryPlan = nil
	}
	return nil
}

func (entry SessionBankEntry) MarshalJSON() ([]byte, error) {
	type alias SessionBankEntry
	encoded, err := json.Marshal(alias(entry))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	if entry.RecoveryPlanPresent {
		plan := entry.RecoveryPlan
		if plan == nil {
			plan = []WorkloadRecoveryDescriptor{}
		}
		planRaw, err := json.Marshal(plan)
		if err != nil {
			return nil, err
		}
		fields["recoveryPlan"] = planRaw
	} else if len(entry.RecoveryPlan) == 0 {
		delete(fields, "recoveryPlan")
	}
	return json.Marshal(fields)
}

func (req *UpdateBankedRecoveryRequest) UnmarshalJSON(raw []byte) error {
	type alias UpdateBankedRecoveryRequest
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	var decoded alias
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	*req = UpdateBankedRecoveryRequest(decoded)
	_, req.RecoveryPlanPresent = fields["recoveryPlan"]
	if req.RecoveryPlanPresent && bytes.Equal(bytes.TrimSpace(fields["recoveryPlan"]), []byte("null")) {
		req.RecoveryPlan = nil
	}
	return nil
}

// RegisterRoutes registers the tmux routes on the given mux
func (h *TmuxHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tmux/sessions", h.ListSessions)
	mux.HandleFunc("POST /api/tmux/sessions", h.CreateSession)
	mux.HandleFunc("POST /api/tmux/sessions/{name}/persistence", h.EnablePersistentAgent)
	mux.HandleFunc("DELETE /api/tmux/sessions/{name}/persistence", h.DisablePersistentAgent)
	mux.HandleFunc("POST /api/tmux/sessions/{name}/send", h.SendToSession)
	mux.HandleFunc("POST /api/tmux/session-bank/{name}/recovery", h.UpdateBankedRecovery)
	mux.HandleFunc("POST /api/tmux/session-bank/{name}/recover", h.RecoverBankedSession)
	mux.HandleFunc("PUT /api/tmux/session-bank/{name}/entry", h.RestoreBankedSessionEntry)
	mux.HandleFunc("DELETE /api/tmux/session-bank/{name}", h.ForgetBankedSession)
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
	socket    string
	workDir   string
	ownerHome string
	unixUser  string
}

var (
	tmuxCurrentUser = osuser.Current
	tmuxLookupUser  = osuser.Lookup
)

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
	if current, err := tmuxCurrentUser(); err == nil && current.Username != "" {
		allowed[current.Username] = true
	}
	return allowed
}

func (h *TmuxHandler) targetForUnixUser(unixUser string) (tmuxTarget, error) {
	unixUser = strings.TrimSpace(unixUser)
	if unixUser == "" {
		target := tmuxTarget{socket: h.socket, workDir: h.workDir}
		if current, err := tmuxCurrentUser(); err == nil && current != nil {
			target.ownerHome = strings.TrimSpace(current.HomeDir)
		}
		return target, nil
	}

	allowed := allowedTerminalUsers()
	if !allowed[unixUser] {
		return tmuxTarget{}, fmt.Errorf("Unix user %q is not allowed for terminal launch", unixUser)
	}

	socketMap := parseUserValueMap(os.Getenv("CHROTE_TERMINAL_USER_SOCKETS"))
	workDirMap := parseUserValueMap(os.Getenv("CHROTE_TERMINAL_USER_WORKDIRS"))
	homeMap := parseUserValueMap(os.Getenv("CHROTE_TERMINAL_USER_HOMES"))
	target := tmuxTarget{
		socket:    socketMap[unixUser],
		workDir:   workDirMap[unixUser],
		ownerHome: homeMap[unixUser],
		unixUser:  unixUser,
	}

	account, err := tmuxLookupUser(unixUser)
	if err != nil {
		if target.socket != "" && target.workDir != "" {
			return target, nil
		}
		return tmuxTarget{}, fmt.Errorf("lookup Unix user %q: %w", unixUser, err)
	}
	currentUser := ""
	if current, err := tmuxCurrentUser(); err == nil && current != nil {
		currentUser = current.Username
	}
	if target.workDir == "" {
		if currentUser == unixUser && h.workDir != "" {
			target.workDir = h.workDir
		} else {
			target.workDir = account.HomeDir
		}
	}
	if target.ownerHome == "" {
		target.ownerHome = account.HomeDir
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
const defaultSessionDropsDir = "/srv/data/chrote/session-drops"
const reservedInternalSessionPrefix = "chrote-probe-"

func isReservedInternalSessionName(name string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), reservedInternalSessionPrefix)
}

func defaultSessionBankPath() string {
	if override := strings.TrimSpace(os.Getenv("CHROTE_SESSION_BANK_PATH")); override != "" {
		return override
	}
	return defaultSessionBankFile
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
	ID        string            `json:"id"`
	Session   string            `json:"session"`
	UnixUser  string            `json:"unixUser,omitempty"`
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

var agentSessionIDRegex = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

const (
	sessionBankRecoveryMaxRequestBytes   int64 = 1 << 20
	sessionBankRecoveryMaxDescriptors          = 128
	sessionBankRecoveryMaxWindows              = 32
	sessionBankRecoveryMaxPanesPerWindow       = 32
)

var errRecoveryRequestBodyTooLarge = errors.New("recovery request body too large")

func resumeCommandForSession(session core.Session) string {
	name := strings.TrimSpace(session.Name)
	if name == "" {
		return ""
	}
	return "/resume " + name
}

func canonicalAgentResumeCommand(kind, sessionID string) (string, bool) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	sessionID = strings.TrimSpace(sessionID)
	if !agentSessionIDRegex.MatchString(sessionID) {
		return "", false
	}
	switch kind {
	case "codex":
		return "codex resume " + sessionID, true
	case "claude":
		return "claude --resume " + sessionID, true
	default:
		return "", false
	}
}

func sanitizeRecoveryPath(value string, requireAbs bool) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\x00\n\r#") {
		return ""
	}
	if requireAbs && !filepath.IsAbs(value) {
		return ""
	}
	return value
}

func sessionBankOwnerRef(unixUser, sessionName string) string {
	unixUser = strings.TrimSpace(unixUser)
	sessionName = strings.TrimSpace(sessionName)
	if unixUser == "" {
		return sessionName
	}
	return unixUser + "/" + sessionName
}

func (h *TmuxHandler) ensureExternalRecoveryOwnershipAvailable(name, unixUser string) error {
	if h == nil || h.managed == nil {
		return nil
	}
	entries, err := h.managed.Read()
	if err != nil {
		return fmt.Errorf("managed status registry: %w", err)
	}
	key := sessionBankKey(name, unixUser)
	for _, entry := range entries {
		if sessionBankKey(entry.Name, entry.UnixUser) == key {
			return &recoveryOwnershipConflict{OwnerKind: entry.Owner.Kind, OwnerRef: entry.Owner.Ref}
		}
	}
	return nil
}

func (h *TmuxHandler) ensureSessionBankOwnershipAvailable(name, unixUser string) error {
	if h == nil {
		return nil
	}
	if err := h.ensureExternalRecoveryOwnershipAvailable(name, unixUser); err != nil {
		return err
	}
	if h.persistent == nil {
		return nil
	}
	found, err := h.persistent.IsPersistent(name, unixUser)
	if err != nil {
		return fmt.Errorf("persistent agent store: %w", err)
	}
	if found {
		return &recoveryOwnershipConflict{
			OwnerKind: RecoveryOwnerPersistentAgent,
			OwnerRef:  persistentAgentOwnerRef(unixUser, name),
		}
	}
	return nil
}

func trustedSessionBankOwnerHome(target tmuxTarget) (string, error) {
	ownerHome := strings.TrimSpace(target.ownerHome)
	if ownerHome == "" {
		return "", fmt.Errorf("trusted owner home is required for descriptor recovery")
	}
	path, err := canonicalPath(ownerHome)
	if err != nil {
		return "", fmt.Errorf("trusted owner home is unsafe: %w", err)
	}
	return path, nil
}

func recoveryKindForStoredPlan(plan []WorkloadRecoveryDescriptor) string {
	if len(plan) == 0 {
		return ""
	}
	allTopology := true
	for _, desc := range plan {
		switch strings.ToLower(strings.TrimSpace(desc.Mode)) {
		case RecoveryModeUnresolved:
			return RecoveryModeUnresolved
		case RecoveryModeTopology:
		default:
			allTopology = false
		}
	}
	if allTopology {
		return RecoveryModeTopology
	}
	return "descriptor-plan"
}

func sanitizeSessionBankEntry(entry SessionBankEntry) (SessionBankEntry, bool) {
	entry.Name = strings.TrimSpace(entry.Name)
	entry.UnixUser = strings.TrimSpace(entry.UnixUser)
	if isReservedInternalSessionName(entry.Name) {
		return SessionBankEntry{}, false
	}
	if valid, _ := core.ValidateSessionName(entry.Name, "session name"); !valid {
		return SessionBankEntry{}, false
	}
	if entry.Group == "" {
		entry.Group = core.CategorizeSession(entry.Name)
	}
	entry.CWD = sanitizeRecoveryPath(entry.CWD, true)
	entry.TranscriptPath = sanitizeRecoveryPath(entry.TranscriptPath, false)
	if entry.RecoveryPlanPresent || len(entry.RecoveryPlan) > 0 {
		if len(entry.RecoveryPlan) == 0 {
			entry.RecoveryPlan = []WorkloadRecoveryDescriptor{}
			entry.AgentKind = ""
			entry.AgentSessionID = ""
			entry.ResumeCommand = ""
			entry.RecoveryKind = RecoveryModeUnresolved
			return entry, true
		}
		entry.RecoveryPlanPresent = true
		entry.AgentKind = ""
		entry.AgentSessionID = ""
		entry.RecoveryKind = recoveryKindForStoredPlan(entry.RecoveryPlan)
		entry.ResumeCommand = ""
		return entry, true
	}
	if command, ok := canonicalAgentResumeCommand(entry.AgentKind, entry.AgentSessionID); ok {
		entry.AgentKind = strings.ToLower(strings.TrimSpace(entry.AgentKind))
		entry.AgentSessionID = strings.TrimSpace(entry.AgentSessionID)
		entry.ResumeCommand = command
		entry.RecoveryKind = "agent"
		return entry, true
	}
	entry.AgentKind = ""
	entry.AgentSessionID = ""
	entry.RecoveryKind = "shell"
	entry.ResumeCommand = resumeCommandForSession(core.Session{Name: entry.Name})
	return entry, true
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

func (s *sessionBankStore) Forget(name, unixUser string) (bool, error) {
	if s == nil {
		return false, nil
	}
	name = strings.TrimSpace(name)
	unixUser = strings.TrimSpace(unixUser)
	if valid, _ := core.ValidateSessionName(name, "session name"); !valid {
		return false, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return false, err
	}
	entries = sanitizeSessionBankEntries(entries)
	key := sessionBankKey(name, unixUser)
	filtered := make([]SessionBankEntry, 0, len(entries))
	removed := false
	for _, entry := range entries {
		if sessionBankKey(entry.Name, entry.UnixUser) == key {
			removed = true
			continue
		}
		filtered = append(filtered, entry)
	}
	if !removed {
		return false, nil
	}
	if err := s.saveLocked(filtered); err != nil {
		return false, err
	}
	return true, nil
}

func (s *sessionBankStore) Find(name, unixUser string) (SessionBankEntry, bool, error) {
	if s == nil {
		return SessionBankEntry{}, false, nil
	}
	name = strings.TrimSpace(name)
	unixUser = strings.TrimSpace(unixUser)
	if valid, _ := core.ValidateSessionName(name, "session name"); !valid {
		return SessionBankEntry{}, false, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return SessionBankEntry{}, false, err
	}
	key := sessionBankKey(name, unixUser)
	for _, entry := range sanitizeSessionBankEntries(entries) {
		if sessionBankKey(entry.Name, entry.UnixUser) == key {
			return entry, true, nil
		}
	}
	return SessionBankEntry{}, false, nil
}

func (s *sessionBankStore) RestoreEntry(name, unixUser string, entry SessionBankEntry, ownerHome string) (SessionBankEntry, error) {
	if s == nil {
		return SessionBankEntry{}, fmt.Errorf("session bank is unavailable")
	}
	name = strings.TrimSpace(name)
	unixUser = strings.TrimSpace(unixUser)
	if valid, errMsg := core.ValidateSessionName(name, "session name"); !valid {
		return SessionBankEntry{}, fmt.Errorf("%s", errMsg)
	}
	entry.Name = strings.TrimSpace(entry.Name)
	entry.UnixUser = strings.TrimSpace(entry.UnixUser)
	if entry.Name != name {
		return SessionBankEntry{}, fmt.Errorf("entry name %q does not match route %q", entry.Name, name)
	}
	if entry.UnixUser != unixUser {
		return SessionBankEntry{}, fmt.Errorf("entry unixUser %q does not match route %q", entry.UnixUser, unixUser)
	}
	if entry.RecoveryPlanPresent && len(entry.RecoveryPlan) > 0 {
		plan, err := validateSessionBankRecoveryPlan(entry.Name, entry.UnixUser, ownerHome, entry.RecoveryPlan, true)
		if err != nil {
			return SessionBankEntry{}, err
		}
		entry.RecoveryPlan = plan.Descriptors
	}
	entry, ok := sanitizeSessionBankEntry(entry)
	if !ok {
		return SessionBankEntry{}, fmt.Errorf("session bank entry is unsafe")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return SessionBankEntry{}, err
	}
	entries = sanitizeSessionBankEntries(entries)
	key := sessionBankKey(name, unixUser)
	replaced := false
	for i := range entries {
		if sessionBankKey(entries[i].Name, entries[i].UnixUser) == key {
			entries[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, entry)
	}
	if err := s.saveLocked(entries); err != nil {
		return SessionBankEntry{}, err
	}
	return entry, nil
}

type sessionBankRecoveryPane struct {
	Index      int
	CWD        string
	Descriptor WorkloadRecoveryDescriptor
	Command    string
}

type sessionBankRecoveryWindow struct {
	Index  int
	Name   string
	Layout string
	CWD    string
	Panes  []sessionBankRecoveryPane
}

type validatedSessionBankRecoveryPlan struct {
	Descriptors []WorkloadRecoveryDescriptor
	Windows     []sessionBankRecoveryWindow
}

func validateSessionBankRecoveryPlan(name, unixUser, ownerHome string, descriptors []WorkloadRecoveryDescriptor, topologyOnly bool) (validatedSessionBankRecoveryPlan, error) {
	name = strings.TrimSpace(name)
	unixUser = strings.TrimSpace(unixUser)
	ownerHome = strings.TrimSpace(ownerHome)
	if len(descriptors) == 0 {
		return validatedSessionBankRecoveryPlan{}, fmt.Errorf("recovery plan is empty")
	}
	if len(descriptors) > sessionBankRecoveryMaxDescriptors {
		return validatedSessionBankRecoveryPlan{}, fmt.Errorf("recovery plan descriptors %d exceeds max %d", len(descriptors), sessionBankRecoveryMaxDescriptors)
	}
	if valid, errMsg := core.ValidateSessionName(name, "session name"); !valid {
		return validatedSessionBankRecoveryPlan{}, fmt.Errorf("%s", errMsg)
	}
	expectedOwnerRef := sessionBankOwnerRef(unixUser, name)
	var owner *WorkloadRecoveryOwner
	targets := map[string]bool{}
	paneIDs := map[string]bool{}
	windows := map[int]*sessionBankRecoveryWindow{}
	result := validatedSessionBankRecoveryPlan{
		Descriptors: make([]WorkloadRecoveryDescriptor, 0, len(descriptors)),
	}

	for i, raw := range descriptors {
		desc, err := CanonicalizeWorkloadRecoveryDescriptor(raw, ownerHome)
		if err != nil {
			return validatedSessionBankRecoveryPlan{}, fmt.Errorf("unsafe recovery descriptor %d: %w", i, err)
		}
		if desc.Owner.Kind != RecoveryOwnerSessionBank {
			return validatedSessionBankRecoveryPlan{}, fmt.Errorf("recovery descriptor %d must be session_bank-owned", i)
		}
		if desc.Owner.Ref != expectedOwnerRef {
			return validatedSessionBankRecoveryPlan{}, fmt.Errorf("recovery owner ref %q does not match target %q", desc.Owner.Ref, expectedOwnerRef)
		}
		if owner == nil {
			ownerCopy := desc.Owner
			owner = &ownerCopy
		} else if desc.Owner.Kind != owner.Kind || desc.Owner.Ref != owner.Ref {
			return validatedSessionBankRecoveryPlan{}, fmt.Errorf("conflicting recovery owners")
		}
		if desc.Topology.SessionName != name {
			return validatedSessionBankRecoveryPlan{}, fmt.Errorf("recovery descriptor %d targets session %q, want %q", i, desc.Topology.SessionName, name)
		}
		if desc.Topology.PaneCurrentPath == "" {
			return validatedSessionBankRecoveryPlan{}, fmt.Errorf("recovery descriptor %d requires pane cwd", i)
		}
		if desc.Mode == RecoveryModeUnresolved && !topologyOnly {
			return validatedSessionBankRecoveryPlan{}, fmt.Errorf("unresolved recovery descriptor requires topologyOnly")
		}
		targetKey := fmt.Sprintf("%d.%d", desc.Topology.WindowIndex, desc.Topology.PaneIndex)
		if targets[targetKey] {
			return validatedSessionBankRecoveryPlan{}, fmt.Errorf("duplicate recovery pane target %s", targetKey)
		}
		targets[targetKey] = true
		if desc.Topology.PaneID != "" {
			if paneIDs[desc.Topology.PaneID] {
				return validatedSessionBankRecoveryPlan{}, fmt.Errorf("duplicate recovery pane id %s", desc.Topology.PaneID)
			}
			paneIDs[desc.Topology.PaneID] = true
		}

		window := windows[desc.Topology.WindowIndex]
		if window == nil {
			window = &sessionBankRecoveryWindow{
				Index:  desc.Topology.WindowIndex,
				Name:   desc.Topology.WindowName,
				Layout: desc.Topology.WindowLayout,
			}
			windows[window.Index] = window
		} else {
			if window.Name != desc.Topology.WindowName {
				return validatedSessionBankRecoveryPlan{}, fmt.Errorf("conflicting recovery window name for index %d", window.Index)
			}
			if window.Layout != desc.Topology.WindowLayout {
				return validatedSessionBankRecoveryPlan{}, fmt.Errorf("conflicting recovery window layout for index %d", window.Index)
			}
		}
		if desc.Topology.PaneIndex == 0 {
			window.CWD = desc.Topology.PaneCurrentPath
		}
		command := ""
		if !topologyOnly {
			if desc.Mode == RecoveryModeAgent || desc.Mode == RecoveryModeCommand {
				var ok bool
				command, ok = desc.CanonicalCommand(ownerHome)
				if !ok {
					return validatedSessionBankRecoveryPlan{}, fmt.Errorf("recovery descriptor %d has no canonical command", i)
				}
			}
		}
		window.Panes = append(window.Panes, sessionBankRecoveryPane{
			Index:      desc.Topology.PaneIndex,
			CWD:        desc.Topology.PaneCurrentPath,
			Descriptor: desc,
			Command:    command,
		})
		result.Descriptors = append(result.Descriptors, desc)
	}

	if len(windows) == 0 {
		return validatedSessionBankRecoveryPlan{}, fmt.Errorf("recovery plan has no windows")
	}
	if len(windows) > sessionBankRecoveryMaxWindows {
		return validatedSessionBankRecoveryPlan{}, fmt.Errorf("recovery plan windows %d exceeds max %d", len(windows), sessionBankRecoveryMaxWindows)
	}
	windowIndexes := make([]int, 0, len(windows))
	for index := range windows {
		windowIndexes = append(windowIndexes, index)
	}
	sort.Ints(windowIndexes)
	firstWindowIndex := windowIndexes[0]
	for windowOrdinal, index := range windowIndexes {
		if index != firstWindowIndex+windowOrdinal {
			return validatedSessionBankRecoveryPlan{}, fmt.Errorf("recovery windows must be contiguous")
		}
		window := windows[index]
		sort.Slice(window.Panes, func(i, j int) bool { return window.Panes[i].Index < window.Panes[j].Index })
		if len(window.Panes) == 0 {
			return validatedSessionBankRecoveryPlan{}, fmt.Errorf("recovery panes for window %d are empty", index)
		}
		if len(window.Panes) > sessionBankRecoveryMaxPanesPerWindow {
			return validatedSessionBankRecoveryPlan{}, fmt.Errorf("recovery panes for window %d count %d exceeds max %d", index, len(window.Panes), sessionBankRecoveryMaxPanesPerWindow)
		}
		firstPaneIndex := window.Panes[0].Index
		for paneOrdinal, pane := range window.Panes {
			if pane.Index != firstPaneIndex+paneOrdinal {
				return validatedSessionBankRecoveryPlan{}, fmt.Errorf("recovery panes for window %d must be contiguous", index)
			}
		}
		if window.CWD == "" {
			window.CWD = window.Panes[0].CWD
		}
		normalizedWindow := *window
		normalizedWindow.Index = windowOrdinal
		normalizedWindow.Panes = make([]sessionBankRecoveryPane, len(window.Panes))
		for paneOrdinal, pane := range window.Panes {
			pane.Index = paneOrdinal
			normalizedWindow.Panes[paneOrdinal] = pane
		}
		result.Windows = append(result.Windows, normalizedWindow)
	}

	return result, nil
}

func (s *sessionBankStore) UpsertRecovery(name, unixUser string, req UpdateBankedRecoveryRequest, ownerHome string) (SessionBankEntry, error) {
	if s == nil {
		return SessionBankEntry{}, fmt.Errorf("session bank is unavailable")
	}
	name = strings.TrimSpace(name)
	unixUser = strings.TrimSpace(unixUser)
	if valid, errMsg := core.ValidateSessionName(name, "session name"); !valid {
		return SessionBankEntry{}, fmt.Errorf("%s", errMsg)
	}
	var plan validatedSessionBankRecoveryPlan
	if req.RecoveryPlanPresent && len(req.RecoveryPlan) == 0 {
		return SessionBankEntry{}, fmt.Errorf("recovery plan is empty")
	}
	if req.RecoveryPlanPresent || len(req.RecoveryPlan) > 0 {
		var err error
		plan, err = validateSessionBankRecoveryPlan(name, unixUser, ownerHome, req.RecoveryPlan, true)
		if err != nil {
			return SessionBankEntry{}, err
		}
	}
	command, ok := canonicalAgentResumeCommand(req.AgentKind, req.AgentSessionID)
	if len(req.RecoveryPlan) == 0 && !ok {
		return SessionBankEntry{}, fmt.Errorf("unsafe or unsupported agent recovery metadata")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return SessionBankEntry{}, err
	}
	entries = sanitizeSessionBankEntries(entries)
	key := sessionBankKey(name, unixUser)
	updated := false
	var resultEntry SessionBankEntry
	for i, entry := range entries {
		if sessionBankKey(entry.Name, entry.UnixUser) != key {
			continue
		}
		if len(req.RecoveryPlan) > 0 {
			entry.AgentKind = ""
			entry.AgentSessionID = ""
			entry.ResumeCommand = ""
			entry.RecoveryKind = recoveryKindForStoredPlan(plan.Descriptors)
			entry.RecoveryPlan = plan.Descriptors
			entry.RecoveryPlanPresent = true
			entry.Windows = len(plan.Windows)
			entry.CWD = plan.Windows[0].CWD
		} else {
			entry.AgentKind = strings.ToLower(strings.TrimSpace(req.AgentKind))
			entry.AgentSessionID = strings.TrimSpace(req.AgentSessionID)
			entry.ResumeCommand = command
			entry.RecoveryKind = "agent"
			entry.RecoveryPlan = nil
			entry.RecoveryPlanPresent = false
			entry.CWD = sanitizeRecoveryPath(req.CWD, true)
		}
		entry.TranscriptPath = sanitizeRecoveryPath(req.TranscriptPath, false)
		if entry.FirstSeen == "" {
			entry.FirstSeen = now
		}
		entry.LastSeen = now
		entry, _ = sanitizeSessionBankEntry(entry)
		entries[i] = entry
		resultEntry = entry
		updated = true
		break
	}
	if !updated {
		entry := SessionBankEntry{
			Name:      name,
			UnixUser:  unixUser,
			Group:     core.CategorizeSession(name),
			Windows:   1,
			FirstSeen: now,
			LastSeen:  now,
		}
		if len(req.RecoveryPlan) > 0 {
			entry.Windows = len(plan.Windows)
			entry.RecoveryKind = recoveryKindForStoredPlan(plan.Descriptors)
			entry.CWD = plan.Windows[0].CWD
			entry.RecoveryPlan = plan.Descriptors
			entry.RecoveryPlanPresent = true
		} else {
			entry.AgentKind = strings.ToLower(strings.TrimSpace(req.AgentKind))
			entry.AgentSessionID = strings.TrimSpace(req.AgentSessionID)
			entry.ResumeCommand = command
			entry.RecoveryKind = "agent"
			entry.RecoveryPlanPresent = false
			entry.CWD = sanitizeRecoveryPath(req.CWD, true)
		}
		entry.TranscriptPath = sanitizeRecoveryPath(req.TranscriptPath, false)
		entry, _ = sanitizeSessionBankEntry(entry)
		entries = append(entries, entry)
		resultEntry = entry
	}
	if err := s.saveLocked(entries); err != nil {
		return SessionBankEntry{}, err
	}
	return resultEntry, nil
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
		entry, ok := sanitizeSessionBankEntry(entry)
		if !ok {
			continue
		}
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
		entry, _ = sanitizeSessionBankEntry(entry)
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
		entry, ok := sanitizeSessionBankEntry(entry)
		if !ok {
			continue
		}
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
		if isReservedInternalSessionName(name) {
			continue
		}
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
		Managed:       []ManagedRecoveryStatusEntry{},
		TerminalUsers: advertisedTerminalUsers(),
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
	}
	var managedEntries []ManagedRecoveryStatusEntry
	var managedErr error
	if h.managed != nil {
		managedEntries, managedErr = h.managed.Read()
		if managedErr != nil {
			response.Error = appendSessionResponseError(response.Error, "managed status: "+managedErr.Error())
		} else {
			response.Managed = managedEntries
		}
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

	if h.persistent != nil {
		var persistentErr error
		response.Sessions, persistentErr = h.persistent.AnnotateSessions(response.Sessions)
		if persistentErr != nil {
			response.Error = appendSessionResponseError(response.Error, "persistent agents: "+persistentErr.Error())
		}
	}
	if managedErr == nil {
		response.Sessions = filterLiveSessionsForManagedStatus(response.Sessions, managedEntries)
	}
	core.SortSessions(response.Sessions)
	response.Grouped = core.GroupSessions(response.Sessions)
	if h.bank != nil {
		var banked []SessionBankEntry
		var bankErr error
		if queryUnixUser == "" && response.Error == "" {
			liveForBank := response.Sessions
			if h.persistent != nil {
				var filterLiveErr error
				liveForBank, filterLiveErr = h.persistent.FilterLiveSessionsForBank(liveForBank)
				if filterLiveErr != nil {
					response.Error = appendSessionResponseError(response.Error, "persistent agents: "+filterLiveErr.Error())
				}
			}
			if managedErr == nil {
				liveForBank = filterLiveSessionsForManagedStatus(liveForBank, managedEntries)
			}
			banked, bankErr = h.bank.Snapshot(liveForBank)
		} else {
			banked, bankErr = h.bank.Read()
		}
		if bankErr != nil {
			response.Error = appendSessionResponseError(response.Error, "session bank: "+bankErr.Error())
		} else {
			if h.persistent != nil {
				var filterErr error
				banked, filterErr = h.persistent.FilterBanked(banked)
				if filterErr != nil {
					response.Error = appendSessionResponseError(response.Error, "persistent agents: "+filterErr.Error())
				}
			}
			if managedErr == nil {
				banked = filterBankedForManagedStatus(banked, managedEntries)
			}
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

// ForgetBankedSession handles DELETE /api/tmux/session-bank/{name}.
// It removes a durable offline resume hint from the session bank without
// touching live tmux sessions. A later full scan will re-add a session if it is
// still live.
func (h *TmuxHandler) ForgetBankedSession(w http.ResponseWriter, r *http.Request) {
	sessionName := strings.TrimSpace(r.PathValue("name"))
	valid, errMsg := core.ValidateSessionName(sessionName, "session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}
	unixUser := ""
	if r != nil {
		unixUser = strings.TrimSpace(r.URL.Query().Get("unixUser"))
	}
	if h.bank == nil {
		core.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success":   true,
			"removed":   false,
			"session":   sessionName,
			"unixUser":  unixUser,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	removed, err := h.bank.Forget(sessionName, unixUser)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "SESSION_BANK_ERROR", err.Error())
		return
	}
	h.invalidateCache()
	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"removed":   removed,
		"session":   sessionName,
		"unixUser":  unixUser,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// RestoreBankedSessionEntry replaces one Session Bank entry from a previously
// accepted API response. It is used only for snapshot rollback compensation and
// never touches tmux.
func (h *TmuxHandler) RestoreBankedSessionEntry(w http.ResponseWriter, r *http.Request) {
	sessionName := strings.TrimSpace(r.PathValue("name"))
	valid, errMsg := core.ValidateSessionName(sessionName, "session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}
	unixUser := ""
	if r != nil {
		unixUser = strings.TrimSpace(r.URL.Query().Get("unixUser"))
	}
	if h.bank == nil {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Session bank is unavailable")
		return
	}

	var entry SessionBankEntry
	if err := decodeOptionalJSONBodyLimited(w, r, &entry, sessionBankRecoveryMaxRequestBytes); err != nil {
		if errors.Is(err, errRecoveryRequestBodyTooLarge) {
			core.WriteError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", fmt.Sprintf("Session Bank entry body exceeds %d bytes", sessionBankRecoveryMaxRequestBytes))
			return
		}
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}
	if strings.TrimSpace(entry.Name) == "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Session Bank entry name is required")
		return
	}
	if strings.TrimSpace(entry.Name) != sessionName {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Session Bank entry name does not match route")
		return
	}
	if strings.TrimSpace(entry.UnixUser) != unixUser {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Session Bank entry unixUser does not match route")
		return
	}

	ownerHome := ""
	if entry.RecoveryPlanPresent && len(entry.RecoveryPlan) > 0 {
		target, targetErr := h.targetForUnixUser(unixUser)
		if targetErr != nil {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", targetErr.Error())
			return
		}
		var ownerHomeErr error
		ownerHome, ownerHomeErr = trustedSessionBankOwnerHome(target)
		if ownerHomeErr != nil {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", ownerHomeErr.Error())
			return
		}
	}
	h.recoveryMu.Lock()
	defer h.recoveryMu.Unlock()
	if err := h.ensureSessionBankOwnershipAvailable(sessionName, unixUser); err != nil {
		writeRecoveryOwnershipError(w, "SESSION_BANK_OWNERSHIP_CONFLICT", "SESSION_BANK_ERROR", err)
		return
	}
	restored, err := h.bank.RestoreEntry(sessionName, unixUser, entry, ownerHome)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	h.invalidateCache()
	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"session":      restored.Name,
		"unixUser":     restored.UnixUser,
		"recoveryKind": restored.RecoveryKind,
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
	})
}

func bankedRecoveryWorkDir(entry SessionBankEntry, target tmuxTarget) string {
	if entry.CWD != "" {
		return entry.CWD
	}
	if target.workDir != "" {
		return target.workDir
	}
	return core.GetWorkDir()
}

const (
	tmuxCreationTokenEnv          = "CHROTE_CREATION_TOKEN"
	tmuxOwnershipMismatchResponse = "CHROTE_OWNERSHIP_MISMATCH"
)

var (
	tmuxSessionIDPattern     = regexp.MustCompile(`^\$[0-9]+$`)
	tmuxWindowIDPattern      = regexp.MustCompile(`^@[0-9]+$`)
	tmuxPaneIDPattern        = regexp.MustCompile(`^%[0-9]+$`)
	tmuxCreationTokenPattern = regexp.MustCompile(`^[0-9a-f]{24}$`)
)

type ownedTmuxSession struct {
	ID    string
	Name  string
	Token string
}

type ownedTmuxRecoverySession struct {
	ownedTmuxSession
	WindowID string
	PaneID   string
}

type createdRecoveryWindow struct {
	ID    string
	Panes map[int]string
}

func newTmuxCreationToken() (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func isTmuxMissingTargetError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return isTmuxNoServerError(err.Error()) ||
		strings.Contains(message, "can't find session") ||
		strings.Contains(message, "no such session")
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
		if isTmuxMissingTargetError(err) {
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

func parseTmuxRecoverySessionOutput(output string) (string, string, string, error) {
	parts := strings.Split(strings.TrimSpace(output), "\t")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("tmux recovery session output should contain session, window, and pane IDs")
	}
	sessionID := strings.TrimSpace(parts[0])
	windowID := strings.TrimSpace(parts[1])
	paneID := strings.TrimSpace(parts[2])
	if !tmuxSessionIDPattern.MatchString(sessionID) {
		return "", "", "", fmt.Errorf("tmux recovery session output has invalid session ID %q", sessionID)
	}
	if !tmuxWindowIDPattern.MatchString(windowID) {
		return "", "", "", fmt.Errorf("tmux recovery session output has invalid window ID %q", windowID)
	}
	if !tmuxPaneIDPattern.MatchString(paneID) {
		return "", "", "", fmt.Errorf("tmux recovery session output has invalid pane ID %q", paneID)
	}
	return sessionID, windowID, paneID, nil
}

func parseTmuxRecoveryWindowOutput(output string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(output), "\t")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("tmux recovery window output should contain window and pane IDs")
	}
	windowID := strings.TrimSpace(parts[0])
	paneID := strings.TrimSpace(parts[1])
	if !tmuxWindowIDPattern.MatchString(windowID) {
		return "", "", fmt.Errorf("tmux recovery window output has invalid window ID %q", windowID)
	}
	if !tmuxPaneIDPattern.MatchString(paneID) {
		return "", "", fmt.Errorf("tmux recovery window output has invalid pane ID %q", paneID)
	}
	return windowID, paneID, nil
}

func parseTmuxRecoveryPaneOutput(output string) (string, error) {
	paneID := strings.TrimSpace(output)
	if !tmuxPaneIDPattern.MatchString(paneID) {
		return "", fmt.Errorf("tmux recovery pane output has invalid pane ID %q", paneID)
	}
	return paneID, nil
}

func (h *TmuxHandler) createOwnedTmuxRecoverySessionWithWindow(parent context.Context, socket, name, workDir, windowName string) (ownedTmuxRecoverySession, error) {
	token, err := newTmuxCreationToken()
	if err != nil {
		return ownedTmuxRecoverySession{}, fmt.Errorf("generate tmux creation token: %w", err)
	}
	session := ownedTmuxRecoverySession{
		ownedTmuxSession: ownedTmuxSession{Name: name, Token: token},
	}
	args := []string{
		"new-session", "-d", "-P", "-F", "#{session_id}\t#{window_id}\t#{pane_id}",
		"-e", tmuxCreationTokenEnv + "=" + token,
		"-s", name, "-c", workDir,
	}
	if strings.TrimSpace(windowName) != "" {
		args = append(args, "-n", strings.TrimSpace(windowName))
	}
	output, createErr := h.runTmuxOnSocketContext(parent, socket, args...)
	if createErr != nil {
		return ownedTmuxRecoverySession{}, h.cleanupOwnedTmuxSessionAfterError(socket, session.ownedTmuxSession, createErr)
	}
	sessionID, windowID, paneID, parseErr := parseTmuxRecoverySessionOutput(output)
	if parseErr != nil {
		return ownedTmuxRecoverySession{}, h.cleanupOwnedTmuxSessionAfterError(socket, session.ownedTmuxSession, parseErr)
	}
	session.ID = sessionID
	session.WindowID = windowID
	session.PaneID = paneID
	return session, nil
}

func (h *TmuxHandler) tmuxSessionExists(parent context.Context, socket, name string) (bool, error) {
	if parent == nil {
		parent = context.Background()
	}
	_, err := h.runTmuxOnSocketContext(parent, socket, "has-session", "-t", name)
	if err == nil {
		return true, nil
	}
	if isTmuxMissingTargetError(err) {
		return false, nil
	}
	return false, err
}

func (h *TmuxHandler) recoverSessionBankPlan(parent context.Context, target tmuxTarget, entry SessionBankEntry, plan validatedSessionBankRecoveryPlan, mouseScroll bool) (int, error) {
	if len(plan.Windows) == 0 {
		return 0, fmt.Errorf("recovery plan has no windows")
	}
	session, err := h.createOwnedTmuxRecoverySessionWithWindow(parent, target.socket, entry.Name, plan.Windows[0].CWD, plan.Windows[0].Name)
	if err != nil {
		return 0, err
	}
	createdWindows := map[int]createdRecoveryWindow{
		plan.Windows[0].Index: {
			ID:    session.WindowID,
			Panes: map[int]string{0: session.PaneID},
		},
	}

	for _, window := range plan.Windows[1:] {
		args := []string{"new-window", "-d", "-P", "-F", "#{window_id}\t#{pane_id}", "-t", session.ID}
		if window.Name != "" {
			args = append(args, "-n", window.Name)
		}
		args = append(args, "-c", window.CWD)
		output, err := h.runTmuxOnSocketContext(parent, target.socket, args...)
		if err != nil {
			return 0, h.cleanupOwnedTmuxSessionAfterError(target.socket, session.ownedTmuxSession, err)
		}
		windowID, paneID, err := parseTmuxRecoveryWindowOutput(output)
		if err != nil {
			return 0, h.cleanupOwnedTmuxSessionAfterError(target.socket, session.ownedTmuxSession, err)
		}
		createdWindows[window.Index] = createdRecoveryWindow{
			ID:    windowID,
			Panes: map[int]string{0: paneID},
		}
	}

	for _, window := range plan.Windows {
		created := createdWindows[window.Index]
		basePaneID := created.Panes[0]
		if basePaneID == "" {
			return 0, h.cleanupOwnedTmuxSessionAfterError(target.socket, session.ownedTmuxSession, fmt.Errorf("missing base pane ID for recovery window %d", window.Index))
		}
		for _, pane := range window.Panes {
			if pane.Index == 0 {
				continue
			}
			output, err := h.runTmuxOnSocketContext(parent, target.socket,
				"split-window", "-d", "-P", "-F", "#{pane_id}", "-t", basePaneID, "-c", pane.CWD,
			)
			if err != nil {
				return 0, h.cleanupOwnedTmuxSessionAfterError(target.socket, session.ownedTmuxSession, err)
			}
			paneID, err := parseTmuxRecoveryPaneOutput(output)
			if err != nil {
				return 0, h.cleanupOwnedTmuxSessionAfterError(target.socket, session.ownedTmuxSession, err)
			}
			created.Panes[pane.Index] = paneID
		}
		createdWindows[window.Index] = created
	}

	for _, window := range plan.Windows {
		if window.Layout == "" {
			continue
		}
		windowID := createdWindows[window.Index].ID
		if windowID == "" {
			return 0, h.cleanupOwnedTmuxSessionAfterError(target.socket, session.ownedTmuxSession, fmt.Errorf("missing window ID for recovery window %d", window.Index))
		}
		if _, err := h.runTmuxOnSocketContext(parent, target.socket, "select-layout", "-t", windowID, window.Layout); err != nil {
			return 0, h.cleanupOwnedTmuxSessionAfterError(target.socket, session.ownedTmuxSession, err)
		}
	}

	mouseValue := "off"
	if mouseScroll {
		mouseValue = "on"
	}
	if err := h.applyMouseMode(parent, target.socket, mouseValue); err != nil {
		return 0, h.cleanupOwnedTmuxSessionAfterError(target.socket, session.ownedTmuxSession, err)
	}

	launched := 0
	for _, window := range plan.Windows {
		for _, pane := range window.Panes {
			if pane.Command == "" {
				continue
			}
			paneTarget := createdWindows[window.Index].Panes[pane.Index]
			if paneTarget == "" {
				return launched, h.cleanupOwnedTmuxSessionAfterError(target.socket, session.ownedTmuxSession, fmt.Errorf("missing pane ID for recovery target %d.%d", window.Index, pane.Index))
			}
			if _, err := h.runTmuxOnSocketContext(parent, target.socket, "send-keys", "-t", paneTarget, "-l", pane.Command); err != nil {
				return launched, h.cleanupOwnedTmuxSessionAfterError(target.socket, session.ownedTmuxSession, err)
			}
			if _, err := h.runTmuxOnSocketContext(parent, target.socket, "send-keys", "-t", paneTarget, "Enter"); err != nil {
				return launched, h.cleanupOwnedTmuxSessionAfterError(target.socket, session.ownedTmuxSession, err)
			}
			launched++
		}
	}

	return launched, nil
}

func decodeOptionalJSONBody(r *http.Request, dest any) error {
	return decodeOptionalJSONBodyLimited(nil, r, dest, 0)
}

func decodeOptionalJSONBodyLimited(w http.ResponseWriter, r *http.Request, dest any, maxBytes int64) error {
	if r == nil || r.Body == nil {
		return nil
	}
	if maxBytes <= 0 {
		if err := json.NewDecoder(r.Body).Decode(dest); err != nil && err != io.EOF {
			return err
		}
		return nil
	}
	if r.ContentLength > maxBytes {
		return errRecoveryRequestBodyTooLarge
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
	if err != nil {
		return err
	}
	if int64(len(raw)) > maxBytes {
		return errRecoveryRequestBodyTooLarge
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request body must contain exactly one JSON value")
		}
		return err
	}
	return nil
}

// UpdateBankedRecovery handles POST /api/tmux/session-bank/{name}/recovery.
// It records safe agent resume metadata captured outside the tmux listing path.
func (h *TmuxHandler) UpdateBankedRecovery(w http.ResponseWriter, r *http.Request) {
	sessionName := strings.TrimSpace(r.PathValue("name"))
	valid, errMsg := core.ValidateSessionName(sessionName, "session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}
	var req UpdateBankedRecoveryRequest
	if err := decodeOptionalJSONBodyLimited(w, r, &req, sessionBankRecoveryMaxRequestBytes); err != nil {
		if errors.Is(err, errRecoveryRequestBodyTooLarge) {
			core.WriteError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", fmt.Sprintf("Recovery request body exceeds %d bytes", sessionBankRecoveryMaxRequestBytes))
			return
		}
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}
	unixUser := strings.TrimSpace(req.UnixUser)
	if r != nil {
		if queryUser := strings.TrimSpace(r.URL.Query().Get("unixUser")); queryUser != "" {
			unixUser = queryUser
		}
	}
	if h.bank == nil {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Session bank is unavailable")
		return
	}
	ownerHome := ""
	if len(req.RecoveryPlan) > 0 {
		target, targetErr := h.targetForUnixUser(unixUser)
		if targetErr != nil {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", targetErr.Error())
			return
		}
		var ownerHomeErr error
		ownerHome, ownerHomeErr = trustedSessionBankOwnerHome(target)
		if ownerHomeErr != nil {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", ownerHomeErr.Error())
			return
		}
	}
	h.recoveryMu.Lock()
	defer h.recoveryMu.Unlock()
	if err := h.ensureSessionBankOwnershipAvailable(sessionName, unixUser); err != nil {
		writeRecoveryOwnershipError(w, "SESSION_BANK_OWNERSHIP_CONFLICT", "SESSION_BANK_ERROR", err)
		return
	}
	entry, err := h.bank.UpsertRecovery(sessionName, unixUser, req, ownerHome)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	h.invalidateCache()
	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":        true,
		"session":        entry.Name,
		"unixUser":       entry.UnixUser,
		"recoveryKind":   entry.RecoveryKind,
		"agentKind":      entry.AgentKind,
		"agentSessionId": entry.AgentSessionID,
		"resumeCommand":  entry.ResumeCommand,
		"cwd":            entry.CWD,
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	})
}

// RecoverBankedSession handles POST /api/tmux/session-bank/{name}/recover.
// It creates fresh tmux transport for a durable Codex/Claude agent transcript.
func (h *TmuxHandler) RecoverBankedSession(w http.ResponseWriter, r *http.Request) {
	sessionName := strings.TrimSpace(r.PathValue("name"))
	valid, errMsg := core.ValidateSessionName(sessionName, "session name")
	if !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", errMsg)
		return
	}

	var req RecoverBankedSessionRequest
	if err := decodeOptionalJSONBodyLimited(w, r, &req, sessionBankRecoveryMaxRequestBytes); err != nil {
		if errors.Is(err, errRecoveryRequestBodyTooLarge) {
			core.WriteError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", fmt.Sprintf("Recovery request body exceeds %d bytes", sessionBankRecoveryMaxRequestBytes))
			return
		}
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}
	unixUser := strings.TrimSpace(req.UnixUser)
	if r != nil {
		if queryUser := strings.TrimSpace(r.URL.Query().Get("unixUser")); queryUser != "" {
			unixUser = queryUser
		}
	}
	if h.bank == nil {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Session bank is unavailable")
		return
	}
	h.recoveryMu.Lock()
	defer h.recoveryMu.Unlock()
	if err := h.ensureSessionBankOwnershipAvailable(sessionName, unixUser); err != nil {
		writeRecoveryOwnershipError(w, "SESSION_BANK_OWNERSHIP_CONFLICT", "SESSION_BANK_ERROR", err)
		return
	}
	entry, found, err := h.bank.Find(sessionName, unixUser)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "SESSION_BANK_ERROR", err.Error())
		return
	}
	if !found {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Session bank entry not found")
		return
	}
	if entry.RecoveryPlanPresent && len(entry.RecoveryPlan) == 0 {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "recovery plan is empty")
		return
	}

	target, targetErr := h.targetForUnixUser(entry.UnixUser)
	if targetErr != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", targetErr.Error())
		return
	}
	mouseScroll := true
	if req.MouseScroll != nil {
		mouseScroll = *req.MouseScroll
	}

	if len(entry.RecoveryPlan) > 0 {
		ownerHome, ownerHomeErr := trustedSessionBankOwnerHome(target)
		if ownerHomeErr != nil {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", ownerHomeErr.Error())
			return
		}
		plan, err := validateSessionBankRecoveryPlan(entry.Name, entry.UnixUser, ownerHome, entry.RecoveryPlan, req.TopologyOnly)
		if err != nil {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
			return
		}
		exists, err := h.tmuxSessionExists(r.Context(), target.socket, entry.Name)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
			return
		}
		if exists {
			core.WriteJSON(w, http.StatusOK, map[string]interface{}{
				"success":      true,
				"action":       "skip-live",
				"session":      entry.Name,
				"unixUser":     entry.UnixUser,
				"topologyOnly": req.TopologyOnly,
				"launched":     0,
				"timestamp":    time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
		launched, err := h.recoverSessionBankPlan(r.Context(), target, entry, plan, mouseScroll)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
			return
		}
		h.invalidateCache()
		core.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success":      true,
			"action":       "recovered",
			"session":      entry.Name,
			"unixUser":     entry.UnixUser,
			"topologyOnly": req.TopologyOnly,
			"launched":     launched,
			"windows":      len(plan.Windows),
			"timestamp":    time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	resumeCommand, ok := canonicalAgentResumeCommand(entry.AgentKind, entry.AgentSessionID)
	if !ok {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Session bank entry has no safe agent resume metadata")
		return
	}
	exists, err := h.tmuxSessionExists(r.Context(), target.socket, entry.Name)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		return
	}
	if exists {
		core.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success":        true,
			"action":         "skip-live",
			"session":        entry.Name,
			"unixUser":       entry.UnixUser,
			"agentKind":      entry.AgentKind,
			"agentSessionId": entry.AgentSessionID,
			"resumeCommand":  resumeCommand,
			"timestamp":      time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	workDir := bankedRecoveryWorkDir(entry, target)
	if workDir == "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "No recovery working directory available")
		return
	}

	session, err := h.createOwnedTmuxSession(r.Context(), target.socket, entry.Name, workDir)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "TMUX_ERROR", err.Error())
		return
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
	if _, err := h.runTmuxOnSocketContext(r.Context(), target.socket, "send-keys", "-t", session.ID, "-l", resumeCommand); err != nil {
		err = h.cleanupOwnedTmuxSessionAfterError(target.socket, session, err)
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		return
	}
	if _, err := h.runTmuxOnSocketContext(r.Context(), target.socket, "send-keys", "-t", session.ID, "Enter"); err != nil {
		err = h.cleanupOwnedTmuxSessionAfterError(target.socket, session, err)
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		return
	}

	h.invalidateCache()
	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":        true,
		"session":        entry.Name,
		"unixUser":       entry.UnixUser,
		"agentKind":      entry.AgentKind,
		"agentSessionId": entry.AgentSessionID,
		"resumeCommand":  resumeCommand,
		"cwd":            workDir,
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
	})
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

func writeSessionDrop(r *http.Request, sessionName string, target tmuxTarget) (sessionDropManifest, error) {
	if r == nil {
		return sessionDropManifest{}, fmt.Errorf("request is missing")
	}
	text := strings.TrimRight(strings.ReplaceAll(r.FormValue("text"), "\r\n", "\n"), "\x00")
	fileHeaders := []*multipart.FileHeader{}
	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		fileHeaders = append(fileHeaders, r.MultipartForm.File["files"]...)
		fileHeaders = append(fileHeaders, r.MultipartForm.File["file"]...)
	}
	if strings.TrimSpace(text) == "" && len(fileHeaders) == 0 {
		return sessionDropManifest{}, fmt.Errorf("send text or at least one file")
	}

	dropID, err := newSessionDropID()
	if err != nil {
		return sessionDropManifest{}, fmt.Errorf("create drop id: %w", err)
	}
	dropRoot := defaultSessionDropsPath()
	if strings.TrimSpace(dropRoot) == "" {
		return sessionDropManifest{}, fmt.Errorf("session drops path is empty")
	}
	dropPath := filepath.Join(dropRoot, dropID)
	filesDir := filepath.Join(dropPath, "files")
	if err := os.MkdirAll(filesDir, 0o755); err != nil {
		return sessionDropManifest{}, fmt.Errorf("create drop directory: %w", err)
	}
	_ = os.Chmod(dropRoot, 0o755)
	_ = os.Chmod(dropPath, 0o755)
	_ = os.Chmod(filesDir, 0o755)

	manifest := sessionDropManifest{
		ID:        dropID,
		Session:   sessionName,
		UnixUser:  target.unixUser,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Payload:   filepath.Join(dropPath, "payload.txt"),
		Files:     []sessionDropFile{},
	}
	if text != "" {
		manifest.TextPath = filepath.Join(dropPath, "text.txt")
		if err := os.WriteFile(manifest.TextPath, []byte(text), 0o644); err != nil {
			return sessionDropManifest{}, fmt.Errorf("write drop text: %w", err)
		}
		_ = os.Chmod(manifest.TextPath, 0o644)
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
		dest, err := os.OpenFile(destPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
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
		_ = os.Chmod(destPath, 0o644)
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
	if err := os.WriteFile(manifest.Payload, []byte(payload), 0o644); err != nil {
		return sessionDropManifest{}, fmt.Errorf("write drop payload: %w", err)
	}
	_ = os.Chmod(manifest.Payload, 0o644)

	manifestPath := filepath.Join(dropPath, "manifest.json")
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return sessionDropManifest{}, fmt.Errorf("marshal drop manifest: %w", err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		return sessionDropManifest{}, fmt.Errorf("write drop manifest: %w", err)
	}
	_ = os.Chmod(manifestPath, 0o644)
	return manifest, nil
}

func submitFormValue(raw string) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	return raw == "" || raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

// SendToSession handles POST /api/tmux/sessions/{name}/send.
// It stores sent text/files durably on disk, then pastes a composed payload into
// the target tmux session via tmux buffers so multiline content never rides in
// command argv.
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
	target, targetErr := targetFromRequest(h, r, r.FormValue("unixUser"))
	if targetErr != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", targetErr.Error())
		return
	}

	manifest, err := writeSessionDrop(r, sessionName, target)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	bufferName := "chrote-send-" + manifest.ID
	if _, err := h.runTmuxOnSocket(target.socket, "load-buffer", "-b", bufferName, manifest.Payload); err != nil {
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		return
	}
	if _, err := h.runTmuxOnSocket(target.socket, "paste-buffer", "-d", "-b", bufferName, "-t", sessionName); err != nil {
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
		return
	}
	if submitFormValue(r.FormValue("submit")) {
		if _, err := h.runTmuxOnSocket(target.socket, "send-keys", "-t", sessionName, "Enter"); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
			return
		}
	}
	core.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"session":   sessionName,
		"unixUser":  target.unixUser,
		"dropId":    manifest.ID,
		"dropPath":  filepath.Dir(manifest.Payload),
		"payload":   manifest.Payload,
		"files":     manifest.Files,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
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
	if err := h.ensureManagedRecoveryOwnershipAvailable(name, target.unixUser); err != nil {
		core.WriteError(w, http.StatusConflict, "SESSION_OWNERSHIP_CONFLICT", err.Error())
		return
	}

	workDir := target.workDir
	if workDir == "" {
		workDir = core.GetWorkDir()
	}

	// Create the session (detached) with an ownership marker and immutable ID.
	session, err := h.createOwnedTmuxSession(r.Context(), target.socket, name, workDir)
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
	if err := h.ensureManagedRecoveryOwnershipAvailable(sessionName, target.unixUser); err != nil {
		core.WriteError(w, http.StatusConflict, "SESSION_OWNERSHIP_CONFLICT", err.Error())
		return
	}
	if h.persistent != nil {
		persistent, err := h.persistent.IsPersistent(sessionName, target.unixUser)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "PERSISTENT_AGENT_ERROR", err.Error())
			return
		}
		if persistent {
			core.WriteError(w, http.StatusConflict, "PERSISTENT_AGENT_LOCKED", "Session is persistent. Make it mortal before killing it.")
			return
		}
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
	managedNames := map[string]bool{}
	if h.managed != nil {
		managedEntries, managedErr := h.managed.Read()
		if managedErr != nil {
			core.WriteError(w, http.StatusConflict, "SESSION_OWNERSHIP_CONFLICT", "managed status registry: "+managedErr.Error())
			return
		}
		for _, entry := range managedEntries {
			if strings.TrimSpace(entry.UnixUser) == strings.TrimSpace(target.unixUser) {
				managedNames[entry.Name] = true
			}
		}
	}

	// Get list of all sessions first
	persistentNames := map[string]bool{}
	if h.persistent != nil {
		var persistentErr error
		persistentNames, persistentErr = h.persistent.NamesForUser(target.unixUser)
		if persistentErr != nil {
			core.WriteError(w, http.StatusInternalServerError, "PERSISTENT_AGENT_ERROR", persistentErr.Error())
			return
		}
	}
	output, err := h.runTmuxOnSocket(target.socket, "list-sessions", "-F", "#{session_name}")
	var sessionNames []string
	var protectedNames []string
	if err == nil {
		lines := strings.Split(strings.TrimSpace(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				if protectedSessions[line] || persistentNames[line] || managedNames[line] {
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
	h.recoveryMu.Lock()
	defer h.recoveryMu.Unlock()
	if err := h.ensureExternalRecoveryOwnershipAvailable(oldName, target.unixUser); err != nil {
		writeRecoveryOwnershipError(w, "SESSION_OWNERSHIP_CONFLICT", "PERSISTENT_AGENT_ERROR", err)
		return
	}
	if oldName != req.NewName {
		if err := h.ensureExternalRecoveryOwnershipAvailable(req.NewName, target.unixUser); err != nil {
			writeRecoveryOwnershipError(w, "SESSION_OWNERSHIP_CONFLICT", "PERSISTENT_AGENT_ERROR", err)
			return
		}
	}
	sourcePersistent := false
	if h.persistent != nil {
		var persistentErr error
		sourcePersistent, persistentErr = h.persistent.IsPersistent(oldName, target.unixUser)
		if persistentErr != nil {
			core.WriteError(w, http.StatusInternalServerError, "PERSISTENT_AGENT_ERROR", persistentErr.Error())
			return
		}
	}
	if oldName != req.NewName {
		ownerHome, ownerHomeErr := trustedSessionBankOwnerHome(target)
		if ownerHomeErr != nil {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", ownerHomeErr.Error())
			return
		}
		if err := h.ensurePersistentAgentOwnershipAvailable(oldName, target.unixUser, ownerHome); err != nil {
			writeRecoveryOwnershipError(w, "PERSISTENT_AGENT_OWNERSHIP_CONFLICT", "PERSISTENT_AGENT_ERROR", err)
			return
		}
		if err := h.ensurePersistentAgentOwnershipAvailable(req.NewName, target.unixUser, ownerHome); err != nil {
			writeRecoveryOwnershipError(w, "PERSISTENT_AGENT_OWNERSHIP_CONFLICT", "PERSISTENT_AGENT_ERROR", err)
			return
		}
		if h.persistent != nil {
			if err := h.persistent.EnsureTargetAvailable(req.NewName, target.unixUser); err != nil {
				core.WriteError(w, http.StatusConflict, "PERSISTENT_AGENT_TARGET_EXISTS", err.Error())
				return
			}
		}
	}

	if sourcePersistent {
		if err := h.persistent.Rename(oldName, req.NewName, target.unixUser); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "PERSISTENT_AGENT_ERROR", err.Error())
			return
		}
		if _, err := h.runTmuxOnSocket(target.socket, "rename-session", "-t", oldName, req.NewName); err != nil {
			if rollbackErr := h.persistent.Rename(req.NewName, oldName, target.unixUser); rollbackErr != nil {
				core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", errors.Join(fmt.Errorf("tmux rename failed: %w", err), fmt.Errorf("persistent rollback failed: %w", rollbackErr)).Error())
				return
			}
			core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
			return
		}
	} else {
		_, err := h.runTmuxOnSocket(target.socket, "rename-session", "-t", oldName, req.NewName)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", err.Error())
			return
		}
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
