package formations

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCreateToolMigratesSchemaOneAndDerivesExactDescriptorShape(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-migration"
	source := toolSchemaMigrationLegacyFixture()
	writeFixture(t, store.BoardPath(slug), source)
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
	if got, want := toolAuthoringCoreNodeIDs(result.Board), toolAuthoringCoreNodeIDs(before); !reflect.DeepEqual(got, want) {
		t.Fatalf("first-Tool migration changed retained node ids/order: got %v want %v", got, want)
	}
	if len(result.Board.Connections) != len(before.Connections) {
		t.Fatalf("first-Tool migration connection count = %d, want %d", len(result.Board.Connections), len(before.Connections))
	}
	for index, want := range before.Connections {
		got := result.Board.Connections[index]
		if got.ID != want.ID || got.From != want.From || got.To != want.To {
			t.Fatalf("first-Tool migration changed connection %d identity/endpoints: got %#v want %#v", index, got, want)
		}
	}
	retainedSpans := []string{
		`x_owner = "keep" # unknown top-level field`,
		`[[mission]]
id = "mis_main"
title = "Main"
goal = "Review the work"
beadId = "ctx-test"
x_mission_note = "keep"`,
		`[[formation]]
id = "fmn_work" # stable Formation id
type = "solo"
title = "Worker"`,
		`x_port_hint = "keep" # unknown safe port field`,
		`[[connection]]
id = "edge_start"
from = "mis_main:out"
to = "fmn_work:port_work_in"
x_route_note = "keep" # unknown safe connection field`,
		`[x_extension]
note = "keep this table byte-for-byte"`,
	}
	previous := -1
	for _, span := range retainedSpans {
		index := strings.Index(result.Board.TOML, span)
		if index < 0 {
			t.Fatalf("first-Tool migration did not retain exact source span %q:\n%s", span, result.Board.TOML)
		}
		if index <= previous {
			t.Fatalf("first-Tool migration reordered retained span %q", span)
		}
		previous = index
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

func TestCreateToolSchemaTwoExactPlacementBumpsOnceWithoutChangingConnections(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-schema-two-exact"
	boardRaw := toolAuthoringBoardFixture(slug, 9, true, toolAuthoringConnectedToolsTail())
	writeFixture(t, store.BoardPath(slug), boardRaw)
	before, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read schema-2 board: %v", err)
	}
	x, y := 901, 317

	result, err := store.CreateTool(
		slug,
		toolAuthoringCreateRequest(ToolPlacement{X: &x, Y: &y}),
		toolAuthoringAbsentOptions(before),
	)
	if err != nil {
		t.Fatalf("create Tool on schema-2 board: %v", err)
	}
	if result.Board.Schema != CurrentBoardSchema || result.Board.Rev != before.Rev+1 {
		t.Fatalf("schema-2 create schema/rev = %d/%d, want %d/%d", result.Board.Schema, result.Board.Rev, CurrentBoardSchema, before.Rev+1)
	}
	if !reflect.DeepEqual(result.Board.Connections, before.Connections) {
		t.Fatalf("schema-2 create changed existing connections:\n got %#v\nwant %#v", result.Board.Connections, before.Connections)
	}
	for _, connection := range result.Board.Connections {
		if strings.Contains(connection.From, result.Tool.ID+":") || strings.Contains(connection.To, result.Tool.ID+":") {
			t.Fatalf("schema-2 exact placement implicitly wired new Tool: %#v", connection)
		}
	}
	const retainedConnection = `[[connection]]
id = "edge_existing"
channel = "workflow"
from = "tool_source:port_source_out"
to = "tool_target:port_target_in"
x_connection_note = "keep exact"`
	if !strings.Contains(result.Board.TOML, retainedConnection) {
		t.Fatalf("schema-2 create rewrote retained connection source:\n%s", result.Board.TOML)
	}
	if result.Layout == nil || len(result.Layout.Nodes) != 1 || result.Layout.Nodes[0] != (LayoutNode{ID: result.Tool.ID, X: x, Y: y}) {
		t.Fatalf("exact absent-layout placement = %#v, want only new Tool at %d,%d", result.Layout, x, y)
	}
}

func TestCreateToolPresentLayoutPreservesAuthoritativeSpansAndRelativeOrder(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-layout-source"
	boardRaw := toolAuthoringBoardFixture(slug, 5, true, toolAuthoringConnectedToolsTail())
	layoutRaw := `schema = 1 # preserve top comment
boardId = "brd_tool-layout-source"
boardRev = 5 # preserve revision comment
updatedAt = "2026-07-19T00:00:00Z"

[[edge]]
# authoritative edge deliberately precedes nodes
id = "edge_existing"
lane = "north-by-northwest" # keep unusual lane

[[node]]
# inert node should disappear
id = "stale_node"
x = 999
y = 999

[[node]]
# retained target deliberately precedes mission and source
id = "tool_target"
x = 1400 # keep target x
y = 420

[[node]]
# retained mission comment
id = "mis_main"
x = 84
y = 196 # keep mission y

[[edge]]
# inert edge should disappear
id = "stale_edge"
lane = "stale"

[[node]]
# retained source is last among authored nodes
id = "tool_source"
x = 700
y = 420 # keep source y
`
	writeFixture(t, store.BoardPath(slug), boardRaw)
	writeFixture(t, store.LayoutPath(slug), layoutRaw)
	board, layout := toolAuthoringReadPair(t, store, slug)
	x, y := 2072, 532

	result, err := store.CreateTool(
		slug,
		toolAuthoringCreateRequest(ToolPlacement{X: &x, Y: &y}),
		toolAuthoringPresentOptions(board, layout),
	)
	if err != nil {
		t.Fatalf("create Tool in source-rich layout: %v", err)
	}
	if strings.Contains(result.Layout.TOML, `id = "stale_node"`) || strings.Contains(result.Layout.TOML, `id = "stale_edge"`) {
		t.Fatalf("source-rich layout retained inert blocks:\n%s", result.Layout.TOML)
	}
	wantNodeIDs := []string{"mis_main", "tool_source", "tool_target", result.Tool.ID}
	gotNodeIDs := make([]string, 0, len(result.Layout.Nodes))
	for _, node := range result.Layout.Nodes {
		gotNodeIDs = append(gotNodeIDs, node.ID)
	}
	sort.Strings(wantNodeIDs)
	sort.Strings(gotNodeIDs)
	if !reflect.DeepEqual(gotNodeIDs, wantNodeIDs) {
		t.Fatalf("filtered layout node ids = %v, want exact board-authoritative set %v", gotNodeIDs, wantNodeIDs)
	}
	if got := result.Layout.Edges; len(got) != 1 || got[0] != (LayoutEdge{ID: "edge_existing", Lane: "north-by-northwest"}) {
		t.Fatalf("filtered layout edges = %#v, want exact authoritative edge", got)
	}
	retainedSpans := []string{
		`[[edge]]
# authoritative edge deliberately precedes nodes
id = "edge_existing"
lane = "north-by-northwest" # keep unusual lane`,
		`[[node]]
# retained target deliberately precedes mission and source
id = "tool_target"
x = 1400 # keep target x
y = 420`,
		`[[node]]
# retained mission comment
id = "mis_main"
x = 84
y = 196 # keep mission y`,
		`[[node]]
# retained source is last among authored nodes
id = "tool_source"
x = 700
y = 420 # keep source y`,
		`[[node]]
id = "` + result.Tool.ID + `"
x = 2072
y = 532`,
	}
	previous := -1
	for _, span := range retainedSpans {
		index := strings.Index(result.Layout.TOML, span)
		if index < 0 {
			t.Fatalf("present-layout create did not retain exact source span %q:\n%s", span, result.Layout.TOML)
		}
		if index <= previous {
			t.Fatalf("present-layout create reordered retained source span %q", span)
		}
		previous = index
	}
}

func TestCreateToolFiltersOwnedLayoutSubtreesWithoutReattachingStaleDescendants(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-layout-owned-subtrees"
	boardRaw := toolAuthoringBoardFixture(slug, 3, true, "")
	layoutRaw := `schema = 1
boardId = "brd_tool-layout-owned-subtrees"
boardRev = 3

[[node]]
id = "stale_node"
x = 999
y = 999
[node.meta]
note = "stale descendant must disappear"

# leading retained comment must survive stale subtree filtering
[[node]]
id = "mis_main"
x = 140
y = 280
[node.meta]
note = "retained descendant stays exact"
[node.meta.nested]
token = "retained nested source"
`
	writeFixture(t, store.BoardPath(slug), boardRaw)
	writeFixture(t, store.LayoutPath(slug), layoutRaw)
	board, layout := toolAuthoringReadPair(t, store, slug)
	x, y := 700, 560

	result, err := store.CreateTool(
		slug,
		toolAuthoringCreateRequest(ToolPlacement{X: &x, Y: &y}),
		toolAuthoringPresentOptions(board, layout),
	)
	if err != nil {
		t.Fatalf("create Tool while filtering layout subtrees: %v", err)
	}
	if strings.Contains(result.Layout.TOML, "stale_node") || strings.Contains(result.Layout.TOML, "stale descendant must disappear") {
		t.Fatalf("stale node descendant source survived filtering:\n%s", result.Layout.TOML)
	}
	const retained = `# leading retained comment must survive stale subtree filtering
[[node]]
id = "mis_main"
x = 140
y = 280
[node.meta]
note = "retained descendant stays exact"
[node.meta.nested]
token = "retained nested source"`
	if !strings.Contains(result.Layout.TOML, retained) {
		t.Fatalf("retained layout subtree source changed:\n%s", result.Layout.TOML)
	}
}

func TestCreateToolPreservesCommentLeadingRetainedBlockAfterFlatStaleBlock(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-layout-leading-comment"
	boardRaw := toolAuthoringBoardFixture(slug, 3, true, "")
	layoutRaw := `schema = 1
boardId = "brd_tool-layout-leading-comment"
boardRev = 3

[[node]]
id = "stale_node"
x = 999
y = 999

# this comment documents the retained Mission
[[node]]
id = "mis_main"
x = 140
y = 280
`
	writeFixture(t, store.BoardPath(slug), boardRaw)
	writeFixture(t, store.LayoutPath(slug), layoutRaw)
	board, layout := toolAuthoringReadPair(t, store, slug)
	x, y := 700, 560

	result, err := store.CreateTool(
		slug,
		toolAuthoringCreateRequest(ToolPlacement{X: &x, Y: &y}),
		toolAuthoringPresentOptions(board, layout),
	)
	if err != nil {
		t.Fatalf("create Tool after flat stale layout node: %v", err)
	}
	const retained = `# this comment documents the retained Mission
[[node]]
id = "mis_main"
x = 140
y = 280`
	if !strings.Contains(result.Layout.TOML, retained) {
		t.Fatalf("stale filtering dropped retained-leading comment:\n%s", result.Layout.TOML)
	}
}

func TestCreateToolRejectsMalformedOrOrphanedOwnedLayoutSourceWithoutMutation(t *testing.T) {
	tests := []struct {
		name string
		tail string
	}{
		{name: "unclosed node array header", tail: "[[node]\nid = \"mis_main\"\nx = 1\ny = 2\n"},
		{name: "edge header trailing text", tail: "[[edge]] trailing\nid = \"edge_stale\"\nlane = \"north\"\n"},
		{name: "escaped basic-quoted node header", tail: "[[\"no\\u0064e\"\nid = \"mis_main\"\nx = 1\ny = 2\n"},
		{name: "unterminated literal-quoted edge header", tail: "[['edge\nid = \"edge_stale\"\nlane = \"north\"\n"},
		{name: "malformed unrelated header", tail: "[layout-extension\nnote = \"invalid TOML\"\n"},
		{name: "orphan node descendant", tail: "[node.meta]\nnote = \"no owning node\"\n"},
		{name: "competing edge descendant under node", tail: "[[node]]\nid = \"mis_main\"\nx = 1\ny = 2\n[edge.meta]\nnote = \"wrong owner\"\n"},
		{name: "top-level node root assignment", tail: "node = \"reserved authority root\"\n"},
		{name: "top-level edge root assignment", tail: "edge = \"reserved authority root\"\n"},
		{name: "malformed key-path assignment", tail: "\"layout\\q\" = 1\n"},
		{name: "invalid nonassignment line", tail: "this is not valid TOML\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-layout-owned-source"
			boardRaw := toolAuthoringBoardFixture(slug, 2, true, "")
			layoutRaw := "schema = 1\nboardId = \"brd_tool-layout-owned-source\"\nboardRev = 2\n\n" + test.tail
			writeFixture(t, store.BoardPath(slug), boardRaw)
			writeFixture(t, store.LayoutPath(slug), layoutRaw)
			board, layout := toolAuthoringReadPair(t, store, slug)

			_, err := store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), toolAuthoringPresentOptions(board, layout))
			if err == nil || !strings.Contains(err.Error(), "invalid_layout_owned_source") {
				t.Fatalf("malformed/orphaned owned layout error = %v, want invalid_layout_owned_source", err)
			}
			assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, &layoutRaw)
		})
	}
}

