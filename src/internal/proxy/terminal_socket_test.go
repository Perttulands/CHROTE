package proxy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewTerminalProxy_PreservesConfiguredSocketMappings(t *testing.T) {
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-2001/default,build=/tmp/tmux-2002/default")

	proxy := NewTerminalProxy(7683)
	env := proxy.launchEnv()

	want := "CHROTE_TMUX_SOCKET=alice=/tmp/tmux-2001/default,build=/tmp/tmux-2002/default"
	for _, entry := range env {
		if entry == want {
			return
		}
	}
	t.Fatalf("default terminal launch env does not contain %q; env=%v", want, env)
}

func TestLaunchScript_SoleMappingSupportsLegacyAttachWithoutUnixUser(t *testing.T) {
	scriptPath := launchScriptPath(t)
	tmpDir := t.TempDir()
	argsPath := filepath.Join(tmpDir, "tmux.args")
	writeFakeTmux(t, filepath.Join(tmpDir, "tmux"))

	cmd := exec.Command("bash", scriptPath, "tile", "shell-one")
	cmd.Env = append(os.Environ(),
		"PATH="+tmpDir+":"+os.Getenv("PATH"),
		"TMUX_ARGS="+argsPath,
		"HOME="+tmpDir,
		"CHROTE_WORKDIR="+tmpDir,
		"CHROTE_TMUX_SOCKET=alice=/tmp/tmux-a",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("launch script failed: %v\n%s", err, output)
	}
	if raw, err := os.ReadFile(argsPath); err != nil || !strings.Contains(string(raw), "-S /tmp/tmux-a attach-session -d -t shell-one") {
		t.Fatalf("legacy attach did not resolve the sole configured mapping: args=%q err=%v", raw, err)
	}
}

// The launch script used to take the FIRST matching entry while the Go API takes
// the LAST, so a duplicated user key made listing and attaching resolve
// different tmux servers. Both parsers must now refuse the duplicate.
func TestLaunchScript_RejectsDuplicateUserSocketKey(t *testing.T) {
	scriptPath := launchScriptPath(t)
	tmpDir := t.TempDir()
	argsPath := filepath.Join(tmpDir, "tmux.args")
	writeFakeTmux(t, filepath.Join(tmpDir, "tmux"))

	cmd := exec.Command("bash", scriptPath, "tile", "shell-one", "bob")
	cmd.Env = append(os.Environ(),
		"PATH="+tmpDir+":"+os.Getenv("PATH"),
		"TMUX_ARGS="+argsPath,
		"HOME="+tmpDir,
		"CHROTE_WORKDIR="+tmpDir,
		"CHROTE_TMUX_SOCKET=alice=/tmp/tmux-a,bob=/tmp/tmux-b,bob=/tmp/tmux-b2",
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("launch script accepted a duplicate socket entry for bob and attached anyway; output=%s", output)
	}
	combined := string(output)
	for _, want := range []string{"duplicate", "bob", "/tmp/tmux-b", "/tmp/tmux-b2"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("launch script output %q does not name %q", combined, want)
		}
	}
	if raw, readErr := os.ReadFile(argsPath); readErr == nil && strings.TrimSpace(string(raw)) != "" {
		t.Fatalf("launch script invoked tmux despite the duplicate key; args=%q", string(raw))
	}
}

// A tmux 3.4 client cannot talk to a 3.6a server at all, so the attach path must
// use the pinned CHROTE_TMUX_BIN rather than whatever "tmux" PATH resolves to.
func TestLaunchScript_UsesPinnedTmuxBin(t *testing.T) {
	scriptPath := launchScriptPath(t)
	tmpDir := t.TempDir()
	pathDir := filepath.Join(tmpDir, "pathbin")
	if err := os.MkdirAll(pathDir, 0755); err != nil {
		t.Fatalf("mkdir pathbin: %v", err)
	}
	pinnedArgs := filepath.Join(tmpDir, "pinned.args")
	pathArgs := filepath.Join(tmpDir, "path.args")
	pinnedTmux := filepath.Join(tmpDir, "tmux-pinned")
	writeFakeTmuxRecording(t, pinnedTmux, pinnedArgs)
	writeFakeTmuxRecording(t, filepath.Join(pathDir, "tmux"), pathArgs)

	cmd := exec.Command("bash", scriptPath, "tile", "shell-one", "bob")
	cmd.Env = append(os.Environ(),
		"PATH="+pathDir+":"+os.Getenv("PATH"),
		"HOME="+tmpDir,
		"CHROTE_WORKDIR="+tmpDir,
		"CHROTE_TMUX_SOCKET=bob=/tmp/tmux-b",
		"CHROTE_TMUX_BIN="+pinnedTmux,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("launch script failed: %v\n%s", err, output)
	}

	pinnedRaw, err := os.ReadFile(pinnedArgs)
	if err != nil {
		t.Fatalf("pinned tmux was never invoked (CHROTE_TMUX_BIN ignored): %v", err)
	}
	if !strings.Contains(string(pinnedRaw), "-S /tmp/tmux-b attach-session -d -t shell-one") {
		t.Fatalf("pinned tmux args %q do not show the attach", string(pinnedRaw))
	}
	if raw, readErr := os.ReadFile(pathArgs); readErr == nil && strings.TrimSpace(string(raw)) != "" {
		t.Fatalf("launch script used the PATH tmux instead of CHROTE_TMUX_BIN; args=%q", string(raw))
	}
}

