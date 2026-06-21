package proxy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewTerminalProxy_DefaultProfileUsesConfiguredSocket(t *testing.T) {
	t.Setenv("CHROTE_DEFAULT_TMUX_SOCKET", "/tmp/tmux-1001/default")

	proxy := NewTerminalProxy(7683)
	env := proxy.launchEnv()

	want := "CHROTE_TMUX_SOCKET=/tmp/tmux-1001/default"
	for _, entry := range env {
		if entry == want {
			return
		}
	}
	t.Fatalf("default terminal launch env does not contain %q; env=%v", want, env)
}

func TestLaunchScript_AttachesWithExplicitSocketWhenSet(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	scriptPath := filepath.Join(repoRoot, "terminal-launch.sh")

	raw, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read launch script %s: %v", scriptPath, err)
	}
	content := string(raw)

	if !strings.Contains(content, "CHROTE_TMUX_SOCKET") {
		t.Fatalf("launch script %s does not reference CHROTE_TMUX_SOCKET", scriptPath)
	}
	if !strings.Contains(content, `-S "$CHROTE_TMUX_SOCKET"`) && !strings.Contains(content, `-S "${CHROTE_TMUX_SOCKET}"`) {
		t.Fatalf("launch script %s does not pass -S \"$CHROTE_TMUX_SOCKET\" to tmux", scriptPath)
	}
}
