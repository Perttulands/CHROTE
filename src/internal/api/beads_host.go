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
// marked stale and pending, until the new one is published. The operator
// opens this tab now and then, so a visit after hours away is the normal one:
// every discovered store's last projection is kept for as long as the store is
// on the host, outliving the reader's idle eviction, and with a cache directory
// it is also written to disk, so a server restart serves it at once too. A
// projection whose generation still matches the manifest is current, read
// from disk or not, and costs no bd at all.
//
// Search runs here too, over the text the table does not carry: matching ids
// are the answer, so the browser never holds every description on the host.
// The same retained text answers for one Bead when the tab shows it.

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

// How long the projection of a store that is no longer discovered (one that
// left the host, or a manual path nobody asks for) is kept after the last
// request that read it. A discovered store's projection is never dropped.
// Eviction runs with discovery, so with no requests nothing is dropped.
const hostViewIdle = time.Hour

// The on-disk form of a projection. A file of another version is ignored and
// rewritten by the next read.
const hostViewFileVersion = 1

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

// hostBeadText is the part of a Bead the table does not carry: what the tab
// shows for the selected Bead, and what search reads.
type hostBeadText struct {
	Description string   `json:"description,omitempty"`
	Design      string   `json:"design,omitempty"`
	Acceptance  string   `json:"acceptance,omitempty"`
	Notes       string   `json:"notes,omitempty"`
	CloseReason string   `json:"closeReason,omitempty"`
	Assignee    string   `json:"assignee,omitempty"`
	Labels      []string `json:"labels,omitempty"`
}

// hostStoreView is one store's published projection. Never mutated after
// publication; a newer generation replaces the pointer.
type hostStoreView struct {
	generation string
	prefix     string
	readAt     time.Time
	rows       json.RawMessage
	ids        []string
	// The position of each id in ids.
	at map[string]int
	// Each row's own text, in the order of ids.
	texts []hostBeadText
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
	// What a store is called when its folder says less than its role does.
	names map[string]string

	// Where projections are kept across restarts; empty keeps them in memory
	// only. loaded closes once the kept ones are back.
	dir    string
	loaded chan struct{}
	saveMu sync.Mutex
}

func newBeadsHostIndex(h *BeadsHandler) *beadsHostIndex {
	loaded := make(chan struct{})
	close(loaded)
	return &beadsHostIndex{beads: h, now: time.Now, views: make(map[string]*hostStoreView),
		used: make(map[string]time.Time), names: make(map[string]string), loaded: loaded,
		discover: func() []string { return nil }}
}

// NameStore gives the store in folder a name for the tab to show instead of
// its prefix: the store a host's configuration gives a role, such as the brain
// the Brain tab reads, is called by that role rather than by its folder.
func (h *BeadsHandler) NameStore(folder, name string) {
	folder, name = strings.TrimSpace(folder), strings.TrimSpace(name)
	if folder == "" || name == "" {
		return
	}
	if resolved, err := filepath.EvalSymlinks(folder); err == nil {
		folder = resolved
	}
	h.host.mu.Lock()
	defer h.host.mu.Unlock()
	h.host.names[filepath.Clean(folder)] = name
}

// KeepHostViewsIn keeps every store's projection in dir, so a restarted server
// shows the last read of every store at once instead of after a full read. The
// kept projections are loaded in the background; requests wait for that, which
// takes a fraction of the reads it saves. A directory that cannot be made
// keeps projections in memory only, and the log says why.
func (h *BeadsHandler) KeepHostViewsIn(dir string) {
	x := h.host
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Printf("beads host views are kept in memory only: %v", err)
		return
	}
	loaded := make(chan struct{})
	x.mu.Lock()
	x.dir, x.loaded = dir, loaded
	x.mu.Unlock()
	go func() {
		defer close(loaded)
		x.loadViews(dir)
	}()
}

