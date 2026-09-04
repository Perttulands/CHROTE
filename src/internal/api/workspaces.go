// Package api provides HTTP handlers for the API.
//
// This file answers one question for three surfaces: which folders on this
// host are workspaces? The Agents rail lists them to ask what an agent there
// would see, the Beads rail reads the ones with a store, and the launcher
// offers them as suggestions. A workspace is any folder a live session runs
// in, any configured Beads project, and any git root or Beads store under a
// configured root or a launchable user's home, to a fixed depth. The list is
// computed on request from the filesystem and the tmux inventory: nothing is
// cached and nothing is watched, which is what makes it true.
package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/chrote/server/internal/core"
)

// Why a folder is on the list. One folder can be there for several reasons.
const (
	workspaceSourceSession = "session"
	workspaceSourceBeads   = "beads"
	workspaceSourceGit     = "git"
	workspaceSourceStore   = "store"
)

// The order sources are reported in, so two requests over one tree read alike.
var workspaceSourceOrder = []string{workspaceSourceSession, workspaceSourceBeads, workspaceSourceGit, workspaceSourceStore}

// How far under a root the walk looks. Three levels is where a project's own
// folder sits under a root such as /srv or a home; deeper is the project's
// business.
const workspaceWalkDepth = 3

// How many stores are asked at once. Each is one bd process.
const workspaceProbeFanOut = 8

// Workspace is one folder worth starting an agent in, reading a store from, or
// asking what an agent there would see.
type Workspace struct {
	Path string `json:"path"`
	// session, beads, git, store: every reason this folder is on the list.
	Sources []string `json:"sources"`
	// The live sessions running in this folder.
	Sessions []string `json:"sessions"`
	// The prefix the store's Bead ids carry, when the folder holds a store
	// and bd could say.
	BeadsPrefix string `json:"beadsPrefix,omitempty"`
	// How many Beads in the store are not closed. Absent when the folder
	// holds no store or the counts projection is not ready.
	OpenBeads *int `json:"openBeads,omitempty"`
	// The complete status and type projection, absent until its manifest-keyed
	// cache is filled.
	BeadsCounts *BeadsCounts `json:"beadsCounts,omitempty"`
	// The newest update in the complete store projection.
	BeadsNewestUpdate string `json:"beadsNewestUpdate,omitempty"`
	// Why the store projection could not be read. The store remains listed.
	BeadsError string `json:"beadsError,omitempty"`
	// True only on the first response that started a background projection.
	BeadsSummaryPending bool `json:"beadsSummaryPending,omitempty"`
	// How many instruction files the folder owns itself.
	Instructions int `json:"instructions"`
	// When a session here last saw input or output, RFC 3339 UTC. Empty
	// when no session runs here.
	LastActivity string `json:"lastActivity,omitempty"`
}

// WorkspacesHandler computes the workspace list.
type WorkspacesHandler struct {
	// The live sessions, with the folder each runs in.
	sessions func() []core.Session
	// The configured roots.
	roots func() []string
	// The Unix users the launcher can start a session as; their homes are
	// roots too.
	users func() []string
	// How a Unix user becomes a home directory.
	homeForUser func(string) (string, error)
	// The configured Beads projects.
	beadsWorkspaces func() []string
	// Paths nothing under is listed, however it was found.
	skipped []string
	// The store validation and the bd calls, shared with the Beads routes.
	beads *BeadsHandler
}

// NewWorkspacesHandler creates the handler over the host's own sessions,
// accounts and configuration.
func NewWorkspacesHandler(tmux *TmuxHandler, beads *BeadsHandler) *WorkspacesHandler {
	return &WorkspacesHandler{
		sessions:        tmux.liveSessions,
		roots:           core.GetAllowedRoots,
		users:           configuredTerminalUsers,
		homeForUser:     unixUserHomeDir,
		beadsWorkspaces: configuredBeadsWorkspaces,
		skipped:         []string{"/tmp"},
		beads:           beads,
	}
}

// RegisterRoutes registers the workspaces route on the given mux.
func (h *WorkspacesHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/workspaces", h.List)
}

// List handles GET /api/workspaces: every workspace, ordered by last session
// activity and then by path. With beads=1 cached store summaries are included
// and misses refresh in the background, so the first response never waits for
// bd. beads=wait is the browser's one follow-up after it has painted the rail.
func (h *WorkspacesHandler) List(w http.ResponseWriter, r *http.Request) {
	beadsMode := r.URL.Query().Get("beads")
	core.WriteJSON(w, http.StatusOK, h.list(beadsMode != "", beadsMode == "wait"))
}

type workspaceEntry struct {
	Workspace
	sources map[string]bool
}

// workspaceList collects folders under their resolved paths, so a folder
// reached through a symlink and through its real path is one entry.
type workspaceList struct {
	handler *WorkspacesHandler
	entries map[string]*workspaceEntry
}

// add records why a folder is on the list and answers with its entry, or nil
// when the folder is not a directory or sits where nothing is listed.
func (l *workspaceList) add(path, source string) *workspaceEntry {
	resolved, ok := l.handler.resolveWorkspace(path)
	if !ok {
		return nil
	}
	entry := l.entries[resolved]
	if entry == nil {
		entry = &workspaceEntry{sources: map[string]bool{}}
		entry.Path = resolved
		entry.Sessions = []string{}
		l.entries[resolved] = entry
	}
	entry.sources[source] = true
	return entry
}

