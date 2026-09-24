package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Each invocation holds both output pipes open in a real child. Recording both
// PIDs also lets the failing, pre-fix test clean up without touching any tmux.
func beadsProcessFixture(t *testing.T) (*BeadsHandler, string, string) {
	t.Helper()
	dir := t.TempDir()
	project := filepath.Join(dir, "project")
	makeValidBeadsWorkspace(t, project)
	t.Setenv("CHROTE_ROOTS", dir)
	t.Setenv("CHROTE_BD_TEST_DIR", dir)
	command := filepath.Join(dir, "bd")
	script := `#!/bin/sh
sleep 30 &
child=$!
printf '%s %s\n' "$$" "$child" > "$CHROTE_BD_TEST_DIR/pids-$$"
if [ "$CHROTE_BD_TEST_EXIT" = 1 ]; then exit 0; fi
wait
if [ "$1" = version ]; then printf 'bd test\n'; else printf '[]'; fi
`
	if err := os.WriteFile(command, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHROTE_BD_COMMAND", command)
	h := NewBeadsHandler()
	h.execTimeout = 10 * time.Second
	t.Cleanup(func() {
		for _, pair := range beadsFixturePIDs(t, dir) {
			for _, pid := range pair {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
	return h, project, dir
}

func beadsFixturePIDs(t *testing.T, dir string) [][2]int {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "pids-*"))
	if err != nil {
		t.Fatal(err)
	}
	var pairs [][2]int
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var parent, child int
		if _, err := fmt.Sscanf(string(b), "%d %d", &parent, &child); err == nil && parent > 0 && child > 0 {
			pairs = append(pairs, [2]int{parent, child})
		}
	}
	return pairs
}

func awaitBeadsProcess(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("Beads process condition did not complete within 2s")
}

func beadsProcessRunning(pid int) bool {
	b, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}
	// A killed orphan may remain a zombie until init reaps it.
	fields := strings.Fields(string(b)[strings.LastIndex(string(b), ")")+1:])
	return len(fields) > 0 && fields[0] != "Z"
}

func startBeadsWork(h *BeadsHandler, ctx context.Context, project string) <-chan *httptest.ResponseRecorder {
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.Work(rec, httptest.NewRequest(http.MethodGet, "/api/beads/work?path="+project, nil).WithContext(ctx))
		done <- rec
	}()
	return done
}

func TestBeadsCommandStopsDescendants(t *testing.T) {
	for _, cause := range []string{"deadline", "disconnect", "wrapper_exit"} {
		t.Run(cause, func(t *testing.T) {
			h, project, dir := beadsProcessFixture(t)
			if cause == "deadline" {
				h.execTimeout = 200 * time.Millisecond
			}
			if cause == "wrapper_exit" {
				t.Setenv("CHROTE_BD_TEST_EXIT", "1")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := startBeadsWork(h, ctx, project)
			awaitBeadsProcess(t, func() bool { return len(beadsFixturePIDs(t, dir)) == 1 })
			if cause == "disconnect" {
				cancel()
			}
			select {
			case rec := <-done:
				if rec.Code != http.StatusBadGateway {
					t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
				}
				want := "context canceled"
				if cause == "deadline" {
					want = "timed out"
				}
				if cause == "wrapper_exit" {
					want = "WaitDelay"
				}
				if !strings.Contains(rec.Body.String(), want) {
					t.Fatalf("missing %q: %s", want, rec.Body.String())
				}
			case <-time.After(2 * time.Second):
				t.Fatal("handler remained blocked on a descendant after cancellation")
			}
			pair := beadsFixturePIDs(t, dir)[0]
			awaitBeadsProcess(t, func() bool { return !beadsProcessRunning(pair[0]) && !beadsProcessRunning(pair[1]) })
		})
	}
}

func TestBeadsCommandsShareLimitAndCanceledWaitersNeverStart(t *testing.T) {
	h, project, dir := beadsProcessFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var work []<-chan *httptest.ResponseRecorder
	for i := 0; i < 4; i++ {
		work = append(work, startBeadsWork(h, ctx, project))
	}
	awaitBeadsProcess(t, func() bool { return len(beadsFixturePIDs(t, dir)) >= 4 })
	waitCtx, stopWaiting := context.WithCancel(context.Background())
	defer stopWaiting()
	waiting := startBeadsWork(h, waitCtx, project)
	health := make(chan struct{})
	go func() {
		h.Health(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/beads/health", nil).WithContext(waitCtx))
		close(health)
	}()
	// The shared refresh must use the same slots but outlive unrelated requests.
	summary := make(chan error, 1)
	go func() {
		_, _, _, err := h.cachedStoreSummary(project, true)
		summary <- err
	}()
	// While all slots are occupied there is no completion event to await.
	// Observe a bounded interval for incorrectly admitted commands to start.
	time.Sleep(100 * time.Millisecond)
	if got := len(beadsFixturePIDs(t, dir)); got != 4 {
		t.Fatalf("launched %d commands while all four slots were occupied", got)
	}
	stopWaiting()
	select {
	case <-waiting:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled work waiter did not return")
	}
	select {
	case <-health:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled health waiter did not return")
	}
	cancel()
	for _, done := range work {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("active work did not release its slot")
		}
	}
	awaitBeadsProcess(t, func() bool { return len(beadsFixturePIDs(t, dir)) >= 5 })
	// Finish the surviving summary command successfully by releasing its child.
	for _, pair := range beadsFixturePIDs(t, dir) {
		if beadsProcessRunning(pair[1]) {
			if err := syscall.Kill(pair[1], syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
		}
	}
	select {
	case err := <-summary:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shared summary did not recover after slots were released")
	}
	if got := len(beadsFixturePIDs(t, dir)); got != 5 {
		t.Fatalf("launched %d commands; canceled waiters must never launch", got)
	}
}
