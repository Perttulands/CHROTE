package formations

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBoardWriterPreservesUnknownFieldsAndCommentsWithMinimalDiff(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `# human note stays put
schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search" # inline title note
rev = 7
updatedBy = "agent:archon"
updatedAt = "2026-06-03T16:00:00Z"
customFuture = "do not touch"

[[formation]]
id = "fmn_01J9_research"
reviewerNotes = "preserve me"
`)

	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	after, err := store.UpdateBoardMetadata("session-search", BoardMetadataPatch{
		Title:     stringPtr("Improve session search quickly"),
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("update board: %v", err)
	}

	if after.Rev != 8 {
		t.Fatalf("rev = %d, want 8", after.Rev)
	}
	got := readFile(t, store.BoardPath("session-search"))
	for _, want := range []string{
		"# human note stays put",
		`title = "Improve session search quickly" # inline title note`,
		`customFuture = "do not touch"`,
		`reviewerNotes = "preserve me"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("updated TOML lost %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "2026-06-03T16:00:00Z") {
		t.Fatalf("updatedAt was not refreshed:\n%s", got)
	}
	if !strings.Contains(got, `updatedAt = "2026-06-03T17:00:00Z"`) {
		t.Fatalf("updatedAt did not use writer clock:\n%s", got)
	}
	if after.ETag == before.ETag {
		t.Fatal("ETag did not change after structural write")
	}
}

func TestBoardStructuralWriteRequiresMatchingETagAndBumpsRevision(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))

	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	if _, err := store.UpdateBoardMetadata("session-search", BoardMetadataPatch{
		Title:     stringPtr("blind write"),
		UpdatedBy: "agent:test",
	}, WriteOptions{}); !errors.Is(err, ErrPreconditionRequired) {
		t.Fatalf("missing precondition error = %v, want ErrPreconditionRequired", err)
	}
	if _, err := store.UpdateBoardMetadata("session-search", BoardMetadataPatch{
		Title:     stringPtr("stale write"),
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: "stale", ExpectedRev: board.Rev}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale ETag error = %v, want ErrConflict", err)
	}

	after, err := store.UpdateBoardMetadata("session-search", BoardMetadataPatch{
		Title:     stringPtr("fresh write"),
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
	if err != nil {
		t.Fatalf("fresh update: %v", err)
	}
	if after.Rev != 8 {
		t.Fatalf("rev = %d, want 8", after.Rev)
	}

	if _, err := store.UpdateBoardMetadata("session-search", BoardMetadataPatch{
		Title:     stringPtr("old rev"),
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: after.ETag, ExpectedRev: board.Rev}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale rev error = %v, want ErrConflict", err)
	}
}

func TestLayoutWriteDoesNotChangeBoardBytesOrBoardRevision(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))
	writeFixture(t, store.LayoutPath("session-search"), `schema = 1
boardId = "brd_01J9_sesssearch"
boardRev = 7
updatedAt = "2026-06-03T16:02:00Z"

[[node]]
id = "fmn_01J9_research"
x = 420
y = 160
`)

	boardBefore := readFile(t, store.BoardPath("session-search"))
	layoutBefore, err := store.ReadLayout("session-search")
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}
	if _, err := store.UpdateLayoutMetadata("session-search", LayoutMetadataPatch{}, WriteOptions{}); !errors.Is(err, ErrPreconditionRequired) {
		t.Fatalf("missing layout precondition error = %v, want ErrPreconditionRequired", err)
	}

	layoutAfter, err := store.UpdateLayoutMetadata("session-search", LayoutMetadataPatch{
		UpdatedAt: time.Date(2026, 6, 3, 17, 5, 0, 0, time.UTC),
	}, WriteOptions{ExpectedETag: layoutBefore.ETag})
	if err != nil {
		t.Fatalf("update layout: %v", err)
	}

	if got := readFile(t, store.BoardPath("session-search")); got != boardBefore {
		t.Fatalf("layout write changed board definition:\n%s", got)
	}
	boardAfter, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board after layout write: %v", err)
	}
	if boardAfter.Rev != 7 {
		t.Fatalf("board rev after layout write = %d, want 7", boardAfter.Rev)
	}
	if layoutAfter.ETag == layoutBefore.ETag {
		t.Fatal("layout ETag did not change after layout write")
	}
}

func TestConcurrentBoardWritesReturnConflictNotClobber(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))

	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, title := range []string{"first", "second"} {
		title := title
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.UpdateBoardMetadata("session-search", BoardMetadataPatch{
				Title:     &title,
				UpdatedBy: "agent:test",
			}, WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	var successes, conflicts int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent write error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d, want one of each", successes, conflicts)
	}

	after, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board after concurrent writes: %v", err)
	}
	if after.Rev != 8 {
		t.Fatalf("rev after concurrent writes = %d, want exactly 8", after.Rev)
	}
}

func TestSchemaVersionRefusesNewerAndMigratesOlderPreservingContent(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("future"), `schema = 99
id = "brd_future"
slug = "future"
title = "Future"
rev = 1
`)
	if _, err := store.ReadBoard("future"); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("future schema error = %v, want ErrUnsupportedSchema", err)
	}

	writeFixture(t, store.BoardPath("old"), `schema = 0
id = "brd_old"
slug = "old"
title = "Old"
rev = 1
legacyKey = "keep"
`)
	old, err := store.ReadBoard("old")
	if err != nil {
		t.Fatalf("read old schema: %v", err)
	}
	if old.Schema != CurrentSchema {
		t.Fatalf("read old schema = %d, want migrated schema %d", old.Schema, CurrentSchema)
	}
	migrated := readFile(t, store.BoardPath("old"))
	for _, want := range []string{`schema = 1`, `legacyKey = "keep"`} {
		if !strings.Contains(migrated, want) {
			t.Fatalf("read-time migration lost %q:\n%s", want, migrated)
		}
	}
	after, err := store.UpdateBoardMetadata("old", BoardMetadataPatch{
		Title:     stringPtr("Old migrated"),
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: old.ETag, ExpectedRev: old.Rev})
	if err != nil {
		t.Fatalf("update old schema: %v", err)
	}
	if after.Schema != CurrentSchema {
		t.Fatalf("schema after migration = %d, want %d", after.Schema, CurrentSchema)
	}
	got := readFile(t, store.BoardPath("old"))
	for _, want := range []string{`schema = 1`, `legacyKey = "keep"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("old schema migration lost %q:\n%s", want, got)
		}
	}

	writeFixture(t, store.LayoutPath("old"), `schema = 0
boardId = "brd_old"
boardRev = 1
updatedAt = "2026-06-03T16:02:00Z"
layoutNote = "keep"
`)
	layout, err := store.ReadLayout("old")
	if err != nil {
		t.Fatalf("read old layout schema: %v", err)
	}
	if layout.Schema != CurrentSchema {
		t.Fatalf("layout schema after read = %d, want %d", layout.Schema, CurrentSchema)
	}
	if got := readFile(t, store.LayoutPath("old")); !strings.Contains(got, `layoutNote = "keep"`) || !strings.Contains(got, `schema = 1`) {
		t.Fatalf("layout read-time migration did not preserve content:\n%s", got)
	}
}

