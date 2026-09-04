package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/chrote/server/internal/core"
)

// A tree with one of everything the walk decides about, under a base that is
// not itself skipped: t.TempDir sits under /tmp, which the real handler skips,
// so the skip list here names a folder inside the tree instead.
type workspaceTestTree struct {
	base    string
	root    string
	home    string
	handler *WorkspacesHandler
}

func newWorkspaceTestTree(t *testing.T) *workspaceTestTree {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	root := filepath.Join(base, "srv")
	home := filepath.Join(base, "home", "operator")
	mkdirAll(t, root)
	mkdirAll(t, home)
	beads := &BeadsHandler{bdCommand: filepath.Join(base, "no-such-bd"), execTimeout: 5 * time.Second}
	return &workspaceTestTree{
		base: base,
		root: root,
		home: home,
		handler: &WorkspacesHandler{
			sessions:        func() []core.Session { return nil },
			roots:           func() []string { return []string{root} },
			users:           func() []string { return []string{"operator"} },
			homeForUser:     func(string) (string, error) { return home, nil },
			beadsWorkspaces: func() []string { return nil },
			skipped:         []string{filepath.Join(base, "tmp")},
			beads:           beads,
		},
	}
}

func (tree *workspaceTestTree) gitRoot(t *testing.T, path string) string {
	t.Helper()
	mkdirAll(t, filepath.Join(path, ".git"))
	return path
}

