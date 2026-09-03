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

// Every test here builds its own corpus in a temp directory and inits a git
// repository in it. Nothing in this file may touch a real corpus: the Library's
// whole point is that it commits to the operator's own context library, and a
// test that reached one would be writing his notes.

const testLibraryAuthor = "Test Operator <operator@example.invalid>"

// newLibraryCorpus creates a temp corpus, commits it, and returns its root.
func newLibraryCorpus(t *testing.T) string {
	t.Helper()
	root := newGitRepository(t)

	write := func(relative, content string) {
		absolute := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatalf("make corpus directory: %v", err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0o644); err != nil {
			t.Fatalf("write corpus page: %v", err)
		}
	}

	write("README.md", "# The corpus\n\nWhat this library holds.\n")
	write("preferences/workflow.md", "# Workflow Preferences\n\nPrefer small, verifiable changes.\n")
	write("preferences/tools.md", "Tools the operator reaches for.\n")
	write("knowledge/testing.md", "# Test isolation\n\nA serious lab gets a durable path.\n")
	write("knowledge/notes/deep.md", "# Nested\n\nA page below the shelf's own directory.\n")
	write("knowledge/diagram.png", "not a page")
	write(".hidden/secret.md", "# Hidden\n\nA dot-directory is not a shelf.\n")

	gitCommitAll(t, root, "Seed the corpus")
	runGit(t, root, "-c", "user.email=t@example.com", "-c", "user.name=T",
		"commit", "-q", "--allow-empty", "-m", "Curate the shelves")
	return root
}

func newLibraryHandlerForTest(t *testing.T, config LibraryConfig) *LibraryHandler {
	t.Helper()
	return NewLibraryHandler(config)
}

// libraryRequest runs one request through the registered routes.
func libraryRequest(t *testing.T, handler *LibraryHandler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decodeLibrary[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(rec.Body.Bytes(), &value); err != nil {
		t.Fatalf("response is not the expected JSON: %v: %s", err, rec.Body.String())
	}
	return value
}

func TestLoadLibraryConfig(t *testing.T) {
	root := t.TempDir()
	page := filepath.Join(root, "page.md")
	if err := os.WriteFile(page, []byte("# Page\n"), 0o644); err != nil {
		t.Fatalf("write page: %v", err)
	}

	tests := []struct {
		name     string
		env      map[string]string
		wantErr  string
		wantRoot string
		wantEdit string
	}{
		{
			name: "unset is a host without a library",
			env:  map[string]string{},
		},
		{
			name:     "a directory is the corpus",
			env:      map[string]string{"CHROTE_LIBRARY_ROOT": root},
			wantRoot: root,
		},
		{
			name:    "a missing root is an operator mistake",
			env:     map[string]string{"CHROTE_LIBRARY_ROOT": filepath.Join(root, "nowhere")},
			wantErr: "CHROTE_LIBRARY_ROOT",
		},
		{
			name:    "a file is not a corpus",
			env:     map[string]string{"CHROTE_LIBRARY_ROOT": page},
			wantErr: "is not a directory",
		},
		{
			name:     "an author is Name and an address",
			env:      map[string]string{"CHROTE_LIBRARY_AUTHOR": testLibraryAuthor},
			wantEdit: testLibraryAuthor,
		},
		{
			name:    "a bare name cannot be committed as",
			env:     map[string]string{"CHROTE_LIBRARY_AUTHOR": "Test Operator"},
			wantErr: "Name <email>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"CHROTE_LIBRARY_ROOT", "CHROTE_LIBRARY_AUTHOR", "CHROTE_LIBRARIAN_SESSION", "CHROTE_LIBRARY_BEADS"} {
				t.Setenv(key, "")
			}
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			config, err := LoadLibraryConfig()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want one naming %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantRoot != "" {
				resolved, _ := filepath.EvalSymlinks(tt.wantRoot)
				if config.Root != filepath.Clean(resolved) {
					t.Fatalf("root = %q, want %q", config.Root, resolved)
				}
			}
			if config.Author != tt.wantEdit {
				t.Fatalf("author = %q, want %q", config.Author, tt.wantEdit)
			}
		})
	}
}

