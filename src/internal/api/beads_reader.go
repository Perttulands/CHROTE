package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/chrote/server/internal/core"
)

// BeadsStoreState separates the snapshot a consumer can use from the source
// most recently checked. Counts are absent until an actual successful read.
type BeadsStoreState struct {
	Path                string       `json:"path"`
	AvailableGeneration string       `json:"availableGeneration,omitempty"`
	ObservedGeneration  string       `json:"observedGeneration,omitempty"`
	ReadAt              string       `json:"readAt,omitempty"`
	CheckedAt           string       `json:"checkedAt,omitempty"`
	Pending             bool         `json:"pending"`
	Error               string       `json:"error,omitempty"`
	Counts              *BeadsCounts `json:"counts,omitempty"`
	OpenBeads           *int         `json:"openBeads,omitempty"`
	NewestUpdate        string       `json:"newestUpdate,omitempty"`
}

// Never mutate a published snapshot. Each route builds its own projection;
// replacing the pointer also replaces records and relationship index together.
type beadsStoreSnapshot struct {
	issues     []map[string]interface{}
	byID       map[string]map[string]interface{}
	prefix     string
	generation string
	readAt     time.Time
	bytes      int64
}

type beadsStoreEntry struct {
	path       string
	snapshot   *beadsStoreSnapshot
	observed   string
	checkedAt  time.Time
	accessedAt time.Time
	err        error
	failures   int
	retryAt    time.Time
	job        *beadsStoreJob
}

type beadsStoreJob struct {
	key               string
	path              string
	foreground        bool
	owner             *beadsStoreReader
	promoted          chan struct{}
	releaseBackground func()
	running           bool
	queuedAt          time.Time
	ctx               context.Context
	cancel            context.CancelFunc
	timer             *time.Timer
	done              chan struct{}
	snapshot          *beadsStoreSnapshot
	observed          string
	checkedAt         time.Time
	err               error
	costs             beadsReadCosts
}

// These costs belong to one refresh goroutine, including both verification
// attempts. They never change source truth or the command admission policy.
type beadsReadCosts struct {
	sourceCalls   int
	readAttempts  int
	bdAdmission   time.Duration
	bdProcess     time.Duration
	jsonDecode    time.Duration
	snapshotBuild time.Duration
}

func beadsCostsFromContext(ctx context.Context) *beadsReadCosts {
	if job, _ := ctx.Value(beadsJobContextKey{}).(*beadsStoreJob); job != nil {
		return &job.costs
	}
	return nil
}

// Jobs, including their queue wait, have the same bounded lifetime as runBd.
// Two background reads leave capacity for the selected project and open card.
// There are no per-store pollers: visible demand checks the cheap fingerprint.
type beadsStoreReader struct {
	mu          sync.Mutex
	entries     map[string]*beadsStoreEntry
	queue       []*beadsStoreJob
	active      int
	background  int
	bytes       int64
	maxBytes    int64
	maxStores   int
	idle        time.Duration
	now         func() time.Time
	fingerprint func(string) (string, error)
	read        func(context.Context, string) ([]map[string]interface{}, error)
	timeout     func() time.Duration
}

func newBeadsStoreReader(h *BeadsHandler) *beadsStoreReader {
	return &beadsStoreReader{
		entries: make(map[string]*beadsStoreEntry),
		// Four times JSON size is a conservative working estimate for decoded
		// maps/strings and indexes, not a promise about exact Go heap usage.
		maxBytes: 128 << 20, maxStores: 128, idle: 10 * time.Minute,
		now: time.Now, fingerprint: storeManifestHash,
		read: func(ctx context.Context, path string) ([]map[string]interface{}, error) {
			return h.execBdIssues(ctx, path, "list", "--status", "all", "--limit", "0")
		},
		timeout: func() time.Duration { return h.execTimeout },
	}
}

func (s *beadsStoreReader) retainedPrefix(path string) string {
	key, err := filepath.EvalSymlinks(filepath.Join(path, ".beads"))
	if err != nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry := s.entries[key]; entry != nil && entry.snapshot != nil {
		return entry.snapshot.prefix
	}
	return ""
}

func (s *beadsStoreReader) sweepLocked(now time.Time) {
	for key, entry := range s.entries {
		if entry.job == nil && now.Sub(entry.accessedAt) >= s.idle {
			s.removeLocked(key)
		}
	}
}

