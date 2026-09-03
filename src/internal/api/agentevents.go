package api

// Agent events: how CHROTE learns that an agent finished a turn or is waiting
// on the operator. Nothing here guesses from terminal output or tmux silence.
// Each harness's own completion hook runs the chrote-agent-event script, which
// posts to this route with the tmux session name it is running in; the server
// keeps that report as the session's last event, in memory, until the
// operator focuses the session. The launcher installs the hooks per launch
// through the harness's own flags.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chrote/server/internal/core"
)

const (
	agentEventFinished   = "finished"
	agentEventNeedsInput = "needs-input"

	// maxAgentEventSummaryRunes bounds what a hook may say about the event. A
	// summary is a hint for a toast and a status line, not a transcript, so a
	// longer one is cut rather than refused: a refused report is a lost one.
	maxAgentEventSummaryRunes = 500
	// maxAgentEventBodyBytes is more than any hook payload; past it the request
	// is not a report.
	maxAgentEventBodyBytes = 64 << 10

	// agentEventURLEnv is the variable the launcher sets in every session it
	// creates while hooks are configured: where chrote-agent-event posts.
	agentEventURLEnv = "CHROTE_AGENT_EVENT_URL"
	// agentEventHookEnv names the script; unset, it is looked for beside the
	// server binary, which is where the installer puts it.
	agentEventHookEnv = "CHROTE_AGENT_EVENT_HOOK"
	// agentHooksDirEnv is where the generated Claude Code settings files go.
	agentHooksDirEnv = "CHROTE_AGENT_HOOKS_DIR"

	agentEventHookScriptName = "chrote-agent-event"
	defaultAgentHooksDir     = "/srv/data/chrote/agent-hooks"
	agentEventTimeLayout     = "2006-01-02T15:04:05.000Z07:00"
	// claudeHookTimeoutSeconds bounds each hook command Claude Code runs. The
	// script bounds its own request to two seconds; this is the ceiling above
	// it, so a wedged script cannot hold a turn open for the default minute.
	claudeHookTimeoutSeconds = 5
)

var errAgentEventNoSuchSession = errors.New("no such session")

// agentEventKey identifies a session across the configured Unix users. Two
// users may own sessions of the same name; their events are not each other's.
type agentEventKey struct {
	unixUser string
	session  string
}

// agentEventStore holds the last event per session. It lives as long as the
// process and nowhere else: a restart forgets, which is what an in-memory
// notice should do. Entries are pruned when a listing shows the session gone,
// so it holds one entry per live session that has ever reported.
type agentEventStore struct {
	mu     sync.Mutex
	events map[agentEventKey]core.AgentEvent
}

func newAgentEventStore() *agentEventStore {
	return &agentEventStore{events: map[agentEventKey]core.AgentEvent{}}
}

// record replaces whatever the session last reported. A new event is unseen
// even if the previous one was looked at: it is news again.
func (s *agentEventStore) record(unixUser, session string, event core.AgentEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	event.Seen = false
	s.events[agentEventKey{unixUser: unixUser, session: session}] = event
}

// markSeen keeps the event as the record of what last happened and drops its
// claim on the operator's attention. It reports whether there was one.
func (s *agentEventStore) markSeen(unixUser, session string) (core.AgentEvent, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := agentEventKey{unixUser: unixUser, session: session}
	event, ok := s.events[key]
	if !ok {
		return core.AgentEvent{}, false
	}
	event.Seen = true
	s.events[key] = event
	return event, true
}

// attach puts each listed session's last event on it and forgets the events
// of this user's sessions that the listing no longer shows. The listing is
// the one place the live set is known, so this is where the store's bound
// comes from; no sweep runs on its own.
func (s *agentEventStore) attach(unixUser string, sessions []core.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	live := make(map[string]bool, len(sessions))
	for i := range sessions {
		live[sessions[i].Name] = true
		if event, ok := s.events[agentEventKey{unixUser: unixUser, session: sessions[i].Name}]; ok {
			copied := event
			sessions[i].LastEvent = &copied
		}
	}
	for key := range s.events {
		if key.unixUser == unixUser && !live[key.session] {
			delete(s.events, key)
		}
	}
}

// AgentEventRequest is the body of POST /api/agent/event, as the hook script
// posts it.
type AgentEventRequest struct {
	Session  string `json:"session"`
	UnixUser string `json:"unixUser,omitempty"`
	Event    string `json:"event"`
	Summary  string `json:"summary,omitempty"`
}

