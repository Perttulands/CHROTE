package api

import (
	"strings"
	"testing"
)

// A duplicated Unix user key is the bug behind "sessions appear in the UI and
// refuse to open": listing and attaching can otherwise resolve different tmux
// servers. The server must refuse to start instead of silently picking one, and
// a relative socket path is refused for the same reason, because what it
// resolves to depends on the working directory of whoever ran the command.
//
// Every row sets both variables, so the verdict is the fixture's and never the
// ambient configuration of the machine running the suite.
func TestValidateTerminalUserEnv(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		sockets   string
		workdirs  string
		wantError bool
		// wantNamed is what the refusal has to say for an operator to know
		// which entry to remove.
		wantNamed []string
	}{
		{
			name:      "one entry per user is accepted, spaces and a trailing comma and all",
			sockets:   " alice=/run/user/2001/chrote-tmux/tmux-1000/default, build = /tmp/tmux-2002/default ,",
			workdirs:  "alice=/home/alice,build=/home/build",
			wantError: false,
		},
		{
			name:      "a duplicate socket key is refused and both sockets named",
			sockets:   "alice=/run/user/2001/chrote-tmux/tmux-1000/default,build=/tmp/tmux-2002/default,alice=/run/user/2001/chrote-alt-tmux/default",
			workdirs:  "",
			wantError: true,
			wantNamed: []string{
				"CHROTE_TMUX_SOCKET",
				"alice",
				"/run/user/2001/chrote-tmux/tmux-1000/default",
				"/run/user/2001/chrote-alt-tmux/default",
			},
		},
		{
			name:      "a duplicate working-directory key is refused and both directories named",
			sockets:   "alice=/tmp/tmux-2001/default",
			workdirs:  "alice=/home/alice,alice=/srv",
			wantError: true,
			wantNamed: []string{"CHROTE_TERMINAL_USER_WORKDIRS", "alice", "/home/alice", "/srv"},
		},
		{
			name:      "a relative socket path is refused as not absolute",
			sockets:   "alice=relative/default",
			workdirs:  "",
			wantError: true,
			wantNamed: []string{"absolute"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("CHROTE_TMUX_SOCKET", testCase.sockets)
			t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", testCase.workdirs)

			err := ValidateTerminalUserEnv()
			if !testCase.wantError {
				if err != nil {
					t.Fatalf("valid configuration rejected: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("the server would have started on a configuration that cannot resolve one socket per user")
			}
			for _, want := range testCase.wantNamed {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q does not name %q, so an operator cannot tell what to fix", err, want)
				}
			}
		})
	}
}