func (s *beadsStoreReader) removeLocked(key string) {
	if snapshot := s.entries[key].snapshot; snapshot != nil {
		s.bytes -= snapshot.bytes
	}
	delete(s.entries, key)
}

func (s *beadsStoreReader) oldestLocked(except string, snapshotsOnly bool) string {
	oldest := ""
	for key, entry := range s.entries {
		if key == except || (snapshotsOnly && entry.snapshot == nil) || (!snapshotsOnly && entry.job != nil) {
			continue
		}
		if oldest == "" || entry.accessedAt.Before(s.entries[oldest].accessedAt) {
			oldest = key
		}
	}
	return oldest
}

func (s *beadsStoreReader) stateLocked(path string, entry *beadsStoreEntry, snapshot *beadsStoreSnapshot) BeadsStoreState {
	state := BeadsStoreState{Path: path, ObservedGeneration: entry.observed, Pending: entry.job != nil}
	if !entry.checkedAt.IsZero() {
		state.CheckedAt = entry.checkedAt.UTC().Format(time.RFC3339Nano)
	}
	if entry.err != nil {
		state.Error = entry.err.Error()
	}
	if snapshot != nil {
		state.AvailableGeneration = snapshot.generation
		state.ReadAt = snapshot.readAt.UTC().Format(time.RFC3339Nano)
		summary := snapshotSummary(snapshot, s.now())
		state.Counts = &summary.Counts
		open := openBeadCount(summary.Counts.Status)
		state.OpenBeads = &open
		state.NewestUpdate = summary.NewestUpdated
	}
	return state
}

type optionalBdContextKey struct{}
type beadsJobContextKey struct{}

// Prefix catalog reads and background snapshots share this admission, before
// runBd's four process slots. Promotion releases the original job's background
// lease and wakes it if it is still waiting for one. No second read is spawned.
func (h *BeadsHandler) admitOptionalBd(ctx context.Context) (func(), error) {
	job, _ := ctx.Value(beadsJobContextKey{}).(*beadsStoreJob)
	optional, _ := ctx.Value(optionalBdContextKey{}).(bool)
	if job == nil && !optional {
		return func() {}, nil
	}
	var promoted <-chan struct{}
	if job != nil {
		promoted = job.promoted
	}
	select {
	case <-promoted:
		return func() {}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case h.optionalSlots <- struct{}{}:
	}
	release := sync.OnceFunc(func() { <-h.optionalSlots })
	if job != nil {
		job.owner.mu.Lock()
		if job.foreground {
			release()
		} else {
			job.releaseBackground = release
		}
		job.owner.mu.Unlock()
	}
	return release, nil
}

type storeReadDemand struct {
	foreground bool
	force      bool
	waitFresh  bool
	waitCold   bool
}

