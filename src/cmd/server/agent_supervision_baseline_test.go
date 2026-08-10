package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Retiring Persistence v2 means removing the capability, not leaving dormant
// host-control pieces that can be accidentally re-enabled by configuration.
func TestPersistenceV2HostArtifactsAndInstallerSurfaceAbsent(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	for _, relative := range []string{
		"scripts/chrote-agent-ensure.sh",
		"scripts/chrote-agentctl",
		"services/chrote-agent@.service",
	} {
		path := filepath.Join(repoRoot, relative)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("retired Persistence v2 host artifact still exists: %s", relative)
		}
	}

	for _, relative := range []string{".env.example", "install.sh"} {
		path := filepath.Join(repoRoot, relative)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		for _, forbidden := range []string{
			"CHROTE_PERSISTENT_AGENTS_PATH",
			"chrote-agent-ensure",
			"chrote-agentctl",
			"chrote-agent@.service",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("%s still configures retired Persistence v2 surface %q", relative, forbidden)
			}
		}
	}
}

// Ordinary session creation can start a tmux server when its socket is absent.
// Persistence retirement must not change the established service cgroup
// boundary and begin killing those sessions on a CHROTE restart.
func TestPersistenceRetirementKeepsTmuxServiceLifecycleUnchanged(t *testing.T) {
	unitPath := filepath.Join("..", "..", "..", "services", "chrote-srv.service")
	raw, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read %s: %v", unitPath, err)
	}
	if !strings.Contains(string(raw), "KillMode=process") {
		t.Fatal("persistence retirement must not alter ordinary tmux restart survival")
	}
}
