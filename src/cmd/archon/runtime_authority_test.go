package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrote/server/internal/formations"
)

func TestArchonNewRunStartDefinitionErrorsPrecedeUnavailableAuthority(t *testing.T) {
	tests := []struct {
		name     string
		slug     string
		board    string
		args     []string
		wantCode string
	}{
		{
			name:     "mission missing board",
			args:     []string{"mission", "run", "missing", "--mission", "mis_missing", "--json"},
			wantCode: "not_found",
		},
		{
			name:     "formation missing board",
			args:     []string{"formation", "run", "missing", "fmn_missing", "--json"},
			wantCode: "not_found",
		},
		{
			name:     "mission missing root",
			slug:     "session-search",
			board:    archonS4BoardFixture(),
			args:     []string{"mission", "run", "session-search", "--mission", "mis_missing", "--json"},
			wantCode: "not_found",
		},
		{
			name:     "formation missing root",
			slug:     "session-search",
			board:    archonS4BoardFixture(),
			args:     []string{"formation", "run", "session-search", "fmn_missing", "--json"},
			wantCode: "not_found",
		},
		{
			name:     "mission reachable legacy script gate",
			slug:     "session-search",
			board:    archonLegacyScriptGateBoardFixture(),
			args:     []string{"mission", "run", "session-search", "--json"},
			wantCode: formations.LegacyScriptGateMigrationCode,
		},
		{
			name:     "mission legacy inline verification",
			slug:     "session-search",
			board:    archonRuntimeAuthorityLegacyInlineVerificationFixture(),
			args:     []string{"mission", "run", "session-search", "--json"},
			wantCode: formations.LegacyInlineVerificationMigrationCode,
		},
		{
			name:     "formation legacy inline verification",
			slug:     "session-search",
			board:    archonRuntimeAuthorityLegacyInlineVerificationFixture(),
			args:     []string{"formation", "run", "session-search", "fmn_work", "--json"},
			wantCode: formations.LegacyInlineVerificationMigrationCode,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			privateRoot := filepath.Join(t.TempDir(), "wsa_private_authority")
			t.Setenv("CHROTE_FORMATIONS_DATA_ROOT", privateRoot)
			if test.board != "" {
				writeArchonFile(t, formations.NewStore(workspace).BoardPath(test.slug), test.board)
			}
			tmuxCapture := installArchonRuntimeAuthorityTmuxTripwire(t, workspace)
			runner := &fakeTmux{live: map[string]bool{}}
			command := append([]string{"--workspace", workspace}, test.args...)
			var stdout, stderr bytes.Buffer
			if code := run(command, &stdout, &stderr, runner); code != 1 {
				t.Errorf("exit = %d, want 1; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want structured error on stderr only", stdout.String())
			}
			body := stderr.String()
			if !strings.Contains(body, `"code": "`+test.wantCode+`"`) {
				t.Errorf("stderr lacks selected-definition code %q: %s", test.wantCode, body)
			}
			if strings.Contains(body, `"code": "runtime_authority_non_authorizing"`) {
				t.Errorf("runtime authority masked selected-definition error: %s", body)
			}
			assertArchonRuntimeAuthorityResponseIsPrivate(t, body, workspace, privateRoot)
			assertNoArchonRuntimeAuthorityEffects(t, workspace, tmuxCapture, runner)
		})
	}
}

func TestArchonResumeAbortAndVerdictRemainAuthorityFirst(t *testing.T) {
	workspace := t.TempDir()
	privateRoot := filepath.Join(t.TempDir(), "wsa_private_authority")
	t.Setenv("CHROTE_FORMATIONS_DATA_ROOT", privateRoot)
	tmuxCapture := installArchonRuntimeAuthorityTmuxTripwire(t, workspace)
	runner := &fakeTmux{live: map[string]bool{}}
	commands := []struct {
		name string
		args []string
	}{
		{"resume", []string{"run", "resume", "run_missing", "--json"}},
		{"abort", []string{"run", "abort", "run_missing", "--json"}},
		{"verdict", []string{"gate", "approve", "run_missing", "gate_missing", "--json"}},
	}
	for _, command := range commands {
		t.Run(command.name, func(t *testing.T) {
			args := append([]string{"--workspace", workspace}, command.args...)
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr, runner); code != 1 {
				t.Fatalf("exit = %d, want 1; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want structured error on stderr only", stdout.String())
			}
			body := stderr.String()
			if !strings.Contains(body, `"code": "runtime_authority_non_authorizing"`) {
				t.Fatalf("stderr lacks typed runtime authority error: %s", body)
			}
			assertArchonRuntimeAuthorityResponseIsPrivate(t, body, workspace, privateRoot)
			assertNoArchonRuntimeAuthorityEffects(t, workspace, tmuxCapture, runner)
		})
	}
}

func archonRuntimeAuthorityLegacyInlineVerificationFixture() string {
	return strings.Replace(archonS4BoardFixture(), `[[formation.input]]`, `[formation.verification]
id = "ver_work"
kinds = ["code"]
criterion = "Tests pass"
onFail = "block"

[[formation.input]]`, 1)
}

func installArchonRuntimeAuthorityTmuxTripwire(t *testing.T, workspace string) string {
	t.Helper()
	binDir := t.TempDir()
	capturePath := filepath.Join(t.TempDir(), "tmux-called")
	fakeTmux := filepath.Join(binDir, "tmux")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$ARCHON_RUNTIME_AUTHORITY_TMUX_CAPTURE\"\nexit 99\n"
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatalf("write tmux tripwire: %v", err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("ARCHON_RUNTIME_AUTHORITY_TMUX_CAPTURE", capturePath)
	t.Setenv("CHROTE_FORMATIONS_LAB_HARNESSES", "")
	t.Setenv("CHROTE_FORMATIONS_TMUX_HARNESSES", "openai-codex")
	t.Setenv("CHROTE_FORMATIONS_TMUX_SOCKET", filepath.Join(t.TempDir(), "default"))
	t.Setenv("CHROTE_FORMATIONS_TMUX_CWD", workspace)
	t.Setenv("CHROTE_FORMATIONS_TMUX_ROOTS", workspace)
	return capturePath
}

func assertArchonRuntimeAuthorityResponseIsPrivate(t *testing.T, body, workspace, privateRoot string) {
	t.Helper()
	if strings.Contains(body, workspace) || strings.Contains(body, privateRoot) || strings.Contains(body, "wsa_") {
		t.Fatalf("stderr leaked private authority identity: %s", body)
	}
}

func assertNoArchonRuntimeAuthorityEffects(t *testing.T, workspace, tmuxCapture string, runner *fakeTmux) {
	t.Helper()
	if matches, err := filepath.Glob(filepath.Join(workspace, ".formations", "runs", "*")); err != nil || len(matches) != 0 {
		t.Fatalf("command left run artifacts: matches=%v err=%v", matches, err)
	}
	if len(runner.spawned) != 0 || len(runner.attach) != 0 {
		t.Fatalf("runtime rejection touched CLI tmux runner: spawned=%v attach=%v", runner.spawned, runner.attach)
	}
	if raw, err := os.ReadFile(tmuxCapture); err == nil {
		t.Fatalf("runtime rejection reached formations tmux: %s", raw)
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect tmux tripwire: %v", err)
	}
}