// get never waits with the reader mutex held. Warm HTTP projections use their
// exact retained pointer; a concurrent replacement cannot relabel their data.
// waitFresh is only the legacy workspace beads=wait contract. Cold projection
// requests always join the original job, regardless of subscriber cancellation.
func (s *beadsStoreReader) get(ctx context.Context, path string, demand storeReadDemand) (*beadsStoreSnapshot, BeadsStoreState, error) {
	storePath, err := filepath.EvalSymlinks(filepath.Join(path, ".beads"))
	if err != nil {
		return nil, BeadsStoreState{Path: path, Error: err.Error()}, err
	}
	key := storePath
	s.mu.Lock()
	now := s.now()
	s.sweepLocked(now)
	entry := s.entries[key]
	if entry == nil {
		if len(s.entries) >= s.maxStores {
			if oldest := s.oldestLocked("", false); oldest != "" {
				s.removeLocked(oldest)
			} else {
				s.mu.Unlock()
				err := errors.New("Beads store reader is at capacity; retry after pending stores finish")
				return nil, BeadsStoreState{Path: path, Error: err.Error()}, err
			}
		}
		entry = &beadsStoreEntry{path: path}
		s.entries[key] = entry
	}
	entry.path = path
	entry.accessedAt, entry.checkedAt = now, now
	hash, checkErr := s.fingerprint(entry.path)
	if checkErr == nil {
		entry.observed = hash
	} else if entry.err == nil {
		// A successful cheap check never clears an authoritative read error.
		entry.err = checkErr
	}
	need := entry.snapshot == nil || entry.snapshot.generation != hash || checkErr != nil || entry.err != nil || demand.force
	if entry.job == nil && need && (demand.force || !now.Before(entry.retryAt)) {
		jobCtx, cancel := context.WithTimeout(context.Background(), s.timeout())
		job := &beadsStoreJob{key: key, path: entry.path, foreground: demand.foreground,
			queuedAt: now, cancel: cancel, done: make(chan struct{}), owner: s, promoted: make(chan struct{})}
		job.ctx = context.WithValue(jobCtx, beadsJobContextKey{}, job)
		if demand.foreground {
			close(job.promoted)
		}
		entry.job = job
		s.queue = append(s.queue, job)
		job.timer = time.AfterFunc(s.timeout(), func() { s.expire(job) })
	}
	job := entry.job
	if job != nil && demand.foreground && !job.foreground {
		job.foreground = true
		close(job.promoted)
		if job.releaseBackground != nil {
			job.releaseBackground()
		}
		if job.running {
			s.background--
		}
	}
	s.dispatchLocked()
	snapshot := entry.snapshot
	state := s.stateLocked(path, entry, snapshot)
	readErr := entry.err
	s.mu.Unlock()
	if job == nil || (!demand.waitFresh && (snapshot != nil || !demand.waitCold)) {
		return snapshot, state, readErr
	}
	select {
	case <-ctx.Done():
		return snapshot, state, ctx.Err()
	case <-job.done:
	}
	s.mu.Lock()
	// This job result survives eviction and belongs to this subscriber. A
	// subsequent refresh may already have replaced the entry's snapshot.
	if job.snapshot != nil {
		snapshot = job.snapshot
	}
	entry = s.entries[key]
	if entry != nil {
		state = s.stateLocked(path, entry, snapshot)
	} else {
		// The job still supplies exact metadata if its completed entry was
		// evicted before this subscriber resumed.
		state = s.stateLocked(path, &beadsStoreEntry{observed: job.observed,
			checkedAt: job.checkedAt, err: job.err}, snapshot)
	}
	s.mu.Unlock()
	return snapshot, state, job.err
}

func (s *beadsStoreReader) dispatchLocked() {
	for s.active < 4 {
		chosen := -1
		for i, job := range s.queue {
			if job.foreground {
				chosen = i
				break
			}
		}
		if chosen < 0 && s.background < 2 && len(s.queue) > 0 {
			chosen = 0
		}
		if chosen < 0 {
			return
		}
		job := s.queue[chosen]
		s.queue = append(s.queue[:chosen], s.queue[chosen+1:]...)
		job.running = true
		s.active++
		if !job.foreground {
			s.background++
		}
		go s.refresh(job)
	}
}

func (s *beadsStoreReader) expire(job *beadsStoreJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entries[job.key]
	if entry == nil || entry.job != job || job.running {
		return // runBd owns cancellation and descendant cleanup once running.
	}
	for i, queued := range s.queue {
		if queued == job {
			s.queue = append(s.queue[:i], s.queue[i+1:]...)
			break
		}
	}
	s.finishLocked(job, nil, fmt.Errorf("bd list timed out while waiting for store read: %w", context.DeadlineExceeded))
	s.dispatchLocked()
}

func (s *beadsStoreReader) readVerified(job *beadsStoreJob) (*beadsStoreSnapshot, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if err := job.ctx.Err(); err != nil {
			return nil, fmt.Errorf("bd list timed out: %w", err)
		}
		before, beforeErr := s.fingerprint(job.path)
		// The ordinary wrapped command may repair caller-owned manifest ACLs.
		// Without a trusted before generation, its result still needs a new
		// verified pair; it cannot be certified by an after-only fingerprint.
		job.costs.readAttempts++
		issues, err := s.read(job.ctx, job.path)
		if err != nil {
			return nil, err
		}
		after, afterErr := s.fingerprint(job.path)
		switch {
		case beforeErr != nil:
			lastErr = beforeErr
		case afterErr != nil:
			lastErr = afterErr
		case before != after:
			lastErr = errors.New("Beads changed during read; waiting for a verified snapshot")
		default:
			buildStarted := time.Now()
			encoded, err := json.Marshal(issues)
			if err != nil {
				job.costs.snapshotBuild += time.Since(buildStarted)
				return nil, fmt.Errorf("measure Beads snapshot: %w", err)
			}
			snapshot := &beadsStoreSnapshot{issues: issues, byID: make(map[string]map[string]interface{}, len(issues)),
				generation: before, readAt: s.now(), bytes: int64(len(encoded)) * 4}
			if snapshot.bytes > s.maxBytes {
				job.costs.snapshotBuild += time.Since(buildStarted)
				return nil, fmt.Errorf("Beads snapshot exceeds the %d MiB reader budget", s.maxBytes>>20)
			}
			for _, issue := range issues {
				if id := beadString(issue, "id"); id != "" {
					snapshot.byID[id] = issue
					if snapshot.prefix == "" {
						snapshot.prefix = beadPrefix(id)
					}
				}
			}
			job.costs.snapshotBuild += time.Since(buildStarted)
			return snapshot, nil
		}
	}
	return nil, lastErr
}

