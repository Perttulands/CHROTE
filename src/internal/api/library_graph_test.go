package api

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// gitCommitAllAt commits everything at an exact moment. The graph reports when
// a page arrived and when it last moved; two commits made in the same second
// would make those two facts indistinguishable, and the test would pass on a
// slow machine and fail on a fast one.
func gitCommitAllAt(t *testing.T, repository, message, when string) {
	t.Helper()
	runGit(t, repository, "add", "-A")
	cmd := exec.Command("git",
		"-c", "user.email=t@example.com", "-c", "user.name=T", "commit", "-q", "-m", message)
	cmd.Dir = repository
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_DATE="+when,
		"GIT_COMMITTER_DATE="+when,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit %q: %v\n%s", message, err, output)
	}
}

// The two moments the corpus is built at, far enough apart to read.
const (
	libraryGraphSeededAt  = "2026-01-02T03:04:05+00:00"
	libraryGraphRevisedAt = "2026-02-03T04:05:06+00:00"
)

// newLibraryGraphCorpus builds a corpus whose links and tags exercise every
// resolution rule: a link by file name, by title and by path; a name two pages
// share; a link to nothing; a tag two pages share and a tag the corpus wears
// everywhere; a candidate; a page git has never seen.
func newLibraryGraphCorpus(t *testing.T) string {
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

	write("knowledge/alpha.md", "# Alpha\n\nSee [[beta]], [[Gamma Title]], [[knowledge/notes/delta]], [[nowhere]], [[BETA|again]] and [[alpha]].\n")
	write("knowledge/beta.md", "# Beta\n\nOne two three.\n")
	write("knowledge/gamma.md", "# Gamma Title\n")
	write("knowledge/notes/delta.md", "# Delta\n\n[[tools#section]]\n")
	write("identity/notes.md", "# Notes elsewhere\n")
	write("preferences/notes.md", "---\ntags: [mass]\n---\n# Notes\n")
	write("preferences/tools.md", "---\nlifecycle: candidate\ntags: [shared, mass]\n---\n# Tools\n")
	write("preferences/workflow.md", "---\ntags:\n  - shared\n  - mass\n---\n# Workflow\n\n[[notes]]\n")
	write("telos/goals.md", "---\ntags: [lonely, MASS]\n---\n# Goals\n\n[[preferences/tools.md|the tools]]\n")
	gitCommitAllAt(t, root, "Seed the corpus", libraryGraphSeededAt)

	write("knowledge/beta.md", "# Beta\n\nOne two three four.\n")
	gitCommitAllAt(t, root, "Revise beta", libraryGraphRevisedAt)

	write("telos/uncommitted.md", "# Uncommitted\n")
	return root
}