// resolveWorkspace is the one path a folder is known by: symlinks followed,
// and nothing under a skipped path.
func (h *WorkspacesHandler) resolveWorkspace(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" || !filepath.IsAbs(path) {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", false
	}
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		return "", false
	}
	if isPathUnder(resolved, h.skipped) {
		return "", false
	}
	return resolved, true
}

func (h *WorkspacesHandler) list(probeStores bool, waitForStores ...bool) []Workspace {
	list := &workspaceList{handler: h, entries: map[string]*workspaceEntry{}}

	for _, session := range h.sessions() {
		entry := list.add(session.CWD, workspaceSourceSession)
		if entry == nil {
			continue
		}
		entry.Sessions = append(entry.Sessions, session.Name)
		if session.Activity > entry.LastActivity {
			entry.LastActivity = session.Activity
		}
	}
	for _, project := range h.beadsWorkspaces() {
		if entry := list.add(project, workspaceSourceBeads); entry != nil && h.holdsStore(entry.Path) {
			entry.sources[workspaceSourceStore] = true
		}
	}
	for _, root := range h.walkRoots() {
		h.walk(list, root, 0)
	}

	entries := make([]*workspaceEntry, 0, len(list.entries))
	for _, entry := range list.entries {
		entries = append(entries, entry)
	}
	if probeStores {
		wait := len(waitForStores) > 0 && waitForStores[0]
		h.probeStores(entries, wait)
	}

	found := make([]Workspace, 0, len(entries))
	for _, entry := range entries {
		sort.Strings(entry.Sessions)
		entry.Instructions = countInstructionFiles(entry.Path)
		entry.Sources = []string{}
		for _, source := range workspaceSourceOrder {
			if entry.sources[source] {
				entry.Sources = append(entry.Sources, source)
			}
		}
		found = append(found, entry.Workspace)
	}
	// The most recently active folder first; among the rest, the path. An
	// empty activity sorts after every timestamp, so folders without a
	// session follow the ones with one.
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].LastActivity != found[j].LastActivity {
			return found[i].LastActivity > found[j].LastActivity
		}
		return found[i].Path < found[j].Path
	})
	return found
}

// walkRoots is every configured root and every launchable user's home, each
// resolved, none of them under another: a root inside a root would only walk
// the same folders twice.
func (h *WorkspacesHandler) walkRoots() []string {
	candidates := append([]string{}, h.roots()...)
	for _, unixUser := range h.users() {
		if home, err := h.homeForUser(unixUser); err == nil {
			candidates = append(candidates, home)
		}
	}
	resolved := []string{}
	for _, candidate := range candidates {
		if root, ok := h.resolveWorkspace(candidate); ok {
			resolved = append(resolved, root)
		}
	}
	sort.Strings(resolved)
	roots := []string{}
	for _, root := range resolved {
		if !isPathUnder(root, roots) {
			roots = append(roots, root)
		}
	}
	return roots
}

// walk lists every git root and Beads store from dir down to the depth limit.
// Dot-directories, node_modules and worktrees are neither listed nor entered:
// what they hold is tooling, dependencies, or a checkout of a project that is
// already on the list.
func (h *WorkspacesHandler) walk(list *workspaceList, dir string, depth int) {
	if isWorktreePath(dir) {
		return
	}
	if h.holdsStore(dir) {
		list.add(dir, workspaceSourceStore)
	}
	if holdsGit(dir) {
		list.add(dir, workspaceSourceGit)
	}
	if depth == workspaceWalkDepth {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		// A symlinked directory is not entered: the folder it points at is
		// walked where it really is, or is deliberately outside the roots.
		if !entry.IsDir() || skipWalkName(entry.Name()) {
			continue
		}
		h.walk(list, filepath.Join(dir, entry.Name()), depth+1)
	}
}

func skipWalkName(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules" || name == "worktrees"
}

func isWorktreePath(dir string) bool {
	path := dir + string(os.PathSeparator)
	return strings.Contains(path, "/worktrees/") || strings.Contains(path, "/.worktrees/")
}

func holdsGit(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}

func (h *WorkspacesHandler) holdsStore(dir string) bool {
	_, err := h.beads.checkBeadsDirectory(dir)
	return err == nil
}

func openBeadCount(counts BeadsStatusCounts) int {
	return counts.Open + counts.InProgress + counts.Blocked + counts.Deferred
}

// probeStores reads every cache hit and starts each miss, a bounded number at
// a time. Each goroutine writes only its own entry. Waiting is reserved for the
// browser's one follow-up request after the first response has painted.
func (h *WorkspacesHandler) probeStores(entries []*workspaceEntry, waitForSummary bool) {
	var group sync.WaitGroup
	slots := make(chan struct{}, workspaceProbeFanOut)
	for _, entry := range entries {
		if !entry.sources[workspaceSourceStore] {
			continue
		}
		group.Add(1)
		go func(entry *workspaceEntry) {
			defer group.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			summary, ready, pending, err := h.beads.cachedStoreSummary(entry.Path, waitForSummary)
			entry.BeadsSummaryPending = pending
			if err != nil {
				entry.BeadsError = err.Error()
				return
			}
			if !ready {
				return
			}
			entry.BeadsPrefix = summary.Prefix
			entry.BeadsCounts = &summary.Counts
			entry.BeadsNewestUpdate = summary.NewestUpdated
			open := openBeadCount(summary.Counts.Status)
			entry.OpenBeads = &open
		}(entry)
	}
	group.Wait()
}
