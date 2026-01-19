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
	Name     string `json:"name"`
	Windows  int    `json:"windows"`
	Attached bool   `json:"attached"`
	Group    string `json:"group"`
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

// GetTmuxTmpdir returns the TMUX_TMPDIR environment variable or default
func GetTmuxTmpdir() string {
	tmpdir := strings.TrimSpace(os.Getenv("TMUX_TMPDIR"))
	if tmpdir != "" {
		return tmpdir
	}
	return "/tmp"
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