func TestCreateToolRejectsMalformedUnknownLayoutValuesWithoutMutation(t *testing.T) {
	const slug = "tool-layout-unknown-values"
	tests := []struct {
		name string
		tail string
	}{
		{name: "invalid basic string escape", tail: "x_owner = \"\\q\"\n"},
		{name: "unterminated array", tail: "x_owner = [\n"},
		{name: "duplicate unknown key", tail: "x_owner = 1\nx_owner = 2\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			boardRaw := toolAuthoringBoardFixture(slug, 2, true, "")
			layoutRaw := "schema = 1\nboardId = \"brd_tool-layout-unknown-values\"\nboardRev = 2\n" + test.tail
			writeFixture(t, store.BoardPath(slug), boardRaw)
			writeFixture(t, store.LayoutPath(slug), layoutRaw)
			board, layout := toolAuthoringReadPair(t, store, slug)

			_, err := store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), toolAuthoringPresentOptions(board, layout))
			if err == nil || !strings.Contains(err.Error(), "invalid_layout_owned_source") {
				t.Fatalf("malformed unknown layout value error = %v, want invalid_layout_owned_source", err)
			}
			assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, &layoutRaw)
		})
	}
}

func TestCreateToolRejectsMalformedUnknownBoardTOMLWithoutMutation(t *testing.T) {
	tests := []struct {
		name string
		tail string
	}{
		{name: "invalid unknown string escape", tail: "[unknown_extension]\nvalue = \"\\q\"\n"},
		{name: "unterminated unknown array", tail: "[unknown_extension]\nvalue = [\n"},
		{name: "duplicate unknown key", tail: "[unknown_extension]\nvalue = 1\nvalue = 2\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-create-malformed-source"
			boardRaw := toolAuthoringBoardFixture(slug, 2, true, test.tail)
			writeFixture(t, store.BoardPath(slug), boardRaw)
			before, err := store.ReadBoard(slug)
			if err != nil {
				t.Fatalf("inspection must expose malformed unknown source before authoring rejection: %v", err)
			}

			_, err = store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), toolAuthoringAbsentOptions(before))
			if err == nil || !strings.Contains(err.Error(), "invalid_board_source") {
				t.Fatalf("malformed unknown board source error = %v, want invalid_board_source", err)
			}
			assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, nil)
		})
	}
}

