// Package core provides business logic and utility functions
package core

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Session represents a tmux session
type Session struct {
	ID                                  string `json:"id,omitempty"`
	Name                                string `json:"name"`
	Windows                             int    `json:"windows"`
	Attached                            bool   `json:"attached"`
	Group                               string `json:"group"`
	UnixUser                            string `json:"unixUser,omitempty"`
	Persistent                          bool   `json:"persistent,omitempty"`
	PersistentIdentity                  string `json:"persistentIdentity,omitempty"`
	PersistentAgentKind                 string `json:"persistentAgentKind,omitempty"`
	PersistentAgentSessionID            string `json:"persistentAgentSessionId,omitempty"`
	PersistentHermesProfile             string `json:"persistentHermesProfile,omitempty"`
	// Supervision health comes from the session's own systemd unit, read live
	// (ADR-0014). The retired fields here -- resume command, a six-state ladder,
	// launch-failure counters, retry and last-check timestamps -- described an
	// in-server supervisor that no longer exists; four of them were never read by
	// any client even while it did.
	PersistentUnit        string `json:"persistentUnit,omitempty"`
	PersistentHealth      string `json:"persistentHealth,omitempty"`
	PersistentActiveState string `json:"persistentActiveState,omitempty"`
	PersistentDetail      string `json:"persistentDetail,omitempty"`
}

// GroupPriority defines the sort order for session groups
var GroupPriority = map[string]int{
	"hq":   0,
	"main": 1,
}

// GetGroupPriority returns the sort priority for a group
func GetGroupPriority(group string) int {
	if p, ok := GroupPriority[group]; ok {
		return p
	}
	if strings.HasPrefix(group, "gt-") {
		return 3
	}
	return 4
}

// CategorizeSession determines the group for a session based on its name
func CategorizeSession(name string) string {
	if strings.HasPrefix(name, "hq-") {
		return "hq"
	}
	if name == "main" || name == "shell" {
		return "main"
	}
	if strings.HasPrefix(name, "gt-") {
		parts := strings.Split(name, "-")
		if len(parts) >= 2 {
			return parts[0] + "-" + parts[1]
		}
		return "gt-unknown"
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

// GetTmuxTmpdir returns the TMUX_TMPDIR environment variable or a portable default.
// Prefers XDG_RUNTIME_DIR/tmux, falls back to /tmp/tmux-<uid>.
func GetTmuxTmpdir() string {
	tmpdir := strings.TrimSpace(os.Getenv("TMUX_TMPDIR"))
	if tmpdir != "" {
		return tmpdir
	}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return xdg + "/tmux"
	}
	return fmt.Sprintf("/tmp/tmux-%d", os.Getuid())
}

// TmuxBin returns the tmux client binary every CHROTE code path must invoke.
// CHROTE_TMUX_BIN pins one client version across the Go server,
// terminal-launch.sh and the grants helper: a tmux 3.4 client cannot talk to a
// 3.6a server at all, so resolving "tmux" from PATH per code path silently
// breaks terminals. Falls back to PATH lookup when unset.
func TmuxBin() string {
	if bin := strings.TrimSpace(os.Getenv("CHROTE_TMUX_BIN")); bin != "" {
		return bin
	}
	return "tmux"
}

// GetTmuxEnv returns the environment for tmux commands
func GetTmuxEnv() []string {
	env := os.Environ()
	tmpdir := GetTmuxTmpdir()
	// Ensure TMUX_TMPDIR is set
	found := false
	for i, e := range env {
		if strings.HasPrefix(e, "TMUX_TMPDIR=") {
			env[i] = "TMUX_TMPDIR=" + tmpdir
			found = true
			break
		}
	}
	if !found {
		env = append(env, "TMUX_TMPDIR="+tmpdir)
	}
	return env
}
