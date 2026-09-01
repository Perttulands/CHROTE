package api

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// A session nobody is looking at still has a width, and whatever is running in
// it writes to that width. Left alone, tmux gives it 80x24, so an agent in a
// never-viewed session wraps its output at 80 columns and that is what gets
// relayed onward.
//
// This is the canonical size CHROTE gives such a session. It is applied once,
// when CHROTE creates the session, and nothing revisits it afterwards: while a
// tile is attached the viewer's size wins by definition, and after that the
// last viewer's size is the size. See docs/adr/0017-terminal-viewing-model.md.
const (
	defaultCanonicalWindowCols = 200
	defaultCanonicalWindowRows = 50
	// A canonical size below tmux's own default would defeat the point, so a
	// smaller configured value is ignored rather than honoured.
	minimumCanonicalWindowCols = 80
	minimumCanonicalWindowRows = 24
)

// canonicalWindowSize is the one place the canonical size is decided.
func canonicalWindowSize() (int, int) {
	cols := canonicalWindowDimension("CHROTE_TERMINAL_UNOBSERVED_COLS", defaultCanonicalWindowCols, minimumCanonicalWindowCols)
	rows := canonicalWindowDimension("CHROTE_TERMINAL_UNOBSERVED_ROWS", defaultCanonicalWindowRows, minimumCanonicalWindowRows)
	return cols, rows
}

func canonicalWindowDimension(envName string, fallback, minimum int) int {
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < minimum {
		return fallback
	}
	return parsed
}

// sizeCreatedSession gives a session the canonical size exactly once, through a
// control-mode client that attaches, sets the size and leaves. tmux sizes a
// clientless window to the arriving client and keeps that size after it
// detaches, and unlike resize-window this leaves window-size on latest, so the
// next real viewer still decides the size. The client needs no pty.
func (h *TmuxHandler) sizeCreatedSession(parent context.Context, socket, target string) error {
	cols, rows := canonicalWindowSize()
	input := fmt.Sprintf("refresh-client -C %d,%d\n", cols, rows)
	if _, err := h.runTmuxOnSocketInput(parent, socket, input, "-C", "attach-session", "-t", target); err != nil {
		return fmt.Errorf("size new tmux session %q to %dx%d: %w", target, cols, rows, err)
	}
	return nil
}