func TestBoardChangeSignalDetectsExternalEdit(t *testing.T) {
	store := NewStore(t.TempDir())
	writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))

	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	writeFixture(t, store.BoardPath("session-search"), strings.Replace(minimalBoard("session-search", 7), "Improve session search", "Externally edited", 1))

	signal, err := store.BoardChangeSince("session-search", board.ETag)
	if err != nil {
		t.Fatalf("detect board change: %v", err)
	}
	if !signal.Changed {
		t.Fatalf("Changed = false, want true")
	}
	if signal.Signal != "board.changed" {
		t.Fatalf("Signal = %q, want board.changed", signal.Signal)
	}
	if signal.ETag == board.ETag {
		t.Fatal("change signal ETag did not change")
	}
	if signal.ModifiedAt == "" {
		t.Fatal("change signal missing ModifiedAt")
	}
}

func TestCreateFormationPersistsDefaultSlotsAndLayoutOnlyCoordinates(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `# board comments stay
schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedBy = "agent:archon"
updatedAt = "2026-06-03T16:00:00Z"
customFuture = "keep me"
`)

	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	result, err := store.CreateFormation("session-search", FormationCreateRequest{
		Type:      FormationTypePeer,
		Title:     "Research huddle",
		X:         840,
		Y:         135,
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("create formation: %v", err)
	}

	if result.Board.Rev != 8 {
		t.Fatalf("board rev = %d, want 8", result.Board.Rev)
	}
	if result.Formation.ID == "" || !strings.HasPrefix(result.Formation.ID, "fmn_") {
		t.Fatalf("formation id = %q, want fmn_ prefix", result.Formation.ID)
	}
	if result.Formation.Type != FormationTypePeer || result.Formation.Title != "Research huddle" {
		t.Fatalf("formation = %+v, want peer Research huddle", result.Formation)
	}
	if len(result.Formation.Inputs) != 1 || !strings.HasPrefix(result.Formation.Inputs[0].ID, "port_") {
		t.Fatalf("inputs = %+v, want one stable port", result.Formation.Inputs)
	}
	if len(result.Formation.Outputs) != 1 || !strings.HasPrefix(result.Formation.Outputs[0].ID, "port_") {
		t.Fatalf("outputs = %+v, want one stable port", result.Formation.Outputs)
	}
	if len(result.Formation.Slots) != 2 {
		t.Fatalf("slots len = %d, want two peer slots", len(result.Formation.Slots))
	}
	for _, slot := range result.Formation.Slots {
		if !strings.HasPrefix(slot.ID, "slot_") {
			t.Fatalf("slot id = %q, want slot_ prefix", slot.ID)
		}
		if slot.AgentID != "" || slot.Harness != "" {
			t.Fatalf("S2 create stored staffing fields too early: %+v", slot)
		}
		if slot.Controller {
			t.Fatalf("peer slot unexpectedly controller: %+v", slot)
		}
	}

	boardTOML := readFile(t, store.BoardPath("session-search"))
	for _, want := range []string{
		"# board comments stay",
		`customFuture = "keep me"`,
		`[[formation]]`,
		`type = "peer"`,
		`title = "Research huddle"`,
		`label = "Peer"`,
	} {
		if !strings.Contains(boardTOML, want) {
			t.Fatalf("board TOML missing %q:\n%s", want, boardTOML)
		}
	}
	if strings.Contains(boardTOML, "x = 840") || strings.Contains(boardTOML, "y = 135") {
		t.Fatalf("board definition contains layout coordinates:\n%s", boardTOML)
	}

	layoutTOML := readFile(t, store.LayoutPath("session-search"))
	for _, want := range []string{
		`boardId = "brd_01J9_sesssearch"`,
		`boardRev = 8`,
		`[[node]]`,
		`x = 840`,
		`y = 135`,
	} {
		if !strings.Contains(layoutTOML, want) {
			t.Fatalf("layout TOML missing %q:\n%s", want, layoutTOML)
		}
	}
	if result.Layout.BoardRev != 8 {
		t.Fatalf("layout boardRev = %d, want 8", result.Layout.BoardRev)
	}
	if len(result.Layout.Nodes) != 1 || result.Layout.Nodes[0].ID != result.Formation.ID {
		t.Fatalf("layout nodes = %+v, want created formation node", result.Layout.Nodes)
	}

	reread, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("reread board: %v", err)
	}
	if len(reread.Formations) != 1 || reread.Formations[0].ID != result.Formation.ID {
		t.Fatalf("reread formations = %+v, want created formation", reread.Formations)
	}
}

