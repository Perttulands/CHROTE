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
	cols := canonicalWindowDimension("CHROTE_TERMINAL_CREATION_COLS", defaultCanonicalWindowCols, minimumCanonicalWindowCols)
	rows := canonicalWindowDimension("CHROTE_TERMINAL_CREATION_ROWS", defaultCanonicalWindowRows, minimumCanonicalWindowRows)
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

// sizeCreatedSession gives a session the canonical size exactly once. One tmux
// command resizes the window and then unsets its window-size option, which
// resize-window had set to manual, so the window inherits latest again and the
// next real viewer still decides the size. A clientless window keeps the size
// it was given.
//
// This deliberately avoids a control-mode client (-C attach plus
// refresh-client -C). tmux 3.6a sets CLIENT_CONTROL before control_start runs,
// so a notification broadcast in that window (another session created, a
// rename, a detach) reaches control_write with a NULL control_state and the
// whole server segfaults. That took down a shared pool on 2026-11-06 and
// reproduces on a scratch socket. See ADR 0017, decision 3.
func (h *TmuxHandler) sizeCreatedSession(parent context.Context, socket, target string) error {
	cols, rows := canonicalWindowSize()
	if _, err := h.runTmuxOnSocketContext(parent, socket,
		"resize-window", "-t", target, "-x", strconv.Itoa(cols), "-y", strconv.Itoa(rows),
		";", "set-window-option", "-t", target, "-u", "window-size"); err != nil {
		return fmt.Errorf("size new tmux session %q to %dx%d: %w", target, cols, rows, err)
	}
	return nil
}