func TestCreateToolRejectsMalformedDuplicateOrCompetingLayoutIdentityFieldsWithoutMutation(t *testing.T) {
	const slug = "tool-layout-reserved-fields"
	tests := []struct {
		name   string
		layout string
	}{
		{name: "duplicate boardId first mismatched last correct", layout: "schema = 1\nboardId = \"brd_other\"\nboardId = \"brd_tool-layout-reserved-fields\"\nboardRev = 2\n"},
		{name: "duplicate schema first wrong last correct", layout: "schema = 2\nschema = 1\nboardId = \"brd_tool-layout-reserved-fields\"\nboardRev = 2\n"},
		{name: "duplicate boardRev", layout: "schema = 1\nboardId = \"brd_tool-layout-reserved-fields\"\nboardRev = 1\nboardRev = 2\n"},
		{name: "duplicate updatedAt", layout: "schema = 1\nboardId = \"brd_tool-layout-reserved-fields\"\nboardRev = 2\nupdatedAt = \"first\"\nupdatedAt = \"second\"\n"},
		{name: "missing schema", layout: "boardId = \"brd_tool-layout-reserved-fields\"\nboardRev = 2\n"},
		{name: "missing boardId", layout: "schema = 1\nboardRev = 2\n"},
		{name: "schema leading zero", layout: "schema = 01\nboardId = \"brd_tool-layout-reserved-fields\"\nboardRev = 2\n"},
		{name: "unquoted boardId", layout: "schema = 1\nboardId = brd_tool-layout-reserved-fields\nboardRev = 2\n"},
		{name: "hex boardRev", layout: "schema = 1\nboardId = \"brd_tool-layout-reserved-fields\"\nboardRev = 0x2\n"},
		{name: "unquoted updatedAt", layout: "schema = 1\nboardId = \"brd_tool-layout-reserved-fields\"\nboardRev = 2\nupdatedAt = invalid-timestamp\n"},
		{name: "dotted schema root", layout: "schema = 1\nschema.extension = 1\nboardId = \"brd_tool-layout-reserved-fields\"\nboardRev = 2\n"},
		{name: "dotted boardId root", layout: "schema = 1\nboardId = \"brd_tool-layout-reserved-fields\"\nboardId.extension = \"x\"\nboardRev = 2\n"},
		{name: "dotted boardRev root", layout: "schema = 1\nboardId = \"brd_tool-layout-reserved-fields\"\nboardRev = 2\nboardRev.extension = 3\n"},
		{name: "dotted updatedAt root", layout: "schema = 1\nboardId = \"brd_tool-layout-reserved-fields\"\nboardRev = 2\nupdatedAt.extension = \"x\"\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			boardRaw := toolAuthoringBoardFixture(slug, 2, true, "")
			writeFixture(t, store.BoardPath(slug), boardRaw)
			writeFixture(t, store.LayoutPath(slug), test.layout)
			board, layout := toolAuthoringReadPair(t, store, slug)

			_, err := store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), toolAuthoringPresentOptions(board, layout))
			if err == nil || !strings.Contains(err.Error(), "invalid_layout_identity") {
				t.Fatalf("reserved layout identity error = %v, want invalid_layout_identity", err)
			}
			layoutRaw := test.layout
			assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, &layoutRaw)
		})
	}
}

