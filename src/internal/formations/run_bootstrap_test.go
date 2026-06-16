package formations

import (
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestBootstrapRunRejectsExtraDataOverwritingSharedField pins the fail-loud
// guard in the shared run-start writer: a caller's ExtraData must never silently
// shadow a shared run_started field (it would make the two bootstrap paths look
// identical while carrying different values for the same key). The writer must
// refuse with a conflict and write nothing.
func TestBootstrapRunRejectsExtraDataOverwritingSharedField(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4RunBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	mission, ok := findMission(board, "mis_showcase")
	if !ok {
		t.Fatalf("mission fixture missing")
	}
	bindings, err := resolveRunBindings(board, personas)
	if err != nil {
		t.Fatalf("resolve bindings: %v", err)
	}

	_, err = store.bootstrapRun(runBootstrap{
		Slug:      "session-search",
		Board:     board,
		BoardRaw:  []byte(board.TOML),
		Mission:   mission,
		Bindings:  bindings,
		Actor:     "agent:test",
		ExtraData: map[string]any{"objective": "shadowed"},
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("bootstrapRun extra-data overwrite error = %v, want ErrConflict", err)
	}

	matches, globErr := filepath.Glob(filepath.Join(store.Workspace, ".formations", "runs", "session-search", "*"))
	if globErr != nil {
		t.Fatalf("glob run artifacts: %v", globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("rejected bootstrap left run artifacts behind: %v", matches)
	}
}

// TestS4SingleFormationAndMissionRunsShareBootstrapShape pins the run-start
// contract: a single-formation run and a mission run must produce a run_started
// event with identical envelope and identical shared data fields. Both paths
// snapshot the board and write the seq-1 run_started event through one shared
// bootstrap; if startFormationRun ever diverges from store.StartRun (different
// data keys, different envelope, different snapshot semantics) this test fails.
// The only sanctioned difference is the single-formation marker (mode +
// formationId), which mission runs do not carry.
func TestS4SingleFormationAndMissionRunsShareBootstrapShape(t *testing.T) {
	missionStart, missionWorkspace := startMissionRunForBootstrap(t)
	formationStart, formationWorkspace := startSingleFormationRunForBootstrap(t)

	// Both runs use their own temp workspace, so normalize the absolute board
	// path to a workspace-relative one before comparing. The path itself is
	// produced by identical logic (store.BoardPath(slug)); the prefix differs
	// only because the test fixtures use separate temp dirs.
	missionStart.Data["boardPath"] = trimWorkspacePrefix(t, missionStart.Data["boardPath"], missionWorkspace)
	formationStart.Data["boardPath"] = trimWorkspacePrefix(t, formationStart.Data["boardPath"], formationWorkspace)

	if missionStart.Type != formationStart.Type {
		t.Fatalf("run_started type mismatch: mission %q, formation %q", missionStart.Type, formationStart.Type)
	}
	if missionStart.Type != RunEventStarted {
		t.Fatalf("first event type = %q, want %q", missionStart.Type, RunEventStarted)
	}
	if missionStart.Seq != 1 || formationStart.Seq != 1 {
		t.Fatalf("run_started seq: mission %d, formation %d, want both 1", missionStart.Seq, formationStart.Seq)
	}
	if missionStart.Epoch != formationStart.Epoch || missionStart.Attempt != formationStart.Attempt {
		t.Fatalf("run_started epoch/attempt mismatch: mission epoch=%d attempt=%d, formation epoch=%d attempt=%d",
			missionStart.Epoch, missionStart.Attempt, formationStart.Epoch, formationStart.Attempt)
	}
	if missionStart.BoardID == "" || missionStart.BoardID != formationStart.BoardID {
		t.Fatalf("run_started boardId mismatch: mission %q, formation %q", missionStart.BoardID, formationStart.BoardID)
	}
	if missionStart.BoardRev == 0 || missionStart.BoardRev != formationStart.BoardRev {
		t.Fatalf("run_started boardRev mismatch: mission %d, formation %d", missionStart.BoardRev, formationStart.BoardRev)
	}

	// The shared data fields must match exactly in key set so the two paths stay
	// structurally identical. Path-specific run IDs and the formation marker are
	// excluded; everything else is the shared bootstrap contract.
	missionKeys := dataKeys(missionStart.Data)
	formationKeys := dataKeys(formationStart.Data, "mode", "formationId")
	if !reflect.DeepEqual(missionKeys, formationKeys) {
		t.Fatalf("run_started data keys diverge:\n mission   = %v\n formation = %v", missionKeys, formationKeys)
	}

	for _, key := range []string{"boardSlug", "boardPath", "boardRev", "limits"} {
		if !reflect.DeepEqual(missionStart.Data[key], formationStart.Data[key]) {
			t.Fatalf("run_started shared data[%q] diverges: mission %#v, formation %#v", key, missionStart.Data[key], formationStart.Data[key])
		}
	}

	// The single-formation marker must be present only on the formation path.
	if _, ok := missionStart.Data["mode"]; ok {
		t.Fatalf("mission run_started carries a single-formation mode marker: %#v", missionStart.Data["mode"])
	}
	if formationStart.Data["mode"] != "formation" {
		t.Fatalf("single-formation run_started mode = %#v, want %q", formationStart.Data["mode"], "formation")
	}
	if formationStart.Data["formationId"] != "fmn_research" {
		t.Fatalf("single-formation run_started formationId = %#v, want %q", formationStart.Data["formationId"], "fmn_research")
	}
}

func startMissionRunForBootstrap(t *testing.T) (RunEvent, string) {
	t.Helper()
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4RunBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)
	if _, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 5, WallClockSeconds: 60},
	}); err != nil {
		t.Fatalf("run mission: %v", err)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	if len(events) == 0 {
		t.Fatalf("mission run produced no events")
	}
	return events[0], store.Workspace
}

func startSingleFormationRunForBootstrap(t *testing.T) (RunEvent, string) {
	t.Helper()
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4RunBoardFixture())
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)
	if _, err := engine.RunFormation("session-search", "fmn_research", FormationRunRequest{
		Actor:  "agent:test",
		Limits: RunLimits{MaxDispatch: 5, WallClockSeconds: 60},
	}); err != nil {
		t.Fatalf("run formation: %v", err)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	if len(events) == 0 {
		t.Fatalf("single-formation run produced no events")
	}
	return events[0], store.Workspace
}

func trimWorkspacePrefix(t *testing.T, value any, workspace string) string {
	t.Helper()
	path, ok := value.(string)
	if !ok {
		t.Fatalf("boardPath = %#v, want string", value)
	}
	rel := strings.TrimPrefix(path, filepath.ToSlash(workspace))
	return strings.TrimPrefix(rel, "/")
}

func dataKeys(data map[string]any, exclude ...string) []string {
	excluded := make(map[string]bool, len(exclude))
	for _, key := range exclude {
		excluded[key] = true
	}
	keys := make([]string, 0, len(data))
	for key := range data {
		if excluded[key] {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
