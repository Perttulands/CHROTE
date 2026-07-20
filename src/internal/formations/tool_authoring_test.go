package formations

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCreateToolMigratesSchemaOneAndDerivesExactDescriptorShape(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-migration"
	writeFixture(t, store.BoardPath(slug), toolSchemaMigrationLegacyFixture())
	before, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read schema-1 source: %v", err)
	}

	result, err := store.CreateTool(slug, ToolCreateRequest{
		ProfileID:      "json.normalize",
		ProfileVersion: "1",
		Title:          "Normalize report",
		Params:         map[string]any{"mode": "strict"},
		Placement:      ToolPlacement{},
		UpdatedBy:      "agent:store-test",
	}, toolAuthoringAbsentOptions(before))
	if err != nil {
		t.Fatalf("create first Tool: %v", err)
	}
	if result.Board.Schema != CurrentBoardSchema || result.Board.Rev != before.Rev+1 {
		t.Fatalf("created board schema/rev = %d/%d, want %d/%d", result.Board.Schema, result.Board.Rev, CurrentBoardSchema, before.Rev+1)
	}
	if len(result.Board.Connections) != len(before.Connections) {
		t.Fatalf("create added implicit wires: connections %d -> %d", len(before.Connections), len(result.Board.Connections))
	}
	if len(result.Board.Tools) != 1 || !reflect.DeepEqual(result.Board.Tools[0], result.Tool) {
		t.Fatalf("created Tool/result mismatch: tools=%#v result=%#v", result.Board.Tools, result.Tool)
	}
	assertJSONNormalizeToolDerivedFromDescriptor(t, result.Tool)
	if result.Tool.Title != "Normalize report" || !reflect.DeepEqual(result.Tool.Params, map[string]any{"mode": "strict"}) {
		t.Fatalf("created Tool authored fields = %#v", result.Tool)
	}
	if result.Layout == nil || result.Layout.BoardID != result.Board.ID || result.Layout.BoardRev != result.Board.Rev {
		t.Fatalf("created layout identity = %#v, want current board", result.Layout)
	}
	if len(result.Layout.Nodes) != 1 || result.Layout.Nodes[0].ID != result.Tool.ID {
		t.Fatalf("absent layout materialization nodes = %#v, want only new Tool", result.Layout.Nodes)
	}
	if !strings.Contains(result.Board.TOML, `x_extension]`) ||
		!strings.Contains(result.Board.TOML, `channel = "judge"`) ||
		!strings.Contains(result.Board.TOML, `acceptedMediaTypes = ["text/plain", "text/markdown", "application/json"]`) {
		t.Fatalf("schema migration did not preserve extension and add typed defaults:\n%s", result.Board.TOML)
	}
}

func TestCreateToolExactPlacementPreservesRetainedLayoutSourceAndFiltersInertBlocks(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-exact-placement"
	boardRaw := toolAuthoringBoardFixture(slug, 3, true, "")
	layoutRaw := `schema = 1 # keep schema comment
boardId = "brd_tool-exact-placement"
boardRev = 3 # old revision comment
updatedAt = "2026-07-19T00:00:00Z"

# retained node comment
[[node]]
id = "mis_main"
x = 17 # keep exact x
y = 29

[[node]]
id = "stale_node"
x = 999
y = 999

[[edge]]
id = "stale_edge"
lane = "north" # stale route
`
	writeFixture(t, store.BoardPath(slug), boardRaw)
	writeFixture(t, store.LayoutPath(slug), layoutRaw)
	board, layout := toolAuthoringReadPair(t, store, slug)
	x, y := 345, 678

	result, err := store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{X: &x, Y: &y}), toolAuthoringPresentOptions(board, layout))
	if err != nil {
		t.Fatalf("create Tool at exact coordinate: %v", err)
	}
	if result.Layout == nil || len(result.Layout.Nodes) != 2 {
		t.Fatalf("filtered layout nodes = %#v, want retained Mission plus Tool", result.Layout)
	}
	position, ok := toolAuthoringLayoutNode(result.Layout.Nodes, result.Tool.ID)
	if !ok || position.X != x || position.Y != y {
		t.Fatalf("exact Tool position = %#v found=%t, want %d,%d", position, ok, x, y)
	}
	if strings.Contains(result.Layout.TOML, "stale_node") || strings.Contains(result.Layout.TOML, "stale_edge") {
		t.Fatalf("inert layout blocks survived filtering:\n%s", result.Layout.TOML)
	}
	const retained = `# retained node comment
[[node]]
id = "mis_main"
x = 17 # keep exact x
y = 29`
	if !strings.Contains(result.Layout.TOML, retained) {
		t.Fatalf("retained node source changed:\n%s", result.Layout.TOML)
	}
	if result.Layout.BoardRev != result.Board.Rev {
		t.Fatalf("layout boardRev = %d, want %d", result.Layout.BoardRev, result.Board.Rev)
	}
}

