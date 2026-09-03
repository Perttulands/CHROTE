// Package api provides HTTP handlers for the API.
//
// This file answers one question: what does an agent started in this folder,
// under this harness, as this user, already have in its head before the
// operator types a word? Three layers make up the answer — the instruction
// stack in the order the harness loads it, the skills that are reachable, and
// the memories that were written for this folder — and every row names where it
// came from.
//
// The server reads as its own account and says so. A file it cannot read is
// listed with readable false rather than dropped: an instruction the operator
// cannot see through CHROTE is still an instruction the agent loaded, and
// hiding it would be the one failure this surface exists to prevent. Nothing
// here is cached or watched; the panel and the tab each ask once.
package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chrote/server/internal/core"
)

// The harnesses CHROTE can resolve a context for.
const (
	harnessClaudeCode = "claude-code"
	harnessCodex      = "codex"
)

// Where a row sits in the stack, in the harness's own loading order.
const (
	scopeUser     = "user"
	scopeAncestor = "ancestor"
	scopeProject  = "project"
)

// What a row is: an instruction file the harness reads, or its configuration.
const (
	kindClaudeMd = "CLAUDE.md"
	kindAgentsMd = "AGENTS.md"
	kindSettings = "settings"
)

// Where a skill was reached from.
const (
	sourceProject = "project"
	sourceUser    = "user"
	sourceShared  = "shared"
)

// Who wrote a memory.
const (
	memoryClaudeAuto = "claude-auto"
	memoryCodex      = "codex"
	memoryBd         = "bd"
)

// The Claude Code auto-memory index; it reads as the file it is, not as a slug.
const claudeMemoryIndex = "MEMORY.md"

// codexMemoryFile is where Codex 0.152 keeps the durable handbook. It is one
// global Markdown file partitioned into "# Task Group:" blocks, each carrying
// an "applies_to: cwd=<path>" line — the folder key is inside the document,
// not in the path, so resolving a folder means reading the blocks.
var codexMemoryFile = filepath.Join(".codex", "memories", "MEMORY.md")

const codexTaskGroupPrefix = "# Task Group:"
const codexAppliesToPrefix = "applies_to:"

// How much of a handbook or frontmatter block is read before giving up. Both
// bounds exist so one enormous file cannot turn a request into a scan.
const (
	codexMemoryMaxLines = 200000
	frontmatterMaxLines = 64
)

// AgentInstruction is one file in the stack, and where it sits in it.
type AgentInstruction struct {
	Path  string `json:"path"`
	Scope string `json:"scope"`
	Kind  string `json:"kind"`
	// False when the file exists but this server's account cannot open it.
	Readable bool  `json:"readable"`
	Size     int64 `json:"size"`
	// The base name a symlink points at, so a stack that lists CLAUDE.md and
	// AGENTS.md separately says when they are the same file.
	Link string `json:"link,omitempty"`
}

// AgentSkill is one skill the harness can reach, named as its own file names it.
type AgentSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Source      string `json:"source"`
}

// AgentMemory is one thing an agent was told to remember about this folder.
type AgentMemory struct {
	Kind string `json:"kind"`
	// Empty for a memory that is not a file of its own, such as a bd memory.
	Path     string `json:"path"`
	Title    string `json:"title"`
	Updated  string `json:"updated"`
	Readable bool   `json:"readable"`
}

// AgentContextResponse is the body of GET /api/agent/context.
type AgentContextResponse struct {
	Folder       string             `json:"folder"`
	Harness      string             `json:"harness"`
	User         string             `json:"user"`
	Instructions []AgentInstruction `json:"instructions"`
	Skills       []AgentSkill       `json:"skills"`
	Memories     []AgentMemory      `json:"memories"`
}

// AgentFileResponse is the body of GET /api/agent/file.
type AgentFileResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// AgentTender names the session that tends the instruction layer, as the host
// configured it. Empty fields mean the host said nothing, and the desk says so.
// It is the body of GET /api/agent/tender.
type AgentTender struct {
	Session string `json:"session"`
	Beads   string `json:"beads"`
	Folder  string `json:"folder"`
}

