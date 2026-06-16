package api

// F2 real-backend round-trip tests.
//
// WHY THIS FILE EXISTS: an audit found that the dashboard's e2e specs MOCK the
// API (page.route) and assert only the request SHAPE, never backend ACCEPTANCE.
// A mock that always returns 200 cannot catch a body the real backend rejects.
// That is exactly how two contract bugs shipped: the UI sent addPort
// direction:"in" while the backend only accepts "input"/"output", and
// mission-create assumed a required bead the backend treats as optional.
//
// Every test here drives the REAL FormationsHandler over the REAL Store writing
// REAL TOML in a temp workspace, sends the EXACT JSON body shape the UI
// op-builders produce (see dashboard/src/components/formationsApi.ts,
// formationsBoardModel.ts and FormationsCockpit.tsx), and proves acceptance by
// re-reading the persisted board. A regression to the old contract on the
// backend side must turn one of these green tests red.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chrote/server/internal/formations"
)

// roundTripBoardFixture is a minimal board with one solo formation (no ports)
// and one mission. It is the smallest board that exercises the previously-broken
// add/remove-port, mission, and wire write paths.
func roundTripBoardFixture() string {
	return `schema = 1
id = "brd_01J9_roundtrip"
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
`
}

// newRoundTripBoard writes the fixture, returns a wired mux plus the live store
// so each test can re-read the persisted board after every PATCH.
func newRoundTripBoard(t *testing.T) (*http.ServeMux, *formations.Store) {
	t.Helper()
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath("session-search"), roundTripBoardFixture())
	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return mux, store
}

// patchRoundTripBoard issues a UI-shaped board PATCH against the real handler
// using the current persisted ETag/Rev. It returns the recorder so callers can
// assert the status code precisely (200 vs 400/404/409).
func patchRoundTripBoard(t *testing.T, mux *http.ServeMux, store *formations.Store, op map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board for patch: %v", err)
	}
	// The UI always sends expectedRev + updatedBy:"agent:ui" alongside the op
	// (see patchBoardDocument in formationsApi.ts); mirror that envelope exactly.
	body := map[string]any{
		"expectedRev": board.Rev,
		"updatedBy":   "agent:ui",
	}
	for k, v := range op {
		body[k] = v
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal patch body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/session-search", bytes.NewReader(raw))
	req.Header.Set("If-Match", board.ETag)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// TestRoundTripAddPortUsesInputOutputDirection guards the exact bug that shipped:
// the UI op-builder must send direction:"input"/"output", NOT "in"/"out". The
// real backend (formation_authoring.go AddPort) rejects any other value with
// ErrInvalidSlug -> 400, so a regression to "in"/"out" on either side fails here
// instead of silently passing a mock that returns 200.
func TestRoundTripAddPortUsesInputOutputDirection(t *testing.T) {
	mux, store := newRoundTripBoard(t)

	// Exact UI body for "Add input port" (FormationsCockpit.addPortOp).
	inRec := patchRoundTripBoard(t, mux, store, map[string]any{
		"addPort": map[string]any{"formationId": "fmn_frame", "direction": "input", "label": "Input"},
	})
	if inRec.Code != http.StatusOK {
		t.Fatalf("addPort input status = %d, want 200 (real backend rejected the UI body): %s", inRec.Code, inRec.Body.String())
	}

	// Exact UI body for "Add output port".
	outRec := patchRoundTripBoard(t, mux, store, map[string]any{
		"addPort": map[string]any{"formationId": "fmn_frame", "direction": "output", "label": "Output"},
	})
	if outRec.Code != http.StatusOK {
		t.Fatalf("addPort output status = %d, want 200: %s", outRec.Code, outRec.Body.String())
	}

	// Re-read: both ports must persist on the formation.
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("re-read board: %v", err)
	}
	frame := findRoundTripFormation(t, board, "fmn_frame")
	if len(frame.Inputs) != 1 || frame.Inputs[0].Label != "Input" {
		t.Fatalf("inputs = %+v, want one persisted Input port from the UI add-port op", frame.Inputs)
	}
	if len(frame.Outputs) != 1 || frame.Outputs[0].Label != "Output" {
		t.Fatalf("outputs = %+v, want one persisted Output port from the UI add-port op", frame.Outputs)
	}

	// Regression assertion: the OLD buggy contract ("in"/"out") MUST be rejected.
	// If the backend ever loosens this, the contract drift surfaces loudly here.
	badRec := patchRoundTripBoard(t, mux, store, map[string]any{
		"addPort": map[string]any{"formationId": "fmn_frame", "direction": "in", "label": "Input"},
	})
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("addPort direction=\"in\" status = %d, want 400; the old buggy contract must stay rejected: %s", badRec.Code, badRec.Body.String())
	}
}