func (s *beadsStoreReader) refresh(job *beadsStoreJob) {
	started := s.now()
	snapshot, err := s.readVerified(job)
	s.mu.Lock()
	s.active--
	if !job.foreground {
		s.background--
	}
	s.finishLocked(job, snapshot, err)
	s.dispatchLocked()
	s.mu.Unlock()
	readAt := ""
	if snapshot != nil {
		readAt = snapshot.readAt.UTC().Format(time.RFC3339Nano)
	}
	// command retains its original whole-refresh meaning, through publication
	// fingerprint/bookkeeping. Admission and process are same-job sub-stages.
	log.Printf("beads snapshot path=%q readerQueue=%s command=%s sourceCalls=%d readAttempts=%d bdAdmission=%s bdProcess=%s jsonDecode=%s snapshotBuild=%s readAt=%q error=%v",
		job.path, started.Sub(job.queuedAt), s.now().Sub(started), job.costs.sourceCalls, job.costs.readAttempts,
		job.costs.bdAdmission, job.costs.bdProcess, job.costs.jsonDecode, job.costs.snapshotBuild, readAt, err)
}

func (s *beadsStoreReader) finishLocked(job *beadsStoreJob, snapshot *beadsStoreSnapshot, err error) {
	entry := s.entries[job.key]
	job.timer.Stop()
	job.cancel()
	job.snapshot, job.err = snapshot, err
	entry.job = nil
	if err != nil {
		entry.err = err
		entry.failures++
		entry.retryAt = s.now().Add(min(30*time.Second, time.Second<<min(entry.failures-1, 5)))
	} else {
		if entry.snapshot != nil {
			s.bytes -= entry.snapshot.bytes
		}
		entry.snapshot = snapshot
		s.bytes += snapshot.bytes
		entry.err, entry.failures, entry.retryAt = nil, 0, time.Time{}
		// Evict oldest retained data, including an older snapshot whose refresh
		// is active, without dropping its job or making a duplicate read.
		for s.bytes > s.maxBytes {
			oldest := s.oldestLocked(job.key, true)
			if oldest == "" {
				break
			}
			s.bytes -= s.entries[oldest].snapshot.bytes
			s.entries[oldest].snapshot = nil
		}
	}
	entry.checkedAt = s.now()
	if hash, checkErr := s.fingerprint(entry.path); checkErr == nil {
		entry.observed = hash
	} else if entry.err == nil {
		entry.err = checkErr
	}
	job.observed, job.checkedAt = entry.observed, entry.checkedAt
	close(job.done)
}

// State checks only the listed, validated stores. It never discovers folders,
// asks for prefixes, or waits for an authoritative read.
func (h *BeadsHandler) State(w http.ResponseWriter, r *http.Request) {
	foreground := make(map[string]bool)
	for _, path := range r.URL.Query()["foreground"] {
		foreground[path] = true
	}
	states := make([]BeadsStoreState, 0)
	seen := make(map[string]bool)
	for _, requested := range r.URL.Query()["path"] {
		path, code, message := validateBeadsProjectPath(requested)
		if code != "" {
			states = append(states, BeadsStoreState{Path: requested, Error: message})
			continue
		}
		if seen[path] {
			continue
		}
		seen[path] = true
		if _, err := h.checkBeadsDirectory(path); err != nil {
			states = append(states, BeadsStoreState{Path: path, Error: err.Error()})
			continue
		}
		_, state, _ := h.stores.get(r.Context(), path, storeReadDemand{foreground: foreground[requested] || foreground[path], force: r.URL.Query().Get("refresh") == "true"})
		states = append(states, state)
	}
	core.WriteSuccess(w, map[string]interface{}{"stores": states})
}