func TestCreateToolHeuristicUsesLockedPredecessorAndSuccessorWithoutMovingOrWiring(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-hints"
	boardRaw := toolAuthoringBoardFixture(slug, 4, true,
		toolStructuralJSONNormalizeToolBlock("tool_existing", "Existing", "port_existing_in", "port_existing_out"))
	layoutRaw := `schema = 1
boardId = "brd_tool-hints"
boardRev = 4

[[node]]
id = "mis_main"
x = 112
y = 112

[[node]]
id = "tool_existing"
x = 1120
y = 112
`
	writeFixture(t, store.BoardPath(slug), boardRaw)
	writeFixture(t, store.LayoutPath(slug), layoutRaw)
	board, layout := toolAuthoringReadPair(t, store, slug)

	result, err := store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{
		PredecessorNodeID: "mis_main",
		SuccessorNodeID:   "tool_existing",
	}), toolAuthoringPresentOptions(board, layout))
	if err != nil {
		t.Fatalf("create Tool from predecessor/successor hints: %v", err)
	}
	position, ok := toolAuthoringLayoutNode(result.Layout.Nodes, result.Tool.ID)
	if !ok || position.X != 616 || position.Y != 112 {
		t.Fatalf("hinted Tool position = %#v found=%t, want midpoint 616,112", position, ok)
	}
	if len(result.Board.Connections) != 0 {
		t.Fatalf("placement hints created implicit wires: %#v", result.Board.Connections)
	}
	for id, want := range map[string]LayoutNode{
		"mis_main":      {ID: "mis_main", X: 112, Y: 112},
		"tool_existing": {ID: "tool_existing", X: 1120, Y: 112},
	} {
		got, found := toolAuthoringLayoutNode(result.Layout.Nodes, id)
		if !found || got != want {
			t.Fatalf("retained node %q moved: got %#v found=%t want %#v", id, got, found, want)
		}
	}
}

