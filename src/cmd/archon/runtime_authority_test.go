package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchonRuntimeCommandsFailWithoutPrivateAuthorityFallback(t *testing.T) {
	workspace := t.TempDir()
	runner := &fakeTmux{live: map[string]bool{}}
	commands := [][]string{
		{"--workspace", workspace, "mission", "run", "missing", "--json"},
		{"--workspace", workspace, "formation", "run", "missing", "formation_missing", "--json"},
		{"--workspace", workspace, "run", "resume", "run_missing", "--json"},
		{"--workspace", workspace, "run", "abort", "run_missing", "--json"},
		{"--workspace", workspace, "gate", "approve", "run_missing", "gate_missing", "--json"},
	}
	for _, command := range commands {
		t.Run(strings.Join(command[2:4], "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(command, &stdout, &stderr, runner); code != 1 {
				t.Fatalf("exit = %d, want 1; stdout=%s stderr=%s", code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), `"code": "runtime_authority_non_authorizing"`) {
				t.Fatalf("stderr lacks typed runtime authority error: %s", stderr.String())
			}
			if strings.Contains(stderr.String(), workspace) || strings.Contains(stderr.String(), "wsa_") {
				t.Fatalf("stderr leaked private identity: %s", stderr.String())
			}
			if matches, err := filepath.Glob(filepath.Join(workspace, ".formations", "runs", "*")); err != nil || len(matches) != 0 {
				t.Fatalf("command left run artifacts: matches=%v err=%v", matches, err)
			}
		})
	}
	if len(runner.spawned) != 0 || len(runner.attach) != 0 {
		t.Fatalf("runtime rejection touched tmux: spawned=%v attach=%v", runner.spawned, runner.attach)
	}
}
