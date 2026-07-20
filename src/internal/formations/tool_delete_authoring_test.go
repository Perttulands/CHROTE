package formations

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestDeleteToolPresentLayoutRemovesOwnedDefinitionAndFiltersLayoutAuthority(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-delete-present"
	boardRaw := toolDeleteConnectedBoardFixture(slug, 7)
	boardRaw = strings.Replace(
		boardRaw,
		"schema = 2\n",
		"schema = 2 # preserve schema source\nx_board_owner = 'keep exact' # preserve root extension\n",
		1,
	)
	if err := validateToolMutationBoardSource([]byte(boardRaw)); err != nil {
		t.Fatalf("nested connection delete fixture is not valid TOML: %v", err)
	}
	layoutRaw := `schema = 1 # preserve schema comment
boardId = "brd_tool-delete-present"
boardRev = 7 # preserve revision comment
updatedAt = "2026-07-19T00:00:00Z" # preserve timestamp comment
x_layout_owner = "keep"

# the unrelated authoritative edge deliberately precedes nodes
[[edge]]
id = "edge_keep"
lane = "north-by-northwest" # preserve exact lane

# deleted Tool node
[[node]]
id = "tool_target"
x = 420
y = 421

# incoming incident edge
[[edge]]
id = "edge_into_target"
lane = "delete incoming"

# outgoing incident edge
[[edge]]
id = "edge_from_target"
lane = "delete outgoing"

# stale authority unrelated to the deletion
[[node]]
id = "stale_node"
x = 900
y = 901

[[edge]]
id = "stale_edge"
lane = "discard"

# retained Mission coordinate source
[[node]]
id = "mis_main"
x = 0x70
y = +224

# retained source Tool coordinate source
[[node]]
id = "tool_source"
x = 1_12
y = 336

# retained sink stays intentionally unplaced
[[node]]
id = "tool_sink"
# coordinates deliberately absent

# retained independent Tool coordinate source
[[node]]
id = "tool_keep"
x = 728
y = 0x1c0

[layout_extension]
note = "preserve this table exactly"
`
	writeFixture(t, store.BoardPath(slug), boardRaw)
	writeFixture(t, store.LayoutPath(slug), layoutRaw)
	beforeBoard, beforeLayout := toolAuthoringReadPair(t, store, slug)

	result, err := store.DeleteTool(
		slug,
		ToolDeleteRequest{ID: "tool_target", UpdatedBy: "agent:delete-test"},
		toolAuthoringPresentOptions(beforeBoard, beforeLayout),
	)
	if err != nil {
		t.Fatalf("delete Tool with present layout: %v", err)
	}
	if result == nil || result.Board == nil || result.Layout == nil {
		t.Fatalf("present-layout delete result = %#v, want board and layout", result)
	}
	if result.ToolID != "tool_target" {
		t.Fatalf("deleted Tool id = %q, want tool_target", result.ToolID)
	}
	if result.Board.Schema != CurrentBoardSchema {
		t.Fatalf("Tool delete changed schema = %d, want %d", result.Board.Schema, CurrentBoardSchema)
	}
	if result.Board.Rev != beforeBoard.Rev+1 || result.Layout.BoardRev != result.Board.Rev {
		t.Fatalf(
			"paired delete revisions = board %d layout %d, want %d",
			result.Board.Rev,
			result.Layout.BoardRev,
			beforeBoard.Rev+1,
		)
	}
	if result.Board.UpdatedBy != "agent:delete-test" ||
		result.Board.UpdatedAt != "2026-07-20T08:30:00Z" ||
		result.Layout.UpdatedAt != result.Board.UpdatedAt {
		t.Fatalf("paired delete provenance = board %#v layout %#v", result.Board, result.Layout)
	}

	if _, found := toolNodeByID(result.Board, "tool_target"); found {
		t.Fatalf("deleted Tool remains in board: %#v", result.Board.Tools)
	}
	for _, id := range []string{"tool_source", "tool_sink", "tool_keep"} {
		if _, found := toolNodeByID(result.Board, id); !found {
			t.Fatalf("delete removed retained Tool %q: %#v", id, result.Board.Tools)
		}
	}
	if len(result.Board.Connections) != 1 ||
		result.Board.Connections[0].ID != "edge_keep" ||
		result.Board.Connections[0].From != "tool_source:port_source_out" ||
		result.Board.Connections[0].To != "tool_keep:port_keep_in" {
		t.Fatalf("delete retained connections = %#v, want only edge_keep", result.Board.Connections)
	}
	for _, deletedSource := range []string{
		`id = "tool_target"`,
		`id = "port_target_in"`,
		`id = "port_target_out"`,
		`id = "edge_into_target"`,
		`id = "edge_from_target"`,
		`marker = "delete incoming subtree"`,
		`marker = "delete outgoing subtree"`,
	} {
		if strings.Contains(result.Board.TOML, deletedSource) {
			t.Fatalf("delete retained owned target source %q:\n%s", deletedSource, result.Board.TOML)
		}
	}
	retainedBoardSpans := []string{
		"schema = 2 # preserve schema source\n" +
			"x_board_owner = 'keep exact' # preserve root extension",
		toolStructuralJSONNormalizeToolBlock("tool_source", "Source", "port_source_in", "port_source_out"),
		toolStructuralJSONNormalizeToolBlock("tool_sink", "Sink", "port_sink_in", "port_sink_out"),
		toolStructuralJSONNormalizeToolBlock("tool_keep", "Keep", "port_keep_in", "port_keep_out"),
		`[[connection]]
id = "edge_keep"
channel = "workflow"
from = "tool_source:port_source_out"
to = "tool_keep:port_keep_in"
x_connection_note = "preserve exact"

[connection.metadata]
note = 'retain nested exact'

[connection.metadata.route]
lane = "center"`,
	}
	for _, span := range retainedBoardSpans {
		if !strings.Contains(result.Board.TOML, span) {
			t.Fatalf("delete changed retained board source %q:\n%s", span, result.Board.TOML)
		}
	}

	if got := toolDeleteLayoutIDs(result.Layout.Nodes); !reflect.DeepEqual(got, []string{"mis_main", "tool_source", "tool_sink", "tool_keep"}) {
		t.Fatalf("retained layout nodes = %v, want only authoritative retained nodes in source order", got)
	}
	if got := toolDeleteLayoutEdgeIDs(result.Layout.Edges); !reflect.DeepEqual(got, []string{"edge_keep"}) {
		t.Fatalf("retained layout edges = %v, want only edge_keep", got)
	}
	for _, deletedSource := range []string{
		`id = "tool_target"`,
		`id = "edge_into_target"`,
		`id = "edge_from_target"`,
		`id = "stale_node"`,
		`id = "stale_edge"`,
	} {
		if strings.Contains(result.Layout.TOML, deletedSource) {
			t.Fatalf("delete retained absent layout authority %q:\n%s", deletedSource, result.Layout.TOML)
		}
	}
	retainedLayoutSpans := []string{
		`# the unrelated authoritative edge deliberately precedes nodes
[[edge]]
id = "edge_keep"
lane = "north-by-northwest" # preserve exact lane`,
		`# retained Mission coordinate source
[[node]]
id = "mis_main"
x = 0x70
y = +224`,
		`# retained source Tool coordinate source
[[node]]
id = "tool_source"
x = 1_12
y = 336`,
		`# retained sink stays intentionally unplaced
[[node]]
id = "tool_sink"
# coordinates deliberately absent`,
		`# retained independent Tool coordinate source
[[node]]
id = "tool_keep"
x = 728
y = 0x1c0`,
		`[layout_extension]
note = "preserve this table exactly"`,
	}
	previous := -1
	for _, span := range retainedLayoutSpans {
		index := strings.Index(result.Layout.TOML, span)
		if index < 0 {
			t.Fatalf("delete did not preserve exact retained layout span %q:\n%s", span, result.Layout.TOML)
		}
		if index <= previous {
			t.Fatalf("delete reordered retained layout span %q", span)
		}
		previous = index
	}
	if !strings.Contains(result.Layout.TOML, `boardRev = 8 # preserve revision comment`) ||
		!strings.Contains(result.Layout.TOML, `updatedAt = "2026-07-20T08:30:00Z" # preserve timestamp comment`) {
		t.Fatalf("delete did not advance layout identity in place:\n%s", result.Layout.TOML)
	}
	for _, span := range []string{
		"schema = 1 # preserve schema comment",
		`x_layout_owner = "keep"`,
	} {
		if !strings.Contains(result.Layout.TOML, span) {
			t.Fatalf("delete changed untouched layout root source %q:\n%s", span, result.Layout.TOML)
		}
	}
	assertToolDeletePersistedResult(t, store, slug, result)
}