func TestCreateToolHeuristicCoversEmptyAndSingleHintModes(t *testing.T) {
	tests := []struct {
		name      string
		placement ToolPlacement
		wantX     int
		wantY     int
	}{
		{name: "empty hints", placement: ToolPlacement{}, wantX: 448, wantY: 112},
		{name: "predecessor only", placement: ToolPlacement{PredecessorNodeID: "mis_main"}, wantX: 448, wantY: 112},
		{name: "successor only", placement: ToolPlacement{SuccessorNodeID: "tool_existing"}, wantX: 784, wantY: 112},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-single-hint"
			writeFixture(t, store.BoardPath(slug), toolAuthoringBoardFixture(slug, 4, true,
				toolStructuralJSONNormalizeToolBlock("tool_existing", "Existing", "port_existing_in", "port_existing_out")))
			writeFixture(t, store.LayoutPath(slug), `schema = 1
boardId = "brd_tool-single-hint"
boardRev = 4

[[node]]
id = "mis_main"
x = 112
y = 112

[[node]]
id = "tool_existing"
x = 1120
y = 112
`)
			board, layout := toolAuthoringReadPair(t, store, slug)
			result, err := store.CreateTool(slug, toolAuthoringCreateRequest(test.placement), toolAuthoringPresentOptions(board, layout))
			if err != nil {
				t.Fatalf("create Tool: %v", err)
			}
			position, found := toolAuthoringLayoutNode(result.Layout.Nodes, result.Tool.ID)
			if !found || position.X != test.wantX || position.Y != test.wantY {
				t.Fatalf("position = %#v found=%t, want %d,%d", position, found, test.wantX, test.wantY)
			}
			if len(result.Board.Connections) != 0 {
				t.Fatalf("hint mode created connections: %#v", result.Board.Connections)
			}
			for id, want := range map[string]LayoutNode{
				"mis_main":      {ID: "mis_main", X: 112, Y: 112},
				"tool_existing": {ID: "tool_existing", X: 1120, Y: 112},
			} {
				got, ok := toolAuthoringLayoutNode(result.Layout.Nodes, id)
				if !ok || got != want {
					t.Fatalf("retained node %q moved: got %#v found=%t want %#v", id, got, ok, want)
				}
			}
		})
	}
}

func TestCreateToolNeverAdoptsStaleSameIDLayoutEntry(t *testing.T) {
	store := newToolAuthoringStore(t)
	ids := []string{"tool_stale", "port_created_in", "port_created_out"}
	store.newToolDefinitionID = func(prefix string) string {
		if len(ids) == 0 {
			t.Fatalf("unexpected extra Tool id request for prefix %q", prefix)
		}
		id := ids[0]
		ids = ids[1:]
		return id
	}
	slug := "tool-stale-layout"
	writeFixture(t, store.BoardPath(slug), toolAuthoringBoardFixture(slug, 2, true, ""))
	writeFixture(t, store.LayoutPath(slug), `schema = 1
boardId = "brd_tool-stale-layout"
boardRev = 2

[[node]]
id = "mis_main"
x = 112
y = 112

[[node]]
id = "tool_stale"
x = 999
y = 999
`)
	board, layout := toolAuthoringReadPair(t, store, slug)
	x, y := 420, 504
	result, err := store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{X: &x, Y: &y}), toolAuthoringPresentOptions(board, layout))
	if err != nil {
		t.Fatalf("create Tool over stale same-id layout entry: %v", err)
	}
	if result.Tool.ID != "tool_stale" {
		t.Fatalf("created Tool id = %q, want forced store-owned id", result.Tool.ID)
	}
	position, found := toolAuthoringLayoutNode(result.Layout.Nodes, result.Tool.ID)
	if !found || position.X != x || position.Y != y {
		t.Fatalf("new Tool adopted stale coordinate: %#v found=%t", position, found)
	}
	count := 0
	for _, node := range result.Layout.Nodes {
		if node.ID == result.Tool.ID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("same-id layout node count = %d, want exactly one new placement", count)
	}
}

func TestCreateToolHonorsExactBoardAndLayoutPreconditions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ToolWriteOptions)
	}{
		{name: "board ETag", mutate: func(opts *ToolWriteOptions) { opts.Board.ExpectedETag = strings.Repeat("0", 64) }},
		{name: "board revision", mutate: func(opts *ToolWriteOptions) { opts.Board.ExpectedRev++ }},
		{name: "layout ETag", mutate: func(opts *ToolWriteOptions) { opts.Layout.ETag = strings.Repeat("f", 64) }},
		{name: "layout state", mutate: func(opts *ToolWriteOptions) { opts.Layout.State, opts.Layout.ETag = "absent", "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-create-cas"
			boardRaw := toolAuthoringBoardFixture(slug, 2, true, "")
			layoutRaw := "schema = 1\nboardId = \"brd_tool-create-cas\"\nboardRev = 2\n"
			writeFixture(t, store.BoardPath(slug), boardRaw)
			writeFixture(t, store.LayoutPath(slug), layoutRaw)
			board, layout := toolAuthoringReadPair(t, store, slug)
			opts := toolAuthoringPresentOptions(board, layout)
			test.mutate(&opts)
			_, err := store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), opts)
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("stale precondition error = %v, want ErrConflict", err)
			}
			assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, &layoutRaw)
		})
	}
}