// AgentContextHandler resolves what an agent sees.
type AgentContextHandler struct {
	bdCommand   string
	execTimeout time.Duration
	// How a Unix user becomes a home directory. A field so a test can answer
	// with a temporary home instead of the host's real accounts.
	homeForUser func(string) (string, error)
	// The roots a folder has to sit under, unless it sits under the home.
	allowedRoots func() []string
	// The tender's configuration, read from the environment at construction.
	tender AgentTender
}

// NewAgentContextHandler creates a handler over the host's own accounts.
func NewAgentContextHandler() *AgentContextHandler {
	bdCommand := os.Getenv("CHROTE_BD_COMMAND")
	if bdCommand == "" {
		bdCommand = "bd"
	}
	return &AgentContextHandler{
		bdCommand:    bdCommand,
		execTimeout:  30 * time.Second,
		homeForUser:  defaultHomeForUser,
		allowedRoots: core.GetAllowedRoots,
		tender: AgentTender{
			Session: strings.TrimSpace(os.Getenv("CHROTE_TENDER_SESSION")),
			Beads:   strings.TrimSpace(os.Getenv("CHROTE_TENDER_BEADS")),
			Folder:  strings.TrimSpace(os.Getenv("CHROTE_TENDER_FOLDER")),
		},
	}
}

// RegisterRoutes registers the agent-context routes on the given mux.
func (h *AgentContextHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/agent/context", h.Context)
	mux.HandleFunc("GET /api/agent/file", h.File)
	mux.HandleFunc("GET /api/agent/tender", h.Tender)
}

// defaultHomeForUser answers with the named account's home, or with this
// process's own when no user was named.
func defaultHomeForUser(unixUser string) (string, error) {
	unixUser = strings.TrimSpace(unixUser)
	if unixUser == "" {
		if home := os.Getenv("HOME"); home != "" {
			return home, nil
		}
		current, err := user.Current()
		if err != nil {
			return "", err
		}
		return current.HomeDir, nil
	}
	account, err := user.Lookup(unixUser)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(account.HomeDir) == "" {
		return "", errors.New("account has no home directory")
	}
	return account.HomeDir, nil
}

// Context handles GET /api/agent/context.
func (h *AgentContextHandler) Context(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	harness := strings.TrimSpace(query.Get("harness"))
	if harness != harnessClaudeCode && harness != harnessCodex {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST",
			"harness must be "+harnessClaudeCode+" or "+harnessCodex)
		return
	}
	unixUser := strings.TrimSpace(query.Get("user"))
	home, err := h.homeForUser(unixUser)
	if err != nil {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Cannot resolve the home directory: "+err.Error())
		return
	}
	folder, code, message := h.validateFolder(query.Get("folder"), home)
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, message)
		return
	}

	core.WriteJSON(w, http.StatusOK, h.resolve(folder, harness, unixUser, home))
}

// File handles GET /api/agent/file. The path has to be one the context route
// just listed for the same folder and harness, so this route reads exactly what
// the panel drew and nothing else.
func (h *AgentContextHandler) File(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	harness := strings.TrimSpace(query.Get("harness"))
	if harness != harnessClaudeCode && harness != harnessCodex {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST",
			"harness must be "+harnessClaudeCode+" or "+harnessCodex)
		return
	}
	requested := strings.TrimSpace(query.Get("path"))
	if requested == "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Missing required parameter: path")
		return
	}
	unixUser := strings.TrimSpace(query.Get("user"))
	home, err := h.homeForUser(unixUser)
	if err != nil {
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Cannot resolve the home directory: "+err.Error())
		return
	}
	folder, code, message := h.validateFolder(query.Get("folder"), home)
	if code != "" {
		core.WriteError(w, core.GetErrorStatusCode(code), code, message)
		return
	}

	resolved := h.resolve(folder, harness, unixUser, home)
	if !resolvedListsPath(resolved, filepath.Clean(requested)) {
		core.WriteError(w, http.StatusForbidden, "FORBIDDEN",
			"Not a file this folder's stack lists: "+requested)
		return
	}
	content, err := os.ReadFile(filepath.Clean(requested))
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			core.WriteError(w, http.StatusForbidden, "FORBIDDEN",
				"Not readable by the server as user "+effectiveUsername()+": "+requested)
			return
		}
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Cannot read "+requested+": "+err.Error())
		return
	}
	core.WriteJSON(w, http.StatusOK, AgentFileResponse{Path: filepath.Clean(requested), Content: string(content)})
}