func TestDeleteToolFindsLiteralStringIDAndDeletingLastToolNeverDowngradesSchema(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-delete-literal-id"
	boardRaw := toolAuthoringBoardFixture(slug, 3, true, toolUpdateTargetBlock())
	boardRaw = strings.Replace(
		boardRaw,
		`id = "tool_target" # immutable generated Tool id`,
		`id = 'tool_target' # immutable generated Tool id`,
		1,
	)
	writeFixture(t, store.BoardPath(slug), boardRaw)
	before, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read literal-string Tool source: %v", err)
	}

	result, err := store.DeleteTool(
		slug,
		ToolDeleteRequest{ID: "tool_target", UpdatedBy: "agent:delete-test"},
		toolAuthoringAbsentOptions(before),
	)
	if err != nil {
		t.Fatalf("delete literal-string Tool id: %v", err)
	}
	if result.ToolID != "tool_target" || len(result.Board.Tools) != 0 {
		t.Fatalf("last-Tool delete result = %#v", result)
	}
	if result.Board.Schema != CurrentBoardSchema || !strings.Contains(result.Board.TOML, "schema = 2\n") {
		t.Fatalf("last-Tool delete downgraded schema:\n%s", result.Board.TOML)
	}
	if strings.Contains(result.Board.TOML, "[[tool]]") ||
		strings.Contains(result.Board.TOML, "[tool.params]") ||
		strings.Contains(result.Board.TOML, "[[tool.input]]") ||
		strings.Contains(result.Board.TOML, "[[tool.output]]") ||
		strings.Contains(result.Board.TOML, "tool_target") ||
		strings.Contains(result.Board.TOML, "port_target_") {
		t.Fatalf("literal-id delete retained Tool-owned source:\n%s", result.Board.TOML)
	}
	if result.Layout != nil {
		t.Fatalf("last-Tool absent-layout delete returned layout %#v", result.Layout)
	}
	assertToolDeletePersistedResult(t, store, slug, result)
}

