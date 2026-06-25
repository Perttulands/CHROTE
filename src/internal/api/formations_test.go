package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chrote/server/internal/formations"
)

func TestFormationsHandlerListsBoardsThroughSharedStore(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
`)
	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/formations/boards", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Boards []struct {
				ID    string `json:"id"`
				Slug  string `json:"slug"`
				Title string `json:"title"`
				Rev   int    `json:"rev"`
			} `json:"boards"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v\n%s", err, rec.Body.String())
	}
	if !response.Success {
		t.Fatalf("success = false: %s", rec.Body.String())
	}
	if len(response.Data.Boards) != 1 {
		t.Fatalf("boards len = %d, want 1", len(response.Data.Boards))
	}
	board := response.Data.Boards[0]
	if board.ID != "brd_01J9_sesssearch" || board.Slug != "session-search" || board.Title != "Improve session search" || board.Rev != 7 {
		t.Fatalf("board summary = %+v, want persisted board metadata", board)
	}
}

func TestFormationsHandlerBoardPatchReturnsConflictForStaleETag(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
`)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	if _, err := store.UpdateBoardMetadata("session-search", formations.BoardMetadataPatch{
		Title:     stringPtrForFormationsAPI("external edit"),
		UpdatedBy: "agent:external",
	}, formations.WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev}); err != nil {
		t.Fatalf("external update: %v", err)
	}

	handler := NewFormationsHandlerWithStore(store)
	req := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search", bytes.NewBufferString(`{"title":"stale edit","expectedRev":7,"updatedBy":"agent:test"}`))
	req.Header.Set("If-Match", board.ETag)
	req.SetPathValue("board", "session-search")
	rec := httptest.NewRecorder()

	handler.PatchBoard(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestFormationsHandlerRequiresPreconditionsForWrites(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
`)
	writeFormationsAPIFixture(t, store.LayoutPath("session-search"), `schema = 1
boardId = "brd_01J9_sesssearch"
boardRev = 7
updatedAt = "2026-06-03T16:02:00Z"
`)
	handler := NewFormationsHandlerWithStore(store)

	boardReq := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search", bytes.NewBufferString(`{"title":"blind edit"}`))
	boardReq.SetPathValue("board", "session-search")
	boardRec := httptest.NewRecorder()
	handler.PatchBoard(boardRec, boardReq)
	if boardRec.Code != http.StatusPreconditionRequired {
		t.Fatalf("board status = %d, want %d: %s", boardRec.Code, http.StatusPreconditionRequired, boardRec.Body.String())
	}

	layoutReq := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search/layout", bytes.NewBufferString(`{}`))
	layoutReq.SetPathValue("board", "session-search")
	layoutRec := httptest.NewRecorder()
	handler.PatchLayout(layoutRec, layoutReq)
	if layoutRec.Code != http.StatusPreconditionRequired {
		t.Fatalf("layout status = %d, want %d: %s", layoutRec.Code, http.StatusPreconditionRequired, layoutRec.Body.String())
	}
}