func TestCreateToolRequiresOneClosedLayoutExpectation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ToolWriteOptions)
	}{
		{name: "missing layout expectation", mutate: func(opts *ToolWriteOptions) { opts.Layout = nil }},
		{name: "present without ETag", mutate: func(opts *ToolWriteOptions) { opts.Layout.ETag = "" }},
		{name: "absent with ETag", mutate: func(opts *ToolWriteOptions) { opts.Layout.State = "absent" }},
		{name: "unknown state", mutate: func(opts *ToolWriteOptions) { opts.Layout.State = "maybe" }},
		{name: "missing board ETag", mutate: func(opts *ToolWriteOptions) { opts.Board.ExpectedETag = "" }},
		{name: "missing board revision", mutate: func(opts *ToolWriteOptions) { opts.Board.ExpectedRev = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-create-precondition"
			boardRaw := toolAuthoringBoardFixture(slug, 2, true, "")
			layoutRaw := "schema = 1\nboardId = \"brd_tool-create-precondition\"\nboardRev = 2\n"
			writeFixture(t, store.BoardPath(slug), boardRaw)
			writeFixture(t, store.LayoutPath(slug), layoutRaw)
			board, layout := toolAuthoringReadPair(t, store, slug)
			opts := toolAuthoringPresentOptions(board, layout)
			test.mutate(&opts)
			_, err := store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), opts)
			if !errors.Is(err, ErrPreconditionRequired) {
				t.Fatalf("invalid expectation error = %v, want ErrPreconditionRequired", err)
			}
			assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, &layoutRaw)
		})
	}
}

func TestCreateToolAllowsStructuralDraftGaps(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-draft-gaps"
	boardRaw := toolAuthoringBoardFixture(slug, 1, false, `[[gate]]
id = "gate_unroutable"
title = "Incomplete review"
kinds = ["code"]
criterion = ""
`)
	writeFixture(t, store.BoardPath(slug), boardRaw)
	board, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read draft board: %v", err)
	}
	result, err := store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), toolAuthoringAbsentOptions(board))
	if err != nil {
		t.Fatalf("author Tool on no-Mission/unroutable-Gate draft: %v", err)
	}
	if len(result.Board.Missions) != 0 || len(result.Board.Gates) != 1 || len(result.Board.Tools) != 1 || len(result.Board.Connections) != 0 {
		t.Fatalf("draft authoring result = %#v", result.Board)
	}
	if len(result.Tool.Inputs) != 1 || result.Tool.Inputs[0].Required == nil || !*result.Tool.Inputs[0].Required {
		t.Fatalf("draft did not retain unwired required Tool input: %#v", result.Tool.Inputs)
	}
}