func TestCreateFormationProvidesAllFourDefaultShapes(t *testing.T) {
	tests := []struct {
		formationType string
		wantSlots     int
		wantControl   int
	}{
		{FormationTypeSolo, 1, 0},
		{FormationTypePeer, 2, 0},
		{FormationTypeFlow, 3, 0},
		{FormationTypeOrchestrated, 3, 1},
	}

	for _, tt := range tests {
		t.Run(tt.formationType, func(t *testing.T) {
			store := NewStore(t.TempDir())
			store.Now = fixedClock()
			writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))
			before, err := store.ReadBoard("session-search")
			if err != nil {
				t.Fatalf("read board: %v", err)
			}

			result, err := store.CreateFormation("session-search", FormationCreateRequest{
				Type:      tt.formationType,
				Title:     "New " + tt.formationType,
				X:         100,
				Y:         200,
				UpdatedBy: "agent:test",
			}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
			if err != nil {
				t.Fatalf("create formation: %v", err)
			}
			if len(result.Formation.Inputs) != 1 || len(result.Formation.Outputs) != 1 {
				t.Fatalf("ports = in:%+v out:%+v, want one input and one output", result.Formation.Inputs, result.Formation.Outputs)
			}
			if len(result.Formation.Slots) != tt.wantSlots {
				t.Fatalf("slots len = %d, want %d", len(result.Formation.Slots), tt.wantSlots)
			}
			var controllers int
			for _, slot := range result.Formation.Slots {
				if slot.Controller {
					controllers++
				}
			}
			if controllers != tt.wantControl {
				t.Fatalf("controllers = %d, want %d in %+v", controllers, tt.wantControl, result.Formation.Slots)
			}
		})
	}
}

func TestBoardReadsPersistedConnectionsWithStableEndpoints(t *testing.T) {
	store := NewStore(t.TempDir())
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
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
type = "peer"
title = "Research"

[[formation.input]]
id = "port_research_in"
label = "Input"

[[connection]]
id = "edge_frame_research"
from = "fmn_frame:port_frame_out"
to = "fmn_research:port_research_in"
`)

	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	if len(board.Connections) != 1 {
		t.Fatalf("connections len = %d, want 1", len(board.Connections))
	}
	connection := board.Connections[0]
	if connection.ID != "edge_frame_research" || connection.From != "fmn_frame:port_frame_out" || connection.To != "fmn_research:port_research_in" {
		t.Fatalf("connection = %+v, want stable edge endpoints from board TOML", connection)
	}
}

func TestUpdateLayoutNodesRecreatesDeletedLayoutSidecarWithoutDirtyingBoard(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_research"
type = "peer"
title = "Research"
`)
	boardBefore := readFile(t, store.BoardPath("session-search"))

	layout, err := store.UpdateLayoutNodes("session-search", []LayoutNode{{ID: "fmn_research", X: 320, Y: 240}}, WriteOptions{ExpectedETag: "*"})
	if err != nil {
		t.Fatalf("recreate layout sidecar: %v", err)
	}
	if layout.BoardID != "brd_01J9_sesssearch" || layout.BoardRev != 7 {
		t.Fatalf("layout board ref = %s rev %d, want board id/rev from definition", layout.BoardID, layout.BoardRev)
	}
	if len(layout.Nodes) != 1 || layout.Nodes[0].X != 320 || layout.Nodes[0].Y != 240 {
		t.Fatalf("layout nodes = %+v, want recreated node position", layout.Nodes)
	}
	if got := readFile(t, store.BoardPath("session-search")); got != boardBefore {
		t.Fatalf("layout recreation dirtied board definition:\n%s", got)
	}
	if _, err := os.Stat(store.LayoutPath("session-search")); err != nil {
		t.Fatalf("layout sidecar was not recreated: %v", err)
	}
}

