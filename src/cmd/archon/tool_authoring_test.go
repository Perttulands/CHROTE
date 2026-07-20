package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/chrote/server/internal/formations"
)

func TestArchonToolCreatePublishesCanonicalPairWithoutReflowOrImplicitWiring(t *testing.T) {
	tests := []struct {
		name       string
		withLayout bool
		placement  []string
		wantX      int
		wantY      int
		exact      bool
	}{
		{
			name:       "present layout exact coordinates",
			withLayout: true,
			placement:  []string{"--x", "123", "--y", "-456"},
			wantX:      123,
			wantY:      -456,
			exact:      true,
		},
		{
			name: "absent layout default heuristic",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newArchonToolAuthoringHarness(t, test.withLayout, false)
			beforeBoard := mustReadArchonToolBoard(t, harness.store, harness.slug)
			var beforeLayout *formations.LayoutDocument
			if test.withLayout {
				beforeLayout = mustReadArchonToolLayout(t, harness.store, harness.slug)
			}

			args := []string{
				"tool", "create", harness.slug,
				"--profile-id", "json.normalize",
				"--profile-version", "1",
				"--title", "Created through Archon",
				"--params-json", `{"mode":"strict"}`,
				"--updated-by", "agent:archon-test",
				"--json",
			}
			args = append(args, test.placement...)
			stdout, stderr, code := harness.run(t, args...)
			if code != 0 || stderr != "" {
				t.Fatalf("tool create code=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
			assertArchonToolJSONRootKeys(t, stdout, "board", "layout", "tool")

			var result formations.ToolCreateResult
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatalf("decode Tool create result: %v\n%s", err, stdout)
			}
			if result.Board == nil || result.Layout == nil || result.Tool.ID == "" {
				t.Fatalf("Tool create result = %#v, want canonical board/layout/generated Tool", result)
			}
			if result.Tool.Title != "Created through Archon" ||
				result.Tool.ProfileID != "json.normalize" || result.Tool.ProfileVersion != "1" ||
				!reflect.DeepEqual(result.Tool.Params, map[string]any{"mode": "strict"}) {
				t.Fatalf("created Tool = %#v, want exact requested definition", result.Tool)
			}
			if len(result.Tool.Inputs) != 1 || result.Tool.Inputs[0].Name != "input" ||
				len(result.Tool.Outputs) != 1 || result.Tool.Outputs[0].Name != "output" {
				t.Fatalf("created Tool ports = inputs %#v outputs %#v, want descriptor-derived ports", result.Tool.Inputs, result.Tool.Outputs)
			}
			if result.Board.Rev != beforeBoard.Rev+1 || result.Layout.BoardRev != result.Board.Rev {
				t.Fatalf("created pair revisions = board %d layout %d, want board %d", result.Board.Rev, result.Layout.BoardRev, beforeBoard.Rev+1)
			}
			if !reflect.DeepEqual(result.Board.Connections, beforeBoard.Connections) {
				t.Fatalf("Tool create implicitly changed connections:\n before=%#v\n after=%#v", beforeBoard.Connections, result.Board.Connections)
			}
			if beforeLayout != nil {
				assertArchonToolRetainedLayoutNodes(t, beforeLayout, result.Layout, nil)
			}
			createdPosition, found := archonToolLayoutNode(result.Layout, result.Tool.ID)
			if !found {
				t.Fatalf("created Tool %q has no layout node: %#v", result.Tool.ID, result.Layout.Nodes)
			}
			if test.exact && (createdPosition.X != test.wantX || createdPosition.Y != test.wantY) {
				t.Fatalf("created Tool position = %#v, want %d,%d", createdPosition, test.wantX, test.wantY)
			}
			wantNodes := []formations.LayoutNode{createdPosition}
			var wantEdges []formations.LayoutEdge
			if beforeLayout != nil {
				wantNodes = append(append([]formations.LayoutNode(nil), beforeLayout.Nodes...), createdPosition)
				wantEdges = beforeLayout.Edges
			}
			assertArchonToolLayoutInventory(t, result.Layout, wantNodes, wantEdges)
			if strings.Contains(stdout, `"toml"`) || result.Board.TOML != "" || result.Layout.TOML != "" {
				t.Fatalf("Tool create leaked raw TOML: %s", stdout)
			}
			assertArchonToolCanonicalPair(t, harness, result.Board, result.Layout)
		})
	}
}

