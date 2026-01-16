package tmux

import (
	"sort"
	"strings"
)

// Group priority constants
const (
	PriorityHQ    = 0
	PriorityMain  = 1
	PriorityGT    = 2
	PriorityOther = 100
)

// CategorizeSession extracts the group category from a session name
func CategorizeSession(name string) string {
	// hq-mayor → "hq"
	if strings.HasPrefix(name, "hq-") {
		return "hq"
	}

	// main or shell → "main"
	if name == "main" || name == "shell" {
		return "main"
	}

	// gt-gastown-jack → "gt-gastown"
	if strings.HasPrefix(name, "gt-") {
		parts := strings.Split(name, "-")
		if len(parts) >= 2 {
			return parts[0] + "-" + parts[1]
		}
		return "gt-unknown"
	}

	return "other"
}

// GetGroupPriority returns the sort priority for a group
func GetGroupPriority(group string) int {
	switch group {
	case "hq":
		return PriorityHQ
	case "main":
		return PriorityMain
	default:
		if strings.HasPrefix(group, "gt-") {
			return PriorityGT
		}
		return PriorityOther
	}
}

// GetGroupDisplayName returns the human-readable group name
func GetGroupDisplayName(group string) string {
	switch group {
	case "hq":
		return "HQ"
	case "main":
		return "Main"
	case "other":
		return "Other"
	default:
		// gt-gastown → "Gastown"
		if strings.HasPrefix(group, "gt-") {
			parts := strings.Split(group, "-")
			if len(parts) >= 2 {
				return strings.Title(parts[1])
			}
		}
		return strings.Title(group)
	}
}

// SortSessions sorts sessions by group priority, group name, then session name
func SortSessions(sessions []Session) {
	sort.Slice(sessions, func(i, j int) bool {
		// First by group priority
		pi := GetGroupPriority(sessions[i].Group)
		pj := GetGroupPriority(sessions[j].Group)
		if pi != pj {
			return pi < pj
		}

		// Then by group name
		if sessions[i].Group != sessions[j].Group {
			return sessions[i].Group < sessions[j].Group
		}

		// Finally by session name
		return sessions[i].Name < sessions[j].Name
	})
}

// GroupSessions organizes sessions into a map by group
func GroupSessions(sessions []Session) map[string][]Session {
	grouped := make(map[string][]Session)

	for _, session := range sessions {
		grouped[session.Group] = append(grouped[session.Group], session)
	}

	return grouped
}

// SortedGroupKeys returns group keys sorted by priority
func SortedGroupKeys(grouped map[string][]Session) []string {
	keys := make([]string, 0, len(grouped))
	for k := range grouped {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		pi := GetGroupPriority(keys[i])
		pj := GetGroupPriority(keys[j])
		if pi != pj {
			return pi < pj
		}
		return keys[i] < keys[j]
	})

	return keys
}