// AgentEventSeenRequest is the body of POST /api/agent/event/seen.
type AgentEventSeenRequest struct {
	Session  string `json:"session"`
	UnixUser string `json:"unixUser,omitempty"`
}

// AgentEventHandler serves the event routes over the tmux handler's sockets
// and store.
type AgentEventHandler struct {
	tmux *TmuxHandler
}

// NewAgentEventHandler creates the handler for the tmux handler whose sessions
// the events belong to.
func NewAgentEventHandler(tmux *TmuxHandler) *AgentEventHandler {
	return &AgentEventHandler{tmux: tmux}
}

// RegisterRoutes registers the agent event routes on the given mux.
func (h *AgentEventHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/agent/event", h.Record)
	mux.HandleFunc("POST /api/agent/event/seen", h.Seen)
}

// Record handles POST /api/agent/event. The session must exist on the managed
// socket of the Unix user it names: a report about a session that is not
// there is a mistake, not a notice.
func (h *AgentEventHandler) Record(w http.ResponseWriter, r *http.Request) {
	var req AgentEventRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAgentEventBodyBytes)).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}
	session := strings.TrimSpace(req.Session)
	if valid, message := core.ValidateSessionName(session, "session"); !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", message)
		return
	}
	event := strings.TrimSpace(req.Event)
	if event != agentEventFinished && event != agentEventNeedsInput {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST",
			fmt.Sprintf("event must be %q or %q", agentEventFinished, agentEventNeedsInput))
		return
	}
	target, err := sendTargetFromRequest(h.tmux, r, req.UnixUser)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if err := h.tmux.requireSession(target.socket, session); err != nil {
		if errors.Is(err, errAgentEventNoSuchSession) {
			core.WriteError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("session %q does not exist", session))
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "TMUX_ERROR", publicTmuxSourceError(err))
		return
	}
	recorded := core.AgentEvent{
		Event:   event,
		Time:    time.Now().UTC().Format(agentEventTimeLayout),
		Summary: boundSummary(req.Summary),
	}
	h.tmux.events.record(target.unixUser, session, recorded)
	core.WriteJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"session":   session,
		"unixUser":  target.unixUser,
		"lastEvent": recorded,
	})
}

// Seen handles POST /api/agent/event/seen: the operator focused the session,
// so its event no longer asks for attention. A session with nothing recorded
// is answered the same way; there is nothing to refuse.
func (h *AgentEventHandler) Seen(w http.ResponseWriter, r *http.Request) {
	var req AgentEventSeenRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAgentEventBodyBytes)).Decode(&req); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}
	session := strings.TrimSpace(req.Session)
	if valid, message := core.ValidateSessionName(session, "session"); !valid {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", message)
		return
	}
	target, err := sendTargetFromRequest(h.tmux, r, req.UnixUser)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	response := map[string]any{
		"success":  true,
		"session":  session,
		"unixUser": target.unixUser,
	}
	if event, ok := h.tmux.events.markSeen(target.unixUser, session); ok {
		response["lastEvent"] = event
	}
	core.WriteJSON(w, http.StatusOK, response)
}

// requireSession asks tmux whether exactly this session exists on the socket.
func (h *TmuxHandler) requireSession(socket, session string) error {
	_, err := h.runTmuxOnSocket(socket, "has-session", "-t", "="+session)
	if err == nil {
		return nil
	}
	diagnostic := strings.TrimPrefix(strings.TrimSpace(tmuxErrorDiagnostic(err)), "exit status 1: ")
	if isTmuxNoServerErrorForSocket(diagnostic, socket) || strings.HasPrefix(diagnostic, "can't find session") {
		return errAgentEventNoSuchSession
	}
	return err
}

func boundSummary(summary string) string {
	summary = strings.TrimSpace(summary)
	runes := []rune(summary)
	if len(runes) > maxAgentEventSummaryRunes {
		return string(runes[:maxAgentEventSummaryRunes])
	}
	return summary
}

// AgentHooks is what the launcher needs to wire a harness's completion hooks
// to this server: the script they run, where it posts, and where a generated
// Claude Code settings file may be written. Its zero value installs nothing.
type AgentHooks struct {
	// Script is the absolute path of chrote-agent-event.
	Script string
	// EventURL is this server's own POST /api/agent/event, as a process on
	// this host reaches it.
	EventURL string
	// SettingsDir holds the generated Claude Code settings, one file per Unix
	// user and session name, overwritten by the next launch of that name.
	SettingsDir string
}

func (hooks AgentHooks) enabled() bool {
	return hooks.Script != "" && hooks.EventURL != ""
}