func TestArchonToolCreateMigratesSchemaOneBoardOnItsFirstTool(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	const slug = "first-tool"
	writeArchonFile(t, store.BoardPath(slug), `schema = 1
id = "brd_first_tool"
slug = "first-tool"
title = "First Tool"
rev = 1
x_schema_one_sentinel = "keep"
`)
	harness := &archonToolAuthoringHarness{
		workspace: workspace,
		slug:      slug,
		store:     store,
		runner:    &fakeTmux{live: map[string]bool{}},
	}
	stdout, stderr, code := harness.run(
		t,
		"tool", "create", slug,
		"--profile-id", "json.normalize",
		"--profile-version", "1",
		"--title", "First normalized step",
		"--params-json", `{"mode":"strict"}`,
		"--json",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("first Tool schema-1 create code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var result formations.ToolCreateResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("decode first Tool schema-1 create: %v\n%s", err, stdout)
	}
	if result.Board == nil || result.Board.Schema != formations.CurrentBoardSchema || result.Board.Rev != 2 ||
		len(result.Board.Tools) != 1 || result.Board.Tools[0].ID != result.Tool.ID {
		t.Fatalf("first Tool migrated board = %#v, Tool=%#v", result.Board, result.Tool)
	}
	if result.Layout == nil || result.Layout.BoardID != result.Board.ID || result.Layout.BoardRev != result.Board.Rev {
		t.Fatalf("first Tool migrated layout = %#v, board=%#v", result.Layout, result.Board)
	}
	position, found := archonToolLayoutNode(result.Layout, result.Tool.ID)
	if !found {
		t.Fatalf("first Tool has no layout node: %#v", result.Layout.Nodes)
	}
	assertArchonToolLayoutInventory(t, result.Layout, []formations.LayoutNode{position}, nil)
	if raw := readArchonFile(t, store.BoardPath(slug)); !strings.Contains(raw, `x_schema_one_sentinel = "keep"`) {
		t.Fatalf("first Tool migration dropped schema-1 retained source:\n%s", raw)
	}
	assertArchonToolCanonicalPair(t, harness, result.Board, result.Layout)
}

func TestArchonToolCreatePlacementUnionIsStoreValidatedAndNeverReflowsExistingNodes(t *testing.T) {
	validHints := []struct {
		name         string
		flags        []string
		wantX, wantY int
	}{
		{name: "predecessor only", flags: []string{"--predecessor-node-id", "tool_sink"}, wantX: 1064, wantY: 420},
		{name: "successor only", flags: []string{"--successor-node-id", "gate_review"}, wantX: 1120, wantY: 420},
		{name: "predecessor and successor", flags: []string{"--predecessor-node-id", "tool_sink", "--successor-node-id", "gate_review"}, wantX: 1036, wantY: 420},
	}
	for _, test := range validHints {
		t.Run(test.name+" places only the new node", func(t *testing.T) {
			harness := newArchonToolAuthoringHarness(t, true, false)
			beforeBoard := mustReadArchonToolBoard(t, harness.store, harness.slug)
			beforeLayout := mustReadArchonToolLayout(t, harness.store, harness.slug)
			args := []string{
				"tool", "create", harness.slug,
				"--profile-id", "json.normalize",
				"--profile-version", "1",
				"--title", "Placed with hints",
				"--params-json", `{"mode":"strict"}`,
				"--json",
			}
			args = append(args, test.flags...)
			stdout, stderr, code := harness.run(t, args...)
			if code != 0 || stderr != "" {
				t.Fatalf("hinted Tool create code=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
			var result formations.ToolCreateResult
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatalf("decode hinted Tool create: %v\n%s", err, stdout)
			}
			if result.Layout == nil {
				t.Fatalf("hinted Tool create layout is nil: %#v", result)
			}
			position, found := archonToolLayoutNode(result.Layout, result.Tool.ID)
			if !found {
				t.Fatalf("hinted Tool %q has no new layout node: %#v", result.Tool.ID, result.Layout.Nodes)
			}
			if position.X != test.wantX || position.Y != test.wantY {
				t.Fatalf("hinted Tool position = %#v, want %d,%d", position, test.wantX, test.wantY)
			}
			assertArchonToolRetainedLayoutNodes(t, beforeLayout, result.Layout, nil)
			wantNodes := append(append([]formations.LayoutNode(nil), beforeLayout.Nodes...), position)
			assertArchonToolLayoutInventory(t, result.Layout, wantNodes, beforeLayout.Edges)
			if !reflect.DeepEqual(result.Board.Connections, beforeBoard.Connections) {
				t.Fatalf("placement hints implicitly wired the Tool:\n before=%#v\n after=%#v", beforeBoard.Connections, result.Board.Connections)
			}
			assertArchonToolCanonicalPair(t, harness, result.Board, result.Layout)
		})
	}

	invalid := []struct {
		name      string
		placement []string
	}{
		{name: "x without y", placement: []string{"--x", "10"}},
		{name: "y without x", placement: []string{"--y", "10"}},
		{name: "exact coordinates plus hint", placement: []string{"--x", "10", "--y", "20", "--predecessor-node-id", "tool_sink"}},
		{name: "same predecessor and successor", placement: []string{"--predecessor-node-id", "tool_sink", "--successor-node-id", "tool_sink"}},
		{name: "unknown predecessor", placement: []string{"--predecessor-node-id", "node_missing"}},
		{name: "explicit empty predecessor", placement: []string{"--predecessor-node-id", ""}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			harness := newArchonToolAuthoringHarness(t, true, false)
			before := snapshotArchonToolParityTree(t, harness.workspace)
			beforeFiles := snapshotArchonToolPairFileIdentity(t, harness)
			args := []string{
				"tool", "create", harness.slug,
				"--profile-id", "json.normalize",
				"--profile-version", "1",
				"--title", "Must not be created",
				"--params-json", `{"mode":"strict"}`,
				"--json",
			}
			args = append(args, test.placement...)
			stdout, stderr, code := harness.run(t, args...)
			assertArchonToolJSONError(t, stdout, stderr, code, "invalid_tool_mutation", "tool", "json.normalize@1")
			if after := snapshotArchonToolParityTree(t, harness.workspace); !reflect.DeepEqual(after, before) {
				t.Fatalf("invalid placement changed definition pair:\n before=%#v\n after=%#v", before, after)
			}
			assertArchonToolPairFileIdentity(t, harness, beforeFiles)
		})
	}
}

func TestArchonToolUpdatePreservesPresentOrAbsentLayoutState(t *testing.T) {
	tests := []struct {
		name       string
		withLayout bool
		flags      []string
		wantTitle  string
	}{
		{
			name:       "present layout title and complete parameters",
			withLayout: true,
			flags:      []string{"--title", "Renamed through Archon", "--params-json", `{"mode":"strict"}`},
			wantTitle:  "Renamed through Archon",
		},
		{
			name:      "absent layout complete parameters only",
			flags:     []string{"--params-json", `{"mode":"strict"}`},
			wantTitle: "Normalize report",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newArchonToolAuthoringHarness(t, test.withLayout, false)
			beforeBoard := mustReadArchonToolBoard(t, harness.store, harness.slug)
			var beforeLayout *formations.LayoutDocument
			if test.withLayout {
				beforeLayout = mustReadArchonToolLayout(t, harness.store, harness.slug)
			}
			args := []string{"tool", "update", harness.slug, "normalize-report", "--updated-by", "agent:archon-test", "--json"}
			args = append(args, test.flags...)
			stdout, stderr, code := harness.run(t, args...)
			if code != 0 || stderr != "" {
				t.Fatalf("Tool update code=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
			assertArchonToolJSONRootKeys(t, stdout, "board", "layout", "tool")
			var result formations.ToolUpdateResult
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatalf("decode Tool update result: %v\n%s", err, stdout)
			}
			if result.Board == nil || result.Tool.ID != "tool_normalize" || result.Tool.Title != test.wantTitle || result.Tool.Params["mode"] != "strict" {
				t.Fatalf("Tool update result = %#v, want selected Tool with exact replacement", result)
			}
			if result.Board.Rev != beforeBoard.Rev+1 {
				t.Fatalf("updated board rev = %d, want %d", result.Board.Rev, beforeBoard.Rev+1)
			}
			if test.withLayout {
				if result.Layout == nil || result.Layout.BoardRev != result.Board.Rev {
					t.Fatalf("present update layout = %#v, want boardRev %d", result.Layout, result.Board.Rev)
				}
				assertArchonToolRetainedLayoutNodes(t, beforeLayout, result.Layout, nil)
				assertArchonToolLayoutInventory(t, result.Layout, beforeLayout.Nodes, beforeLayout.Edges)
			} else {
				if result.Layout != nil || !strings.Contains(stdout, `"layout": null`) {
					t.Fatalf("absent-layout update output = %s, want explicit layout:null", stdout)
				}
				assertArchonToolLayoutAbsent(t, harness)
			}
			assertArchonToolCanonicalPair(t, harness, result.Board, result.Layout)
		})
	}

	invalid := []struct {
		name  string
		flags []string
	}{
		{name: "no authored field"},
		{name: "explicit blank title", flags: []string{"--title", " \t"}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			harness := newArchonToolAuthoringHarness(t, true, false)
			before := snapshotArchonToolParityTree(t, harness.workspace)
			beforeFiles := snapshotArchonToolPairFileIdentity(t, harness)
			args := []string{"tool", "update", harness.slug, "tool_normalize", "--json"}
			args = append(args, test.flags...)
			stdout, stderr, code := harness.run(t, args...)
			assertArchonToolJSONError(t, stdout, stderr, code, "invalid_tool_mutation", "tool", "tool_normalize")
			if after := snapshotArchonToolParityTree(t, harness.workspace); !reflect.DeepEqual(after, before) {
				t.Fatalf("invalid Tool update changed definition pair:\n before=%#v\n after=%#v", before, after)
			}
			assertArchonToolPairFileIdentity(t, harness, beforeFiles)
		})
	}
}

