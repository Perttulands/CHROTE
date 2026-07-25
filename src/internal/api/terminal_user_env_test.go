package api

import (
	"strings"
	"testing"
)

// A duplicated Unix user key is the bug behind "sessions appear in the UI and
// refuse to open": parseUserValueMap is last-wins while terminal-launch.sh is
// first-wins, so listing and attaching resolved different tmux servers. The
// server must refuse to start instead of silently picking one.
func TestValidateTerminalUserEnv_RejectsDuplicateUserSocketKey(t *testing.T) {
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "perttu=/run/user/1000/chrote-tmux/tmux-1000/default,tavern=/tmp/tmux-1001/default,perttu=/run/user/1000/chrote-formations-tmux/default")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "")

	err := ValidateTerminalUserEnv()
	if err == nil {
		t.Fatal("duplicate perttu entry in CHROTE_TERMINAL_USER_SOCKETS was accepted; the server would start with listing and attach resolving different sockets")
	}
	message := err.Error()
	for _, want := range []string{
		"CHROTE_TERMINAL_USER_SOCKETS",
		"perttu",
		"/run/user/1000/chrote-tmux/tmux-1000/default",
		"/run/user/1000/chrote-formations-tmux/default",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("error %q does not name %q, so an operator cannot tell which entry to remove", message, want)
		}
	}
}

func TestValidateTerminalUserEnv_RejectsDuplicateWorkdirKey(t *testing.T) {
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "perttu=/home/perttu,perttu=/srv")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "")

	if err := ValidateTerminalUserEnv(); err == nil {
		t.Fatal("duplicate perttu entry in CHROTE_TERMINAL_USER_WORKDIRS was accepted")
	}
}

func TestValidateTerminalUserEnv_AcceptsOneEntryPerUser(t *testing.T) {
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", " perttu=/run/user/1000/chrote-tmux/tmux-1000/default, tavern = /tmp/tmux-1001/default ,")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "perttu=/home/perttu,tavern=/home/tavern")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "")

	if err := ValidateTerminalUserEnv(); err != nil {
		t.Fatalf("valid one-entry-per-user configuration rejected: %v", err)
	}
}

// The Go parser and terminal-launch.sh must agree on the socket for every
// configured user. With duplicates rejected at startup, agreement holds because
// exactly one entry can match.
func TestParseUserValueMap_ResolvesTheSingleEntryPerUser(t *testing.T) {
	sockets := parseUserValueMap(" perttu=/run/user/1000/chrote-tmux/tmux-1000/default, tavern = /tmp/tmux-1001/default ")
	want := map[string]string{
		"perttu": "/run/user/1000/chrote-tmux/tmux-1000/default",
		"tavern": "/tmp/tmux-1001/default",
	}
	for user, socket := range want {
		if sockets[user] != socket {
			t.Fatalf("Go parser resolved %s=%q, want %q", user, sockets[user], socket)
		}
	}
}
