package api

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Gas City transcript recovery beyond a single bounded peek.
//
// `gc session peek` reads volatile tmux pane scrollback, which the supervisor
// recreates empty on restart, and `gc session logs` has no provider-native
// transcript for the local tmux/mock sessions (no session_key, no configured
// observe_paths). So a live peek alone cannot recover a session transcript
// after a supervisor restart.
//
// CHROTE owns recovery (per the substrate map). This archive captures every
// successful sanitized peek to a bounded, CHROTE-owned on-disk store keyed by
// the immutable gc-* session id. When a later peek fails or returns nothing
// (supervisor down, pane recreated empty after restart, session pruned), the
// transcript route can serve the last archived snapshot so an operator still
// recovers the most recent Gas City-owned session output.
//
// This is an operator-recovery cache, not a durable memory source: Context
// Citadel remains durable-context truth and Gas City remains orchestration
// truth. Retention is bounded by per-snapshot byte cap (the existing transcript
// output cap), one snapshot per session, and an LRU cap on archived sessions.

const (
	// gasCityArchiveMaxSessions bounds how many distinct sessions keep an
	// archived snapshot. Oldest-captured snapshots are evicted past this.
	gasCityArchiveMaxSessions = 64
	// gasCityArchiveFilePerm / DirPerm keep the archive private to the user.
	gasCityArchiveDirPerm  os.FileMode = 0o700
	gasCityArchiveFilePerm os.FileMode = 0o600
)

// gasCityTranscriptSnapshot is one archived, already-sanitized peek.
type gasCityTranscriptSnapshot struct {
	SessionID  string `json:"sessionId"`
	Alias      string `json:"alias,omitempty"`
	Template   string `json:"template,omitempty"`
	State      string `json:"state,omitempty"`
	City       string `json:"city,omitempty"`
	Lines      int    `json:"lines"`
	LineCount  int    `json:"lineCount"`
	Transcript string `json:"transcript"`
	Truncated  bool   `json:"truncated"`
	CapturedAt string `json:"capturedAt"`
}

// gasCityTranscriptArchive is a bounded on-disk store of the most recent
// sanitized peek per session. A zero dir disables archiving (store is inert).
type gasCityTranscriptArchive struct {
	dir string
	now func() time.Time
	mu  sync.Mutex
}

func newGasCityTranscriptArchive(dir string) *gasCityTranscriptArchive {
	return &gasCityTranscriptArchive{dir: strings.TrimSpace(dir), now: time.Now}
}

func (a *gasCityTranscriptArchive) enabled() bool {
	return a != nil && a.dir != ""
}

// snapshotPath maps an already-validated gc-* session id to its archive file.
// The id is validated by validateGasCitySessionID before this is reached, so it
// cannot contain path separators or traversal; we still Base-clean defensively.
func (a *gasCityTranscriptArchive) snapshotPath(sessionID string) string {
	return filepath.Join(a.dir, filepath.Base(sessionID)+".json")
}

// save best-effort writes the snapshot and enforces the LRU session cap. Any
// error is returned for logging but must never break the live transcript path.
func (a *gasCityTranscriptArchive) save(snapshot gasCityTranscriptSnapshot) error {
	if !a.enabled() {
		return nil
	}
	if snapshot.SessionID == "" {
		return errors.New("transcript archive: empty session id")
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := os.MkdirAll(a.dir, gasCityArchiveDirPerm); err != nil {
		return err
	}
	snapshot.CapturedAt = a.now().UTC().Format(time.RFC3339)
	body, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	// Atomic replace so a reader never sees a partial snapshot.
	target := a.snapshotPath(snapshot.SessionID)
	tmp, err := os.CreateTemp(a.dir, "."+filepath.Base(snapshot.SessionID)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(gasCityArchiveFilePerm); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		return err
	}
	a.evictLocked()
	return nil
}

// load returns the last archived snapshot for a session, if any.
func (a *gasCityTranscriptArchive) load(sessionID string) (gasCityTranscriptSnapshot, bool) {
	if !a.enabled() || sessionID == "" {
		return gasCityTranscriptSnapshot{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	body, err := os.ReadFile(a.snapshotPath(sessionID))
	if err != nil {
		return gasCityTranscriptSnapshot{}, false
	}
	var snapshot gasCityTranscriptSnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return gasCityTranscriptSnapshot{}, false
	}
	if snapshot.SessionID == "" {
		snapshot.SessionID = sessionID
	}
	return snapshot, true
}

// evictLocked keeps at most gasCityArchiveMaxSessions snapshots, removing the
// oldest by file modification time. Caller holds a.mu.
func (a *gasCityTranscriptArchive) evictLocked() {
	entries, err := os.ReadDir(a.dir)
	if err != nil {
		return
	}
	type fileMeta struct {
		path    string
		modTime time.Time
	}
	var snapshots []fileMeta
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		snapshots = append(snapshots, fileMeta{
			path:    filepath.Join(a.dir, entry.Name()),
			modTime: info.ModTime(),
		})
	}
	if len(snapshots) <= gasCityArchiveMaxSessions {
		return
	}
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].modTime.Before(snapshots[j].modTime)
	})
	for _, meta := range snapshots[:len(snapshots)-gasCityArchiveMaxSessions] {
		os.Remove(meta.path)
	}
}