// Tender handles GET /api/agent/tender: the tender the host configured. The
// folders the Agents tab offers come from GET /api/workspaces.
func (h *AgentContextHandler) Tender(w http.ResponseWriter, r *http.Request) {
	core.WriteJSON(w, http.StatusOK, h.tender)
}

// validateFolder answers with the folder to resolve, or with the code and
// message that say why it cannot be resolved. The home is always allowed: it is
// where every stack starts, and a host does not have to configure it as a root
// for the operator to ask what an agent started there would see.
func (h *AgentContextHandler) validateFolder(requested, home string) (string, string, string) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", "BAD_REQUEST", "Missing required parameter: folder"
	}
	if !filepath.IsAbs(requested) {
		return "", "BAD_REQUEST", "folder must be an absolute path: " + requested
	}
	folder := filepath.Clean(requested)
	if !isPathUnder(folder, h.allowedRoots()) && !core.IsPathUnderRoot(folder, home) {
		return "", "FORBIDDEN", "Folder is not in the allowed roots or the user's home: " + folder
	}
	info, err := os.Stat(folder)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return "", "FORBIDDEN", "Folder is not readable by the server as user " + effectiveUsername() + ": " + folder
		}
		return "", "NOT_FOUND", "Folder does not exist: " + folder
	}
	if !info.IsDir() {
		return "", "BAD_REQUEST", "Not a folder: " + folder
	}
	return folder, "", ""
}

// resolve is the whole answer, for one folder under one harness.
func (h *AgentContextHandler) resolve(folder, harness, unixUser, home string) AgentContextResponse {
	response := AgentContextResponse{
		Folder:       folder,
		Harness:      harness,
		User:         unixUser,
		Instructions: []AgentInstruction{},
		Skills:       []AgentSkill{},
		Memories:     []AgentMemory{},
	}
	if harness == harnessCodex {
		response.Instructions = codexInstructions(home, folder)
		response.Skills = codexSkills(home, folder)
		response.Memories = codexMemories(home, folder)
	} else {
		response.Instructions = claudeInstructions(home, folder)
		response.Skills = claudeSkills(home, folder)
		response.Memories = claudeMemories(home, folder)
	}
	response.Memories = append(response.Memories, h.bdMemories(folder)...)
	return response
}

// resolvedListsPath reports whether the stack just resolved names this file.
func resolvedListsPath(resolved AgentContextResponse, path string) bool {
	for _, instruction := range resolved.Instructions {
		if instruction.Path == path {
			return true
		}
	}
	for _, memory := range resolved.Memories {
		if memory.Path == path {
			return true
		}
	}
	return false
}

// describeFile answers with the row for a file, and whether there is a row at
// all. A file that is not there has no row; a file that is there but closed to
// this account has one, saying so.
func describeFile(path string) (AgentInstruction, bool) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return AgentInstruction{Path: path, Readable: false}, true
		}
		return AgentInstruction{}, false
	}
	if !info.Mode().IsRegular() {
		return AgentInstruction{}, false
	}
	row := AgentInstruction{Path: path, Size: info.Size(), Readable: true}
	if target, err := os.Readlink(path); err == nil {
		row.Link = filepath.Base(target)
	}
	file, err := os.Open(path)
	if err != nil {
		row.Readable = false
		return row, true
	}
	_ = file.Close()
	return row, true
}

// instructionStack collects rows in loading order, once each.
type instructionStack struct {
	rows []AgentInstruction
	seen map[string]bool
}

func newInstructionStack() *instructionStack {
	return &instructionStack{rows: []AgentInstruction{}, seen: map[string]bool{}}
}

// add appends the file when it exists and has not been listed already, and
// reports whether it did.
func (s *instructionStack) add(path, scope, kind string) bool {
	path = filepath.Clean(path)
	if s.seen[path] {
		return false
	}
	row, ok := describeFile(path)
	if !ok {
		return false
	}
	row.Scope, row.Kind = scope, kind
	s.seen[path] = true
	s.rows = append(s.rows, row)
	return true
}