func TestFormationsHandlerSurfacesBoardChangedSignal(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
`)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "External edit"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
`)
	handler := NewFormationsHandlerWithStore(store)
	req := httptest.NewRequest(http.MethodGet, "/api/formations/boards/session-search/changes?etag="+board.ETag, nil)
	req.SetPathValue("board", "session-search")
	rec := httptest.NewRecorder()

	handler.GetBoardChanges(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Signal struct {
				Changed bool   `json:"changed"`
				Signal  string `json:"signal"`
			} `json:"signal"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v\n%s", err, rec.Body.String())
	}
	if !response.Data.Signal.Changed || response.Data.Signal.Signal != "board.changed" {
		t.Fatalf("signal = %+v, want board.changed changed signal", response.Data.Signal)
	}
}

func TestFormationsHandlerS2CreatesFormationThroughSharedStore(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
customFuture = "keep me"
`)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search", bytes.NewBufferString(`{"createFormation":{"type":"peer","title":"Research huddle","x":840,"y":135},"expectedRev":7,"updatedBy":"agent:test"}`))
	req.Header.Set("If-Match", board.ETag)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Board struct {
				Rev        int                        `json:"rev"`
				Formations []formations.FormationNode `json:"formations"`
			} `json:"board"`
			Formation formations.FormationNode `json:"formation"`
			Layout    struct {
				BoardRev int                     `json:"boardRev"`
				Nodes    []formations.LayoutNode `json:"nodes"`
			} `json:"layout"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v\n%s", err, rec.Body.String())
	}
	if !response.Success {
		t.Fatalf("success=false: %s", rec.Body.String())
	}
	if response.Data.Board.Rev != 8 {
		t.Fatalf("board rev = %d, want 8", response.Data.Board.Rev)
	}
	if response.Data.Formation.Type != formations.FormationTypePeer || len(response.Data.Formation.Slots) != 2 {
		t.Fatalf("formation = %+v, want peer with two default slots", response.Data.Formation)
	}
	if len(response.Data.Layout.Nodes) != 1 || response.Data.Layout.Nodes[0].X != 840 || response.Data.Layout.Nodes[0].Y != 135 {
		t.Fatalf("layout nodes = %+v, want created x/y", response.Data.Layout.Nodes)
	}
	raw := readFormationsAPIFile(t, store.BoardPath("session-search"))
	if !bytes.Contains([]byte(raw), []byte(`customFuture = "keep me"`)) || bytes.Contains([]byte(raw), []byte("x = 840")) {
		t.Fatalf("board file did not preserve unknown fields or leaked layout:\n%s", raw)
	}

	postReq := httptest.NewRequest(http.MethodPost, "/api/formations/boards/session-search/formations", bytes.NewBufferString(`{"type":"peer"}`))
	postRec := httptest.NewRecorder()
	mux.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusNotFound {
		t.Fatalf("legacy POST create route status = %d, want %d; body=%s", postRec.Code, http.StatusNotFound, postRec.Body.String())
	}
}

func TestFormationsHandlerS5ResumeVerdictAndEscalations(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	personas := formations.NewPersonaStore(t.TempDir())
	personas.Now = fixedFormationsAPIClock()
	if _, err := personas.CreatePersona(formations.CreatePersonaRequest{ID: "scout", Kind: "specialist", Harness: "openai-codex"}); err != nil {
		t.Fatalf("create persona: %v", err)
	}
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), formationsAPIS5CascadeBoardFixture())
	handler := NewFormationsHandlerWithStores(store, personas)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	startReq := httptest.NewRequest(http.MethodPost, "/api/formations/runs", bytes.NewBufferString(`{"board":"session-search","missionId":"mis_showcase","limits":{"maxDispatch":1}}`))
	startRec := httptest.NewRecorder()
	mux.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want %d: %s", startRec.Code, http.StatusOK, startRec.Body.String())
	}
	started := decodeFormationsRunStartResponse(t, startRec.Body.Bytes())
	if started.RunID == "" || started.Status.Status != formations.RunStatusBlocked {
		t.Fatalf("started = %+v, want blocked run", started)
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/api/formations/runs/"+started.RunID+"/resume", bytes.NewBufferString(`{"reason":"continue","actor":"agent:test"}`))
	resumeRec := httptest.NewRecorder()
	mux.ServeHTTP(resumeRec, resumeReq)
	if resumeRec.Code != http.StatusOK {
		t.Fatalf("resume status = %d, want %d: %s", resumeRec.Code, http.StatusOK, resumeRec.Body.String())
	}
	resumed := decodeFormationsStatusResponse(t, resumeRec.Body.Bytes())
	if resumed.Status != formations.RunStatusBlocked || resumed.Epoch != 1 {
		t.Fatalf("resumed = %+v, want blocked epoch 1 without fake executor", resumed)
	}

	writeFormationsAPIFixture(t, store.BoardPath("human-search"), formationsAPIS5HumanGateBoardFixture())
	humanBoard, err := store.ReadBoard("human-search")
	if err != nil {
		t.Fatalf("read human board: %v", err)
	}
	engine := formations.NewRunEngine(store, personas, apiTestRunExecutor{})
	engine.SetGateEvaluator(apiTestGateEvaluator{verdict: "pass"})
	humanStarted, err := engine.RunMission("human-search", formations.RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: humanBoard.ETag,
		ExpectedBoardRev:  humanBoard.Rev,
		Personas:          personas,
		Limits:            formations.RunLimits{MaxDispatch: 5, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("start human waiting run: %v", err)
	}
	if humanStarted.Status != formations.RunStatusRunning {
		t.Fatalf("human started = %+v, want running human wait", humanStarted)
	}
	if _, err := store.RecordEscalationFromCapture(humanStarted.RunID, "fmn_work", "<<<CHROTE-ESCALATE run-id="+humanStarted.RunID+" severity=needs-attention reason='found a better direction'>>>"); err != nil {
		t.Fatalf("record escalation: %v", err)
	}
	verdictReq := httptest.NewRequest(http.MethodPost, "/api/formations/runs/"+humanStarted.RunID+"/gates/gate_review/verdict", bytes.NewBufferString(`{"verdict":"pass","reason":"direction is right","actor":"human:perttu"}`))
	verdictRec := httptest.NewRecorder()
	mux.ServeHTTP(verdictRec, verdictReq)
	if verdictRec.Code != http.StatusOK {
		t.Fatalf("verdict status = %d, want %d: %s", verdictRec.Code, http.StatusOK, verdictRec.Body.String())
	}
	approved := decodeFormationsStatusResponse(t, verdictRec.Body.Bytes())
	if approved.Status != formations.RunStatusBlocked {
		t.Fatalf("approved = %+v, want blocked when public verdict path lacks executor", approved)
	}

	escalationsReq := httptest.NewRequest(http.MethodGet, "/api/formations/runs/"+humanStarted.RunID+"/escalations", nil)
	escalationsRec := httptest.NewRecorder()
	mux.ServeHTTP(escalationsRec, escalationsReq)
	if escalationsRec.Code != http.StatusOK {
		t.Fatalf("escalations status = %d, want %d: %s", escalationsRec.Code, http.StatusOK, escalationsRec.Body.String())
	}
	var escalationsResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Escalations []formations.OpenEscalation `json:"escalations"`
		} `json:"data"`
	}
	if err := json.Unmarshal(escalationsRec.Body.Bytes(), &escalationsResponse); err != nil {
		t.Fatalf("decode escalations: %v\n%s", err, escalationsRec.Body.String())
	}
	if len(escalationsResponse.Data.Escalations) != 1 || escalationsResponse.Data.Escalations[0].Reason != "found a better direction" {
		t.Fatalf("escalations = %+v, want ledger-derived reason", escalationsResponse.Data.Escalations)
	}
}

func TestFormationsHandlerStartsSingleFormationByID(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	personas := formations.NewPersonaStore(t.TempDir())
	personas.Now = fixedFormationsAPIClock()
	if _, err := personas.CreatePersona(formations.CreatePersonaRequest{ID: "scout", Kind: "specialist", Harness: "openai-codex"}); err != nil {
		t.Fatalf("create persona: %v", err)
	}
	t.Setenv("CHROTE_FORMATIONS_LAB_HARNESSES", "openai-codex")
	t.Setenv("CHROTE_FORMATIONS_LAB_CWD", store.Workspace)
	t.Setenv("CHROTE_FORMATIONS_LAB_ROOTS", store.Workspace)
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), formationsAPIS5CascadeBoardFixture())
	handler := NewFormationsHandlerWithStores(store, personas)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	startReq := httptest.NewRequest(http.MethodPost, "/api/formations/runs", bytes.NewBufferString(`{"board":"session-search","formationId":"fmn_work","actor":"agent:test","limits":{"maxDispatch":1,"maxAttempts":1}}`))
	startRec := httptest.NewRecorder()
	mux.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want %d: %s", startRec.Code, http.StatusOK, startRec.Body.String())
	}
	started := decodeFormationsRunStartResponse(t, startRec.Body.Bytes())
	if started.RunID == "" || started.Status.Status != formations.RunStatusSucceeded {
		t.Fatalf("started = %+v, want succeeded single-formation run", started)
	}
	events, err := store.ReadRunEvents(started.RunID)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if !apiEventsContain(events, formations.RunEventStarted, formations.RunEventNodeStarted, formations.RunEventNodeOutput, formations.RunEventSucceeded) {
		t.Fatalf("events = %v, want single formation output path", apiEventTypes(events))
	}
}

func TestFormationsHandlerStartRunRequiresExactlyOneTargetAndHonorsFormationPreconditions(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	personas := formations.NewPersonaStore(t.TempDir())
	personas.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), formationsAPIS5CascadeBoardFixture())
	handler := NewFormationsHandlerWithStores(store, personas)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	cases := []struct {
		name string
		body string
	}{
		{name: "missing target", body: `{"board":"session-search"}`},
		{name: "ambiguous target", body: `{"board":"session-search","missionId":"mis_showcase","formationId":"fmn_work"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/formations/runs", bytes.NewBufferString(tc.body))
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}

	staleReq := httptest.NewRequest(http.MethodPost, "/api/formations/runs", bytes.NewBufferString(`{"board":"session-search","formationId":"fmn_work","expectedRev":7}`))
	staleReq.Header.Set("If-Match", "stale-etag")
	staleRec := httptest.NewRecorder()
	mux.ServeHTTP(staleRec, staleReq)
	if staleRec.Code != http.StatusConflict {
		t.Fatalf("stale status = %d, want %d: %s", staleRec.Code, http.StatusConflict, staleRec.Body.String())
	}
}