// TestRoundTripRemovePort proves the UI removePort op (formationId+portId, the
// inverse of addPort emitted by undoBoardPatch) round-trips: the port is gone on
// re-read and the formation survives.
func TestRoundTripRemovePort(t *testing.T) {
	mux, store := newRoundTripBoard(t)

	addRec := patchRoundTripBoard(t, mux, store, map[string]any{
		"addPort": map[string]any{"formationId": "fmn_frame", "direction": "input", "label": "Input"},
	})
	if addRec.Code != http.StatusOK {
		t.Fatalf("seed addPort status = %d, want 200: %s", addRec.Code, addRec.Body.String())
	}
	seeded, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read seeded board: %v", err)
	}
	portID := findRoundTripFormation(t, seeded, "fmn_frame").Inputs[0].ID

	// Exact UI body for removePort (FormationsCockpit.removePortOp / undo inverse).
	rmRec := patchRoundTripBoard(t, mux, store, map[string]any{
		"removePort": map[string]any{"formationId": "fmn_frame", "portId": portID},
	})
	if rmRec.Code != http.StatusOK {
		t.Fatalf("removePort status = %d, want 200: %s", rmRec.Code, rmRec.Body.String())
	}

	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("re-read board: %v", err)
	}
	if got := findRoundTripFormation(t, board, "fmn_frame"); len(got.Inputs) != 0 {
		t.Fatalf("inputs after removePort = %+v, want empty (port should round-trip out)", got.Inputs)
	}
}

// TestRoundTripCreateMissionWithoutBead guards the old required-bead bug: the UI
// createMission op sends NO beadId ({title, goal, x, y}). The real backend treats
// the bead as OPTIONAL (validateOptionalBeadID), so this must persist a bead-less
// mission. If the backend ever made bead required again, this test fails instead
// of a mock silently swallowing it.
//
// The createMission op only adds the FIRST mission to a board (the one-mission
// invariant rejects a second), so this exercises it against a fresh board that
// has no mission yet — exactly the state in which the UI emits createMissionAt.
func TestRoundTripCreateMissionWithoutBead(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath("blank"), `schema = 1
id = "brd_blank"
slug = "blank"
title = "Blank"
rev = 1
updatedAt = "2026-06-03T16:00:00Z"
`)
	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	board, err := store.ReadBoard("blank")
	if err != nil {
		t.Fatalf("read blank board: %v", err)
	}
	// Exact UI body for createMissionAt: NO beadId field at all.
	body, err := json.Marshal(map[string]any{
		"expectedRev":   board.Rev,
		"updatedBy":     "agent:ui",
		"createMission": map[string]any{"title": "New mission", "goal": "", "x": 120, "y": 80},
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/blank", bytes.NewReader(body))
	req.Header.Set("If-Match", board.ETag)
	req.SetPathValue("board", "blank")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("createMission (no bead) status = %d, want 200; the UI never sends a bead here: %s", rec.Code, rec.Body.String())
	}

	reread, err := store.ReadBoard("blank")
	if err != nil {
		t.Fatalf("re-read board: %v", err)
	}
	created := findRoundTripMission(t, reread, "New mission")
	if created.BeadID != "" {
		t.Fatalf("created mission beadId = %q, want empty (UI sent no bead)", created.BeadID)
	}
}

// TestRoundTripUpdateMission exercises the UI updateMission op shape
// ({missionId, title, goal, beadId}, built by updateMissionOp): happy path
// full-replace, empty beadId clears the link, malformed beadId -> 400, unknown
// mission -> 404. These are the precise status codes the UI relies on, mapped
// from ErrInvalidBeadID and ErrNotFound in the real handler.
func TestRoundTripUpdateMission(t *testing.T) {
	mux, store := newRoundTripBoard(t)

	// Happy path: full-replace title/goal/bead, id stays stable.
	okRec := patchRoundTripBoard(t, mux, store, map[string]any{
		"updateMission": map[string]any{"missionId": "mis_showcase", "title": "Showcase v2", "goal": "Ship it", "beadId": "home-vdki.34.1"},
	})
	if okRec.Code != http.StatusOK {
		t.Fatalf("updateMission status = %d, want 200: %s", okRec.Code, okRec.Body.String())
	}
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("re-read board: %v", err)
	}
	got := findRoundTripMission(t, board, "Showcase v2")
	if got.ID != "mis_showcase" || got.Goal != "Ship it" || got.BeadID != "home-vdki.34.1" {
		t.Fatalf("updated mission = %+v, want stable id with new goal/bead", got)
	}

	// Empty beadId clears the link (UI sends beadId:"" to unlink).
	clearRec := patchRoundTripBoard(t, mux, store, map[string]any{
		"updateMission": map[string]any{"missionId": "mis_showcase", "title": "Showcase v2", "goal": "Ship it", "beadId": ""},
	})
	if clearRec.Code != http.StatusOK {
		t.Fatalf("updateMission clear-bead status = %d, want 200: %s", clearRec.Code, clearRec.Body.String())
	}
	cleared, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("re-read cleared board: %v", err)
	}
	if got := findRoundTripMission(t, cleared, "Showcase v2"); got.BeadID != "" {
		t.Fatalf("beadId after empty update = %q, want cleared", got.BeadID)
	}

	// Malformed beadId -> 400 (ErrInvalidBeadID). The UI surfaces this to the user.
	badRec := patchRoundTripBoard(t, mux, store, map[string]any{
		"updateMission": map[string]any{"missionId": "mis_showcase", "title": "Showcase v2", "goal": "Ship it", "beadId": "Not A Bead"},
	})
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("updateMission malformed bead status = %d, want 400: %s", badRec.Code, badRec.Body.String())
	}

	// Unknown mission -> 404 (ErrNotFound).
	missingRec := patchRoundTripBoard(t, mux, store, map[string]any{
		"updateMission": map[string]any{"missionId": "mis_ghost", "title": "Nope", "goal": "Nope", "beadId": ""},
	})
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("updateMission unknown mission status = %d, want 404: %s", missingRec.Code, missingRec.Body.String())
	}
}

