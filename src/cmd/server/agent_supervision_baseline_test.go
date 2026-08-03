package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ADR-0014's central claim is a property of this process: it does not supervise
// agent lifetime. A locked agent belongs to its own systemd unit, so the server
// can restart, crash, or be upgraded without interrupting one.
//
// This is pinned two ways because either alone is weak. The source assertion
// catches a supervisor being wired back into startup; the symbol assertion
// catches the supervisor being resurrected under a new name in the api package.
// A grep-only test would pass the day someone renamed the function.

func TestServerStartsNoAgentSupervisionGoroutine(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	body := string(source)
	for _, forbidden := range []string{
		"StartPersistentAgentSupervisor",
		"ReconcilePersistentAgents",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("main.go calls %s; agent supervision belongs to systemd, not this process (ADR-0014)", forbidden)
		}
	}
}

func TestAPIPackageDefinesNoAgentReconcileLoop(t *testing.T) {
	root := filepath.Join("..", "..", "internal", "api")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read api package: %v", err)
	}
	// A supervisor is a ticker plus a reconcile function. Look for the shape,
	// not one name: `time.NewTicker` inside a file that also reconciles agents.
	reconcileName := regexp.MustCompile(`func \([^)]*\) (Reconcile|Supervise|StartPersistent)\w*Agent\w*\(`)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		body := string(raw)
		if match := reconcileName.FindString(body); match != "" {
			t.Fatalf("%s defines %q; the agent reconcile loop was deleted in ADR-0014 and must not return", entry.Name(), strings.TrimSpace(match))
		}
	}
}
