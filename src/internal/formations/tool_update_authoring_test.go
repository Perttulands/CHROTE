package formations

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestUpdateToolPresentLayoutChangesOnlyAuthoredFieldsAndFiltersStaleAuthority(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-update-present"
	boardRaw := toolUpdateConnectedBoardFixture(slug, 7)
	layoutRaw := `schema = 1 # preserve schema comment
boardId = "brd_tool-update-present"
boardRev = 7 # preserve revision comment
updatedAt = "2026-07-19T00:00:00Z" # preserve timestamp comment
x_layout_owner = "keep"

# authoritative edge deliberately precedes nodes
[[edge]]
id = "edge_existing"
lane = "north-by-northwest" # preserve exact lane

# target remains intentionally unplaced
[[node]]
id = "tool_target"
# coordinates deliberately absent

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

[layout_extension]
note = "preserve this table exactly"
`
	writeFixture(t, store.BoardPath(slug), boardRaw)
	writeFixture(t, store.LayoutPath(slug), layoutRaw)
	beforeBoard, beforeLayout := toolAuthoringReadPair(t, store, slug)
	beforeTarget := toolUpdateFindTool(t, beforeBoard, "tool_target")
	newTitle := "Renamed target"
	newParams := map[string]any{"mode": "strict"}

	result, err := store.UpdateTool(slug, ToolUpdateRequest{
		ToolID:    "tool_target",
		Title:     &newTitle,
		Params:    &newParams,
		UpdatedBy: "agent:update-test",
	}, toolAuthoringPresentOptions(beforeBoard, beforeLayout))
	if err != nil {
		t.Fatalf("update Tool with present layout: %v", err)
	}
	if result.Board == nil || result.Layout == nil {
		t.Fatalf("present-layout update result = %#v, want board and layout", result)
	}
	if result.Board.Rev != beforeBoard.Rev+1 || result.Layout.BoardRev != result.Board.Rev {
		t.Fatalf("paired update revisions = board %d layout %d, want %d", result.Board.Rev, result.Layout.BoardRev, beforeBoard.Rev+1)
	}
	if result.Board.UpdatedBy != "agent:update-test" || result.Board.UpdatedAt != "2026-07-20T08:30:00Z" || result.Layout.UpdatedAt != result.Board.UpdatedAt {
		t.Fatalf("paired update provenance = board %#v layout %#v", result.Board, result.Layout)
	}

	updatedTarget := toolUpdateFindTool(t, result.Board, "tool_target")
	if updatedTarget.Title != newTitle || !reflect.DeepEqual(updatedTarget.Params, newParams) {
		t.Fatalf("updated authored Tool fields = %#v, want title %q params %#v", updatedTarget, newTitle, newParams)
	}
	if !reflect.DeepEqual(result.Tool, updatedTarget) {
		t.Fatalf("updated Tool result = %#v, want board projection %#v", result.Tool, updatedTarget)
	}
	if updatedTarget.ID != beforeTarget.ID || updatedTarget.ProfileID != beforeTarget.ProfileID || updatedTarget.ProfileVersion != beforeTarget.ProfileVersion ||
		!reflect.DeepEqual(updatedTarget.Inputs, beforeTarget.Inputs) || !reflect.DeepEqual(updatedTarget.Outputs, beforeTarget.Outputs) {
		t.Fatalf("update changed immutable Tool identity/tuple/ports:\n before %#v\n after  %#v", beforeTarget, updatedTarget)
	}
	if !reflect.DeepEqual(result.Board.Connections, beforeBoard.Connections) {
		t.Fatalf("update changed connections:\n got  %#v\n want %#v", result.Board.Connections, beforeBoard.Connections)
	}
	if !strings.Contains(result.Board.TOML, `title = "Renamed target" # preserve title comment`) ||
		!strings.Contains(result.Board.TOML, `mode = "\u0073trict" # preserve parameter comment`) {
		t.Fatalf("update did not preserve authored Tool source around the changed title/equal replacement params:\n%s", result.Board.TOML)
	}
	const immutablePortSpan = `[[tool.input]]
id = "port_target_in" # immutable generated input id
name = "input"
label = "Report"
direction = "input"
kind = "work"
acceptedMediaTypes = ["application/json"]
required = true
role = "data"`
	if !strings.Contains(result.Board.TOML, immutablePortSpan) {
		t.Fatalf("update rewrote immutable Tool port source:\n%s", result.Board.TOML)
	}

	if strings.Contains(result.Layout.TOML, "stale_node") || strings.Contains(result.Layout.TOML, "stale_edge") {
		t.Fatalf("update retained layout entries outside board authority:\n%s", result.Layout.TOML)
	}
	targetBlock := toolUpdateLayoutOwnedBlock(t, result.Layout.TOML, "node", "tool_target")
	if strings.Contains(targetBlock, "\nx =") || strings.Contains(targetBlock, "\ny =") {
		t.Fatalf("update invented target coordinates:\n%s", targetBlock)
	}
	retainedSpans := []string{
		`# authoritative edge deliberately precedes nodes
[[edge]]
id = "edge_existing"
lane = "north-by-northwest" # preserve exact lane`,
		`# target remains intentionally unplaced
[[node]]
id = "tool_target"
# coordinates deliberately absent`,
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
		`[layout_extension]
note = "preserve this table exactly"`,
	}
	previous := -1
	for _, span := range retainedSpans {
		index := strings.Index(result.Layout.TOML, span)
		if index < 0 {
			t.Fatalf("update did not preserve exact retained layout span %q:\n%s", span, result.Layout.TOML)
		}
		if index <= previous {
			t.Fatalf("update reordered retained layout span %q", span)
		}
		previous = index
	}
	if !strings.Contains(result.Layout.TOML, `boardRev = 8 # preserve revision comment`) ||
		!strings.Contains(result.Layout.TOML, `updatedAt = "2026-07-20T08:30:00Z" # preserve timestamp comment`) {
		t.Fatalf("update did not advance layout identity in place:\n%s", result.Layout.TOML)
	}
}

