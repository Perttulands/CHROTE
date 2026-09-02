package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// inventoryLine builds one line in the shape sessionInventoryFormat produces.
func inventoryLine(fields ...string) string {
	if len(fields) != sessionInventoryFieldCount {
		panic("inventory line needs every field")
	}
	return strings.Join(fields, "\t") + "\n"
}

func TestParseSessionsOutputReportsFactsThatContradictAppearances(t *testing.T) {
	output := inventoryLine("$1", "pinned", "1", "1", "/home/operator", "bash", "3", "100", "30", "manual", "0", "/dev/pts/9,/dev/pts/12")

	sessions := parseSessionsOutput(output, "operator", map[string]bool{"/dev/pts/9": true})
	if len(sessions) != 1 {
		t.Fatalf("sessions = %+v, want one", sessions)
	}
	session := sessions[0]
	if !session.SizePinned {
		t.Fatalf("SizePinned = false, want true for window-size manual")
	}
	if session.Width != 100 || session.Height != 30 {
		t.Fatalf("size = %dx%d, want 100x30", session.Width, session.Height)
	}
	if session.Panes != 3 {
		t.Fatalf("Panes = %d, want 3", session.Panes)
	}
	if session.MouseEnabled == nil || *session.MouseEnabled {
		t.Fatalf("MouseEnabled = %v, want an explicit false", session.MouseEnabled)
	}
	if got := strings.Join(session.ForeignClients, ","); got != "/dev/pts/12" {
		t.Fatalf("ForeignClients = %q, want only the client CHROTE did not create", got)
	}
	// Viewers counts everyone watching, whoever created them: tmux draws the
	// window once, so a second viewer means this pane is somebody else's size.
	if session.Viewers != 2 {
		t.Fatalf("Viewers = %d, want both attached clients counted", session.Viewers)
	}
}

func TestParseSessionsOutputCountsEveryViewerIncludingCHROTEsOwn(t *testing.T) {
	for name, tt := range map[string]struct {
		attachedList string
		want         int
	}{
		"nobody watching":        {attachedList: "", want: 0},
		"one viewer":             {attachedList: "/dev/pts/9", want: 1},
		"a viewer and a watcher": {attachedList: "/dev/pts/9,/dev/pts/12", want: 2},
	} {
		t.Run(name, func(t *testing.T) {
			output := inventoryLine("$1", "shared", "1", "1", "/home/operator", "bash", "1", "120", "40", "latest", "1", tt.attachedList)

			sessions := parseSessionsOutput(output, "operator", map[string]bool{"/dev/pts/9": true})

			if sessions[0].Viewers != tt.want {
				t.Fatalf("Viewers = %d, want %d", sessions[0].Viewers, tt.want)
			}
		})
	}
}

func TestParseSessionsOutputRaisesNoClaimForAnOrdinarySession(t *testing.T) {
	output := inventoryLine("$1", "ordinary", "1", "1", "/home/operator", "bash", "1", "120", "40", "latest", "1", "/dev/pts/9")

	sessions := parseSessionsOutput(output, "operator", map[string]bool{"/dev/pts/9": true})
	if len(sessions) != 1 {
		t.Fatalf("sessions = %+v, want one", sessions)
	}
	session := sessions[0]
	if session.SizePinned {
		t.Fatalf("SizePinned = true, want false for window-size latest")
	}
	if session.MouseEnabled == nil || !*session.MouseEnabled {
		t.Fatalf("MouseEnabled = %v, want an explicit true", session.MouseEnabled)
	}
	if len(session.ForeignClients) != 0 {
		t.Fatalf("ForeignClients = %#v, want none for a client CHROTE spawned", session.ForeignClients)
	}
	if session.Panes != 1 {
		t.Fatalf("Panes = %d, want 1", session.Panes)
	}
}

func TestParseSessionsOutputTreatsAnUnattributableClientAsSilent(t *testing.T) {
	// A control-mode client has no tty, so nothing can be said about it.
	output := inventoryLine("$1", "sizing", "1", "1", "/home/operator", "bash", "1", "120", "40", "latest", "1", "")

	sessions := parseSessionsOutput(output, "operator", map[string]bool{})
	if len(sessions[0].ForeignClients) != 0 {
		t.Fatalf("ForeignClients = %#v, want none when no client reports a tty", sessions[0].ForeignClients)
	}
}

func TestParseSessionsOutputKeepsOlderShapesClaimFree(t *testing.T) {
	for name, line := range map[string]string{
		"through the foreground command": "$1\tlegacy\t2\t1\t/home/operator\tbash\n",
		"through the working directory":  "$1\tlegacy\t2\t1\t/home/operator\n",
	} {
		t.Run(name, func(t *testing.T) {
			sessions := parseSessionsOutput(line, "operator", map[string]bool{})
			if len(sessions) != 1 {
				t.Fatalf("sessions = %+v, want one", sessions)
			}
			session := sessions[0]
			if session.Name != "legacy" || session.Windows != 2 || !session.Attached {
				t.Fatalf("session = %+v, want the older fields still read", session)
			}
			if session.SizePinned || session.MouseEnabled != nil || session.Panes != 0 || len(session.ForeignClients) != 0 {
				t.Fatalf("session = %+v, want no badge claim from an older shape", session)
			}
		})
	}
}

