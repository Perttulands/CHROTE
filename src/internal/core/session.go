// Package core provides business logic and utility functions
package core

import (
	"os"
	"regexp"
	"sort"
	"strings"
)

// Session represents a tmux session
type Session struct {
	ID             string `json:"id,omitempty"`
	Name           string `json:"name"`
	Windows        int    `json:"windows"`
	Attached       bool   `json:"attached"`
	Group          string `json:"group"`
	UnixUser       string `json:"unixUser,omitempty"`
	CWD            string `json:"cwd,omitempty"`
	CurrentCommand string `json:"currentCommand,omitempty"`
	// Activity is when the session last saw output or input, as tmux counts
	// it, in RFC 3339 UTC. Empty when the inventory did not report it.
	Activity string `json:"activity,omitempty"`

	// The fields below carry facts that contradict what a session looks like
	// from the outside, so the dashboard can say so. They describe the
	// session's current window. Each is optional: an older or degraded
	// inventory simply omits it, and an omitted fact raises no claim.

	// Panes counts the panes in the current window. More than one means the
	// terminal shows only part of what is running.
	Panes int `json:"panes,omitempty"`
	// Width and Height are the current window's size in cells.
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
	// SizePinned reports tmux window-size manual, which fixes the window at
	// Width by Height and makes CHROTE unable to resize it.
	SizePinned bool `json:"sizePinned,omitempty"`
	// MouseEnabled is the session's tmux mouse option. Nil when unknown.
	MouseEnabled *bool `json:"mouseEnabled,omitempty"`
	// ForeignClients lists the ttys of attached clients CHROTE did not
	// create, such as an SSH login. Opening the session watches alongside
	// them; it no longer displaces them.
	ForeignClients []string `json:"foreignClients,omitempty"`
	// Viewers counts every client attached to the session, CHROTE's own and
	// foreign alike. More than one means the window is drawn once for all of
	// them, at the size the claiming viewer set.
	Viewers int `json:"viewers,omitempty"`
	// LastEvent is what the agent inside the session last reported through
	// its own completion hook. Absent when nothing has been reported.
	LastEvent *AgentEvent `json:"lastEvent,omitempty"`
}

// AgentEvent is one report from a harness's own completion hook: the agent
// finished a turn, or it is waiting on the operator. CHROTE keeps the latest
// per session, in memory, until the operator looks at the session.
type AgentEvent struct {
	// Event is "finished" or "needs-input".
	Event string `json:"event"`
	// Time is when the report arrived, RFC 3339 with milliseconds, UTC.
	Time string `json:"time"`
	// Summary is what the harness said about it, if anything.
	Summary string `json:"summary,omitempty"`
	// Seen is true once the operator focused the session after the event.
	Seen bool `json:"seen"`
}

// GroupPriority defines the sort order for session groups
var GroupPriority = map[string]int{
	"main":  1,
	"other": 100,
}

// GetGroupPriority returns the sort priority for a group
func GetGroupPriority(group string) int {
	if p, ok := GroupPriority[group]; ok {
		return p
	}
	return 4
}

// CategorizeSession determines the group for a session based on its name
func CategorizeSession(name string) string {
	if name == "main" || name == "shell" {
		return "main"
	}
	if prefix, _, ok := strings.Cut(name, "-"); ok && prefix != "" {
		return prefix
	}
	trimmed := strings.TrimRight(name, "0123456789")
	if trimmed != "" && trimmed != name {
		return trimmed
	}
	return "other"
}

// SortSessions sorts sessions by group priority, then group name, then session name
func SortSessions(sessions []Session) {
	sort.Slice(sessions, func(i, j int) bool {
		priorityI := GetGroupPriority(sessions[i].Group)
		priorityJ := GetGroupPriority(sessions[j].Group)
		if priorityI != priorityJ {
			return priorityI < priorityJ
		}
		if sessions[i].Group != sessions[j].Group {
			return sessions[i].Group < sessions[j].Group
		}
		return sessions[i].Name < sessions[j].Name
	})
}

// GroupSessions organizes sessions by group
func GroupSessions(sessions []Session) map[string][]Session {
	grouped := make(map[string][]Session)
	for _, s := range sessions {
		grouped[s.Group] = append(grouped[s.Group], s)
	}
	return grouped
}

// SessionNameRegex validates session names
var SessionNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ValidateSessionName validates a session name
func ValidateSessionName(name, paramName string) (bool, string) {
	if name == "" {
		return false, paramName + " is required."
	}
	if !SessionNameRegex.MatchString(name) {
		return false, "Invalid " + paramName + ". Use only letters, numbers, dashes, and underscores."
	}
	if len(name) > 50 {
		return false, paramName + " too long (max 50 characters)."
	}
	return true, ""
}

// TmuxBin returns the tmux client binary every CHROTE code path must invoke.
// CHROTE_TMUX_BIN pins one client version across the Go server and the grants
// helper: a tmux 3.4 client cannot talk to a 3.6a server at all, so resolving
// "tmux" from PATH per code path silently breaks terminals. Falls back to PATH
// lookup when unset.
func TmuxBin() string {
	if bin := strings.TrimSpace(os.Getenv("CHROTE_TMUX_BIN")); bin != "" {
		return bin
	}
	return "tmux"
}