func TestDeleteToolAllowsDocumentedDraftGapsAndKeepsAbsentLayoutAbsent(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-delete-draft"
	boardRaw := toolAuthoringBoardFixture(
		slug,
		2,
		false,
		toolUpdateTargetBlock()+
			toolStructuralJSONNormalizeToolBlock("tool_survivor", "Survivor", "port_survivor_in", "port_survivor_out")+
			`[[gate]]
id = "gate_unroutable"
title = "Incomplete review"
kinds = ["code"]
criterion = ""
`,
	)
	writeFixture(t, store.BoardPath(slug), boardRaw)
	before, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read draft delete source: %v", err)
	}

	result, err := store.DeleteTool(
		slug,
		ToolDeleteRequest{ID: "tool_target"},
		toolAuthoringAbsentOptions(before),
	)
	if err != nil {
		t.Fatalf("delete Tool across documented draft gaps: %v", err)
	}
	if result.Layout != nil || len(result.Board.Missions) != 0 || len(result.Board.Gates) != 1 || len(result.Board.Connections) != 0 {
		t.Fatalf("draft delete changed documented structural gaps: %#v", result)
	}
	survivor, found := toolNodeByID(result.Board, "tool_survivor")
	if !found || len(survivor.Inputs) != 1 || survivor.Inputs[0].Required == nil || !*survivor.Inputs[0].Required {
		t.Fatalf("draft delete did not retain unwired required Tool input: %#v", survivor)
	}
	if _, err := os.Lstat(store.LayoutPath(slug)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("draft delete materialized absent layout: %v", err)
	}
	assertToolDeletePersistedResult(t, store, slug, result)
}