func TestUpdateLayoutNodesWildcardOnlyRecreatesMissingLayoutSidecar(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))
	writeFixture(t, store.LayoutPath("session-search"), `schema = 1
boardId = "brd_session-search"
boardRev = 7
updatedAt = "2026-06-03T16:02:00Z"

[[node]]
id = "fmn_research"
x = 120
y = 80
`)
	before := readFile(t, store.LayoutPath("session-search"))

	if _, err := store.UpdateLayoutNodes("session-search", []LayoutNode{{ID: "fmn_research", X: 320, Y: 240}}, WriteOptions{ExpectedETag: "*"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("wildcard existing layout error = %v, want ErrConflict", err)
	}
	if got := readFile(t, store.LayoutPath("session-search")); got != before {
		t.Fatalf("wildcard clobbered existing layout:\n%s", got)
	}
}

func TestS3InverseMutationsRequireFreshETags(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	created, err := store.CreateFormation("session-search", FormationCreateRequest{
		Type:      FormationTypeSolo,
		Title:     "Undo me",
		X:         300,
		Y:         180,
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("create formation: %v", err)
	}

	if _, err := store.DeleteFormation("session-search", FormationDeleteRequest{
		ID:        created.Formation.ID,
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale inverse delete error = %v, want ErrConflict", err)
	}

	result, err := store.DeleteFormation("session-search", FormationDeleteRequest{
		ID:        created.Formation.ID,
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: created.Board.ETag, ExpectedRev: created.Board.Rev})
	if err != nil {
		t.Fatalf("fresh inverse delete: %v", err)
	}
	if result.Board.Rev != 9 {
		t.Fatalf("board rev = %d, want 9", result.Board.Rev)
	}
	if len(result.Board.Formations) != 0 {
		t.Fatalf("formations = %+v, want deleted formation gone", result.Board.Formations)
	}
	if len(result.Layout.Nodes) != 0 {
		t.Fatalf("layout nodes = %+v, want deleted formation node gone", result.Layout.Nodes)
	}
	if got := readFile(t, store.BoardPath("session-search")); strings.Contains(got, created.Formation.ID) {
		t.Fatalf("board still contains deleted formation id:\n%s", got)
	}
}

func TestS3UndoNeverRestoresWholeBoardSnapshots(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
customFuture = "keep me"
`)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	created, err := store.CreateFormation("session-search", FormationCreateRequest{
		Type:      FormationTypePeer,
		Title:     "Peer review",
		X:         120,
		Y:         80,
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("create formation: %v", err)
	}
	externalTitle := "Externally renamed board"
	external, err := store.UpdateBoardMetadata("session-search", BoardMetadataPatch{
		Title:     &externalTitle,
		UpdatedBy: "agent:external",
	}, WriteOptions{ExpectedETag: created.Board.ETag, ExpectedRev: created.Board.Rev})
	if err != nil {
		t.Fatalf("external edit: %v", err)
	}

	if _, err := store.DeleteFormation("session-search", FormationDeleteRequest{
		ID:        created.Formation.ID,
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: created.Board.ETag, ExpectedRev: created.Board.Rev}); !errors.Is(err, ErrConflict) {
		t.Fatalf("undo with stale create snapshot error = %v, want ErrConflict", err)
	}
	after, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read after stale undo: %v", err)
	}
	if after.Title != external.Title || after.Rev != external.Rev {
		t.Fatalf("stale undo restored an old board snapshot: after=%+v external=%+v", after, external)
	}
	if got := readFile(t, store.BoardPath("session-search")); !strings.Contains(got, `customFuture = "keep me"`) {
		t.Fatalf("undo path lost unknown fields:\n%s", got)
	}
}

func TestS3InversePrimitivesCoverBoardAndLayoutMutations(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
customFuture = "keep me"

[[mission]]
id = "mis_showcase"
title = "Showcase"
goal = "Build the page"
beadId = "home-7kc4.5"

[[formation]]
id = "fmn_frame"
type = "orchestrated"
title = "Frame"

[[formation.input]]
id = "port_frame_in"
label = "Input"

[[formation.output]]
id = "port_frame_out"
label = "Output"

[[formation.slot]]
id = "slot_lead"
label = "Lead"
controller = true
agentId = "mason"
harness = "codex"

[[formation.slot]]
id = "slot_worker"
label = "Worker"
controller = false

[[formation.brief]]
goal = "Frame it"
beadId = "home-7kc4.1"
files = ["README.md"]
links = ["https://example.invalid"]

[[formation.verification]]
id = "ver_frame"
kinds = ["code"]
criterion = "Tests pass"
onFail = "block"

[[gate]]
id = "gate_review"
title = "Review"
kinds = ["code", "formation"]
criterion = "Review the frame"

[[connection]]
id = "edge_mission_frame"
from = "mis_showcase:out"
to = "fmn_frame:port_frame_in"

[[connection]]
id = "edge_frame_gate"
from = "fmn_frame:port_frame_out"
to = "gate_review:in"

[[connection]]
id = "edge_gate_judge"
from = "gate_review:judge"
to = "fmn_frame:port_frame_in"
`)
	writeFixture(t, store.LayoutPath("session-search"), `schema = 1
boardId = "brd_01J9_sesssearch"
boardRev = 7
updatedAt = "2026-06-03T16:02:00Z"

[[node]]
id = "mis_showcase"
x = 80
y = -120

[[node]]
id = "fmn_frame"
x = 120
y = 80

[[node]]
id = "gate_review"
x = 440
y = 80

[[edge]]
id = "edge_frame_gate"
lane = "220"
`)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	clearedBrief, err := store.ClearFormationBrief("session-search", FormationBriefClearRequest{
		FormationID: "fmn_frame",
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("clear brief inverse: %v", err)
	}
	if clearedBrief.Formations[0].Brief != nil {
		t.Fatalf("brief after clear = %+v, want removed", clearedBrief.Formations[0].Brief)
	}
	if got := readFile(t, store.BoardPath("session-search")); strings.Contains(got, "[[formation.brief]]") {
		t.Fatalf("brief block still present after clear:\n%s", got)
	}

	removedVerification, err := store.RemoveFormationVerification("session-search", FormationVerificationRemovalRequest{
		FormationID: "fmn_frame",
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: clearedBrief.ETag, ExpectedRev: clearedBrief.Rev})
	if err != nil {
		t.Fatalf("remove verification inverse: %v", err)
	}
	if removedVerification.Formations[0].Verification != nil {
		t.Fatalf("verification after removal = %+v, want removed", removedVerification.Formations[0].Verification)
	}

	deletedGate, err := store.DeleteGate("session-search", GateDeleteRequest{
		ID:        "gate_review",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: removedVerification.ETag, ExpectedRev: removedVerification.Rev})
	if err != nil {
		t.Fatalf("delete gate inverse: %v", err)
	}
	if len(deletedGate.Board.Gates) != 0 {
		t.Fatalf("gates after delete = %+v, want none", deletedGate.Board.Gates)
	}
	for _, connection := range deletedGate.Board.Connections {
		if strings.Contains(connection.From, "gate_review") || strings.Contains(connection.To, "gate_review") {
			t.Fatalf("gate connection survived delete: %+v", connection)
		}
	}
	for _, node := range deletedGate.Layout.Nodes {
		if node.ID == "gate_review" {
			t.Fatalf("gate layout node survived delete: %+v", deletedGate.Layout.Nodes)
		}
	}

	deletedMission, err := store.DeleteMission("session-search", MissionDeleteRequest{
		ID:        "mis_showcase",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: deletedGate.Board.ETag, ExpectedRev: deletedGate.Board.Rev})
	if err != nil {
		t.Fatalf("delete mission inverse: %v", err)
	}
	if len(deletedMission.Board.Missions) != 0 {
		t.Fatalf("missions after delete = %+v, want none", deletedMission.Board.Missions)
	}
	if len(deletedMission.Board.Connections) != 0 {
		t.Fatalf("connections after deleting mission/gate = %+v, want all touched wires pruned", deletedMission.Board.Connections)
	}
	for _, node := range deletedMission.Layout.Nodes {
		if node.ID == "mis_showcase" {
			t.Fatalf("mission layout node survived delete: %+v", deletedMission.Layout.Nodes)
		}
	}
	if got := readFile(t, store.BoardPath("session-search")); !strings.Contains(got, `customFuture = "keep me"`) {
		t.Fatalf("inverse primitives lost unknown fields:\n%s", got)
	}
}

func TestS3SlotAssignmentUsesPersonaIDsWithoutSessionNames(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
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

[[formation.slot]]
id = "slot_peer_b"
label = "Peer B"
controller = false
`)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	assigned, err := store.AssignFormationSlot("session-search", FormationSlotAssignmentRequest{
		FormationID: "fmn_frame",
		SlotID:      "slot_peer_a",
		AgentID:     "susie",
		Harness:     "openai-codex",
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("assign slot: %v", err)
	}
	assignedAgain, err := store.AssignFormationSlot("session-search", FormationSlotAssignmentRequest{
		FormationID: "fmn_frame",
		SlotID:      "slot_peer_b",
		AgentID:     "susie",
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: assigned.ETag, ExpectedRev: assigned.Rev})
	if err != nil {
		t.Fatalf("assign same persona to another slot: %v", err)
	}

	slots := assignedAgain.Formations[0].Slots
	if slots[0].AgentID != "susie" || slots[0].Harness != "openai-codex" {
		t.Fatalf("slot A = %+v, want susie/openai-codex", slots[0])
	}
	if slots[1].AgentID != "susie" {
		t.Fatalf("slot B = %+v, want same persona reference", slots[1])
	}
	raw := readFile(t, store.BoardPath("session-search"))
	for _, forbidden := range []string{"sessionName", "sessionStem", "tmux"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("slot assignment stored runtime session field %q:\n%s", forbidden, raw)
		}
	}
}

func TestS3OrchestratedControllerIsExactlyOne(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_orch"
type = "orchestrated"
title = "Orchestrate"

[[formation.slot]]
id = "slot_controller"
label = "Controller"
controller = true

[[formation.slot]]
id = "slot_worker"
label = "Worker"
controller = false
`)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	after, err := store.SetFormationController("session-search", FormationControllerRequest{
		FormationID: "fmn_orch",
		SlotID:      "slot_worker",
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("make controller: %v", err)
	}
	var controllers int
	var workerController, previousController bool
	for _, slot := range after.Formations[0].Slots {
		if slot.Controller {
			controllers++
		}
		if slot.ID == "slot_worker" {
			workerController = slot.Controller
		}
		if slot.ID == "slot_controller" {
			previousController = slot.Controller
		}
	}
	if controllers != 1 || !workerController || previousController {
		t.Fatalf("controller slots = %+v, want only slot_worker controller", after.Formations[0].Slots)
	}
}

func TestS3BriefAndVerificationPersistHomeBeadAndOnFail(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"
customFuture = "keep me"

[[formation]]
id = "fmn_ship"
type = "solo"
title = "Ship"
`)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	withBrief, err := store.SetFormationBrief("session-search", FormationBriefRequest{
		FormationID: "fmn_ship",
		Goal:        "Ship the change",
		BeadID:      "home-7kc4.5",
		Files:       []string{"src/SessionPanel.tsx"},
		Links:       []string{"https://example.com/spec"},
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("set brief: %v", err)
	}
	after, err := store.SetFormationVerification("session-search", FormationVerificationRequest{
		FormationID: "fmn_ship",
		Kinds:       []string{"code", "human"},
		Criterion:   "Tests pass and the handoff is clear.",
		OnFail:      "pushback",
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: withBrief.ETag, ExpectedRev: withBrief.Rev})
	if err != nil {
		t.Fatalf("set verification: %v", err)
	}

	formation := after.Formations[0]
	if formation.Brief == nil || formation.Brief.Goal != "Ship the change" || formation.Brief.BeadID != "home-7kc4.5" {
		t.Fatalf("brief = %+v, want goal and home bead", formation.Brief)
	}
	if formation.Verification == nil || !strings.HasPrefix(formation.Verification.ID, "ver_") || formation.Verification.OnFail != "pushback" {
		t.Fatalf("verification = %+v, want ver_ id and onFail pushback", formation.Verification)
	}
	raw := readFile(t, store.BoardPath("session-search"))
	for _, want := range []string{
		`customFuture = "keep me"`,
		`[formation.brief]`,
		`beadId = "home-7kc4.5"`,
		`files = ["src/SessionPanel.tsx"]`,
		`links = ["https://example.com/spec"]`,
		`[formation.verification]`,
		`onFail = "pushback"`,
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("board TOML missing %q:\n%s", want, raw)
		}
	}
}

func TestS3DynamicPortsKeepStableIDsAndPruneEdges(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_ship"
type = "solo"
title = "Ship"

[[formation.input]]
id = "port_ship_a"
label = "Input A"

[[formation.input]]
id = "port_ship_b"
label = "Input B"

[[formation.input]]
id = "port_ship_c"
label = "Input C"

[[formation.output]]
id = "port_ship_out"
label = "Output"

[[connection]]
id = "edge_keep"
from = "fmn_ship:port_ship_out"
to = "fmn_ship:port_ship_a"

[[connection]]
id = "edge_prune"
from = "fmn_ship:port_ship_out"
to = "fmn_ship:port_ship_b"
`)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	withPort, err := store.AddFormationPort("session-search", FormationPortRequest{
		FormationID: "fmn_ship",
		Direction:   FormationPortInput,
		Label:       "Second input",
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("add input: %v", err)
	}
	added := withPort.Formations[0].Inputs[len(withPort.Formations[0].Inputs)-1]
	if !strings.HasPrefix(added.ID, "port_") || added.ID == "port_ship_a" || added.ID == "port_ship_b" || added.ID == "port_ship_c" {
		t.Fatalf("added input id = %q, want fresh stable port_", added.ID)
	}
	after, err := store.RemoveFormationPort("session-search", FormationPortRemovalRequest{
		FormationID: "fmn_ship",
		PortID:      "port_ship_b",
		UpdatedBy:   "agent:test",
	}, WriteOptions{ExpectedETag: withPort.ETag, ExpectedRev: withPort.Rev})
	if err != nil {
		t.Fatalf("remove input: %v", err)
	}
	if len(after.Connections) != 1 || after.Connections[0].ID != "edge_keep" {
		t.Fatalf("connections after removing port = %+v, want only edge_keep", after.Connections)
	}
	raw := readFile(t, store.BoardPath("session-search"))
	if strings.Contains(raw, "port_ship_b") || !strings.Contains(raw, "port_ship_c") || !strings.Contains(raw, added.ID) {
		t.Fatalf("port removal renumbered or failed to prune correctly:\n%s", raw)
	}
}

func TestS3WireRejectsSelfDuplicateAndSecondInput(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), s3ConnectionsBoardFixture())
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	wired, err := store.WireFormationPorts("session-search", FormationWireRequest{
		From:      "fmn_frame:port_frame_out",
		To:        "fmn_research:port_research_in",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("wire ports: %v", err)
	}
	if len(wired.Connections) != 1 || !strings.HasPrefix(wired.Connections[0].ID, "edge_") {
		t.Fatalf("connections = %+v, want one stable edge", wired.Connections)
	}
	if _, err := store.WireFormationPorts("session-search", FormationWireRequest{
		From:      "fmn_frame:port_frame_out",
		To:        "fmn_research:port_research_in",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: wired.ETag, ExpectedRev: wired.Rev}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate wire error = %v, want ErrConflict", err)
	}
	if _, err := store.WireFormationPorts("session-search", FormationWireRequest{
		From:      "fmn_research:port_research_out",
		To:        "fmn_research:port_research_in",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: wired.ETag, ExpectedRev: wired.Rev}); !errors.Is(err, ErrConflict) {
		t.Fatalf("self wire error = %v, want ErrConflict", err)
	}
	if _, err := store.WireFormationPorts("session-search", FormationWireRequest{
		From:      "fmn_ship:port_ship_out",
		To:        "fmn_research:port_research_in",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: wired.ETag, ExpectedRev: wired.Rev}); !errors.Is(err, ErrConflict) {
		t.Fatalf("second input wire error = %v, want ErrConflict", err)
	}
}

func TestS3HandRouteWritesLayoutOnly(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), s3ConnectionsBoardFixture())
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	wired, err := store.WireFormationPorts("session-search", FormationWireRequest{
		From:      "fmn_frame:port_frame_out",
		To:        "fmn_research:port_research_in",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("wire ports: %v", err)
	}
	boardBeforeLane := readFile(t, store.BoardPath("session-search"))
	writeFixture(t, store.LayoutPath("session-search"), `schema = 1
boardId = "brd_01J9_sesssearch"
boardRev = 8
updatedAt = "2026-06-03T16:02:00Z"
`)
	layout, err := store.ReadLayout("session-search")
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}
	routed, err := store.UpdateLayoutEdges("session-search", []LayoutEdge{{ID: wired.Connections[0].ID, Lane: "mid-2"}}, WriteOptions{ExpectedETag: layout.ETag})
	if err != nil {
		t.Fatalf("hand route edge: %v", err)
	}
	if got := readFile(t, store.BoardPath("session-search")); got != boardBeforeLane {
		t.Fatalf("hand route changed board definition:\n%s", got)
	}
	if len(routed.Edges) != 1 || routed.Edges[0].Lane != "mid-2" {
		t.Fatalf("layout edges = %+v, want hand-routed lane", routed.Edges)
	}
}

