package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// agentTestHost is a whole world in a temporary directory: a home, a root of
// folders, and nothing of the machine the test runs on. Every test here builds
// one, so a resolution rule is proved against files the test wrote rather than
// against whatever the operator happens to have in his own home.
type agentTestHost struct {
	home    string
	root    string
	handler *AgentContextHandler
}

func newAgentTestHost(t *testing.T) *agentTestHost {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	home := filepath.Join(base, "home", "operator")
	root := filepath.Join(base, "root")
	mkdirAll(t, home)
	mkdirAll(t, root)

	return &agentTestHost{
		home: home,
		root: root,
		handler: &AgentContextHandler{
			// A command that does not exist: bd contributes nothing, and the
			// rest of the stack is what these tests are about.
			bdCommand:    filepath.Join(base, "no-such-bd"),
			execTimeout:  5 * time.Second,
			homeForUser:  func(string) (string, error) { return home, nil },
			allowedRoots: func() []string { return []string{root} },
		},
	}
}

func mkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func symlink(t *testing.T, target, path string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("symlink %s -> %s: %v", path, target, err)
	}
}

func instructionPaths(rows []AgentInstruction) []string {
	paths := make([]string, 0, len(rows))
	for _, row := range rows {
		paths = append(paths, row.Path)
	}
	return paths
}

func skillNames(skills []AgentSkill) []string {
	names := make([]string, 0, len(skills))
	for _, skill := range skills {
		names = append(names, skill.Name)
	}
	return names
}

func memoryTitles(memories []AgentMemory) []string {
	titles := make([]string, 0, len(memories))
	for _, memory := range memories {
		titles = append(titles, memory.Title)
	}
	return titles
}

func assertSequence(t *testing.T, got, want []string, what string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", what, got, want)
		}
	}
}