func TestArchonToolDeletePreservesPresentOrAbsentLayoutState(t *testing.T) {
	for _, withLayout := range []bool{true, false} {
		name := "absent layout"
		if withLayout {
			name = "present layout"
		}
		t.Run(name, func(t *testing.T) {
			harness := newArchonToolAuthoringHarness(t, withLayout, true)
			beforeBoard := mustReadArchonToolBoard(t, harness.store, harness.slug)
			var beforeLayout *formations.LayoutDocument
			if withLayout {
				beforeLayout = mustReadArchonToolLayout(t, harness.store, harness.slug)
			}

			stdout, stderr, code := harness.run(
				t,
				"tool", "delete", harness.slug, "normalize-report",
				"--updated-by", "agent:archon-test",
				"--json",
			)
			if code != 0 || stderr != "" {
				t.Fatalf("Tool delete code=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
			assertArchonToolJSONRootKeys(t, stdout, "board", "layout", "toolId")
			var result formations.ToolDeleteResult
			if err := json.Unmarshal([]byte(stdout), &result); err != nil {
				t.Fatalf("decode Tool delete result: %v\n%s", err, stdout)
			}
			if result.Board == nil || result.ToolID != "tool_normalize" || result.Board.Rev != beforeBoard.Rev+1 {
				t.Fatalf("Tool delete result = %#v, want deleted identity and next board rev", result)
			}
			for _, tool := range result.Board.Tools {
				if tool.ID == result.ToolID {
					t.Fatalf("deleted Tool remains in board: %#v", result.Board.Tools)
				}
			}
			for _, connection := range result.Board.Connections {
				if connection.ID == "edge_tool" || strings.HasPrefix(connection.From, result.ToolID+":") || strings.HasPrefix(connection.To, result.ToolID+":") {
					t.Fatalf("deleted Tool incident connection remains: %#v", result.Board.Connections)
				}
			}
			if withLayout {
				if result.Layout == nil {
					t.Fatal("present-layout Tool delete returned nil layout")
				}
				if _, found := archonToolLayoutNode(result.Layout, result.ToolID); found {
					t.Fatalf("deleted Tool layout node remains: %#v", result.Layout.Nodes)
				}
				for _, edge := range result.Layout.Edges {
					if edge.ID == "edge_tool" {
						t.Fatalf("deleted Tool layout edge remains: %#v", result.Layout.Edges)
					}
				}
				assertArchonToolRetainedLayoutNodes(t, beforeLayout, result.Layout, map[string]bool{"tool_normalize": true})
				wantNodes := make([]formations.LayoutNode, 0, len(beforeLayout.Nodes)-1)
				for _, node := range beforeLayout.Nodes {
					if node.ID != "tool_normalize" {
						wantNodes = append(wantNodes, node)
					}
				}
				assertArchonToolLayoutInventory(t, result.Layout, wantNodes, nil)
			} else {
				if result.Layout != nil || !strings.Contains(stdout, `"layout": null`) {
					t.Fatalf("absent-layout delete output = %s, want explicit layout:null", stdout)
				}
				assertArchonToolLayoutAbsent(t, harness)
			}
			assertArchonToolCanonicalPair(t, harness, result.Board, result.Layout)
		})
	}
}

func TestArchonToolInspectIsReadOnlyAndSelectorSafe(t *testing.T) {
	harness := newArchonToolAuthoringHarness(t, true, false)
	before := snapshotArchonToolParityTree(t, harness.workspace)
	for _, selector := range []string{"tool_normalize", "Normalize report", "normalize-report"} {
		t.Run("select "+selector, func(t *testing.T) {
			stdout, stderr, code := harness.run(t, "tool", "inspect", harness.slug, selector, "--json")
			if code != 0 || stderr != "" {
				t.Fatalf("Tool inspect %q code=%d stdout=%s stderr=%s", selector, code, stdout, stderr)
			}
			var topLevel map[string]json.RawMessage
			if err := json.Unmarshal([]byte(stdout), &topLevel); err != nil {
				t.Fatalf("decode Tool inspect top level %q: %v\n%s", selector, err, stdout)
			}
			assertArchonToolExactJSONKeys(t, topLevel, "board", "tool")
			var boardIdentity map[string]json.RawMessage
			if err := json.Unmarshal(topLevel["board"], &boardIdentity); err != nil {
				t.Fatalf("decode Tool inspect board identity %q: %v\n%s", selector, err, stdout)
			}
			assertArchonToolExactJSONKeys(t, boardIdentity, "id", "slug", "title", "rev", "etag")
			var response struct {
				Board archonBoardIdentity `json:"board"`
				Tool  formations.ToolNode `json:"tool"`
			}
			if err := json.Unmarshal([]byte(stdout), &response); err != nil {
				t.Fatalf("decode Tool inspect %q: %v\n%s", selector, err, stdout)
			}
			if response.Board.ID != "brd_tool_parity" || response.Board.Slug != harness.slug || response.Board.Title != "Tool parity" || response.Board.Rev != 4 || response.Board.ETag == "" {
				t.Fatalf("Tool inspect board identity = %#v", response.Board)
			}
			if response.Tool.ID != "tool_normalize" || response.Tool.Title != "Normalize report" ||
				response.Tool.ProfileID != "json.normalize" || response.Tool.ProfileVersion != "1" ||
				response.Tool.Params["mode"] != "strict" || len(response.Tool.Inputs) != 1 || len(response.Tool.Outputs) != 1 {
				t.Fatalf("Tool inspect projection = %#v", response.Tool)
			}
			lower := strings.ToLower(stdout)
			for _, forbidden := range []string{`"toml"`, `"raw"`, `"runtime"`, `"session"`, `"authority"`, `"executable"`, `"command"`, `"argv"`, `"shell"`, `"cwd"`, `"env"`} {
				if strings.Contains(lower, forbidden) {
					t.Fatalf("Tool inspect leaked forbidden %s vocabulary: %s", forbidden, stdout)
				}
			}
		})
	}
	if after := snapshotArchonToolParityTree(t, harness.workspace); !reflect.DeepEqual(after, before) {
		t.Fatalf("Tool inspection changed workspace tree:\n before=%#v\n after=%#v", before, after)
	}

	t.Run("missing selector", func(t *testing.T) {
		stdout, stderr, code := harness.run(t, "tool", "inspect", harness.slug, "tool_missing", "--json")
		assertArchonToolJSONError(t, stdout, stderr, code, "not_found", "tool", "tool_missing")
	})

	t.Run("ambiguous title selector", func(t *testing.T) {
		ambiguous := newArchonToolAuthoringHarness(t, true, false)
		raw := archonToolParityBoardFixture()
		raw = strings.Replace(raw, `title = "Normalize report"`, `title = "Same Tool"`, 1)
		raw = strings.Replace(raw, `title = "Receive normalized report"`, `title = "Same Tool"`, 1)
		writeArchonFile(t, ambiguous.store.BoardPath(ambiguous.slug), raw)
		beforeAmbiguous := snapshotArchonToolParityTree(t, ambiguous.workspace)
		stdout, stderr, code := ambiguous.run(t, "tool", "inspect", ambiguous.slug, "same-tool", "--json")
		assertArchonToolJSONError(t, stdout, stderr, code, "ambiguous_selector", "tool", "same-tool")
		if after := snapshotArchonToolParityTree(t, ambiguous.workspace); !reflect.DeepEqual(after, beforeAmbiguous) {
			t.Fatalf("ambiguous Tool inspect changed workspace tree:\n before=%#v\n after=%#v", beforeAmbiguous, after)
		}
	})
}

func TestArchonToolUpdateAndDeleteShareExactTitleMissingAndAmbiguousSelectors(t *testing.T) {
	for _, verb := range []string{"update", "delete"} {
		t.Run(verb, func(t *testing.T) {
			command := func(slug, selector string) []string {
				args := []string{"tool", verb, slug, selector}
				if verb == "update" {
					args = append(args, "--title", "Selected by exact title")
				}
				return append(args, "--json")
			}

			t.Run("exact title", func(t *testing.T) {
				harness := newArchonToolAuthoringHarness(t, true, false)
				stdout, stderr, code := harness.run(t, command(harness.slug, "Normalize report")...)
				if code != 0 || stderr != "" {
					t.Fatalf("Tool %s exact-title selector code=%d stdout=%s stderr=%s", verb, code, stdout, stderr)
				}
				var response struct {
					Tool   formations.ToolNode `json:"tool"`
					ToolID string              `json:"toolId"`
				}
				if err := json.Unmarshal([]byte(stdout), &response); err != nil {
					t.Fatalf("decode Tool %s exact-title result: %v\n%s", verb, err, stdout)
				}
				selectedID := response.Tool.ID
				if verb == "delete" {
					selectedID = response.ToolID
				}
				if selectedID != "tool_normalize" {
					t.Fatalf("Tool %s exact-title selected %q, want tool_normalize", verb, selectedID)
				}
			})

			t.Run("missing", func(t *testing.T) {
				harness := newArchonToolAuthoringHarness(t, true, false)
				before := snapshotArchonToolParityTree(t, harness.workspace)
				stdout, stderr, code := harness.run(t, command(harness.slug, "tool_missing")...)
				assertArchonToolJSONError(t, stdout, stderr, code, "not_found", "tool", "tool_missing")
				if after := snapshotArchonToolParityTree(t, harness.workspace); !reflect.DeepEqual(after, before) {
					t.Fatalf("missing Tool %s selector changed definition pair:\n before=%#v\n after=%#v", verb, before, after)
				}
			})

			t.Run("ambiguous", func(t *testing.T) {
				harness := newArchonToolAuthoringHarness(t, true, false)
				raw := archonToolParityBoardFixture()
				raw = strings.Replace(raw, `title = "Normalize report"`, `title = "Same Tool"`, 1)
				raw = strings.Replace(raw, `title = "Receive normalized report"`, `title = "Same Tool"`, 1)
				writeArchonFile(t, harness.store.BoardPath(harness.slug), raw)
				before := snapshotArchonToolParityTree(t, harness.workspace)
				stdout, stderr, code := harness.run(t, command(harness.slug, "Same Tool")...)
				assertArchonToolJSONError(t, stdout, stderr, code, "ambiguous_selector", "tool", "Same Tool")
				if after := snapshotArchonToolParityTree(t, harness.workspace); !reflect.DeepEqual(after, before) {
					t.Fatalf("ambiguous Tool %s selector changed definition pair:\n before=%#v\n after=%#v", verb, before, after)
				}
			})
		})
	}
}

func TestArchonToolTextOutputIsStableAndCarriesGeneratedIdentity(t *testing.T) {
	harness := newArchonToolAuthoringHarness(t, false, false)
	stdout, stderr, code := harness.run(
		t,
		"tool", "create", harness.slug,
		"--profile-id", "json.normalize",
		"--profile-version", "1",
		"--title", "Text Tool",
		"--params-json", `{"mode":"strict"}`,
	)
	if code != 0 || stderr != "" || !strings.HasPrefix(stdout, "created tool_") || !strings.HasSuffix(stdout, "\n") {
		t.Fatalf("Tool create text code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	toolID := strings.TrimSuffix(strings.TrimPrefix(stdout, "created "), "\n")
	if toolID == "" || strings.Contains(toolID, "\n") {
		t.Fatalf("created Tool identity = %q", toolID)
	}

	stdout, stderr, code = harness.run(t, "tool", "update", harness.slug, toolID, "--title", "Text Renamed")
	if code != 0 || stderr != "" || stdout != "updated "+toolID+"\n" {
		t.Fatalf("Tool update text code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	stdout, stderr, code = harness.run(t, "tool", "inspect", harness.slug, toolID)
	if code != 0 || stderr != "" || !reflect.DeepEqual(strings.Fields(stdout), []string{toolID, "Text", "Renamed", "json.normalize@1"}) {
		t.Fatalf("Tool inspect text code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	stdout, stderr, code = harness.run(t, "tool", "delete", harness.slug, toolID)
	if code != 0 || stderr != "" || stdout != "deleted "+toolID+"\n" {
		t.Fatalf("Tool delete text code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestParseArchonToolParametersJSONFramesOneExactScalarObject(t *testing.T) {
	valid := []struct {
		name string
		raw  string
		want map[string]any
	}{
		{name: "empty object", raw: `{}`, want: map[string]any{}},
		{
			name: "string boolean and int64 endpoints",
			raw:  `{"text":"literal","flag":false,"minimum":-9223372036854775808,"maximum":9223372036854775807}`,
			want: map[string]any{"text": "literal", "flag": false, "minimum": int64(-9223372036854775808), "maximum": int64(9223372036854775807)},
		},
		{
			name: "valid surrogate pair and intentional replacement character",
			raw:  `{"emoji":"\uD83D\uDE00","replacement":"\uFFFD"}`,
			want: map[string]any{"emoji": "😀", "replacement": "�"},
		},
	}
	for _, test := range valid {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseArchonToolParametersJSON(test.raw)
			if err != nil {
				t.Fatalf("parse valid Tool parameter object: %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Tool parameters = %#v, want %#v", got, test.want)
			}
		})
	}

	invalidUTF8 := string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'})
	invalid := []struct {
		name string
		raw  string
	}{
		{name: "empty input"},
		{name: "array root", raw: `[]`},
		{name: "string root", raw: `"not an object"`},
		{name: "null root", raw: `null`},
		{name: "trailing JSON", raw: `{} {}`},
		{name: "duplicate decoded key", raw: `{"mode":"strict","\u006dode":"strict"}`},
		{name: "nested object", raw: `{"mode":{"value":"strict"}}`},
		{name: "nested array", raw: `{"mode":["strict"]}`},
		{name: "null value", raw: `{"mode":null}`},
		{name: "float", raw: `{"count":1.0}`},
		{name: "exponent", raw: `{"count":1e0}`},
		{name: "int64 overflow", raw: `{"count":9223372036854775808}`},
		{name: "raw invalid UTF-8", raw: invalidUTF8},
		{name: "unpaired high surrogate", raw: `{"mode":"\uD800"}`},
		{name: "unpaired low surrogate", raw: `{"mode":"\uDC00"}`},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if got, err := parseArchonToolParametersJSON(test.raw); err == nil {
				t.Fatalf("invalid Tool parameter frame parsed as %#v", got)
			}
		})
	}
}

func TestArchonToolParameterErrorsAreStructuredAndLeaveThePairUntouched(t *testing.T) {
	invalid := []struct {
		name   string
		params string
	}{
		{name: "duplicate key", params: `{"mode":"strict","mode":"strict"}`},
		{name: "nested value", params: `{"mode":["strict"]}`},
		{name: "unpaired surrogate", params: `{"mode":"\uD800"}`},
		{name: "descriptor-invalid value", params: `{"mode":"lenient"}`},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			harness := newArchonToolAuthoringHarness(t, true, false)
			before := snapshotArchonToolParityTree(t, harness.workspace)
			beforeFiles := snapshotArchonToolPairFileIdentity(t, harness)
			stdout, stderr, code := harness.run(
				t,
				"tool", "create", harness.slug,
				"--profile-id", "json.normalize",
				"--profile-version", "1",
				"--title", "Must not persist",
				"--params-json", test.params,
				"--json",
			)
			assertArchonToolJSONError(t, stdout, stderr, code, "invalid_tool_mutation", "tool", "json.normalize@1")
			if after := snapshotArchonToolParityTree(t, harness.workspace); !reflect.DeepEqual(after, before) {
				t.Fatalf("invalid Tool parameters changed definition pair:\n before=%#v\n after=%#v", before, after)
			}
			assertArchonToolPairFileIdentity(t, harness, beforeFiles)
		})
	}
}

func TestArchonToolProfileAndUpdateParameterMappingsCannotBeIgnored(t *testing.T) {
	unknownTuples := []struct {
		name, profileID, profileVersion string
	}{
		{name: "unknown profile", profileID: "json.unknown", profileVersion: "1"},
		{name: "unknown version", profileID: "json.normalize", profileVersion: "999"},
	}
	for _, test := range unknownTuples {
		t.Run(test.name, func(t *testing.T) {
			harness := newArchonToolAuthoringHarness(t, true, false)
			before := snapshotArchonToolParityTree(t, harness.workspace)
			stdout, stderr, code := harness.run(
				t,
				"tool", "create", harness.slug,
				"--profile-id", test.profileID,
				"--profile-version", test.profileVersion,
				"--title", "Unknown tuple must not persist",
				"--params-json", `{"mode":"strict"}`,
				"--json",
			)
			assertArchonToolJSONError(t, stdout, stderr, code, "invalid_tool_mutation", "tool", test.profileID+"@"+test.profileVersion)
			if after := snapshotArchonToolParityTree(t, harness.workspace); !reflect.DeepEqual(after, before) {
				t.Fatalf("unknown Tool tuple changed definition pair:\n before=%#v\n after=%#v", before, after)
			}
		})
	}

	invalidUpdates := []struct {
		name, params string
	}{
		{name: "valid title with invalid parameter value", params: `{"mode":"lenient"}`},
		{name: "valid title with explicit empty parameters", params: `{}`},
		{name: "valid title with malformed empty parameter frame", params: ``},
	}
	for _, test := range invalidUpdates {
		t.Run(test.name, func(t *testing.T) {
			harness := newArchonToolAuthoringHarness(t, true, false)
			before := snapshotArchonToolParityTree(t, harness.workspace)
			stdout, stderr, code := harness.run(
				t,
				"tool", "update", harness.slug, "tool_normalize",
				"--title", "Valid replacement title",
				"--params-json", test.params,
				"--json",
			)
			assertArchonToolJSONError(t, stdout, stderr, code, "invalid_tool_mutation", "tool", "tool_normalize")
			if after := snapshotArchonToolParityTree(t, harness.workspace); !reflect.DeepEqual(after, before) {
				t.Fatalf("invalid Tool update parameters changed definition pair:\n before=%#v\n after=%#v", before, after)
			}
		})
	}
}

func TestArchonToolUsageAndStableErrorCodes(t *testing.T) {
	harness := newArchonToolAuthoringHarness(t, true, false)
	usage := []struct {
		name string
		args []string
		want string
	}{
		{name: "create arity", args: []string{"tool", "create"}, want: "usage: archon tool create"},
		{name: "update arity", args: []string{"tool", "update", harness.slug}, want: "usage: archon tool update"},
		{name: "delete arity", args: []string{"tool", "delete", harness.slug}, want: "usage: archon tool delete"},
		{name: "inspect arity", args: []string{"tool", "inspect", harness.slug}, want: "usage: archon tool inspect"},
		{name: "unknown verb", args: []string{"tool", "launch"}, want: `unknown tool command "launch"`},
		{name: "no run authority", args: []string{"tool", "run"}, want: `unknown tool command "run"`},
		{name: "no spawn authority", args: []string{"tool", "spawn"}, want: `unknown tool command "spawn"`},
		{name: "no attach authority", args: []string{"tool", "attach"}, want: `unknown tool command "attach"`},
		{name: "no exec authority", args: []string{"tool", "exec"}, want: `unknown tool command "exec"`},
	}
	for _, test := range usage {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr, code := harness.run(t, test.args...)
			if code != 2 || stdout != "" || !strings.Contains(stderr, test.want) {
				t.Fatalf("Tool usage code=%d stdout=%q stderr=%q, want code 2 and %q", code, stdout, stderr, test.want)
			}
		})
	}

	if got := archonErrorCode(formations.ErrInvalidToolMutation); got != "invalid_tool_mutation" {
		t.Fatalf("InvalidToolMutation Archon code = %q", got)
	}
	if got := archonErrorCode(formations.ErrDefinitionPublicationUncertain); got != "definition_publication_uncertain" {
		t.Fatalf("DefinitionPublicationUncertain Archon code = %q", got)
	}
	joined := errors.Join(formations.ErrInvalidToolMutation, formations.ErrDefinitionPublicationUncertain)
	if got := archonErrorCode(joined); got != "definition_publication_uncertain" {
		t.Fatalf("joined Tool publication error code = %q, want uncertainty precedence", got)
	}
	private := fmt.Errorf("private reconciliation detail: %w", joined)
	response := archonErrorFromError(private, "tool", "tool_target")
	if response.Message != "Reload both board and layout before any explicit retry" ||
		strings.Contains(response.Message, "private") || strings.Contains(response.Message, formations.ErrDefinitionPublicationUncertain.Error()) {
		t.Fatalf("publication uncertainty JSON message = %q, want exact safe guidance", response.Message)
	}
	var textError bytes.Buffer
	if code := failJSON(&textError, private, false, "tool", "tool_target"); code != 1 ||
		textError.String() != "Reload both board and layout before any explicit retry\n" {
		t.Fatalf("publication uncertainty text error code=%d message=%q, want safe guidance", code, textError.String())
	}
}

func TestArchonToolUsageRejectsMisplacedPlacementAndPublicCASFlagsWithoutMutation(t *testing.T) {
	tests := []struct {
		name string
		args func(string) []string
	}{
		{
			name: "update exact placement",
			args: func(slug string) []string {
				return []string{"tool", "update", slug, "tool_normalize", "--title", "No move", "--x", "10", "--y", "20"}
			},
		},
		{
			name: "update heuristic placement",
			args: func(slug string) []string {
				return []string{"tool", "update", slug, "tool_normalize", "--title", "No move", "--predecessor-node-id", "tool_sink"}
			},
		},
		{
			name: "delete exact placement",
			args: func(slug string) []string {
				return []string{"tool", "delete", slug, "tool_normalize", "--x", "10", "--y", "20"}
			},
		},
		{
			name: "delete heuristic placement",
			args: func(slug string) []string {
				return []string{"tool", "delete", slug, "tool_normalize", "--successor-node-id", "gate_review"}
			},
		},
		{
			name: "expected revision flag",
			args: func(slug string) []string {
				return []string{"tool", "update", slug, "tool_normalize", "--title", "No public CAS", "--expected-rev", "4"}
			},
		},
		{
			name: "expected board etag flag",
			args: func(slug string) []string {
				return []string{"tool", "update", slug, "tool_normalize", "--title", "No public CAS", "--expected-etag", "etag"}
			},
		},
		{
			name: "layout state flag",
			args: func(slug string) []string {
				return []string{"tool", "update", slug, "tool_normalize", "--title", "No public CAS", "--layout-state", "present"}
			},
		},
		{
			name: "layout etag flag",
			args: func(slug string) []string {
				return []string{"tool", "update", slug, "tool_normalize", "--title", "No public CAS", "--layout-etag", "etag"}
			},
		},
		{
			name: "HTTP if-match flag",
			args: func(slug string) []string {
				return []string{"tool", "update", slug, "tool_normalize", "--title", "No public CAS", "--if-match", "etag"}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newArchonToolAuthoringHarness(t, true, false)
			before := snapshotArchonToolParityTree(t, harness.workspace)
			stdout, stderr, code := harness.run(t, test.args(harness.slug)...)
			if code != 2 || stdout != "" || !strings.Contains(stderr, "flag provided but not defined") {
				t.Fatalf("rejected Tool flag code=%d stdout=%q stderr=%q, want usage exit 2", code, stdout, stderr)
			}
			if after := snapshotArchonToolParityTree(t, harness.workspace); !reflect.DeepEqual(after, before) {
				t.Fatalf("rejected Tool flag changed definition pair:\n before=%#v\n after=%#v", before, after)
			}
		})
	}
}

type archonToolAuthoringHarness struct {
	workspace string
	slug      string
	store     *formations.Store
	runner    *fakeTmux
}

type archonToolPairFileIdentity struct {
	board  os.FileInfo
	layout os.FileInfo
}

func newArchonToolAuthoringHarness(t *testing.T, withLayout, withConnection bool) *archonToolAuthoringHarness {
	t.Helper()
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	const slug = "tool-parity"
	boardRaw := archonToolParityBoardFixture()
	layoutRaw := archonToolParityLayoutFixture()
	if withConnection {
		boardRaw += `
[[connection]]
id = "edge_tool"
from = "tool_normalize:port_tool_out"
to = "tool_sink:port_sink_in"
`
		layoutRaw += `
[[edge]]
id = "edge_tool"
lane = "work"
`
	}
	writeArchonFile(t, store.BoardPath(slug), boardRaw)
	if withLayout {
		writeArchonFile(t, store.LayoutPath(slug), layoutRaw)
	}
	return &archonToolAuthoringHarness{
		workspace: workspace,
		slug:      slug,
		store:     store,
		runner:    &fakeTmux{live: map[string]bool{}},
	}
}

func (h *archonToolAuthoringHarness) run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runtimeStoreCalls := 0
	allArgs := append([]string{"--workspace", h.workspace}, args...)
	code := runWithRuntimeStoreFactory(allArgs, &stdout, &stderr, h.runner, func(workspace string) *formations.Store {
		runtimeStoreCalls++
		return formations.NewStore(workspace)
	})
	if runtimeStoreCalls != 0 {
		t.Fatalf("Tool definition command reached runtime Store factory %d time(s)", runtimeStoreCalls)
	}
	if h.runner.liveSessionCalls != 0 || len(h.runner.spawned) != 0 || len(h.runner.attach) != 0 {
		t.Fatalf("Tool definition command reached tmux: live=%d spawned=%v attach=%v", h.runner.liveSessionCalls, h.runner.spawned, h.runner.attach)
	}
	return stdout.String(), stderr.String(), code
}

func snapshotArchonToolPairFileIdentity(t *testing.T, harness *archonToolAuthoringHarness) archonToolPairFileIdentity {
	t.Helper()
	board, err := os.Stat(harness.store.BoardPath(harness.slug))
	if err != nil {
		t.Fatalf("stat Tool board identity: %v", err)
	}
	layout, err := os.Stat(harness.store.LayoutPath(harness.slug))
	if err != nil {
		t.Fatalf("stat Tool layout identity: %v", err)
	}
	return archonToolPairFileIdentity{board: board, layout: layout}
}

func assertArchonToolPairFileIdentity(t *testing.T, harness *archonToolAuthoringHarness, before archonToolPairFileIdentity) {
	t.Helper()
	after := snapshotArchonToolPairFileIdentity(t, harness)
	for name, pair := range map[string][2]os.FileInfo{
		"board":  {before.board, after.board},
		"layout": {before.layout, after.layout},
	} {
		if !os.SameFile(pair[0], pair[1]) || pair[0].Mode() != pair[1].Mode() || !pair[0].ModTime().Equal(pair[1].ModTime()) {
			t.Fatalf("rejected Tool command changed %s file identity: before=%v after=%v", name, pair[0], pair[1])
		}
	}
}

func assertArchonToolJSONError(t *testing.T, stdout, stderr string, code int, wantCode, wantBoundary, wantSelector string) {
	t.Helper()
	if code != 1 || stdout != "" {
		t.Fatalf("Tool error code=%d stdout=%q stderr=%q, want exit 1 and empty stdout", code, stdout, stderr)
	}
	var response archonErrorResponse
	if err := json.Unmarshal([]byte(stderr), &response); err != nil {
		t.Fatalf("decode Tool JSON error: %v\nstderr=%s", err, stderr)
	}
	if response.Code != wantCode || response.Boundary != wantBoundary || response.Selector != wantSelector || response.Message == "" {
		t.Fatalf("Tool JSON error = %#v, want code=%q boundary=%q selector=%q", response, wantCode, wantBoundary, wantSelector)
	}
}

func assertArchonToolJSONRootKeys(t *testing.T, raw string, want ...string) {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &object); err != nil {
		t.Fatalf("decode Tool JSON root: %v\n%s", err, raw)
	}
	assertArchonToolExactJSONKeys(t, object, want...)
}