func TestDeleteToolRejectsInvalidRequestTargetOrBoardWithoutMutation(t *testing.T) {
	tests := []struct {
		name       string
		board      func(string) string
		request    ToolDeleteRequest
		wantIs     error
		wantMarker string
	}{
		{
			name:    "missing target id",
			board:   func(slug string) string { return toolAuthoringBoardFixture(slug, 3, true, toolUpdateTargetBlock()) },
			request: ToolDeleteRequest{},
			wantIs:  ErrNotFound,
		},
		{
			name:    "unknown target id",
			board:   func(slug string) string { return toolAuthoringBoardFixture(slug, 3, true, toolUpdateTargetBlock()) },
			request: ToolDeleteRequest{ID: "tool_missing"},
			wantIs:  ErrNotFound,
		},
		{
			name:       "invalid updatedBy TOML",
			board:      func(slug string) string { return toolAuthoringBoardFixture(slug, 3, true, toolUpdateTargetBlock()) },
			request:    ToolDeleteRequest{ID: "tool_target", UpdatedBy: "agent\a"},
			wantMarker: "invalid_tool_updated_by",
		},
		{
			name: "schema one cannot authorize Tool delete",
			board: func(slug string) string {
				return strings.Replace(toolAuthoringBoardFixture(slug, 3, true, toolUpdateTargetBlock()), "schema = 2\n", "schema = 1\n", 1)
			},
			request:    ToolDeleteRequest{ID: "tool_target"},
			wantMarker: "schema 2",
		},
		{
			name: "unknown current Tool tuple",
			board: func(slug string) string {
				return strings.Replace(
					toolAuthoringBoardFixture(
						slug,
						3,
						true,
						toolUpdateTargetBlock()+toolStructuralJSONNormalizeToolBlock("tool_survivor", "Survivor", "port_survivor_in", "port_survivor_out"),
					),
					`profileVersion = "1"`,
					`profileVersion = "2"`,
					1,
				)
			},
			request:    ToolDeleteRequest{ID: "tool_target"},
			wantMarker: FindingInvalidTool,
		},
		{
			name: "dangling retained connection",
			board: func(slug string) string {
				return toolAuthoringBoardFixture(
					slug,
					3,
					true,
					toolUpdateTargetBlock()+
						toolStructuralJSONNormalizeToolBlock("tool_source", "Source", "port_source_in", "port_source_out")+
						toolStructuralJSONNormalizeToolBlock("tool_survivor", "Survivor", "port_survivor_in", "port_survivor_out")+
						`[[connection]]
id = "edge_invalid"
channel = "workflow"
from = "tool_source:port_missing"
to = "tool_survivor:port_survivor_in"
`,
				)
			},
			request:    ToolDeleteRequest{ID: "tool_target"},
			wantMarker: FindingDanglingConnection,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-delete-invalid"
			boardRaw := test.board(slug)
			writeFixture(t, store.BoardPath(slug), boardRaw)
			before, err := store.ReadBoard(slug)
			if err != nil {
				t.Fatalf("read invalid-delete source: %v", err)
			}

			result, err := store.DeleteTool(slug, test.request, toolAuthoringAbsentOptions(before))
			if err == nil {
				t.Fatalf("invalid Tool delete returned result %#v", result)
			}
			if test.wantIs != nil && !errors.Is(err, test.wantIs) {
				t.Fatalf("invalid Tool delete error = %v, want errors.Is(%v)", err, test.wantIs)
			}
			if test.wantMarker != "" && !strings.Contains(err.Error(), test.wantMarker) {
				t.Fatalf("invalid Tool delete error = %v, want marker %q", err, test.wantMarker)
			}
			assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, nil)
		})
	}
}