func TestCreateToolPreservesUnknownValidTopLevelLayoutFields(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-layout-unknown-root"
	boardRaw := toolAuthoringBoardFixture(slug, 2, true, "")
	layoutRaw := `schema = 1
boardId = "brd_tool-layout-unknown-root"
boardRev = 2
x_owner = { note = "keep", rank = 7 } # unknown valid root
`
	writeFixture(t, store.BoardPath(slug), boardRaw)
	writeFixture(t, store.LayoutPath(slug), layoutRaw)
	board, layout := toolAuthoringReadPair(t, store, slug)
	x, y := 700, 560

	result, err := store.CreateTool(
		slug,
		toolAuthoringCreateRequest(ToolPlacement{X: &x, Y: &y}),
		toolAuthoringPresentOptions(board, layout),
	)
	if err != nil {
		t.Fatalf("create Tool with unknown valid layout root: %v", err)
	}
	if !strings.Contains(result.Layout.TOML, `x_owner = { note = "keep", rank = 7 } # unknown valid root`) {
		t.Fatalf("unknown valid top-level layout field changed:\n%s", result.Layout.TOML)
	}
	if count := strings.Count(result.Layout.TOML, "[[node]]"); count != 1 {
		t.Fatalf("created layout node count = %d, want exactly one:\n%s", count, result.Layout.TOML)
	}
}

func TestCreateToolProjectsRetainedLiteralStringLayoutIdentity(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-layout-literal-identity"
	boardRaw := toolAuthoringBoardFixture(slug, 2, true, toolAuthoringConnectedToolsTail())
	layoutRaw := `schema = 1
boardId = "brd_tool-layout-literal-identity"
boardRev = 2

[[node]]
id = 'mis_main'
x = 112
y = 224

[[edge]]
id = 'edge_existing'
lane = 'review'
`
	writeFixture(t, store.BoardPath(slug), boardRaw)
	writeFixture(t, store.LayoutPath(slug), layoutRaw)
	board, layout := toolAuthoringReadPair(t, store, slug)

	result, err := store.CreateTool(
		slug,
		toolAuthoringCreateRequest(ToolPlacement{}),
		toolAuthoringPresentOptions(board, layout),
	)
	if err != nil {
		t.Fatalf("create Tool with literal-string layout identity: %v", err)
	}
	if _, ok := toolAuthoringLayoutNode(result.Layout.Nodes, "mis_main"); !ok {
		t.Fatalf("retained literal-string node identity was not projected: %#v", result.Layout.Nodes)
	}
	if len(result.Layout.Edges) != 1 || result.Layout.Edges[0].ID != "edge_existing" || result.Layout.Edges[0].Lane != "review" {
		t.Fatalf("retained literal-string edge projection = %#v", result.Layout.Edges)
	}
	for _, retained := range []string{"id = 'mis_main'", "id = 'edge_existing'", "lane = 'review'"} {
		if !strings.Contains(result.Layout.TOML, retained) {
			t.Fatalf("literal-string layout source changed; missing %q:\n%s", retained, result.Layout.TOML)
		}
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

func TestCreateToolOwnsDescriptorOrderedGloballyUniqueIDsAcrossCreates(t *testing.T) {
	store := newToolAuthoringStore(t)
	type generatedID struct {
		prefix string
		id     string
	}
	sequence := []generatedID{
		{prefix: "tool", id: "tool_first"},
		{prefix: "port", id: "port_first_input"},
		{prefix: "port", id: "port_first_output"},
		{prefix: "tool", id: "tool_second"},
		{prefix: "port", id: "port_second_input"},
		{prefix: "port", id: "port_second_output"},
	}
	generated := 0
	store.newToolDefinitionID = func(prefix string) string {
		if generated >= len(sequence) {
			t.Fatalf("Tool create exhausted the approved id sequence with extra prefix %q", prefix)
		}
		next := sequence[generated]
		if prefix != next.prefix {
			t.Fatalf("generated id call %d prefix = %q, want descriptor-ordered %q", generated, prefix, next.prefix)
		}
		generated++
		return next.id
	}
	slug := "tool-id-ownership"
	writeFixture(t, store.BoardPath(slug), toolAuthoringBoardFixture(slug, 2, true, ""))
	board, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read board: %v", err)
	}

	first, err := store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), toolAuthoringAbsentOptions(board))
	if err != nil {
		t.Fatalf("create first Tool with forced ids: %v", err)
	}
	if first.Tool.ID != "tool_first" || len(first.Tool.Inputs) != 1 || first.Tool.Inputs[0].ID != "port_first_input" ||
		len(first.Tool.Outputs) != 1 || first.Tool.Outputs[0].ID != "port_first_output" {
		t.Fatalf("first Tool descriptor-order ids = %#v", first.Tool)
	}
	second, err := store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), toolAuthoringPresentOptions(first.Board, first.Layout))
	if err != nil {
		t.Fatalf("create second Tool with forced ids: %v", err)
	}
	if second.Tool.ID != "tool_second" || len(second.Tool.Inputs) != 1 || second.Tool.Inputs[0].ID != "port_second_input" ||
		len(second.Tool.Outputs) != 1 || second.Tool.Outputs[0].ID != "port_second_output" {
		t.Fatalf("second Tool descriptor-order ids = %#v", second.Tool)
	}
	if generated != len(sequence) {
		t.Fatalf("Tool creates consumed %d generated ids, want exact sequence length %d", generated, len(sequence))
	}

	seen := map[string]string{"mis_main": "Mission"}
	for _, tool := range second.Board.Tools {
		if owner, exists := seen[tool.ID]; exists {
			t.Fatalf("Tool id %q duplicates %s", tool.ID, owner)
		}
		seen[tool.ID] = "Tool"
		for _, port := range append(append([]ToolPort(nil), tool.Inputs...), tool.Outputs...) {
			if owner, exists := seen[port.ID]; exists {
				t.Fatalf("Tool port id %q duplicates %s", port.ID, owner)
			}
			seen[port.ID] = "Tool port"
		}
	}
	if len(seen) != 7 {
		t.Fatalf("board-global id set = %v, want Mission plus two Tools and four ports", seen)
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

func TestCreateToolRejectsExpectedPresentWhenLayoutIsActuallyAbsent(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-create-actual-layout-absent"
	boardRaw := toolAuthoringBoardFixture(slug, 2, true, "")
	writeFixture(t, store.BoardPath(slug), boardRaw)
	board, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read board with absent layout: %v", err)
	}
	opts := toolAuthoringAbsentOptions(board)
	opts.Layout = &LayoutWriteExpectation{State: LayoutWritePresent, ETag: strings.Repeat("a", 64)}

	_, err = store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), opts)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("actual-absent/expected-present error = %v, want ErrConflict", err)
	}
	assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, nil)
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