func assertArchonToolExactJSONKeys(t *testing.T, object map[string]json.RawMessage, want ...string) {
	t.Helper()
	wantSet := make(map[string]bool, len(want))
	for _, key := range want {
		wantSet[key] = true
	}
	if len(object) != len(wantSet) {
		t.Fatalf("JSON keys = %v, want exactly %v", reflect.ValueOf(object).MapKeys(), want)
	}
	for key := range object {
		if !wantSet[key] {
			t.Fatalf("unexpected JSON key %q; want exactly %v", key, want)
		}
	}
}

func mustReadArchonToolBoard(t *testing.T, store *formations.Store, slug string) *formations.BoardDocument {
	t.Helper()
	board, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read Tool board %q: %v", slug, err)
	}
	return board
}

func mustReadArchonToolLayout(t *testing.T, store *formations.Store, slug string) *formations.LayoutDocument {
	t.Helper()
	layout, err := store.ReadLayout(slug)
	if err != nil {
		t.Fatalf("read Tool layout %q: %v", slug, err)
	}
	return layout
}

func assertArchonToolCanonicalPair(t *testing.T, harness *archonToolAuthoringHarness, board *formations.BoardDocument, layout *formations.LayoutDocument) {
	t.Helper()
	if board == nil {
		t.Fatal("Tool mutation returned nil board")
	}
	if board.TOML != "" {
		t.Fatal("Tool mutation response retained raw board TOML")
	}
	persistedBoard := mustReadArchonToolBoard(t, harness.store, harness.slug)
	persistedBoard.TOML = ""
	if !reflect.DeepEqual(persistedBoard, board) {
		t.Fatalf("Tool mutation board response differs from canonical reread:\n response=%#v\n persisted=%#v", board, persistedBoard)
	}
	if layout == nil {
		assertArchonToolLayoutAbsent(t, harness)
		return
	}
	if layout.TOML != "" {
		t.Fatal("Tool mutation response retained raw layout TOML")
	}
	persistedLayout := mustReadArchonToolLayout(t, harness.store, harness.slug)
	persistedLayout.TOML = ""
	if !reflect.DeepEqual(persistedLayout, layout) {
		t.Fatalf("Tool mutation layout response differs from canonical reread:\n response=%#v\n persisted=%#v", layout, persistedLayout)
	}
}