// addClaudePair lists a directory's CLAUDE.md and, beside it, its AGENTS.md.
// The sibling takes the CLAUDE.md's scope, because it is the same rung of the
// stack — on this host CLAUDE.md is usually a symlink to it.
func (s *instructionStack) addClaudePair(dir, scope string) {
	if s.add(filepath.Join(dir, kindClaudeMd), scope, kindClaudeMd) {
		s.add(filepath.Join(dir, kindAgentsMd), scope, kindAgentsMd)
	}
}

// ancestorsOf lists every directory from the filesystem root down to the
// folder's parent. The folder itself is the project scope and is added by the
// caller, so it is deliberately not here.
func ancestorsOf(folder string) []string {
	folder = filepath.Clean(folder)
	var reversed []string
	for dir := filepath.Dir(folder); ; dir = filepath.Dir(dir) {
		reversed = append(reversed, dir)
		if dir == filepath.Dir(dir) {
			break
		}
	}
	ancestors := make([]string, 0, len(reversed))
	for i := len(reversed) - 1; i >= 0; i-- {
		ancestors = append(ancestors, reversed[i])
	}
	return ancestors
}

// claudeInstructions is the stack Claude Code loads, in its order: the user's
// own files, then every ancestor from the root down, then the project.
func claudeInstructions(home, folder string) []AgentInstruction {
	stack := newInstructionStack()
	stack.addClaudePair(filepath.Join(home, ".claude"), scopeUser)
	stack.addClaudePair(home, scopeUser)
	stack.add(filepath.Join(home, ".claude", "settings.json"), scopeUser, kindSettings)

	for _, ancestor := range ancestorsOf(folder) {
		stack.addClaudePair(ancestor, scopeAncestor)
	}
	stack.addClaudePair(folder, scopeProject)
	stack.add(filepath.Join(folder, ".claude", "settings.json"), scopeProject, kindSettings)
	stack.add(filepath.Join(folder, ".claude", "settings.local.json"), scopeProject, kindSettings)
	return stack.rows
}

// codexInstructions is the same stack as Codex reads it: AGENTS.md all the way
// down, and config.toml as the harness's own configuration.
func codexInstructions(home, folder string) []AgentInstruction {
	stack := newInstructionStack()
	stack.add(filepath.Join(home, ".codex", kindAgentsMd), scopeUser, kindAgentsMd)
	stack.add(filepath.Join(home, ".codex", "config.toml"), scopeUser, kindSettings)
	stack.add(filepath.Join(home, kindAgentsMd), scopeUser, kindAgentsMd)

	for _, ancestor := range ancestorsOf(folder) {
		stack.add(filepath.Join(ancestor, kindAgentsMd), scopeAncestor, kindAgentsMd)
	}
	stack.add(filepath.Join(folder, kindAgentsMd), scopeProject, kindAgentsMd)
	return stack.rows
}

// skillSourceRank orders the sources the way the stack reads: the nearest
// first.
var skillSourceRank = map[string]int{sourceProject: 0, sourceUser: 1, sourceShared: 2}

// collectSkills lists every directory under dir that holds a SKILL.md.
//
// Every entry is resolved to the directory it really is, so the path names the
// file the operator would edit rather than the link that reached it, and the
// same shared skill reached through two directories is listed once.
//
// Where the caller says so, an entry that resolves outside the directory it was
// found in is a different source: a shared tree the user linked into a harness's
// skills directory belongs to every harness, not to this one.
func collectSkills(dir, source string, sharedWhenLinked bool, seen map[string]bool, into *[]AgentSkill) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			continue
		}
		skillSource := source
		if sharedWhenLinked && !core.IsPathUnderRoot(resolved, dir) {
			skillSource = sourceShared
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			continue
		}
		manifest := filepath.Join(resolved, "SKILL.md")
		if manifestInfo, err := os.Stat(manifest); err != nil || !manifestInfo.Mode().IsRegular() {
			continue
		}
		if seen[resolved] {
			continue
		}
		seen[resolved] = true
		name, description := skillFrontmatter(manifest)
		if name == "" {
			name = entry.Name()
		}
		*into = append(*into, AgentSkill{Name: name, Description: description, Path: resolved, Source: skillSource})
	}
}