func TestFormationsHandlerS3DeletesGateAndMissionThroughBoardPatch(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[mission]]
id = "mis_showcase"
title = "Showcase"
goal = "Build"
beadId = "home-7kc4.5"

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

[[gate]]
id = "gate_review"
title = "Review"
kinds = ["code"]
criterion = "Review"

[[connection]]
id = "edge_mission_frame"
from = "mis_showcase:out"
to = "fmn_frame:port_frame_in"

[[connection]]
id = "edge_frame_gate"
from = "fmn_frame:port_frame_out"
to = "gate_review:in"
`)
	writeFormationsAPIFixture(t, store.LayoutPath("session-search"), `schema = 1
boardId = "brd_01J9_sesssearch"
boardRev = 7
updatedAt = "2026-06-03T16:02:00Z"

[[node]]
id = "mis_showcase"
x = 80
y = -120

[[node]]
id = "gate_review"
x = 440
y = 80
`)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	handler := NewFormationsHandlerWithStore(store)

	gateReq := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search", bytes.NewBufferString(`{"deleteGate":{"id":"gate_review"},"expectedRev":7,"updatedBy":"agent:test"}`))
	gateReq.Header.Set("If-Match", board.ETag)
	gateReq.SetPathValue("board", "session-search")
	gateRec := httptest.NewRecorder()
	handler.PatchBoard(gateRec, gateReq)
	if gateRec.Code != http.StatusOK {
		t.Fatalf("delete gate status = %d, want %d: %s", gateRec.Code, http.StatusOK, gateRec.Body.String())
	}
	var gateResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Board  formations.BoardDocument  `json:"board"`
			Layout formations.LayoutDocument `json:"layout"`
		} `json:"data"`
	}
	if err := json.Unmarshal(gateRec.Body.Bytes(), &gateResponse); err != nil {
		t.Fatalf("decode gate response: %v\n%s", err, gateRec.Body.String())
	}
	if len(gateResponse.Data.Board.Gates) != 0 || len(gateResponse.Data.Board.Connections) != 1 {
		t.Fatalf("delete gate board = %+v, want gate and touched wire removed", gateResponse.Data.Board)
	}

	missionReq := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search", bytes.NewBufferString(`{"deleteMission":{"id":"mis_showcase"},"expectedRev":8,"updatedBy":"agent:test"}`))
	missionReq.Header.Set("If-Match", gateRec.Header().Get("ETag"))
	missionReq.SetPathValue("board", "session-search")
	missionRec := httptest.NewRecorder()
	handler.PatchBoard(missionRec, missionReq)
	if missionRec.Code != http.StatusOK {
		t.Fatalf("delete mission status = %d, want %d: %s", missionRec.Code, http.StatusOK, missionRec.Body.String())
	}
	var missionResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Board  formations.BoardDocument  `json:"board"`
			Layout formations.LayoutDocument `json:"layout"`
		} `json:"data"`
	}
	if err := json.Unmarshal(missionRec.Body.Bytes(), &missionResponse); err != nil {
		t.Fatalf("decode mission response: %v\n%s", err, missionRec.Body.String())
	}
	if len(missionResponse.Data.Board.Missions) != 0 || len(missionResponse.Data.Board.Connections) != 0 {
		t.Fatalf("delete mission board = %+v, want mission and touched wire removed", missionResponse.Data.Board)
	}
	if len(missionResponse.Data.Layout.Nodes) != 0 {
		t.Fatalf("layout nodes = %+v, want deleted gate/mission layout pruned", missionResponse.Data.Layout.Nodes)
	}
}

func TestFormationsHandlerS2ReadsBoardByIDAndMovesNodeInLayoutOnly(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
`)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	created, err := store.CreateFormation("session-search", formations.FormationCreateRequest{
		Type:      formations.FormationTypeSolo,
		Title:     "Frame the goal",
		X:         120,
		Y:         80,
		UpdatedBy: "agent:test",
	}, formations.WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("create fixture formation: %v", err)
	}
	boardBytesBeforeMove := readFormationsAPIFile(t, store.BoardPath("session-search"))

	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/api/formations/boards/"+created.Board.ID, nil))
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET by board id status = %d, want %d: %s", getRec.Code, http.StatusOK, getRec.Body.String())
	}
	if !bytes.Contains(getRec.Body.Bytes(), []byte(`"slug":"session-search"`)) || !bytes.Contains(getRec.Body.Bytes(), []byte(`"title":"Frame the goal"`)) {
		t.Fatalf("GET by id response missing board/formation fields: %s", getRec.Body.String())
	}

	layout, err := store.ReadLayout("session-search")
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/"+created.Board.ID+"/layout", bytes.NewBufferString(`{"nodes":[{"id":"`+created.Formation.ID+`","x":910,"y":220}]}`))
	patchReq.Header.Set("If-Match", layout.ETag)
	patchRec := httptest.NewRecorder()
	mux.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("PATCH layout by board id status = %d, want %d: %s", patchRec.Code, http.StatusOK, patchRec.Body.String())
	}
	if got := readFormationsAPIFile(t, store.BoardPath("session-search")); got != boardBytesBeforeMove {
		t.Fatalf("layout move changed board definition:\n%s", got)
	}
	moved, err := store.ReadLayout("session-search")
	if err != nil {
		t.Fatalf("read moved layout: %v", err)
	}
	if len(moved.Nodes) != 1 || moved.Nodes[0].X != 910 || moved.Nodes[0].Y != 220 {
		t.Fatalf("layout nodes after move = %+v, want moved node", moved.Nodes)
	}
}

