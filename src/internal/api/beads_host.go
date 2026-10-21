package api

// The host index: every Bead in every store on this host, as the Beads tab
// reads them.
//
// The tab is one table over all stores, so its read is one request. Each store
// contributes a projection built once per store generation, the Dolt manifest
// hash the shared reader already keys its snapshots by. A warm request only
// fingerprints the stores and copies retained bytes: no bd process runs because
// the operator looked. A store whose manifest moved is re-read through the
// shared reader in the background, and its previous projection is served,
// marked pending, until the new one is published. Projections outlive the
// reader's idle eviction, because they are a small fraction of a snapshot.
//
// Search runs here too, over the text the table does not carry: matching ids
// are the answer, so the browser never holds every description on the host.

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/chrote/server/internal/core"
)

// How long a discovered store list is served before the next request walks the
// roots again in the background. New stores are rare; the walk is not free.
const hostDiscoveryTTL = 30 * time.Second

// How long a store's projection is kept after the last request that read it.
const hostViewIdle = time.Hour

// Responses above this size are compressed for clients that accept it.
const hostGzipThreshold = 32 << 10

// hostBeadRow is one Bead as the table, the strip and the work graph draw it.
// Times are Unix seconds, so the browser buckets thousands of rows without
// parsing a date. Size counts the characters of the Bead's own text.
type hostBeadRow struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Status     string   `json:"status"`
	Type       string   `json:"type,omitempty"`
	Priority   int      `json:"priority"`
	Created    int64    `json:"created,omitempty"`
	Updated    int64    `json:"updated,omitempty"`
	Closed     int64    `json:"closed,omitempty"`
	Size       int      `json:"size,omitempty"`
	Parent     string   `json:"parent,omitempty"`
	BlockedBy  []string `json:"blockedBy,omitempty"`
	DeferUntil string   `json:"deferUntil,omitempty"`
}

// hostStoreView is one store's published projection. Never mutated after
// publication; a newer generation replaces the pointer.
type hostStoreView struct {
	generation string
	prefix     string
	readAt     time.Time
	rows       json.RawMessage
	ids        []string
	// Lowercased id, title and text of each row, in the order of ids.
	text []string
}

type beadsHostIndex struct {
	beads    *BeadsHandler
	discover func() []string
	now      func() time.Time

	discoverMu   sync.Mutex
	mu           sync.Mutex
	paths        []string
	discoveredAt time.Time
	refreshing   bool
	views        map[string]*hostStoreView
	used         map[string]time.Time
}

func newBeadsHostIndex(h *BeadsHandler) *beadsHostIndex {
	return &beadsHostIndex{beads: h, now: time.Now, views: make(map[string]*hostStoreView),
		used: make(map[string]time.Time), discover: func() []string { return nil }}
}

// DiscoverStoresWith names the function that lists every store folder on the
// host. The workspace walk owns that question; the Beads routes only consume it.
func (h *BeadsHandler) DiscoverStoresWith(discover func() []string) {
	h.host.mu.Lock()
	defer h.host.mu.Unlock()
	h.host.discover = discover
	h.host.paths, h.host.discoveredAt = nil, time.Time{}
}

// storePaths answers from the last walk and refreshes it in the background once
// it is older than the TTL. Only the very first request waits for a walk.
func (x *beadsHostIndex) storePaths() []string {
	x.mu.Lock()
	if x.paths != nil {
		paths := x.paths
		if x.now().Sub(x.discoveredAt) >= hostDiscoveryTTL && !x.refreshing {
			x.refreshing = true
			go x.walk()
		}
		x.mu.Unlock()
		return paths
	}
	x.mu.Unlock()
	return x.walk()
}

