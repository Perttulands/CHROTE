package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chrote/server/internal/core"
)

// The Library is a reading room over one Markdown corpus held in git. The
// corpus is the operator's own context library: its top-level directories are
// the shelves, its files are the pages, and its history is what changed. CHROTE
// reads it, searches it, and commits the operator's own small corrections back
// to it. Everything here goes through the git binary run inside the corpus
// root; nothing else in CHROTE writes there, and no path outside the root is
// reachable however it is spelled.

// LibraryConfig is what the host says the Library is. Every field is a single
// environment variable, and an empty one means that part of the surface is not
// configured rather than that it is broken.
type LibraryConfig struct {
	// Root is the corpus directory. Empty means no library at all.
	Root string
	// Author is the git identity the operator's edits are committed as, in
	// "Name <email>" form. Empty refuses edits.
	Author string
	// LibrarianSession is the tmux session the Front desk talks to.
	LibrarianSession string
	// BeadsProject is the store whose open Beads are the proposals in flight.
	BeadsProject string
}

// libraryAuthorPattern is the "Name <email>" form git itself writes. It is
// checked at startup because an identity git will not accept turns every save
// into a failure the operator only discovers mid-edit.
var libraryAuthorPattern = regexp.MustCompile(`^[^<>]+\s+<[^<>\s]+>$`)

const (
	// libraryHistoryLimit caps the commits a page carries.
	libraryHistoryLimit = 20
	// librarySearchLimit caps the results a search returns.
	librarySearchLimit = 50
	// librarySearchCandidateLimit bounds the files a search reads, so the
	// ordering rule is applied to a bounded superset of the corpus.
	librarySearchCandidateLimit = 2000
	// libraryChangesDefault and libraryChangesLimit bound the recent-changes list.
	libraryChangesDefault = 30
	libraryChangesLimit   = 200
	// libraryFileLimit bounds the bytes read from one page.
	libraryFileLimit = 1 << 20
	// libraryGitOutputLimit bounds the bytes read from one git invocation.
	libraryGitOutputLimit = 8 << 20
	// libraryGitTimeout bounds one git invocation.
	libraryGitTimeout = 20 * time.Second
	// libraryPageExtension is what counts as a page.
	libraryPageExtension = ".md"
	// librarySnippetLimit bounds one search snippet.
	librarySnippetLimit = 200
)

// libraryRecordSeparator and libraryFieldSeparator are the ASCII separators git
// writes for us, so a commit subject containing newlines or tabs still parses.
const (
	libraryRecordSeparator = "\x1e"
	libraryFieldSeparator  = "\x1f"
)

// LibraryShelf is one top-level directory of the corpus.
type LibraryShelf struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Pages int    `json:"pages"`
}

// LibraryShelvesResponse is the body of GET /api/library/shelves. It carries
// the configuration the tab needs to draw itself as well as the shelves: the
// root says whether there is a library at all, and the session and the store
// say whether the Front desk and the Proposals shelf have anything behind them.
type LibraryShelvesResponse struct {
	Root             string         `json:"root"`
	Shelves          []LibraryShelf `json:"shelves"`
	LibrarianSession string         `json:"librarianSession"`
	BeadsProject     string         `json:"beadsProject"`
}

// LibraryPage is one page as a shelf lists it.
type LibraryPage struct {
	Path    string `json:"path"`
	Title   string `json:"title"`
	Updated string `json:"updated"`
	Author  string `json:"author"`
}

// LibraryCommit is one entry of a page's history.
type LibraryCommit struct {
	Hash    string `json:"hash"`
	Time    string `json:"time"`
	Author  string `json:"author"`
	Message string `json:"message"`
}

// LibraryChange is one commit of the corpus with the pages it touched.
type LibraryChange struct {
	LibraryCommit
	Files []string `json:"files"`
}

// LibraryPagesResponse is the body of GET /api/library/pages. The shelf is
// walked on disk, so its pages are there whatever git says; Error is why they
// carry no dates when git would not read the corpus.
type LibraryPagesResponse struct {
	Pages []LibraryPage `json:"pages"`
	Error string        `json:"error,omitempty"`
}

