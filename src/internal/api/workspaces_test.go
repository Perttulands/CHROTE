package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func (tree *workspaceTestTree) list(t *testing.T) []Workspace {
	t.Helper()
	recorder := httptest.NewRecorder()
	tree.handler.List(recorder, httptest.NewRequest(http.MethodGet, "/api/workspaces", nil))
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

	found := tree.list(t)

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
	if strings.Join(storeEntry.Sources, ",") != "store" || storeEntry.OpenBeads != nil {
		t.Fatalf("store = %+v, want a store bd could not be asked about", storeEntry)
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

	found := tree.list(t)

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

	found := tree.list(t)

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
}

func TestWorkspacesAskBdAgainForThePrefixOfAQuietStore(t *testing.T) {
	tree := newWorkspaceTestTree(t)
	store := filepath.Join(tree.root, "store")
	makeValidBeadsWorkspace(t, store)
	makeSequencedBdCommand(t, `{"issues":[]}`, `[{"id":"chr-9zz","status":"closed"}]`)
	tree.handler.beads = NewBeadsHandler()

	found := tree.list(t)

	entry, ok := workspaceByPath(found, store)
	if !ok || entry.BeadsPrefix != "chr" || entry.OpenBeads == nil || *entry.OpenBeads != 0 {
		t.Fatalf("store = %+v, want the prefix of a closed Bead and nothing open", entry)
	}
}