// TestRoundTripWireAndUnwire proves the UI wireConnection/unwireConnection op
// shapes ({from, to}) round-trip through the real backend: wiring two ports
// persists a connection, and unwiring the same endpoints removes it.
func TestRoundTripWireAndUnwire(t *testing.T) {
	mux, store := newRoundTripBoard(t)

	// Give fmn_frame an output port and add a second formation with an input port.
	if rec := patchRoundTripBoard(t, mux, store, map[string]any{
		"addPort": map[string]any{"formationId": "fmn_frame", "direction": "output", "label": "Output"},
	}); rec.Code != http.StatusOK {
		t.Fatalf("seed frame output port status = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := patchRoundTripBoard(t, mux, store, map[string]any{
		"createFormation": map[string]any{"type": "solo", "title": "Ship", "x": 480, "y": 80},
	}); rec.Code != http.StatusOK {
		t.Fatalf("seed ship formation status = %d: %s", rec.Code, rec.Body.String())
	}
	seeded, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read seeded board: %v", err)
	}
	ship := findRoundTripFormation(t, seeded, "")
	frameOut := findRoundTripFormation(t, seeded, "fmn_frame").Outputs[0].ID
	if rec := patchRoundTripBoard(t, mux, store, map[string]any{
		"addPort": map[string]any{"formationId": ship.ID, "direction": "input", "label": "Input"},
	}); rec.Code != http.StatusOK {
		t.Fatalf("seed ship input port status = %d: %s", rec.Code, rec.Body.String())
	}
	withPort, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board with ship port: %v", err)
	}
	shipIn := findRoundTripFormationByID(t, withPort, ship.ID).Inputs[0].ID

	from := "fmn_frame:" + frameOut
	to := ship.ID + ":" + shipIn

	// Exact UI wireConnection body.
	wireRec := patchRoundTripBoard(t, mux, store, map[string]any{
		"wireConnection": map[string]any{"from": from, "to": to},
	})
	if wireRec.Code != http.StatusOK {
		t.Fatalf("wireConnection status = %d, want 200: %s", wireRec.Code, wireRec.Body.String())
	}
	wired, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("re-read wired board: %v", err)
	}
	if !hasAPIConnection(wired.Connections, from, to) {
		t.Fatalf("connections = %+v, want wired %s -> %s", wired.Connections, from, to)
	}

	// Exact UI unwireConnection body (also the undo inverse of a wire).
	unwireRec := patchRoundTripBoard(t, mux, store, map[string]any{
		"unwireConnection": map[string]any{"from": from, "to": to},
	})
	if unwireRec.Code != http.StatusOK {
		t.Fatalf("unwireConnection status = %d, want 200: %s", unwireRec.Code, unwireRec.Body.String())
	}
	unwired, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("re-read unwired board: %v", err)
	}
	if hasAPIConnection(unwired.Connections, from, to) {
		t.Fatalf("connections = %+v, want %s -> %s removed", unwired.Connections, from, to)
	}
}