// LibraryChangesResponse is the body of GET /api/library/changes. A corpus that
// git refused and a corpus with nothing new both list no changes, and Error is
// the difference between them.
type LibraryChangesResponse struct {
	Changes []LibraryChange `json:"changes"`
	Error   string          `json:"error,omitempty"`
}

// LibraryPageResponse is the body of GET /api/library/page. Error is why the
// history is empty when git refused the corpus; the page itself is read off
// disk and is there regardless.
type LibraryPageResponse struct {
	Path    string          `json:"path"`
	Title   string          `json:"title"`
	Content string          `json:"content"`
	Updated string          `json:"updated"`
	Author  string          `json:"author"`
	History []LibraryCommit `json:"history"`
	Error   string          `json:"error,omitempty"`
}

// LibrarySearchResult is one hit of GET /api/library/search.
type LibrarySearchResult struct {
	Path    string `json:"path"`
	Title   string `json:"title"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

// LibrarySaveRequest is the body of PUT /api/library/page.
type LibrarySaveRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Summary string `json:"summary"`
}

// LibraryHandler serves the Library routes over one configured corpus.
type LibraryHandler struct {
	config LibraryConfig
}

// LoadLibraryConfig reads the Library's environment and refuses a configuration
// nobody can use. A corpus root that is not a directory is an operator mistake
// worth surfacing at startup rather than at the first read, the way the
// launcher's configuration is; an unset root is not a mistake, it is a host
// without a library.
func LoadLibraryConfig() (LibraryConfig, error) {
	config := LibraryConfig{
		Root:             strings.TrimSpace(os.Getenv("CHROTE_LIBRARY_ROOT")),
		Author:           strings.TrimSpace(os.Getenv("CHROTE_LIBRARY_AUTHOR")),
		LibrarianSession: strings.TrimSpace(os.Getenv("CHROTE_LIBRARIAN_SESSION")),
		BeadsProject:     strings.TrimSpace(os.Getenv("CHROTE_LIBRARY_BEADS")),
	}
	if config.Root != "" {
		resolved, err := filepath.Abs(config.Root)
		if err != nil {
			return LibraryConfig{}, fmt.Errorf("CHROTE_LIBRARY_ROOT %q: %w", config.Root, err)
		}
		if canonical, err := filepath.EvalSymlinks(resolved); err == nil {
			resolved = canonical
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return LibraryConfig{}, fmt.Errorf("CHROTE_LIBRARY_ROOT %q: %w", config.Root, err)
		}
		if !info.IsDir() {
			return LibraryConfig{}, fmt.Errorf("CHROTE_LIBRARY_ROOT %q is not a directory", config.Root)
		}
		config.Root = filepath.Clean(resolved)
	}
	if config.Author != "" && !libraryAuthorPattern.MatchString(config.Author) {
		return LibraryConfig{}, fmt.Errorf("CHROTE_LIBRARY_AUTHOR %q must read \"Name <email>\"", config.Author)
	}
	return config, nil
}

// NewLibraryHandler creates the Library handler for a validated configuration.
func NewLibraryHandler(config LibraryConfig) *LibraryHandler {
	return &LibraryHandler{config: config}
}

// RegisterRoutes registers the Library routes.
func (h *LibraryHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/library/shelves", h.Shelves)
	mux.HandleFunc("GET /api/library/pages", h.Pages)
	mux.HandleFunc("GET /api/library/page", h.Page)
	mux.HandleFunc("GET /api/library/search", h.Search)
	mux.HandleFunc("GET /api/library/changes", h.Changes)
	mux.HandleFunc("PUT /api/library/page", h.SavePage)
}

// configured reports whether there is a corpus, and writes the reason when
// there is not. Shelves answers this question itself, because a tab that cannot
// say "No library is configured" has nothing to draw.
func (h *LibraryHandler) configured(w http.ResponseWriter) bool {
	if h.config.Root != "" {
		return true
	}
	core.WriteError(w, http.StatusNotFound, "NOT_CONFIGURED", "No library is configured")
	return false
}

// resolveLibraryPath turns a corpus-relative path into an absolute one inside
// the root. Every page CHROTE names is relative to the corpus, so an absolute
// path, a traversal, and a symlink out of the tree are all the same refusal.
func (h *LibraryHandler) resolveLibraryPath(requested string) (string, error) {
	trimmed := strings.TrimSpace(requested)
	if trimmed == "" {
		return "", errors.New("Missing path")
	}
	slashed := filepath.ToSlash(trimmed)
	if strings.HasPrefix(slashed, "/") {
		return "", errors.New("Path must be relative to the library root")
	}
	cleaned := path.Clean(slashed)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", errors.New("Path is outside the library")
	}
	absolute := filepath.Join(h.config.Root, filepath.FromSlash(cleaned))
	canonical, err := canonicalPathAllowMissing(absolute)
	if err != nil {
		return "", errors.New("Path is outside the library")
	}
	if canonical != h.config.Root && !strings.HasPrefix(canonical, h.config.Root+string(os.PathSeparator)) {
		return "", errors.New("Path is outside the library")
	}
	return canonical, nil
}

// libraryRelative is the corpus-relative spelling of an absolute path, which is
// the only spelling that leaves the server.
func (h *LibraryHandler) libraryRelative(absolute string) string {
	relative, err := filepath.Rel(h.config.Root, absolute)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(relative)
}

// libraryIgnoresDirectory keeps the walk out of git's own store and out of any
// other dot-directory: those hold no pages the operator wrote.
func libraryIgnoresDirectory(name string) bool {
	return strings.HasPrefix(name, ".")
}

func isLibraryPage(name string) bool {
	return strings.EqualFold(filepath.Ext(name), libraryPageExtension)
}

// git runs one git command inside the corpus root and returns its stdout and
// what git said when it refused. A refusal is not an empty corpus - the server
// reads a corpus the operator owns, and git will not read a repository owned by
// somebody else without being told it is safe - so every caller carries the
// refusal into its response rather than answering as if there were no history.
func (h *LibraryHandler) git(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, libraryGitTimeout)
	defer cancel()
	output, _, err := runGitCommand(ctx, h.config.Root, libraryGitOutputLimit, args...)
	return output, err
}

// libraryLogFormat is the field line every history read shares.
const libraryLogFormat = "--format=" + libraryRecordSeparator + "%H" + libraryFieldSeparator +
	"%cI" + libraryFieldSeparator + "%an" + libraryFieldSeparator + "%s"

// parseLibraryLog reads the records of a `git log` run with libraryLogFormat.
// With --name-only each record carries the paths the commit touched; without
// it, none.
func parseLibraryLog(output string) []LibraryChange {
	changes := make([]LibraryChange, 0)
	for _, record := range strings.Split(output, libraryRecordSeparator) {
		if strings.TrimSpace(record) == "" {
			continue
		}
		lines := strings.Split(record, "\n")
		fields := strings.Split(lines[0], libraryFieldSeparator)
		if len(fields) < 4 {
			continue
		}
		change := LibraryChange{
			LibraryCommit: LibraryCommit{
				Hash:    fields[0],
				Time:    fields[1],
				Author:  fields[2],
				Message: fields[3],
			},
			Files: make([]string, 0),
		}
		for _, line := range lines[1:] {
			file := strings.TrimSpace(line)
			if file != "" {
				change.Files = append(change.Files, file)
			}
		}
		changes = append(changes, change)
	}
	return changes
}

// libraryTitle is the page's first heading, or its file name when it has none.
func libraryTitle(content string, relative string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		heading := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		if heading != "" {
			return heading
		}
	}
	return strings.TrimSuffix(filepath.Base(relative), filepath.Ext(relative))
}

// readLibraryFile reads one page, bounded. A page longer than the bound is read
// to the bound and no further: the Library shows prose, not archives.
func readLibraryFile(absolute string) (string, error) {
	file, err := os.Open(absolute)
	if err != nil {
		return "", err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, libraryFileLimit))
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// Shelves handles GET /api/library/shelves.
func (h *LibraryHandler) Shelves(w http.ResponseWriter, r *http.Request) {
	response := LibraryShelvesResponse{
		Root:             h.config.Root,
		Shelves:          make([]LibraryShelf, 0),
		LibrarianSession: h.config.LibrarianSession,
		BeadsProject:     h.config.BeadsProject,
	}
	if h.config.Root == "" {
		core.WriteJSON(w, http.StatusOK, response)
		return
	}

	entries, err := os.ReadDir(h.config.Root)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() || libraryIgnoresDirectory(entry.Name()) {
			continue
		}
		response.Shelves = append(response.Shelves, LibraryShelf{
			Name:  entry.Name(),
			Path:  entry.Name(),
			Pages: countLibraryPages(filepath.Join(h.config.Root, entry.Name())),
		})
	}
	sort.Slice(response.Shelves, func(i, j int) bool {
		return response.Shelves[i].Name < response.Shelves[j].Name
	})
	core.WriteJSON(w, http.StatusOK, response)
}

func countLibraryPages(directory string) int {
	count := 0
	_ = filepath.WalkDir(directory, func(walked string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if walked != directory && libraryIgnoresDirectory(entry.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() && isLibraryPage(entry.Name()) {
			count++
		}
		return nil
	})
	return count
}

// Pages handles GET /api/library/pages?shelf= - the pages of one shelf, each
// with the title it gives itself and the commit that last touched it. One `git
// log` answers the whole shelf, so a shelf of thirty pages is one process.
func (h *LibraryHandler) Pages(w http.ResponseWriter, r *http.Request) {
	if !h.configured(w) {
		return
	}
	shelf := strings.TrimSpace(r.URL.Query().Get("shelf"))
	if shelf == "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Missing shelf")
		return
	}
	absoluteShelf, err := h.resolveLibraryPath(shelf)
	if err != nil {
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}
	info, err := os.Stat(absoluteShelf)
	if err != nil || !info.IsDir() {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", "No such shelf")
		return
	}

	lastChange, gitErr := h.lastChangeByPath(r.Context(), h.libraryRelative(absoluteShelf))
	pages := make([]LibraryPage, 0)
	_ = filepath.WalkDir(absoluteShelf, func(walked string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if walked != absoluteShelf && libraryIgnoresDirectory(entry.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || !isLibraryPage(entry.Name()) {
			return nil
		}
		relative := h.libraryRelative(walked)
		content, readErr := readLibraryFile(walked)
		if readErr != nil {
			content = ""
		}
		page := LibraryPage{Path: relative, Title: libraryTitle(content, relative)}
		if commit, known := lastChange[relative]; known {
			page.Updated = commit.Time
			page.Author = commit.Author
		}
		pages = append(pages, page)
		return nil
	})
	sort.Slice(pages, func(i, j int) bool { return pages[i].Path < pages[j].Path })
	response := LibraryPagesResponse{Pages: pages}
	if gitErr != nil {
		response.Error = gitErr.Error()
	}
	core.WriteJSON(w, http.StatusOK, response)
}