// awaitLoaded holds a request until the kept projections are back, so the
// first visit after a restart is answered from them rather than as cold.
func (x *beadsHostIndex) awaitLoaded(ctx context.Context) {
	x.mu.Lock()
	loaded := x.loaded
	x.mu.Unlock()
	select {
	case <-loaded:
	case <-ctx.Done():
	}
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
	// A store that left the host, or a manual one nobody has asked for in a
	// while, takes its projection with it. A discovered store keeps its own:
	// the next visit, however far off, shows it at once.
	listed := make(map[string]bool, len(paths))
	for _, path := range paths {
		listed[path] = true
	}
	var dropped []string
	for path, used := range x.used {
		if !listed[path] && x.now().Sub(used) >= hostViewIdle {
			delete(x.views, path)
			delete(x.used, path)
			dropped = append(dropped, path)
		}
	}
	dir := x.dir
	x.mu.Unlock()
	for _, path := range dropped {
		if dir != "" {
			_ = os.Remove(hostViewFile(dir, path))
		}
	}
	return paths
}

// hostStoreResult is one store's entry in the host response.
type hostStoreResult struct {
	Path string `json:"path"`
	// The store's name, when its role names it better than its prefix.
	Name       string `json:"name,omitempty"`
	Prefix     string `json:"prefix,omitempty"`
	Generation string `json:"generation,omitempty"`
	ReadAt     string `json:"readAt,omitempty"`
	Pending    bool   `json:"pending"`
	// The rows are the store as it was at readAt: it has been written since,
	// or could not be checked.
	Stale     bool            `json:"stale,omitempty"`
	Error     string          `json:"error,omitempty"`
	Unchanged bool            `json:"unchanged,omitempty"`
	Rows      json.RawMessage `json:"rows,omitempty"`
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
	// A manifest that cannot be fingerprinted fails the read too, so the
	// reader's backoff, not this poll, decides when bd runs for it again.
	snapshot, state, _ := reader.get(ctx, path, storeReadDemand{})
	if snapshot != nil && (view == nil || view.generation != snapshot.generation) {
		view = x.publish(path, snapshot)
	}
	result.Pending = state.Pending
	result.Error = state.Error
	result.Stale = view != nil && (fingerprintErr != nil || view.generation != generation)
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
	dir := x.dir
	if dir != "" {
		go x.saveView(dir, path, view)
	}
	return view
}

// haystack is what search reads for one Bead: its id, title and text.
func haystack(builder *strings.Builder, id, title string, text hostBeadText) string {
	builder.Reset()
	for _, part := range []string{id, title, text.Description, text.Design, text.Acceptance, text.Notes, text.CloseReason} {
		builder.WriteString(part)
		builder.WriteByte('\n')
	}
	for _, label := range text.Labels {
		builder.WriteString(label)
		builder.WriteByte(' ')
	}
	return strings.ToLower(builder.String())
}

