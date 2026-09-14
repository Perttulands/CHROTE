package api

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The projects shelf holds each project's own README under a Library path: a
// symlink, never a copy. These tests pin what the Library follows and what it
// refuses. Breaking them means either a project README vanishes from the map
// or a symlink somewhere else in the corpus starts reading files it must not.

// newLinkedCorpus is the seeded corpus plus a projects shelf of symlinks. It
// returns the root and the directory the accepted link points into.
func newLinkedCorpus(t *testing.T) (root, outside string) {
	t.Helper()
	root = newLibraryCorpus(t)
	outside = t.TempDir()
	link := func(relative, target string) {
		absolute := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatalf("make link directory: %v", err)
		}
		if err := os.Symlink(target, absolute); err != nil {
			t.Fatalf("link %s: %v", relative, err)
		}
	}
	write := func(absolute, content string) {
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatalf("make directory: %v", err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", absolute, err)
		}
	}
	write(filepath.Join(outside, "alpha", "README.md"), "# Alpha\n\nA project README that links [[workflow]].\n")
	write(filepath.Join(outside, "gamma", "notes.txt"), "not markdown")
	write(filepath.Join(outside, "delta", "README.md"), "# Delta\n")

	link("projects/alpha/README.md", filepath.Join(outside, "alpha", "README.md"))     // accepted
	link("projects/beta/README.md", filepath.Join(root, "preferences", "workflow.md")) // inside the corpus: refused
	link("projects/gamma/README.md", filepath.Join(outside, "gamma", "notes.txt"))     // not markdown: refused
	link("projects/epsilon/README.md", filepath.Join(outside, "alpha"))                // a directory: refused
	link("projects/zeta/README.md", filepath.Join(outside, "missing", "README.md"))    // dangling: refused
	link("projects/eta", filepath.Join(outside, "delta"))                              // a linked directory: not walked
	link("preferences/linked.md", filepath.Join(outside, "alpha", "README.md"))        // wrong shelf: refused
	gitCommitAll(t, root, "Link the projects")
	return root, outside
}