func TestCreateToolRejectsInvalidAuthoringInputsWithoutMutation(t *testing.T) {
	x, y := 112, 224
	tests := []struct {
		name   string
		mutate func(*ToolCreateRequest)
	}{
		{name: "blank title", mutate: func(req *ToolCreateRequest) { req.Title = " \t" }},
		{name: "unknown tuple", mutate: func(req *ToolCreateRequest) { req.ProfileVersion = "2" }},
		{name: "null parameter object", mutate: func(req *ToolCreateRequest) { req.Params = nil }},
		{name: "partial parameter object", mutate: func(req *ToolCreateRequest) { req.Params = map[string]any{} }},
		{name: "invalid parameter enum", mutate: func(req *ToolCreateRequest) { req.Params = map[string]any{"mode": "relaxed"} }},
		{name: "nested parameter", mutate: func(req *ToolCreateRequest) { req.Params = map[string]any{"mode": []string{"strict"}} }},
		{name: "partial exact coordinates", mutate: func(req *ToolCreateRequest) { req.Placement.X = &x }},
		{name: "coordinate and hint union", mutate: func(req *ToolCreateRequest) {
			req.Placement = ToolPlacement{X: &x, Y: &y, PredecessorNodeID: "mis_main"}
		}},
		{name: "unknown predecessor", mutate: func(req *ToolCreateRequest) { req.Placement.PredecessorNodeID = "missing" }},
		{name: "same predecessor and successor", mutate: func(req *ToolCreateRequest) {
			req.Placement.PredecessorNodeID = "mis_main"
			req.Placement.SuccessorNodeID = "mis_main"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-invalid-request"
			boardRaw := toolAuthoringBoardFixture(slug, 2, true, "")
			writeFixture(t, store.BoardPath(slug), boardRaw)
			board, err := store.ReadBoard(slug)
			if err != nil {
				t.Fatalf("read board: %v", err)
			}
			req := toolAuthoringCreateRequest(ToolPlacement{})
			test.mutate(&req)
			if _, err := store.CreateTool(slug, req, toolAuthoringAbsentOptions(board)); err == nil {
				t.Fatal("invalid Tool create succeeded")
			}
			assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, nil)
		})
	}
}

func TestCreateToolRejectsSchemaMigrationHazardWithoutMutation(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-migration-hazard"
	boardRaw := `schema = 1
id = "brd_tool-migration-hazard"
slug = "tool-migration-hazard"
title = "Migration hazard"
rev = 3

[[gate]]
id = "gate_legacy"
title = "Legacy"
kinds = ["code"]
criterion = "Run"
command = "./unsafe.sh"
`
	writeFixture(t, store.BoardPath(slug), boardRaw)
	board, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read migration source: %v", err)
	}
	if _, err := store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), toolAuthoringAbsentOptions(board)); err == nil ||
		!strings.Contains(err.Error(), LegacyScriptGateMigrationCode) {
		t.Fatalf("migration hazard error = %v, want stable legacy script fence", err)
	}
	assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, nil)
}

func TestCreateToolRejectsMismatchedMalformedAndDuplicateLayoutIDs(t *testing.T) {
	tests := []struct {
		name   string
		layout string
	}{
		{name: "board mismatch", layout: "schema = 1\nboardId = \"brd_other\"\nboardRev = 2\n"},
		{name: "missing node id", layout: "schema = 1\nboardId = \"brd_tool-layout-invalid\"\nboardRev = 2\n\n[[node]]\nx = 1\ny = 2\n"},
		{name: "duplicate node id", layout: "schema = 1\nboardId = \"brd_tool-layout-invalid\"\nboardRev = 2\n\n[[node]]\nid = \"mis_main\"\nx = 1\ny = 2\n\n[[node]]\nid = \"mis_main\"\nx = 3\ny = 4\n"},
		{name: "duplicate edge id", layout: "schema = 1\nboardId = \"brd_tool-layout-invalid\"\nboardRev = 2\n\n[[edge]]\nid = \"edge_stale\"\nlane = \"a\"\n\n[[edge]]\nid = \"edge_stale\"\nlane = \"b\"\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-layout-invalid"
			boardRaw := toolAuthoringBoardFixture(slug, 2, true, "")
			writeFixture(t, store.BoardPath(slug), boardRaw)
			writeFixture(t, store.LayoutPath(slug), test.layout)
			board, layout := toolAuthoringReadPair(t, store, slug)
			if _, err := store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), toolAuthoringPresentOptions(board, layout)); err == nil {
				t.Fatal("invalid layout accepted")
			}
			layoutRaw := test.layout
			assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, &layoutRaw)
		})
	}
}