func buildHostStoreView(snapshot *beadsStoreSnapshot) *hostStoreView {
	rows := make([]hostBeadRow, 0, len(snapshot.issues))
	view := &hostStoreView{generation: snapshot.generation, readAt: snapshot.readAt,
		ids: make([]string, 0, len(snapshot.issues)), text: make([]string, 0, len(snapshot.issues)),
		texts: make([]hostBeadText, 0, len(snapshot.issues)), at: make(map[string]int, len(snapshot.issues))}
	prefixes := make(map[string]int)
	var builder strings.Builder
	for _, issue := range snapshot.issues {
		id := beadString(issue, "id")
		if id == "" {
			continue
		}
		if prefix := beadPrefix(id); prefix != "" {
			prefixes[prefix]++
		}
		text := hostBeadText{
			Description: beadString(issue, "description"),
			Design:      beadString(issue, "design"),
			Acceptance:  beadString(issue, "acceptance_criteria"),
			Notes:       beadString(issue, "notes"),
			CloseReason: beadString(issue, "close_reason"),
			Assignee:    beadString(issue, "assignee"),
		}
		if labels, ok := issue["labels"].([]interface{}); ok {
			for _, label := range labels {
				if name, ok := label.(string); ok {
					text.Labels = append(text.Labels, name)
				}
			}
		}
		row := hostBeadRow{
			ID:         id,
			Title:      beadString(issue, "title"),
			Status:     beadString(issue, "status"),
			Type:       firstString(issue["issue_type"], issue["type"]),
			Priority:   issuePriority(issue),
			Created:    unixSeconds(firstString(issue["created_at"], issue["created"])),
			Updated:    unixSeconds(firstString(issue["updated_at"], issue["updated"])),
			Closed:     unixSeconds(beadString(issue, "closed_at")),
			Size:       utf8.RuneCountInString(text.Description) + utf8.RuneCountInString(text.Design) + utf8.RuneCountInString(text.Acceptance) + utf8.RuneCountInString(text.Notes),
			Parent:     beadString(issue, "parent"),
			BlockedBy:  beadDependencyIDs(issue, blocksDependency),
			DeferUntil: deferredUntil(issue),
		}
		if len(row.BlockedBy) == 0 {
			row.BlockedBy = nil
		}
		rows = append(rows, row)
		view.at[id] = len(view.ids)
		view.ids = append(view.ids, id)
		view.texts = append(view.texts, text)
		view.text = append(view.text, haystack(&builder, id, row.Title, text))
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
	if err != nil || at.Unix() <= 0 {
		// A zero time is a missing date, not a day in 1970 or year one.
		return 0
	}
	return at.Unix()
}

// hostPaths is the walk plus the manual stores a request names, each validated
// the same way every Beads route validates a path.
func (x *beadsHostIndex) hostPaths(r *http.Request) ([]string, []hostStoreResult) {
	paths := append([]string{}, x.storePaths()...)
	// Discovery names stores by their resolved folders; a manual path that is
	// a link to one of them is the same store, and is not listed twice.
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
		real := path
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			real = resolved
		}
		if seen[path] || seen[real] {
			continue
		}
		seen[path], seen[real] = true, true
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
	h.host.awaitLoaded(r.Context())
	paths, refused := h.host.hostPaths(r)
	h.host.mu.Lock()
	names := make(map[string]string, len(h.host.names))
	for folder, name := range h.host.names {
		names[folder] = name
	}
	h.host.mu.Unlock()
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
			result.Name = names[path]
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
		h.host.awaitLoaded(r.Context())
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

// hostBeadTextReply is one Bead's own text, as the tab shows the selected Bead.
type hostBeadTextReply struct {
	ID string `json:"id"`
	hostBeadText
}

// Text handles GET /api/beads/text?path=&id=: one Bead's description, design,
// acceptance, notes, close reason, assignee and labels, from the store's
// published projection. It never starts a bd process: a store not yet read
// answers 404, and a stale projection answers with its own read time.
func (h *BeadsHandler) Text(w http.ResponseWriter, r *http.Request) {
	path, code, message := validateBeadsProjectPath(r.URL.Query().Get("path"))
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, message)
		return
	}
	id, code, message := requiredIssueID(r)
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, message)
		return
	}
	h.host.awaitLoaded(r.Context())
	view := h.host.viewFor(path)
	if view == nil {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", "The Beads in "+path+" have not been read yet")
		return
	}
	at, known := view.at[id]
	if !known {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("No Bead %s in %s as read at %s", id, path, view.readAt.UTC().Format(time.RFC3339)))
		return
	}
	writeBeadsCompressed(w, r, map[string]interface{}{
		"bead":       hostBeadTextReply{ID: id, hostBeadText: view.texts[at]},
		"generation": view.generation,
		"readAt":     view.readAt.UTC().Format(time.RFC3339Nano),
	})
}

// viewFor is the published projection of the store in path, by the name the
// host response gave it or by its resolved folder.
func (x *beadsHostIndex) viewFor(path string) *hostStoreView {
	x.mu.Lock()
	defer x.mu.Unlock()
	if view := x.views[path]; view != nil {
		return view
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return x.views[resolved]
	}
	return nil
}

// knownPrefix answers a store's prefix from its projection, read now or kept
// from before a restart. An empty store is known to have none. Ids do not
// change prefix when a store is written, so a stale projection still answers.
func (x *beadsHostIndex) knownPrefix(ctx context.Context, path string) (string, bool) {
	x.awaitLoaded(ctx)
	if view := x.viewFor(path); view != nil {
		return view.prefix, true
	}
	return "", false
}