func assertArchonToolLayoutAbsent(t *testing.T, harness *archonToolAuthoringHarness) {
	t.Helper()
	_, err := os.Stat(harness.store.LayoutPath(harness.slug))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Tool mutation changed absent layout state: stat err=%v", err)
	}
}

func assertArchonToolRetainedLayoutNodes(t *testing.T, before, after *formations.LayoutDocument, excluded map[string]bool) {
	t.Helper()
	if before == nil || after == nil {
		t.Fatalf("cannot compare nil retained layouts: before=%#v after=%#v", before, after)
	}
	afterByID := make(map[string]formations.LayoutNode, len(after.Nodes))
	for _, node := range after.Nodes {
		afterByID[node.ID] = node
	}
	for _, node := range before.Nodes {
		if excluded[node.ID] {
			continue
		}
		if got, ok := afterByID[node.ID]; !ok || got != node {
			t.Fatalf("Tool mutation moved or removed retained node %q: got %#v found=%t want %#v", node.ID, got, ok, node)
		}
	}
}

func archonToolLayoutNode(layout *formations.LayoutDocument, id string) (formations.LayoutNode, bool) {
	if layout == nil {
		return formations.LayoutNode{}, false
	}
	for _, node := range layout.Nodes {
		if node.ID == id {
			return node, true
		}
	}
	return formations.LayoutNode{}, false
}

