package api

import (
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/chrote/server/internal/core"
)

// The map reads the corpus as a graph: every page is a node, every resolved
// wiki link is an edge, and every tag two pages share is a fainter one. All of
// it is derived on request from the tree and one git log; nothing is indexed
// or stored, so the map can never disagree with the shelves.

// libraryGraphPageLimit bounds the pages the graph reads, the way search bounds
// the pages it walks.
const libraryGraphPageLimit = 2000

// LibraryGraphPage is one page as the map draws it: where it sits, what it is
// called, how much it says, when it arrived and when it last moved, and whether
// it is still a candidate rather than a page the operator has accepted.
type LibraryGraphPage struct {
	Path    string `json:"path"`
	Shelf   string `json:"shelf"`
	Title   string `json:"title"`
	Words   int    `json:"words"`
	Updated string `json:"updated"`
	// Created is when the commit that first named this path was made, which is
	// the corpus's own answer to when the page was written. Empty for a page
	// git has never seen.
	Created   string `json:"created"`
	Candidate bool   `json:"candidate"`
}

// LibraryGraphResponse is the body of GET /api/library/graph. Links are
// [from, to] pairs of page paths; Tags are [from, to, tag] triples, one per
// tag two pages share. Error is why no page carries a date when git refused
// the corpus; the pages and their links are read off disk and are there
// regardless.
type LibraryGraphResponse struct {
	Pages []LibraryGraphPage `json:"pages"`
	Links [][2]string        `json:"links"`
	Tags  [][3]string        `json:"tags"`
	Error string             `json:"error,omitempty"`
}

// libraryWikiLink matches [[target]], [[target|label]] and [[target#anchor]].
// The target is the first group; the label and the anchor are the writer's
// business and are not what the link resolves by.
var libraryWikiLink = regexp.MustCompile(`\[\[([^\[\]|#]+)(?:#[^\[\]|]*)?(?:\|[^\[\]]*)?\]\]`)

// libraryFrontMatter is what the map reads out of a page's YAML front matter.
// The parser reads the two forms the corpus writes for tags — `tags: [a, b]`
// and a block list under `tags:` — and the one lifecycle value that matters,
// because a YAML library for two keys is a dependency for nothing.
type libraryFrontMatter struct {
	tags      []string
	candidate bool
}

// splitLibraryFrontMatter separates a page's front matter from its body. A
// page that does not open with a `---` line has no front matter, and one whose
// front matter never closes is read as prose.
func splitLibraryFrontMatter(content string) (string, string) {
	lines := strings.SplitAfter(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", content
	}
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "---" {
			return strings.Join(lines[1:index], ""), strings.Join(lines[index+1:], "")
		}
	}
	return "", content
}

func cleanLibraryTag(raw string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(raw), `"'`))
}

func parseLibraryFrontMatter(front string) libraryFrontMatter {
	var meta libraryFrontMatter
	seen := make(map[string]bool)
	add := func(raw string) {
		tag := cleanLibraryTag(raw)
		if tag == "" || seen[tag] {
			return
		}
		seen[tag] = true
		meta.tags = append(meta.tags, tag)
	}
	lines := strings.Split(front, "\n")
	for index, line := range lines {
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '-' || line[0] == '#' {
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "lifecycle":
			meta.candidate = strings.EqualFold(strings.Trim(value, `"'`), "candidate")
		case "tags":
			if value == "" {
				for _, item := range lines[index+1:] {
					trimmed := strings.TrimSpace(item)
					if !strings.HasPrefix(trimmed, "-") {
						break
					}
					add(strings.TrimPrefix(trimmed, "-"))
				}
				continue
			}
			for _, item := range strings.Split(strings.Trim(value, "[]"), ",") {
				add(item)
			}
		}
	}
	return meta
}

// libraryShelfOf is the shelf a page sits on: the first segment of its
// corpus-relative path, or nothing for a page at the root.
func libraryShelfOf(relative string) string {
	if index := strings.Index(relative, "/"); index > 0 {
		return relative[:index]
	}
	return ""
}

// libraryIndex resolves a wiki link's target to a page. A target is tried as a
// path, then as a file name, then as a title, all case-insensitively, which is
// the order a writer means them in: `[[telos/goals]]` names one page, `[[goals]]`
// names it by file, and `[[Verification evidence]]` names it by what it calls
// itself.
type libraryIndex struct {
	byPath  map[string]string
	byName  map[string][]string
	byTitle map[string][]string
}

func newLibraryIndex(pages []LibraryGraphPage) libraryIndex {
	index := libraryIndex{
		byPath:  make(map[string]string, len(pages)),
		byName:  make(map[string][]string),
		byTitle: make(map[string][]string),
	}
	for _, page := range pages {
		stem := strings.ToLower(strings.TrimSuffix(page.Path, path.Ext(page.Path)))
		index.byPath[stem] = page.Path
		name := path.Base(stem)
		index.byName[name] = append(index.byName[name], page.Path)
		title := strings.ToLower(strings.TrimSpace(page.Title))
		index.byTitle[title] = append(index.byTitle[title], page.Path)
	}
	return index
}

