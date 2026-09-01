package core

import "testing"

func TestGetGroupPriority(t *testing.T) {
	tests := []struct {
		name     string
		group    string
		expected int
	}{
		{"main group", "main", 1},
		{"named group", "project", 4},
		{"another named group", "team", 4},
		{"other group", "random", 4},
		{"ungrouped sessions last", "other", 100},
		{"empty group", "", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetGroupPriority(tt.group)
			if result != tt.expected {
				t.Errorf("GetGroupPriority(%q) = %d, expected %d", tt.group, result, tt.expected)
			}
		})
	}
}

func TestCategorizeSession(t *testing.T) {
	tests := []struct {
		name     string
		session  string
		expected string
	}{
		{"main session", "main", "main"},
		{"shell session", "shell", "main"},
		{"generic project session", "project-api", "project"},
		{"generic project worker", "project-worker-1", "project"},
		{"dynamic dashed prefix", "alice-shell", "alice"},
		{"dynamic multi dash prefix", "worker-agent-1", "worker"},
		{"dynamic numeric suffix prefix", "forge1", "forge"},
		{"random session", "random", "other"},
		{"tmux default numeric prefix", "tmux1", "tmux"},
		{"empty", "", "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CategorizeSession(tt.session)
			if result != tt.expected {
				t.Errorf("CategorizeSession(%q) = %q, expected %q", tt.session, result, tt.expected)
			}
		})
	}
}

func TestSortSessions(t *testing.T) {
	sessions := []Session{
		{Name: "random1", Group: "other"},
		{Name: "team-worker", Group: "team"},
		{Name: "main", Group: "main"},
		{Name: "project-api", Group: "project"},
		{Name: "project-worker", Group: "project"},
	}

	SortSessions(sessions)

	// Main comes first, then named groups alphabetically, and ungrouped sessions last.
	expectedOrder := []string{"main", "project-api", "project-worker", "team-worker", "random1"}

	for i, expected := range expectedOrder {
		if sessions[i].Name != expected {
			t.Errorf("Position %d: got %q, expected %q", i, sessions[i].Name, expected)
		}
	}
}

func TestGroupSessions(t *testing.T) {
	sessions := []Session{
		{Name: "project-1", Group: "project"},
		{Name: "project-2", Group: "project"},
		{Name: "main", Group: "main"},
	}

	grouped := GroupSessions(sessions)

	if len(grouped["project"]) != 2 {
		t.Errorf("Expected 2 project sessions, got %d", len(grouped["project"]))
	}
	if len(grouped["main"]) != 1 {
		t.Errorf("Expected 1 main session, got %d", len(grouped["main"]))
	}
}

func TestValidateSessionName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		paramName string
		valid     bool
		errMsg    string
	}{
		{"valid simple", "mysession", "session name", true, ""},
		{"valid with dash", "my-session", "session name", true, ""},
		{"valid with underscore", "my_session", "session name", true, ""},
		{"valid with numbers", "session123", "session name", true, ""},
		{"empty", "", "session name", false, "session name is required."},
		{"with spaces", "my session", "session name", false, "Invalid session name. Use only letters, numbers, dashes, and underscores."},
		{"with special chars", "my@session", "session name", false, "Invalid session name. Use only letters, numbers, dashes, and underscores."},
		{"too long", "aaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeee1", "session name", false, "session name too long (max 50 characters)."},
		{"exactly 50 chars", "aaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeee", "session name", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, errMsg := ValidateSessionName(tt.input, tt.paramName)
			if valid != tt.valid {
				t.Errorf("ValidateSessionName(%q) valid = %v, expected %v", tt.input, valid, tt.valid)
			}
			if errMsg != tt.errMsg {
				t.Errorf("ValidateSessionName(%q) errMsg = %q, expected %q", tt.input, errMsg, tt.errMsg)
			}
		})
	}
}