// The instruction stack is the whole point of the route: the harness's own
// order, every rung named, and the sibling AGENTS.md carrying its CLAUDE.md's
// scope because it is the same rung.
func TestAgentContext_ResolvesTheInstructionStackInLoadingOrder(t *testing.T) {
	tests := []struct {
		name    string
		harness string
		build   func(t *testing.T, host *agentTestHost) string
		want    func(host *agentTestHost, folder string) []string
	}{
		{
			name:    "claude code reads the user, every ancestor and the project",
			harness: harnessClaudeCode,
			build: func(t *testing.T, host *agentTestHost) string {
				folder := filepath.Join(host.root, "project")
				writeFile(t, filepath.Join(host.home, ".claude", "CLAUDE.md"), "# user\n")
				writeFile(t, filepath.Join(host.home, "CLAUDE.md"), "# home\n")
				writeFile(t, filepath.Join(host.home, ".claude", "settings.json"), "{}\n")
				writeFile(t, filepath.Join(host.root, "CLAUDE.md"), "# root\n")
				writeFile(t, filepath.Join(folder, "CLAUDE.md"), "# project\n")
				writeFile(t, filepath.Join(folder, ".claude", "settings.json"), "{}\n")
				writeFile(t, filepath.Join(folder, ".claude", "settings.local.json"), "{}\n")
				return folder
			},
			want: func(host *agentTestHost, folder string) []string {
				return []string{
					filepath.Join(host.home, ".claude", "CLAUDE.md"),
					filepath.Join(host.home, "CLAUDE.md"),
					filepath.Join(host.home, ".claude", "settings.json"),
					filepath.Join(host.root, "CLAUDE.md"),
					filepath.Join(folder, "CLAUDE.md"),
					filepath.Join(folder, ".claude", "settings.json"),
					filepath.Join(folder, ".claude", "settings.local.json"),
				}
			},
		},
		{
			name:    "an AGENTS.md beside a CLAUDE.md is the same rung",
			harness: harnessClaudeCode,
			build: func(t *testing.T, host *agentTestHost) string {
				folder := filepath.Join(host.root, "project")
				writeFile(t, filepath.Join(folder, "AGENTS.md"), "# project\n")
				symlink(t, "AGENTS.md", filepath.Join(folder, "CLAUDE.md"))
				return folder
			},
			want: func(host *agentTestHost, folder string) []string {
				return []string{
					filepath.Join(folder, "CLAUDE.md"),
					filepath.Join(folder, "AGENTS.md"),
				}
			},
		},
		{
			name:    "an AGENTS.md with no CLAUDE.md beside it is not Claude Code's rung",
			harness: harnessClaudeCode,
			build: func(t *testing.T, host *agentTestHost) string {
				folder := filepath.Join(host.root, "project")
				writeFile(t, filepath.Join(folder, "AGENTS.md"), "# project\n")
				return folder
			},
			want: func(host *agentTestHost, folder string) []string { return []string{} },
		},
		{
			name:    "codex reads AGENTS.md down the tree and its own config",
			harness: harnessCodex,
			build: func(t *testing.T, host *agentTestHost) string {
				folder := filepath.Join(host.root, "project")
				writeFile(t, filepath.Join(host.home, ".codex", "AGENTS.md"), "# user\n")
				writeFile(t, filepath.Join(host.home, ".codex", "config.toml"), "model = \"x\"\n")
				writeFile(t, filepath.Join(host.root, "AGENTS.md"), "# root\n")
				writeFile(t, filepath.Join(folder, "AGENTS.md"), "# project\n")
				writeFile(t, filepath.Join(folder, "CLAUDE.md"), "# not codex's\n")
				return folder
			},
			want: func(host *agentTestHost, folder string) []string {
				return []string{
					filepath.Join(host.home, ".codex", "AGENTS.md"),
					filepath.Join(host.home, ".codex", "config.toml"),
					filepath.Join(host.root, "AGENTS.md"),
					filepath.Join(folder, "AGENTS.md"),
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := newAgentTestHost(t)
			folder := tt.build(t, host)
			resolved := host.handler.resolve(folder, tt.harness, "", host.home)
			assertSequence(t, instructionPaths(resolved.Instructions), tt.want(host, folder), "instructions")
		})
	}
}

// A CLAUDE.md that is a symlink to the AGENTS.md listed under it is one file
// twice, and the stack says so rather than making the operator compare sizes.
func TestAgentContext_NamesTheFileASymlinkedInstructionPointsAt(t *testing.T) {
	host := newAgentTestHost(t)
	folder := filepath.Join(host.root, "project")
	writeFile(t, filepath.Join(folder, "AGENTS.md"), "# project\n")
	symlink(t, "AGENTS.md", filepath.Join(folder, "CLAUDE.md"))

	resolved := host.handler.resolve(folder, harnessClaudeCode, "", host.home)
	if got := resolved.Instructions[0].Link; got != "AGENTS.md" {
		t.Fatalf("link = %q, want %q", got, "AGENTS.md")
	}
	if got := resolved.Instructions[1].Link; got != "" {
		t.Fatalf("the target itself links to %q, want no link", got)
	}
}

// The one failure this surface exists to prevent is an instruction the operator
// cannot see. A file the server cannot open stays on the list, marked.
func TestAgentContext_ListsAnUnreadableInstructionRatherThanDroppingIt(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root opens every file, so there is no unreadable case to make")
	}
	host := newAgentTestHost(t)
	folder := filepath.Join(host.root, "project")
	path := writeFile(t, filepath.Join(folder, "CLAUDE.md"), "# secret\n")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	resolved := host.handler.resolve(folder, harnessClaudeCode, "", host.home)
	if len(resolved.Instructions) != 1 {
		t.Fatalf("instructions = %v, want the unreadable file", instructionPaths(resolved.Instructions))
	}
	if resolved.Instructions[0].Readable {
		t.Fatalf("readable = true for a file with mode 0000")
	}
}