func TestS3GatePersistsKindsCriterionWithoutVerdictOrOnFail(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	after, err := store.CreateGate("session-search", GateCreateRequest{
		Title:     "Review gate",
		Kinds:     []string{"code", "human"},
		Criterion: "Research is sound and safe to build.",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("create gate: %v", err)
	}
	if len(after.Gates) != 1 || after.Gates[0].Title != "Review gate" || strings.Join(after.Gates[0].Kinds, ",") != "code,human" {
		t.Fatalf("gates = %+v, want review gate with code,human", after.Gates)
	}
	raw := readFile(t, store.BoardPath("session-search"))
	if strings.Contains(raw, "verdict") || strings.Contains(raw, "onFail") {
		t.Fatalf("gate persisted runtime verdict/onFail fields:\n%s", raw)
	}
}

func TestS3GateFailBehaviorIsOnlyWiring(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), s3ConnectionsBoardFixture())
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	withGate, err := store.CreateGate("session-search", GateCreateRequest{
		Title:     "Review gate",
		Kinds:     []string{"code"},
		Criterion: "Check it.",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("create gate: %v", err)
	}
	gateID := withGate.Gates[0].ID
	after, err := store.WireFormationPorts("session-search", FormationWireRequest{
		From:      gateID + ":fail",
		To:        "fmn_frame:port_frame_in",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: withGate.ETag, ExpectedRev: withGate.Rev})
	if err != nil {
		t.Fatalf("wire fail pushback: %v", err)
	}
	if len(after.Connections) != 1 || after.Connections[0].From != gateID+":fail" {
		t.Fatalf("connections = %+v, want fail wire as behavior", after.Connections)
	}
	raw := readFile(t, store.BoardPath("session-search"))
	if strings.Contains(raw, "onFail") || strings.Contains(raw, "verdict") {
		t.Fatalf("gate fail behavior stored as field instead of wiring:\n%s", raw)
	}
}

