package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func readerProjects(t *testing.T, n int) (*BeadsHandler, []string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("CHROTE_ROOTS", root)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	paths := make([]string, n)
	for i := range paths {
		paths[i] = filepath.Join(root, fmt.Sprintf("project-%d", i))
		makeValidBeadsWorkspace(t, paths[i])
	}
	h := NewBeadsHandler()
	t.Cleanup(func() {
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
	return h, paths
}

func readerRecords(title string) []map[string]interface{} {
	return []map[string]interface{}{{"id": "test-aaa", "title": title, "status": "open", "issue_type": "task"},
		{"id": "test-bbb", "title": "Finished", "status": "closed", "issue_type": "task", "parent": "test-aaa"}}
}

func readerManifest(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(path, ".beads", "embeddeddolt", "test", ".dolt", "noms", "manifest"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func awaitReaderEvent[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("store reader event did not complete")
		var zero T
		return zero
	}
}

func readerWork(h *BeadsHandler, path string, ctx context.Context) <-chan *httptest.ResponseRecorder {
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.Work(rec, httptest.NewRequest(http.MethodGet, "/api/beads/work?path="+path, nil).WithContext(ctx))
		done <- rec
	}()
	return done
}

func TestBeadsStatePromotesColdStoreWithoutWaitingAndProjectionsShareTheRead(t *testing.T) {
	h, paths := readerProjects(t, 1)
	path := paths[0]
	started, release := make(chan struct{}, 1), make(chan struct{})
	var calls atomic.Int32
	h.stores.read = func(ctx context.Context, _ string) ([]map[string]interface{}, error) {
		calls.Add(1)
		started <- struct{}{}
		select {
		case <-release:
			return readerRecords("Current"), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	t.Cleanup(func() { close(release) })
	stateDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.State(rec, httptest.NewRequest(http.MethodGet, "/api/beads/state?path="+path+"&foreground="+path, nil))
		stateDone <- rec
	}()
	rec := awaitReaderEvent(t, stateDone)
	awaitReaderEvent(t, started)
	data := decodeBeadsData(t, rec)
	state := data["stores"].([]interface{})[0].(map[string]interface{})
	if state["pending"] != true || state["counts"] != nil || state["availableGeneration"] != nil {
		t.Fatalf("cold state = %#v, want pending with unknown counts", state)
	}
	work := readerWork(h, path, context.Background())
	closed, card := make(chan *httptest.ResponseRecorder, 1), make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ClosedWork(rec, httptest.NewRequest(http.MethodGet, "/api/beads/closed?path="+path, nil))
		closed <- rec
	}()
	go func() {
		rec := httptest.NewRecorder()
		h.IssueDetail(rec, httptest.NewRequest(http.MethodGet, "/api/beads/issue?path="+path+"&id=test-aaa", nil))
		card <- rec
	}()
	release <- struct{}{}
	var generation, readAt string
	for name, result := range map[string]<-chan *httptest.ResponseRecorder{"work": work, "closed": closed, "card": card} {
		rec := awaitReaderEvent(t, result)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d: %s", name, rec.Code, rec.Body.String())
		}
		projection := decodeBeadsData(t, rec)
		state := projection["state"].(map[string]interface{})
		got, gotRead := state["availableGeneration"].(string), state["readAt"].(string)
		if generation == "" {
			generation, readAt = got, gotRead
		}
		if got != generation || gotRead != readAt || state["pending"] != false {
			t.Fatalf("%s projection did not identify the shared snapshot: %#v", name, state)
		}
	}
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.State(rec, httptest.NewRequest(http.MethodGet, "/api/beads/state?path="+path, nil))
	}
	summary, ready, _, err := h.cachedStoreSummary(path, true)
	if err != nil || !ready || summary.Counts.Status.Open != 1 || summary.Counts.Status.Closed != 1 || calls.Load() != 1 {
		t.Fatalf("summary=%+v ready=%v err=%v reads=%d", summary, ready, err, calls.Load())
	}
}