// resolve returns the page a target names from the page that names it. Two
// pages with one name are told apart by the shelf the writer was on.
func (index libraryIndex) resolve(target, from string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(target))
	if strings.HasSuffix(key, libraryPageExtension) {
		key = strings.TrimSuffix(key, libraryPageExtension)
	}
	key = strings.Trim(key, "/")
	if key == "" {
		return "", false
	}
	if found, known := index.byPath[key]; known {
		return found, true
	}
	candidates := index.byName[path.Base(key)]
	if len(candidates) == 0 {
		candidates = index.byTitle[key]
	}
	if len(candidates) == 0 {
		return "", false
	}
	shelf := libraryShelfOf(from)
	for _, candidate := range candidates {
		if libraryShelfOf(candidate) == shelf {
			return candidate, true
		}
	}
	return candidates[0], true
}

// libraryLinks resolves every wiki link in every page. A link to a page that is
// not there is dropped, a page's link to itself is dropped, and a page that
// names another twice links to it once.
func libraryLinks(pages []LibraryGraphPage, bodies map[string]string) [][2]string {
	index := newLibraryIndex(pages)
	links := make([][2]string, 0)
	for _, page := range pages {
		seen := make(map[string]bool)
		for _, match := range libraryWikiLink.FindAllStringSubmatch(bodies[page.Path], -1) {
			target, resolved := index.resolve(match[1], page.Path)
			if !resolved || target == page.Path || seen[target] {
				continue
			}
			seen[target] = true
			links = append(links, [2]string{page.Path, target})
		}
	}
	sort.Slice(links, func(i, j int) bool {
		if links[i][0] != links[j][0] {
			return links[i][0] < links[j][0]
		}
		return links[i][1] < links[j][1]
	})
	return links
}

// libraryTagEdges joins every two pages that share a tag. A tag carried by
// more than a quarter of the corpus says nothing about any two pages — it is
// the corpus's own word for itself — so it draws no edge.
func libraryTagEdges(pages []LibraryGraphPage, tags map[string][]string) [][3]string {
	carriers := make(map[string][]string)
	for _, page := range pages {
		for _, tag := range tags[page.Path] {
			carriers[tag] = append(carriers[tag], page.Path)
		}
	}
	edges := make([][3]string, 0)
	for tag, paths := range carriers {
		if len(paths) < 2 || len(paths)*4 > len(pages) {
			continue
		}
		sort.Strings(paths)
		for i := 0; i < len(paths); i++ {
			for j := i + 1; j < len(paths); j++ {
				edges = append(edges, [3]string{paths[i], paths[j], tag})
			}
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		for field := 0; field < 3; field++ {
			if edges[i][field] != edges[j][field] {
				return edges[i][field] < edges[j][field]
			}
		}
		return false
	})
	return edges
}

// Graph handles GET /api/library/graph - the corpus as the map draws it. One
// walk reads every page, one git log dates them all, from the commit each
// arrived in to the one that last touched it.
func (h *LibraryHandler) Graph(w http.ResponseWriter, r *http.Request) {
	if !h.configured(w) {
		return
	}
	lastChange, firstChange, gitErr := h.changeSpanByPath(r.Context(), "")

	pages := make([]LibraryGraphPage, 0)
	bodies := make(map[string]string)
	tags := make(map[string][]string)
	_ = filepath.WalkDir(h.config.Root, func(walked string, entry fs.DirEntry, err error) error {
		if err != nil || r.Context().Err() != nil {
			if r.Context().Err() != nil {
				return fs.SkipAll
			}
			return nil
		}
		if entry.IsDir() {
			if walked != h.config.Root && libraryIgnoresDirectory(entry.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || !isLibraryPage(entry.Name()) {
			return nil
		}
		if len(pages) >= libraryGraphPageLimit {
			return fs.SkipAll
		}
		content, readErr := readLibraryFile(walked)
		if readErr != nil {
			return nil
		}
		relative := h.libraryRelative(walked)
		front, body := splitLibraryFrontMatter(content)
		meta := parseLibraryFrontMatter(front)
		page := LibraryGraphPage{
			Path:      relative,
			Shelf:     libraryShelfOf(relative),
			Title:     libraryTitle(body, relative),
			Words:     len(strings.Fields(body)),
			Candidate: meta.candidate,
		}
		if commit, known := lastChange[relative]; known {
			page.Updated = commit.Time
		}
		if commit, known := firstChange[relative]; known {
			page.Created = commit.Time
		}
		pages = append(pages, page)
		bodies[relative] = body
		tags[relative] = meta.tags
		return nil
	})
	sort.Slice(pages, func(i, j int) bool { return pages[i].Path < pages[j].Path })

	response := LibraryGraphResponse{
		Pages: pages,
		Links: libraryLinks(pages, bodies),
		Tags:  libraryTagEdges(pages, tags),
	}
	if gitErr != nil {
		response.Error = gitErr.Error()
	}
	core.WriteJSON(w, http.StatusOK, response)
}
