package core

import "testing"

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

// Sorting and grouping are two readings of one ordering rule, so they share a
// fixture: the order the panel shows and the buckets it draws must agree. A
// session whose group could not be read carries an empty group and still sorts
// among the named ones, rather than falling into the trailing block reserved for
// sessions that were categorised as ungrouped.
func TestSortSessionsOrdersGroupsAndBucketsThem(t *testing.T) {
	sessions := []Session{
		{Name: "random1", Group: "other"},
		{Name: "team-worker", Group: "team"},
		{Name: "main", Group: "main"},
		{Name: "project-api", Group: "project"},
		{Name: "project-worker", Group: "project"},
		{Name: "unreadable-group", Group: ""},
	}

	SortSessions(sessions)

	// Main comes first, then named groups alphabetically, and ungrouped sessions last.
	expectedOrder := []string{"main", "unreadable-group", "project-api", "project-worker", "team-worker", "random1"}

	for i, expected := range expectedOrder {
		if sessions[i].Name != expected {
			t.Errorf("Position %d: got %q, expected %q", i, sessions[i].Name, expected)
		}
	}

	grouped := GroupSessions(sessions)
	wantSizes := map[string]int{"main": 1, "project": 2, "team": 1, "other": 1, "": 1}
	for group, want := range wantSizes {
		if got := len(grouped[group]); got != want {
			t.Errorf("group %q holds %d sessions, expected %d", group, got, want)
		}
	}
	if len(grouped) != len(wantSizes) {
		t.Errorf("groups = %d, expected %d: %#v", len(grouped), len(wantSizes), grouped)
	}
}

// CHROTE_TMUX_BIN pins the tmux the server runs. Falling back to a bare name is
// what lets a test put a fake first on PATH, and the pin is what lets an
// operator name a tmux that is not on the service's PATH at all.
func TestTmuxBin_PrefersPinnedBinaryOverPathLookup(t *testing.T) {
	t.Setenv("CHROTE_TMUX_BIN", " /home/linuxbrew/.linuxbrew/bin/tmux ")
	if got := TmuxBin(); got != "/home/linuxbrew/.linuxbrew/bin/tmux" {
		t.Fatalf("TmuxBin() = %q, want the pinned CHROTE_TMUX_BIN path", got)
	}

	t.Setenv("CHROTE_TMUX_BIN", "")
	if got := TmuxBin(); got != "tmux" {
		t.Fatalf("TmuxBin() = %q, want %q when CHROTE_TMUX_BIN is unset", got, "tmux")
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