func TestDeleteToolRequiresExactClosedPairCASWithoutMutation(t *testing.T) {
	tests := []struct {
		name             string
		mutate           func(*ToolWriteOptions)
		wantPrecondition bool
	}{
		{name: "board ETag conflict", mutate: func(opts *ToolWriteOptions) { opts.Board.ExpectedETag = strings.Repeat("0", 64) }},
		{name: "board revision conflict", mutate: func(opts *ToolWriteOptions) { opts.Board.ExpectedRev++ }},
		{name: "layout ETag conflict", mutate: func(opts *ToolWriteOptions) { opts.Layout.ETag = strings.Repeat("f", 64) }},
		{name: "present layout expected absent", mutate: func(opts *ToolWriteOptions) { opts.Layout.State, opts.Layout.ETag = LayoutWriteAbsent, "" }},
		{name: "missing layout expectation", mutate: func(opts *ToolWriteOptions) { opts.Layout = nil }, wantPrecondition: true},
		{name: "present expectation missing ETag", mutate: func(opts *ToolWriteOptions) { opts.Layout.ETag = "" }, wantPrecondition: true},
		{name: "absent expectation carries ETag", mutate: func(opts *ToolWriteOptions) { opts.Layout.State = LayoutWriteAbsent }, wantPrecondition: true},
		{name: "unknown layout state", mutate: func(opts *ToolWriteOptions) { opts.Layout.State = "maybe" }, wantPrecondition: true},
		{name: "missing board ETag", mutate: func(opts *ToolWriteOptions) { opts.Board.ExpectedETag = "" }, wantPrecondition: true},
		{name: "missing board revision", mutate: func(opts *ToolWriteOptions) { opts.Board.ExpectedRev = 0 }, wantPrecondition: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-delete-cas"
			boardRaw := toolAuthoringBoardFixture(slug, 5, true, toolUpdateTargetBlock())
			layoutRaw := "schema = 1\nboardId = \"brd_tool-delete-cas\"\nboardRev = 5\n\n[[node]]\nid = \"tool_target\"\nx = 112\ny = 224\n"
			writeFixture(t, store.BoardPath(slug), boardRaw)
			writeFixture(t, store.LayoutPath(slug), layoutRaw)
			board, layout := toolAuthoringReadPair(t, store, slug)
			opts := toolAuthoringPresentOptions(board, layout)
			test.mutate(&opts)

			_, err := store.DeleteTool(slug, ToolDeleteRequest{ID: "tool_target"}, opts)
			want := ErrConflict
			if test.wantPrecondition {
				want = ErrPreconditionRequired
			}
			if !errors.Is(err, want) {
				t.Fatalf("closed pair expectation error = %v, want errors.Is(%v)", err, want)
			}
			assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, &layoutRaw)
		})
	}

	t.Run("absent layout expected present", func(t *testing.T) {
		store := newToolAuthoringStore(t)
		slug := "tool-delete-cas-absent"
		boardRaw := toolAuthoringBoardFixture(slug, 5, true, toolUpdateTargetBlock())
		writeFixture(t, store.BoardPath(slug), boardRaw)
		board, err := store.ReadBoard(slug)
		if err != nil {
			t.Fatalf("read absent-layout CAS source: %v", err)
		}
		opts := ToolWriteOptions{
			Board:  WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev},
			Layout: &LayoutWriteExpectation{State: LayoutWritePresent, ETag: strings.Repeat("a", 64)},
		}

		_, err = store.DeleteTool(slug, ToolDeleteRequest{ID: "tool_target"}, opts)
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("absent/present inverse expectation error = %v, want ErrConflict", err)
		}
		assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, nil)
	})
}