func TestFormationsHandlerS2RecreatesDeletedLayoutSidecarOnLayoutMove(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_01J9_research"
type = "peer"
title = "Research huddle"
`)
	boardBefore := readFormationsAPIFile(t, store.BoardPath("session-search"))
	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search/layout", bytes.NewBufferString(`{"nodes":[{"id":"fmn_01J9_research","x":260,"y":180}]}`))
	req.Header.Set("If-Match", "*")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Layout struct {
				BoardRev int                     `json:"boardRev"`
				Nodes    []formations.LayoutNode `json:"nodes"`
			} `json:"layout"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v\n%s", err, rec.Body.String())
	}
	if len(response.Data.Layout.Nodes) != 1 || response.Data.Layout.Nodes[0].X != 260 || response.Data.Layout.Nodes[0].Y != 180 {
		t.Fatalf("layout nodes = %+v, want recreated moved position", response.Data.Layout.Nodes)
	}
	if response.Data.Layout.BoardRev != 7 {
		t.Fatalf("layout boardRev = %d, want 7", response.Data.Layout.BoardRev)
	}
	if got := readFormationsAPIFile(t, store.BoardPath("session-search")); got != boardBefore {
		t.Fatalf("layout move dirtied board definition:\n%s", got)
	}
}

func TestS3InverseMutationsRequireFreshETags(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
`)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	created, err := store.CreateFormation("session-search", formations.FormationCreateRequest{
		Type:      formations.FormationTypeSolo,
		Title:     "Undo me",
		X:         300,
		Y:         180,
		UpdatedBy: "agent:test",
	}, formations.WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("create formation: %v", err)
	}

	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	staleReq := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search", bytes.NewBufferString(`{"deleteFormation":{"id":"`+created.Formation.ID+`"},"expectedRev":7,"updatedBy":"agent:test"}`))
	staleReq.Header.Set("If-Match", before.ETag)
	staleRec := httptest.NewRecorder()
	mux.ServeHTTP(staleRec, staleReq)
	if staleRec.Code != http.StatusConflict {
		t.Fatalf("stale delete status = %d, want %d: %s", staleRec.Code, http.StatusConflict, staleRec.Body.String())
	}

	freshReq := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search", bytes.NewBufferString(`{"deleteFormation":{"id":"`+created.Formation.ID+`"},"expectedRev":8,"updatedBy":"agent:test"}`))
	freshReq.Header.Set("If-Match", created.Board.ETag)
	freshRec := httptest.NewRecorder()
	mux.ServeHTTP(freshRec, freshReq)
	if freshRec.Code != http.StatusOK {
		t.Fatalf("fresh delete status = %d, want %d: %s", freshRec.Code, http.StatusOK, freshRec.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Board struct {
				Rev        int                        `json:"rev"`
				Formations []formations.FormationNode `json:"formations"`
			} `json:"board"`
			Layout struct {
				Nodes []formations.LayoutNode `json:"nodes"`
			} `json:"layout"`
		} `json:"data"`
	}
	if err := json.Unmarshal(freshRec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v\n%s", err, freshRec.Body.String())
	}
	if response.Data.Board.Rev != 9 || len(response.Data.Board.Formations) != 0 || len(response.Data.Layout.Nodes) != 0 {
		t.Fatalf("delete response = %+v, want rev 9 with formation/layout node removed", response.Data)
	}
}

func TestS3SlotAssignmentUsesPersonaIDsWithoutSessionNames(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), `schema = 1
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
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search", bytes.NewBufferString(`{"assignSlot":{"formationId":"fmn_frame","slotId":"slot_peer_a","agentId":"conductor","harness":"openai-codex"},"expectedRev":7,"updatedBy":"agent:test"}`))
	req.Header.Set("If-Match", board.ETag)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("sessionName")) || bytes.Contains(rec.Body.Bytes(), []byte("sessionStem")) {
		t.Fatalf("assignment response leaked runtime session fields: %s", rec.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Board struct {
				Rev        int                        `json:"rev"`
				Formations []formations.FormationNode `json:"formations"`
			} `json:"board"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v\n%s", err, rec.Body.String())
	}
	slot := response.Data.Board.Formations[0].Slots[0]
	if response.Data.Board.Rev != 8 || slot.AgentID != "conductor" || slot.Harness != "openai-codex" {
		t.Fatalf("assignment response = %+v, want rev 8 conductor/openai-codex", response.Data.Board)
	}
}

func TestS3BriefAndVerificationPersistProjectBeadAndOnFail(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_ship"
type = "solo"
title = "Ship"
`)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	briefReq := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search", bytes.NewBufferString(`{"setBrief":{"formationId":"fmn_ship","goal":"Ship the change","beadId":"srv-abc.2","files":["src/SessionPanel.tsx"],"links":["https://example.com/spec"]},"expectedRev":7,"updatedBy":"agent:test"}`))
	briefReq.Header.Set("If-Match", board.ETag)
	briefRec := httptest.NewRecorder()
	mux.ServeHTTP(briefRec, briefReq)
	if briefRec.Code != http.StatusOK {
		t.Fatalf("brief status = %d, want %d: %s", briefRec.Code, http.StatusOK, briefRec.Body.String())
	}
	withBrief, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read brief board: %v", err)
	}

	verifyReq := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search", bytes.NewBufferString(`{"setVerification":{"formationId":"fmn_ship","kinds":["code"],"criterion":"Tests pass.","onFail":"block"},"expectedRev":8,"updatedBy":"agent:test"}`))
	verifyReq.Header.Set("If-Match", withBrief.ETag)
	verifyRec := httptest.NewRecorder()
	mux.ServeHTTP(verifyRec, verifyReq)
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("verification status = %d, want %d: %s", verifyRec.Code, http.StatusOK, verifyRec.Body.String())
	}
	if !bytes.Contains(verifyRec.Body.Bytes(), []byte(`"beadId":"srv-abc.2"`)) || !bytes.Contains(verifyRec.Body.Bytes(), []byte(`"onFail":"block"`)) {
		t.Fatalf("verification response missing brief/onFail: %s", verifyRec.Body.String())
	}
}

