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
			if strings.Contains(stdout, `"toml"`) || result.Board.TOML != "" || result.Layout.TOML != "" {
				t.Fatalf("Tool create leaked raw TOML: %s", stdout)
			}
			assertArchonToolCanonicalPair(t, harness, result.Board, result.Layout)
		})
	}
}

func TestArchonToolCreatePlacementUnionIsStoreValidatedAndNeverReflowsExistingNodes(t *testing.T) {
	validHints := []struct {
		name  string
		flags []string
	}{
		{name: "predecessor only", flags: []string{"--predecessor-node-id", "tool_sink"}},
		{name: "successor only", flags: []string{"--successor-node-id", "gate_review"}},
		{name: "predecessor and successor", flags: []string{"--predecessor-node-id", "tool_sink", "--successor-node-id", "gate_review"}},
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
			if _, found := archonToolLayoutNode(result.Layout, result.Tool.ID); !found {
				t.Fatalf("hinted Tool %q has no new layout node: %#v", result.Tool.ID, result.Layout.Nodes)
			}
			assertArchonToolRetainedLayoutNodes(t, beforeLayout, result.Layout, nil)
			if !reflect.DeepEqual(result.Board.Connections, beforeBoard.Connections) {
				t.Fatalf("placement hints implicitly wired the Tool:\n before=%#v\n after=%#v", beforeBoard.Connections, result.Board.Connections)
			}
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
			args := []string{"tool", "update", harness.slug, "tool_normalize", "--json"}
			args = append(args, test.flags...)
			stdout, stderr, code := harness.run(t, args...)
			assertArchonToolJSONError(t, stdout, stderr, code, "invalid_tool_mutation", "tool", "tool_normalize")
			if after := snapshotArchonToolParityTree(t, harness.workspace); !reflect.DeepEqual(after, before) {
				t.Fatalf("invalid Tool update changed definition pair:\n before=%#v\n after=%#v", before, after)
			}
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
			var response struct {
				Board archonBoardIdentity `json:"board"`
				Tool  formations.ToolNode `json:"tool"`
			}
			if err := json.Unmarshal([]byte(stdout), &response); err != nil {
				t.Fatalf("decode Tool inspect %q: %v\n%s", selector, err, stdout)
			}
			if response.Board.ID != "brd_tool_parity" || response.Board.Slug != harness.slug || response.Board.Rev != 4 || response.Board.ETag == "" {
				t.Fatalf("Tool inspect board identity = %#v", response.Board)
			}
			if response.Tool.ID != "tool_normalize" || response.Tool.Title != "Normalize report" ||
				response.Tool.ProfileID != "json.normalize" || response.Tool.ProfileVersion != "1" ||
				response.Tool.Params["mode"] != "strict" || len(response.Tool.Inputs) != 1 || len(response.Tool.Outputs) != 1 {
				t.Fatalf("Tool inspect projection = %#v", response.Tool)
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
	if code != 0 || stderr != "" || stdout != toolID+"\tText Renamed\tjson.normalize@1\n" {
		t.Fatalf("Tool inspect text code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	stdout, stderr, code = harness.run(t, "tool", "delete", harness.slug, toolID)
	if code != 0 || stderr != "" || stdout != "deleted "+toolID+"\n" {
		t.Fatalf("Tool delete text code=%d stdout=%q stderr=%q", code, stdout, stderr)
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

type archonToolAuthoringHarness struct {
	workspace string
	slug      string
	store     *formations.Store
	runner    *fakeTmux
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
