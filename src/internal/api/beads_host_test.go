package api

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type hostStoreReply struct {
	Path       string                   `json:"path"`
	Name       string                   `json:"name"`
	Prefix     string                   `json:"prefix"`
	Generation string                   `json:"generation"`
	ReadAt     string                   `json:"readAt"`
	Pending    bool                     `json:"pending"`
	Stale      bool                     `json:"stale"`
	Error      string                   `json:"error"`
	Unchanged  bool                     `json:"unchanged"`
	Rows       []map[string]interface{} `json:"rows"`
}

func hostRequest(t *testing.T, h *BeadsHandler, query url.Values) map[string]hostStoreReply {
	t.Helper()
	rec := httptest.NewRecorder()
	h.Host(rec, httptest.NewRequest(http.MethodGet, "/api/beads/host?"+query.Encode(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("host status=%d: %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Data struct {
			Stores []hostStoreReply `json:"stores"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode host response: %v", err)
	}
	stores := make(map[string]hostStoreReply, len(envelope.Data.Stores))
	for _, store := range envelope.Data.Stores {
		stores[store.Path] = store
	}
	return stores
}

// awaitHostStore repeats the browser's poll until the store has settled.
func awaitHostStore(t *testing.T, h *BeadsHandler, path string) hostStoreReply {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		store := hostRequest(t, h, nil)[path]
		if !store.Pending {
			return store
		}
		if time.Now().After(deadline) {
			t.Fatalf("store %s still pending: %+v", path, store)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func hostRecords(title string) []map[string]interface{} {
	return []map[string]interface{}{
		{"id": "test-aaa", "title": title, "status": "open", "issue_type": "epic", "priority": float64(1),
			"created_at": "2026-10-20T10:00:00Z", "updated_at": "2026-10-21T08:00:00Z",
			"description": "Grill the brain's ownership", "notes": "ä", "labels": []interface{}{"brain-organization"}},
		{"id": "test-bbb", "title": "Finished child", "status": "closed", "issue_type": "task", "parent": "test-aaa",
			"created_at": "2026-10-20T11:00:00Z", "closed_at": "2026-10-20T12:00:00Z",
			"close_reason": "Shipped in 66dfbe6",
			"dependencies": []interface{}{
				map[string]interface{}{"issue_id": "test-bbb", "depends_on_id": "ctx-5t7", "type": "blocks"},
				map[string]interface{}{"issue_id": "test-bbb", "depends_on_id": "test-aaa", "type": "parent-child"},
			}},
		{"id": "other-ccc", "title": "Filed with a foreign prefix", "status": "open"},
	}
}

// The tab's one read: a cold store starts one background read and is pending;
// once read, every later request answers from the published projection, and a
// browser that already holds that generation is told only that it is unchanged.
// Looking again never starts another bd read while the manifest stands still.
func TestBeadsHostServesEveryStoreFromOneReadPerGeneration(t *testing.T) {
	h, paths := readerProjects(t, 2)
	h.DiscoverStoresWith(func() []string { return paths })
	var calls atomic.Int32
	h.stores.read = func(_ context.Context, path string) ([]map[string]interface{}, error) {
		calls.Add(1)
		if path == paths[1] {
			return nil, errors.New("bd list exploded")
		}
		return hostRecords("Grill ownership"), nil
	}

	first := hostRequest(t, h, nil)
	if !first[paths[0]].Pending || first[paths[0]].Rows != nil {
		t.Fatalf("cold store = %+v, want pending without rows", first[paths[0]])
	}
	store := awaitHostStore(t, h, paths[0])
	if store.Prefix != "test" || store.Generation == "" || len(store.Rows) != 3 {
		t.Fatalf("read store = %+v, want three rows under the majority prefix", store)
	}
	epic, child := store.Rows[0], store.Rows[1]
	if epic["created"] != float64(time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC).Unix()) || epic["priority"] != float64(1) {
		t.Errorf("epic row times and priority = %#v", epic)
	}
	if epic["size"] != float64(len([]rune("Grill the brain's ownership"))+1) {
		t.Errorf("epic size = %v, want the rune count of its own text", epic["size"])
	}
	if child["parent"] != "test-aaa" || child["closed"] == nil {
		t.Errorf("child row = %#v, want its parent and close time", child)
	}
	if blockers, _ := child["blockedBy"].([]interface{}); len(blockers) != 1 || blockers[0] != "ctx-5t7" {
		t.Errorf("child blockers = %#v, want only the blocks edge, across stores", child["blockedBy"])
	}
	failed := awaitHostStore(t, h, paths[1])
	if !strings.Contains(failed.Error, "bd list exploded") || failed.Rows != nil {
		t.Errorf("failed store = %+v, want its error and no rows", failed)
	}

	readsBefore := calls.Load()
	known := url.Values{"known": {store.Generation + ":" + paths[0]}}
	for i := 0; i < 3; i++ {
		again := hostRequest(t, h, known)[paths[0]]
		if !again.Unchanged || again.Rows != nil || again.Pending {
			t.Fatalf("known generation = %+v, want unchanged without rows", again)
		}
	}
	// Only the failing store may retry, on its own backoff; the good one never.
	if extra := calls.Load() - readsBefore; extra > 1 {
		t.Fatalf("repeat requests ran %d more reads, want none for the current store", extra)
	}
}

// A write moves the manifest: the last projection is served, marked pending,
// while exactly one read publishes the next.
func TestBeadsHostServesThePreviousProjectionWhileAWriteIsRead(t *testing.T) {
	h, paths := readerProjects(t, 1)
	path := paths[0]
	h.DiscoverStoresWith(func() []string { return paths })
	release := make(chan struct{})
	var calls atomic.Int32
	h.stores.read = func(ctx context.Context, _ string) ([]map[string]interface{}, error) {
		if calls.Add(1) == 1 {
			return hostRecords("Before"), nil
		}
		select {
		case <-release:
			return hostRecords("After"), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	hostRequest(t, h, nil)
	before := awaitHostStore(t, h, path)

	readerManifest(t, path, "written\n")
	during := hostRequest(t, h, nil)[path]
	if !during.Pending || len(during.Rows) != 3 || during.Rows[0]["title"] != "Before" || during.Generation != before.Generation {
		t.Fatalf("during read = %+v, want the previous rows marked pending", during)
	}
	close(release)
	after := awaitHostStore(t, h, path)
	if after.Generation == before.Generation || after.Rows[0]["title"] != "After" || calls.Load() != 2 {
		t.Fatalf("after read = %+v (reads %d), want the new generation from one more read", after, calls.Load())
	}
}

// Search reads the text the table never carries, ANDs the terms and ignores
// case, and only ever consults published projections.
func TestBeadsSearchMatchesEveryTermInIdTitleAndText(t *testing.T) {
	h, paths := readerProjects(t, 1)
	path := paths[0]
	h.DiscoverStoresWith(func() []string { return paths })
	var calls atomic.Int32
	h.stores.read = func(context.Context, string) ([]map[string]interface{}, error) {
		calls.Add(1)
		return hostRecords("Grill ownership"), nil
	}
	hostRequest(t, h, nil)
	awaitHostStore(t, h, path)

	search := func(q string) []string {
		rec := httptest.NewRecorder()
		h.Search(rec, httptest.NewRequest(http.MethodGet, "/api/beads/search?q="+url.QueryEscape(q), nil))
		matches := decodeBeadsData(t, rec)["matches"].(map[string]interface{})
		ids := []string{}
		if found, ok := matches[path].([]interface{}); ok {
			for _, id := range found {
				ids = append(ids, id.(string))
			}
		}
		return ids
	}
	for q, want := range map[string]string{
		"BRAIN'S ownership":  "test-aaa",
		"brain-organization": "test-aaa",
		"66dfbe6":            "test-bbb",
		"foreign other-ccc":  "other-ccc",
		"grill nowhere":      "",
	} {
		got := strings.Join(search(q), ",")
		if got != want {
			t.Errorf("search %q = %q, want %q", q, got, want)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("search ran %d reads, want only the original one", calls.Load())
	}
}

// The walk is not free: one request pays for it, the next ones within the TTL
// do not, and an expired list is served while the walk refreshes it.
func TestBeadsHostDiscoveryIsSharedAndRefreshedInTheBackground(t *testing.T) {
	h, paths := readerProjects(t, 1)
	h.stores.read = func(context.Context, string) ([]map[string]interface{}, error) { return hostRecords("x"), nil }
	var walks atomic.Int32
	walked := make(chan struct{}, 4)
	h.DiscoverStoresWith(func() []string {
		walks.Add(1)
		walked <- struct{}{}
		return paths
	})
	now := time.Now()
	h.host.now = func() time.Time { return now }
	hostRequest(t, h, nil)
	hostRequest(t, h, nil)
	if walks.Load() != 1 {
		t.Fatalf("walks = %d, want one shared walk", walks.Load())
	}
	<-walked
	now = now.Add(hostDiscoveryTTL)
	if stores := hostRequest(t, h, nil); len(stores) != 1 {
		t.Fatalf("expired list served %d stores, want the previous list", len(stores))
	}
	awaitReaderEvent(t, walked)
}

func TestBeadsHostCompressesLargeResponsesForClientsThatAcceptIt(t *testing.T) {
	h, paths := readerProjects(t, 1)
	h.DiscoverStoresWith(func() []string { return paths })
	many := make([]map[string]interface{}, 0, 600)
	for i := 0; i < 600; i++ {
		many = append(many, map[string]interface{}{"id": "test-" + strings.Repeat("a", 3) + string(rune('a'+i%26)), "title": strings.Repeat("A long enough title ", 4), "status": "open"})
	}
	h.stores.read = func(context.Context, string) ([]map[string]interface{}, error) { return many, nil }
	hostRequest(t, h, nil)
	awaitHostStore(t, h, paths[0])

	request := httptest.NewRequest(http.MethodGet, "/api/beads/host", nil)
	request.Header.Set("Accept-Encoding", "gzip, deflate")
	rec := httptest.NewRecorder()
	h.Host(rec, request)
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("large response encoding = %q, want gzip", rec.Header().Get("Content-Encoding"))
	}
	reader, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(reader)
	if !strings.Contains(string(body), `"success":true`) || len(body) <= rec.Body.Len() {
		t.Fatalf("decompressed body is not the larger success envelope")
	}
}

// Discovery names stores by their resolved folders; a manual path that links
// to one of them is the same store, and the table lists it once.
func TestBeadsHostListsALinkedManualStoreOnce(t *testing.T) {
	h, paths := readerProjects(t, 1)
	real, err := filepath.EvalSymlinks(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	h.DiscoverStoresWith(func() []string { return []string{real} })
	h.stores.read = func(context.Context, string) ([]map[string]interface{}, error) { return hostRecords("x"), nil }
	link := filepath.Join(filepath.Dir(paths[0]), "linked")
	if err := os.Symlink(paths[0], link); err != nil {
		t.Fatal(err)
	}
	stores := hostRequest(t, h, url.Values{"path": {link}})
	if _, ok := stores[real]; len(stores) != 1 || !ok {
		t.Fatalf("stores = %v, want only the discovered %s", stores, real)
	}
}

// A zero time is a missing date: it must not stretch the strip back to 1970
// or to year one.
func TestBeadsHostReadsAZeroTimeAsNoTime(t *testing.T) {
	for value, want := range map[string]int64{
		"":                     0,
		"not a time":           0,
		"0001-01-01T00:00:00Z": 0,
		"1970-01-01T00:00:00Z": 0,
		"2026-10-20T10:00:00Z": time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC).Unix(),
	} {
		if got := unixSeconds(value); got != want {
			t.Errorf("unixSeconds(%q) = %d, want %d", value, got, want)
		}
	}
}

func hostText(t *testing.T, h *BeadsHandler, path, id string) (int, map[string]interface{}) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.Text(rec, httptest.NewRequest(http.MethodGet, "/api/beads/text?"+url.Values{"path": {path}, "id": {id}}.Encode(), nil))
	if rec.Code != http.StatusOK {
		return rec.Code, nil
	}
	return rec.Code, decodeBeadsData(t, rec)
}

// The operator opens the tab now and then. Hours of idleness and a server
// restart both lose the reader's snapshots; neither may make a visit cold.
// The restarted server answers at once from what it kept: a store whose
// manifest has not moved is current without any bd, and one written since is
// served as it was, marked stale and pending, while one read replaces it.
func TestBeadsHostServesKeptProjectionsAtOnceAfterARestart(t *testing.T) {
	keep := t.TempDir()
	h, paths := readerProjects(t, 2)
	h.KeepHostViewsIn(keep)
	h.DiscoverStoresWith(func() []string { return paths })
	h.stores.read = func(context.Context, string) ([]map[string]interface{}, error) { return hostRecords("Kept"), nil }
	hostRequest(t, h, nil)
	first := awaitHostStore(t, h, paths[0])
	awaitHostStore(t, h, paths[1])
	deadline := time.Now().Add(2 * time.Second)
	for {
		files, _ := filepath.Glob(filepath.Join(keep, "*.json.gz"))
		if len(files) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("kept files = %v, want one per store", files)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// A new server over the same stores: one of them was written meanwhile.
	readerManifest(t, paths[1], "written while the server was down\n")
	restarted := NewBeadsHandler()
	restarted.DiscoverStoresWith(func() []string { return paths })
	release := make(chan struct{})
	var calls atomic.Int32
	restarted.stores.read = func(ctx context.Context, _ string) ([]map[string]interface{}, error) {
		calls.Add(1)
		select {
		case <-release:
			return hostRecords("Fresh"), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	t.Cleanup(func() { close(release) })
	restarted.KeepHostViewsIn(keep)

	stores := hostRequest(t, restarted, nil)
	current, written := stores[paths[0]], stores[paths[1]]
	if current.Pending || current.Stale || current.Generation != first.Generation || len(current.Rows) != 3 || current.ReadAt != first.ReadAt {
		t.Fatalf("unchanged store after restart = %+v, want its kept rows, current", current)
	}
	if !written.Pending || !written.Stale || len(written.Rows) != 3 || written.Rows[0]["title"] != "Kept" {
		t.Fatalf("written store after restart = %+v, want its kept rows marked stale and pending", written)
	}
	if calls.Load() > 1 {
		t.Fatalf("restart ran %d reads, want one, for the written store only", calls.Load())
	}
	// The kept text answers search and the selected Bead without a read.
	code, text := hostText(t, restarted, paths[0], "test-aaa")
	if code != http.StatusOK || text["bead"].(map[string]interface{})["description"] != "Grill the brain's ownership" {
		t.Fatalf("kept text = %d %v", code, text)
	}
}

// Idleness evicts the reader's snapshot, never the tab's projection: a visit
// after hours away is answered from it, with no bd for an unchanged store.
func TestBeadsHostKeepsADiscoveredStoresProjectionThroughIdleness(t *testing.T) {
	h, paths := readerProjects(t, 1)
	path := paths[0]
	h.DiscoverStoresWith(func() []string { return paths })
	var calls atomic.Int32
	h.stores.read = func(context.Context, string) ([]map[string]interface{}, error) {
		calls.Add(1)
		return hostRecords("Read once"), nil
	}
	now := time.Now()
	h.host.now = func() time.Time { return now }
	h.stores.now = func() time.Time { return now }
	hostRequest(t, h, nil)
	read := awaitHostStore(t, h, path)

	// Thirty hours on: the reader's sweep evicts its snapshot, and the next
	// discovery walk finds the store still on the host.
	now = now.Add(30 * time.Hour)
	h.stores.mu.Lock()
	h.stores.sweepLocked(now)
	evicted := len(h.stores.entries) == 0
	h.stores.mu.Unlock()
	if !evicted {
		t.Fatal("the reader's idle snapshot was not evicted; the test no longer simulates idleness")
	}
	h.host.mu.Lock()
	h.host.refreshing = true
	h.host.mu.Unlock()
	h.host.walk()

	visit := hostRequest(t, h, nil)[path]
	if visit.Pending || visit.Stale || visit.Generation != read.Generation || len(visit.Rows) != 3 || calls.Load() != 1 {
		t.Fatalf("visit after idleness = %+v (reads %d), want the kept rows at once and no read", visit, calls.Load())
	}
}

// A Bead's own text comes from the projection, never from bd, and a store or
// id the projection does not hold is plainly not found.
func TestBeadsTextAnswersTheSelectedBeadFromTheProjection(t *testing.T) {
	h, paths := readerProjects(t, 1)
	path := paths[0]
	h.DiscoverStoresWith(func() []string { return paths })
	var calls atomic.Int32
	h.stores.read = func(context.Context, string) ([]map[string]interface{}, error) {
		calls.Add(1)
		return hostRecords("Grill ownership"), nil
	}
	if code, _ := hostText(t, h, path, "test-aaa"); code != http.StatusNotFound {
		t.Fatalf("text before any read = %d, want 404", code)
	}
	hostRequest(t, h, nil)
	awaitHostStore(t, h, path)

	code, data := hostText(t, h, path, "test-bbb")
	bead, _ := data["bead"].(map[string]interface{})
	if code != http.StatusOK || bead["id"] != "test-bbb" || bead["closeReason"] != "Shipped in 66dfbe6" || data["readAt"] == "" {
		t.Fatalf("text = %d %v, want the closed Bead's close reason and read time", code, data)
	}
	_, data = hostText(t, h, path, "test-aaa")
	bead = data["bead"].(map[string]interface{})
	if bead["notes"] != "ä" || bead["labels"].([]interface{})[0] != "brain-organization" {
		t.Fatalf("epic text = %v, want its notes and labels", bead)
	}
	if code, _ := hostText(t, h, path, "test-zzz"); code != http.StatusNotFound {
		t.Fatalf("unknown id = %d, want 404", code)
	}
	if calls.Load() != 1 {
		t.Fatalf("text ran %d reads, want only the tab's own", calls.Load())
	}
}

// Terminal links need every store's prefix on each page load. A store the tab
// has read answers from its projection, an empty store included, so a page
// load runs no bd once every store is known.
func TestBeadsProjectPrefixComesFromTheProjectionWithoutBd(t *testing.T) {
	h, paths := readerProjects(t, 2)
	h.DiscoverStoresWith(func() []string { return paths })
	h.stores.read = func(_ context.Context, path string) ([]map[string]interface{}, error) {
		if path == paths[1] {
			return []map[string]interface{}{}, nil
		}
		return hostRecords("x"), nil
	}
	hostRequest(t, h, nil)
	awaitHostStore(t, h, paths[0])
	awaitHostStore(t, h, paths[1])
	h.stores.mu.Lock()
	for key := range h.stores.entries {
		h.stores.removeLocked(key)
	}
	h.stores.mu.Unlock()
	h.bdCommand = filepath.Join(t.TempDir(), "bd-must-not-run")

	for i, want := range []string{"test", ""} {
		prefix, err := h.projectPrefix(context.Background(), paths[i])
		if err != nil || prefix != want {
			t.Fatalf("prefix of %s = %q, %v; want %q without bd", paths[i], prefix, err, want)
		}
	}
}

// The store the Brain tab reads is called the brain, whatever its folder is.
func TestBeadsHostNamesAStoreByItsRole(t *testing.T) {
	h, paths := readerProjects(t, 2)
	h.DiscoverStoresWith(func() []string { return paths })
	h.stores.read = func(context.Context, string) ([]map[string]interface{}, error) { return hostRecords("x"), nil }
	h.NameStore(paths[1], "brain")
	stores := hostRequest(t, h, nil)
	if stores[paths[1]].Name != "brain" || stores[paths[0]].Name != "" {
		t.Fatalf("names = %q and %q, want only the configured store named", stores[paths[0]].Name, stores[paths[1]].Name)
	}
}