func sortSkills(skills []AgentSkill) {
	sort.SliceStable(skills, func(i, j int) bool {
		left, right := skills[i], skills[j]
		if skillSourceRank[left.Source] != skillSourceRank[right.Source] {
			return skillSourceRank[left.Source] < skillSourceRank[right.Source]
		}
		return left.Name < right.Name
	})
}

// claudeSkills lists the project's skills, then the user's own, then the shared
// trees the user's skills directory links into.
func claudeSkills(home, folder string) []AgentSkill {
	skills := []AgentSkill{}
	seen := map[string]bool{}
	collectSkills(filepath.Join(folder, ".claude", "skills"), sourceProject, false, seen, &skills)
	collectSkills(filepath.Join(home, ".claude", "skills"), sourceUser, true, seen, &skills)
	sortSkills(skills)
	return skills
}

// codexSkills is the same rule over Codex's own directories, plus ~/.agents,
// which is where a shared tree is linked for every harness at once.
func codexSkills(home, folder string) []AgentSkill {
	skills := []AgentSkill{}
	seen := map[string]bool{}
	collectSkills(filepath.Join(folder, ".codex", "skills"), sourceProject, false, seen, &skills)
	collectSkills(filepath.Join(home, ".codex", "skills"), sourceUser, true, seen, &skills)
	collectSkills(filepath.Join(home, ".agents", "skills"), sourceUser, true, seen, &skills)
	sortSkills(skills)
	return skills
}

// skillFrontmatter reads the name and description a SKILL.md declares.
func skillFrontmatter(path string) (string, string) {
	fields := readFrontmatter(path)
	return fields["name"], fields["description"]
}

// readFrontmatter parses the leading YAML block's top-level scalars. It is
// deliberately not a YAML parser: these files declare a name and a description
// on one line each, and anything nested belongs to the harness, not here.
func readFrontmatter(path string) map[string]string {
	fields := map[string]string{}
	file, err := os.Open(path)
	if err != nil {
		return fields
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return fields
	}
	for line := 0; line < frontmatterMaxLines && scanner.Scan(); line++ {
		text := scanner.Text()
		if strings.TrimSpace(text) == "---" {
			break
		}
		if text == "" || text[0] == ' ' || text[0] == '\t' || text[0] == '#' {
			continue
		}
		key, value, found := strings.Cut(text, ":")
		if !found {
			continue
		}
		fields[strings.TrimSpace(key)] = unquote(strings.TrimSpace(value))
	}
	return fields
}

func unquote(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// claudeMemorySlug is how Claude Code names a folder's own directory: the
// absolute path with every separator turned into a dash.
func claudeMemorySlug(folder string) string {
	return strings.ReplaceAll(filepath.Clean(folder), string(os.PathSeparator), "-")
}

// claudeMemories lists the auto-memories written for this folder: the index
// first, then the rest with the newest at the top, because a memory is read to
// find out what the agent was last told.
func claudeMemories(home, folder string) []AgentMemory {
	dir := filepath.Join(home, ".claude", "projects", claudeMemorySlug(folder), "memory")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []AgentMemory{}
	}
	var index []AgentMemory
	var rest []AgentMemory
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		row, ok := describeFile(path)
		if !ok {
			continue
		}
		memory := AgentMemory{
			Kind:     memoryClaudeAuto,
			Path:     path,
			Title:    memoryTitle(path, entry.Name(), row.Readable),
			Updated:  modifiedAt(path),
			Readable: row.Readable,
		}
		if entry.Name() == claudeMemoryIndex {
			index = append(index, memory)
			continue
		}
		rest = append(rest, memory)
	}
	sort.SliceStable(rest, func(i, j int) bool {
		if rest[i].Updated != rest[j].Updated {
			return rest[i].Updated > rest[j].Updated
		}
		return rest[i].Title < rest[j].Title
	})
	return append(index, rest...)
}

// memoryTitle is the name the memory gives itself: its frontmatter name, else
// its first heading, else the file name without its extension. The index keeps
// its file name, because that is what it is called everywhere else.
func memoryTitle(path, fileName string, readable bool) string {
	if fileName == claudeMemoryIndex {
		return claudeMemoryIndex
	}
	if readable {
		if name := readFrontmatter(path)["name"]; name != "" {
			return name
		}
		if heading := firstHeading(path); heading != "" {
			return heading
		}
	}
	return strings.TrimSuffix(fileName, ".md")
}