// lastChangeByPath maps every path under scope to the commit that last touched
// it. git log is newest first, so the first record naming a path is its last
// change.
func (h *LibraryHandler) lastChangeByPath(ctx context.Context, scope string) (map[string]LibraryCommit, error) {
	args := []string{"log", libraryLogFormat, "--name-only"}
	if scope != "" && scope != "." {
		args = append(args, "--", scope)
	}
	output, err := h.git(ctx, args...)
	last := make(map[string]LibraryCommit)
	for _, change := range parseLibraryLog(output) {
		for _, file := range change.Files {
			if _, seen := last[file]; !seen {
				last[file] = change.LibraryCommit
			}
		}
	}
	return last, err
}

// Page handles GET /api/library/page?path= - one page and its history.
func (h *LibraryHandler) Page(w http.ResponseWriter, r *http.Request) {
	if !h.configured(w) {
		return
	}
	requested := r.URL.Query().Get("path")
	absolute, err := h.resolveLibraryPath(requested)
	if err != nil {
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}
	info, err := os.Stat(absolute)
	if err != nil || info.IsDir() {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", "No such page")
		return
	}
	content, err := readLibraryFile(absolute)
	if err != nil {
		core.WriteError(w, http.StatusForbidden, "PERMISSION_DENIED", err.Error())
		return
	}

	relative := h.libraryRelative(absolute)
	history, gitErr := h.pageHistory(r.Context(), relative)
	response := LibraryPageResponse{
		Path:    relative,
		Title:   libraryTitle(content, relative),
		Content: content,
		History: history,
	}
	if len(history) > 0 {
		response.Updated = history[0].Time
		response.Author = history[0].Author
	}
	if gitErr != nil {
		response.Error = gitErr.Error()
	}
	core.WriteJSON(w, http.StatusOK, response)
}