func TestS3JudgeLoopAndChainUseConnectionsWithSingleSendAndReturn(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), s3JudgeBoardFixture())
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	after, err := store.SetGateJudgeChain("session-search", GateJudgeRequest{
		GateID:    "gate_review",
		Chain:     []string{"fmn_j1", "fmn_j2"},
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("set judge chain: %v", err)
	}
	var sends, returns int
	for _, connection := range after.Connections {
		if connection.From == "gate_review:judge" {
			sends++
		}
		if connection.To == "gate_review:judge" {
			returns++
		}
	}
	if sends != 1 || returns != 1 || len(after.Connections) != 3 {
		t.Fatalf("judge connections = %+v, want one send, one mid, one return", after.Connections)
	}
	if !containsString(after.Gates[0].Kinds, "formation") {
		t.Fatalf("gate kinds = %+v, want formation kind while judge attached", after.Gates[0].Kinds)
	}
}

func TestS3JudgeReturnCanMoveWithoutBreakingEntry(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), s3JudgeBoardFixture())
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	first, err := store.SetGateJudgeChain("session-search", GateJudgeRequest{
		GateID:    "gate_review",
		Chain:     []string{"fmn_j1", "fmn_j2"},
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("set first judge chain: %v", err)
	}
	after, err := store.SetGateJudgeChain("session-search", GateJudgeRequest{
		GateID:    "gate_review",
		Chain:     []string{"fmn_j1", "fmn_j2", "fmn_j3"},
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: first.ETag, ExpectedRev: first.Rev})
	if err != nil {
		t.Fatalf("move judge return: %v", err)
	}
	if !hasConnection(after.Connections, "gate_review:judge", "fmn_j1:port_j1_in") {
		t.Fatalf("judge entry was not preserved: %+v", after.Connections)
	}
	if !hasConnection(after.Connections, "fmn_j3:port_j3_out", "gate_review:judge") {
		t.Fatalf("judge return did not move to j3: %+v", after.Connections)
	}
}