// LoadAgentHooks reads the hook configuration for a server bound at host and
// port. A missing script does not stop the server: launches then run without
// hooks, and the reason is returned for the startup log.
func LoadAgentHooks(host string, port int) (AgentHooks, string) {
	hooks := AgentHooks{
		EventURL:    agentEventURL(host, port),
		SettingsDir: strings.TrimSpace(os.Getenv(agentHooksDirEnv)),
	}
	if hooks.SettingsDir == "" {
		hooks.SettingsDir = defaultAgentHooksDir
	}
	script := strings.TrimSpace(os.Getenv(agentEventHookEnv))
	if script == "" {
		executable, err := os.Executable()
		if err != nil {
			return AgentHooks{}, fmt.Sprintf("agent event hooks are off: cannot locate the server binary to find %s beside it: %v", agentEventHookScriptName, err)
		}
		script = filepath.Join(filepath.Dir(executable), agentEventHookScriptName)
	}
	info, err := os.Stat(script)
	if err != nil {
		return AgentHooks{}, fmt.Sprintf("agent event hooks are off: %s is not installed at %s (%v)", agentEventHookScriptName, script, err)
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return AgentHooks{}, fmt.Sprintf("agent event hooks are off: %s is not an executable file", script)
	}
	hooks.Script = script
	return hooks, ""
}

// agentEventURL is where a process on this host reaches the server. A bind to
// every interface is reached on loopback; a bind to one address is reached on
// that address, because loopback may not be among the ones it answers on.
func agentEventURL(host string, port int) string {
	host = strings.TrimSpace(host)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/api/agent/event"
}

// sessionEnv is what every session created while hooks are configured is
// given, whatever it runs: a hook the operator wired himself finds the same
// address.
func (hooks AgentHooks) sessionEnv() []string {
	if !hooks.enabled() {
		return nil
	}
	return []string{agentEventURLEnv + "=" + hooks.EventURL}
}

// commandFor adds the harness's hook flags to the launch command. Each
// harness has its own way of being told: Claude Code reads hooks from a
// settings file named on its command line, Codex takes a notify program in a
// -c override. A harness this server does not know launches as it was. It
// returns the command and, when hooks were wanted but could not be installed,
// the reason.
func (hooks AgentHooks) commandFor(harnessID, command, unixUser, session string) (string, string) {
	if !hooks.enabled() || command == "" {
		return command, ""
	}
	switch harnessID {
	case "claude-code":
		path, err := hooks.writeClaudeSettings(unixUser, session)
		if err != nil {
			return command, fmt.Sprintf("launched without completion hooks: %v", err)
		}
		return command + " --settings " + shellQuote(path), ""
	case "codex":
		return command + " -c " + shellQuote(`notify=["`+tomlEscape(hooks.Script)+`","`+agentEventFinished+`"]`), ""
	default:
		return command, ""
	}
}

// writeClaudeSettings writes the settings file a Claude Code launch reads: a
// Stop hook that reports finished and a Notification hook that reports
// needs-input. The file is readable by anyone, because the session's Unix user
// is not necessarily the server's and it holds nothing but two commands.
func (hooks AgentHooks) writeClaudeSettings(unixUser, session string) (string, error) {
	hook := func(event string) map[string]any {
		return map[string]any{
			"hooks": []map[string]any{{
				"type":    "command",
				"command": shellQuote(hooks.Script) + " " + event,
				"timeout": claudeHookTimeoutSeconds,
			}},
		}
	}
	settings := map[string]any{
		"hooks": map[string]any{
			"Stop":         []map[string]any{hook(agentEventFinished)},
			"Notification": []map[string]any{hook(agentEventNeedsInput)},
		},
	}
	raw, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode Claude Code settings: %w", err)
	}
	raw = append(raw, '\n')
	if err := os.MkdirAll(hooks.SettingsDir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", hooks.SettingsDir, err)
	}
	path := filepath.Join(hooks.SettingsDir, unixUser+"."+session+".claude-settings.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	// WriteFile's mode is subject to the umask; the file has to be readable by
	// the session's user whatever the server's umask says.
	if err := os.Chmod(path, 0o644); err != nil {
		return "", fmt.Errorf("make %s readable: %w", path, err)
	}
	return path, nil
}

// shellQuote makes one word of s for the login shell the command is typed
// into. Single quotes are the only quoting every POSIX shell agrees on.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// tomlEscape makes s safe inside a TOML basic string.
func tomlEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}