func TestDeleteToolRejectsInvalidLayoutAuthorityWithoutMutation(t *testing.T) {
	tests := []struct {
		name       string
		layout     func(string) string
		wantMarker string
	}{
		{
			name: "layout board mismatch",
			layout: func(string) string {
				return "schema = 1\nboardId = \"brd_other\"\nboardRev = 3\n"
			},
			wantMarker: "does not match",
		},
		{
			name: "duplicate layout node id",
			layout: func(slug string) string {
				return "schema = 1\nboardId = \"brd_" + slug + "\"\nboardRev = 3\n\n[[node]]\nid = \"tool_target\"\nx = 1\ny = 2\n\n[[node]]\nid = \"tool_target\"\nx = 3\ny = 4\n"
			},
			wantMarker: "duplicate_layout_id",
		},
		{
			name: "out of range retained coordinate",
			layout: func(slug string) string {
				return "schema = 1\nboardId = \"brd_" + slug + "\"\nboardRev = 3\n\n[[node]]\nid = \"mis_main\"\nx = 2147483648\ny = 4\n"
			},
			wantMarker: "invalid_layout_coordinate",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-delete-layout-authority"
			boardRaw := toolAuthoringBoardFixture(slug, 3, true, toolUpdateTargetBlock())
			layoutRaw := test.layout(slug)
			writeFixture(t, store.BoardPath(slug), boardRaw)
			writeFixture(t, store.LayoutPath(slug), layoutRaw)
			board, layout := toolAuthoringReadPair(t, store, slug)

			_, err := store.DeleteTool(
				slug,
				ToolDeleteRequest{ID: "tool_target"},
				toolAuthoringPresentOptions(board, layout),
			)
			if err == nil || !strings.Contains(err.Error(), test.wantMarker) {
				t.Fatalf("invalid layout authority error = %v, want marker %q", err, test.wantMarker)
			}
			assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, &layoutRaw)
		})
	}
}

func toolDeleteConnectedBoardFixture(slug string, rev int) string {
	return toolAuthoringBoardFixture(
		slug,
		rev,
		true,
		toolStructuralJSONNormalizeToolBlock("tool_source", "Source", "port_source_in", "port_source_out")+
			toolUpdateTargetBlock()+
			toolStructuralJSONNormalizeToolBlock("tool_sink", "Sink", "port_sink_in", "port_sink_out")+
			toolStructuralJSONNormalizeToolBlock("tool_keep", "Keep", "port_keep_in", "port_keep_out")+
			`[[connection]]
id = "edge_into_target"
channel = "workflow"
from = "tool_source:port_source_out"
to = 'tool_target:port_target_in'
x_connection_note = "remove incoming"

[connection.metadata]
marker = "delete incoming subtree"

[connection.metadata.trace]
depth = 1

[[connection]]
id = "edge_keep"
channel = "workflow"
from = "tool_source:port_source_out"
to = "tool_keep:port_keep_in"
x_connection_note = "preserve exact"

[connection.metadata]
note = 'retain nested exact'

[connection.metadata.route]
lane = "center"

[[connection]]
id = "edge_from_target"
channel = "workflow"
from = 'tool_target:port_target_out'
to = "tool_sink:port_sink_in"
x_connection_note = "remove outgoing"

[connection.metadata]
marker = "delete outgoing subtree"

[connection.metadata.trace]
depth = 2
`,
	)
}

func toolDeleteLayoutIDs(nodes []LayoutNode) []string {
	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.ID)
	}
	return ids
}

func toolDeleteLayoutEdgeIDs(edges []LayoutEdge) []string {
	ids := make([]string, 0, len(edges))
	for _, edge := range edges {
		ids = append(ids, edge.ID)
	}
	return ids
}

func assertToolDeletePersistedResult(t *testing.T, store *Store, slug string, result *ToolDeleteResult) {
	t.Helper()
	persistedBoard, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read persisted Tool delete board: %v", err)
	}
	if result == nil || result.Board == nil || !reflect.DeepEqual(persistedBoard, result.Board) {
		t.Fatalf("returned Tool delete board is not canonical:\n returned %#v\npersisted %#v", result, persistedBoard)
	}
	if result.Layout == nil {
		if _, err := os.Lstat(store.LayoutPath(slug)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("absent Tool delete layout became canonical: %v", err)
		}
		return
	}
	persistedLayout, err := store.ReadLayout(slug)
	if err != nil {
		t.Fatalf("read persisted Tool delete layout: %v", err)
	}
	if !reflect.DeepEqual(persistedLayout, result.Layout) {
		t.Fatalf("returned Tool delete layout is not canonical:\n returned %#v\npersisted %#v", result.Layout, persistedLayout)
	}
}