func TestCreateToolRejectsUpdatedByThatCannotRoundTripAsTOMLWithoutMutation(t *testing.T) {
	tests := []struct {
		name      string
		updatedBy string
	}{
		{name: "Go bell escape", updatedBy: "agent:\a"},
		{name: "Go vertical-tab escape", updatedBy: "agent:\v"},
		{name: "Go hex escape", updatedBy: "agent:" + string([]byte{0x01})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-invalid-updated-by"
			boardRaw := toolAuthoringBoardFixture(slug, 2, true, "")
			writeFixture(t, store.BoardPath(slug), boardRaw)
			board, err := store.ReadBoard(slug)
			if err != nil {
				t.Fatalf("read board: %v", err)
			}
			req := toolAuthoringCreateRequest(ToolPlacement{})
			req.UpdatedBy = test.updatedBy

			_, err = store.CreateTool(slug, req, toolAuthoringAbsentOptions(board))
			if err == nil || !strings.Contains(err.Error(), "invalid_tool_updated_by") {
				t.Fatalf("non-TOML UpdatedBy error = %v, want invalid_tool_updated_by", err)
			}
			assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, nil)
		})
	}
}

func TestCreateToolRejectsOutOfSigned32BitExactCoordinatesWithoutMutation(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("host int cannot represent the out-of-range exact-coordinate contract cases")
	}
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	tests := []struct {
		name string
		x    int
		y    int
	}{
		{name: "maximum host int x", x: maxInt, y: 112},
		{name: "minimum host int y", x: 112, y: minInt},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-exact-coordinate-range"
			boardRaw := toolAuthoringBoardFixture(slug, 2, true, "")
			writeFixture(t, store.BoardPath(slug), boardRaw)
			board, err := store.ReadBoard(slug)
			if err != nil {
				t.Fatalf("read board: %v", err)
			}

			_, err = store.CreateTool(
				slug,
				toolAuthoringCreateRequest(ToolPlacement{X: &test.x, Y: &test.y}),
				toolAuthoringAbsentOptions(board),
			)
			if err == nil || !strings.Contains(err.Error(), "invalid_layout_coordinate") {
				t.Fatalf("out-of-range exact coordinate error = %v, want invalid_layout_coordinate", err)
			}
			assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, nil)
		})
	}
}

func TestCreateToolAcceptsInclusiveSigned32BitExactCoordinateBoundary(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-exact-coordinate-boundary"
	writeFixture(t, store.BoardPath(slug), toolAuthoringBoardFixture(slug, 2, true, ""))
	board, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	x, y := int(1<<31-1), int(-1<<31)

	result, err := store.CreateTool(
		slug,
		toolAuthoringCreateRequest(ToolPlacement{X: &x, Y: &y}),
		toolAuthoringAbsentOptions(board),
	)
	if err != nil {
		t.Fatalf("create Tool at inclusive signed-32 boundary: %v", err)
	}
	if result.Layout == nil || len(result.Layout.Nodes) != 1 || result.Layout.Nodes[0] != (LayoutNode{ID: result.Tool.ID, X: x, Y: y}) {
		t.Fatalf("signed-32 boundary placement = %#v", result.Layout)
	}
}

