package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	chroteapi "github.com/chrote/server/internal/api"
	"github.com/chrote/server/internal/formations"
)

type fakeTmux struct {
	live    map[string]bool
	spawned []string
	attach  []string
}

func (f *fakeTmux) LiveSessions() ([]formations.LiveAgentSession, error) {
	live := make([]formations.LiveAgentSession, 0, len(f.live))
	for name := range f.live {
		live = append(live, formations.LiveAgentSession{Name: name, Status: "live"})
	}
	return live, nil
}

func (f *fakeTmux) Spawn(name, command string) error {
	f.spawned = append(f.spawned, name+":"+command)
	f.live[name] = true
	return nil
}

func (f *fakeTmux) Attach(name string) error {
	f.attach = append(f.attach, name)
	return nil
}

func withoutArchonTmuxPrefix(t *testing.T) {
	t.Helper()
	t.Setenv("CHROTE_FORMATIONS_TMUX_SESSION_PREFIX", "")
}

func TestArchonTmuxCommandUsesConfiguredFormationSocketAndDropsAmbientTmux(t *testing.T) {
	t.Setenv("CHROTE_FORMATIONS_TMUX_SOCKET", "/tmp/chrote-formations-test/default")
	t.Setenv("TMUX", "/tmp/ambient,123,0")

	args := archonTmuxArgs("new-session", "-d", "-s", "dogfood-scout")
	wantArgs := []string{"-S", "/tmp/chrote-formations-test/default", "new-session", "-d", "-s", "dogfood-scout"}
	if strings.Join(args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("tmux args = %#v, want %#v", args, wantArgs)
	}
	for _, item := range archonTmuxEnv() {
		if strings.HasPrefix(item, "TMUX=") {
			t.Fatalf("archon tmux env leaked ambient TMUX: %q", item)
		}
	}
}

func TestRealTmuxRunnerSpawnUsesConfiguredSocketAndDropsAmbientTmux(t *testing.T) {
	binDir := t.TempDir()
	capturePath := filepath.Join(t.TempDir(), "tmux-capture")
	fakeTmux := filepath.Join(binDir, "tmux")
	if err := os.WriteFile(fakeTmux, []byte("#!/usr/bin/env bash\nprintf 'args:%s\\n' \"$*\" >> \"$ARCHON_TMUX_CAPTURE\"\nif env | grep '^TMUX=' >> \"$ARCHON_TMUX_CAPTURE\"; then :; else printf 'TMUX:<unset>\\n' >> \"$ARCHON_TMUX_CAPTURE\"; fi\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ARCHON_TMUX_CAPTURE", capturePath)
	t.Setenv("CHROTE_FORMATIONS_TMUX_SOCKET", "/tmp/chrote-formations-test/default")
	t.Setenv("TMUX", "/tmp/ambient,123,0")

	if err := (realTmuxRunner{}).Spawn("dogfood-scout", "printf hi"); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	captured, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("read fake tmux capture: %v", err)
	}
	text := string(captured)
	if !strings.Contains(text, "args:-S /tmp/chrote-formations-test/default new-session -d -s dogfood-scout printf hi") {
		t.Fatalf("capture missing explicit socket args:\n%s", text)
	}
	if strings.Contains(text, "TMUX=/tmp/ambient") {
		t.Fatalf("capture leaked ambient TMUX:\n%s", text)
	}
	if !strings.Contains(text, "TMUX:<unset>") {
		t.Fatalf("capture did not prove TMUX was removed:\n%s", text)
	}
}