func firstHeading(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 0; line < frontmatterMaxLines*4 && scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(text, "#") {
			return strings.TrimSpace(strings.TrimLeft(text, "#"))
		}
	}
	return ""
}

func modifiedAt(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return info.ModTime().UTC().Format(time.RFC3339)
}

// codexMemories reads the folder's blocks out of Codex's one handbook.
//
// Codex 0.152 keeps every memory in ~/.codex/memories/MEMORY.md, partitioned
// into "# Task Group:" blocks. The folder key is a field inside the block —
// "applies_to: cwd=<path>; reuse_rule=..." — so a folder's memories are the
// blocks whose cwd is that folder or a folder above it. There is no per-folder
// file, slug or hash to look up.
func codexMemories(home, folder string) []AgentMemory {
	path := filepath.Join(home, codexMemoryFile)
	row, ok := describeFile(path)
	if !ok {
		return []AgentMemory{}
	}
	updated := modifiedAt(path)
	if !row.Readable {
		return []AgentMemory{{
			Kind:     memoryCodex,
			Path:     path,
			Title:    claudeMemoryIndex,
			Updated:  updated,
			Readable: false,
		}}
	}
	memories := []AgentMemory{}
	for _, title := range codexTaskGroups(path, folder) {
		memories = append(memories, AgentMemory{
			Kind:     memoryCodex,
			Path:     path,
			Title:    title,
			Updated:  updated,
			Readable: true,
		})
	}
	return memories
}

// codexTaskGroups lists the titles of the handbook's blocks that apply to this
// folder.
func codexTaskGroups(path, folder string) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var titles []string
	title := ""
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 0; line < codexMemoryMaxLines && scanner.Scan(); line++ {
		text := scanner.Text()
		if strings.HasPrefix(text, codexTaskGroupPrefix) {
			title = strings.TrimSpace(strings.TrimPrefix(text, codexTaskGroupPrefix))
			continue
		}
		if title == "" || !strings.HasPrefix(strings.TrimSpace(text), codexAppliesToPrefix) {
			continue
		}
		if codexAppliesTo(strings.TrimSpace(text), folder) {
			titles = append(titles, title)
		}
		title = ""
	}
	return titles
}

// codexAppliesTo reads the cwd field of an applies_to line and reports whether
// the folder is inside it. A block written for a folder above this one still
// applies to it: that is what Codex's "cwd family" means.
func codexAppliesTo(line, folder string) bool {
	for _, field := range strings.Split(strings.TrimPrefix(line, codexAppliesToPrefix), ";") {
		field = strings.TrimSpace(field)
		if !strings.HasPrefix(field, "cwd=") {
			continue
		}
		value := strings.TrimPrefix(field, "cwd=")
		for _, token := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
			token = filepath.Clean(strings.TrimSpace(token))
			if filepath.IsAbs(token) && core.IsPathUnderRoot(folder, token) {
				return true
			}
		}
	}
	return false
}

// bdMemories asks bd for the folder's own memories. A folder with no store, or
// a bd that will not answer, contributes nothing: the rest of the stack is
// still worth showing, and an error here would hide it.
func (h *AgentContextHandler) bdMemories(folder string) []AgentMemory {
	ctx, cancel := context.WithTimeout(context.Background(), h.execTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, h.bdCommand, "memories", "--json")
	cmd.Dir = folder
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	var entries map[string]string
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil
	}
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	memories := make([]AgentMemory, 0, len(keys))
	for _, key := range keys {
		memories = append(memories, AgentMemory{Kind: memoryBd, Title: key, Readable: true})
	}
	return memories
}

// workspaceInstructionFiles is what a folder can own of its own instruction
// layer. The count on a workspace row is how many of these exist.
var workspaceInstructionFiles = []string{
	kindClaudeMd,
	kindAgentsMd,
	filepath.Join(".claude", "settings.json"),
	filepath.Join(".claude", "settings.local.json"),
}

func countInstructionFiles(dir string) int {
	count := 0
	for _, name := range workspaceInstructionFiles {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && info.Mode().IsRegular() {
			count++
		}
	}
	return count
}
