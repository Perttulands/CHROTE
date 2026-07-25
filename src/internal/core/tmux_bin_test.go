package core

import "testing"

func TestTmuxBin_PrefersPinnedBinary(t *testing.T) {
	t.Setenv("CHROTE_TMUX_BIN", " /home/linuxbrew/.linuxbrew/bin/tmux ")
	if got := TmuxBin(); got != "/home/linuxbrew/.linuxbrew/bin/tmux" {
		t.Fatalf("TmuxBin() = %q, want the pinned CHROTE_TMUX_BIN path", got)
	}
}

func TestTmuxBin_FallsBackToPathLookup(t *testing.T) {
	t.Setenv("CHROTE_TMUX_BIN", "")
	if got := TmuxBin(); got != "tmux" {
		t.Fatalf("TmuxBin() = %q, want %q when CHROTE_TMUX_BIN is unset", got, "tmux")
	}
}