func TestLibraryShelvesCarriesTheConfiguration(t *testing.T) {
	root := newLibraryCorpus(t)

	tests := []struct {
		name       string
		config     LibraryConfig
		wantRoot   string
		wantShelf  []string
		wantPages  map[string]int
		wantDesk   string
		wantStore  string
		wantStatus int
	}{
		{
			name:       "no root is a tab that says so",
			config:     LibraryConfig{},
			wantRoot:   "",
			wantShelf:  []string{},
			wantStatus: http.StatusOK,
		},
		{
			name: "the shelves are the top-level directories",
			config: LibraryConfig{
				Root:             root,
				LibrarianSession: "librarian",
				BeadsProject:     "/corpus/store",
			},
			wantRoot:   root,
			wantShelf:  []string{"knowledge", "preferences"},
			wantPages:  map[string]int{"knowledge": 2, "preferences": 2},
			wantDesk:   "librarian",
			wantStore:  "/corpus/store",
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := libraryRequest(t, newLibraryHandlerForTest(t, tt.config), http.MethodGet, "/api/library/shelves", "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			response := decodeLibrary[LibraryShelvesResponse](t, rec)
			if response.Root != tt.wantRoot {
				t.Fatalf("root = %q, want %q", response.Root, tt.wantRoot)
			}
			names := make([]string, 0, len(response.Shelves))
			for _, shelf := range response.Shelves {
				names = append(names, shelf.Name)
				if want, known := tt.wantPages[shelf.Name]; known && shelf.Pages != want {
					t.Fatalf("%s pages = %d, want %d", shelf.Name, shelf.Pages, want)
				}
			}
			if strings.Join(names, ",") != strings.Join(tt.wantShelf, ",") {
				t.Fatalf("shelves = %v, want %v", names, tt.wantShelf)
			}
			if response.LibrarianSession != tt.wantDesk {
				t.Fatalf("librarianSession = %q, want %q", response.LibrarianSession, tt.wantDesk)
			}
			if response.BeadsProject != tt.wantStore {
				t.Fatalf("beadsProject = %q, want %q", response.BeadsProject, tt.wantStore)
			}
		})
	}
}