func (x *beadsHostIndex) walk() []string {
	x.discoverMu.Lock()
	defer x.discoverMu.Unlock()
	x.mu.Lock()
	discover := x.discover
	if x.paths != nil && !x.refreshing {
		paths := x.paths
		x.mu.Unlock()
		return paths
	}
	x.mu.Unlock()
	found := discover()
	paths := make([]string, 0, len(found))
	seen := make(map[string]bool, len(found))
	for _, path := range found {
		if path != "" && !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	x.mu.Lock()
	x.paths, x.discoveredAt, x.refreshing = paths, x.now(), false
	// A store nobody has asked for in a while, or that left the host, takes
	// its projection with it.
	for path, used := range x.used {
		if x.now().Sub(used) >= hostViewIdle {
			delete(x.views, path)
			delete(x.used, path)
		}
	}
	x.mu.Unlock()
	return paths
}

// hostStoreResult is one store's entry in the host response.
type hostStoreResult struct {
	Path       string          `json:"path"`
	Prefix     string          `json:"prefix,omitempty"`
	Generation string          `json:"generation,omitempty"`
	ReadAt     string          `json:"readAt,omitempty"`
	Pending    bool            `json:"pending"`
	Error      string          `json:"error,omitempty"`
	Unchanged  bool            `json:"unchanged,omitempty"`
	Rows       json.RawMessage `json:"rows,omitempty"`
}

// store answers one store from its published view when the manifest has not
// moved, and otherwise asks the shared reader without waiting for it.
func (x *beadsHostIndex) store(ctx context.Context, path string) (*hostStoreView, hostStoreResult) {
	result := hostStoreResult{Path: path}
	reader := x.beads.stores
	generation, fingerprintErr := reader.fingerprint(path)
	x.mu.Lock()
	view := x.views[path]
	x.used[path] = x.now()
	x.mu.Unlock()
	if fingerprintErr == nil && view != nil && view.generation == generation {
		// The tab is live demand for every store: keep the reader's snapshot,
		// which the card reads, from idling out while the projection is current.
		reader.touch(path)
		return view, result
	}
	snapshot, state, _ := reader.get(ctx, path, storeReadDemand{})
	if snapshot != nil && (view == nil || view.generation != snapshot.generation) {
		view = x.publish(path, snapshot)
	}
	result.Pending = state.Pending
	result.Error = state.Error
	return view, result
}

// publish builds a store's view once per snapshot generation.
func (x *beadsHostIndex) publish(path string, snapshot *beadsStoreSnapshot) *hostStoreView {
	x.mu.Lock()
	if current := x.views[path]; current != nil && current.generation == snapshot.generation {
		x.mu.Unlock()
		return current
	}
	x.mu.Unlock()
	view := buildHostStoreView(snapshot)
	x.mu.Lock()
	defer x.mu.Unlock()
	if current := x.views[path]; current != nil && current.readAt.After(view.readAt) {
		return current
	}
	x.views[path] = view
	return view
}

func buildHostStoreView(snapshot *beadsStoreSnapshot) *hostStoreView {
	rows := make([]hostBeadRow, 0, len(snapshot.issues))
	view := &hostStoreView{generation: snapshot.generation, readAt: snapshot.readAt,
		ids: make([]string, 0, len(snapshot.issues)), text: make([]string, 0, len(snapshot.issues))}
	prefixes := make(map[string]int)
	var haystack strings.Builder
	for _, issue := range snapshot.issues {
		id := beadString(issue, "id")
		if id == "" {
			continue
		}
		if prefix := beadPrefix(id); prefix != "" {
			prefixes[prefix]++
		}
		description, design := beadString(issue, "description"), beadString(issue, "design")
		acceptance, notes := beadString(issue, "acceptance_criteria"), beadString(issue, "notes")
		row := hostBeadRow{
			ID:         id,
			Title:      beadString(issue, "title"),
			Status:     beadString(issue, "status"),
			Type:       firstString(issue["issue_type"], issue["type"]),
			Priority:   issuePriority(issue),
			Created:    unixSeconds(firstString(issue["created_at"], issue["created"])),
			Updated:    unixSeconds(firstString(issue["updated_at"], issue["updated"])),
			Closed:     unixSeconds(beadString(issue, "closed_at")),
			Size:       utf8.RuneCountInString(description) + utf8.RuneCountInString(design) + utf8.RuneCountInString(acceptance) + utf8.RuneCountInString(notes),
			Parent:     beadString(issue, "parent"),
			BlockedBy:  beadDependencyIDs(issue, blocksDependency),
			DeferUntil: deferredUntil(issue),
		}
		if len(row.BlockedBy) == 0 {
			row.BlockedBy = nil
		}
		rows = append(rows, row)
		haystack.Reset()
		for _, part := range []string{id, row.Title, description, design, acceptance, notes, beadString(issue, "close_reason")} {
			haystack.WriteString(part)
			haystack.WriteByte('\n')
		}
		if labels, ok := issue["labels"].([]interface{}); ok {
			for _, label := range labels {
				if text, ok := label.(string); ok {
					haystack.WriteString(text)
					haystack.WriteByte(' ')
				}
			}
		}
		view.ids = append(view.ids, id)
		view.text = append(view.text, strings.ToLower(haystack.String()))
	}
	// A store's own prefix is the one most of its ids carry: a few Beads filed
	// with a foreign prefix must not rename the store.
	best := 0
	for prefix, count := range prefixes {
		if count > best || (count == best && prefix < view.prefix) {
			view.prefix, best = prefix, count
		}
	}
	if view.prefix == "" {
		view.prefix = snapshot.prefix
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		encoded = []byte("[]")
	}
	view.rows = encoded
	return view
}

func unixSeconds(value string) int64 {
	if value == "" {
		return 0
	}
	at, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return 0
	}
	return at.Unix()
}