func TestCreateToolRejectsOutOfSigned32BitPersistedCoordinatesWithoutMutation(t *testing.T) {
	tests := []struct {
		name       string
		coordinate string
	}{
		{name: "x above maximum", coordinate: "x = 2147483648\ny = 112"},
		{name: "y below minimum", coordinate: "x = 112\ny = -2147483649"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-persisted-coordinate-range"
			boardRaw := toolAuthoringBoardFixture(slug, 2, true, "")
			layoutRaw := "schema = 1\nboardId = \"brd_tool-persisted-coordinate-range\"\nboardRev = 2\n\n[[node]]\nid = \"mis_main\"\n" + test.coordinate + "\n"
			writeFixture(t, store.BoardPath(slug), boardRaw)
			writeFixture(t, store.LayoutPath(slug), layoutRaw)
			board, layout := toolAuthoringReadPair(t, store, slug)

			_, err := store.CreateTool(
				slug,
				toolAuthoringCreateRequest(ToolPlacement{PredecessorNodeID: "mis_main"}),
				toolAuthoringPresentOptions(board, layout),
			)
			if err == nil || !strings.Contains(err.Error(), "invalid_layout_coordinate") {
				t.Fatalf("out-of-range persisted coordinate error = %v, want invalid_layout_coordinate", err)
			}
			if errors.Is(err, ErrInvalidToolMutation) {
				t.Fatalf("out-of-range persisted coordinate error %v was classified as ErrInvalidToolMutation", err)
			}
			if errors.Is(err, ErrConflict) {
				t.Fatalf("out-of-range persisted coordinate error %v was classified as ErrConflict", err)
			}
			assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, &layoutRaw)
		})
	}
}

func TestCreateToolProjectsFullTOMLIntegerCoordinatesWithoutRewritingSource(t *testing.T) {
	tests := []struct {
		name       string
		coordinate string
		wantX      int
	}{
		{name: "hexadecimal", coordinate: "x = 0x70\ny = 112", wantX: 112},
		{name: "underscored", coordinate: "x = 1_12\ny = 112", wantX: 112},
		{name: "explicit plus", coordinate: "x = +112\ny = 112", wantX: 112},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-persisted-coordinate-decimal"
			boardRaw := toolAuthoringBoardFixture(slug, 2, true, "")
			layoutRaw := "schema = 1\nboardId = \"brd_tool-persisted-coordinate-decimal\"\nboardRev = 2\n\n[[node]]\nid = \"mis_main\"\n" + test.coordinate + "\n"
			writeFixture(t, store.BoardPath(slug), boardRaw)
			writeFixture(t, store.LayoutPath(slug), layoutRaw)
			board, layout := toolAuthoringReadPair(t, store, slug)

			result, err := store.CreateTool(
				slug,
				toolAuthoringCreateRequest(ToolPlacement{PredecessorNodeID: "mis_main"}),
				toolAuthoringPresentOptions(board, layout),
			)
			if err != nil {
				t.Fatalf("create Tool from valid TOML integer coordinates: %v", err)
			}
			retained, found := toolAuthoringLayoutNode(result.Layout.Nodes, "mis_main")
			if !found || retained.X != test.wantX || retained.Y != 112 {
				t.Fatalf("projected retained coordinate = %#v found=%t, want %d,112", retained, found, test.wantX)
			}
			if !strings.Contains(result.Layout.TOML, test.coordinate) {
				t.Fatalf("valid TOML coordinate source was rewritten:\n%s", result.Layout.TOML)
			}
		})
	}
}

func TestCreateToolHeuristicUsesCheckedMathAtSigned32PersistedBoundaries(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-persisted-coordinate-boundary"
	boardRaw := toolAuthoringBoardFixture(slug, 4, true,
		toolStructuralJSONNormalizeToolBlock("tool_existing", "Existing", "port_existing_in", "port_existing_out"))
	layoutRaw := `schema = 1
boardId = "brd_tool-persisted-coordinate-boundary"
boardRev = 4

[[node]]
id = "mis_main"
x = 2147483647
y = -2147483648

[[node]]
id = "tool_existing"
x = -2147483648
y = 2147483647
`
	writeFixture(t, store.BoardPath(slug), boardRaw)
	writeFixture(t, store.LayoutPath(slug), layoutRaw)
	board, layout := toolAuthoringReadPair(t, store, slug)

	result, err := store.CreateTool(
		slug,
		toolAuthoringCreateRequest(ToolPlacement{PredecessorNodeID: "mis_main", SuccessorNodeID: "tool_existing"}),
		toolAuthoringPresentOptions(board, layout),
	)
	if err != nil {
		t.Fatalf("create Tool from signed-32 boundary midpoint: %v", err)
	}
	position, found := toolAuthoringLayoutNode(result.Layout.Nodes, result.Tool.ID)
	if !found || position.X != 112 || position.Y != 112 {
		t.Fatalf("signed-32 boundary midpoint = %#v found=%t, want clamped 112,112", position, found)
	}
}