func TestUpdateToolPresenceAwareTitleAndCompleteParamsKeepAbsentLayoutAbsent(t *testing.T) {
	tests := []struct {
		name            string
		request         func() ToolUpdateRequest
		wantTitle       string
		wantParamSource string
	}{
		{
			name: "title only preserves omitted params",
			request: func() ToolUpdateRequest {
				title := "Title only"
				return ToolUpdateRequest{ToolID: "tool_target", Title: &title, UpdatedBy: "agent:update-test"}
			},
			wantTitle:       "Title only",
			wantParamSource: `mode = "\u0073trict" # preserve parameter comment`,
		},
		{
			name: "params only preserves omitted title",
			request: func() ToolUpdateRequest {
				params := map[string]any{"mode": "strict"}
				return ToolUpdateRequest{ToolID: "tool_target", Params: &params, UpdatedBy: "agent:update-test"}
			},
			wantTitle:       "Original target",
			wantParamSource: `mode = "\u0073trict" # preserve parameter comment`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-update-absent"
			boardRaw := toolAuthoringBoardFixture(slug, 4, true, toolUpdateTargetBlock())
			writeFixture(t, store.BoardPath(slug), boardRaw)
			before, err := store.ReadBoard(slug)
			if err != nil {
				t.Fatalf("read update source: %v", err)
			}

			result, err := store.UpdateTool(slug, test.request(), toolAuthoringAbsentOptions(before))
			if err != nil {
				t.Fatalf("presence-aware update: %v", err)
			}
			if result.Layout != nil {
				t.Fatalf("absent-layout update returned layout %#v", result.Layout)
			}
			if _, err := os.Lstat(store.LayoutPath(slug)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("absent-layout update materialized layout: %v", err)
			}
			updated := toolUpdateFindTool(t, result.Board, "tool_target")
			if updated.Title != test.wantTitle || !reflect.DeepEqual(updated.Params, map[string]any{"mode": "strict"}) {
				t.Fatalf("presence-aware Tool = %#v", updated)
			}
			if !strings.Contains(result.Board.TOML, test.wantParamSource) {
				t.Fatalf("presence-aware parameter source mismatch, want %q:\n%s", test.wantParamSource, result.Board.TOML)
			}
			if result.Board.Rev != before.Rev+1 || !reflect.DeepEqual(result.Tool, updated) {
				t.Fatalf("absent-layout update result = %#v", result)
			}
		})
	}
}