// Skills are read from the directories the harness reads them from, and a
// symlink into a shared tree is named as shared with the path it really has.
func TestAgentContext_ResolvesSkillsWithTheirSource(t *testing.T) {
	tests := []struct {
		name       string
		harness    string
		build      func(t *testing.T, host *agentTestHost) string
		wantNames  []string
		wantSource map[string]string
	}{
		{
			name:    "project first, then the user's own, then the shared tree",
			harness: harnessClaudeCode,
			build: func(t *testing.T, host *agentTestHost) string {
				folder := filepath.Join(host.root, "project")
				shared := filepath.Join(host.home, "skills")
				writeFile(t, filepath.Join(folder, "skills", "deploy", "SKILL.md"),
					"---\nname: deploy\ndescription: Ship it.\n---\n")
				symlink(t, filepath.Join(folder, "skills", "deploy"), filepath.Join(folder, ".claude", "skills", "deploy"))
				writeFile(t, filepath.Join(host.home, ".claude", "skills", "own", "SKILL.md"),
					"---\nname: own\ndescription: Mine.\n---\n")
				writeFile(t, filepath.Join(shared, "beads", "SKILL.md"),
					"---\nname: beads\ndescription: Work state.\n---\n")
				symlink(t, filepath.Join(shared, "beads"), filepath.Join(host.home, ".claude", "skills", "beads"))
				return folder
			},
			wantNames:  []string{"deploy", "own", "beads"},
			wantSource: map[string]string{"deploy": sourceProject, "own": sourceUser, "beads": sourceShared},
		},
		{
			name:    "a directory with no SKILL.md is not a skill",
			harness: harnessClaudeCode,
			build: func(t *testing.T, host *agentTestHost) string {
				folder := filepath.Join(host.root, "project")
				mkdirAll(t, filepath.Join(host.home, ".claude", "skills", "empty"))
				writeFile(t, filepath.Join(host.home, ".claude", "skills", "real", "SKILL.md"),
					"---\nname: real\ndescription: Yes.\n---\n")
				return folder
			},
			wantNames:  []string{"real"},
			wantSource: map[string]string{"real": sourceUser},
		},
		{
			name:    "codex reads its own and the shared agents tree, once each",
			harness: harnessCodex,
			build: func(t *testing.T, host *agentTestHost) string {
				folder := filepath.Join(host.root, "project")
				shared := filepath.Join(host.home, "skills")
				writeFile(t, filepath.Join(shared, "beads", "SKILL.md"),
					"---\nname: beads\ndescription: Work state.\n---\n")
				symlink(t, filepath.Join(shared, "beads"), filepath.Join(host.home, ".codex", "skills", "beads"))
				symlink(t, shared, filepath.Join(host.home, ".agents", "skills"))
				return folder
			},
			wantNames:  []string{"beads"},
			wantSource: map[string]string{"beads": sourceShared},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := newAgentTestHost(t)
			folder := tt.build(t, host)
			resolved := host.handler.resolve(folder, tt.harness, "", host.home)
			assertSequence(t, skillNames(resolved.Skills), tt.wantNames, "skills")
			for _, skill := range resolved.Skills {
				if want := tt.wantSource[skill.Name]; skill.Source != want {
					t.Fatalf("%s source = %q, want %q", skill.Name, skill.Source, want)
				}
			}
		})
	}
}

func TestAgentContext_ReadsASkillsNameAndDescriptionFromItsFrontmatter(t *testing.T) {
	host := newAgentTestHost(t)
	folder := filepath.Join(host.root, "project")
	writeFile(t, filepath.Join(host.home, ".claude", "skills", "dir-name", "SKILL.md"),
		"---\nname: declared-name\ndescription: \"What it is for.\"\nmetadata:\n  ignored: true\n---\n\n# Heading\n")

	resolved := host.handler.resolve(folder, harnessClaudeCode, "", host.home)
	if len(resolved.Skills) != 1 {
		t.Fatalf("skills = %v, want one", skillNames(resolved.Skills))
	}
	if got := resolved.Skills[0].Name; got != "declared-name" {
		t.Fatalf("name = %q, want the frontmatter's", got)
	}
	if got := resolved.Skills[0].Description; got != "What it is for." {
		t.Fatalf("description = %q, want the frontmatter's without its quotes", got)
	}
}