// One sizing client per window is what the flags buy: a tile takes the session
// over with -d, and a peek attaches without ever sizing the window.
func TestLaunchScript_ViewingModeSelectsTheAttachFlags(t *testing.T) {
	for _, tt := range []struct {
		mode string
		want string
	}{
		{mode: "tile", want: "-S /tmp/tmux-b attach-session -d -t shell-one"},
		{mode: "peek", want: "-S /tmp/tmux-b attach-session -f ignore-size -t shell-one"},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			scriptPath := launchScriptPath(t)
			tmpDir := t.TempDir()
			argsPath := filepath.Join(tmpDir, "tmux.args")
			writeFakeTmux(t, filepath.Join(tmpDir, "tmux"))

			cmd := exec.Command("bash", scriptPath, tt.mode, "shell-one", "bob")
			cmd.Env = append(os.Environ(),
				"PATH="+tmpDir+":"+os.Getenv("PATH"),
				"TMUX_ARGS="+argsPath,
				"HOME="+tmpDir,
				"CHROTE_WORKDIR="+tmpDir,
				"CHROTE_TMUX_SOCKET=bob=/tmp/tmux-b",
			)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("launch script failed: %v\n%s", err, output)
			}
			raw, err := os.ReadFile(argsPath)
			if err != nil {
				t.Fatalf("read fake tmux args: %v", err)
			}
			if !strings.Contains(string(raw), tt.want) {
				t.Fatalf("%s attach args %q do not contain %q", tt.mode, string(raw), tt.want)
			}
			if strings.Contains(string(raw), "resize-window") {
				t.Fatalf("attach used resize-window, which pins window-size manual; args=%q", string(raw))
			}
		})
	}
}

// A caller that names no mode must not be attached under a guessed one.
func TestLaunchScript_RejectsAnUnknownViewingMode(t *testing.T) {
	scriptPath := launchScriptPath(t)
	tmpDir := t.TempDir()
	argsPath := filepath.Join(tmpDir, "tmux.args")
	writeFakeTmux(t, filepath.Join(tmpDir, "tmux"))

	cmd := exec.Command("bash", scriptPath, "shell-one", "bob")
	cmd.Env = append(os.Environ(),
		"PATH="+tmpDir+":"+os.Getenv("PATH"),
		"TMUX_ARGS="+argsPath,
		"HOME="+tmpDir,
		"CHROTE_WORKDIR="+tmpDir,
		"CHROTE_TMUX_SOCKET=bob=/tmp/tmux-b",
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("launch script attached without a viewing mode; output=%s", output)
	}
	if !strings.Contains(string(output), "viewing mode") {
		t.Fatalf("launch script output %q does not explain the missing viewing mode", string(output))
	}
	if raw, readErr := os.ReadFile(argsPath); readErr == nil && strings.TrimSpace(string(raw)) != "" {
		t.Fatalf("launch script invoked tmux despite the unknown mode; args=%q", string(raw))
	}
}

func launchScriptPath(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(filepath.Clean(filepath.Join(wd, "..", "..", "..")), "terminal-launch.sh")
}

func writeFakeTmux(t *testing.T, path string) {
	t.Helper()
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$TMUX_ARGS"
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write fake tmux %s: %v", path, err)
	}
}

func writeFakeTmuxRecording(t *testing.T, path, argsPath string) {
	t.Helper()
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + argsPath + "\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("write fake tmux %s: %v", path, err)
	}
}

func TestLaunchScript_TrimsConfiguredSocketCSV(t *testing.T) {
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

	cmd := exec.Command("bash", scriptPath, "tile", "shell-one", "bob")
	cmd.Env = append(os.Environ(),
		"PATH="+tmpDir+":"+os.Getenv("PATH"),
		"TMUX_ARGS="+argsPath,
		"HOME="+tmpDir,
		"CHROTE_WORKDIR="+tmpDir,
		"CHROTE_TMUX_SOCKET= alice=/tmp/tmux-a, bob = /tmp/tmux-b ",
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
	if !strings.Contains(args, "-S /tmp/tmux-b attach-session -d -t shell-one") {
		t.Fatalf("fake tmux args %q do not show trimmed socket attach", args)
	}
	if strings.Contains(args, "set-option") || strings.Contains(args, " mouse on") {
		t.Fatalf("launch script should not force mouse mode and override dashboard setting; args=%q", args)
	}
}