func TestCreateToolRejectsStructuralInvalidBoardsWithoutMutation(t *testing.T) {
	validTool := toolStructuralJSONNormalizeToolBlock("tool_existing", "Existing", "port_existing_in", "port_existing_out")
	tests := []struct {
		name string
		tail string
	}{
		{name: "empty node id", tail: `[[gate]]
id = ""
title = "Broken"
kinds = ["human"]
criterion = "Confirm"
`},
		{name: "duplicate node id", tail: validTool + `[[gate]]
id = "tool_existing"
title = "Duplicate"
kinds = ["human"]
criterion = "Confirm"
`},
		{name: "duplicate port id", tail: strings.Replace(validTool, `id = "port_existing_out"`, `id = "port_existing_in"`, 1)},
		{name: "empty edge id", tail: validTool + `[[connection]]
id = ""
channel = "workflow"
from = "tool_existing:port_existing_out"
to = "tool_existing:port_existing_in"
`},
		{name: "dangling edge", tail: validTool + `[[connection]]
id = "edge_dangling"
channel = "workflow"
from = "tool_existing:port_existing_out"
to = "ghost:input"
`},
		{name: "incompatible media", tail: validTool + `[[connection]]
id = "edge_media"
channel = "workflow"
from = "mis_main:out"
to = "tool_existing:port_existing_in"
`},
		{name: "Tool judge misuse", tail: validTool + `[[gate]]
id = "gate_review"
title = "Review"
kinds = ["formation"]
criterion = "Review"

[[connection]]
id = "edge_judge_misuse"
channel = "judge"
from = "tool_existing:port_existing_out"
to = "gate_review:judge"
`},
		{name: "duplicate edge id", tail: validTool +
			toolStructuralJSONNormalizeToolBlock("tool_left", "Left", "port_left_in", "port_left_out") +
			toolStructuralJSONNormalizeToolBlock("tool_right", "Right", "port_right_in", "port_right_out") + `[[connection]]
id = "edge_duplicate"
channel = "workflow"
from = "tool_existing:port_existing_out"
to = "tool_left:port_left_in"

[[connection]]
id = "edge_duplicate"
channel = "workflow"
from = "tool_left:port_left_out"
to = "tool_right:port_right_in"
`},
		{name: "duplicate producer", tail: validTool +
			toolStructuralJSONNormalizeToolBlock("tool_source", "Source", "port_source_in", "port_source_out") +
			toolStructuralJSONNormalizeToolBlock("tool_target", "Target", "port_target_in", "port_target_out") + `[[connection]]
id = "edge_first"
channel = "workflow"
from = "tool_existing:port_existing_out"
to = "tool_target:port_target_in"

[[connection]]
id = "edge_second"
channel = "workflow"
from = "tool_source:port_source_out"
to = "tool_target:port_target_in"
`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-structural-invalid"
			boardRaw := toolAuthoringBoardFixture(slug, 2, true, test.tail)
			writeFixture(t, store.BoardPath(slug), boardRaw)
			board, err := store.ReadBoard(slug)
			if err != nil {
				t.Fatalf("inspection parse should remain readable before structural authoring rejection: %v", err)
			}
			if _, err := store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), toolAuthoringAbsentOptions(board)); err == nil {
				t.Fatal("structural-invalid board accepted Tool mutation")
			}
			assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, nil)
		})
	}
}

func newToolAuthoringStore(t *testing.T) *Store {
	t.Helper()
	store := NewStore(t.TempDir())
	store.Now = func() time.Time {
		return time.Date(2026, 7, 20, 8, 30, 0, 0, time.UTC)
	}
	return store
}

func toolAuthoringBoardFixture(slug string, rev int, includeMission bool, tail string) string {
	raw := fmt.Sprintf("schema = 2\nid = %q\nslug = %q\ntitle = \"Tool authoring\"\nrev = %d\n", "brd_"+slug, slug, rev)
	if includeMission {
		raw += `
[[mission]]
id = "mis_main"
title = "Main"
goal = "Author a Tool"
beadId = "ctx-test"
`
	}
	if tail != "" && !strings.HasSuffix(raw, "\n") {
		raw += "\n"
	}
	return raw + tail
}

