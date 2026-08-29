package core

import "testing"

func TestGetGroupPriority(t *testing.T) {
	tests := []struct {
		name     string
		group    string
		expected int
	}{
		{"hq group", "hq", 0},
		{"main group", "main", 1},
		{"gastown rig", "gt-gastown", 3},
		{"another rig", "gt-otherrig", 3},
		{"other group", "random", 4},
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
		{"hq session", "hq-main", "hq"},
		{"hq coordinator", "hq-coordinator", "hq"},
		{"main session", "main", "main"},
		{"shell session", "shell", "main"},
		{"gastown worker", "gt-gastown-jack", "gt-gastown"},
		{"gastown simple", "gt-gastown", "gt-gastown"},
		{"gt-only", "gt-", "gt-"},
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
		{Name: "gt-gastown-1", Group: "gt-gastown"},
		{Name: "main", Group: "main"},
		{Name: "hq-main", Group: "hq"},
		{Name: "gt-otherrig-1", Group: "gt-otherrig"},
	}

	SortSessions(sessions)

	// Expected order: hq, main, gt-gastown, gt-otherrig, other
	expectedOrder := []string{"hq-main", "main", "gt-gastown-1", "gt-otherrig-1", "random1"}

	for i, expected := range expectedOrder {
		if sessions[i].Name != expected {
			t.Errorf("Position %d: got %q, expected %q", i, sessions[i].Name, expected)
		}
	}
}

func TestGroupSessions(t *testing.T) {
	sessions := []Session{
		{Name: "hq-1", Group: "hq"},
		{Name: "hq-2", Group: "hq"},
		{Name: "main", Group: "main"},
	}

	grouped := GroupSessions(sessions)

	if len(grouped["hq"]) != 2 {
		t.Errorf("Expected 2 hq sessions, got %d", len(grouped["hq"]))
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