func TestUpdateToolAllowsDocumentedStructuralDraftGaps(t *testing.T) {
	store := newToolAuthoringStore(t)
	slug := "tool-update-draft"
	boardRaw := toolAuthoringBoardFixture(slug, 2, false, toolUpdateTargetBlock()+`[[gate]]
id = "gate_unroutable"
title = "Incomplete review"
kinds = ["code"]
criterion = ""
`)
	writeFixture(t, store.BoardPath(slug), boardRaw)
	before, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read draft update source: %v", err)
	}
	title := "Draft target renamed"

	result, err := store.UpdateTool(slug, ToolUpdateRequest{ToolID: "tool_target", Title: &title}, toolAuthoringAbsentOptions(before))
	if err != nil {
		t.Fatalf("update Tool on documented draft gaps: %v", err)
	}
	if result.Layout != nil || len(result.Board.Missions) != 0 || len(result.Board.Gates) != 1 || len(result.Board.Connections) != 0 {
		t.Fatalf("draft update changed structural gaps: %#v", result)
	}
	updated := toolUpdateFindTool(t, result.Board, "tool_target")
	if len(updated.Inputs) != 1 || updated.Inputs[0].Required == nil || !*updated.Inputs[0].Required {
		t.Fatalf("draft update did not retain unwired required Tool input: %#v", updated.Inputs)
	}
}

func TestUpdateToolRejectsInvalidPatchOrTargetWithoutMutation(t *testing.T) {
	tests := []struct {
		name       string
		request    func() ToolUpdateRequest
		wantIs     error
		wantMarker string
	}{
		{name: "no authored field present", request: func() ToolUpdateRequest { return ToolUpdateRequest{ToolID: "tool_target"} }},
		{name: "blank title is present and invalid", request: func() ToolUpdateRequest {
			title := " \t"
			return ToolUpdateRequest{ToolID: "tool_target", Title: &title}
		}, wantMarker: "title"},
		{name: "explicit null parameter object", request: func() ToolUpdateRequest {
			var params map[string]any
			return ToolUpdateRequest{ToolID: "tool_target", Params: &params}
		}, wantMarker: "parameter"},
		{name: "partial replacement parameter object", request: func() ToolUpdateRequest {
			params := map[string]any{}
			return ToolUpdateRequest{ToolID: "tool_target", Params: &params}
		}, wantMarker: "parameter"},
		{name: "unknown replacement parameter", request: func() ToolUpdateRequest {
			params := map[string]any{"mode": "strict", "extra": true}
			return ToolUpdateRequest{ToolID: "tool_target", Params: &params}
		}, wantMarker: "parameter"},
		{name: "invalid replacement enum", request: func() ToolUpdateRequest {
			params := map[string]any{"mode": "relaxed"}
			return ToolUpdateRequest{ToolID: "tool_target", Params: &params}
		}, wantMarker: "mode"},
		{name: "missing target id", request: func() ToolUpdateRequest {
			title := "No target"
			return ToolUpdateRequest{Title: &title}
		}, wantIs: ErrNotFound},
		{name: "unknown target id", request: func() ToolUpdateRequest {
			title := "No target"
			return ToolUpdateRequest{ToolID: "tool_missing", Title: &title}
		}, wantIs: ErrNotFound},
		{name: "invalid updatedBy TOML", request: func() ToolUpdateRequest {
			title := "Unsafe provenance"
			return ToolUpdateRequest{ToolID: "tool_target", Title: &title, UpdatedBy: "agent\a"}
		}, wantMarker: "invalid_tool_updated_by"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newToolAuthoringStore(t)
			slug := "tool-update-invalid"
			boardRaw := toolAuthoringBoardFixture(slug, 3, true, toolUpdateTargetBlock())
			writeFixture(t, store.BoardPath(slug), boardRaw)
			before, err := store.ReadBoard(slug)
			if err != nil {
				t.Fatalf("read invalid-update source: %v", err)
			}

			result, err := store.UpdateTool(slug, test.request(), toolAuthoringAbsentOptions(before))
			if err == nil {
				t.Fatalf("invalid update returned result %#v", result)
			}
			if test.wantIs != nil && !errors.Is(err, test.wantIs) {
				t.Fatalf("invalid update error = %v, want errors.Is(%v)", err, test.wantIs)
			}
			if test.wantMarker != "" && !strings.Contains(err.Error(), test.wantMarker) {
				t.Fatalf("invalid update error = %v, want marker %q", err, test.wantMarker)
			}
			assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, nil)
		})
	}
}