func assertArchonToolLayoutInventory(
	t *testing.T,
	layout *formations.LayoutDocument,
	wantNodes []formations.LayoutNode,
	wantEdges []formations.LayoutEdge,
) {
	t.Helper()
	if layout == nil {
		t.Fatal("Tool mutation returned nil layout inventory")
	}
	if !reflect.DeepEqual(layout.Nodes, wantNodes) {
		t.Fatalf("Tool layout nodes/order = %#v, want %#v", layout.Nodes, wantNodes)
	}
	if !reflect.DeepEqual(layout.Edges, wantEdges) {
		t.Fatalf("Tool layout edges/order = %#v, want %#v", layout.Edges, wantEdges)
	}
	seenNodes := make(map[string]bool, len(layout.Nodes))
	for _, node := range layout.Nodes {
		if seenNodes[node.ID] {
			t.Fatalf("Tool layout contains duplicate node id %q: %#v", node.ID, layout.Nodes)
		}
		seenNodes[node.ID] = true
	}
	seenEdges := make(map[string]bool, len(layout.Edges))
	for _, edge := range layout.Edges {
		if seenEdges[edge.ID] {
			t.Fatalf("Tool layout contains duplicate edge id %q: %#v", edge.ID, layout.Edges)
		}
		seenEdges[edge.ID] = true
	}
}