func TestBeadsReaderRejectsWriteDuringReadAndRetriesBeforeColdSuccess(t *testing.T) {
	h, paths := readerProjects(t, 1)
	path := paths[0]
	var calls atomic.Int32
	h.stores.read = func(context.Context, string) ([]map[string]interface{}, error) {
		if calls.Add(1) == 1 {
			readerManifest(t, path, "a concurrent write")
			return readerRecords("Obsolete"), nil
		}
		return readerRecords("New"), nil
	}
	rec := awaitReaderEvent(t, readerWork(h, path, context.Background()))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	data := decodeBeadsData(t, rec)
	rows := data["beads"].([]interface{})
	state := data["state"].(map[string]interface{})
	generation, _ := storeManifestHash(path)
	if calls.Load() != 2 || rows[0].(map[string]interface{})["title"] != "New" || state["availableGeneration"] != generation {
		t.Fatalf("write-racing records were certified as the new generation: reads=%d %s", calls.Load(), rec.Body.String())
	}
}

func TestBeadsReaderRepeatedWritesReturnErrorOrTruthfulLastSuccess(t *testing.T) {
	for _, warm := range []bool{false, true} {
		t.Run(fmt.Sprint(warm), func(t *testing.T) {
			h, paths := readerProjects(t, 1)
			path := paths[0]
			var calls atomic.Int32
			h.stores.read = func(context.Context, string) ([]map[string]interface{}, error) {
				n := calls.Add(1)
				if !warm || n > 1 {
					readerManifest(t, path, fmt.Sprint(n))
				}
				return readerRecords(fmt.Sprint(n)), nil
			}
			var previous *beadsStoreSnapshot
			if warm {
				previous, _, _ = h.stores.get(context.Background(), path, storeReadDemand{foreground: true, waitCold: true})
			}
			snapshot, state, err := h.stores.get(context.Background(), path, storeReadDemand{foreground: true, force: true, waitFresh: true, waitCold: true})
			if err == nil || !strings.Contains(err.Error(), "changed during read") || state.Error == "" || state.Pending {
				t.Fatalf("racing read snapshot=%v state=%+v err=%v", snapshot, state, err)
			}
			if !warm && (snapshot != nil || state.Counts != nil || state.AvailableGeneration != "") {
				t.Fatalf("cold racing read fabricated success: %+v", state)
			}
			if warm && (snapshot != previous || state.AvailableGeneration != previous.generation || state.ObservedGeneration == previous.generation) {
				t.Fatalf("last-success generation was relabeled: %+v", state)
			}
		})
	}
}