// Claude Code writes a folder's memories into a directory named after the
// folder's own path. The index comes first and the rest newest first, because
// what an agent was last told is what the operator is looking for.
func TestAgentContext_ResolvesClaudeMemoriesForTheFoldersOwnSlug(t *testing.T) {
	host := newAgentTestHost(t)
	folder := filepath.Join(host.root, "project")
	mkdirAll(t, folder)
	memoryDir := filepath.Join(host.home, ".claude", "projects", claudeMemorySlug(folder), "memory")

	writeFile(t, filepath.Join(memoryDir, "MEMORY.md"), "- [older](older.md)\n")
	older := writeFile(t, filepath.Join(memoryDir, "older.md"), "---\nname: older-memory\n---\n\nold.\n")
	newer := writeFile(t, filepath.Join(memoryDir, "newer.md"), "# Newer memory\n\nnew.\n")
	writeFile(t, filepath.Join(memoryDir, "notes.txt"), "not a memory\n")
	// Memories of another folder never leak into this one.
	writeFile(t, filepath.Join(host.home, ".claude", "projects", "-somewhere-else", "memory", "other.md"), "# Other\n")

	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(older, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	newTime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(newer, newTime, newTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	resolved := host.handler.resolve(folder, harnessClaudeCode, "", host.home)
	assertSequence(t, memoryTitles(resolved.Memories),
		[]string{"MEMORY.md", "Newer memory", "older-memory"}, "memories")
	for _, memory := range resolved.Memories {
		if memory.Kind != memoryClaudeAuto {
			t.Fatalf("kind = %q, want %q", memory.Kind, memoryClaudeAuto)
		}
	}
}

// Codex 0.152 keeps one handbook and keys each block by the cwd it was written
// for, so resolving a folder means reading the blocks rather than a path.
func TestAgentContext_ResolvesCodexMemoriesByTheirAppliesToCwd(t *testing.T) {
	host := newAgentTestHost(t)
	folder := filepath.Join(host.root, "project")
	mkdirAll(t, folder)
	writeFile(t, filepath.Join(host.home, codexMemoryFile), ""+
		"# Task Group: "+host.root+" the root's own work\n"+
		"scope: broad\n"+
		"applies_to: cwd="+host.root+"; reuse_rule=durable\n"+
		"\n"+
		"# Task Group: "+folder+" the project's work\n"+
		"applies_to: cwd="+folder+"; reuse_rule=durable\n"+
		"\n"+
		"# Task Group: somewhere else entirely\n"+
		"applies_to: cwd=/elsewhere; reuse_rule=durable\n")

	resolved := host.handler.resolve(folder, harnessCodex, "", host.home)
	assertSequence(t, memoryTitles(resolved.Memories), []string{
		host.root + " the root's own work",
		folder + " the project's work",
	}, "memories")
	for _, memory := range resolved.Memories {
		if memory.Kind != memoryCodex {
			t.Fatalf("kind = %q, want %q", memory.Kind, memoryCodex)
		}
	}
}

func TestAgentContext_HasNoCodexMemoriesWhenTheHandbookIsAbsent(t *testing.T) {
	host := newAgentTestHost(t)
	folder := filepath.Join(host.root, "project")
	mkdirAll(t, folder)

	resolved := host.handler.resolve(folder, harnessCodex, "", host.home)
	if len(resolved.Memories) != 0 {
		t.Fatalf("memories = %v, want none", memoryTitles(resolved.Memories))
	}
}

func (host *agentTestHost) get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	host.handler.RegisterRoutes(mux)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func TestAgentContextRoute_AnswersAndRefuses(t *testing.T) {
	host := newAgentTestHost(t)
	folder := filepath.Join(host.root, "project")
	writeFile(t, filepath.Join(folder, "CLAUDE.md"), "# project\n")

	tests := []struct {
		name       string
		target     string
		wantStatus int
	}{
		{
			name:       "a folder in a root under a known harness",
			target:     "/api/agent/context?folder=" + folder + "&harness=claude-code",
			wantStatus: http.StatusOK,
		},
		{
			name:       "an unknown harness",
			target:     "/api/agent/context?folder=" + folder + "&harness=emacs",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "no folder",
			target:     "/api/agent/context?harness=claude-code",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "a relative folder",
			target:     "/api/agent/context?folder=project&harness=claude-code",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "a folder outside every root and the home",
			target:     "/api/agent/context?folder=/etc&harness=claude-code",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "a folder that is not there",
			target:     "/api/agent/context?folder=" + filepath.Join(host.root, "absent") + "&harness=claude-code",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := host.get(t, tt.target)
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
		})
	}
}

func TestAgentContextRoute_ReportsTheFolderHarnessAndUserItAnswered(t *testing.T) {
	host := newAgentTestHost(t)
	folder := filepath.Join(host.root, "project")
	writeFile(t, filepath.Join(folder, "CLAUDE.md"), "# project\n")

	recorder := host.get(t, "/api/agent/context?folder="+folder+"&harness=claude-code&user=operator")
	var response AgentContextResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode: %v: %s", err, recorder.Body.String())
	}
	if response.Folder != folder || response.Harness != harnessClaudeCode || response.User != "operator" {
		t.Fatalf("response = %+v, want the request's folder, harness and user", response)
	}
}

