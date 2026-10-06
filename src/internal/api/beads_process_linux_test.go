package api

import (
	"context"
	"errors"
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

func TestBeadsSnapshotCostsCanceledAdmissionDoesNotCountAProcess(t *testing.T) {
	for _, admission := range []string{"optional", "process"} {
		t.Run(admission, func(t *testing.T) {
			h, project, dir := beadsProcessFixture(t)
			job := &beadsStoreJob{owner: h.stores, promoted: make(chan struct{})}
			if admission == "optional" {
				for i := 0; i < cap(h.optionalSlots); i++ {
					h.optionalSlots <- struct{}{}
				}
			} else {
				job.foreground = true
				close(job.promoted)
				for i := 0; i < cap(h.execSlots); i++ {
					h.execSlots <- struct{}{}
				}
			}
			ctx := context.WithValue(context.Background(), beadsJobContextKey{}, job)
			_, err := h.runBd(ctx, project, 30*time.Millisecond, "list")
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("held %s admission did not expire: %v", admission, err)
			}
			if job.costs.bdAdmission <= 0 || job.costs.sourceCalls != 0 || job.costs.bdProcess != 0 {
				t.Fatalf("waiting was attributed to an unlaunched process: %+v", job.costs)
			}
			if pids := beadsFixturePIDs(t, dir); len(pids) != 0 {
				t.Fatalf("expired admission launched source processes: %v", pids)
			}
		})
	}
}

func TestBeadsSnapshotCostsCountRealProcessesAcrossVerificationRetry(t *testing.T) {
	h, project, dir := beadsProcessFixture(t)
	command, err := os.ReadFile(h.bdCommand)
	if err != nil {
		t.Fatal(err)
	}
	// Keep the real runner and descendant lifecycle fixture, returning a record
	// that the reader must decode and index after successful verification.
	command = []byte(strings.Replace(string(command), "printf '[]'", `printf '%s' '[{"id":"test-aaa","title":"Current","status":"open","issue_type":"task"}]'`, 1))
	if err := os.WriteFile(h.bdCommand, command, 0700); err != nil {
		t.Fatal(err)
	}
	read := h.stores.read
	captured := make(chan *beadsStoreJob, 1)
	calls := 0
	h.stores.read = func(ctx context.Context, path string) ([]map[string]interface{}, error) {
		calls++
		if calls == 1 {
			captured <- ctx.Value(beadsJobContextKey{}).(*beadsStoreJob)
		}
		issues, err := read(ctx, path)
		if err == nil && calls == 1 {
			// The first actual command finished, then the source changed. Its
			// now-obsolete records must not be relabeled as the newer generation.
			readerManifest(t, path, "concurrent source checkpoint")
			issues[0]["title"] = "Obsolete"
		}
		return issues, err
	}
	for i := 0; i < cap(h.execSlots); i++ {
		h.execSlots <- struct{}{}
	}
	done := readerWork(h, project, context.Background())
	job := awaitReaderEvent(t, captured)
	if pids := beadsFixturePIDs(t, dir); len(pids) != 0 {
		t.Fatalf("full process admission launched source processes: %v", pids)
	}
	<-h.execSlots // Admit the same queued source call, not another job.
	for count := 1; count <= 2; count++ {
		awaitBeadsProcess(t, func() bool { return len(beadsFixturePIDs(t, dir)) == count })
		for _, pair := range beadsFixturePIDs(t, dir) {
			// Ending the held child lets its parent return valid JSON normally.
			_ = syscall.Kill(pair[1], syscall.SIGTERM)
		}
	}
	rec := awaitReaderEvent(t, done)
	awaitReaderEvent(t, job.done)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	data := decodeBeadsData(t, rec)
	rows := data["beads"].([]interface{})
	if rows[0].(map[string]interface{})["title"] != "Current" || job.snapshot.byID["test-aaa"]["title"] != "Current" {
		t.Fatalf("unverified first records were published: %s", rec.Body.String())
	}
	if job.costs.sourceCalls != 2 || job.costs.readAttempts != 2 || job.costs.bdAdmission <= 0 || job.costs.bdProcess <= 0 || job.costs.jsonDecode <= 0 || job.costs.snapshotBuild <= 0 {
		t.Fatalf("actual two-command verification costs were not retained: %+v", job.costs)
	}
	state := data["state"].(map[string]interface{})
	if state["readAt"] != job.snapshot.readAt.UTC().Format(time.RFC3339Nano) || state["availableGeneration"] != job.snapshot.generation {
		t.Fatalf("diagnostic job and published snapshot identify different reads: %s", rec.Body.String())
	}
}

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

// Runner ownership is independent of HTTP subscribers: a store read must
// outlive a disconnected consumer, while runBd must still own all descendants.
func startBeadsCommand(h *BeadsHandler, ctx context.Context, project string) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := h.execBdIssues(ctx, project, "list", "--status", "all", "--limit", "0")
		done <- err
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
			done := startBeadsCommand(h, ctx, project)
			awaitBeadsProcess(t, func() bool { return len(beadsFixturePIDs(t, dir)) == 1 })
			if cause == "disconnect" {
				cancel()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("runner completed without its cancellation/pipe error")
				}
				want := "context canceled"
				if cause == "deadline" {
					want = "timed out"
				}
				if cause == "wrapper_exit" {
					want = "WaitDelay"
				}
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("missing %q: %v", want, err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("handler remained blocked on a descendant after cancellation")
			}
			pair := beadsFixturePIDs(t, dir)[0]
			awaitBeadsProcess(t, func() bool { return !beadsProcessRunning(pair[0]) && !beadsProcessRunning(pair[1]) })
		})
	}
}