func (h *LibraryHandler) pageHistory(ctx context.Context, relative string) ([]LibraryCommit, error) {
	output, err := h.git(ctx, "log", "-n", strconv.Itoa(libraryHistoryLimit), libraryLogFormat, "--", relative)
	changes := parseLibraryLog(output)
	history := make([]LibraryCommit, 0, len(changes))
	for _, change := range changes {
		history = append(history, change.LibraryCommit)
	}
	return history, err
}

// Search handles GET /api/library/search?q= - a case-insensitive walk of every
// page, name matches first. There is no index: the corpus is prose the operator
// wrote, and a walk of it is cheaper than a store that can go stale.
func (h *LibraryHandler) Search(w http.ResponseWriter, r *http.Request) {
	if !h.configured(w) {
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		core.WriteJSON(w, http.StatusOK, []LibrarySearchResult{})
		return
	}
	needle := strings.ToLower(query)

	type candidate struct {
		result    LibrarySearchResult
		nameMatch bool
	}
	candidates := make([]candidate, 0)
	read := 0
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
		if read >= librarySearchCandidateLimit {
			return fs.SkipAll
		}
		read++

		relative := h.libraryRelative(walked)
		content, readErr := readLibraryFile(walked)
		if readErr != nil {
			return nil
		}
		nameMatch := strings.Contains(strings.ToLower(relative), needle)
		line, snippet := firstLibraryMatch(content, needle)
		if !nameMatch && line == 0 {
			return nil
		}
		candidates = append(candidates, candidate{
			result: LibrarySearchResult{
				Path:    relative,
				Title:   libraryTitle(content, relative),
				Line:    line,
				Snippet: snippet,
			},
			nameMatch: nameMatch,
		})
		return nil
	})

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].nameMatch != candidates[j].nameMatch {
			return candidates[i].nameMatch
		}
		return candidates[i].result.Path < candidates[j].result.Path
	})
	results := make([]LibrarySearchResult, 0, len(candidates))
	for _, found := range candidates {
		if len(results) >= librarySearchLimit {
			break
		}
		results = append(results, found.result)
	}
	core.WriteJSON(w, http.StatusOK, results)
}