func TestS3JudgeChainRejectsSecondIncomingInputLikeNormalConnections(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), s3JudgeBoardFixture()+`
[[connection]]
id = "edge_existing_input"
from = "fmn_j2:port_j2_out"
to = "fmn_j1:port_j1_in"
`)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	if _, err := store.SetGateJudgeChain("session-search", GateJudgeRequest{
		GateID:    "gate_review",
		Chain:     []string{"fmn_j1"},
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev}); !errors.Is(err, ErrConflict) {
		t.Fatalf("judge chain second input error = %v, want ErrConflict", err)
	}
	after, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board after conflict: %v", err)
	}
	if after.Rev != before.Rev || len(after.Connections) != 1 {
		t.Fatalf("conflicting judge chain mutated board: before rev %d after %+v", before.Rev, after)
	}
}

func TestS3MissionCreateRequiresHomeBeadIDAndSingleOut(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), minimalBoard("session-search", 7))
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	if _, err := store.CreateMission("session-search", MissionCreateRequest{
		Title:     "Showcase site",
		Goal:      "Build the showcase",
		BeadID:    "bd-204",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev}); !errors.Is(err, ErrInvalidSlug) {
		t.Fatalf("bd-prefixed mission error = %v, want ErrInvalidSlug", err)
	}
	after, err := store.CreateMission("session-search", MissionCreateRequest{
		Title:     "Showcase site",
		Goal:      "Build the showcase",
		BeadID:    "home-7kc4.5",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("create mission: %v", err)
	}
	if len(after.Missions) != 1 || !strings.HasPrefix(after.Missions[0].ID, "mis_") || after.Missions[0].BeadID != "home-7kc4.5" {
		t.Fatalf("missions = %+v, want one home-backed mission", after.Missions)
	}
	raw := readFile(t, store.BoardPath("session-search"))
	if strings.Contains(raw, "chain") || strings.Count(raw, "out") != 0 {
		t.Fatalf("mission stored a chain or explicit dynamic port instead of fixed out:\n%s", raw)
	}
}