func (tree *workspaceTestTree) list(t *testing.T, query string) []Workspace {
	t.Helper()
	recorder := httptest.NewRecorder()
	tree.handler.List(recorder, httptest.NewRequest(http.MethodGet, "/api/workspaces"+query, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var found []Workspace
	if err := json.Unmarshal(recorder.Body.Bytes(), &found); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return found
}

func workspacePaths(found []Workspace) []string {
	paths := make([]string, 0, len(found))
	for _, workspace := range found {
		paths = append(paths, workspace.Path)
	}
	return paths
}

func workspaceByPath(found []Workspace, path string) (Workspace, bool) {
	for _, workspace := range found {
		if workspace.Path == path {
			return workspace, true
		}
	}
	return Workspace{}, false
}

func TestWorkspacesWalkListsGitRootsAndStoresToDepthThreeAndNothingExcluded(t *testing.T) {
	tree := newWorkspaceTestTree(t)
	repo := tree.gitRoot(t, filepath.Join(tree.root, "repo"))
	store := filepath.Join(tree.root, "store")
	makeValidBeadsWorkspace(t, store)
	both := tree.gitRoot(t, filepath.Join(tree.root, "group", "both"))
	makeValidBeadsWorkspace(t, both)
	deepest := tree.gitRoot(t, filepath.Join(tree.root, "one", "two", "three"))
	homeRepo := tree.gitRoot(t, filepath.Join(tree.home, "work", "notes"))
	writeFile(t, filepath.Join(repo, "CLAUDE.md"), "# repo\n")
	writeFile(t, filepath.Join(repo, ".claude", "settings.json"), "{}\n")

	tooDeep := tree.gitRoot(t, filepath.Join(tree.root, "one", "two", "three", "four"))
	hidden := tree.gitRoot(t, filepath.Join(tree.root, ".hidden", "repo"))
	dependency := tree.gitRoot(t, filepath.Join(tree.root, "node_modules", "pkg"))
	worktree := tree.gitRoot(t, filepath.Join(tree.root, "worktrees", "wt"))
	dotWorktree := tree.gitRoot(t, filepath.Join(tree.root, "repo", ".worktrees", "wt"))
	scratch := tree.gitRoot(t, filepath.Join(tree.base, "tmp", "scratch"))
	partial := filepath.Join(tree.root, "partial")
	makePartialBeadsDirectory(t, partial)
	mkdirAll(t, filepath.Join(tree.root, "plain"))
	// A root that is itself under a skipped path contributes nothing.
	tree.handler.roots = func() []string { return []string{tree.root, filepath.Join(tree.base, "tmp")} }

	found := tree.list(t, "")

	want := []string{homeRepo, both, deepest, repo, store}
	if got := workspacePaths(found); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("workspaces = %v, want %v", got, want)
	}
	for _, excluded := range []string{tooDeep, hidden, dependency, worktree, dotWorktree, scratch, partial, filepath.Join(tree.root, "plain")} {
		if _, ok := workspaceByPath(found, excluded); ok {
			t.Fatalf("workspaces = %v, want %s excluded", workspacePaths(found), excluded)
		}
	}

	repoEntry, _ := workspaceByPath(found, repo)
	if strings.Join(repoEntry.Sources, ",") != "git" || repoEntry.Instructions != 2 {
		t.Fatalf("repo = %+v, want a git root with two instruction files", repoEntry)
	}
	bothEntry, _ := workspaceByPath(found, both)
	if strings.Join(bothEntry.Sources, ",") != "git,store" {
		t.Fatalf("both = %+v, want git and store", bothEntry)
	}
	storeEntry, _ := workspaceByPath(found, store)
	if strings.Join(storeEntry.Sources, ",") != "store" || storeEntry.OpenBeads != nil || storeEntry.BeadsPrefix != "" {
		t.Fatalf("store = %+v, want a store nobody asked bd about", storeEntry)
	}
}

func TestWorkspacesOrderBySessionActivityThenPathAndMergeOnResolvedPath(t *testing.T) {
	tree := newWorkspaceTestTree(t)
	alpha := tree.gitRoot(t, filepath.Join(tree.root, "alpha"))
	beta := tree.gitRoot(t, filepath.Join(tree.root, "beta"))
	elsewhere := filepath.Join(tree.base, "elsewhere")
	mkdirAll(t, elsewhere)
	link := filepath.Join(tree.base, "link-to-beta")
	if err := os.Symlink(beta, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	configured := filepath.Join(tree.base, "configured")
	makeValidBeadsWorkspace(t, configured)
	tree.handler.beadsWorkspaces = func() []string { return []string{configured, filepath.Join(tree.base, "missing")} }
	tree.handler.sessions = func() []core.Session {
		return []core.Session{
			{Name: "old", CWD: elsewhere, Activity: "2026-09-03T10:00:00Z"},
			{Name: "newest", CWD: link, Activity: "2026-09-03T12:00:00Z"},
			{Name: "earlier", CWD: beta, Activity: "2026-09-03T11:00:00Z"},
			{Name: "nowhere", CWD: filepath.Join(tree.base, "gone"), Activity: "2026-09-03T13:00:00Z"},
			{Name: "scratch", CWD: filepath.Join(tree.base, "tmp", "x"), Activity: "2026-09-03T14:00:00Z"},
		}
	}
	mkdirAll(t, filepath.Join(tree.base, "tmp", "x"))

	found := tree.list(t, "")

	want := []string{beta, elsewhere, configured, alpha}
	if got := workspacePaths(found); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("workspaces = %v, want %v", got, want)
	}
	betaEntry, _ := workspaceByPath(found, beta)
	if strings.Join(betaEntry.Sessions, ",") != "earlier,newest" || betaEntry.LastActivity != "2026-09-03T12:00:00Z" {
		t.Fatalf("beta = %+v, want both sessions under the resolved path and the latest activity", betaEntry)
	}
	if strings.Join(betaEntry.Sources, ",") != "session,git" {
		t.Fatalf("beta sources = %v, want session then git", betaEntry.Sources)
	}
	elsewhereEntry, _ := workspaceByPath(found, elsewhere)
	if strings.Join(elsewhereEntry.Sources, ",") != "session" {
		t.Fatalf("elsewhere = %+v, want a folder listed only because a session runs there", elsewhereEntry)
	}
	configuredEntry, _ := workspaceByPath(found, configured)
	if strings.Join(configuredEntry.Sources, ",") != "beads,store" {
		t.Fatalf("configured = %+v, want beads and store", configuredEntry)
	}
}

func TestWorkspacesAskBdForEachStoresPrefixAndOpenCount(t *testing.T) {
	tree := newWorkspaceTestTree(t)
	store := filepath.Join(tree.root, "store")
	makeValidBeadsWorkspace(t, store)
	_, argsPath := makeSequencedBdCommand(t,
		`{"issues":[{"id":"chr-1ab","status":"open"},{"id":"chr-2cd","status":"in_progress"},{"id":"chr-3ef","status":"closed"}]}`,
	)
	tree.handler.beads = NewBeadsHandler()

	found := tree.list(t, "?beads=wait")

	entry, ok := workspaceByPath(found, store)
	if !ok || entry.BeadsPrefix != "chr" || entry.OpenBeads == nil || *entry.OpenBeads != 2 {
		t.Fatalf("store = %+v, want prefix chr and two open Beads", entry)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read bd args: %v", err)
	}
	if calls := strings.Count(string(args), "\n"); calls != 1 {
		t.Fatalf("bd was called %d times, want once when the open list carries the prefix:\n%s", calls, args)
	}
	if strings.TrimSpace(string(args)) != "--json list --status all --limit 0" {
		t.Fatalf("bd args = %q, want the complete projection", strings.TrimSpace(string(args)))
	}
}

func TestWorkspacesFirstBeadsResponseDoesNotWaitForTheProjection(t *testing.T) {
	tree := newWorkspaceTestTree(t)
	store := filepath.Join(tree.root, "store")
	makeValidBeadsWorkspace(t, store)

	dir := t.TempDir()
	releasePath := filepath.Join(dir, "release")
	if err := syscall.Mkfifo(releasePath, 0o600); err != nil {
		t.Fatalf("create release pipe: %v", err)
	}
	scriptPath := filepath.Join(dir, "bd")
	script := "#!/bin/sh\n" +
		"read release < \"$BD_RELEASE_PIPE\"\n" +
		"printf '%s' '[{\"id\":\"chr-1aa\",\"status\":\"open\",\"issue_type\":\"task\"}]'\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write blocking bd: %v", err)
	}
	t.Setenv("BD_RELEASE_PIPE", releasePath)
	t.Setenv("CHROTE_BD_COMMAND", scriptPath)
	tree.handler.beads = NewBeadsHandler()

	found := tree.list(t, "?beads=1")
	entry, ok := workspaceByPath(found, store)
	if !ok || !entry.BeadsSummaryPending || entry.BeadsCounts != nil || entry.OpenBeads != nil {
		t.Fatalf("first store response = %+v, want the store with a pending projection", entry)
	}

	release, err := os.OpenFile(releasePath, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open release pipe: %v", err)
	}
	if _, err := release.WriteString("continue\n"); err != nil {
		t.Fatalf("release projection: %v", err)
	}
	if err := release.Close(); err != nil {
		t.Fatalf("close release pipe: %v", err)
	}

	found = tree.list(t, "?beads=wait")
	entry, ok = workspaceByPath(found, store)
	if !ok || entry.BeadsSummaryPending || entry.BeadsCounts == nil || entry.OpenBeads == nil || *entry.OpenBeads != 1 {
		t.Fatalf("follow-up store response = %+v, want the completed projection", entry)
	}
}