func TestS3WireRejectsSelfDuplicateAndSecondInput(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_frame"
type = "solo"
title = "Frame"

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
`)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search", bytes.NewBufferString(`{"wireConnection":{"from":"fmn_frame:port_frame_out","to":"fmn_ship:port_ship_in"},"expectedRev":7,"updatedBy":"agent:test"}`))
	req.Header.Set("If-Match", board.ETag)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("wire status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"from":"fmn_frame:port_frame_out"`)) || !bytes.Contains(rec.Body.Bytes(), []byte(`"to":"fmn_ship:port_ship_in"`)) {
		t.Fatalf("wire response missing stable endpoints: %s", rec.Body.String())
	}
}

func TestS3RewireRejectsOccupiedTargetWithoutDroppingOriginal(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_frame"
type = "solo"
title = "Frame"

[[formation.output]]
id = "port_frame_out"
label = "Output"

[[formation]]
id = "fmn_research"
type = "solo"
title = "Research"

[[formation.input]]
id = "port_research_in"
label = "Input"

[[formation.output]]
id = "port_research_out"
label = "Output"

[[formation]]
id = "fmn_ship"
type = "solo"
title = "Ship"

[[formation.input]]
id = "port_ship_in"
label = "Input"

[[connection]]
id = "edge_frame_research"
from = "fmn_frame:port_frame_out"
to = "fmn_research:port_research_in"

[[connection]]
id = "edge_research_ship"
from = "fmn_research:port_research_out"
to = "fmn_ship:port_ship_in"
`)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search", bytes.NewBufferString(`{"rewireConnection":{"from":"fmn_frame:port_frame_out","previousTo":"fmn_research:port_research_in","to":"fmn_ship:port_ship_in"},"expectedRev":7,"updatedBy":"agent:test"}`))
	req.Header.Set("If-Match", board.ETag)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("rewire occupied target status = %d, want %d: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	after, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read after failed rewire: %v", err)
	}
	if len(after.Connections) != 2 || !hasAPIConnection(after.Connections, "fmn_frame:port_frame_out", "fmn_research:port_research_in") {
		t.Fatalf("failed rewire changed connections: %+v", after.Connections)
	}
}

func TestS3GatePersistsKindsCriterionWithoutVerdictOrOnFail(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
`)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search", bytes.NewBufferString(`{"createGate":{"title":"Review gate","kinds":["code","human"],"criterion":"Research is sound.","commandArgv":["npm","run","lint"],"commandCwd":"dashboard"},"expectedRev":7,"updatedBy":"agent:test"}`))
	req.Header.Set("If-Match", board.ETag)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("gate create status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"kinds":["code","human"]`)) ||
		!bytes.Contains(rec.Body.Bytes(), []byte(`"commandArgv":["npm","run","lint"]`)) ||
		!bytes.Contains(rec.Body.Bytes(), []byte(`"commandCwd":"dashboard"`)) ||
		bytes.Contains(rec.Body.Bytes(), []byte("verdict")) || bytes.Contains(rec.Body.Bytes(), []byte("onFail")) {
		t.Fatalf("gate response wrong: %s", rec.Body.String())
	}
}

