package proxy

import (
	"os"
	"os/exec"
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
	if !strings.Contains(content, `attach_explicit_socket "$CHROTE_TMUX_SOCKET"`) && !strings.Contains(content, `attach_explicit_socket "${CHROTE_TMUX_SOCKET}"`) {
		t.Fatalf("launch script %s does not route CHROTE_TMUX_SOCKET through explicit socket attach", scriptPath)
	}
}

func TestLaunchScript_TrimsConfiguredTerminalUserCSVs(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	scriptPath := filepath.Join(repoRoot, "terminal-launch.sh")
	tmpDir := t.TempDir()
	argsPath := filepath.Join(tmpDir, "tmux.args")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	fakeScript := `#!/bin/sh
printf '%s\n' "$*" >> "$TMUX_ARGS"
exit 0
`
	if err := os.WriteFile(fakeTmux, []byte(fakeScript), 0755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}

	cmd := exec.Command("bash", scriptPath, "shell-one", "bob")
	cmd.Env = append(os.Environ(),
		"PATH="+tmpDir+":"+os.Getenv("PATH"),
		"TMUX_ARGS="+argsPath,
		"HOME="+tmpDir,
		"CHROTE_WORKDIR="+tmpDir,
		"CHROTE_TERMINAL_USERS= alice, bob ,",
		"CHROTE_TERMINAL_USER_SOCKETS= alice=/tmp/tmux-a, bob = /tmp/tmux-b ",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("launch script failed: %v\n%s", err, output)
	}

	raw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read fake tmux args: %v", err)
	}
	args := string(raw)
	if !strings.Contains(args, "-S /tmp/tmux-b has-session -t shell-one") {
		t.Fatalf("fake tmux args %q do not show trimmed socket has-session", args)
	}
	if !strings.Contains(args, "-S /tmp/tmux-b attach-session -t shell-one") {
		t.Fatalf("fake tmux args %q do not show trimmed socket attach", args)
	}
}