func TestWorkspacesReadsThePrefixAndCountsOfAQuietStore(t *testing.T) {
	tree := newWorkspaceTestTree(t)
	store := filepath.Join(tree.root, "store")
	makeValidBeadsWorkspace(t, store)
	_, argsPath := makeSequencedBdCommand(t, `[{"id":"chr-9zz","status":"closed","issue_type":"task"}]`)
	tree.handler.beads = NewBeadsHandler()

	found := tree.list(t, "?beads=wait")

	entry, ok := workspaceByPath(found, store)
	if !ok || entry.BeadsPrefix != "chr" || entry.OpenBeads == nil || *entry.OpenBeads != 0 {
		t.Fatalf("store = %+v, want the prefix of a closed Bead and nothing open", entry)
	}
	if calls := readSequencedBdCalls(t, argsPath); len(calls) != 1 || calls[0] != "--json list --status all --limit 0" {
		t.Fatalf("bd calls = %#v, want one complete projection", calls)
	}
}

func TestWorkspacesStoreProjectionIsCachedUntilTheManifestChanges(t *testing.T) {
	tree := newWorkspaceTestTree(t)
	store := filepath.Join(tree.root, "store")
	makeValidBeadsWorkspace(t, store)
	first := `[
		{"id":"chr-1aa","status":"open","issue_type":"epic","updated_at":"2026-09-01T00:00:00Z"},
		{"id":"chr-2bb","status":"in_progress","issue_type":"task","updated_at":"2026-09-02T00:00:00Z"},
		{"id":"chr-3cc","status":"open","issue_type":"bug","dependencies":[{"depends_on_id":"chr-2bb","type":"blocks"}]},
		{"id":"chr-4dd","status":"open","issue_type":"feature","defer_until":"2099-01-01T00:00:00Z"},
		{"id":"chr-5ee","status":"closed","issue_type":"decision"}
	]`
	second := `[{"id":"chr-5ee","status":"closed","issue_type":"decision","updated_at":"2026-09-03T00:00:00Z"}]`
	_, argsPath := makeSequencedBdCommand(t, first, second)
	tree.handler.beads = NewBeadsHandler()

	found := tree.list(t, "?beads=wait")
	entry, ok := workspaceByPath(found, store)
	if !ok || entry.BeadsCounts == nil {
		t.Fatalf("store = %+v, want its complete counts projection", entry)
	}
	wantStatus := BeadsStatusCounts{Open: 1, InProgress: 1, Blocked: 1, Closed: 1, Deferred: 1}
	if entry.BeadsCounts.Status != wantStatus {
		t.Fatalf("status counts = %+v, want %+v", entry.BeadsCounts.Status, wantStatus)
	}
	wantTypes := BeadsTypeCounts{Epic: 1, Task: 1, Bug: 1, Feature: 1, Decision: 1}
	if entry.BeadsCounts.Type != wantTypes {
		t.Fatalf("type counts = %+v, want %+v", entry.BeadsCounts.Type, wantTypes)
	}
	if entry.BeadsNewestUpdate != "2026-09-02T00:00:00Z" || entry.OpenBeads == nil || *entry.OpenBeads != 4 {
		t.Fatalf("store = %+v, want newest update and four non-closed Beads", entry)
	}

	_ = tree.list(t, "?beads=wait")
	if calls := readSequencedBdCalls(t, argsPath); len(calls) != 1 {
		t.Fatalf("bd was called %d times without a manifest change, want once: %#v", len(calls), calls)
	}

	manifest := filepath.Join(store, ".beads", "embeddeddolt", "test", ".dolt", "noms", "manifest")
	if err := os.WriteFile(manifest, []byte("manifest after a write\n"), 0600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	found = tree.list(t, "?beads=wait")
	entry, _ = workspaceByPath(found, store)
	if entry.OpenBeads == nil || *entry.OpenBeads != 0 || entry.BeadsCounts == nil || entry.BeadsCounts.Status.Closed != 1 {
		t.Fatalf("store after manifest change = %+v, want the refreshed projection", entry)
	}
	if calls := readSequencedBdCalls(t, argsPath); len(calls) != 2 {
		t.Fatalf("bd was called %d times after a manifest change, want twice: %#v", len(calls), calls)
	}
}

// Reading a store rewrites its Dolt manifest with the same bytes and a new
// mtime. A time-keyed cache expired on the very read that filled it, so the
// live service answered every request pending and re-spawned bd across every
// store. The projection has to survive a read and only a write may end it.
func TestWorkspacesStoreProjectionSurvivesTheManifestRewriteThatReadingCauses(t *testing.T) {
	tree := newWorkspaceTestTree(t)
	store := filepath.Join(tree.root, "store")
	makeValidBeadsWorkspace(t, store)
	manifest := filepath.Join(store, ".beads", "embeddeddolt", "test", ".dolt", "noms", "manifest")

	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	callPath := filepath.Join(dir, "calls.txt")
	scriptPath := filepath.Join(dir, "bd")
	// Every call rewrites the manifest with identical bytes at a later time,
	// which is what Dolt does when bd only reads the store.
	script := "#!/bin/sh\n" +
		"printf '%s ' \"$@\" >> \"$BD_ARGS_FILE\"\n" +
		"printf '\\n' >> \"$BD_ARGS_FILE\"\n" +
		"n=$(cat \"$BD_CALL_FILE\" 2>/dev/null || echo 0)\n" +
		"n=$((n+1))\n" +
		"printf '%s' \"$n\" > \"$BD_CALL_FILE\"\n" +
		"cat \"$BD_MANIFEST\" > \"$BD_MANIFEST.copy\"\n" +
		"mv \"$BD_MANIFEST.copy\" \"$BD_MANIFEST\"\n" +
		"touch -t \"20300101000${n}\" \"$BD_MANIFEST\"\n" +
		"printf '%s' '[{\"id\":\"chr-1aa\",\"status\":\"open\",\"issue_type\":\"task\"}]'\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		t.Fatalf("write fake bd command: %v", err)
	}
	t.Setenv("BD_ARGS_FILE", argsPath)
	t.Setenv("BD_CALL_FILE", callPath)
	t.Setenv("BD_MANIFEST", manifest)
	t.Setenv("CHROTE_BD_COMMAND", scriptPath)
	tree.handler.beads = NewBeadsHandler()

	before, err := os.Stat(manifest)
	if err != nil {
		t.Fatalf("stat manifest: %v", err)
	}

	found := tree.list(t, "?beads=wait")
	entry, ok := workspaceByPath(found, store)
	if !ok || entry.BeadsCounts == nil || entry.OpenBeads == nil || *entry.OpenBeads != 1 {
		t.Fatalf("store after the first wait = %+v, want its counts", entry)
	}

	after, err := os.Stat(manifest)
	if err != nil {
		t.Fatalf("stat manifest after the read: %v", err)
	}
	if after.ModTime().Equal(before.ModTime()) {
		t.Fatal("the fake bd did not rewrite the manifest, so this test proves nothing")
	}

	// The projection was filled, so an ordinary beads=1 request must answer
	// from the cache: counts present, nothing pending, and no second bd.
	found = tree.list(t, "?beads=1")
	entry, ok = workspaceByPath(found, store)
	if !ok || entry.BeadsSummaryPending || entry.BeadsCounts == nil || entry.BeadsPrefix != "chr" {
		t.Fatalf("store after the read rewrote its manifest = %+v, want the cached projection", entry)
	}
	if entry.OpenBeads == nil || *entry.OpenBeads != 1 {
		t.Fatalf("open count = %+v, want the cached one", entry.OpenBeads)
	}
	if calls := readSequencedBdCalls(t, argsPath); len(calls) != 1 {
		t.Fatalf("bd was called %d times when only a read had happened, want once: %#v", len(calls), calls)
	}

	// A write leaves different bytes behind, and only that ends the entry.
	if err := os.WriteFile(manifest, []byte("manifest after a write\n"), 0600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	found = tree.list(t, "?beads=wait")
	entry, _ = workspaceByPath(found, store)
	if entry.BeadsCounts == nil || entry.OpenBeads == nil || *entry.OpenBeads != 1 {
		t.Fatalf("store after a write = %+v, want the recomputed projection", entry)
	}
	if calls := readSequencedBdCalls(t, argsPath); len(calls) != 2 {
		t.Fatalf("bd was called %d times after a write, want twice: %#v", len(calls), calls)
	}
}