func TestBeadsReaderSelectedStorePromotesOriginalQueuedJobPastSlowHostPeers(t *testing.T) {
	h, paths := readerProjects(t, 5)
	started := make(chan string, 8)
	release := make(map[string]chan struct{}, len(paths))
	for _, path := range paths {
		release[path] = make(chan struct{})
	}
	t.Cleanup(func() {
		for _, ch := range release {
			close(ch)
		}
	})
	h.stores.read = func(ctx context.Context, path string) ([]map[string]interface{}, error) {
		started <- path
		select {
		case <-release[path]:
			return readerRecords(path), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	for _, path := range paths {
		_, state, _ := h.stores.get(context.Background(), path, storeReadDemand{})
		if !state.Pending {
			t.Fatalf("cold host store not pending: %+v", state)
		}
	}
	first, second := awaitReaderEvent(t, started), awaitReaderEvent(t, started)
	if first == paths[4] || second == paths[4] {
		t.Fatal("fixture did not create queued selected work")
	}
	// This is the same foreground=true used by /state and still does not wait.
	_, state, _ := h.stores.get(context.Background(), paths[4], storeReadDemand{foreground: true})
	if !state.Pending || awaitReaderEvent(t, started) != paths[4] {
		t.Fatal("selected store was not promoted past queued background peers")
	}
	h.stores.mu.Lock()
	active, background := h.stores.active, h.stores.background
	h.stores.mu.Unlock()
	if active != 3 || background != 2 {
		t.Fatalf("active=%d background=%d, want selected plus two background reads", active, background)
	}
	work := readerWork(h, paths[4], context.Background())
	release[paths[4]] <- struct{}{}
	if rec := awaitReaderEvent(t, work); rec.Code != http.StatusOK {
		t.Fatalf("selected read waited for the whole host: %s", rec.Body.String())
	}
}

func TestBeadsReaderKeepsSuccessAndRetriesFailuresAtTheSameGeneration(t *testing.T) {
	h, paths := readerProjects(t, 1)
	path := paths[0]
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	h.stores.now = func() time.Time { return time.Unix(0, clock.Load()) }
	var calls atomic.Int32
	h.stores.read = func(context.Context, string) ([]map[string]interface{}, error) {
		if calls.Add(1) == 2 {
			return nil, errors.New("authoritative export failed")
		}
		return readerRecords("Current"), nil
	}
	first, _, err := h.stores.get(context.Background(), path, storeReadDemand{foreground: true, waitCold: true})
	if err != nil {
		t.Fatal(err)
	}
	previous, state, err := h.stores.get(context.Background(), path, storeReadDemand{foreground: true, force: true, waitFresh: true, waitCold: true})
	if err == nil || previous != first || state.Error != "authoritative export failed" || state.ReadAt != first.readAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("failed refresh lost last success: state=%+v err=%v", state, err)
	}
	_, state, _ = h.stores.get(context.Background(), path, storeReadDemand{})
	if state.Error != "authoritative export failed" || state.Pending || calls.Load() != 2 {
		t.Fatalf("fingerprint check hid failure or bypassed backoff: %+v reads=%d", state, calls.Load())
	}
	clock.Add(int64(1100 * time.Millisecond))
	newSnapshot, state, err := h.stores.get(context.Background(), path, storeReadDemand{foreground: true, waitFresh: true, waitCold: true})
	if err != nil || state.Error != "" || newSnapshot == first || newSnapshot.generation != first.generation || calls.Load() != 3 {
		t.Fatalf("unchanged-generation failure did not recover: %+v err=%v reads=%d", state, err, calls.Load())
	}
}

func TestBeadsReaderCanceledSubscriberCannotAbandonAnotherSubscriber(t *testing.T) {
	h, paths := readerProjects(t, 1)
	started, release := make(chan struct{}, 1), make(chan struct{})
	t.Cleanup(func() { close(release) })
	var calls atomic.Int32
	h.stores.read = func(ctx context.Context, _ string) ([]map[string]interface{}, error) {
		calls.Add(1)
		started <- struct{}{}
		select {
		case <-release:
			return readerRecords("Survived"), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	first := readerWork(h, paths[0], ctx)
	awaitReaderEvent(t, started)
	second := readerWork(h, paths[0], context.Background())
	cancel()
	if rec := awaitReaderEvent(t, first); rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "context canceled") {
		t.Fatalf("canceled subscriber response: %s", rec.Body.String())
	}
	release <- struct{}{}
	if rec := awaitReaderEvent(t, second); rec.Code != http.StatusOK || calls.Load() != 1 {
		t.Fatalf("shared command was abandoned/duplicated: %s reads=%d", rec.Body.String(), calls.Load())
	}
}

func TestBeadsReaderDeferralExpiresWithoutSourceWriteOrRead(t *testing.T) {
	h, paths := readerProjects(t, 1)
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	h.stores.now = func() time.Time { return time.Unix(0, clock.Load()) }
	var calls atomic.Int32
	until := h.stores.now().Add(time.Second).UTC().Format(time.RFC3339)
	h.stores.read = func(context.Context, string) ([]map[string]interface{}, error) {
		calls.Add(1)
		return []map[string]interface{}{{"id": "test-aaa", "status": "deferred", "defer_until": until}, {"id": "test-bbb", "status": "deferred"}, {"id": "test-ccc", "status": "deferred", "defer_until": "invalid"}}, nil
	}
	snapshot, before, _ := h.stores.get(context.Background(), paths[0], storeReadDemand{foreground: true, waitCold: true})
	clock.Add(int64(2 * time.Second))
	_, after, _ := h.stores.get(context.Background(), paths[0], storeReadDemand{})
	if before.Counts.Status.Deferred != 3 || after.Counts.Status.Deferred != 2 || after.Counts.Status.Open != 1 || calls.Load() != 1 || snapshot.generation != after.AvailableGeneration {
		t.Fatalf("expiry required a write/read or classified incorrectly: before=%+v after=%+v reads=%d", before, after, calls.Load())
	}
	work := httptest.NewRecorder()
	h.Work(work, httptest.NewRequest(http.MethodGet, "/api/beads/work?path="+paths[0], nil))
	workData := decodeBeadsData(t, work)
	rows := workData["beads"].([]interface{})
	card := httptest.NewRecorder()
	h.IssueDetail(card, httptest.NewRequest(http.MethodGet, "/api/beads/issue?path="+paths[0]+"&id=test-aaa", nil))
	cardData := decodeBeadsData(t, card)["bead"].(map[string]interface{})
	if rows[0].(map[string]interface{})["status"] != "open" || cardData["status"] != "open" || cardData["deferUntil"] != until || calls.Load() != 1 {
		t.Fatalf("expiry did not reach rows/card without another read: work=%s card=%s reads=%d", work.Body.String(), card.Body.String(), calls.Load())
	}

}

func TestBeadsReaderCanonicalStoreAliasesShareOneSnapshot(t *testing.T) {
	h, paths := readerProjects(t, 2)
	shared := filepath.Join(paths[0], "shared-store")
	if err := os.Rename(filepath.Join(paths[0], ".beads"), shared); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, filepath.Join(paths[0], ".beads")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(paths[1], ".beads")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, filepath.Join(paths[1], ".beads")); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	h.stores.read = func(context.Context, string) ([]map[string]interface{}, error) {
		calls.Add(1)
		return readerRecords("Shared"), nil
	}
	first, _, err := h.stores.get(context.Background(), paths[0], storeReadDemand{foreground: true, waitCold: true})
	if err != nil {
		t.Fatal(err)
	}
	second, state, err := h.stores.get(context.Background(), paths[1], storeReadDemand{foreground: true, waitCold: true})
	if err != nil || first != second || calls.Load() != 1 || state.Path != paths[1] {
		t.Fatalf("alias made another source read: state=%+v err=%v reads=%d", state, err, calls.Load())
	}
}

func TestBeadsReaderBudgetIdleExpiryAndOversizedFailureRemainBounded(t *testing.T) {
	h, paths := readerProjects(t, 3)
	var clock atomic.Int64
	clock.Store(time.Now().UnixNano())
	h.stores.now = func() time.Time { return time.Unix(0, clock.Load()) }
	var calls atomic.Int32
	h.stores.read = func(context.Context, string) ([]map[string]interface{}, error) {
		calls.Add(1)
		return readerRecords("Same weight"), nil
	}
	first, _, _ := h.stores.get(context.Background(), paths[0], storeReadDemand{foreground: true, waitCold: true})
	h.stores.maxBytes = first.bytes * 2
	for _, path := range paths[1:] {
		clock.Add(int64(time.Second))
		if _, _, err := h.stores.get(context.Background(), path, storeReadDemand{foreground: true, waitCold: true}); err != nil {
			t.Fatal(err)
		}
	}
	retained := 0
	for _, entry := range h.stores.entries {
		if entry.snapshot != nil {
			retained++
		}
	}
	if h.stores.bytes > h.stores.maxBytes || retained != 2 {
		t.Fatalf("cache exceeded bounds: bytes=%d retained=%d", h.stores.bytes, retained)
	}
	if _, _, err := h.stores.get(context.Background(), paths[0], storeReadDemand{foreground: true, waitCold: true}); err != nil || calls.Load() != 4 {
		t.Fatalf("evicted same-generation projection did not recover: err=%v reads=%d", err, calls.Load())
	}
	clock.Add(int64(11 * time.Minute))
	if _, _, err := h.stores.get(context.Background(), paths[0], storeReadDemand{foreground: true, waitCold: true}); err != nil || calls.Load() != 5 || len(h.stores.entries) != 1 {
		t.Fatalf("expired scope did not recover same-generation demand: err=%v reads=%d entries=%d", err, calls.Load(), len(h.stores.entries))
	}
	h.stores.maxBytes = first.bytes - 1
	snapshot, state, err := h.stores.get(context.Background(), paths[1], storeReadDemand{foreground: true, waitCold: true})
	if err == nil || snapshot != nil || state.AvailableGeneration != "" || state.Counts != nil || !strings.Contains(state.Error, "budget") {
		t.Fatalf("oversized cold read fabricated success: state=%+v err=%v", state, err)
	}
}

func TestBeadsReaderFingerprintFailureCanRecoverThroughWrappedRead(t *testing.T) {
	h, paths := readerProjects(t, 1)
	var readable atomic.Bool
	var calls atomic.Int32
	h.stores.fingerprint = func(path string) (string, error) {
		if !readable.Load() {
			return "", errors.New("manifest unreadable before owner wrapper repairs it")
		}
		return storeManifestHash(path)
	}
	h.stores.read = func(context.Context, string) ([]map[string]interface{}, error) {
		calls.Add(1)
		readable.Store(true)
		return readerRecords("Verified after repair"), nil
	}
	snapshot, state, err := h.stores.get(context.Background(), paths[0], storeReadDemand{foreground: true, waitCold: true})
	if err != nil || snapshot == nil || state.Error != "" || calls.Load() != 2 || state.AvailableGeneration == "" {
		t.Fatalf("fingerprint failure trapped owner read or certified after-only data: state=%+v err=%v reads=%d", state, err, calls.Load())
	}
}

func TestBeadsReaderQueuedJobsExpireWithoutStartingAnotherCommand(t *testing.T) {
	h, paths := readerProjects(t, 3)
	h.execTimeout = 50 * time.Millisecond
	started := make(chan string, 3)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	h.stores.read = func(ctx context.Context, path string) ([]map[string]interface{}, error) {
		started <- path
		<-release // Keep running jobs in descendant cleanup while a queued deadline expires.
		return nil, ctx.Err()
	}
	for _, path := range paths {
		_, _, _ = h.stores.get(context.Background(), path, storeReadDemand{})
	}
	awaitReaderEvent(t, started)
	awaitReaderEvent(t, started)
	key, _ := filepath.EvalSymlinks(filepath.Join(paths[2], ".beads"))
	h.stores.mu.Lock()
	job := h.stores.entries[key].job
	h.stores.mu.Unlock()
	awaitReaderEvent(t, job.done)
	if job.snapshot != nil || job.err == nil || !strings.Contains(job.err.Error(), "timed out") {
		t.Fatalf("queued source read did not expire: %v", job.err)
	}
	select {
	case path := <-started:
		t.Fatalf("expired queued source launched: %s", path)
	default:
	}
}

func TestBeadsReaderEntryCountAndRetainedPrefixDoNotStartAnotherRead(t *testing.T) {
	h, paths := readerProjects(t, 3)
	h.stores.maxStores = 2
	var calls atomic.Int32
	h.stores.read = func(context.Context, string) ([]map[string]interface{}, error) {
		calls.Add(1)
		return readerRecords("Known"), nil
	}
	for _, path := range paths {
		if _, _, err := h.stores.get(context.Background(), path, storeReadDemand{foreground: true, waitCold: true}); err != nil {
			t.Fatal(err)
		}
	}
	if len(h.stores.entries) != 2 {
		t.Fatalf("retained %d entries above store bound", len(h.stores.entries))
	}
	h.bdCommand = "missing-bd-command-for-retained-prefix"
	if prefix := h.projectPrefix(context.Background(), paths[2]); prefix != "test" || calls.Load() != 3 {
		t.Fatalf("known terminal prefix demanded another command: prefix=%q reads=%d", prefix, calls.Load())
	}
}
