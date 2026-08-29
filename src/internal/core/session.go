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
