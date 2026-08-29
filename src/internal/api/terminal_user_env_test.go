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
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/run/user/2001/chrote-tmux/tmux-1000/default,build=/tmp/tmux-2002/default,alice=/run/user/2001/chrote-alt-tmux/default")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "")

	err := ValidateTerminalUserEnv()
	if err == nil {
		t.Fatal("duplicate alice entry in CHROTE_TERMINAL_USER_SOCKETS was accepted; the server would start with listing and attach resolving different sockets")
	}
	message := err.Error()
	for _, want := range []string{
		"CHROTE_TERMINAL_USER_SOCKETS",
		"alice",
		"/run/user/2001/chrote-tmux/tmux-1000/default",
		"/run/user/2001/chrote-alt-tmux/default",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("error %q does not name %q, so an operator cannot tell which entry to remove", message, want)
		}
	}
}

func TestValidateTerminalUserEnv_RejectsDuplicateWorkdirKey(t *testing.T) {
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice,alice=/srv")

	if err := ValidateTerminalUserEnv(); err == nil {
		t.Fatal("duplicate alice entry in CHROTE_TERMINAL_USER_WORKDIRS was accepted")
	}
}

func TestValidateTerminalUserEnv_AcceptsOneEntryPerUser(t *testing.T) {
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", " alice=/run/user/2001/chrote-tmux/tmux-1000/default, build = /tmp/tmux-2002/default ,")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice,build=/home/build")

	if err := ValidateTerminalUserEnv(); err != nil {
		t.Fatalf("valid one-entry-per-user configuration rejected: %v", err)
	}
}

// The Go parser and terminal-launch.sh must agree on the socket for every
// configured user. With duplicates rejected at startup, agreement holds because
// exactly one entry can match.
func TestParseUserValueMap_ResolvesTheSingleEntryPerUser(t *testing.T) {
	sockets := parseUserValueMap(" alice=/run/user/2001/chrote-tmux/tmux-1000/default, build = /tmp/tmux-2002/default ")
	want := map[string]string{
		"alice": "/run/user/2001/chrote-tmux/tmux-1000/default",
		"build": "/tmp/tmux-2002/default",
	}
	for user, socket := range want {
		if sockets[user] != socket {
			t.Fatalf("Go parser resolved %s=%q, want %q", user, sockets[user], socket)
		}
	}
}
