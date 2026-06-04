package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestArchonAgentSpawnUsesFakeTmuxWithoutDuplicateSession(t *testing.T) {
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

func TestArchonAgentSpawnAndAttachUseExplicitHarnessStem(t *testing.T) {
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

	stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "gate", "create", "session-search", "--kinds", "code,human", "--criterion", "research is sound and safe to build", "--json")
	if code != 0 {
		t.Fatalf("gate create code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	raw := readArchonFile(t, store.BoardPath("session-search"))
	if !strings.Contains(raw, `[[gate]]`) || !strings.Contains(raw, `kinds = ["code", "human"]`) || strings.Contains(raw, "verdict") || strings.Contains(raw, "onFail") {
		t.Fatalf("gate create persisted wrong fields:\n%s", raw)
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

func TestArchonS3MissionCreateWireRejectsBDPrefix(t *testing.T) {
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

	if _, stderr, code := runArchon(t, runner, "--workspace", workspace, "mission", "create", "session-search", "--title", "Showcase", "--goal", "Build it", "--bead", "bd-204"); code == 0 || !strings.Contains(stderr, "home-") {
		t.Fatalf("mission create bd-prefix code=%d stderr=%s, want rejection", code, stderr)
	}
	stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "mission", "create", "session-search", "--title", "Showcase", "--goal", "Build it", "--bead", "home-7kc4.5", "--json")
	if code != 0 {
		t.Fatalf("mission create code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	raw := readArchonFile(t, store.BoardPath("session-search"))
	if !strings.Contains(raw, `[[mission]]`) || !strings.Contains(raw, `beadId = "home-7kc4.5"`) || strings.Contains(raw, "chain") {
		t.Fatalf("mission create persisted wrong fields:\n%s", raw)
	}
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read mission board: %v", err)
	}
	if len(board.Missions) != 1 {
		t.Fatalf("missions = %+v, want one mission", board.Missions)
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

type archonTestRunExecutor struct{}

func (archonTestRunExecutor) ExecuteFormation(req formations.FormationExecution) (formations.FormationExecutionResult, error) {
	return formations.FormationExecutionResult{
		Status:    "done",
		ReportRef: "refs/" + req.NodeID + ".md",
		Text:      "archon test output " + req.NodeID,
	}, nil
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