// hostViewFile is where the projection of the store in path is kept.
func hostViewFile(dir, path string) string {
	sum := sha256.Sum256([]byte(path))
	return filepath.Join(dir, hex.EncodeToString(sum[:12])+".json.gz")
}

// hostViewRecord is a projection as it is kept on disk: the rows exactly as
// served, and each row's own text, from which search's text is rebuilt.
type hostViewRecord struct {
	Version    int             `json:"version"`
	Path       string          `json:"path"`
	Generation string          `json:"generation"`
	Prefix     string          `json:"prefix"`
	ReadAt     time.Time       `json:"readAt"`
	Rows       json.RawMessage `json:"rows"`
	Texts      []hostBeadText  `json:"texts"`
}

// saveView writes a store's projection beside the others, replacing the file
// whole so a crash leaves the previous one. A projection already replaced by a
// newer one is not written; the newer one's own write follows.
func (x *beadsHostIndex) saveView(dir, path string, view *hostStoreView) {
	x.saveMu.Lock()
	defer x.saveMu.Unlock()
	x.mu.Lock()
	current := x.views[path] == view
	x.mu.Unlock()
	if !current {
		return
	}
	if err := writeHostView(dir, path, view); err != nil {
		log.Printf("beads host view for %s was not kept: %v", path, err)
	}
}

func writeHostView(dir, path string, view *hostStoreView) error {
	record := hostViewRecord{Version: hostViewFileVersion, Path: path, Generation: view.generation,
		Prefix: view.prefix, ReadAt: view.readAt, Rows: view.rows, Texts: view.texts}
	temp, err := os.CreateTemp(dir, ".view-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	writer, _ := gzip.NewWriterLevel(temp, gzip.BestSpeed)
	if err := json.NewEncoder(writer).Encode(record); err != nil {
		temp.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), hostViewFile(dir, path))
}

// loadViews brings back every kept projection. A file that does not read as
// one is skipped; the store is read again as if it had never been.
func (x *beadsHostIndex) loadViews(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("beads host views not loaded: %v", err)
		}
		return
	}
	started := time.Now()
	loaded := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json.gz") {
			continue
		}
		path, view, err := readHostView(filepath.Join(dir, entry.Name()))
		if err != nil {
			log.Printf("beads host view %s skipped: %v", entry.Name(), err)
			continue
		}
		x.mu.Lock()
		if current := x.views[path]; current == nil || current.readAt.Before(view.readAt) {
			x.views[path] = view
			x.used[path] = x.now()
			loaded++
		}
		x.mu.Unlock()
	}
	log.Printf("beads host views loaded=%d in=%s", loaded, time.Since(started).Round(time.Millisecond))
}

func readHostView(file string) (string, *hostStoreView, error) {
	handle, err := os.Open(file)
	if err != nil {
		return "", nil, err
	}
	defer handle.Close()
	reader, err := gzip.NewReader(handle)
	if err != nil {
		return "", nil, err
	}
	var record hostViewRecord
	if err := json.NewDecoder(reader).Decode(&record); err != nil {
		return "", nil, err
	}
	if record.Version != hostViewFileVersion || record.Path == "" || record.Generation == "" {
		return "", nil, fmt.Errorf("version %d of %s is not this server's", record.Version, record.Path)
	}
	var rows []hostBeadRow
	if err := json.Unmarshal(record.Rows, &rows); err != nil {
		return "", nil, err
	}
	if len(rows) != len(record.Texts) {
		return "", nil, fmt.Errorf("%d rows but %d texts", len(rows), len(record.Texts))
	}
	view := &hostStoreView{generation: record.Generation, prefix: record.Prefix, readAt: record.ReadAt,
		rows: record.Rows, ids: make([]string, len(rows)), at: make(map[string]int, len(rows)),
		texts: record.Texts, text: make([]string, len(rows))}
	var builder strings.Builder
	for i, row := range rows {
		view.ids[i] = row.ID
		view.at[row.ID] = i
		view.text[i] = haystack(&builder, row.ID, row.Title, record.Texts[i])
	}
	return record.Path, view, nil
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