func toolAuthoringCreateRequest(placement ToolPlacement) ToolCreateRequest {
	return ToolCreateRequest{
		ProfileID:      "json.normalize",
		ProfileVersion: "1",
		Title:          "Normalize report",
		Params:         map[string]any{"mode": "strict"},
		Placement:      placement,
		UpdatedBy:      "agent:store-test",
	}
}

func toolAuthoringReadPair(t *testing.T, store *Store, slug string) (*BoardDocument, *LayoutDocument) {
	t.Helper()
	board, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	layout, err := store.ReadLayout(slug)
	if err != nil {
		t.Fatalf("read layout: %v", err)
	}
	return board, layout
}

func toolAuthoringAbsentOptions(board *BoardDocument) ToolWriteOptions {
	return ToolWriteOptions{
		Board:  WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev},
		Layout: &LayoutWriteExpectation{State: "absent"},
	}
}

func toolAuthoringPresentOptions(board *BoardDocument, layout *LayoutDocument) ToolWriteOptions {
	return ToolWriteOptions{
		Board:  WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev},
		Layout: &LayoutWriteExpectation{State: "present", ETag: layout.ETag},
	}
}

func toolAuthoringLayoutNode(nodes []LayoutNode, id string) (LayoutNode, bool) {
	for _, node := range nodes {
		if node.ID == id {
			return node, true
		}
	}
	return LayoutNode{}, false
}

func assertJSONNormalizeToolDerivedFromDescriptor(t *testing.T, tool ToolNode) {
	t.Helper()
	descriptor, ok := LookupToolProfileDescriptor("json.normalize", "1")
	if !ok {
		t.Fatal("frozen json.normalize@1 descriptor missing")
	}
	if !strings.HasPrefix(tool.ID, "tool_") || tool.ProfileID != descriptor.ProfileID || tool.ProfileVersion != descriptor.ProfileVersion {
		t.Fatalf("Tool identity/tuple = %#v, want generated id and exact descriptor tuple", tool)
	}
	ports := append(append([]ToolPort(nil), tool.Inputs...), tool.Outputs...)
	if len(ports) != len(descriptor.Ports) {
		t.Fatalf("Tool port count = %d, want %d", len(ports), len(descriptor.Ports))
	}
	seenIDs := map[string]bool{}
	for index, want := range descriptor.Ports {
		got := ports[index]
		if !strings.HasPrefix(got.ID, "port_") || seenIDs[got.ID] {
			t.Fatalf("generated Tool port id %q is empty, wrong-prefix, or duplicate", got.ID)
		}
		seenIDs[got.ID] = true
		if got.Name != want.Name || got.Label != want.Label || got.Direction != want.Direction || got.Kind != want.Kind ||
			!reflect.DeepEqual(got.AcceptedMediaTypes, want.AcceptedMediaTypes) || !equalToolBool(got.Required, want.Required) || !equalToolString(got.Role, want.Role) {
			t.Fatalf("Tool port %d = %#v, want exact descriptor %#v", index, got, want)
		}
	}
}

func assertToolAuthoringPairUnchanged(t *testing.T, store *Store, slug, boardRaw string, layoutRaw *string) {
	t.Helper()
	if got := readFile(t, store.BoardPath(slug)); got != boardRaw {
		t.Fatalf("failed Tool mutation changed board:\n got %q\nwant %q", got, boardRaw)
	}
	if layoutRaw != nil {
		if got := readFile(t, store.LayoutPath(slug)); got != *layoutRaw {
			t.Fatalf("failed Tool mutation changed layout:\n got %q\nwant %q", got, *layoutRaw)
		}
		return
	}
	if _, err := os.Lstat(store.LayoutPath(slug)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed Tool mutation materialized absent layout: %v", err)
	}
}