func TestCreateToolRejectsHeuristicStepOutsideSigned32WithoutMutation(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-persisted-coordinate-step"
	boardRaw := toolAuthoringBoardFixture(slug, 2, true, "")
	layoutRaw := `schema = 1
boardId = "brd_tool-persisted-coordinate-step"
boardRev = 2

[[node]]
id = "mis_main"
x = 2147483647
y = 112
`
	writeFixture(t, store.BoardPath(slug), boardRaw)
	writeFixture(t, store.LayoutPath(slug), layoutRaw)
	board, layout := toolAuthoringReadPair(t, store, slug)

	_, err := store.CreateTool(
		slug,
		toolAuthoringCreateRequest(ToolPlacement{PredecessorNodeID: "mis_main"}),
		toolAuthoringPresentOptions(board, layout),
	)
	if err == nil || !strings.Contains(err.Error(), "invalid_layout_coordinate") {
		t.Fatalf("out-of-range heuristic step error = %v, want invalid_layout_coordinate", err)
	}
	if !errors.Is(err, ErrInvalidToolMutation) {
		t.Fatalf("out-of-range heuristic step error = %v, want errors.Is(ErrInvalidToolMutation)", err)
	}
	if errors.Is(err, ErrConflict) {
		t.Fatalf("out-of-range heuristic step error %v was classified as ErrConflict", err)
	}
	assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, &layoutRaw)
}

func TestCreateToolNoFreeHeuristicPositionRemainsConflict(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-no-free-position"
	var boardTail strings.Builder
	var layoutRaw strings.Builder
	layoutRaw.WriteString("schema = 1\nboardId = \"brd_" + slug + "\"\nboardRev = 5\n")
	x, y := layoutPlacementMin, layoutPlacementMin
	for index := 0; index < layoutPlacementMaxAttempts; index++ {
		toolID := fmt.Sprintf("tool_occupied_%02d", index)
		boardTail.WriteString(toolStructuralJSONNormalizeToolBlock(
			toolID,
			fmt.Sprintf("Occupied %02d", index),
			fmt.Sprintf("port_occupied_%02d_in", index),
			fmt.Sprintf("port_occupied_%02d_out", index),
		))
		fmt.Fprintf(&layoutRaw, "\n[[node]]\nid = %q\nx = %d\ny = %d\n", toolID, x, y)
		x += layoutPlacementStep
		if x > layoutPlacementWrapX {
			x = layoutPlacementMin
			y += layoutPlacementStep
		}
	}
	boardRaw := toolAuthoringBoardFixture(slug, 5, false, boardTail.String())
	layout := layoutRaw.String()
	writeFixture(t, store.BoardPath(slug), boardRaw)
	writeFixture(t, store.LayoutPath(slug), layout)
	board, layoutDocument := toolAuthoringReadPair(t, store, slug)

	_, err := store.CreateTool(
		slug,
		toolAuthoringCreateRequest(ToolPlacement{}),
		toolAuthoringPresentOptions(board, layoutDocument),
	)
	if err == nil || !errors.Is(err, ErrConflict) {
		t.Fatalf("exhausted heuristic placement error = %v, want ErrConflict", err)
	}
	if errors.Is(err, ErrInvalidToolMutation) {
		t.Fatalf("exhausted heuristic placement error %v was classified as ErrInvalidToolMutation", err)
	}
	assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, &layout)
}

func TestCreateToolPreservesOpaqueEdgeFieldsNamedXAndY(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-layout-edge-extension"
	boardRaw := toolAuthoringBoardFixture(slug, 5, true, toolAuthoringConnectedToolsTail())
	layoutRaw := `schema = 1
boardId = "brd_tool-layout-edge-extension"
boardRev = 5

[[edge]]
id = "edge_existing"
lane = "north"
x = "opaque edge extension"
y = [1, 2, 3]
`
	writeFixture(t, store.BoardPath(slug), boardRaw)
	writeFixture(t, store.LayoutPath(slug), layoutRaw)
	board, layout := toolAuthoringReadPair(t, store, slug)
	x, y := 700, 560

	result, err := store.CreateTool(
		slug,
		toolAuthoringCreateRequest(ToolPlacement{X: &x, Y: &y}),
		toolAuthoringPresentOptions(board, layout),
	)
	if err != nil {
		t.Fatalf("create Tool beside edge x/y extensions: %v", err)
	}
	const retained = `[[edge]]
id = "edge_existing"
lane = "north"
x = "opaque edge extension"
y = [1, 2, 3]`
	if !strings.Contains(result.Layout.TOML, retained) {
		t.Fatalf("edge x/y extension source changed:\n%s", result.Layout.TOML)
	}
}