func TestLibraryGraph(t *testing.T) {
	root := newLibraryGraphCorpus(t)
	handler := newLibraryHandlerForTest(t, LibraryConfig{Root: root})

	rec := libraryRequest(t, handler, http.MethodGet, "/api/library/graph", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	graph := decodeLibrary[LibraryGraphResponse](t, rec)
	if graph.Error != "" {
		t.Fatalf("graph carries a git error: %s", graph.Error)
	}

	pages := make(map[string]LibraryGraphPage, len(graph.Pages))
	for _, page := range graph.Pages {
		pages[page.Path] = page
	}
	if len(pages) != 10 {
		t.Fatalf("pages = %d, want every page under the root: %#v", len(pages), graph.Pages)
	}

	t.Run("a page knows its shelf, its title, its size and whether it is a candidate", func(t *testing.T) {
		goals := pages["telos/goals.md"]
		if goals.Shelf != "telos" || goals.Title != "Goals" {
			t.Fatalf("goals = %#v", goals)
		}
		if pages["knowledge/notes/delta.md"].Shelf != "knowledge" {
			t.Fatalf("a nested page is on its top-level shelf: %#v", pages["knowledge/notes/delta.md"])
		}
		if pages["knowledge/gamma.md"].Words != 3 {
			t.Fatalf("gamma words = %d, want the body's words", pages["knowledge/gamma.md"].Words)
		}
		if !pages["preferences/tools.md"].Candidate || pages["preferences/workflow.md"].Candidate {
			t.Fatalf("candidate flags are wrong: tools %v, workflow %v",
				pages["preferences/tools.md"].Candidate, pages["preferences/workflow.md"].Candidate)
		}
	})

	t.Run("one git log dates every page", func(t *testing.T) {
		alpha, beta := pages["knowledge/alpha.md"].Updated, pages["knowledge/beta.md"].Updated
		if alpha == "" || beta == "" {
			t.Fatalf("committed pages carry no date: alpha %q, beta %q", alpha, beta)
		}
		if beta < alpha {
			t.Fatalf("beta was revised after alpha but reads older: %q < %q", beta, alpha)
		}
		if pages["telos/uncommitted.md"].Updated != "" {
			t.Fatalf("a page git never saw carries a date: %q", pages["telos/uncommitted.md"].Updated)
		}
	})

	// The map's scrubber shows the corpus growing, which needs the date a page
	// arrived and not only the date it last moved. The corpus carries no such
	// field, so it comes from the commit that first named the path.
	t.Run("a page arrived in its first commit and last moved in its newest", func(t *testing.T) {
		beta := pages["knowledge/beta.md"]
		alpha := pages["knowledge/alpha.md"]
		if beta.Created == "" || beta.Updated == "" {
			t.Fatalf("beta carries no dates: created %q, updated %q", beta.Created, beta.Updated)
		}
		if beta.Created >= beta.Updated {
			t.Fatalf("beta was written then revised, but reads created %q updated %q", beta.Created, beta.Updated)
		}
		// Alpha was written once and never touched again.
		if alpha.Created != alpha.Updated {
			t.Fatalf("alpha was committed once but reads created %q updated %q", alpha.Created, alpha.Updated)
		}
		if beta.Created != alpha.Created {
			t.Fatalf("both arrived in the same commit but read %q and %q", beta.Created, alpha.Created)
		}
		if !strings.HasPrefix(beta.Created, "2026-01-02") || !strings.HasPrefix(beta.Updated, "2026-02-03") {
			t.Fatalf("beta reads created %q updated %q, not the two commits it has", beta.Created, beta.Updated)
		}
		if pages["telos/uncommitted.md"].Created != "" {
			t.Fatalf("a page git never saw carries an arrival: %q", pages["telos/uncommitted.md"].Created)
		}
	})

	t.Run("links resolve by path, by file name and by title, case-insensitively", func(t *testing.T) {
		want := [][2]string{
			{"knowledge/alpha.md", "knowledge/beta.md"},
			{"knowledge/alpha.md", "knowledge/gamma.md"},
			{"knowledge/alpha.md", "knowledge/notes/delta.md"},
			{"knowledge/notes/delta.md", "preferences/tools.md"},
			{"preferences/workflow.md", "preferences/notes.md"},
			{"telos/goals.md", "preferences/tools.md"},
		}
		if !reflect.DeepEqual(graph.Links, want) {
			t.Fatalf("links = %v, want %v", graph.Links, want)
		}
	})

	t.Run("a tag two pages share joins them, a mass tag joins nobody", func(t *testing.T) {
		want := [][3]string{{"preferences/tools.md", "preferences/workflow.md", "shared"}}
		if !reflect.DeepEqual(graph.Tags, want) {
			t.Fatalf("tags = %v, want %v", graph.Tags, want)
		}
	})
}

func TestParseLibraryFrontMatter(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		wantTags      []string
		wantCandidate bool
		wantBody      string
	}{
		{
			name:     "an inline list",
			content:  "---\ntitle: X\ntags: [a, \"B\", c ]\n---\nbody\n",
			wantTags: []string{"a", "b", "c"},
			wantBody: "body\n",
		},
		{
			name:     "a block list",
			content:  "---\ntags:\n  - a\n  - a\n  - b\nother: 1\n---\nbody\n",
			wantTags: []string{"a", "b"},
			wantBody: "body\n",
		},
		{
			name:     "an empty list",
			content:  "---\ntags: []\n---\nbody\n",
			wantBody: "body\n",
		},
		{
			name:          "a candidate",
			content:       "---\nlifecycle: candidate\n---\nbody\n",
			wantCandidate: true,
			wantBody:      "body\n",
		},
		{
			name:     "an accepted page",
			content:  "---\nlifecycle: active\n---\nbody\n",
			wantBody: "body\n",
		},
		{
			name:     "no front matter",
			content:  "# Heading\n\nbody\n",
			wantBody: "# Heading\n\nbody\n",
		},
		{
			name:     "front matter that never closes is prose",
			content:  "---\ntags: [a]\nbody\n",
			wantBody: "---\ntags: [a]\nbody\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			front, body := splitLibraryFrontMatter(tt.content)
			meta := parseLibraryFrontMatter(front)
			if body != tt.wantBody {
				t.Fatalf("body = %q, want %q", body, tt.wantBody)
			}
			if meta.candidate != tt.wantCandidate {
				t.Fatalf("candidate = %v, want %v", meta.candidate, tt.wantCandidate)
			}
			if len(meta.tags) != len(tt.wantTags) || (len(tt.wantTags) > 0 && !reflect.DeepEqual(meta.tags, tt.wantTags)) {
				t.Fatalf("tags = %v, want %v", meta.tags, tt.wantTags)
			}
		})
	}
}