func TestListSessionsRunsOneTmuxCommandPerSocket(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> %s
printf '$1\tone\t1\t0\t/home/operator\tbash\t1\t120\t40\tlatest\t1\t\n'
`, log)
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a,bob=/tmp/tmux-b")

	handler := NewTmuxHandler()
	recorder := httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	calls := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(calls) != 2 {
		t.Fatalf("tmux calls = %#v, want exactly one per configured socket", calls)
	}
	for _, call := range calls {
		if !strings.Contains(call, "list-sessions") {
			t.Fatalf("tmux call %q, want the single inventory command", call)
		}
	}
}

func TestSessionInventoryFormatCarriesEveryBadgeFact(t *testing.T) {
	for _, variable := range []string{
		"#{window_panes}",
		"#{window_width}",
		"#{window_height}",
		"#{window-size}",
		"#{mouse}",
		"#{session_attached_list}",
	} {
		if !strings.Contains(sessionInventoryFormat, variable) {
			t.Fatalf("inventory format is missing %s, so that fact would need a second tmux call", variable)
		}
	}
	if got := strings.Count(sessionInventoryFormat, "\t") + 1; got != sessionInventoryFieldCount {
		t.Fatalf("inventory format has %d fields, want %d", got, sessionInventoryFieldCount)
	}
}

func TestPtyDevicePathTranslatesOnlyPtySlaves(t *testing.T) {
	// 34901 is the tty_nr a real pts/85 client reports: major 136, minor 85.
	if got := ptyDevicePath(34901); got != "/dev/pts/85" {
		t.Fatalf("ptyDevicePath(34901) = %q, want /dev/pts/85", got)
	}
	// Minors above 255 spill past the major field into the top of tty_nr.
	if got := ptyDevicePath((136 << 8) | 0x100000 | 0x2a); got != "/dev/pts/298" {
		t.Fatalf("high-minor decode = %q, want /dev/pts/298", got)
	}
	// Major 4 is a virtual console, never something CHROTE opened.
	if got := ptyDevicePath((4 << 8) | 1); got != "" {
		t.Fatalf("console decode = %q, want no pty", got)
	}
}

// writeFakeProcess lays out the /proc entries ownedPTYs reads: a stat line
// carrying tty_nr, and one children file per thread.
func writeFakeProcess(t *testing.T, root string, pid, ttyNr int, threads map[int][]int) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(pid))
	stat := fmt.Sprintf("%d (some (odd) name) S 1 %d %d %d -1 0 0\n", pid, pid, pid, ttyNr)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("make process dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0o644); err != nil {
		t.Fatalf("write stat: %v", err)
	}
	for tid, children := range threads {
		taskDir := filepath.Join(dir, "task", strconv.Itoa(tid))
		if err := os.MkdirAll(taskDir, 0o755); err != nil {
			t.Fatalf("make task dir: %v", err)
		}
		fields := make([]string, 0, len(children))
		for _, child := range children {
			fields = append(fields, strconv.Itoa(child))
		}
		body := strings.Join(fields, " ")
		if body != "" {
			body += " "
		}
		if err := os.WriteFile(filepath.Join(taskDir, "children"), []byte(body), 0o644); err != nil {
			t.Fatalf("write children: %v", err)
		}
	}
}

func TestOwnedPTYsFollowsDescendantsAcrossEveryThread(t *testing.T) {
	root := t.TempDir()
	// The server forked its transport from a non-main thread, which is what a
	// Go parent looks like in /proc and what an only-main-thread walk misses.
	writeFakeProcess(t, root, 100, 0, map[int][]int{100: nil, 104: {200}})
	writeFakeProcess(t, root, 200, 0, map[int][]int{200: {300, 301}})
	writeFakeProcess(t, root, 300, (136<<8)|8, map[int][]int{300: nil})
	writeFakeProcess(t, root, 301, (136<<8)|11, map[int][]int{301: nil})
	// A process outside the server's tree holds a pty of its own.
	writeFakeProcess(t, root, 900, (136<<8)|64, map[int][]int{900: nil})

	owned := procSource{root: root, pid: 100}.ownedPTYs()
	if !owned["/dev/pts/8"] || !owned["/dev/pts/11"] {
		t.Fatalf("owned = %#v, want both spawned ptys", owned)
	}
	if owned["/dev/pts/64"] {
		t.Fatalf("owned = %#v, want nothing outside the server's process tree", owned)
	}
	if len(owned) != 2 {
		t.Fatalf("owned = %#v, want exactly the two spawned ptys", owned)
	}
}

func TestOwnedPTYsOwnsNothingWhenProcIsUnreadable(t *testing.T) {
	if owned := (procSource{}).ownedPTYs(); len(owned) != 0 {
		t.Fatalf("owned = %#v, want an empty set rather than a claim of ownership", owned)
	}
	if owned := (procSource{root: filepath.Join(t.TempDir(), "absent"), pid: 1}).ownedPTYs(); len(owned) != 0 {
		t.Fatalf("owned = %#v, want an empty set for an absent proc tree", owned)
	}
}

func TestOwnedPTYsSurvivesACycleInTheProcessTree(t *testing.T) {
	root := t.TempDir()
	writeFakeProcess(t, root, 100, (136<<8)|1, map[int][]int{100: {200}})
	writeFakeProcess(t, root, 200, (136<<8)|2, map[int][]int{200: {100}})

	owned := procSource{root: root, pid: 100}.ownedPTYs()
	if len(owned) != 2 {
		t.Fatalf("owned = %#v, want both ptys and no repeat visit", owned)
	}
}
