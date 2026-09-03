package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func newDiffHandler(roots ...string) *FilesHandler {
	return &FilesHandler{
		allowedRoots:   roots,
		maxUploadBytes: defaultMaxUploadBytes,
	}
}

func diffRequest(t *testing.T, handler *FilesHandler, path string) (*httptest.ResponseRecorder, DiffResponse) {
	t.Helper()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	target := "/api/files/diff"
	if path != "" {
		target += "?" + url.Values{"path": {path}}.Encode()
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var response DiffResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("diff response is not JSON: %v: %s", err, rec.Body.String())
		}
	}
	return rec, response
}

// requireGit skips the test when git is absent and isolates every git run in
// the test, including the handler's own, from the host's git configuration.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func runGit(t *testing.T, repository string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repository
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func gitCommitAll(t *testing.T, repository, message string) {
	t.Helper()
	runGit(t, repository, "add", "-A")
	runGit(t, repository, "-c", "user.email=t@example.com", "-c", "user.name=T", "commit", "-q", "-m", message)
}

// newGitRepository returns a canonical temporary repository on branch main.
func newGitRepository(t *testing.T) string {
	t.Helper()
	requireGit(t)
	repository, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "init", "-q", "-b", "main")
	return repository
}

func TestFilesHandlerDiffOutsideRepositoryIsEmpty(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "plain.txt")
	writeFileFixture(t, path, "no repository here\n")

	rec, response := diffRequest(t, newDiffHandler(root), path)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if response.Repository != "" || response.Diff != "" || response.Truncated {
		t.Fatalf("response = %+v, want an empty repository and diff", response)
	}
	if canonical, err := filepath.EvalSymlinks(path); err != nil || response.Path != canonical {
		t.Fatalf("path = %q, want %q", response.Path, canonical)
	}
}

func TestFilesHandlerDiffAgainstHead(t *testing.T) {
	repository := newGitRepository(t)
	unchanged := filepath.Join(repository, "unchanged.txt")
	modified := filepath.Join(repository, "modified.txt")
	nested := filepath.Join(repository, "sub", "dir", "nested.txt")
	writeFileFixture(t, unchanged, "same\n")
	writeFileFixture(t, modified, "one\n")
	writeFileFixture(t, nested, "before\n")
	gitCommitAll(t, repository, "init")
	writeFileFixture(t, modified, "two\n")
	writeFileFixture(t, nested, "after\n")

	tests := []struct {
		name        string
		path        string
		wantRemoved string
		wantAdded   string
	}{
		{name: "unchanged file has an empty diff", path: unchanged},
		{name: "modified file diffs against HEAD", path: modified, wantRemoved: "-one", wantAdded: "+two"},
		{name: "file in a subdirectory", path: nested, wantRemoved: "-before", wantAdded: "+after"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, response := diffRequest(t, newDiffHandler(repository), tt.path)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
			}
			if response.Repository != repository {
				t.Fatalf("repository = %q, want %q", response.Repository, repository)
			}
			if response.Path != tt.path {
				t.Fatalf("path = %q, want %q", response.Path, tt.path)
			}
			if response.Truncated {
				t.Fatalf("truncated = true for a small diff")
			}
			if tt.wantAdded == "" {
				if response.Diff != "" {
					t.Fatalf("diff = %q, want empty for an unchanged file", response.Diff)
				}
				return
			}
			lines := strings.Split(response.Diff, "\n")
			if !containsLine(lines, tt.wantRemoved) || !containsLine(lines, tt.wantAdded) {
				t.Fatalf("diff lacks %q and %q:\n%s", tt.wantRemoved, tt.wantAdded, response.Diff)
			}
			if !strings.Contains(response.Diff, filepath.Base(tt.path)) {
				t.Fatalf("diff does not name %s:\n%s", filepath.Base(tt.path), response.Diff)
			}
		})
	}
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

func TestFilesHandlerDiffRepositoryWithoutCommitsIsEmpty(t *testing.T) {
	repository := newGitRepository(t)
	path := filepath.Join(repository, "fresh.txt")
	writeFileFixture(t, path, "not committed yet\n")

	rec, response := diffRequest(t, newDiffHandler(repository), path)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 when HEAD does not exist: %s", rec.Code, rec.Body.String())
	}
	if response.Repository != repository || response.Diff != "" || response.Truncated {
		t.Fatalf("response = %+v, want the repository with an empty diff", response)
	}
}

func TestFilesHandlerDiffTreatsGitFileAsRepositoryMarker(t *testing.T) {
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
		t.Fatalf("status = %d, want 200 for an unusable gitfile: %s", rec.Code, rec.Body.String())
	}
	if response.Repository != root || response.Diff != "" {
		t.Fatalf("response = %+v, want repository %q with an empty diff", response, root)
	}
}

func TestFilesHandlerDiffBoundsOutput(t *testing.T) {
	repository := newGitRepository(t)
	path := filepath.Join(repository, "large.txt")
	writeFileFixture(t, path, "small\n")
	gitCommitAll(t, repository, "init")
	line := strings.Repeat("x", 99) + "\n"
	writeFileFixture(t, path, strings.Repeat(line, (diffOutputLimit/len(line))+2048))

	rec, response := diffRequest(t, newDiffHandler(repository), path)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !response.Truncated {
		t.Fatalf("truncated = false for a diff over %d bytes", diffOutputLimit)
	}
	if len(response.Diff) != diffOutputLimit {
		t.Fatalf("diff length = %d, want exactly %d", len(response.Diff), diffOutputLimit)
	}
	if !strings.HasPrefix(response.Diff, "diff --git") {
		t.Fatalf("truncated diff lost its head: %q", response.Diff[:64])
	}
}

func TestFilesHandlerDiffRejectsPathsOutsideRoots(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	path := filepath.Join(outside, "secret.txt")
	writeFileFixture(t, path, "secret\n")

	tests := []struct {
		name string
		path string
	}{
		{name: "outside every root", path: path},
		{name: "relative path", path: "secret.txt"},
		{name: "virtual root", path: "/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, _ := diffRequest(t, newDiffHandler(root), tt.path)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403: %s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "secret") {
				t.Fatalf("response leaked outside content: %s", rec.Body.String())
			}
		})
	}
}

func TestFilesHandlerDiffRequiresPath(t *testing.T) {
	root := t.TempDir()

	rec, _ := diffRequest(t, newDiffHandler(root), "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 without a path: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "BAD_REQUEST") {
		t.Fatalf("body = %s, want BAD_REQUEST", rec.Body.String())
	}
}