func TestParseLibraryLog(t *testing.T) {
	record := func(fields ...string) string {
		out := libraryRecordSeparator
		for index, field := range fields {
			if index > 0 {
				out += libraryFieldSeparator
			}
			out += field
		}
		return out
	}
	output := record("abc123", "2026-09-01T10:00:00+03:00", "Ann", "Subject\twith a tab") + "\n\na.md\nb/c.md\n" +
		record("def456", "2026-08-01T10:00:00+03:00", "Bob", "An empty commit") + "\n" +
		record("short", "no fields")

	changes := parseLibraryLog(output)
	if len(changes) != 2 {
		t.Fatalf("changes = %d, want the two records with every field: %#v", len(changes), changes)
	}
	first := changes[0]
	if first.Hash != "abc123" || first.Time != "2026-09-01T10:00:00+03:00" || first.Author != "Ann" || first.Message != "Subject\twith a tab" {
		t.Fatalf("first record = %#v", first)
	}
	if !reflect.DeepEqual(first.Files, []string{"a.md", "b/c.md"}) {
		t.Fatalf("first record files = %v", first.Files)
	}
	if changes[1].Message != "An empty commit" || len(changes[1].Files) != 0 {
		t.Fatalf("second record = %#v", changes[1])
	}
	if got := parseLibraryLog(""); len(got) != 0 {
		t.Fatalf("no output parses to %d changes", len(got))
	}
}