// hostPaths is the walk plus the manual stores a request names, each validated
// the same way every Beads route validates a path.
func (x *beadsHostIndex) hostPaths(r *http.Request) ([]string, []hostStoreResult) {
	paths := append([]string{}, x.storePaths()...)
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		seen[path] = true
	}
	var refused []hostStoreResult
	for _, requested := range r.URL.Query()["path"] {
		path, code, message := validateBeadsProjectPath(requested)
		if code != "" {
			refused = append(refused, hostStoreResult{Path: requested, Error: message})
			continue
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		if _, err := x.beads.checkBeadsDirectory(path); err != nil {
			refused = append(refused, hostStoreResult{Path: path, Error: err.Error()})
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, refused
}

// Host handles GET /api/beads/host: every store's rows, or, for a store whose
// generation the browser already holds (known=<generation>:<path>), only the
// fact that it is unchanged.
func (h *BeadsHandler) Host(w http.ResponseWriter, r *http.Request) {
	known := make(map[string]string)
	for _, value := range r.URL.Query()["known"] {
		if generation, path, ok := strings.Cut(value, ":"); ok {
			known[path] = generation
		}
	}
	paths, refused := h.host.hostPaths(r)
	results := make([]hostStoreResult, len(paths))
	var group sync.WaitGroup
	slots := make(chan struct{}, workspaceProbeFanOut)
	for i, path := range paths {
		group.Add(1)
		go func(i int, path string) {
			defer group.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			view, result := h.host.store(r.Context(), path)
			if view != nil {
				result.Prefix, result.Generation = view.prefix, view.generation
				result.ReadAt = view.readAt.UTC().Format(time.RFC3339Nano)
				if known[path] == view.generation {
					result.Unchanged = true
				} else {
					result.Rows = view.rows
				}
			}
			results[i] = result
		}(i, path)
	}
	group.Wait()
	writeBeadsCompressed(w, r, map[string]interface{}{
		"stores":    append(results, refused...),
		"checkedAt": h.host.now().UTC().Format(time.RFC3339Nano),
	})
}

// Search handles GET /api/beads/search?q=: the ids, per store, whose id, title
// or text holds every whitespace-separated term, case-insensitively. It reads
// only published views, so it never starts a bd process.
func (h *BeadsHandler) Search(w http.ResponseWriter, r *http.Request) {
	terms := strings.Fields(strings.ToLower(r.URL.Query().Get("q")))
	matches := make(map[string][]string)
	if len(terms) > 0 {
		h.host.mu.Lock()
		views := make(map[string]*hostStoreView, len(h.host.views))
		for path, view := range h.host.views {
			views[path] = view
		}
		h.host.mu.Unlock()
		for path, view := range views {
			found := make([]string, 0)
			for i, text := range view.text {
				if containsAll(text, terms) {
					found = append(found, view.ids[i])
				}
			}
			if len(found) > 0 {
				matches[path] = found
			}
		}
	}
	writeBeadsCompressed(w, r, map[string]interface{}{"matches": matches})
}

func containsAll(text string, terms []string) bool {
	for _, term := range terms {
		if !strings.Contains(text, term) {
			return false
		}
	}
	return true
}

// writeBeadsCompressed writes the success envelope, gzipped at the fastest level
// when it is large and the client accepts it: thousands of rows cross a tailnet
// as well as loopback.
func writeBeadsCompressed(w http.ResponseWriter, r *http.Request, data interface{}) {
	body, err := json.Marshal(core.NewSuccessResponse(data))
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Vary", "Accept-Encoding")
	if len(body) < hostGzipThreshold || !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}
	var compressed bytes.Buffer
	writer, _ := gzip.NewWriterLevel(&compressed, gzip.BestSpeed)
	_, _ = writer.Write(body)
	_ = writer.Close()
	w.Header().Set("Content-Encoding", "gzip")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(compressed.Bytes())
}
