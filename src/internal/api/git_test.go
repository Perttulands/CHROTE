package api

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitArgumentsNameOnlyTheirOwnRepository(t *testing.T) {
	args := gitArguments("/corpus", "log", "--name-only")

	if len(args) < 2 || args[0] != "-c" || args[1] != "safe.directory=/corpus" {
		t.Fatalf("argv = %q, want it to lead with -c safe.directory for the repository", args)
	}
	for _, arg := range args {
		if strings.Contains(arg, "safe.directory=*") {
			t.Fatalf("argv = %q, want the repository named rather than every repository on the host", args)
		}
	}
	if args[len(args)-2] != "log" || args[len(args)-1] != "--name-only" {
		t.Fatalf("argv = %q, want the caller's command last", args)
	}
	if !containsPair(args, "-C", "/corpus") {
		t.Fatalf("argv = %q, want git run inside the repository", args)
	}
}

func containsPair(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}

func TestGitSubcommandSkipsConfiguration(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "a plain command", args: []string{"log", "--name-only"}, want: "log"},
		{name: "behind -c options", args: []string{"-c", "user.name=T", "commit", "-m", "x"}, want: "commit"},
		{name: "nothing to name", args: nil, want: "command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gitSubcommand(tt.args); got != tt.want {
				t.Fatalf("gitSubcommand(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

// requireDifferentOwner makes git treat the repository as somebody else's, the
// way it treats the operator's corpus when the server runs as its own account,
// and skips the test when this git cannot pretend that.
func requireDifferentOwner(t *testing.T, repository string) {
	t.Helper()
	t.Setenv("GIT_TEST_ASSUME_DIFFERENT_OWNER", "1")
	cmd := exec.Command("git", "--no-pager", "-C", repository, "log", "-n", "1", "--format=%H")
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
	)
	if err := cmd.Run(); err == nil {
		t.Skip("this git does not refuse a repository owned by another account")
	}
}

// A repository owned by another account is the live case: git refuses it, and
// without safe.directory the routes reported the refusal as an empty corpus.
func TestLibraryReadsARepositoryOwnedByAnotherAccount(t *testing.T) {
	root := newLibraryCorpus(t)
	requireDifferentOwner(t, root)
	handler := newLibraryHandlerForTest(t, LibraryConfig{Root: root})

	page := decodeLibrary[LibraryPageResponse](t,
		libraryRequest(t, handler, http.MethodGet, "/api/library/page?path=preferences/workflow.md", ""))
	if page.Error != "" {
		t.Fatalf("page error = %q, want git to have read the corpus", page.Error)
	}
	if len(page.History) == 0 || page.Updated == "" {
		t.Fatalf("page = %#v, want the history of a corpus git was told is safe", page)
	}

	changes := decodeLibrary[LibraryChangesResponse](t,
		libraryRequest(t, handler, http.MethodGet, "/api/library/changes", ""))
	if changes.Error != "" || len(changes.Changes) == 0 {
		t.Fatalf("changes = %#v, want the commits the corpus holds", changes)
	}

	pages := decodeLibrary[LibraryPagesResponse](t,
		libraryRequest(t, handler, http.MethodGet, "/api/library/pages?shelf=preferences", ""))
	if pages.Error != "" || len(pages.Pages) == 0 || pages.Pages[0].Updated == "" {
		t.Fatalf("pages = %#v, want the shelf with its dates", pages)
	}
}

func TestFilesHandlerDiffsARepositoryOwnedByAnotherAccount(t *testing.T) {
	repository := newGitRepository(t)
	path := filepath.Join(repository, "modified.txt")
	writeFileFixture(t, path, "one\n")
	gitCommitAll(t, repository, "init")
	writeFileFixture(t, path, "two\n")
	requireDifferentOwner(t, repository)

	rec, response := diffRequest(t, newDiffHandler(repository), path)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if response.Error != "" {
		t.Fatalf("error = %q, want git to have diffed the repository", response.Error)
	}
	if !strings.Contains(response.Diff, "+two") {
		t.Fatalf("diff = %q, want the change against HEAD", response.Diff)
	}
}

// When git does refuse, the refusal is the answer: an empty history that says
// nothing is the fault this Bead was filed for.
func TestLibraryRoutesCarryWhatGitSaid(t *testing.T) {
	requireGit(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "preferences"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileFixture(t, filepath.Join(root, "preferences", "workflow.md"), "# Workflow\n")
	handler := newLibraryHandlerForTest(t, LibraryConfig{Root: root, Author: testLibraryAuthor})

	page := decodeLibrary[LibraryPageResponse](t,
		libraryRequest(t, handler, http.MethodGet, "/api/library/page?path=preferences/workflow.md", ""))
	if page.Error == "" {
		t.Fatalf("page = %#v, want the reason its history is empty", page)
	}
	if page.Content == "" {
		t.Fatalf("page carries no content, want the page git has nothing to say about")
	}

	changes := decodeLibrary[LibraryChangesResponse](t,
		libraryRequest(t, handler, http.MethodGet, "/api/library/changes", ""))
	if changes.Error == "" {
		t.Fatalf("changes = %#v, want the reason the list is empty", changes)
	}

	pages := decodeLibrary[LibraryPagesResponse](t,
		libraryRequest(t, handler, http.MethodGet, "/api/library/pages?shelf=preferences", ""))
	if pages.Error == "" || len(pages.Pages) != 1 {
		t.Fatalf("pages = %#v, want the shelf and the reason it carries no dates", pages)
	}

	rec := libraryRequest(t, handler, http.MethodPut, "/api/library/page",
		`{"path":"preferences/workflow.md","content":"# Workflow\n\nEdited.\n","summary":"Edit"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("save status = %d, want 500 when git cannot say whether the page is dirty: %s",
			rec.Code, rec.Body.String())
	}
}

func TestFilesHandlerDiffCarriesWhatGitSaid(t *testing.T) {
	requireGit(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeFileFixture(t, filepath.Join(root, ".git"), "gitdir: /nonexistent/worktree\n")
	path := filepath.Join(root, "inside.txt")
	writeFileFixture(t, path, "content\n")

	rec, response := diffRequest(t, newDiffHandler(root), path)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if response.Error == "" {
		t.Fatalf("response = %+v, want the reason the diff is empty", response)
	}
	if response.Diff != "" {
		t.Fatalf("diff = %q, want nothing from a repository git would not read", response.Diff)
	}
}