func TestCreateToolLeavesMissingRetainedCoordinatesAbsent(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-missing-retained-coordinates"
	boardRaw := toolAuthoringBoardFixture(slug, 2, true, "")
	layoutRaw := `schema = 1
boardId = "brd_tool-missing-retained-coordinates"
boardRev = 2

[[node]]
id = "mis_main"
# x and y are deliberately absent
`
	writeFixture(t, store.BoardPath(slug), boardRaw)
	writeFixture(t, store.LayoutPath(slug), layoutRaw)
	board, layout := toolAuthoringReadPair(t, store, slug)
	x, y := 500, 600

	result, err := store.CreateTool(
		slug,
		toolAuthoringCreateRequest(ToolPlacement{X: &x, Y: &y}),
		toolAuthoringPresentOptions(board, layout),
	)
	if err != nil {
		t.Fatalf("create Tool beside retained node with missing coordinates: %v", err)
	}
	const retained = `[[node]]
id = "mis_main"
# x and y are deliberately absent`
	if !strings.Contains(result.Layout.TOML, retained) {
		t.Fatalf("create filled or rewrote missing retained coordinates:\n%s", result.Layout.TOML)
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
		name       string
		layout     string
		wantMarker string
	}{
		{name: "board mismatch", wantMarker: ErrConflict.Error(), layout: "schema = 1\nboardId = \"brd_other\"\nboardRev = 2\n"},
		{name: "missing node id", wantMarker: "invalid_layout_id", layout: "schema = 1\nboardId = \"brd_tool-layout-invalid\"\nboardRev = 2\n\n[[node]]\nx = 1\ny = 2\n"},
		{name: "malformed node id", wantMarker: "invalid_layout_id", layout: "schema = 1\nboardId = \"brd_tool-layout-invalid\"\nboardRev = 2\n\n[[node]]\nid = []\nx = 1\ny = 2\n"},
		{name: "missing edge id", wantMarker: "invalid_layout_id", layout: "schema = 1\nboardId = \"brd_tool-layout-invalid\"\nboardRev = 2\n\n[[edge]]\nlane = \"a\"\n"},
		{name: "malformed edge id", wantMarker: "invalid_layout_id", layout: "schema = 1\nboardId = \"brd_tool-layout-invalid\"\nboardRev = 2\n\n[[edge]]\nid = []\nlane = \"a\"\n"},
		{name: "duplicate node id", wantMarker: "duplicate_layout_id", layout: "schema = 1\nboardId = \"brd_tool-layout-invalid\"\nboardRev = 2\n\n[[node]]\nid = \"mis_main\"\nx = 1\ny = 2\n\n[[node]]\nid = \"mis_main\"\nx = 3\ny = 4\n"},
		{name: "duplicate edge id", wantMarker: "duplicate_layout_id", layout: "schema = 1\nboardId = \"brd_tool-layout-invalid\"\nboardRev = 2\n\n[[edge]]\nid = \"edge_stale\"\nlane = \"a\"\n\n[[edge]]\nid = \"edge_stale\"\nlane = \"b\"\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-layout-invalid"
			boardRaw := toolAuthoringBoardFixture(slug, 2, true, "")
			writeFixture(t, store.BoardPath(slug), boardRaw)
			writeFixture(t, store.LayoutPath(slug), test.layout)
			board, layout := toolAuthoringReadPair(t, store, slug)
			_, err := store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), toolAuthoringPresentOptions(board, layout))
			if err == nil {
				t.Fatal("invalid layout accepted")
			}
			if !strings.Contains(err.Error(), test.wantMarker) {
				t.Fatalf("invalid layout error = %v, want marker %q", err, test.wantMarker)
			}
			layoutRaw := test.layout
			assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, &layoutRaw)
		})
	}
}

func TestCreateToolRejectsStructuralInvalidBoardsWithoutMutation(t *testing.T) {
	validTool := toolStructuralJSONNormalizeToolBlock("tool_existing", "Existing", "port_existing_in", "port_existing_out")
	tests := []struct {
		name       string
		tail       string
		wantMarker string
	}{
		{name: "empty node id", wantMarker: "invalid_node_id", tail: `[[gate]]
id = ""
title = "Broken"
kinds = ["human"]
criterion = "Confirm"
`},
		{name: "duplicate node id", wantMarker: FindingDuplicateNodeID, tail: validTool + `[[gate]]
id = "tool_existing"
title = "Duplicate"
kinds = ["human"]
criterion = "Confirm"
`},
		{name: "duplicate port id", wantMarker: "duplicate_port_id", tail: strings.Replace(validTool, `id = "port_existing_out"`, `id = "port_existing_in"`, 1)},
		{name: "empty edge id", wantMarker: "invalid_edge_id", tail: validTool + `[[connection]]
id = ""
channel = "workflow"
from = "tool_existing:port_existing_out"
to = "tool_existing:port_existing_in"
`},
		{name: "dangling edge", wantMarker: FindingDanglingConnection, tail: validTool + `[[connection]]
id = "edge_dangling"
channel = "workflow"
from = "tool_existing:port_existing_out"
to = "ghost:input"
`},
		{name: "incompatible media", wantMarker: FindingIncompatibleMedia, tail: validTool + `[[connection]]
id = "edge_media"
channel = "workflow"
from = "mis_main:out"
to = "tool_existing:port_existing_in"
`},
		{name: "incompatible payload kind", wantMarker: FindingIncompatiblePayloadKind, tail: validTool + `[[gate]]
id = "gate_feedback"
title = "Feedback"
kinds = ["human"]
criterion = "Confirm"

[[connection]]
id = "edge_kind"
channel = "workflow"
from = "gate_feedback:fail"
to = "tool_existing:port_existing_in"
`},
		{name: "Tool judge misuse", wantMarker: FindingInvalidJudgeRelationship, tail: validTool + `[[gate]]
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
		{name: "duplicate edge id", wantMarker: "duplicate_edge_id", tail: validTool +
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
		{name: "duplicate producer", wantMarker: FindingDuplicateInputProducer, tail: validTool +
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
			_, err = store.CreateTool(slug, toolAuthoringCreateRequest(ToolPlacement{}), toolAuthoringAbsentOptions(board))
			if err == nil {
				t.Fatal("structural-invalid board accepted Tool mutation")
			}
			if !strings.Contains(err.Error(), test.wantMarker) {
				t.Fatalf("structural rejection = %v, want marker %q", err, test.wantMarker)
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

func toolAuthoringConnectedToolsTail() string {
	return toolStructuralJSONNormalizeToolBlock("tool_source", "Source", "port_source_in", "port_source_out") +
		toolStructuralJSONNormalizeToolBlock("tool_target", "Target", "port_target_in", "port_target_out") + `[[connection]]
id = "edge_existing"
channel = "workflow"
from = "tool_source:port_source_out"
to = "tool_target:port_target_in"
x_connection_note = "keep exact"
`
}

func toolAuthoringCoreNodeIDs(board *BoardDocument) []string {
	ids := make([]string, 0, len(board.Missions)+len(board.Formations)+len(board.Gates))
	for _, mission := range board.Missions {
		ids = append(ids, "Mission:"+mission.ID)
	}
	for _, formation := range board.Formations {
		ids = append(ids, "Formation:"+formation.ID)
	}
	for _, gate := range board.Gates {
		ids = append(ids, "Gate:"+gate.ID)
	}
	return ids
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
