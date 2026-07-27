package api

import (
	"os"
	"strings"
	"testing"
)

// The input-fence prototype fixture leaked six tmux servers onto this host, still resident five
// days later. Its cleanup is correct and does retire them — when it runs. t.Cleanup does not run
// if the test binary is interrupted, and the pane command never exits on its own, so an aborted
// run leaves immortal processes. These tests pin the two properties that survive that case.
// Unlike the prototype itself they are not gated behind CHROTE_INPUT_FENCE_PROTOTYPE: they spawn
// nothing, so they run everywhere and catch a regression before someone enables the prototype.

// The longest-lived orphan came from `timeout --signal=TERM --kill-after=1s 0 ... hanging-tmux`.
// `timeout 0` means "no timeout", so the backstop meant to bound a process made it immortal.
func TestPrototypeFixturePaneLifetimeIsBounded(t *testing.T) {
	if prototypeFixturePaneLifetimeSeconds <= 0 {
		t.Fatalf(
			"prototypeFixturePaneLifetimeSeconds = %d; a non-positive timeout means NO timeout, "+
				"which is the exact footgun that produced an eight-day-old orphan on this host",
			prototypeFixturePaneLifetimeSeconds,
		)
	}
	// Long enough that it cannot influence what the prototype measures, short enough that a
	// leaked pane is a nuisance rather than a resident process.
	if prototypeFixturePaneLifetimeSeconds > 3600 {
		t.Fatalf("prototypeFixturePaneLifetimeSeconds = %d; too long to be a useful backstop", prototypeFixturePaneLifetimeSeconds)
	}
}

// The fixture must not depend on cleanup running at all, because an interrupted test binary
// never runs it. That leaves the pane command itself as the only thing that can bound the pane.
func TestPrototypeFixturePaneCommandCarriesItsOwnTimeout(t *testing.T) {
	source, err := os.ReadFile("tmux_input_fence_prototype_support_test.go")
	if err != nil {
		t.Fatalf("read prototype fixture source: %v", err)
	}
	text := string(source)

	start := strings.Index(text, `"new-session", "-d", "-s", sessionName`)
	if start < 0 {
		t.Fatal("prototype fixture no longer starts sessions with the expected new-session shape; re-check the leak guard")
	}
	window := text[start:min(start+400, len(text))]
	if !strings.Contains(window, `"timeout"`) {
		t.Error("fixture panes are started without a timeout backstop; an interrupted run would orphan them forever")
	}
	if !strings.Contains(window, "prototypeFixturePaneLifetimeSeconds") {
		t.Error("fixture pane timeout is not driven by prototypeFixturePaneLifetimeSeconds, so the bounds test above guards nothing")
	}
}

// Deleting the socket directory while a server still runs is what made the orphans unreachable:
// the process survives, its socket path does not, and nothing that resolves by path can reap it.
func TestPrototypeFixtureProvesServersDiedBeforeDeletingSockets(t *testing.T) {
	source, err := os.ReadFile("tmux_input_fence_prototype_support_test.go")
	if err != nil {
		t.Fatalf("read prototype fixture source: %v", err)
	}
	text := string(source)

	survivorProbe := strings.Index(text, `"list-sessions"`)
	removeRoot := strings.Index(text, "os.RemoveAll(f.root)")
	if survivorProbe < 0 {
		t.Fatal("cleanup no longer probes for surviving servers before removing the socket directory")
	}
	if removeRoot < 0 {
		t.Fatal("cleanup no longer removes the disposable root; re-check this guard")
	}
	if survivorProbe > removeRoot {
		t.Error("cleanup removes the socket directory before proving the servers died, which is how orphans become unreachable")
	}
}