func TestLibraryFollowsLinkedProjectPages(t *testing.T) {
	root, outside := newLinkedCorpus(t)
	handler := newLibraryHandlerForTest(t, LibraryConfig{Root: root, Author: testLibraryAuthor})

	t.Run("the projects shelf lists only the accepted link", func(t *testing.T) {
		rec := libraryRequest(t, handler, http.MethodGet, "/api/library/pages?shelf=projects", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
		}
		response := decodeLibrary[LibraryPagesResponse](t, rec)
		paths := make([]string, 0, len(response.Pages))
		for _, page := range response.Pages {
			paths = append(paths, page.Path+"="+page.Title)
		}
		if got := strings.Join(paths, ","); got != "projects/alpha/README.md=Alpha" {
			t.Fatalf("pages = %s", got)
		}
	})

	t.Run("the shelf count sees the link", func(t *testing.T) {
		rec := libraryRequest(t, handler, http.MethodGet, "/api/library/shelves", "")
		for _, shelf := range decodeLibrary[LibraryShelvesResponse](t, rec).Shelves {
			switch shelf.Name {
			case "projects":
				if shelf.Pages != 1 {
					t.Fatalf("projects pages = %d, want 1", shelf.Pages)
				}
			case "preferences":
				if shelf.Pages != 2 {
					t.Fatalf("preferences pages = %d, want 2 (the wrong-shelf link is not a page)", shelf.Pages)
				}
			}
		}
	})

	t.Run("a linked page reads its project's file under the Library path", func(t *testing.T) {
		rec := libraryRequest(t, handler, http.MethodGet, "/api/library/page?path=projects/alpha/README.md", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
		}
		response := decodeLibrary[LibraryPageResponse](t, rec)
		if response.Path != "projects/alpha/README.md" || response.Title != "Alpha" || !strings.Contains(response.Content, "A project README") {
			t.Fatalf("page = %+v", response)
		}
		if len(response.History) != 1 || response.History[0].Message != "Link the projects" {
			t.Fatalf("history = %+v, want the commit that placed the link", response.History)
		}
	})

	t.Run("refused links are not pages", func(t *testing.T) {
		for _, path := range []string{
			"projects/beta/README.md",
			"projects/gamma/README.md",
			"projects/epsilon/README.md",
			"projects/zeta/README.md",
			"projects/eta/README.md",
			"preferences/linked.md",
		} {
			rec := libraryRequest(t, handler, http.MethodGet, "/api/library/page?"+url.Values{"path": {path}}.Encode(), "")
			if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
				t.Fatalf("%s: status = %d, want a refusal: %s", path, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("search and the graph see the linked page", func(t *testing.T) {
		rec := libraryRequest(t, handler, http.MethodGet, "/api/library/search?q=project+README", "")
		if !strings.Contains(rec.Body.String(), "projects/alpha/README.md") {
			t.Fatalf("search missed the linked page: %s", rec.Body.String())
		}
		rec = libraryRequest(t, handler, http.MethodGet, "/api/library/graph", "")
		graph := decodeLibrary[LibraryGraphResponse](t, rec)
		found := false
		for _, page := range graph.Pages {
			if page.Path == "projects/alpha/README.md" {
				found = page.Shelf == "projects" && page.Title == "Alpha"
			}
			if strings.HasPrefix(page.Path, "projects/") && page.Path != "projects/alpha/README.md" {
				t.Fatalf("graph carries a refused link: %s", page.Path)
			}
		}
		if !found {
			t.Fatalf("graph missed the linked page: %+v", graph.Pages)
		}
		linked := false
		for _, edge := range graph.Links {
			if edge[0] == "projects/alpha/README.md" && edge[1] == "preferences/workflow.md" {
				linked = true
			}
		}
		if !linked {
			t.Fatalf("graph did not resolve the wikilink from the linked page: %v", graph.Links)
		}
	})

	t.Run("a linked page cannot be saved through the Library", func(t *testing.T) {
		rec := libraryRequest(t, handler, http.MethodPut, "/api/library/page",
			`{"path":"projects/alpha/README.md","content":"# Rewritten\n","summary":"Edit the project"}`)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403: %s", rec.Code, rec.Body.String())
		}
		content, err := os.ReadFile(filepath.Join(outside, "alpha", "README.md"))
		if err != nil || !strings.HasPrefix(string(content), "# Alpha") {
			t.Fatalf("the project's file changed: %q %v", content, err)
		}
	})
}

func TestLibraryLinkRootsBoundWhereALinkMayPoint(t *testing.T) {
	root, outside := newLinkedCorpus(t)
	elsewhere := t.TempDir()

	for _, tt := range []struct {
		name  string
		roots []string
		want  int
	}{
		{name: "no roots admits any Markdown outside the corpus", roots: nil, want: http.StatusOK},
		{name: "a root containing the target admits it", roots: []string{elsewhere, outside}, want: http.StatusOK},
		{name: "roots that do not contain the target refuse it", roots: []string{elsewhere}, want: http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			handler := newLibraryHandlerForTest(t, LibraryConfig{Root: root, LinkRoots: tt.roots})
			rec := libraryRequest(t, handler, http.MethodGet, "/api/library/page?path=projects/alpha/README.md", "")
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestLoadLibraryConfigReadsLinkRoots(t *testing.T) {
	root := t.TempDir()
	first := t.TempDir()
	second := t.TempDir()
	t.Setenv("CHROTE_LIBRARY_ROOT", root)
	t.Setenv("CHROTE_LIBRARY_AUTHOR", "")
	t.Setenv("CHROTE_LIBRARY_LINK_ROOTS", first+string(os.PathListSeparator)+" "+second+string(os.PathListSeparator))
	config, err := LoadLibraryConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(config.LinkRoots) != 2 || config.LinkRoots[0] != first || config.LinkRoots[1] != second {
		t.Fatalf("link roots = %v", config.LinkRoots)
	}
}
