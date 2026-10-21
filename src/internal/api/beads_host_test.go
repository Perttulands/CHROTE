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
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type hostStoreReply struct {
	Path       string                   `json:"path"`
	Prefix     string                   `json:"prefix"`
	Generation string                   `json:"generation"`
	Pending    bool                     `json:"pending"`
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