func TestS3MissionWireIsDerivedConnectionNoStoredChain(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("session-search"), `schema = 1
id = "brd_01J9_sesssearch"
slug = "session-search"
title = "Improve session search"
rev = 7
updatedAt = "2026-06-03T16:00:00Z"

[[mission]]
id = "mis_showcase"
title = "Showcase"
goal = "Build it"
beadId = "home-7kc4.5"

[[formation]]
id = "fmn_frame"
type = "solo"
title = "Frame"

[[formation.input]]
id = "port_frame_in"
label = "Input"
`)
	before, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	after, err := store.WireFormationPorts("session-search", FormationWireRequest{
		From:      "mis_showcase:out",
		To:        "fmn_frame:port_frame_in",
		UpdatedBy: "agent:test",
	}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
	if err != nil {
		t.Fatalf("wire mission: %v", err)
	}
	if len(after.Connections) != 1 || after.Connections[0].From != "mis_showcase:out" {
		t.Fatalf("connections = %+v, want mission out connection", after.Connections)
	}
	if got := readFile(t, store.BoardPath("session-search")); strings.Contains(got, "chain") {
		t.Fatalf("mission wire stored a chain instead of a connection:\n%s", got)
	}
}

func fixedClock() func() time.Time {
	return func() time.Time {
		return time.Date(2026, 6, 3, 17, 0, 0, 0, time.UTC)
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func minimalBoard(slug string, rev int) string {
	return `schema = 1
id = "brd_01J9_sesssearch"
slug = "` + slug + `"
title = "Improve session search"
rev = ` + strconv.Itoa(rev) + `
updatedBy = "agent:archon"
updatedAt = "2026-06-03T16:00:00Z"
`
}

func stringPtr(v string) *string {
	return &v
}

func s3ConnectionsBoardFixture() string {
	return `schema = 1
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

[[formation.output]]
id = "port_ship_out"
label = "Output"
`
}

func s3JudgeBoardFixture() string {
	return `schema = 1
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

[[formation]]
id = "fmn_j3"
type = "solo"
title = "Judge 3"

[[formation.input]]
id = "port_j3_in"
label = "Input"

[[formation.output]]
id = "port_j3_out"
label = "Output"
`
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasConnection(connections []BoardConnection, from, to string) bool {
	for _, connection := range connections {
		if connection.From == from && connection.To == to {
			return true
		}
	}
	return false
}