func TestUpdateToolRequiresExactClosedPairCASWithoutMutation(t *testing.T) {
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
			slug := "tool-update-cas"
			boardRaw := toolAuthoringBoardFixture(slug, 5, true, toolUpdateTargetBlock())
			layoutRaw := "schema = 1\nboardId = \"brd_tool-update-cas\"\nboardRev = 5\n\n[[node]]\nid = \"tool_target\"\nx = 112\ny = 224\n"
			writeFixture(t, store.BoardPath(slug), boardRaw)
			writeFixture(t, store.LayoutPath(slug), layoutRaw)
			board, layout := toolAuthoringReadPair(t, store, slug)
			opts := toolAuthoringPresentOptions(board, layout)
			test.mutate(&opts)
			title := "CAS should reject"

			_, err := store.UpdateTool(slug, ToolUpdateRequest{ToolID: "tool_target", Title: &title}, opts)
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
		slug := "tool-update-cas-absent"
		boardRaw := toolAuthoringBoardFixture(slug, 5, true, toolUpdateTargetBlock())
		writeFixture(t, store.BoardPath(slug), boardRaw)
		board, err := store.ReadBoard(slug)
		if err != nil {
			t.Fatalf("read absent-layout CAS source: %v", err)
		}
		title := "CAS should reject"
		opts := ToolWriteOptions{
			Board:  WriteOptions{ExpectedETag: board.ETag, ExpectedRev: board.Rev},
			Layout: &LayoutWriteExpectation{State: LayoutWritePresent, ETag: strings.Repeat("a", 64)},
		}
		_, err = store.UpdateTool(slug, ToolUpdateRequest{ToolID: "tool_target", Title: &title}, opts)
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("absent/present inverse expectation error = %v, want ErrConflict", err)
		}
		assertToolAuthoringPairUnchanged(t, store, slug, boardRaw, nil)
	})
}

func toolUpdateConnectedBoardFixture(slug string, rev int) string {
	return toolAuthoringBoardFixture(slug, rev, true,
		toolStructuralJSONNormalizeToolBlock("tool_source", "Source", "port_source_in", "port_source_out")+
			toolUpdateTargetBlock()+`[[connection]]
id = "edge_existing"
channel = "workflow"
from = "tool_source:port_source_out"
to = "tool_target:port_target_in"
x_connection_note = "preserve exact"
`)
}

func toolUpdateTargetBlock() string {
	return `# target Tool leading comment
[[tool]]
id = "tool_target" # immutable generated Tool id
title = "Original target" # preserve title comment
profileId = "json.normalize" # immutable profile id
profileVersion = "1" # immutable exact version

[tool.params]
mode = "\u0073trict" # preserve parameter comment

[[tool.input]]
id = "port_target_in" # immutable generated input id
name = "input"
label = "Report"
direction = "input"
kind = "work"
acceptedMediaTypes = ["application/json"]
required = true
role = "data"

[[tool.output]]
id = "port_target_out"
name = "output"
label = "Normalized report"
direction = "output"
kind = "work"
acceptedMediaTypes = ["application/json"]

`
}

func toolUpdateFindTool(t *testing.T, board *BoardDocument, id string) ToolNode {
	t.Helper()
	if board == nil {
		t.Fatal("Tool lookup received nil board")
	}
	for _, tool := range board.Tools {
		if tool.ID == id {
			return tool
		}
	}
	t.Fatalf("Tool %q missing from board %#v", id, board.Tools)
	return ToolNode{}
}

func toolUpdateLayoutOwnedBlock(t *testing.T, raw, kind, id string) string {
	t.Helper()
	blocks, err := parseToolLayoutOwnedBlocks([]byte(raw))
	if err != nil {
		t.Fatalf("parse updated layout authority: %v", err)
	}
	lines := splitLines([]byte(raw))
	for _, block := range blocks {
		if block.kind == kind && block.id == id {
			return string(renderTOMLLines(lines[block.start:block.end]))
		}
	}
	t.Fatalf("layout %s block %q missing:\n%s", kind, id, raw)
	return ""
}