// The overview needs current subject membership, real brain metadata and links
// to the evidence; a folder move must be reflected on the next refresh.
func TestLibraryGraphBrainSubjects(t *testing.T) {
	root := newGitRepository(t)
	write := func(relative, content string) {
		t.Helper()
		absolute := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("knowledge/learning/learning.md", "---\nkind: \"topic\"\nsummary: \"A quoted \\\"finding\\\", with evidence.\"\nfacets: [\"software\", \"agentic-work\"]\n---\n# Learning\n\n[[alpha]] and [the study](study.md#result).\n`[[example]]`\n```toml\n[[example]]\n```\n")
	write("knowledge/learning/study.md", "---\nkind: research\nsummary: 'What we''ve learned'\nfacets: [software]\n---\n# Study\n")
	write("knowledge/learning/example.md", "---\nkind: note\n---\n# Example\n")
	write("projects/alpha/README.md", "# Alpha\n")
	write("self/preferences.md", "# Preferences\n")
	for _, relative := range []string{"AGENTS.md", "CLAUDE.md", "knowledge/MAP.md", "knowledge/FACETS.md", "knowledge/maps/software.md", "knowledge/_inbox/pending.md", "_librarian/work.md"} {
		write(relative, "# Housekeeping\n")
	}
	gitCommitAllAt(t, root, "Brain pages", libraryGraphSeededAt)
	handler := newLibraryHandlerForTest(t, LibraryConfig{Root: root})
	graph := decodeLibrary[LibraryGraphResponse](t, libraryRequest(t, handler, http.MethodGet, "/api/library/graph", ""))
	if len(graph.Pages) != 5 {
		t.Fatalf("map has %d pages, want five content pages: %+v", len(graph.Pages), graph.Pages)
	}
	pages := map[string]LibraryGraphPage{}
	for _, page := range graph.Pages {
		pages[page.Path] = page
	}
	topic := pages["knowledge/learning/learning.md"]
	if topic.Subject != "knowledge/learning" || topic.Kind != "topic" || topic.Slug != "learning" || topic.Summary != "A quoted \"finding\", with evidence." || !reflect.DeepEqual(topic.Facets, []string{"software", "agentic-work"}) {
		t.Fatalf("topic metadata = %+v", topic)
	}
	study := pages["knowledge/learning/study.md"]
	if study.Summary != "What we've learned" || !reflect.DeepEqual(study.Facets, []string{"software"}) {
		t.Fatalf("study = %+v", study)
	}
	project := pages["projects/alpha/README.md"]
	if project.Subject != "projects/alpha" || project.Slug != "alpha" || project.Facets == nil {
		t.Fatalf("project = %+v", project)
	}
	self := pages["self/preferences.md"]
	if self.Subject != "self" || self.Kind != "self" {
		t.Fatalf("self = %+v", self)
	}
	wantLinks := [][2]string{{"knowledge/learning/learning.md", "knowledge/learning/study.md"}, {"knowledge/learning/learning.md", "projects/alpha/README.md"}}
	if !reflect.DeepEqual(graph.Links, wantLinks) {
		t.Fatalf("links = %v, want %v", graph.Links, wantLinks)
	}
	if err := os.Rename(filepath.Join(root, "knowledge/learning"), filepath.Join(root, "knowledge/discovery")); err != nil {
		t.Fatal(err)
	}
	graph = decodeLibrary[LibraryGraphResponse](t, libraryRequest(t, handler, http.MethodGet, "/api/library/graph", ""))
	moved := 0
	for _, page := range graph.Pages {
		if page.Shelf == "knowledge" {
			moved++
			if page.Subject != "knowledge/discovery" {
				t.Fatalf("refresh returned former subject: %+v", page)
			}
		}
	}
	if moved != 3 {
		t.Fatalf("moved knowledge pages = %d", moved)
	}
}

// A late-sorting subject must remain discoverable when the corpus grows past
// the former graph bound. This exercises the API, not a private cutoff value.
func TestLibraryGraphIncludesWholeCorpus(t *testing.T) {
	root := newGitRepository(t)
	directory := filepath.Join(root, "knowledge", "large-subject")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2005; index++ {
		name := fmt.Sprintf("page-%04d.md", index)
		if err := os.WriteFile(filepath.Join(directory, name), []byte("---\nkind: note\n---\n# Note\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	last := filepath.Join(root, "self", "values.md")
	if err := os.MkdirAll(filepath.Dir(last), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(last, []byte("# Values\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	handler := newLibraryHandlerForTest(t, LibraryConfig{Root: root})
	rec := libraryRequest(t, handler, http.MethodGet, "/api/library/graph", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	graph := decodeLibrary[LibraryGraphResponse](t, rec)
	if len(graph.Pages) != 2006 || graph.Pages[len(graph.Pages)-1].Path != "self/values.md" {
		t.Fatalf("graph has %d pages; late subject = %+v", len(graph.Pages), graph.Pages[len(graph.Pages)-1])
	}
	hits := decodeLibrary[[]LibrarySearchResult](t, libraryRequest(t, handler, http.MethodGet, "/api/library/search?q=values", ""))
	if len(hits) != 1 || hits[0].Path != "self/values.md" {
		t.Fatalf("late subject search = %+v", hits)
	}
}