func TestS3JudgeLoopAndChainUseConnectionsWithSingleSendAndReturn(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), `schema = 1
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
`)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search", bytes.NewBufferString(`{"setGateJudge":{"gateId":"gate_review","chain":["fmn_j1"]},"expectedRev":7,"updatedBy":"agent:test"}`))
	req.Header.Set("If-Match", board.ETag)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("judge status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"from":"gate_review:judge"`)) || !bytes.Contains(rec.Body.Bytes(), []byte(`"to":"gate_review:judge"`)) {
		t.Fatalf("judge response missing send/return: %s", rec.Body.String())
	}
}

func TestS3MissionCreateAcceptsProjectBeadIDAndSingleOut(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
`)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search", bytes.NewBufferString(`{"createMission":{"title":"Showcase","goal":"Build it","beadId":"chlab-123"},"expectedRev":7,"updatedBy":"agent:test"}`))
	req.Header.Set("If-Match", board.ETag)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mission status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"beadId":"chlab-123"`)) || bytes.Contains(rec.Body.Bytes(), []byte("chain")) {
		t.Fatalf("mission response wrong: %s", rec.Body.String())
	}
}

func TestFormationsHandlerS4RunLifecycleAndSSE(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	personas := formations.NewPersonaStore(t.TempDir())
	if _, err := personas.CreatePersona(formations.CreatePersonaRequest{
		ID:           "scout",
		Kind:         "specialist",
		Capabilities: []string{"research"},
		Harness:      "openai-codex",
	}); err != nil {
		t.Fatalf("create persona: %v", err)
	}
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), s4APIBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	handler := NewFormationsHandlerWithStores(store, personas)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	startReq := httptest.NewRequest(http.MethodPost, "/api/formations/runs", bytes.NewBufferString(`{"board":"session-search","missionId":"mis_showcase","actor":"agent:test","limits":{"maxDispatch":4,"maxAttempts":2}}`))
	startReq.Header.Set("If-Match", board.ETag)
	startRec := httptest.NewRecorder()
	mux.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("start run status = %d, want %d: %s", startRec.Code, http.StatusOK, startRec.Body.String())
	}
	var startResponse struct {
		Data struct {
			RunID  string                         `json:"runId"`
			Status formations.RunStatusProjection `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(startRec.Body.Bytes(), &startResponse); err != nil {
		t.Fatalf("decode start response: %v\n%s", err, startRec.Body.String())
	}
	if startResponse.Data.RunID == "" || startResponse.Data.Status.Status != formations.RunStatusBlocked {
		t.Fatalf("start response = %+v, want fail-loud blocked run id", startResponse.Data)
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/formations/runs/"+startResponse.Data.RunID, nil)
	statusReq.SetPathValue("runId", startResponse.Data.RunID)
	statusRec := httptest.NewRecorder()
	handler.GetRun(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("run status code = %d, want %d: %s", statusRec.Code, http.StatusOK, statusRec.Body.String())
	}
	if !bytes.Contains(statusRec.Body.Bytes(), []byte(`"status":"blocked"`)) {
		t.Fatalf("status response missing blocked projection: %s", statusRec.Body.String())
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/api/formations/runs/"+startResponse.Data.RunID+"/events", nil)
	eventsReq.SetPathValue("runId", startResponse.Data.RunID)
	eventsRec := httptest.NewRecorder()
	handler.GetRunEvents(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("events code = %d, want %d: %s", eventsRec.Code, http.StatusOK, eventsRec.Body.String())
	}
	if !bytes.Contains(eventsRec.Body.Bytes(), []byte(`"type":"run_started"`)) ||
		!bytes.Contains(eventsRec.Body.Bytes(), []byte(`"type":"error"`)) ||
		!bytes.Contains(eventsRec.Body.Bytes(), []byte(`"type":"run_blocked"`)) {
		t.Fatalf("events response missing fail-loud run ledger events: %s", eventsRec.Body.String())
	}
	if bytes.Contains(eventsRec.Body.Bytes(), []byte(`"type":"run_succeeded"`)) ||
		bytes.Contains(eventsRec.Body.Bytes(), []byte(`output from `)) {
		t.Fatalf("events response includes fake success: %s", eventsRec.Body.String())
	}

	streamReq := httptest.NewRequest(http.MethodGet, "/api/formations/runs/"+startResponse.Data.RunID+"/stream?since=0", nil)
	streamReq.SetPathValue("runId", startResponse.Data.RunID)
	streamRec := httptest.NewRecorder()
	handler.StreamRunEvents(streamRec, streamReq)
	if streamRec.Code != http.StatusOK {
		t.Fatalf("stream code = %d, want %d: %s", streamRec.Code, http.StatusOK, streamRec.Body.String())
	}
	if got := streamRec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("stream content-type = %q, want text/event-stream", got)
	}
	if !bytes.Contains(streamRec.Body.Bytes(), []byte("event: run_started")) || !bytes.Contains(streamRec.Body.Bytes(), []byte("id: 1")) {
		t.Fatalf("stream response missing replayed SSE events:\n%s", streamRec.Body.String())
	}

	lastEventReq := httptest.NewRequest(http.MethodGet, "/api/formations/runs/"+startResponse.Data.RunID+"/stream", nil)
	lastEventReq.SetPathValue("runId", startResponse.Data.RunID)
	lastEventReq.Header.Set("Last-Event-ID", "3")
	lastEventRec := httptest.NewRecorder()
	handler.StreamRunEvents(lastEventRec, lastEventReq)
	if lastEventRec.Code != http.StatusOK {
		t.Fatalf("Last-Event-ID stream code = %d, want %d: %s", lastEventRec.Code, http.StatusOK, lastEventRec.Body.String())
	}
	if bytes.Contains(lastEventRec.Body.Bytes(), []byte("id: 1")) || !bytes.Contains(lastEventRec.Body.Bytes(), []byte("id: 4")) {
		t.Fatalf("Last-Event-ID replay body = %s, want events after seq 3 only", lastEventRec.Body.String())
	}

	invalidReplayReq := httptest.NewRequest(http.MethodGet, "/api/formations/runs/"+startResponse.Data.RunID+"/stream", nil)
	invalidReplayReq.SetPathValue("runId", startResponse.Data.RunID)
	invalidReplayReq.Header.Set("Last-Event-ID", "not-a-seq")
	invalidReplayRec := httptest.NewRecorder()
	handler.StreamRunEvents(invalidReplayRec, invalidReplayReq)
	if invalidReplayRec.Code != http.StatusBadRequest {
		t.Fatalf("invalid Last-Event-ID stream code = %d, want %d: %s", invalidReplayRec.Code, http.StatusBadRequest, invalidReplayRec.Body.String())
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
	abortReq := httptest.NewRequest(http.MethodPost, "/api/formations/runs/"+openRun.RunID+"/abort", bytes.NewBufferString(`{"reason":"operator stop","requestedBy":"agent:test"}`))
	abortReq.SetPathValue("runId", openRun.RunID)
	abortRec := httptest.NewRecorder()
	handler.AbortRun(abortRec, abortReq)
	if abortRec.Code != http.StatusOK {
		t.Fatalf("abort code = %d, want %d: %s", abortRec.Code, http.StatusOK, abortRec.Body.String())
	}
	if !bytes.Contains(abortRec.Body.Bytes(), []byte(`"status":"canceled"`)) {
		t.Fatalf("abort response missing canceled status: %s", abortRec.Body.String())
	}
}

func TestFormationsHandlerS4ConfiguredLabExecutorRunsStaffedFormation(t *testing.T) {
	workspace := t.TempDir()
	agentsDir := t.TempDir()
	t.Setenv("CHROTE_FORMATIONS_LAB_HARNESSES", "lab-fake")
	t.Setenv("CHROTE_FORMATIONS_LAB_CWD", workspace)
	t.Setenv("CHROTE_FORMATIONS_LAB_ROOTS", workspace)

	store := formations.NewStore(workspace)
	personas := formations.NewPersonaStore(agentsDir)
	if _, err := personas.CreatePersona(formations.CreatePersonaRequest{
		ID:      "lab-poet",
		Kind:    "specialist",
		Harness: "lab-fake",
	}); err != nil {
		t.Fatalf("create persona: %v", err)
	}
	writeFormationsAPIFixture(t, store.BoardPath("poems"), formationsAPILabPoemBoardFixture())
	board, err := store.ReadBoard("poems")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	handler := NewFormationsHandlerWithStores(store, personas)
	startReq := httptest.NewRequest(http.MethodPost, "/api/formations/runs", bytes.NewBufferString(`{"board":"poems","missionId":"mis_poem","actor":"agent:test","limits":{"maxDispatch":3,"maxAttempts":1}}`))
	startReq.Header.Set("If-Match", board.ETag)
	startRec := httptest.NewRecorder()
	handler.StartRun(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("start run status = %d, want %d: %s", startRec.Code, http.StatusOK, startRec.Body.String())
	}
	started := decodeFormationsRunStartResponse(t, startRec.Body.Bytes())
	if started.Status.Status != formations.RunStatusSucceeded || !started.Status.Final {
		t.Fatalf("start response = %+v, want configured lab success", started)
	}
	events, err := store.ReadRunEvents(started.RunID)
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	if !apiLabEventsContain(events, formations.RunEventSlotDispatch, "fmn_draft") ||
		!apiLabEventsContain(events, formations.RunEventSlotResult, "fmn_draft") ||
		!apiLabEventsContain(events, formations.RunEventNodeOutput, "fmn_draft") {
		t.Fatalf("events = %+v, want lab slot dispatch/result/node output", events)
	}
	if apiLabEventsContainErrorCode(events, "missing_executor") {
		t.Fatalf("configured lab API run still reported missing_executor: %+v", events)
	}
}

func TestFormationsHandlerRejectsOversizedJSONBody(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	handler := NewFormationsHandlerWithStore(store)
	body := `{"board":"session-search","missionId":"mis_showcase","actor":"` + strings.Repeat("x", 2*1024*1024) + `"}`

	req := httptest.NewRequest(http.MethodPost, "/api/formations/runs", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	handler.StartRun(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413: %s", rec.Code, rec.Body.String())
	}
}

func writeFormationsAPIFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func s4APIBoardFixture() string {
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

type formationsRunStartAPIResponse struct {
	RunID  string                         `json:"runId"`
	Status formations.RunStatusProjection `json:"status"`
}

func decodeFormationsRunStartResponse(t *testing.T, raw []byte) formationsRunStartAPIResponse {
	t.Helper()
	var response struct {
		Success bool                          `json:"success"`
		Data    formationsRunStartAPIResponse `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode run start response: %v\n%s", err, string(raw))
	}
	return response.Data
}

func decodeFormationsStatusResponse(t *testing.T, raw []byte) formations.RunStatusProjection {
	t.Helper()
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Status formations.RunStatusProjection `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatalf("decode status response: %v\n%s", err, string(raw))
	}
	return response.Data.Status
}

func apiLabEventsContain(events []formations.RunEvent, typ, nodeID string) bool {
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

func apiLabEventsContainErrorCode(events []formations.RunEvent, code string) bool {
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

func formationsAPILabPoemBoardFixture() string {
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

func formationsAPIS5CascadeBoardFixture() string {
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

func formationsAPIS5HumanGateBoardFixture() string {
	return `schema = 1
id = "brd_01J9_human"
slug = "human-search"
title = "Human gate search"
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

func readFormationsAPIFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func stringPtrForFormationsAPI(v string) *string {
	return &v
}

func hasAPIConnection(connections []formations.BoardConnection, from, to string) bool {
	for _, connection := range connections {
		if connection.From == from && connection.To == to {
			return true
		}
	}
	return false
}

func fixedFormationsAPIClock() func() time.Time {
	return func() time.Time {
		return time.Date(2026, 6, 3, 17, 0, 0, 0, time.UTC)
	}
}