// TestRoundTripCreateGateUndoInverse proves the React-orchestrated undo path
// (ADR-0003: undo is an inverse PATCH, not server-side history). It applies a
// structural op (createGate), snapshots the board, then applies the inverse the
// UI undo emits (deleteGate via undoBoardPatch) and asserts the board returns to
// its prior modeled state. This locks that the inverse primitive actually
// round-trips through the real backend.
func TestRoundTripCreateGateUndoInverse(t *testing.T) {
	mux, store := newRoundTripBoard(t)

	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board before: %v", err)
	}

	gateRec := patchRoundTripBoard(t, mux, store, map[string]any{
		"createGate": map[string]any{"title": "Review gate", "kinds": []string{"code"}, "criterion": "", "x": 440, "y": 80},
	})
	if gateRec.Code != http.StatusOK {
		t.Fatalf("createGate status = %d, want 200: %s", gateRec.Code, gateRec.Body.String())
	}
	withGate, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board with gate: %v", err)
	}
	if len(withGate.Gates) != 1 {
		t.Fatalf("gates after createGate = %d, want 1", len(withGate.Gates))
	}
	gateID := withGate.Gates[0].ID

	// Inverse op the UI undo stack pushes (undoBoardPatch -> {deleteGate:{id}}).
	undoRec := patchRoundTripBoard(t, mux, store, map[string]any{
		"deleteGate": map[string]any{"id": gateID},
	})
	if undoRec.Code != http.StatusOK {
		t.Fatalf("deleteGate undo status = %d, want 200: %s", undoRec.Code, undoRec.Body.String())
	}

	after, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board after undo: %v", err)
	}
	if len(after.Gates) != 0 {
		t.Fatalf("gates after undo = %+v, want none (inverse must round-trip)", after.Gates)
	}
	// Modeled state (missions/formations/connections) must match the pre-op board.
	if len(after.Missions) != len(before.Missions) ||
		len(after.Formations) != len(before.Formations) ||
		len(after.Connections) != len(before.Connections) {
		t.Fatalf("board after undo = %+v, want same modeled shape as before createGate %+v", after, before)
	}
}

// TestRoundTripSetBriefUndoInverse proves the setBrief/clearBrief inverse pair
// round-trips: setBrief attaches a brief, and the UI undo inverse (clearBrief via
// undoBoardPatch when the prior brief was nil) returns the formation to having no
// brief.
func TestRoundTripSetBriefUndoInverse(t *testing.T) {
	mux, store := newRoundTripBoard(t)

	briefRec := patchRoundTripBoard(t, mux, store, map[string]any{
		"setBrief": map[string]any{
			"formationId": "fmn_frame",
			"goal":        "Ship the change",
			"beadId":      "srv-abc.2",
			"files":       []string{"src/SessionPanel.tsx"},
			"links":       []string{"https://example.com/spec"},
		},
	})
	if briefRec.Code != http.StatusOK {
		t.Fatalf("setBrief status = %d, want 200: %s", briefRec.Code, briefRec.Body.String())
	}
	withBrief, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board with brief: %v", err)
	}
	if findRoundTripFormation(t, withBrief, "fmn_frame").Brief == nil {
		t.Fatalf("brief = nil after setBrief, want attached brief")
	}

	// Inverse the UI undo emits when the prior brief was absent:
	// undoBoardPatch({setBrief, brief:undefined}) -> {clearBrief:{formationId}}.
	undoRec := patchRoundTripBoard(t, mux, store, map[string]any{
		"clearBrief": map[string]any{"formationId": "fmn_frame"},
	})
	if undoRec.Code != http.StatusOK {
		t.Fatalf("clearBrief undo status = %d, want 200: %s", undoRec.Code, undoRec.Body.String())
	}
	after, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board after clearBrief: %v", err)
	}
	if findRoundTripFormation(t, after, "fmn_frame").Brief != nil {
		t.Fatalf("brief after undo = %+v, want nil (inverse must round-trip)", findRoundTripFormation(t, after, "fmn_frame").Brief)
	}
}

// findRoundTripFormation returns the formation matching id, or (when id is "")
// the first formation that is not fmn_frame — used to grab the freshly created
// second formation in the wire test without knowing its generated id.
func findRoundTripFormation(t *testing.T, board *formations.BoardDocument, id string) formations.FormationNode {
	t.Helper()
	for _, f := range board.Formations {
		if id == "" {
			if f.ID != "fmn_frame" {
				return f
			}
			continue
		}
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("formation %q not found in %+v", id, board.Formations)
	return formations.FormationNode{}
}

func findRoundTripFormationByID(t *testing.T, board *formations.BoardDocument, id string) formations.FormationNode {
	t.Helper()
	for _, f := range board.Formations {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("formation id %q not found in %+v", id, board.Formations)
	return formations.FormationNode{}
}

func findRoundTripMission(t *testing.T, board *formations.BoardDocument, title string) formations.MissionNode {
	t.Helper()
	for _, m := range board.Missions {
		if m.Title == title {
			return m
		}
	}
	t.Fatalf("mission with title %q not found in %+v", title, board.Missions)
	return formations.MissionNode{}
}