// firstLibraryMatch returns the 1-based line holding the first match and that
// line as a snippet, or zero and the empty string when the text has none.
func firstLibraryMatch(content, needle string) (int, string) {
	for index, line := range strings.Split(content, "\n") {
		if !strings.Contains(strings.ToLower(line), needle) {
			continue
		}
		snippet := strings.TrimSpace(line)
		if len(snippet) > librarySnippetLimit {
			snippet = snippet[:librarySnippetLimit]
		}
		return index + 1, snippet
	}
	return 0, ""
}

// Changes handles GET /api/library/changes?limit= - what has arrived lately,
// each commit with the pages it touched.
func (h *LibraryHandler) Changes(w http.ResponseWriter, r *http.Request) {
	if !h.configured(w) {
		return
	}
	limit := libraryChangesDefault
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "limit must be a positive whole number")
			return
		}
		limit = parsed
	}
	if limit > libraryChangesLimit {
		limit = libraryChangesLimit
	}
	output, gitErr := h.git(r.Context(), "log", "-n", strconv.Itoa(limit), libraryLogFormat, "--name-only")
	response := LibraryChangesResponse{Changes: parseLibraryLog(output)}
	if gitErr != nil {
		response.Error = gitErr.Error()
	}
	core.WriteJSON(w, http.StatusOK, response)
}