func TestLibraryPagesTitlesAndLastChange(t *testing.T) {
	root := newLibraryCorpus(t)
	handler := newLibraryHandlerForTest(t, LibraryConfig{Root: root})

	rec := libraryRequest(t, handler, http.MethodGet, "/api/library/pages?shelf=preferences", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	pages := decodeLibrary[[]LibraryPage](t, rec)
	if len(pages) != 2 {
		t.Fatalf("pages = %d, want 2: %#v", len(pages), pages)
	}
	if pages[0].Path != "preferences/tools.md" || pages[1].Path != "preferences/workflow.md" {
		t.Fatalf("paths = %q, %q, want the shelf in order", pages[0].Path, pages[1].Path)
	}
	// A page with no heading is titled by its file name; one with a heading
	// says what it calls itself.
	if pages[0].Title != "tools" {
		t.Fatalf("title without a heading = %q, want the file name", pages[0].Title)
	}
	if pages[1].Title != "Workflow Preferences" {
		t.Fatalf("title = %q, want the first heading", pages[1].Title)
	}
	if pages[1].Updated == "" || pages[1].Author == "" {
		t.Fatalf("page carries no last change: %#v", pages[1])
	}

	nested := libraryRequest(t, handler, http.MethodGet, "/api/library/pages?shelf=knowledge", "")
	knowledge := decodeLibrary[[]LibraryPage](t, nested)
	found := false
	for _, page := range knowledge {
		if page.Path == "knowledge/notes/deep.md" {
			found = true
		}
		if strings.HasSuffix(page.Path, ".png") {
			t.Fatalf("a shelf listed something that is not a page: %q", page.Path)
		}
	}
	if !found {
		t.Fatalf("a page below the shelf's own directory was not listed: %#v", knowledge)
	}
}

func TestLibraryPageRefusesEveryPathOutsideTheRoot(t *testing.T) {
	root := newLibraryCorpus(t)
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("# Outside\n"), 0o644); err != nil {
		t.Fatalf("write outside page: %v", err)
	}
	handler := newLibraryHandlerForTest(t, LibraryConfig{Root: root})

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{name: "a page in the corpus", path: "preferences/workflow.md", wantStatus: http.StatusOK},
		{name: "the corpus's own README", path: "README.md", wantStatus: http.StatusOK},
		{name: "a traversal", path: "../outside.md", wantStatus: http.StatusForbidden},
		{name: "a traversal inside a shelf", path: "preferences/../../outside.md", wantStatus: http.StatusForbidden},
		{name: "an absolute path", path: outside, wantStatus: http.StatusForbidden},
		{name: "nothing at all", path: "", wantStatus: http.StatusForbidden},
		{name: "a shelf is not a page", path: "preferences", wantStatus: http.StatusNotFound},
		{name: "a page that is not there", path: "preferences/absent.md", wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := "/api/library/page?" + url.Values{"path": {tt.path}}.Encode()
			rec := libraryRequest(t, handler, http.MethodGet, target, "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestLibraryPageCarriesItsHistory(t *testing.T) {
	root := newLibraryCorpus(t)
	handler := newLibraryHandlerForTest(t, LibraryConfig{Root: root})

	rec := libraryRequest(t, handler, http.MethodGet, "/api/library/page?path=preferences/workflow.md", "")
	page := decodeLibrary[LibraryPageResponse](t, rec)
	if page.Title != "Workflow Preferences" {
		t.Fatalf("title = %q", page.Title)
	}
	if !strings.Contains(page.Content, "Prefer small, verifiable changes.") {
		t.Fatalf("content = %q", page.Content)
	}
	if len(page.History) != 1 {
		t.Fatalf("history = %d entries, want the one commit that touched it", len(page.History))
	}
	if page.History[0].Message != "Seed the corpus" || page.History[0].Hash == "" {
		t.Fatalf("history entry = %#v", page.History[0])
	}
	if page.Updated != page.History[0].Time || page.Author != page.History[0].Author {
		t.Fatalf("the page's last change does not agree with its history: %#v", page)
	}
}

func TestLibrarySearch(t *testing.T) {
	root := newLibraryCorpus(t)
	handler := newLibraryHandlerForTest(t, LibraryConfig{Root: root})

	tests := []struct {
		name      string
		query     string
		wantFirst string
		wantCount int
		wantLine  int
	}{
		{name: "an empty query asks nothing", query: "", wantCount: 0},
		{
			name:      "a word in the prose",
			query:     "verifiable",
			wantFirst: "preferences/workflow.md",
			wantCount: 1,
			wantLine:  3,
		},
		{
			name:      "the search is case-insensitive",
			query:     "VERIFIABLE",
			wantFirst: "preferences/workflow.md",
			wantCount: 1,
			wantLine:  3,
		},
		{
			name:      "a name match comes before a prose match",
			query:     "workflow",
			wantFirst: "preferences/workflow.md",
			wantCount: 1,
		},
		{
			name:      "a dot-directory is not searched",
			query:     "Hidden",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := "/api/library/search?" + url.Values{"q": {tt.query}}.Encode()
			rec := libraryRequest(t, handler, http.MethodGet, target, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
			}
			results := decodeLibrary[[]LibrarySearchResult](t, rec)
			if len(results) != tt.wantCount {
				t.Fatalf("results = %d, want %d: %#v", len(results), tt.wantCount, results)
			}
			if tt.wantCount == 0 {
				return
			}
			if results[0].Path != tt.wantFirst {
				t.Fatalf("first result = %q, want %q", results[0].Path, tt.wantFirst)
			}
			if tt.wantLine != 0 && results[0].Line != tt.wantLine {
				t.Fatalf("line = %d, want %d (snippet %q)", results[0].Line, tt.wantLine, results[0].Snippet)
			}
		})
	}
}

func TestLibraryChanges(t *testing.T) {
	root := newLibraryCorpus(t)
	handler := newLibraryHandlerForTest(t, LibraryConfig{Root: root})

	tests := []struct {
		name       string
		target     string
		wantStatus int
		wantCount  int
	}{
		{name: "the whole log by default", target: "/api/library/changes", wantStatus: http.StatusOK, wantCount: 2},
		{name: "a limit takes the newest", target: "/api/library/changes?limit=1", wantStatus: http.StatusOK, wantCount: 1},
		{name: "a limit that is not a number is refused", target: "/api/library/changes?limit=soon", wantStatus: http.StatusBadRequest},
		{name: "a limit of zero is refused", target: "/api/library/changes?limit=0", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := libraryRequest(t, handler, http.MethodGet, tt.target, "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			changes := decodeLibrary[[]LibraryChange](t, rec)
			if len(changes) != tt.wantCount {
				t.Fatalf("changes = %d, want %d: %#v", len(changes), tt.wantCount, changes)
			}
			if changes[0].Message != "Curate the shelves" {
				t.Fatalf("newest change = %q, want the last commit", changes[0].Message)
			}
			if tt.wantCount > 1 && len(changes[1].Files) == 0 {
				t.Fatalf("a change carries no files: %#v", changes[1])
			}
		})
	}
}

func TestLibrarySavePage(t *testing.T) {
	tests := []struct {
		name       string
		author     string
		dirty      bool
		body       string
		wantStatus int
		wantCommit string
		wantOnDisk string
	}{
		{
			name:       "a summary becomes the commit message",
			author:     testLibraryAuthor,
			body:       `{"path":"preferences/workflow.md","content":"# Workflow Preferences\n\nCorrected.\n","summary":"Correct a wording"}`,
			wantStatus: http.StatusOK,
			wantCommit: "Correct a wording",
			wantOnDisk: "Corrected.",
		},
		{
			name:       "no summary is still a commit that says what it touched",
			author:     testLibraryAuthor,
			body:       `{"path":"preferences/workflow.md","content":"# Workflow Preferences\n\nCorrected again.\n"}`,
			wantStatus: http.StatusOK,
			wantCommit: "Edit preferences/workflow.md",
			wantOnDisk: "Corrected again.",
		},
		{
			name:       "an unattributable edit is refused",
			body:       `{"path":"preferences/workflow.md","content":"whatever","summary":"nope"}`,
			wantStatus: http.StatusConflict,
			wantOnDisk: "Prefer small, verifiable changes.",
		},
		{
			name:       "somebody else's uncommitted change is not swallowed",
			author:     testLibraryAuthor,
			dirty:      true,
			body:       `{"path":"preferences/workflow.md","content":"mine","summary":"mine"}`,
			wantStatus: http.StatusConflict,
			wantOnDisk: "Half an edit somebody else left",
		},
		{
			name:       "a path outside the corpus is refused",
			author:     testLibraryAuthor,
			body:       `{"path":"../escape.md","content":"x","summary":"x"}`,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "a page that is not there is not created",
			author:     testLibraryAuthor,
			body:       `{"path":"preferences/new.md","content":"x","summary":"x"}`,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "a body that is not a save request is refused",
			author:     testLibraryAuthor,
			body:       `not json`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := newLibraryCorpus(t)
			page := filepath.Join(root, "preferences", "workflow.md")
			if tt.dirty {
				if err := os.WriteFile(page, []byte("Half an edit somebody else left\n"), 0o644); err != nil {
					t.Fatalf("dirty the page: %v", err)
				}
			}
			handler := newLibraryHandlerForTest(t, LibraryConfig{Root: root, Author: tt.author})

			rec := libraryRequest(t, handler, http.MethodPut, "/api/library/page", tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantOnDisk != "" {
				content, err := os.ReadFile(page)
				if err != nil {
					t.Fatalf("read the page back: %v", err)
				}
				if !strings.Contains(string(content), tt.wantOnDisk) {
					t.Fatalf("page on disk = %q, want it to hold %q", content, tt.wantOnDisk)
				}
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			commit := decodeLibrary[LibraryCommit](t, rec)
			if commit.Message != tt.wantCommit {
				t.Fatalf("commit message = %q, want %q", commit.Message, tt.wantCommit)
			}
			if commit.Hash == "" || commit.Time == "" {
				t.Fatalf("commit = %#v, want the entry the save made", commit)
			}
			if commit.Author != "Test Operator" {
				t.Fatalf("commit author = %q, want the configured operator", commit.Author)
			}
			if dirty := gitPorcelain(t, root); dirty != "" {
				t.Fatalf("the corpus is dirty after a save: %q", dirty)
			}
		})
	}
}

func TestLibrarySaveOfUnchangedContentMakesNoCommit(t *testing.T) {
	root := newLibraryCorpus(t)
	handler := newLibraryHandlerForTest(t, LibraryConfig{Root: root, Author: testLibraryAuthor})
	before := gitCommitCount(t, root)

	body := `{"path":"preferences/workflow.md","content":"# Workflow Preferences\n\nPrefer small, verifiable changes.\n","summary":"No change"}`
	rec := libraryRequest(t, handler, http.MethodPut, "/api/library/page", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if after := gitCommitCount(t, root); after != before {
		t.Fatalf("commits = %s, want the %s there were: a save that changed nothing is not a commit", after, before)
	}
	commit := decodeLibrary[LibraryCommit](t, rec)
	if commit.Message != "Seed the corpus" {
		t.Fatalf("returned entry = %q, want the page's existing last change", commit.Message)
	}
}

func TestLibraryRoutesRefuseWhenNoLibraryIsConfigured(t *testing.T) {
	handler := newLibraryHandlerForTest(t, LibraryConfig{})

	tests := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{name: "pages", method: http.MethodGet, target: "/api/library/pages?shelf=preferences"},
		{name: "page", method: http.MethodGet, target: "/api/library/page?path=a.md"},
		{name: "search", method: http.MethodGet, target: "/api/library/search?q=a"},
		{name: "changes", method: http.MethodGet, target: "/api/library/changes"},
		{name: "save", method: http.MethodPut, target: "/api/library/page", body: `{"path":"a.md","content":"x"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := libraryRequest(t, handler, tt.method, tt.target, tt.body)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusNotFound, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "No library is configured") {
				t.Fatalf("body = %s, want the reason", rec.Body.String())
			}
		})
	}
}

func TestSplitLibraryAuthor(t *testing.T) {
	tests := []struct {
		author    string
		wantName  string
		wantEmail string
	}{
		{author: "Test Operator <operator@example.invalid>", wantName: "Test Operator", wantEmail: "operator@example.invalid"},
		{author: "One Name <a@b>", wantName: "One Name", wantEmail: "a@b"},
	}
	for _, tt := range tests {
		t.Run(tt.author, func(t *testing.T) {
			name, email := splitLibraryAuthor(tt.author)
			if name != tt.wantName || email != tt.wantEmail {
				t.Fatalf("split = %q, %q, want %q, %q", name, email, tt.wantName, tt.wantEmail)
			}
		})
	}
}

func gitPorcelain(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func gitCommitCount(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-list", "--count", "HEAD")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-list: %v", err)
	}
	return strings.TrimSpace(string(output))
}
