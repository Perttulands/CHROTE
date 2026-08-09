package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimeRoutesStartAndStopSessionDropJanitor(t *testing.T) {
	dropsDir := filepath.Join(t.TempDir(), "session-drops")
	t.Setenv("CHROTE_SESSION_DROPS_DIR", dropsDir)
	t.Setenv("CHROTE_SESSION_DROPS_RETENTION", "1h")
	t.Setenv("CHROTE_SESSION_DROPS_MAINTENANCE_INTERVAL", "10ms")

	createExpiredDrop := func(id string) string {
		t.Helper()
		path := filepath.Join(dropsDir, id)
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatalf("create expired drop: %v", err)
		}
		if err := os.WriteFile(filepath.Join(path, "manifest.json"), []byte("{}"), 0o600); err != nil {
			t.Fatalf("write expired manifest: %v", err)
		}
		old := time.Now().Add(-24 * time.Hour)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatalf("age expired drop: %v", err)
		}
		return path
	}

	startupDrop := createExpiredDrop("20260701T000000Z-aaaaaaaaaaaaaaaaaaaaaaaa")
	ctx, cancel := context.WithCancel(context.Background())
	mux := http.NewServeMux()
	_, _, stopRuntimeMaintenance := registerRuntimeRoutes(mux, Config{
		TtydPort:                1,
		StartSessionDropJanitor: true,
	}, ctx)
	t.Cleanup(func() {
		cancel()
		stopRuntimeMaintenance()
	})
	if _, err := os.Stat(startupDrop); !os.IsNotExist(err) {
		t.Fatalf("runtime startup left expired drop: %v", err)
	}

	cancel()
	stopRuntimeMaintenance()
	stoppedDrop := createExpiredDrop("20260702T000000Z-bbbbbbbbbbbbbbbbbbbbbbbb")
	time.Sleep(40 * time.Millisecond)
	if _, err := os.Stat(stoppedDrop); err != nil {
		t.Fatalf("janitor continued after runtime stop: %v", err)
	}
}