func TestArchonAgentNewListInspectAndEditUsePersonaStore(t *testing.T) {
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_AGENTS_DIR", agentsDir)
	runner := &fakeTmux{live: map[string]bool{}}

	stdout, stderr, code := runArchon(t, runner, "agent", "new", "scout", "--kind", "specialist", "--harness", "claude-code", "--capable", "research,go", "--personality", "direct", "--json")
	if code != 0 {
		t.Fatalf("agent new code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, `"id": "scout"`) || !strings.Contains(stdout, `"personality:direct"`) {
		t.Fatalf("new JSON missing card fields: %s", stdout)
	}

	stdout, stderr, code = runArchon(t, runner, "agent", "edit", "scout", "--add-capability", "react", "--note", "ready", "--json")
	if code != 0 {
		t.Fatalf("agent edit code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	raw := readArchonFile(t, filepath.Join(agentsDir, "scout.toml"))
	if !strings.Contains(raw, `"react"`) || !strings.Contains(raw, `text = "ready"`) {
		t.Fatalf("edit did not persist through shared writer:\n%s", raw)
	}

	stdout, stderr, code = runArchon(t, runner, "agent", "list", "--capable", "react", "--json")
	if code != 0 {
		t.Fatalf("agent list code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, `"id": "scout"`) || strings.Contains(stdout, "toml") || strings.Contains(stdout, "harnessVariants") {
		t.Fatalf("list output wrong: %s", stdout)
	}

	stdout, stderr, code = runArchon(t, runner, "agent", "inspect", "scout", "--json")
	if code != 0 {
		t.Fatalf("agent inspect code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, `"harnessDefault": "claude-code"`) || strings.Contains(stdout, "toml") {
		t.Fatalf("inspect output wrong: %s", stdout)
	}
}

func TestArchonAgentNewDuplicateFailsWithoutChangingCard(t *testing.T) {
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_AGENTS_DIR", agentsDir)
	runner := &fakeTmux{live: map[string]bool{}}

	if _, stderr, code := runArchon(t, runner, "agent", "new", "scout", "--kind", "specialist", "--harness", "claude-code"); code != 0 {
		t.Fatalf("first create failed: %d %s", code, stderr)
	}
	before := readArchonFile(t, filepath.Join(agentsDir, "scout.toml"))

	_, stderr, code := runArchon(t, runner, "agent", "new", "scout", "--kind", "specialist", "--harness", "openai-codex")
	if code == 0 || !strings.Contains(stderr, "already exists") {
		t.Fatalf("duplicate create code=%d stderr=%s", code, stderr)
	}
	after := readArchonFile(t, filepath.Join(agentsDir, "scout.toml"))
	if after != before {
		t.Fatalf("duplicate create changed card:\n%s", after)
	}
}

func TestArchonAgentListShowsUnboundLiveSessionsButExcludesAssignable(t *testing.T) {
	withoutArchonTmuxPrefix(t)
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_AGENTS_DIR", agentsDir)
	runner := &fakeTmux{live: map[string]bool{"scratch": true}}

	if _, stderr, code := runArchon(t, runner, "agent", "new", "scout", "--kind", "specialist", "--harness", "claude-code"); code != 0 {
		t.Fatalf("create failed: %d %s", code, stderr)
	}
	stdout, stderr, code := runArchon(t, runner, "agent", "list", "--json")
	if code != 0 {
		t.Fatalf("list failed: %d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, `"id": "scratch"`) || !strings.Contains(stdout, `"unbound": true`) {
		t.Fatalf("list output missing unbound scratch: %s", stdout)
	}

	stdout, stderr, code = runArchon(t, runner, "agent", "list", "--assignable", "--json")
	if code != 0 {
		t.Fatalf("assignable list failed: %d stderr=%s", code, stderr)
	}
	if strings.Contains(stdout, `"id": "scratch"`) {
		t.Fatalf("assignable list included unbound scratch: %s", stdout)
	}
}

func TestArchonAgentNewFromHermesProfilePopulatesLaunchReference(t *testing.T) {
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_AGENTS_DIR", agentsDir)
	source := filepath.Join(t.TempDir(), ".hermes", "profiles", "archon")
	runner := &fakeTmux{live: map[string]bool{}}

	if _, stderr, code := runArchon(t, runner, "agent", "new", "archon", "--kind", "archon", "--from", source); code != 0 {
		t.Fatalf("create from hermes profile failed: %d %s", code, stderr)
	}
	raw := readArchonFile(t, filepath.Join(agentsDir, "archon.toml"))
	if !strings.Contains(raw, `default = "hermes"`) || !strings.Contains(raw, `source = "`+source+`"`) || !strings.Contains(raw, `launch = "hermes --profile`) {
		t.Fatalf("hermes card missing inferred harness/source/launch:\n%s", raw)
	}
}

func TestArchonAgentNewOpenAICodexDefaultsLaunchAndSpawnUsesIt(t *testing.T) {
	withoutArchonTmuxPrefix(t)
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_AGENTS_DIR", agentsDir)
	runner := &fakeTmux{live: map[string]bool{}}
	wantLaunch := "codex --yolo -c check_for_update_on_startup=false"

	stdout, stderr, code := runArchon(t, runner, "agent", "new", "codexer", "--kind", "specialist", "--harness", "openai-codex", "--json")
	if code != 0 {
		t.Fatalf("create openai-codex failed: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	var card formations.PersonaCard
	if err := json.Unmarshal([]byte(stdout), &card); err != nil {
		t.Fatalf("decode created card JSON: %v\n%s", err, stdout)
	}
	variant := card.DefaultVariant()
	if variant.ID != "openai-codex" || variant.Launch != wantLaunch {
		t.Fatalf("openai-codex variant = %#v, want launch %q", variant, wantLaunch)
	}
	raw := readArchonFile(t, filepath.Join(agentsDir, "codexer.toml"))
	if !strings.Contains(raw, `launch = "`+wantLaunch+`"`) {
		t.Fatalf("created TOML missing codex launch %q:\n%s", wantLaunch, raw)
	}

	stdout, stderr, code = runArchon(t, runner, "agent", "spawn", "codexer")
	if code != 0 {
		t.Fatalf("spawn openai-codex failed: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if len(runner.spawned) != 1 || runner.spawned[0] != "codexer:"+wantLaunch {
		t.Fatalf("spawned=%#v, want codexer:%s", runner.spawned, wantLaunch)
	}
}

func TestArchonAgentNewClaudeCodeDefaultsLaunchAndSpawnUsesIt(t *testing.T) {
	withoutArchonTmuxPrefix(t)
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_AGENTS_DIR", agentsDir)
	runner := &fakeTmux{live: map[string]bool{}}
	wantLaunch := "HOME=/home/perttu claude --dangerously-skip-permissions --effort=\"max\""

	stdout, stderr, code := runArchon(t, runner, "agent", "new", "clauder", "--kind", "specialist", "--harness", "claude-code", "--json")
	if code != 0 {
		t.Fatalf("create claude-code failed: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	var card formations.PersonaCard
	if err := json.Unmarshal([]byte(stdout), &card); err != nil {
		t.Fatalf("decode created card JSON: %v\n%s", err, stdout)
	}
	variant := card.DefaultVariant()
	if variant.ID != "claude-code" || variant.Launch != wantLaunch {
		t.Fatalf("claude-code variant = %#v, want launch %q", variant, wantLaunch)
	}

	stdout, stderr, code = runArchon(t, runner, "agent", "spawn", "clauder")
	if code != 0 {
		t.Fatalf("spawn claude-code failed: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if len(runner.spawned) != 1 || runner.spawned[0] != "clauder:"+wantLaunch {
		t.Fatalf("spawned=%#v, want clauder:%s", runner.spawned, wantLaunch)
	}
}

func TestArchonAgentSpawnUsesFakeTmuxWithoutDuplicateSession(t *testing.T) {
	withoutArchonTmuxPrefix(t)
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_AGENTS_DIR", agentsDir)
	runner := &fakeTmux{live: map[string]bool{}}

	if _, stderr, code := runArchon(t, runner, "agent", "new", "scout", "--kind", "specialist", "--harness", "claude-code"); code != 0 {
		t.Fatalf("create failed: %d %s", code, stderr)
	}
	stdout, stderr, code := runArchon(t, runner, "agent", "spawn", "scout")
	if code != 0 {
		t.Fatalf("spawn failed: %d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "spawned scout as scout") || len(runner.spawned) != 1 {
		t.Fatalf("spawn output=%s spawned=%#v", stdout, runner.spawned)
	}
	stdout, stderr, code = runArchon(t, runner, "agent", "spawn", "scout")
	if code != 0 {
		t.Fatalf("second spawn failed: %d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "already live") || len(runner.spawned) != 1 {
		t.Fatalf("second spawn output=%s spawned=%#v", stdout, runner.spawned)
	}
}

func TestArchonAgentSpawnListAttachUseTmuxSessionPrefix(t *testing.T) {
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_AGENTS_DIR", agentsDir)
	t.Setenv("CHROTE_FORMATIONS_TMUX_SESSION_PREFIX", "dogfood-")
	runner := &fakeTmux{live: map[string]bool{}}

	if _, stderr, code := runArchon(t, runner, "agent", "new", "scout", "--kind", "specialist", "--harness", "claude-code"); code != 0 {
		t.Fatalf("create failed: %d %s", code, stderr)
	}
	stdout, stderr, code := runArchon(t, runner, "agent", "spawn", "scout")
	if code != 0 {
		t.Fatalf("spawn failed: %d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "spawned scout as dogfood-scout") || len(runner.spawned) != 1 || !strings.HasPrefix(runner.spawned[0], "dogfood-scout:") {
		t.Fatalf("spawn output=%s spawned=%#v, want prefixed tmux target", stdout, runner.spawned)
	}

	stdout, stderr, code = runArchon(t, runner, "agent", "list", "--json")
	if code != 0 {
		t.Fatalf("list failed: %d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, `"id": "scout"`) || !strings.Contains(stdout, `"liveness": "live"`) || !strings.Contains(stdout, `"sessionId": "dogfood-scout"`) || strings.Contains(stdout, `"id": "dogfood-scout"`) {
		t.Fatalf("list output=%s, want scout live on prefixed tmux target without unbound prefixed alias", stdout)
	}

	stdout, stderr, code = runArchon(t, runner, "agent", "attach", "scout")
	if code != 0 {
		t.Fatalf("attach failed: %d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if len(runner.attach) != 1 || runner.attach[0] != "dogfood-scout" {
		t.Fatalf("attach calls = %#v, want prefixed tmux target", runner.attach)
	}

	stdout, stderr, code = runArchon(t, runner, "agent", "spawn", "scout")
	if code != 0 {
		t.Fatalf("second spawn failed: %d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "already live as dogfood-scout") || len(runner.spawned) != 1 {
		t.Fatalf("second spawn output=%s spawned=%#v, want prefixed liveness to prevent duplicate", stdout, runner.spawned)
	}
}

func TestArchonAgentSpawnAndAttachUseExplicitHarnessStem(t *testing.T) {
	withoutArchonTmuxPrefix(t)
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_AGENTS_DIR", agentsDir)
	runner := &fakeTmux{live: map[string]bool{}}

	if _, stderr, code := runArchon(t, runner, "agent", "new", "susie", "--kind", "specialist", "--harness", "claude-code"); code != 0 {
		t.Fatalf("create susie failed: %d %s", code, stderr)
	}
	if _, stderr, code := runArchon(t, runner, "agent", "edit", "susie", "--add-harness", "openai-codex", "--session-stem", "codex-susie"); code != 0 {
		t.Fatalf("add harness failed: %d %s", code, stderr)
	}

	_, stderr, code := runArchon(t, runner, "agent", "spawn", "susie")
	if code == 0 || !strings.Contains(stderr, "ambiguous") {
		t.Fatalf("spawn without harness code=%d stderr=%s, want ambiguity", code, stderr)
	}

	stdout, stderr, code := runArchon(t, runner, "agent", "spawn", "susie", "--harness", "openai-codex")
	if code != 0 {
		t.Fatalf("spawn with harness failed: %d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "spawned susie as codex-susie") || len(runner.spawned) != 1 {
		t.Fatalf("spawn output=%s spawned=%#v", stdout, runner.spawned)
	}

	if _, stderr, code := runArchon(t, runner, "agent", "attach", "susie", "--harness", "openai-codex"); code != 0 {
		t.Fatalf("attach with harness failed: %d stderr=%s", code, stderr)
	}
	if len(runner.attach) != 1 || runner.attach[0] != "codex-susie" {
		t.Fatalf("attach calls = %#v", runner.attach)
	}
}

func TestArchonBoardValidateReportsStructuredFindings(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[mission]]
id = "mis_showcase"
title = "Showcase"
goal = "Ship it"
beadId = "home-1.1"

[[formation]]
id = "fmn_work"
type = "solo"
title = "Work"

[[formation.input]]
id = "port_work_in"
label = "Input"

[[formation.output]]
id = "port_work_out"
label = "Output"

[[gate]]
id = "gate_orphan"
title = "Orphan gate"
kinds = ["code"]
criterion = "Decide somehow"

[[connection]]
id = "edge_broken"
from = "fmn_work:missing_output"
to = "gate_orphan:in"
`)
	runner := &fakeTmux{live: map[string]bool{}}

	stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "board", "validate", "session-search", "--json")
	if code != 1 {
		t.Fatalf("board validate code=%d stderr=%s stdout=%s, want validation failure code 1", code, stderr, stdout)
	}
	if !strings.Contains(stdout, `"code": "dangling_connection"`) || !strings.Contains(stdout, `"code": "gate_not_routable"`) {
		t.Fatalf("validate JSON missing expected finding codes: %s", stdout)
	}
	if !strings.Contains(stdout, `"warnings"`) {
		t.Fatalf("validate JSON missing warnings envelope: %s", stdout)
	}

	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "board", "validate", "session-search")
	if code != 1 {
		t.Fatalf("text board validate code=%d stderr=%s stdout=%s, want validation failure code 1", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "dangling_connection") || !strings.Contains(stdout, "gate_not_routable") {
		t.Fatalf("validate text missing expected finding codes: %s", stdout)
	}
}

func TestArchonFormationCreateListInspectUseSharedStore(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
customFuture = "keep me"
`)
	runner := &fakeTmux{live: map[string]bool{}}

	stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "formation", "create", "session-search", "peer", "--title", "Research huddle", "--json")
	if code != 0 {
		t.Fatalf("formation create code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, `"type": "peer"`) || !strings.Contains(stdout, `"title": "Research huddle"`) {
		t.Fatalf("create JSON missing formation fields: %s", stdout)
	}
	boardRaw := readArchonFile(t, store.BoardPath("session-search"))
	if !strings.Contains(boardRaw, `customFuture = "keep me"`) || !strings.Contains(boardRaw, `[[formation]]`) || strings.Contains(boardRaw, "x = ") {
		t.Fatalf("formation create did not preserve board structure/layout split:\n%s", boardRaw)
	}

	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "formation", "list", "--json")
	if code != 0 {
		t.Fatalf("formation list code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, `"slug": "session-search"`) || !strings.Contains(stdout, `"rev": 8`) {
		t.Fatalf("list JSON missing board summary: %s", stdout)
	}

	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "formation", "inspect", "brd_01J9_sesssearch", "--json")
	if code != 0 {
		t.Fatalf("formation inspect by id code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, `"slug": "session-search"`) || !strings.Contains(stdout, `"title": "Research huddle"`) {
		t.Fatalf("inspect JSON missing structural formation: %s", stdout)
	}
	if strings.Contains(stdout, `"x":`) || strings.Contains(stdout, `"y":`) {
		t.Fatalf("inspect JSON leaked layout coordinates: %s", stdout)
	}
}

func TestArchonCreateWithoutCoordinatesPlacesNodesWithoutOverlap(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7

[[gate]]
id = "gate_existing"
title = "Existing"
kinds = ["human"]
`)
	writeArchonFile(t, store.LayoutPath("session-search"), `schema = 1
boardId = "brd_01J9_sesssearch"
boardRev = 7

[[node]]
id = "gate_existing"
x = 112
y = 112
`)
	runner := &fakeTmux{live: map[string]bool{}}

	commands := [][]string{
		{"formation", "create", "session-search", "solo", "--title", "Draft"},
		{"gate", "create", "session-search", "--title", "Review"},
		{"mission", "create", "session-search", "--title", "Brief", "--goal", "Write it", "--bead", "ctx-placement"},
	}
	for _, command := range commands {
		if stdout, stderr, code := runArchon(t, runner, append([]string{"--workspace", workspace}, command...)...); code != 0 {
			t.Fatalf("%v code=%d stderr=%s stdout=%s", command, code, stderr, stdout)
		}
	}

	layout, err := store.ReadLayout("session-search")
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}
	if len(layout.Nodes) != 4 {
		t.Fatalf("layout nodes = %+v, want existing plus three created nodes", layout.Nodes)
	}
	abs := func(value int) int {
		if value < 0 {
			return -value
		}
		return value
	}
	for index, node := range layout.Nodes {
		if node.X%28 != 0 || node.Y%28 != 0 {
			t.Fatalf("node is not grid aligned: %+v", node)
		}
		for prior := 0; prior < index; prior++ {
			other := layout.Nodes[prior]
			if abs(node.X-other.X) < 308 && abs(node.Y-other.Y) < 280 {
				t.Fatalf("created nodes overlap: %+v and %+v", other, node)
			}
		}
	}
}

func TestArchonGateCreateAndUpdatePersistStructuredScriptCommand(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
`)
	runner := &fakeTmux{live: map[string]bool{}}

	stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "gate", "create", "session-search", "--kinds", "lint", "--criterion", "Lint passes", "--command-argv", "npm,run,lint", "--command-cwd", "dashboard", "--json")
	if code != 0 {
		t.Fatalf("gate create code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	var created formations.BoardDocument
	if err := json.Unmarshal([]byte(stdout), &created); err != nil {
		t.Fatalf("decode gate create JSON: %v\n%s", err, stdout)
	}
	if len(created.Gates) != 1 || !strings.Contains(stdout, `"commandArgv": [`) || !strings.Contains(stdout, `"commandCwd": "dashboard"`) {
		t.Fatalf("gate create JSON missing structured command: %s", stdout)
	}
	raw := readArchonFile(t, store.BoardPath("session-search"))
	if !strings.Contains(raw, `commandArgv = ["npm", "run", "lint"]`) || !strings.Contains(raw, `commandCwd = "dashboard"`) || strings.Contains(raw, `command = "npm run lint"`) {
		t.Fatalf("gate create TOML missing structured argv/cwd or persisted legacy command:\n%s", raw)
	}

	gateID := created.Gates[0].ID
	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "gate", "update", "session-search", gateID, "--command-shell", "printf ok", "--command-cwd", "dashboard", "--json")
	if code != 0 {
		t.Fatalf("gate update code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	raw = readArchonFile(t, store.BoardPath("session-search"))
	if !strings.Contains(raw, `commandShell = "printf ok"`) || strings.Contains(raw, `commandArgv =`) || strings.Contains(raw, `command =`) {
		t.Fatalf("gate update TOML did not switch cleanly to explicit shell command:\n%s", raw)
	}

	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "gate", "update", "session-search", gateID, "--command", "legacy only", "--json")
	if code != 0 {
		t.Fatalf("legacy gate update code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	raw = readArchonFile(t, store.BoardPath("session-search"))
	if !strings.Contains(raw, `command = "legacy only"`) || strings.Contains(raw, `commandShell =`) || strings.Contains(raw, `commandArgv =`) || strings.Contains(raw, `commandCwd =`) {
		t.Fatalf("legacy command update did not clear executable fields:\n%s", raw)
	}

	_, stderr, code = runArchon(t, runner, "--workspace", workspace, "gate", "create", "session-search", "--command-argv", "npm,run,lint", "--command-shell", "printf ok")
	if code == 0 || !strings.Contains(stderr, "gate command must use only one") {
		t.Fatalf("mixed command mode create code=%d stderr=%s", code, stderr)
	}
}

func TestArchonBoardListAndInspectExposeDurableJSON(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("poems"), `schema = 1
id = "brd_poems"
slug = "poems"
title = "Poems"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
viewport = "browser-only"
selectedNode = "fmn_draft"
popup = "gate-config"
terminalFocus = "pane-1"
undoStack = "browser-only"

[[mission]]
id = "mis_poem"
title = "Simple poem"
goal = "Create a simple poem"
beadId = "home-vdki.33.1"

[[formation]]
id = "fmn_draft"
type = "solo"
title = "Draft poem"

[[formation.input]]
id = "port_draft_in"
label = "Input"

[[formation.output]]
id = "port_draft_out"
label = "Output"

[[formation.slot]]
id = "slot_writer"
label = "Writer"
agentId = "lab-poet"
harness = "lab-fake"
controller = true

[[gate]]
id = "gate_review"
title = "Human review"
kinds = ["human"]
criterion = "Draft is ready"

[[connection]]
id = "edge_draft_review"
from = "fmn_draft:port_draft_out"
to = "gate_review:in"
`)
	runner := &fakeTmux{live: map[string]bool{}}

	stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "board", "list", "--json")
	if code != 0 {
		t.Fatalf("board list code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	var list struct {
		Boards []formations.BoardSummary `json:"boards"`
	}
	if err := json.Unmarshal([]byte(stdout), &list); err != nil {
		t.Fatalf("decode board list: %v\n%s", err, stdout)
	}
	if len(list.Boards) != 1 || list.Boards[0].ID != "brd_poems" || list.Boards[0].Slug != "poems" || list.Boards[0].Rev != 7 || list.Boards[0].ETag == "" {
		t.Fatalf("board list = %+v, want stable board identity with revision", list.Boards)
	}

	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "board", "inspect", "poems", "--json")
	if code != 0 {
		t.Fatalf("board inspect code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	var board formations.BoardDocument
	if err := json.Unmarshal([]byte(stdout), &board); err != nil {
		t.Fatalf("decode board inspect: %v\n%s", err, stdout)
	}
	if board.ID != "brd_poems" || board.Slug != "poems" || board.Rev != 7 || board.ETag == "" {
		t.Fatalf("board inspect identity = %+v, want durable id/slug/rev/etag", board)
	}
	if len(board.Missions) != 1 || board.Missions[0].ID != "mis_poem" ||
		len(board.Formations) != 1 || board.Formations[0].ID != "fmn_draft" ||
		len(board.Formations[0].Slots) != 1 || board.Formations[0].Slots[0].ID != "slot_writer" ||
		len(board.Formations[0].Inputs) != 1 || board.Formations[0].Inputs[0].ID != "port_draft_in" ||
		len(board.Formations[0].Outputs) != 1 || board.Formations[0].Outputs[0].ID != "port_draft_out" ||
		len(board.Gates) != 1 || board.Gates[0].ID != "gate_review" ||
		len(board.Connections) != 1 || board.Connections[0].ID != "edge_draft_review" {
		t.Fatalf("board inspect missing stable graph ids: %+v", board)
	}
	for _, browserOnly := range []string{"viewport", "selectedNode", "popup", "terminalFocus", "undoStack", "toml"} {
		if strings.Contains(stdout, browserOnly) {
			t.Fatalf("board inspect leaked browser-only/raw field %q: %s", browserOnly, stdout)
		}
	}
}

func TestArchonBoardNewCreatesDurableBoardJSONAndText(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	runner := &fakeTmux{live: map[string]bool{}}

	stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "board", "new", "poems", "--title", "Poems", "--json")
	if code != 0 {
		t.Fatalf("board new --json code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	var board formations.BoardDocument
	if err := json.Unmarshal([]byte(stdout), &board); err != nil {
		t.Fatalf("decode board new JSON: %v\n%s", err, stdout)
	}
	if board.Slug != "poems" || board.Title != "Poems" || board.Rev != 1 || !strings.HasPrefix(board.ID, "brd_") || board.ETag == "" {
		t.Fatalf("board new JSON = %+v, want durable board identity", board)
	}
	if board.TOML != "" || strings.Contains(stdout, "toml") {
		t.Fatalf("board new JSON leaked raw TOML: %s", stdout)
	}
	raw := readArchonFile(t, store.BoardPath("poems"))
	for _, want := range []string{`schema = 1`, `slug = "poems"`, `title = "Poems"`, `rev = 1`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("created board file missing %q:\n%s", want, raw)
		}
	}

	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "board", "new", "drafts", "--title", "Drafts")
	if code != 0 {
		t.Fatalf("board new text code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "created drafts") {
		t.Fatalf("board new text stdout=%q, want created slug", stdout)
	}
	drafts, err := store.ReadBoard("drafts")
	if err != nil {
		t.Fatalf("read text-created board: %v", err)
	}
	if drafts.Title != "Drafts" || drafts.Rev != 1 || !strings.HasPrefix(drafts.ID, "brd_") {
		t.Fatalf("text-created board = %+v, want persisted board", drafts)
	}
}

func TestArchonBoardArrangeUsesSharedLayoutOperation(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("arrange"), `schema = 1
id = "brd_arrange"
slug = "arrange"
title = "Arrange"
rev = 2

[[mission]]
id = "mis_start"
title = "Start"

[[formation]]
id = "fmn_finish"
type = "solo"
title = "Finish"

[[connection]]
id = "edge_start_finish"
from = "mis_start:out"
to = "fmn_finish:in"
`)
	writeArchonFile(t, store.LayoutPath("arrange"), `schema = 1
boardId = "brd_arrange"
boardRev = 2

[[node]]
id = "mis_start"
x = 500
y = 500

[[node]]
id = "fmn_finish"
x = 100
y = 100
`)
	stdout, stderr, code := runArchon(t, &fakeTmux{live: map[string]bool{}}, "--workspace", workspace, "board", "arrange", "arrange", "--json")
	if code != 0 {
		t.Fatalf("board arrange code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	var arranged formations.LayoutDocument
	if err := json.Unmarshal([]byte(stdout), &arranged); err != nil {
		t.Fatalf("decode arranged layout: %v\n%s", err, stdout)
	}
	byID := map[string]formations.LayoutNode{}
	for _, node := range arranged.Nodes {
		byID[node.ID] = node
	}
	if byID["mis_start"].X >= byID["fmn_finish"].X {
		t.Fatalf("archon arrange did not order connected nodes: %+v", byID)
	}
}

func TestArchonBoardNewDuplicateFailsWithoutChangingBoard(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	runner := &fakeTmux{live: map[string]bool{}}

	if _, stderr, code := runArchon(t, runner, "--workspace", workspace, "board", "new", "poems", "--title", "Poems"); code != 0 {
		t.Fatalf("first board create failed: %d %s", code, stderr)
	}
	before := readArchonFile(t, store.BoardPath("poems"))

	_, stderr, code := runArchon(t, runner, "--workspace", workspace, "board", "new", "poems", "--title", "Different")
	if code == 0 || !strings.Contains(stderr, "already exists") {
		t.Fatalf("duplicate board create code=%d stderr=%s", code, stderr)
	}
	after := readArchonFile(t, store.BoardPath("poems"))
	if after != before {
		t.Fatalf("duplicate board create changed board:\n%s", after)
	}
}

func TestArchonBoardNewRequiresSlugAndTitle(t *testing.T) {
	workspace := t.TempDir()
	runner := &fakeTmux{live: map[string]bool{}}

	_, stderr, code := runArchon(t, runner, "--workspace", workspace, "board", "new", "--title", "Missing slug")
	if code == 0 || !strings.Contains(stderr, "usage: archon board new") {
		t.Fatalf("missing slug code=%d stderr=%s", code, stderr)
	}

	_, stderr, code = runArchon(t, runner, "--workspace", workspace, "board", "new", "poems")
	if code == 0 || !strings.Contains(stderr, "--title") {
		t.Fatalf("missing title code=%d stderr=%s", code, stderr)
	}
}

func TestArchonBoardInspectFailsLoudOnAmbiguousSelector(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("poems"), `schema = 1
id = "brd_duplicate"
slug = "poems"
title = "Poems"
rev = 1
`)
	writeArchonFile(t, store.BoardPath("drafts"), `schema = 1
id = "brd_duplicate"
slug = "drafts"
title = "Drafts"
rev = 2
`)

	_, stderr, code := runArchon(t, &fakeTmux{live: map[string]bool{}}, "--workspace", workspace, "board", "inspect", "brd_duplicate", "--json")
	if code == 0 || !strings.Contains(stderr, "ambiguous") || !strings.Contains(stderr, "brd_duplicate") {
		t.Fatalf("ambiguous board inspect code=%d stderr=%s, want loud ambiguous selector", code, stderr)
	}
}

func TestArchonMissionListAndInspectExposeReachableChain(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("poems"), `schema = 1
id = "brd_poems"
slug = "poems"
title = "Poems"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
viewport = "browser-only"
undoStack = "browser-only"

[[mission]]
id = "mis_poem"
title = "Simple poem"
goal = "Create a simple poem"
beadId = "home-vdki.33.1"

[[formation]]
id = "fmn_draft"
type = "solo"
title = "Draft poem"

[[formation.input]]
id = "port_draft_in"
label = "Input"

[[formation.output]]
id = "port_draft_out"
label = "Output"

[[gate]]
id = "gate_review"
title = "Human review"
kinds = ["human"]
criterion = "Draft is ready"

[[formation]]
id = "fmn_polish"
type = "solo"
title = "Polish poem"

[[formation.input]]
id = "port_polish_in"
label = "Input"

[[formation.output]]
id = "port_polish_out"
label = "Output"

[[connection]]
id = "edge_mission_draft"
from = "mis_poem:out"
to = "fmn_draft:port_draft_in"

[[connection]]
id = "edge_draft_gate"
from = "fmn_draft:port_draft_out"
to = "gate_review:in"

[[connection]]
id = "edge_gate_polish"
from = "gate_review:pass"
to = "fmn_polish:port_polish_in"
`)
	runner := &fakeTmux{live: map[string]bool{}}

	stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "mission", "list", "poems", "--json")
	if code != 0 {
		t.Fatalf("mission list code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	var list struct {
		Board    archonBoardIdentity      `json:"board"`
		Missions []formations.MissionNode `json:"missions"`
	}
	if err := json.Unmarshal([]byte(stdout), &list); err != nil {
		t.Fatalf("decode mission list: %v\n%s", err, stdout)
	}
	if list.Board.ID != "brd_poems" || list.Board.Slug != "poems" || list.Board.Rev != 7 || list.Board.ETag == "" {
		t.Fatalf("mission list board identity = %+v, want stable board identity", list.Board)
	}
	if len(list.Missions) != 1 || list.Missions[0].ID != "mis_poem" || list.Missions[0].Title != "Simple poem" ||
		list.Missions[0].Goal != "Create a simple poem" || list.Missions[0].BeadID != "home-vdki.33.1" {
		t.Fatalf("mission list missions = %+v, want stable mission fields", list.Missions)
	}

	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "mission", "inspect", "poems", "simple-poem", "--json")
	if code != 0 {
		t.Fatalf("mission inspect code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	var inspect struct {
		Board       archonBoardIdentity          `json:"board"`
		Mission     formations.MissionNode       `json:"mission"`
		Chain       []archonMissionChainNode     `json:"chain"`
		Connections []formations.BoardConnection `json:"connections"`
	}
	if err := json.Unmarshal([]byte(stdout), &inspect); err != nil {
		t.Fatalf("decode mission inspect: %v\n%s", err, stdout)
	}
	if inspect.Mission.ID != "mis_poem" || inspect.Board.Rev != 7 {
		t.Fatalf("mission inspect identity = %+v board=%+v", inspect.Mission, inspect.Board)
	}
	gotChain := make([]string, 0, len(inspect.Chain))
	for _, node := range inspect.Chain {
		gotChain = append(gotChain, node.Kind+":"+node.ID+":"+node.Title)
	}
	wantChain := []string{
		"formation:fmn_draft:Draft poem",
		"gate:gate_review:Human review",
		"formation:fmn_polish:Polish poem",
	}
	if strings.Join(gotChain, "|") != strings.Join(wantChain, "|") {
		t.Fatalf("mission inspect chain = %v, want %v", gotChain, wantChain)
	}
	if len(inspect.Connections) != 3 || inspect.Connections[0].ID != "edge_mission_draft" {
		t.Fatalf("mission inspect connections = %+v, want reachable edges", inspect.Connections)
	}
	for _, browserOnly := range []string{"viewport", "undoStack", "toml"} {
		if strings.Contains(stdout, browserOnly) {
			t.Fatalf("mission inspect leaked browser-only/raw field %q: %s", browserOnly, stdout)
		}
	}
}

func TestArchonMissionInspectJSONSelectorErrorIsStructured(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("poems"), `schema = 1
id = "brd_poems"
slug = "poems"
title = "Poems"
rev = 1

[[mission]]
id = "mis_first"
title = "Simple poem"
goal = "One"
beadId = "home-vdki.33.1"

[[mission]]
id = "mis_second"
title = "Simple poem"
goal = "Two"
beadId = "home-vdki.33.2"
`)

	stdout, stderr, code := runArchon(t, &fakeTmux{live: map[string]bool{}}, "--workspace", workspace, "mission", "inspect", "poems", "simple-poem", "--json")
	if code == 0 {
		t.Fatalf("ambiguous mission inspect code=0 stdout=%s", stdout)
	}
	var response archonErrorResponse
	if err := json.Unmarshal([]byte(stderr), &response); err != nil {
		t.Fatalf("decode selector error: %v\nstderr=%s", err, stderr)
	}
	if response.Code != "ambiguous_selector" || response.Boundary != "mission" || response.Selector != "simple-poem" || !strings.Contains(response.Message, "ambiguous") {
		t.Fatalf("selector error = %+v, want structured ambiguous mission selector", response)
	}
}

func TestArchonGraphSelectorsFailLoudOnAmbiguousTitles(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("poems"), `schema = 1
id = "brd_poems"
slug = "poems"
title = "Poems"
rev = 1

[[mission]]
id = "mis_poem"
title = "Simple poem"
goal = "Create a simple poem"
beadId = "home-vdki.33.1"

[[formation]]
id = "fmn_first"
type = "solo"
title = "Draft poem"

[[formation.input]]
id = "port_first_in"
label = "Input"

[[formation.output]]
id = "port_first_out"
label = "Output"

[[formation.slot]]
id = "slot_writer"
label = "Writer"
controller = true

[[formation]]
id = "fmn_second"
type = "solo"
title = "Draft poem"

[[formation.input]]
id = "port_second_in"
label = "Input"

[[formation.output]]
id = "port_second_out"
label = "Output"

[[formation.slot]]
id = "slot_writer"
label = "Writer"
controller = true

[[gate]]
id = "gate_first"
title = "Review"
kinds = ["human"]
criterion = "Ready"

[[gate]]
id = "gate_second"
title = "Review"
kinds = ["human"]
criterion = "Ready"
`)
	runner := &fakeTmux{live: map[string]bool{}}

	_, stderr, code := runArchon(t, runner, "--workspace", workspace, "formation", "assign", "poems", "draft-poem", "--slot", "slot_writer", "--agent", "lab-poet", "--json")
	if code == 0 || !strings.Contains(stderr, "ambiguous") || !strings.Contains(stderr, "draft-poem") {
		t.Fatalf("ambiguous formation assign code=%d stderr=%s", code, stderr)
	}
	raw := readArchonFile(t, store.BoardPath("poems"))
	if strings.Contains(raw, `agentId = "lab-poet"`) {
		t.Fatalf("ambiguous formation assign mutated board:\n%s", raw)
	}

	_, stderr, code = runArchon(t, runner, "--workspace", workspace, "gate", "judge", "poems", "review", "--chain", "fmn_first", "--json")
	if code == 0 || !strings.Contains(stderr, "ambiguous") || !strings.Contains(stderr, "review") {
		t.Fatalf("ambiguous gate judge code=%d stderr=%s", code, stderr)
	}
	raw = readArchonFile(t, store.BoardPath("poems"))
	if strings.Contains(raw, `formation"]`) || strings.Contains(raw, `gate_first:judge`) || strings.Contains(raw, `gate_second:judge`) {
		t.Fatalf("ambiguous gate judge mutated board:\n%s", raw)
	}
}

func TestArchonFormationUnassignClearsSlotWithoutDeletingPersona(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("poems"), `schema = 1
id = "brd_poems"
slug = "poems"
title = "Poems"
rev = 7

[[formation]]
id = "fmn_draft"
type = "solo"
title = "Draft poem"

[[formation.slot]]
id = "slot_writer"
label = "Writer"
agentId = "lab-poet"
harness = "lab-fake"
controller = true
`)

	stdout, stderr, code := runArchon(t, &fakeTmux{live: map[string]bool{}}, "--workspace", workspace, "formation", "unassign", "poems", "draft-poem", "--slot", "slot_writer", "--json")
	if code != 0 {
		t.Fatalf("formation unassign code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	raw := readArchonFile(t, store.BoardPath("poems"))
	if strings.Contains(raw, "agentId") || strings.Contains(raw, "harness") {
		t.Fatalf("unassign left runtime assignment fields:\n%s", raw)
	}
	if !strings.Contains(raw, `id = "slot_writer"`) || !strings.Contains(raw, `controller = true`) {
		t.Fatalf("unassign removed the slot instead of clearing assignment:\n%s", raw)
	}
}

func TestArchonS3FormationAssignAndSetBrief(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_frame"
type = "peer"
title = "Frame"

[[formation.slot]]
id = "slot_peer_a"
label = "Peer A"
controller = false
`)
	runner := &fakeTmux{live: map[string]bool{}}

	stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "formation", "assign", "session-search", "fmn_frame", "--slot", "slot_peer_a", "--agent", "conductor", "--harness", "openai-codex", "--json")
	if code != 0 {
		t.Fatalf("formation assign code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	raw := readArchonFile(t, store.BoardPath("session-search"))
	if !strings.Contains(raw, `agentId = "conductor"`) || !strings.Contains(raw, `harness = "openai-codex"`) {
		t.Fatalf("assign did not persist slot binding:\n%s", raw)
	}
	if strings.Contains(raw, "sessionName") || strings.Contains(raw, "sessionStem") || strings.Contains(raw, "tmux") {
		t.Fatalf("assign persisted runtime session data:\n%s", raw)
	}

	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "formation", "set-brief", "session-search", "fmn_frame", "--goal", "Frame the goal", "--bead", "home-7kc4.5", "--file", "src/SessionPanel.tsx", "--link", "https://example.com/spec", "--json")
	if code != 0 {
		t.Fatalf("formation set-brief code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	raw = readArchonFile(t, store.BoardPath("session-search"))
	for _, want := range []string{
		`goal = "Frame the goal"`,
		`beadId = "home-7kc4.5"`,
		`files = ["src/SessionPanel.tsx"]`,
		`links = ["https://example.com/spec"]`,
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("set-brief missing %q:\n%s", want, raw)
		}
	}
}

func TestArchonS3FormationWireUnwireAndPorts(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_frame"
type = "solo"
title = "Frame"

[[formation.input]]
id = "port_frame_in"
label = "Input"

[[formation.output]]
id = "port_frame_out"
label = "Output"

[[formation]]
id = "fmn_ship"
type = "solo"
title = "Ship"

[[formation.input]]
id = "port_ship_in"
label = "Input"

[[formation.output]]
id = "port_ship_out"
label = "Output"
`)
	runner := &fakeTmux{live: map[string]bool{}}

	if stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "formation", "add-input", "session-search", "fmn_ship", "--label", "Second input", "--json"); code != 0 {
		t.Fatalf("add-input code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	raw := readArchonFile(t, store.BoardPath("session-search"))
	if !strings.Contains(raw, `label = "Second input"`) {
		t.Fatalf("add-input did not persist a stable input:\n%s", raw)
	}

	if stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "formation", "wire", "session-search", "fmn_frame:port_frame_out", "fmn_ship:port_ship_in", "--json"); code != 0 {
		t.Fatalf("wire code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	raw = readArchonFile(t, store.BoardPath("session-search"))
	if !strings.Contains(raw, `from = "fmn_frame:port_frame_out"`) || !strings.Contains(raw, `to = "fmn_ship:port_ship_in"`) {
		t.Fatalf("wire did not persist stable endpoints:\n%s", raw)
	}

	if stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "formation", "unwire", "session-search", "fmn_frame:port_frame_out", "fmn_ship:port_ship_in", "--json"); code != 0 {
		t.Fatalf("unwire code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	raw = readArchonFile(t, store.BoardPath("session-search"))
	if strings.Contains(raw, `from = "fmn_frame:port_frame_out"`) || strings.Contains(raw, `to = "fmn_ship:port_ship_in"`) {
		t.Fatalf("unwire left connection behind:\n%s", raw)
	}
}

func TestArchonS3GateCreate(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
`)
	runner := &fakeTmux{live: map[string]bool{}}

	stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "gate", "create", "session-search", "--kinds", "code,human", "--criterion", "research is sound and safe to build", "--x", "420", "--y", "260", "--json")
	if code != 0 {
		t.Fatalf("gate create code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	var jsonBoard formations.BoardDocument
	if err := json.Unmarshal([]byte(stdout), &jsonBoard); err != nil {
		t.Fatalf("gate create JSON did not decode as board document: %v\n%s", err, stdout)
	}
	if jsonBoard.ID != "brd_01J9_sesssearch" || len(jsonBoard.Gates) != 1 {
		t.Fatalf("gate create JSON = %+v, want board document with one gate", jsonBoard)
	}
	raw := readArchonFile(t, store.BoardPath("session-search"))
	if !strings.Contains(raw, `[[gate]]`) || !strings.Contains(raw, `kinds = ["code", "human"]`) || strings.Contains(raw, "verdict") || strings.Contains(raw, "onFail") {
		t.Fatalf("gate create persisted wrong fields:\n%s", raw)
	}
	if strings.Contains(raw, "x = 420") || strings.Contains(raw, "y = 260") {
		t.Fatalf("gate create leaked layout coordinates into board TOML:\n%s", raw)
	}
	layout, err := store.ReadLayout("session-search")
	if err != nil {
		t.Fatalf("read gate layout: %v", err)
	}
	if len(layout.Nodes) != 1 || layout.Nodes[0].ID != jsonBoard.Gates[0].ID || layout.Nodes[0].X != 420 || layout.Nodes[0].Y != 260 {
		t.Fatalf("gate layout nodes = %+v, want CLI-created gate at 420,260", layout.Nodes)
	}
}

func TestArchonS3GateJudgeChainAndDetach(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[gate]]
id = "gate_review"
title = "Review"
kinds = ["code"]
criterion = "Check it."

[[formation]]
id = "fmn_j1"
type = "solo"
title = "Judge 1"

[[formation.input]]
id = "port_j1_in"
label = "Input"

[[formation.output]]
id = "port_j1_out"
label = "Output"

[[formation]]
id = "fmn_j2"
type = "solo"
title = "Judge 2"

[[formation.input]]
id = "port_j2_in"
label = "Input"

[[formation.output]]
id = "port_j2_out"
label = "Output"
`)
	runner := &fakeTmux{live: map[string]bool{}}

	if stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "gate", "judge", "session-search", "gate_review", "--chain", "fmn_j1,fmn_j2", "--json"); code != 0 {
		t.Fatalf("gate judge code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	raw := readArchonFile(t, store.BoardPath("session-search"))
	if !strings.Contains(raw, `from = "gate_review:judge"`) || !strings.Contains(raw, `to = "gate_review:judge"`) || !strings.Contains(raw, `kinds = ["code", "formation"]`) {
		t.Fatalf("gate judge chain did not persist expected connections/kind:\n%s", raw)
	}

	if stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "gate", "judge", "session-search", "gate_review", "--detach", "--json"); code != 0 {
		t.Fatalf("gate judge detach code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	raw = readArchonFile(t, store.BoardPath("session-search"))
	if strings.Contains(raw, `gate_review:judge`) || strings.Contains(raw, `formation"]`) {
		t.Fatalf("gate judge detach left judge connection or kind:\n%s", raw)
	}
}

func TestArchonS3MissionCreateWireAcceptsProjectBeadID(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_frame"
type = "solo"
title = "Frame"

[[formation.input]]
id = "port_frame_in"
label = "Input"
`)
	runner := &fakeTmux{live: map[string]bool{}}

	if _, stderr, code := runArchon(t, runner, "--workspace", workspace, "mission", "create", "session-search", "--title", "Showcase", "--goal", "Build it", "--bead", "nohyphen"); code == 0 || !strings.Contains(stderr, "Beads issue id") {
		t.Fatalf("mission create unsafe bead code=%d stderr=%s, want rejection", code, stderr)
	}
	stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "mission", "create", "session-search", "--title", "Showcase", "--goal", "Build it", "--bead", "bd-204", "--x", "180", "--y", "95", "--json")
	if code != 0 {
		t.Fatalf("mission create code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	var jsonBoard formations.BoardDocument
	if err := json.Unmarshal([]byte(stdout), &jsonBoard); err != nil {
		t.Fatalf("mission create JSON did not decode as board document: %v\n%s", err, stdout)
	}
	if jsonBoard.ID != "brd_01J9_sesssearch" || len(jsonBoard.Missions) != 1 {
		t.Fatalf("mission create JSON = %+v, want board document with one mission", jsonBoard)
	}
	raw := readArchonFile(t, store.BoardPath("session-search"))
	if !strings.Contains(raw, `[[mission]]`) || !strings.Contains(raw, `beadId = "bd-204"`) || strings.Contains(raw, "chain") {
		t.Fatalf("mission create persisted wrong fields:\n%s", raw)
	}
	if strings.Contains(raw, "x = 180") || strings.Contains(raw, "y = 95") {
		t.Fatalf("mission create leaked layout coordinates into board TOML:\n%s", raw)
	}
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read mission board: %v", err)
	}
	if len(board.Missions) != 1 {
		t.Fatalf("missions = %+v, want one mission", board.Missions)
	}
	layout, err := store.ReadLayout("session-search")
	if err != nil {
		t.Fatalf("read mission layout: %v", err)
	}
	if len(layout.Nodes) != 1 || layout.Nodes[0].ID != board.Missions[0].ID || layout.Nodes[0].X != 180 || layout.Nodes[0].Y != 95 {
		t.Fatalf("mission layout nodes = %+v, want CLI-created mission at 180,95", layout.Nodes)
	}
	if stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "mission", "wire", "session-search", board.Missions[0].ID, "fmn_frame:port_frame_in", "--json"); code != 0 {
		t.Fatalf("mission wire code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
}

func TestArchonS4MissionRunFormationRunStatusLogsFollowAbort(t *testing.T) {
	workspace := t.TempDir()
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_AGENTS_DIR", agentsDir)

	personas := formations.NewPersonaStore(agentsDir)
	if _, err := personas.CreatePersona(formations.CreatePersonaRequest{
		ID:           "scout",
		Kind:         "specialist",
		Capabilities: []string{"research"},
		Harness:      "openai-codex",
	}); err != nil {
		t.Fatalf("create persona: %v", err)
	}
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("session-search"), archonS4BoardFixture())
	runner := &fakeTmux{live: map[string]bool{}}

	stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "mission", "run", "session-search", "--json")
	if code != 0 {
		t.Fatalf("mission run code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	missionRun := decodeArchonRunResponse(t, stdout)
	if missionRun.RunID == "" || missionRun.Status.Status != formations.RunStatusBlocked {
		t.Fatalf("mission run response = %+v, want fail-loud blocked run", missionRun)
	}

	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "run", "status", missionRun.RunID, "--json")
	if code != 0 {
		t.Fatalf("run status code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	status := decodeArchonStatus(t, stdout)
	if status.RunID != missionRun.RunID || status.Status != formations.RunStatusBlocked || status.Final {
		t.Fatalf("run status = %+v, want same non-final blocked projection", status)
	}

	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "run", "logs", missionRun.RunID, "--node", "fmn_work", "--json")
	if code != 0 {
		t.Fatalf("run logs code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	nodeEvents := decodeArchonEvents(t, stdout)
	if !eventsContain(nodeEvents, formations.RunEventNodeStarted, "fmn_work") || !eventsContain(nodeEvents, formations.RunEventError, "fmn_work") {
		t.Fatalf("node-filtered logs missing fail-loud work node events: %+v", nodeEvents)
	}
	if eventsContain(nodeEvents, formations.RunEventStarted, "") {
		t.Fatalf("node-filtered logs included run-level event: %+v", nodeEvents)
	}

	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "run", "logs", missionRun.RunID, "--json")
	if code != 0 {
		t.Fatalf("run logs code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	failLoudEvents := decodeArchonEvents(t, stdout)
	if !eventsContain(failLoudEvents, formations.RunEventStarted, "") || !eventsContain(failLoudEvents, formations.RunEventBlocked, "") {
		t.Fatalf("logs missing run start/block events: %+v", failLoudEvents)
	}

	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "formation", "run", "session-search", "work", "--json")
	if code != 0 {
		t.Fatalf("formation run code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	formationRun := decodeArchonRunResponse(t, stdout)
	if formationRun.Status.Status != formations.RunStatusBlocked {
		t.Fatalf("formation run status = %+v, want fail-loud blocked", formationRun.Status)
	}
	formationEvents, err := store.ReadRunEvents(formationRun.RunID)
	if err != nil {
		t.Fatalf("read formation run events: %v", err)
	}
	startedNodes := []string{}
	for _, event := range formationEvents {
		if event.Type == formations.RunEventNodeStarted {
			startedNodes = append(startedNodes, event.NodeID)
		}
	}
	if strings.Join(startedNodes, ",") != "fmn_work" {
		t.Fatalf("formation run started nodes = %v, want only fmn_work", startedNodes)
	}
	if !eventsContain(formationEvents, formations.RunEventError, "fmn_work") || eventsContain(formationEvents, formations.RunEventSucceeded, "") {
		t.Fatalf("formation run events = %+v, want run_error without fake success", formationEvents)
	}

	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board for open run: %v", err)
	}
	openRun, err := store.StartRun("session-search", formations.RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
	})
	if err != nil {
		t.Fatalf("start open run: %v", err)
	}
	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "run", "abort", openRun.RunID, "--reason", "operator stop", "--requested-by", "agent:test", "--json")
	if code != 0 {
		t.Fatalf("run abort code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	aborted := decodeArchonStatus(t, stdout)
	if aborted.Status != formations.RunStatusCanceled || !aborted.Final {
		t.Fatalf("abort status = %+v, want canceled final", aborted)
	}
	if err := store.AppendRunEvent(openRun.RunID, formations.RunEvent{Type: formations.RunEventNodeStarted, NodeID: "fmn_after_abort"}); !errors.Is(err, formations.ErrRunFinal) {
		t.Fatalf("append after abort error = %v, want ErrRunFinal", err)
	}
}

func TestArchonS4RunLogsFollowTailsUntilFinal(t *testing.T) {
	workspace := t.TempDir()
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_AGENTS_DIR", agentsDir)
	personas := formations.NewPersonaStore(agentsDir)
	if _, err := personas.CreatePersona(formations.CreatePersonaRequest{
		ID:      "scout",
		Kind:    "specialist",
		Harness: "openai-codex",
	}); err != nil {
		t.Fatalf("create persona: %v", err)
	}
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("session-search"), archonS4BoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	openRun, err := store.StartRun("session-search", formations.RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
	})
	if err != nil {
		t.Fatalf("start open run: %v", err)
	}
	appendDone := make(chan error, 1)
	go func() {
		time.Sleep(20 * time.Millisecond)
		appendDone <- store.AppendRunEvent(openRun.RunID, formations.RunEvent{
			Type:  formations.RunEventCanceled,
			Actor: "agent:test",
			Data: map[string]any{
				"reason": "follow observed final",
				"final":  true,
			},
		})
	}()

	runner := &fakeTmux{live: map[string]bool{}}
	stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "run", "logs", openRun.RunID, "--follow", "--json")
	if err := <-appendDone; err != nil {
		t.Fatalf("append final event: %v", err)
	}
	if code != 0 {
		t.Fatalf("run logs --follow code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if strings.Contains(stderr, "not implemented") {
		t.Fatalf("follow reported not implemented: %s", stderr)
	}
	events := decodeArchonEvents(t, stdout)
	if !eventsContain(events, formations.RunEventStarted, "") || !eventsContain(events, formations.RunEventCanceled, "") {
		t.Fatalf("follow logs did not tail through final event: %+v", events)
	}
}

func TestArchonS4RunLogsFollowJSONReturnsBlockedRun(t *testing.T) {
	workspace := t.TempDir()
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_AGENTS_DIR", agentsDir)
	personas := formations.NewPersonaStore(agentsDir)
	if _, err := personas.CreatePersona(formations.CreatePersonaRequest{
		ID:      "scout",
		Kind:    "specialist",
		Harness: "openai-codex",
	}); err != nil {
		t.Fatalf("create persona: %v", err)
	}
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("session-search"), archonS4BoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	openRun, err := store.StartRun("session-search", formations.RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if err := store.AppendRunEvent(openRun.RunID, formations.RunEvent{
		Type:   formations.RunEventBlocked,
		NodeID: "fmn_work",
		Data: map[string]any{
			"reason":         "waiting for operator",
			"blockedNodeId":  "fmn_work",
			"resumeAllowed":  true,
			"resumePolicy":   "explicit",
			"openDispatches": []map[string]any{},
			"nextEpoch":      1,
		},
	}); err != nil {
		t.Fatalf("append blocked event: %v", err)
	}

	type runResult struct {
		stdout string
		stderr string
		code   int
	}
	done := make(chan runResult, 1)
	go func() {
		stdout, stderr, code := runArchon(t, &fakeTmux{live: map[string]bool{}}, "--workspace", workspace, "run", "logs", openRun.RunID, "--follow", "--json")
		done <- runResult{stdout: stdout, stderr: stderr, code: code}
	}()

	select {
	case result := <-done:
		if result.code != 0 {
			t.Fatalf("run logs --follow --json code=%d stderr=%s stdout=%s", result.code, result.stderr, result.stdout)
		}
		events := decodeArchonEvents(t, result.stdout)
		if !eventsContain(events, formations.RunEventBlocked, "fmn_work") {
			t.Fatalf("follow logs = %+v, want blocked event", events)
		}
	case <-time.After(150 * time.Millisecond):
		t.Fatal("run logs --follow --json did not return for blocked resumable run")
	}
}

func TestArchonS5RunResumeCommandUsesEngine(t *testing.T) {
	workspace := t.TempDir()
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_AGENTS_DIR", agentsDir)
	personas := formations.NewPersonaStore(agentsDir)
	if _, err := personas.CreatePersona(formations.CreatePersonaRequest{ID: "scout", Kind: "specialist", Harness: "openai-codex"}); err != nil {
		t.Fatalf("create persona: %v", err)
	}
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("session-search"), archonS5CascadeBoardFixture())
	runner := &fakeTmux{live: map[string]bool{}}

	stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "mission", "run", "session-search", "--max-dispatch", "1", "--json")
	if code != 0 {
		t.Fatalf("mission run code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	blocked := decodeArchonRunResponse(t, stdout)
	if blocked.Status.Status != formations.RunStatusBlocked || !blocked.Status.ResumeAllowed {
		t.Fatalf("blocked run = %+v, want resumable block", blocked)
	}

	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "run", "resume", blocked.RunID, "--reason", "continue", "--json")
	if code != 0 {
		t.Fatalf("run resume code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	resumed := decodeArchonStatus(t, stdout)
	if resumed.RunID != blocked.RunID || resumed.Epoch != 1 || resumed.Status != formations.RunStatusBlocked {
		t.Fatalf("resume status = %+v, want same run blocked in epoch 1 without fake executor", resumed)
	}
}

func TestArchonS5GateApproveRoutesHumanGate(t *testing.T) {
	workspace := t.TempDir()
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_AGENTS_DIR", agentsDir)
	personas := formations.NewPersonaStore(agentsDir)
	if _, err := personas.CreatePersona(formations.CreatePersonaRequest{ID: "scout", Kind: "specialist", Harness: "openai-codex"}); err != nil {
		t.Fatalf("create persona: %v", err)
	}
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("session-search"), archonS5HumanGateBoardFixture())
	runner := &fakeTmux{live: map[string]bool{}}

	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read human gate board: %v", err)
	}
	engine := formations.NewRunEngine(store, personas, archonTestRunExecutor{})
	engine.SetGateEvaluator(archonTestGateEvaluator{verdict: "pass"})
	waiting, err := engine.RunMission("session-search", formations.RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
		Limits:            formations.RunLimits{MaxDispatch: 5, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("start human waiting run: %v", err)
	}
	if waiting.Status != formations.RunStatusRunning || waiting.Final {
		t.Fatalf("waiting run = %+v, want non-final human wait", waiting)
	}

	stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "gate", "approve", waiting.RunID, "gate_review", "--reason", "direction is right", "--json")
	if code != 0 {
		t.Fatalf("gate approve code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	approved := decodeArchonStatus(t, stdout)
	if approved.Status != formations.RunStatusBlocked || approved.Final {
		t.Fatalf("approved status = %+v, want blocked after human verdict routes to missing executor", approved)
	}
}

func TestArchonS4ConfiguredLabPoemMissionReachesGateAndPolishesAfterApproval(t *testing.T) {
	workspace := t.TempDir()
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_AGENTS_DIR", agentsDir)
	t.Setenv("CHROTE_FORMATIONS_LAB_HARNESSES", "lab-fake")
	t.Setenv("CHROTE_FORMATIONS_LAB_CWD", workspace)
	t.Setenv("CHROTE_FORMATIONS_LAB_ROOTS", workspace)

	personas := formations.NewPersonaStore(agentsDir)
	for _, id := range []string{"lab-poet", "lab-poem-reviewer"} {
		if _, err := personas.CreatePersona(formations.CreatePersonaRequest{
			ID:      id,
			Kind:    "specialist",
			Harness: "lab-fake",
		}); err != nil {
			t.Fatalf("create persona %s: %v", id, err)
		}
	}
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("poems"), archonS4PoemBoardFixture())
	runner := &fakeTmux{live: map[string]bool{}}

	stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "mission", "run", "poems", "--mission", "mis_poem", "--json")
	if code != 0 {
		t.Fatalf("mission run code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	started := decodeArchonRunResponse(t, stdout)
	events, err := store.ReadRunEvents(started.RunID)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if started.Status.Status != formations.RunStatusRunning || started.Status.Final {
		t.Fatalf("configured lab poem run status = %+v events=%s, want running at human gate without missing executor", started.Status, archonEventTypes(events))
	}
	if eventsContainErrorCode(events, "missing_executor") || eventsContainReason(events, "formation executor unavailable") {
		t.Fatalf("configured lab poem run still blocked as missing executor: %+v", events)
	}
	for _, want := range []struct {
		eventType string
		nodeID    string
	}{
		{formations.RunEventStarted, ""},
		{formations.RunEventNodeStarted, "fmn_draft"},
		{formations.RunEventSlotDispatch, "fmn_draft"},
		{formations.RunEventSlotResult, "fmn_draft"},
		{formations.RunEventNodeOutput, "fmn_draft"},
		{formations.RunEventGateEvaluating, "gate_review"},
		{formations.RunEventHumanInputRequested, "gate_review"},
	} {
		if !eventsContain(events, want.eventType, want.nodeID) {
			t.Fatalf("events %s missing %s for %s: %+v", archonEventTypes(events), want.eventType, want.nodeID, events)
		}
	}
	draftReport, err := store.ProjectRunNodeReport(started.RunID, "fmn_draft")
	if err != nil {
		t.Fatalf("project draft report: %v", err)
	}
	if !strings.Contains(draftReport.Text, "lab-poet") || !strings.Contains(draftReport.Text, "Create a simple poem") {
		t.Fatalf("draft report text = %q, want lab-poet output seeded by mission objective", draftReport.Text)
	}

	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "gate", "approve", started.RunID, "gate_review", "--reason", "draft approved", "--json")
	if code != 0 {
		t.Fatalf("gate approve code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	approved := decodeArchonStatus(t, stdout)
	if approved.Status != formations.RunStatusBlocked || approved.Final || !approved.ResumeAllowed {
		t.Fatalf("approved status = %+v, want resumable block before polish dispatch", approved)
	}
	events, err = store.ReadRunEvents(started.RunID)
	if err != nil {
		t.Fatalf("read approved events: %v", err)
	}
	if eventsContain(events, formations.RunEventNodeStarted, "fmn_polish") ||
		eventsContain(events, formations.RunEventSlotDispatch, "fmn_polish") ||
		eventsContain(events, formations.RunEventSucceeded, "") {
		t.Fatalf("approve dispatched or finalized before resume: %s", archonEventTypes(events))
	}

	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "run", "resume", started.RunID, "--reason", "gate approved", "--json")
	if code != 0 {
		t.Fatalf("run resume code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	resumed := decodeArchonStatus(t, stdout)
	if resumed.Status != formations.RunStatusSucceeded || !resumed.Final {
		t.Fatalf("resumed status = %+v, want final succeeded after polish", resumed)
	}
	events, err = store.ReadRunEvents(started.RunID)
	if err != nil {
		t.Fatalf("read final events: %v", err)
	}
	for _, want := range []struct {
		eventType string
		nodeID    string
	}{
		{formations.RunEventHumanVerdictRecorded, "gate_review"},
		{formations.RunEventGateVerdict, "gate_review"},
		{formations.RunEventResumed, ""},
		{formations.RunEventNodeStarted, "fmn_polish"},
		{formations.RunEventSlotDispatch, "fmn_polish"},
		{formations.RunEventSlotResult, "fmn_polish"},
		{formations.RunEventNodeOutput, "fmn_polish"},
		{formations.RunEventSucceeded, ""},
	} {
		if !eventsContain(events, want.eventType, want.nodeID) {
			t.Fatalf("final events %s missing %s for %s: %+v", archonEventTypes(events), want.eventType, want.nodeID, events)
		}
	}
}

func TestArchonPoemMissionRoundTripsThroughCLIAPIFileAndLedger(t *testing.T) {
	workspace := t.TempDir()
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_AGENTS_DIR", agentsDir)
	t.Setenv("CHROTE_FORMATIONS_LAB_HARNESSES", "lab-fake")
	t.Setenv("CHROTE_FORMATIONS_LAB_CWD", workspace)
	t.Setenv("CHROTE_FORMATIONS_LAB_ROOTS", workspace)

	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("poems"), `schema = 1
id = "brd_poems"
slug = "poems"
title = "Poems"
rev = 1
`)
	runner := &fakeTmux{live: map[string]bool{}}
	archon := func(args ...string) string {
		t.Helper()
		stdout, stderr, code := runArchon(t, runner, args...)
		if code != 0 {
			t.Fatalf("archon %q code=%d stderr=%s stdout=%s", strings.Join(args, " "), code, stderr, stdout)
		}
		return stdout
	}
	workspaceArgs := func(args ...string) []string {
		return append([]string{"--workspace", workspace}, args...)
	}

	for _, persona := range []struct {
		id   string
		kind string
	}{
		{id: "lab-poet", kind: "poet"},
		{id: "lab-poem-reviewer", kind: "reviewer"},
	} {
		archon(workspaceArgs("agent", "new", persona.id, "--kind", persona.kind, "--harness", "lab-fake", "--json")...)
	}

	boardList := decodeArchonBoardList(t, archon(workspaceArgs("board", "list", "--json")...))
	if len(boardList.Boards) != 1 || boardList.Boards[0].ID != "brd_poems" || boardList.Boards[0].Slug != "poems" {
		t.Fatalf("board list = %+v, want selected empty poems board", boardList)
	}

	archon(workspaceArgs("mission", "create", "poems", "--title", "Simple poem", "--goal", "Create a simple poem", "--bead", "home-vdki.34.1", "--json")...)
	archon(workspaceArgs("formation", "create", "poems", "solo", "--title", "Draft poem", "--x", "320", "--y", "120", "--json")...)
	archon(workspaceArgs("formation", "create", "poems", "solo", "--title", "Polish poem", "--x", "860", "--y", "120", "--json")...)
	archon(workspaceArgs("gate", "create", "poems", "--title", "Human review", "--kinds", "human", "--criterion", "Draft is ready to polish", "--json")...)

	board := decodeArchonBoard(t, archon(workspaceArgs("board", "inspect", "poems", "--json")...))
	mission := mustMissionByTitle(t, board, "Simple poem")
	draft := mustFormationByTitle(t, board, "Draft poem")
	polish := mustFormationByTitle(t, board, "Polish poem")
	gate := mustGateByTitle(t, board, "Human review")

	archon(workspaceArgs("formation", "assign", "poems", draft.ID, "--slot", draft.Slots[0].ID, "--agent", "lab-poet", "--harness", "lab-fake", "--json")...)
	archon(workspaceArgs("formation", "assign", "poems", polish.ID, "--slot", polish.Slots[0].ID, "--agent", "lab-poem-reviewer", "--harness", "lab-fake", "--json")...)
	archon(workspaceArgs("mission", "wire", "poems", mission.ID, draft.ID+":"+draft.Inputs[0].ID, "--json")...)
	archon(workspaceArgs("formation", "wire", "poems", draft.ID+":"+draft.Outputs[0].ID, gate.ID+":in", "--json")...)
	archon(workspaceArgs("formation", "wire", "poems", gate.ID+":pass", polish.ID+":"+polish.Inputs[0].ID, "--json")...)

	afterAuthoring := decodeArchonBoard(t, archon(workspaceArgs("board", "inspect", "poems", "--json")...))
	draft = mustFormationByTitle(t, afterAuthoring, "Draft poem")
	polish = mustFormationByTitle(t, afterAuthoring, "Polish poem")
	gate = mustGateByTitle(t, afterAuthoring, "Human review")
	mission = mustMissionByTitle(t, afterAuthoring, "Simple poem")
	if len(afterAuthoring.Connections) != 3 {
		t.Fatalf("connections = %+v, want mission->draft, draft->gate, gate->polish", afterAuthoring.Connections)
	}
	for _, edge := range afterAuthoring.Connections {
		if edge.ID == "" || !strings.HasPrefix(edge.ID, "edge_") {
			t.Fatalf("edge lacks stable id: %+v", edge)
		}
	}

	started := decodeArchonRunResponse(t, archon(workspaceArgs("mission", "run", "poems", "--mission", mission.ID, "--json")...))
	if started.RunID == "" || !strings.HasPrefix(started.RunID, "run_") {
		t.Fatalf("run id = %q, want stable run_ id", started.RunID)
	}
	if started.Status.Status != formations.RunStatusRunning || started.Status.Final {
		t.Fatalf("started status = %+v, want running at human gate", started.Status)
	}
	events, err := store.ReadRunEvents(started.RunID)
	if err != nil {
		t.Fatalf("read run events: %v", err)
	}
	for _, want := range []struct {
		eventType string
		nodeID    string
	}{
		{formations.RunEventStarted, ""},
		{formations.RunEventNodeStarted, draft.ID},
		{formations.RunEventSlotDispatch, draft.ID},
		{formations.RunEventSlotResult, draft.ID},
		{formations.RunEventNodeOutput, draft.ID},
		{formations.RunEventGateEvaluating, gate.ID},
		{formations.RunEventHumanInputRequested, gate.ID},
	} {
		if !eventsContain(events, want.eventType, want.nodeID) {
			t.Fatalf("events %s missing %s for %s: %+v", archonEventTypes(events), want.eventType, want.nodeID, events)
		}
	}

	approved := decodeArchonStatus(t, archon(workspaceArgs("gate", "approve", started.RunID, gate.ID, "--reason", "draft approved", "--json")...))
	if approved.Status != formations.RunStatusBlocked || approved.Final || !approved.ResumeAllowed {
		t.Fatalf("approved status = %+v, want resumable block before explicit resume", approved)
	}
	resumed := decodeArchonStatus(t, archon(workspaceArgs("run", "resume", started.RunID, "--reason", "gate approved", "--json")...))
	if resumed.Status != formations.RunStatusSucceeded || !resumed.Final {
		t.Fatalf("resumed status = %+v, want final success", resumed)
	}
	finalStatus := decodeArchonStatus(t, archon(workspaceArgs("run", "status", started.RunID, "--json")...))
	if finalStatus.RunID != started.RunID || finalStatus.BoardSlug != "poems" || finalStatus.MissionID != mission.ID || finalStatus.Status != formations.RunStatusSucceeded {
		t.Fatalf("final archon status = %+v, want same run/board/mission success", finalStatus)
	}

	handler := chroteapi.NewFormationsHandlerWithStores(store, formations.NewPersonaStore(agentsDir))
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	apiBoard := decodeAPIBoard(t, requestArchonFormationsAPI(t, mux, http.MethodGet, "/api/formations/boards/poems", ""))
	apiLayout := decodeAPILayout(t, requestArchonFormationsAPI(t, mux, http.MethodGet, "/api/formations/boards/poems/layout", ""))
	apiStatus := decodeAPIStatus(t, requestArchonFormationsAPI(t, mux, http.MethodGet, "/api/formations/runs/"+started.RunID, ""))
	apiEvents := decodeAPIEvents(t, requestArchonFormationsAPI(t, mux, http.MethodGet, "/api/formations/runs/"+started.RunID+"/events", ""))

	if apiBoard.ID != afterAuthoring.ID || apiBoard.Slug != afterAuthoring.Slug || apiBoard.Rev != afterAuthoring.Rev {
		t.Fatalf("api board identity = %+v, archon board = %+v", apiBoard, afterAuthoring)
	}
	if mustMissionByTitle(t, apiBoard, "Simple poem").ID != mission.ID ||
		mustFormationByTitle(t, apiBoard, "Draft poem").ID != draft.ID ||
		mustFormationByTitle(t, apiBoard, "Polish poem").ID != polish.ID ||
		mustGateByTitle(t, apiBoard, "Human review").ID != gate.ID {
		t.Fatalf("api board ids drifted: api=%+v archon=%+v", apiBoard, afterAuthoring)
	}
	if len(apiBoard.Connections) != len(afterAuthoring.Connections) || connectionIDs(apiBoard.Connections) != connectionIDs(afterAuthoring.Connections) {
		t.Fatalf("api connections drifted: api=%+v archon=%+v", apiBoard.Connections, afterAuthoring.Connections)
	}
	if apiStatus.RunID != started.RunID || apiStatus.Status != formations.RunStatusSucceeded || !apiStatus.Final {
		t.Fatalf("api status = %+v, want final success for same run", apiStatus)
	}
	if len(apiEvents) != finalStatus.EventCount || !eventsContain(apiEvents, formations.RunEventSucceeded, "") {
		t.Fatalf("api events = %s, archon event count = %d", archonEventTypes(apiEvents), finalStatus.EventCount)
	}

	boardRaw := readArchonFile(t, store.BoardPath("poems"))
	layoutRaw := readArchonFile(t, store.LayoutPath("poems"))
	runRaw := readArchonFile(t, filepath.Join(workspace, ".formations", "runs", "poems", started.RunID+".ndjson"))
	if strings.Contains(boardRaw, "[[node]]") || strings.Contains(boardRaw, "x = 320") || strings.Contains(boardRaw, "y = 120") {
		t.Fatalf("board file contains layout sidecar data:\n%s", boardRaw)
	}
	for _, id := range []string{draft.ID, polish.ID} {
		if !layoutHasNode(apiLayout, id) || !strings.Contains(layoutRaw, `id = "`+id+`"`) {
			t.Fatalf("layout sidecar missing node %s:\n%s", id, layoutRaw)
		}
	}
	for _, id := range []string{mission.ID, draft.ID, polish.ID, gate.ID, draft.Slots[0].ID, draft.Inputs[0].ID, draft.Outputs[0].ID, started.RunID} {
		if !strings.Contains(boardRaw+runRaw, id) {
			t.Fatalf("stable id %s missing from board/run files", id)
		}
	}
}

func TestDefaultCockpitPatchRoundTripsToArchonFilesAndLayoutSidecar(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("poems"), `schema = 1
id = "brd_poems"
slug = "poems"
title = "Poems"
rev = 3

[[mission]]
id = "mis_poem"
title = "Simple poem"
goal = "Create a simple poem"
beadId = "home-vdki.34"

[[formation]]
id = "fmn_draft"
type = "solo"
title = "Draft poem"

[[formation.input]]
id = "in"
label = "Input"

[[formation.output]]
id = "out"
label = "Output"

[[formation.slot]]
id = "slot_writer"
label = "Writer"
controller = false
`)
	writeArchonFile(t, store.LayoutPath("poems"), `schema = 1
boardId = "brd_poems"
boardRev = 3

[[node]]
id = "mis_poem"
x = 80
y = 120

[[node]]
id = "fmn_draft"
x = 320
y = 120
`)
	handler := chroteapi.NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	requestAPI := func(method, path, body, etag string) []byte {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if etag != "" {
			req.Header.Set("If-Match", etag)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s status=%d body=%s", method, path, rec.Code, rec.Body.String())
		}
		return rec.Body.Bytes()
	}

	board, err := store.ReadBoard("poems")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	assignRaw := requestAPI(
		http.MethodPatch,
		"/api/formations/boards/poems",
		`{"assignSlot":{"formationId":"fmn_draft","slotId":"slot_writer","agentId":"lab-poet","harness":"lab-fake"},"expectedRev":3,"updatedBy":"agent:ui"}`,
		board.ETag,
	)
	assigned := decodeAPIBoard(t, assignRaw)
	if assigned.Rev != 4 || assigned.Formations[0].Slots[0].AgentID != "lab-poet" || assigned.Formations[0].Slots[0].Harness != "lab-fake" {
		t.Fatalf("assigned board = %+v, want UI slot assignment persisted through API", assigned)
	}

	board, err = store.ReadBoard("poems")
	if err != nil {
		t.Fatalf("read assigned board: %v", err)
	}
	wiredRaw := requestAPI(
		http.MethodPatch,
		"/api/formations/boards/poems",
		`{"wireConnection":{"from":"mis_poem:out","to":"fmn_draft:in"},"expectedRev":4,"updatedBy":"agent:ui"}`,
		board.ETag,
	)
	wired := decodeAPIBoard(t, wiredRaw)
	if wired.Rev != 5 || len(wired.Connections) != 1 || wired.Connections[0].From != "mis_poem:out" || wired.Connections[0].To != "fmn_draft:in" {
		t.Fatalf("wired board = %+v, want UI wire persisted as stable board connection", wired)
	}

	layout, err := store.ReadLayout("poems")
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}
	requestAPI(
		http.MethodPatch,
		"/api/formations/boards/poems/layout",
		`{"nodes":[{"id":"fmn_draft","x":444,"y":222}]}`,
		layout.ETag,
	)

	stdout, stderr, code := runArchon(t, &fakeTmux{live: map[string]bool{}}, "--workspace", workspace, "board", "inspect", "poems", "--json")
	if code != 0 {
		t.Fatalf("archon inspect code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	archonBoard := decodeArchonBoard(t, stdout)
	if archonBoard.Formations[0].Slots[0].ID != "slot_writer" ||
		archonBoard.Formations[0].Slots[0].AgentID != "lab-poet" ||
		len(archonBoard.Connections) != 1 ||
		archonBoard.Connections[0].ID == "" ||
		archonBoard.Connections[0].From != "mis_poem:out" ||
		archonBoard.Connections[0].To != "fmn_draft:in" {
		t.Fatalf("archon board = %+v, want API/UI mutation visible without structural drift", archonBoard)
	}
	boardRaw := readArchonFile(t, store.BoardPath("poems"))
	layoutRaw := readArchonFile(t, store.LayoutPath("poems"))
	if strings.Contains(boardRaw, "[[node]]") || strings.Contains(boardRaw, "x = 444") || strings.Contains(boardRaw, "sessionName") {
		t.Fatalf("board file leaked layout/runtime state:\n%s", boardRaw)
	}
	if !strings.Contains(layoutRaw, `id = "fmn_draft"`) || !strings.Contains(layoutRaw, "x = 444") || !strings.Contains(layoutRaw, "y = 222") {
		t.Fatalf("layout sidecar missing UI node move:\n%s", layoutRaw)
	}
}

func TestArchonS4ConfiguredLabExecutorMissingRootBlocksWithSpecificReason(t *testing.T) {
	workspace := t.TempDir()
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_AGENTS_DIR", agentsDir)
	t.Setenv("CHROTE_FORMATIONS_LAB_HARNESSES", "lab-fake")
	t.Setenv("CHROTE_FORMATIONS_LAB_CWD", workspace)

	personas := formations.NewPersonaStore(agentsDir)
	if _, err := personas.CreatePersona(formations.CreatePersonaRequest{
		ID:      "lab-poet",
		Kind:    "specialist",
		Harness: "lab-fake",
	}); err != nil {
		t.Fatalf("create persona: %v", err)
	}
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("poems"), archonS4PoemMissingRootBoardFixture())

	stdout, stderr, code := runArchon(t, &fakeTmux{live: map[string]bool{}}, "--workspace", workspace, "mission", "run", "poems", "--mission", "mis_poem", "--json")
	if code != 0 {
		t.Fatalf("mission run code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	started := decodeArchonRunResponse(t, stdout)
	if started.Status.Status != formations.RunStatusBlocked || !started.Status.ResumeAllowed {
		t.Fatalf("status = %+v, want resumable block for missing lab root", started.Status)
	}
	events, err := store.ReadRunEvents(started.RunID)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if eventsContainErrorCode(events, "missing_executor") || eventsContainReason(events, "formation executor unavailable") {
		t.Fatalf("configured but incomplete lab executor reported generic missing executor: %+v", events)
	}
	if !eventsContainErrorCode(events, "missing_root") || !eventsContainReason(events, "lab executor root is not configured") {
		t.Fatalf("events = %+v, want missing_root lab configuration block", events)
	}
}

func TestArchonS5RunAskSurfacesOpenEscalations(t *testing.T) {
	workspace := t.TempDir()
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_AGENTS_DIR", agentsDir)
	personas := formations.NewPersonaStore(agentsDir)
	if _, err := personas.CreatePersona(formations.CreatePersonaRequest{ID: "scout", Kind: "specialist", Harness: "openai-codex"}); err != nil {
		t.Fatalf("create persona: %v", err)
	}
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("session-search"), archonS4BoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	started, err := store.StartRun("session-search", formations.RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if _, err := store.RecordEscalationFromCapture(started.RunID, "fmn_work", "<<<CHROTE-ESCALATE run-id="+started.RunID+" severity=needs-attention reason='found a better direction'>>>"); err != nil {
		t.Fatalf("record escalation: %v", err)
	}

	stdout, stderr, code := runArchon(t, &fakeTmux{live: map[string]bool{}}, "--workspace", workspace, "run", "ask", started.RunID, "anything need me?")
	if code != 0 {
		t.Fatalf("run ask code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "found a better direction") || !strings.Contains(stdout, "fmn_work") {
		t.Fatalf("run ask output = %q, want escalation reason and node", stdout)
	}
}

func TestArchonRunListJSONListsDurableRunsAndFiltersBoard(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	alphaRun := startArchonLedgerRun(t, store, "alpha", "brd_alpha", "mis_alpha")
	betaRun := startArchonLedgerRun(t, store, "beta", "brd_beta", "mis_beta")
	if err := store.AppendRunEvent(alphaRun.RunID, formations.RunEvent{Type: formations.RunEventSucceeded}); err != nil {
		t.Fatalf("finish alpha run: %v", err)
	}
	if err := store.AppendRunEvent(betaRun.RunID, formations.RunEvent{
		Type: formations.RunEventBlocked,
		Data: map[string]any{
			"reason":        "beta needs operator",
			"resumeAllowed": true,
		},
	}); err != nil {
		t.Fatalf("block beta run: %v", err)
	}

	stdout, stderr, code := runArchon(t, &fakeTmux{live: map[string]bool{}}, "--workspace", workspace, "run", "list", "--json")
	if code != 0 {
		t.Fatalf("run list --json code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	list := decodeArchonRunList(t, stdout)
	if len(list.Runs) != 2 {
		t.Fatalf("run list returned %+v, want two durable ledger runs", list.Runs)
	}
	byRunID := map[string]formations.RunStatusProjection{}
	for _, run := range list.Runs {
		byRunID[run.RunID] = run
	}
	if byRunID[alphaRun.RunID].BoardSlug != "alpha" || byRunID[alphaRun.RunID].Status != formations.RunStatusSucceeded || !byRunID[alphaRun.RunID].Final {
		t.Fatalf("alpha run projection = %+v, want durable succeeded run", byRunID[alphaRun.RunID])
	}
	if byRunID[betaRun.RunID].BoardSlug != "beta" || byRunID[betaRun.RunID].Status != formations.RunStatusBlocked || !byRunID[betaRun.RunID].ResumeAllowed {
		t.Fatalf("beta run projection = %+v, want durable blocked run", byRunID[betaRun.RunID])
	}

	stdout, stderr, code = runArchon(t, &fakeTmux{live: map[string]bool{}}, "--workspace", workspace, "run", "list", "--board", "brd_alpha", "--json")
	if code != 0 {
		t.Fatalf("run list --board --json code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	filtered := decodeArchonRunList(t, stdout)
	if len(filtered.Runs) != 1 || filtered.Runs[0].RunID != alphaRun.RunID || filtered.Runs[0].BoardSlug != "alpha" {
		t.Fatalf("board-filtered run list = %+v, want only alpha run", filtered.Runs)
	}
}

func TestArchonRunFollowJSONEmitsNDJSONUntilFinal(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	started := startArchonLedgerRun(t, store, "alpha", "brd_alpha", "mis_alpha")
	if err := store.AppendRunEvent(started.RunID, formations.RunEvent{Type: formations.RunEventNodeStarted, NodeID: "fmn_work"}); err != nil {
		t.Fatalf("append node_started: %v", err)
	}

	type runResult struct {
		stdout string
		stderr string
		code   int
	}
	done := make(chan runResult, 1)
	go func() {
		stdout, stderr, code := runArchon(t, &fakeTmux{live: map[string]bool{}}, "--workspace", workspace, "run", "follow", started.RunID, "--json")
		done <- runResult{stdout: stdout, stderr: stderr, code: code}
	}()
	time.Sleep(20 * time.Millisecond)
	if err := store.AppendRunEvent(started.RunID, formations.RunEvent{Type: formations.RunEventSucceeded}); err != nil {
		t.Fatalf("append final event: %v", err)
	}

	var result runResult
	select {
	case result = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run follow --json did not terminate after final ledger event")
	}
	if result.code != 0 {
		t.Fatalf("run follow --json code=%d stderr=%s stdout=%s", result.code, result.stderr, result.stdout)
	}
	if strings.HasPrefix(strings.TrimSpace(result.stdout), "[") {
		t.Fatalf("run follow --json emitted a JSON array instead of NDJSON: %s", result.stdout)
	}
	var asArray []formations.RunEvent
	if err := json.Unmarshal([]byte(result.stdout), &asArray); err == nil {
		t.Fatalf("run follow --json output decoded as a JSON array; want one JSON event per line: %+v", asArray)
	}
	events := decodeArchonNDJSONEvents(t, result.stdout)
	if len(events) != 3 || events[0].Type != formations.RunEventStarted || events[1].Type != formations.RunEventNodeStarted || events[2].Type != formations.RunEventSucceeded {
		t.Fatalf("follow NDJSON events = %+v, want started, node_started, succeeded", events)
	}
}

func TestArchonRunAskJSONIncludesDurableLedgerEvidence(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	started := startArchonLedgerRun(t, store, "session-search", "brd_session_search", "mis_showcase")
	if err := store.AppendRunEvent(started.RunID, formations.RunEvent{Type: formations.RunEventNodeStarted, NodeID: "fmn_work"}); err != nil {
		t.Fatalf("append node_started: %v", err)
	}
	if err := store.AppendRunEvent(started.RunID, formations.RunEvent{
		Type:   formations.RunEventNodeOutput,
		NodeID: "fmn_work",
		Data: map[string]any{
			"status":    "done",
			"reportRef": "reports/fmn_work.md",
			"text":      "durable output from fmn_work",
		},
	}); err != nil {
		t.Fatalf("append node_output: %v", err)
	}
	if recorded, err := store.RecordEscalationFromCapture(started.RunID, "fmn_work", "<<<CHROTE-ESCALATE run-id="+started.RunID+" severity=needs-attention reason='operator should review output'>>>"); err != nil || !recorded {
		t.Fatalf("record escalation recorded=%v err=%v", recorded, err)
	}
	if err := store.AppendRunEvent(started.RunID, formations.RunEvent{
		Type:   formations.RunEventHumanInputRequested,
		NodeID: "gate_review",
		GateID: "gate_review",
		Data: map[string]any{
			"prompt":  "Approve durable output?",
			"choices": []string{"pass", "fail"},
		},
	}); err != nil {
		t.Fatalf("append human input request: %v", err)
	}
	if err := store.AppendRunEvent(started.RunID, formations.RunEvent{
		Type:   formations.RunEventBlocked,
		NodeID: "fmn_work",
		Data: map[string]any{
			"reason":        "waiting for operator",
			"code":          "operator_required",
			"blockedNodeId": "fmn_work",
			"resumeAllowed": true,
		},
	}); err != nil {
		t.Fatalf("append blocked event: %v", err)
	}

	stdout, stderr, code := runArchon(t, &fakeTmux{live: map[string]bool{}}, "--workspace", workspace, "run", "ask", started.RunID, "what happened?", "--json")
	if code != 0 {
		t.Fatalf("run ask --json code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	var response archonRunAskResponse
	if err := json.Unmarshal([]byte(stdout), &response); err != nil {
		t.Fatalf("decode run ask response: %v\n%s", err, stdout)
	}
	if response.RunID != started.RunID || response.Question != "what happened?" || response.Status == nil || response.Status.Status != formations.RunStatusBlocked {
		t.Fatalf("ask identity/status = %+v, want blocked status with question", response)
	}
	if len(response.OpenEscalations) != 1 || response.OpenEscalations[0].Reason != "operator should review output" {
		t.Fatalf("open escalations = %+v, want recorded sentinel escalation", response.OpenEscalations)
	}
	if len(response.CompletedNodes) != 1 || response.CompletedNodes[0].NodeID != "fmn_work" || response.CompletedNodes[0].ReportRef != "reports/fmn_work.md" || response.CompletedNodes[0].Text != "durable output from fmn_work" {
		t.Fatalf("completed nodes = %+v, want durable node_output evidence", response.CompletedNodes)
	}
	if len(response.ProducedOutputs) != 1 || response.ProducedOutputs[0].NodeID != "fmn_work" || response.ProducedOutputs[0].Text == "" {
		t.Fatalf("produced outputs = %+v, want durable produced output", response.ProducedOutputs)
	}
	if len(response.WaitingGates) != 1 || response.WaitingGates[0].GateID != "gate_review" || response.WaitingGates[0].Prompt != "Approve durable output?" {
		t.Fatalf("waiting gates = %+v, want pending human gate evidence", response.WaitingGates)
	}
	if len(response.BlockedReasons) != 1 || response.BlockedReasons[0].Reason != "waiting for operator" || response.BlockedReasons[0].Code != "operator_required" || !response.BlockedReasons[0].ResumeAllowed {
		t.Fatalf("blocked reasons = %+v, want ledger block reason", response.BlockedReasons)
	}
	for _, seq := range []int{response.OpenEscalations[0].Seq, response.CompletedNodes[0].OutputSeq, response.WaitingGates[0].RequestedSeq, response.BlockedReasons[0].Seq} {
		if !intSliceContains(response.EvidenceSeqs, seq) {
			t.Fatalf("evidence seqs = %v, want seq %d from durable run evidence", response.EvidenceSeqs, seq)
		}
	}
	if !strings.Contains(response.Answer, "completed nodes: fmn_work") || !strings.Contains(response.Answer, "latest output from fmn_work") || !strings.Contains(response.Answer, "latest block: waiting for operator") {
		t.Fatalf("answer = %q, want summary of durable evidence", response.Answer)
	}
}

func TestArchonRunJSONStatusNotFoundErrorIsStructured(t *testing.T) {
	workspace := t.TempDir()
	stdout, stderr, code := runArchon(t, &fakeTmux{live: map[string]bool{}}, "--workspace", workspace, "run", "status", "run_missing", "--json")
	if code == 0 {
		t.Fatalf("run status missing code=0 stdout=%s", stdout)
	}
	if stdout != "" {
		t.Fatalf("run status missing stdout=%q, want structured error on stderr only", stdout)
	}
	var response archonErrorResponse
	if err := json.Unmarshal([]byte(stderr), &response); err != nil {
		t.Fatalf("decode JSON error envelope: %v\nstderr=%s", err, stderr)
	}
	if response.Code != "not_found" || response.Boundary != "run" || response.Selector != "run_missing" || response.Message == "" {
		t.Fatalf("structured error = %+v, want run not_found envelope", response)
	}
}

func runArchon(t *testing.T, runner *fakeTmux, args ...string) (string, string, int) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(args, &stdout, &stderr, runner)
	return stdout.String(), stderr.String(), code
}

func readArchonFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func writeArchonFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func startArchonLedgerRun(t *testing.T, store *formations.Store, slug, boardID, missionID string) *formations.RunStartResult {
	t.Helper()
	writeArchonFile(t, store.BoardPath(slug), archonMinimalRunBoardFixture(slug, boardID, missionID))
	started, err := store.StartRun(slug, formations.RunStartRequest{
		MissionID: missionID,
		Actor:     "agent:test",
	})
	if err != nil {
		t.Fatalf("start run for board %s: %v", slug, err)
	}
	return started
}

func intSliceContains(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type archonRunResponse struct {
	RunID  string                         `json:"runId"`
	Status formations.RunStatusProjection `json:"status"`
}

func decodeArchonRunResponse(t *testing.T, raw string) archonRunResponse {
	t.Helper()
	var response archonRunResponse
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		t.Fatalf("decode run response: %v\n%s", err, raw)
	}
	return response
}

func decodeArchonStatus(t *testing.T, raw string) formations.RunStatusProjection {
	t.Helper()
	var status formations.RunStatusProjection
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		t.Fatalf("decode status response: %v\n%s", err, raw)
	}
	return status
}

func decodeArchonEvents(t *testing.T, raw string) []formations.RunEvent {
	t.Helper()
	var events []formations.RunEvent
	if err := json.Unmarshal([]byte(raw), &events); err != nil {
		t.Fatalf("decode events response: %v\n%s", err, raw)
	}
	return events
}

func decodeArchonNDJSONEvents(t *testing.T, raw string) []formations.RunEvent {
	t.Helper()
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		t.Fatal("empty NDJSON event stream")
	}
	if strings.HasPrefix(trimmed, "[") {
		t.Fatalf("NDJSON stream started with JSON array: %s", raw)
	}
	lines := strings.Split(trimmed, "\n")
	events := make([]formations.RunEvent, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event formations.RunEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode NDJSON event line %q: %v\nstream=%s", line, err, raw)
		}
		events = append(events, event)
	}
	return events
}

func decodeArchonRunList(t *testing.T, raw string) struct {
	Runs []formations.RunStatusProjection `json:"runs"`
} {
	t.Helper()
	var list struct {
		Runs []formations.RunStatusProjection `json:"runs"`
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		t.Fatalf("decode run list: %v\n%s", err, raw)
	}
	return list
}

func decodeArchonBoardList(t *testing.T, raw string) struct {
	Boards []formations.BoardSummary `json:"boards"`
} {
	t.Helper()
	var list struct {
		Boards []formations.BoardSummary `json:"boards"`
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		t.Fatalf("decode board list: %v\n%s", err, raw)
	}
	return list
}

func decodeArchonBoard(t *testing.T, raw string) formations.BoardDocument {
	t.Helper()
	var board formations.BoardDocument
	if err := json.Unmarshal([]byte(raw), &board); err != nil {
		t.Fatalf("decode board: %v\n%s", err, raw)
	}
	return board
}

func requestArchonFormationsAPI(t *testing.T, mux *http.ServeMux, method, path, body string) []byte {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s %s status=%d body=%s", method, path, rec.Code, rec.Body.String())
	}
	return rec.Body.Bytes()
}

func decodeAPIBoard(t *testing.T, raw []byte) formations.BoardDocument {
	t.Helper()
	var response struct {
		Data struct {
			Board formations.BoardDocument `json:"board"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode api board: %v\n%s", err, string(raw))
	}
	return response.Data.Board
}

func decodeAPILayout(t *testing.T, raw []byte) formations.LayoutDocument {
	t.Helper()
	var response struct {
		Data struct {
			Layout formations.LayoutDocument `json:"layout"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode api layout: %v\n%s", err, string(raw))
	}
	return response.Data.Layout
}

func decodeAPIStatus(t *testing.T, raw []byte) formations.RunStatusProjection {
	t.Helper()
	var response struct {
		Data struct {
			Status formations.RunStatusProjection `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode api status: %v\n%s", err, string(raw))
	}
	return response.Data.Status
}

func decodeAPIEvents(t *testing.T, raw []byte) []formations.RunEvent {
	t.Helper()
	var response struct {
		Data struct {
			Events []formations.RunEvent `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode api events: %v\n%s", err, string(raw))
	}
	return response.Data.Events
}

func mustMissionByTitle(t *testing.T, board formations.BoardDocument, title string) formations.MissionNode {
	t.Helper()
	for _, mission := range board.Missions {
		if mission.Title == title {
			return mission
		}
	}
	t.Fatalf("mission %q not found in %+v", title, board.Missions)
	return formations.MissionNode{}
}

func mustFormationByTitle(t *testing.T, board formations.BoardDocument, title string) formations.FormationNode {
	t.Helper()
	for _, formation := range board.Formations {
		if formation.Title == title {
			return formation
		}
	}
	t.Fatalf("formation %q not found in %+v", title, board.Formations)
	return formations.FormationNode{}
}

func mustGateByTitle(t *testing.T, board formations.BoardDocument, title string) formations.GateNode {
	t.Helper()
	for _, gate := range board.Gates {
		if gate.Title == title {
			return gate
		}
	}
	t.Fatalf("gate %q not found in %+v", title, board.Gates)
	return formations.GateNode{}
}

func connectionIDs(connections []formations.BoardConnection) string {
	ids := make([]string, 0, len(connections))
	for _, connection := range connections {
		ids = append(ids, connection.ID)
	}
	return strings.Join(ids, ",")
}

func layoutHasNode(layout formations.LayoutDocument, id string) bool {
	for _, node := range layout.Nodes {
		if node.ID == id {
			return true
		}
	}
	return false
}

func eventsContain(events []formations.RunEvent, typ, nodeID string) bool {
	for _, event := range events {
		if event.Type != typ {
			continue
		}
		if nodeID == "" || event.NodeID == nodeID {
			return true
		}
	}
	return false
}

func eventsContainErrorCode(events []formations.RunEvent, code string) bool {
	for _, event := range events {
		if event.Type != formations.RunEventError || event.Data == nil {
			continue
		}
		if event.Data["code"] == code {
			return true
		}
	}
	return false
}

func eventsContainReason(events []formations.RunEvent, reason string) bool {
	for _, event := range events {
		if event.Data == nil {
			continue
		}
		if event.Data["reason"] == reason || event.Data["message"] == reason {
			return true
		}
	}
	return false
}

func archonEventTypes(events []formations.RunEvent) string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		if event.NodeID != "" {
			types = append(types, event.Type+":"+event.NodeID)
		} else {
			types = append(types, event.Type)
		}
	}
	return strings.Join(types, ",")
}

type archonTestRunExecutor struct{}

func (archonTestRunExecutor) ExecuteFormation(req formations.FormationExecution) (formations.FormationExecutionResult, error) {
	text := "archon test output " + req.NodeID
	reportRef := "refs/" + req.NodeID + ".md"
	return formations.FormationExecutionResult{
		Status:    "done",
		ReportRef: reportRef,
		Text:      text,
		Outputs:   archonPayloadsForFormationOutputs(req.Formation, text, reportRef),
	}, nil
}

func archonPayloadsForFormationOutputs(formation formations.FormationNode, text, reportRef string) map[string]formations.FormationOutputPayload {
	outputs := make(map[string]formations.FormationOutputPayload, len(formation.Outputs))
	for _, port := range formation.Outputs {
		outputs[port.ID] = formations.FormationOutputPayload{Text: text, ReportRef: reportRef}
	}
	return outputs
}

type archonTestGateEvaluator struct {
	verdict string
}

func (e archonTestGateEvaluator) EvaluateGate(formations.GateEvaluation) (formations.GateEvaluationResult, error) {
	verdict := e.verdict
	if verdict == "" {
		verdict = "pass"
	}
	return formations.GateEvaluationResult{Verdict: verdict, Reason: "archon test " + verdict}, nil
}

func archonMinimalRunBoardFixture(slug, boardID, missionID string) string {
	return `schema = 1
id = "` + boardID + `"
slug = "` + slug + `"
title = "` + slug + ` board"
rev = 1

[[mission]]
id = "` + missionID + `"
title = "Mission"
goal = "Exercise durable run ledger"
beadId = "home-test.1"
`
}

func archonS4BoardFixture() string {
	return `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedBy = "agent:archon"
updatedAt = "2026-06-03T16:00:00Z"

[[mission]]
id = "mis_showcase"
title = "Showcase"
goal = "Ship a showcase"
beadId = "home-7kc4.7"

[[formation]]
id = "fmn_work"
type = "solo"
title = "Work"

[formation.brief]
goal = "Ship the isolated work"
beadId = "home-7kc4.7"

[[formation.input]]
id = "port_work_in"
label = "Input"

[[formation.output]]
id = "port_work_out"
label = "Output"

[[formation.slot]]
id = "slot_work"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[connection]]
id = "edge_mission_work"
from = "mis_showcase:out"
to = "fmn_work:port_work_in"
`
}

func archonS5CascadeBoardFixture() string {
	return `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7

[[mission]]
id = "mis_showcase"
title = "Showcase"
goal = "Ship a showcase"
beadId = "home-7kc4.8"

[[formation]]
id = "fmn_work"
type = "solo"
title = "Work"

[[formation.input]]
id = "port_work_in"
label = "Input"

[[formation.output]]
id = "port_work_out"
label = "Output"

[[formation.slot]]
id = "slot_work"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[formation]]
id = "fmn_ship"
type = "solo"
title = "Ship"

[[formation.input]]
id = "port_ship_in"
label = "Input"

[[formation.output]]
id = "port_ship_out"
label = "Output"

[[formation.slot]]
id = "slot_ship"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[connection]]
id = "edge_mission_work"
from = "mis_showcase:out"
to = "fmn_work:port_work_in"

[[connection]]
id = "edge_work_ship"
from = "fmn_work:port_work_out"
to = "fmn_ship:port_ship_in"
`
}

func archonS5HumanGateBoardFixture() string {
	return strings.Replace(s5HumanGateBoardFixtureForArchon(), "home-7kc4.7", "home-7kc4.8", 1)
}

func archonS4PoemMissingRootBoardFixture() string {
	return `schema = 1
id = "brd_poems"
slug = "poems"
title = "Poems"
rev = 7

[[mission]]
id = "mis_poem"
title = "Simple poem"
goal = "Create a simple poem"
beadId = "home-vdki.33.1"

[[formation]]
id = "fmn_draft"
type = "solo"
title = "Draft poem"

[[formation.input]]
id = "port_draft_in"
label = "Input"

[[formation.output]]
id = "port_draft_out"
label = "Output"

[[formation.slot]]
id = "slot_writer"
label = "Writer"
agentId = "lab-poet"
harness = "lab-fake"
controller = true

[[connection]]
id = "edge_mission_draft"
from = "mis_poem:out"
to = "fmn_draft:port_draft_in"
`
}

func archonS4PoemBoardFixture() string {
	return archonS4PoemMissingRootBoardFixture() + `
[[gate]]
id = "gate_review"
title = "Human review"
kinds = ["human"]
criterion = "Draft is ready to polish"

[[formation]]
id = "fmn_polish"
type = "solo"
title = "Polish poem"

[[formation.input]]
id = "port_polish_in"
label = "Input"

[[formation.output]]
id = "port_polish_out"
label = "Output"

[[formation.slot]]
id = "slot_reviewer"
label = "Reviewer"
agentId = "lab-poem-reviewer"
harness = "lab-fake"
controller = true

[[connection]]
id = "edge_draft_gate"
from = "fmn_draft:port_draft_out"
to = "gate_review:in"

[[connection]]
id = "edge_gate_pass_polish"
from = "gate_review:pass"
to = "fmn_polish:port_polish_in"
`
}

func s5HumanGateBoardFixtureForArchon() string {
	return `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7

[[mission]]
id = "mis_showcase"
title = "Showcase"
goal = "Ship a showcase"
beadId = "home-7kc4.7"

[[formation]]
id = "fmn_work"
type = "solo"
title = "Work"

[[formation.input]]
id = "port_work_in"
label = "Input"

[[formation.output]]
id = "port_work_out"
label = "Output"

[[formation.slot]]
id = "slot_work"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[gate]]
id = "gate_review"
title = "Review"
kinds = ["code", "human"]
criterion = "Good enough to ship"

[[formation]]
id = "fmn_ship"
type = "solo"
title = "Ship"

[[formation.input]]
id = "port_ship_in"
label = "Input"

[[formation.output]]
id = "port_ship_out"
label = "Output"

[[formation.slot]]
id = "slot_ship"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[connection]]
id = "edge_mission_work"
from = "mis_showcase:out"
to = "fmn_work:port_work_in"

[[connection]]
id = "edge_work_gate"
from = "fmn_work:port_work_out"
to = "gate_review:in"

[[connection]]
id = "edge_gate_pass_ship"
from = "gate_review:pass"
to = "fmn_ship:port_ship_in"
`
}