func TestBeadsCancellationAllowsWriterExitCleanup(t *testing.T) {
	for _, cause := range []string{"deadline", "disconnect", "ignores_term"} {
		t.Run(cause, func(t *testing.T) {
			h, project, dir := beadsProcessFixture(t)
			manifest := filepath.Join(project, ".beads", "embeddeddolt", "test", ".dolt", "noms", "manifest")
			t.Setenv("CHROTE_BD_TEST_MANIFEST", manifest)
			script := `#!/bin/sh
trap 'chmod 660 "$CHROTE_BD_TEST_MANIFEST"' EXIT
trap 'exit 143' TERM
chmod 600 "$CHROTE_BD_TEST_MANIFEST"
sleep 30 &
child=$!
printf '%s %s\n' "$$" "$child" > "$CHROTE_BD_TEST_DIR/pids-$$"
wait
`
			if cause == "ignores_term" {
				script = strings.ReplaceAll(script, "trap 'exit 143' TERM", "trap '' TERM")
			}
			if err := os.WriteFile(h.bdCommand, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			h.execTimeout = 200 * time.Millisecond
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := startBeadsCommand(h, ctx, project)
			awaitBeadsProcess(t, func() bool { return len(beadsFixturePIDs(t, dir)) == 1 })
			before, err := os.Stat(manifest)
			if err != nil || before.Mode().Perm() != 0600 {
				t.Fatalf("writer did not restrict manifest: %v, %v", before, err)
			}
			if cause == "disconnect" {
				cancel()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("canceled writer returned success")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("writer exceeded bounded cancellation cleanup")
			}
			after, err := os.Stat(manifest)
			if err != nil {
				t.Fatal(err)
			}
			want := os.FileMode(0660)
			if cause == "ignores_term" {
				// Forced kill cannot execute cleanup; only catchable exits promise repair.
				want = 0600
			}
			if after.Mode().Perm() != want {
				t.Fatalf("manifest mode after cancellation = %o, want %o", after.Mode().Perm(), want)
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
	var work []<-chan error
	for i := 0; i < 4; i++ {
		work = append(work, startBeadsCommand(h, ctx, project))
	}
	awaitBeadsProcess(t, func() bool { return len(beadsFixturePIDs(t, dir)) >= 4 })
	waitCtx, stopWaiting := context.WithCancel(context.Background())
	defer stopWaiting()
	waiting := startBeadsCommand(h, waitCtx, project)
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

// Prefix catalog and snapshot background demand must together leave process
// capacity for the selected store. This exercises the real runner, not a read
// stub whose concurrency could bypass the four subprocess slots.
func TestBeadsForegroundPromotionEscapesBackgroundCatalogAdmission(t *testing.T) {
	h, project, dir := beadsProcessFixture(t)
	paths := []string{filepath.Join(dir, "background-one"), filepath.Join(dir, "background-two")}
	for _, path := range paths {
		makeValidBeadsWorkspace(t, path)
	}
	catalogCtx, cancelCatalog := context.WithCancel(context.Background())
	catalogDone := make(chan struct{})
	projects := make([]map[string]interface{}, 12)
	for i := range projects {
		projects[i] = map[string]interface{}{"path": project}
	}
	go func() {
		h.addProjectPrefixes(catalogCtx, projects)
		close(catalogDone)
	}()
	t.Cleanup(func() {
		cancelCatalog()
		awaitReaderEvent(t, catalogDone)
		h.stores.mu.Lock()
		jobs := make([]*beadsStoreJob, 0)
		for _, entry := range h.stores.entries {
			if entry.job != nil {
				entry.job.cancel()
				jobs = append(jobs, entry.job)
			}
		}
		h.stores.mu.Unlock()
		for _, job := range jobs {
			awaitReaderEvent(t, job.done)
		}
	})
	awaitBeadsProcess(t, func() bool { return len(beadsFixturePIDs(t, dir)) == 2 })
	initial := make(map[int]bool)
	for _, pair := range beadsFixturePIDs(t, dir) {
		initial[pair[0]] = true
	}
	for _, path := range paths {
		_, _, _ = h.stores.get(context.Background(), path, storeReadDemand{})
	}
	h.stores.mu.Lock()
	if h.stores.active != 2 || h.stores.background != 2 {
		t.Errorf("fixture did not activate two background snapshot jobs")
	}
	h.stores.mu.Unlock()
	// Promote the already active job waiting on optional admission. Its source
	// read must enter a spare process slot without either prefix command ending.
	selected := readerWork(h, paths[0], context.Background())
	awaitBeadsProcess(t, func() bool { return len(beadsFixturePIDs(t, dir)) == 3 })
	if len(h.execSlots) != 3 || len(h.optionalSlots) != 2 {
		t.Fatalf("foreground did not reserve capacity: active=%d optional=%d", len(h.execSlots), len(h.optionalSlots))
	}
	for _, pair := range beadsFixturePIDs(t, dir) {
		if !initial[pair[0]] {
			if err := syscall.Kill(pair[1], syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
		}
	}
	if rec := awaitReaderEvent(t, selected); rec.Code != http.StatusOK {
		t.Fatalf("selected project waited behind catalog demand: %s", rec.Body.String())
	}
	if got := len(beadsFixturePIDs(t, dir)); got != 3 {
		t.Fatalf("selected demand duplicated the store job or admitted excess background commands: %d", got)
	}
}