// SavePage handles PUT /api/library/page - the operator's own correction,
// written and committed in one step so the corpus never holds a change nobody
// signed. A page somebody else is already editing is refused rather than
// swallowed into the operator's commit.
func (h *LibraryHandler) SavePage(w http.ResponseWriter, r *http.Request) {
	if !h.configured(w) {
		return
	}
	if h.config.Author == "" {
		core.WriteError(w, http.StatusConflict, "NOT_CONFIGURED",
			"No library author is configured, so an edit cannot be attributed; set CHROTE_LIBRARY_AUTHOR to \"Name <email>\"")
		return
	}

	var request LibrarySaveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, libraryFileLimit)).Decode(&request); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Body is not a library save request")
		return
	}
	absolute, err := h.resolveLibraryPath(request.Path)
	if err != nil {
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}
	info, err := os.Stat(absolute)
	if err != nil || info.IsDir() {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", "No such page")
		return
	}
	relative := h.libraryRelative(absolute)

	// A status git would not answer leaves the question open, and a save made
	// on an open question is the one that swallows somebody else's edit.
	status, gitErr := h.git(r.Context(), "status", "--porcelain", "--", relative)
	if gitErr != nil {
		core.WriteError(w, http.StatusInternalServerError, "INTERNAL", gitErr.Error())
		return
	}
	if dirty := strings.TrimSpace(status); dirty != "" {
		core.WriteError(w, http.StatusConflict, "DIRTY",
			fmt.Sprintf("%s already has an uncommitted change in the corpus; commit or discard it before saving from here", relative))
		return
	}

	if err := os.WriteFile(absolute, []byte(request.Content), info.Mode().Perm()); err != nil {
		core.WriteError(w, http.StatusForbidden, "PERMISSION_DENIED", err.Error())
		return
	}

	// A save that changed nothing is not a commit. The operator gets the entry
	// that is already there, which is the truth about when the page last moved.
	written, gitErr := h.git(r.Context(), "status", "--porcelain", "--", relative)
	if gitErr != nil {
		core.WriteError(w, http.StatusInternalServerError, "INTERNAL", gitErr.Error())
		return
	}
	if strings.TrimSpace(written) != "" {
		message := strings.TrimSpace(request.Summary)
		if message == "" {
			message = "Edit " + relative
		}
		name, email := splitLibraryAuthor(h.config.Author)
		if _, err := h.git(r.Context(), "add", "--", relative); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		if _, err := h.git(r.Context(),
			"-c", "user.name="+name,
			"-c", "user.email="+email,
			"commit", "-m", message, "--", relative,
		); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
	}

	history, gitErr := h.pageHistory(r.Context(), relative)
	if gitErr != nil {
		core.WriteError(w, http.StatusInternalServerError, "INTERNAL", gitErr.Error())
		return
	}
	if len(history) == 0 {
		core.WriteJSON(w, http.StatusOK, LibraryCommit{})
		return
	}
	core.WriteJSON(w, http.StatusOK, history[0])
}

// splitLibraryAuthor takes the two halves of a validated "Name <email>".
func splitLibraryAuthor(author string) (string, string) {
	opening := strings.LastIndex(author, "<")
	closing := strings.LastIndex(author, ">")
	if opening < 0 || closing < opening {
		return author, ""
	}
	return strings.TrimSpace(author[:opening]), strings.TrimSpace(author[opening+1 : closing])
}