// The file route reads what the panel drew, and refuses anything the stack did
// not list — a path parameter is not a way into the filesystem.
func TestAgentFileRoute_ServesOnlyWhatTheStackLists(t *testing.T) {
	host := newAgentTestHost(t)
	folder := filepath.Join(host.root, "project")
	listed := writeFile(t, filepath.Join(folder, "CLAUDE.md"), "# project\n")
	unlisted := writeFile(t, filepath.Join(folder, "notes.md"), "# notes\n")
	query := "&folder=" + folder + "&harness=claude-code"

	t.Run("a listed file comes back whole", func(t *testing.T) {
		recorder := host.get(t, "/api/agent/file?path="+listed+query)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
		}
		var response AgentFileResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if response.Content != "# project\n" {
			t.Fatalf("content = %q, want the file's", response.Content)
		}
	})

	t.Run("a file the stack does not list is refused", func(t *testing.T) {
		recorder := host.get(t, "/api/agent/file?path="+unlisted+query)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403: %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("a path outside the folder is refused", func(t *testing.T) {
		recorder := host.get(t, "/api/agent/file?path=/etc/passwd"+query)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403: %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("a memory the stack lists comes back too", func(t *testing.T) {
		memory := writeFile(t,
			filepath.Join(host.home, ".claude", "projects", claudeMemorySlug(folder), "memory", "MEMORY.md"),
			"- [one](one.md)\n")
		recorder := host.get(t, "/api/agent/file?path="+memory+query)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
		}
	})
}

// The tab's left column: the home, the configured Beads projects, and the
// folders near the top of a root that carry instructions of their own.
func TestAgentWorkspacesRoute_ListsTheFoldersWorthAskingAbout(t *testing.T) {
	host := newAgentTestHost(t)
	project := filepath.Join(host.root, "project")
	nested := filepath.Join(host.root, "group", "nested")
	tooDeep := filepath.Join(host.root, "group", "deeper", "still", "here")
	writeFile(t, filepath.Join(host.home, "CLAUDE.md"), "# home\n")
	writeFile(t, filepath.Join(host.home, "AGENTS.md"), "# home\n")
	writeFile(t, filepath.Join(project, "CLAUDE.md"), "# project\n")
	writeFile(t, filepath.Join(project, ".claude", "settings.json"), "{}\n")
	writeFile(t, filepath.Join(nested, "AGENTS.md"), "# nested\n")
	writeFile(t, filepath.Join(tooDeep, "AGENTS.md"), "# too deep\n")
	writeFile(t, filepath.Join(host.root, "plain", "readme.md"), "no instructions\n")

	host.handler.tender = AgentTender{Session: "tender", Beads: "/beads", Folder: "/tender"}

	recorder := host.get(t, "/api/agent/workspaces")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response AgentWorkspacesResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode: %v", err)
	}

	found := map[string]AgentWorkspace{}
	for _, workspace := range response.Workspaces {
		found[workspace.Path] = workspace
	}
	if _, ok := found[host.home]; !ok {
		t.Fatalf("workspaces = %+v, want the home", response.Workspaces)
	}
	if got := found[project].Instructions; got != 2 {
		t.Fatalf("%s instruction count = %d, want 2", project, got)
	}
	if _, ok := found[nested]; !ok {
		t.Fatalf("workspaces = %+v, want the folder two levels down", response.Workspaces)
	}
	if _, ok := found[tooDeep]; ok {
		t.Fatalf("workspaces = %+v, want nothing past two levels", response.Workspaces)
	}
	if _, ok := found[filepath.Join(host.root, "plain")]; ok {
		t.Fatalf("workspaces = %+v, want no folder without instructions", response.Workspaces)
	}
	if response.Tender.Session != "tender" || response.Tender.Folder != "/tender" {
		t.Fatalf("tender = %+v, want what the host configured", response.Tender)
	}
}
