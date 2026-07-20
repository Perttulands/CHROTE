package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chrote/server/internal/formations"
)

func TestArchonUsesExistingInspectValidateWireAndArrangeSurfacesForTools(t *testing.T) {
	workspace := t.TempDir()
	store := formations.NewStore(workspace)
	writeArchonFile(t, store.BoardPath("tool-parity"), archonToolParityBoardFixture())
	writeArchonFile(t, store.LayoutPath("tool-parity"), archonToolParityLayoutFixture())
	boardBytesBeforeRead := readArchonFile(t, store.BoardPath("tool-parity"))
	layoutBytesBeforeRead := readArchonFile(t, store.LayoutPath("tool-parity"))
	runner := &fakeTmux{live: map[string]bool{}}

	stdout, stderr, code := runArchon(t, runner, "--workspace", workspace, "board", "inspect", "tool-parity", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("Tool board inspect code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var inspected formations.BoardDocument
	if err := json.Unmarshal([]byte(stdout), &inspected); err != nil {
		t.Fatalf("decode Tool board inspection: %v\n%s", err, stdout)
	}
	assertArchonToolParityProjection(t, inspected)
	if strings.Contains(stdout, `"toml"`) {
		t.Fatalf("Tool board inspection leaked raw TOML: %s", stdout)
	}
	if got := readArchonFile(t, store.BoardPath("tool-parity")); got != boardBytesBeforeRead {
		t.Fatalf("Tool inspection changed board bytes:\n%s", got)
	}
	if got := readArchonFile(t, store.LayoutPath("tool-parity")); got != layoutBytesBeforeRead {
		t.Fatalf("Tool inspection changed layout bytes:\n%s", got)
	}
	invalidRaw := archonInvalidToolValidationFixture()
	writeArchonFile(t, store.BoardPath("tool-invalid"), invalidRaw)
	invalidBytesBeforeValidation := readArchonFile(t, store.BoardPath("tool-invalid"))
	workspaceBeforeValidation := snapshotArchonToolParityTree(t, workspace)
	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "board", "validate", "tool-invalid", "--json")
	if code != 1 || stderr != "" {
		t.Fatalf("invalid Tool validate code=%d stdout=%s stderr=%s, want frozen invalid_tool finding", code, stdout, stderr)
	}
	var invalidValidation struct {
		Errors []formations.BoardFinding `json:"errors"`
	}
	if err := json.Unmarshal([]byte(stdout), &invalidValidation); err != nil {
		t.Fatalf("decode invalid Tool validation: %v\n%s", err, stdout)
	}
	if len(invalidValidation.Errors) != 1 || invalidValidation.Errors[0].Code != formations.FindingInvalidTool ||
		invalidValidation.Errors[0].NodeID != "tool_invalid" ||
		!strings.Contains(invalidValidation.Errors[0].Message, `unknown profile tuple "json.normalize"@"999"`) {
		t.Fatalf("invalid Tool findings = %+v, want exact-tuple invalid_tool cause", invalidValidation.Errors)
	}
	if got := readArchonFile(t, store.BoardPath("tool-invalid")); got != invalidBytesBeforeValidation {
		t.Fatalf("Tool validation changed invalid board bytes:\n%s", got)
	}
	if got := snapshotArchonToolParityTree(t, workspace); !reflect.DeepEqual(got, workspaceBeforeValidation) {
		t.Fatalf("Tool validation changed workspace tree:\n before=%+v\n after=%+v", workspaceBeforeValidation, got)
	}

	stdout, stderr, code = runArchon(
		t,
		runner,
		"--workspace", workspace,
		"formation", "wire", "tool-parity", "tool_normalize:port_tool_out", "tool_sink:port_sink_in", "--json",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("Tool generic wire code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var wired formations.BoardDocument
	if err := json.Unmarshal([]byte(stdout), &wired); err != nil {
		t.Fatalf("decode Tool wire result: %v\n%s", err, stdout)
	}
	assertArchonToolParityProjection(t, wired)
	if len(wired.Connections) != 1 || wired.Connections[0].ID == "" ||
		wired.Connections[0].From != "tool_normalize:port_tool_out" || wired.Connections[0].To != "tool_sink:port_sink_in" {
		t.Fatalf("Tool connection = %+v, want exact Tool-output to Tool-input endpoints", wired.Connections)
	}
	if got := readArchonFile(t, store.LayoutPath("tool-parity")); got != layoutBytesBeforeRead {
		t.Fatalf("Tool wire changed layout bytes:\n%s", got)
	}

	stdout, stderr, code = runArchon(
		t,
		runner,
		"--workspace", workspace,
		"formation", "unwire", "tool-parity", "tool_normalize:port_tool_out", "tool_sink:port_sink_in", "--json",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("Tool generic unwire code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var unwired formations.BoardDocument
	if err := json.Unmarshal([]byte(stdout), &unwired); err != nil {
		t.Fatalf("decode Tool unwire result: %v\n%s", err, stdout)
	}
	if len(unwired.Connections) != 0 {
		t.Fatalf("Tool connections after unwire = %+v, want none", unwired.Connections)
	}
	persistedUnwired, err := store.ReadBoard("tool-parity")
	if err != nil {
		t.Fatalf("read persisted Tool board after unwire: %v", err)
	}
	if persistedUnwired.ETag != unwired.ETag || len(persistedUnwired.Connections) != 0 {
		t.Fatalf("Tool unwire response does not match persisted board: response=%+v persisted=%+v", unwired.Connections, persistedUnwired.Connections)
	}

	stdout, stderr, code = runArchon(
		t,
		runner,
		"--workspace", workspace,
		"formation", "wire", "tool-parity", "tool_normalize:port_tool_out", "gate_review:in", "--json",
	)
	if code != 0 || stderr != "" {
		t.Fatalf("Tool generic rewire setup code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var gateWired formations.BoardDocument
	if err := json.Unmarshal([]byte(stdout), &gateWired); err != nil {
		t.Fatalf("decode Tool-to-Gate wire result: %v\n%s", err, stdout)
	}
	if len(gateWired.Connections) != 1 || gateWired.Connections[0].From != "tool_normalize:port_tool_out" ||
		gateWired.Connections[0].To != "gate_review:in" {
		t.Fatalf("Tool-to-Gate wire result = %+v, want sole exact connection", gateWired.Connections)
	}
	persistedGateWired, err := store.ReadBoard("tool-parity")
	if err != nil {
		t.Fatalf("read persisted Tool-to-Gate board: %v", err)
	}
	if persistedGateWired.ETag != gateWired.ETag || len(persistedGateWired.Connections) != 1 ||
		persistedGateWired.Connections[0] != gateWired.Connections[0] {
		t.Fatalf("Tool-to-Gate response does not match persisted board: response=%+v persisted=%+v", gateWired.Connections, persistedGateWired.Connections)
	}
	boardBytesBeforeArrange := readArchonFile(t, store.BoardPath("tool-parity"))
	stdout, stderr, code = runArchon(t, runner, "--workspace", workspace, "board", "arrange", "tool-parity", "--json")
	if code != 0 || stderr != "" {
		t.Fatalf("Tool board arrange code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var arranged formations.LayoutDocument
	if err := json.Unmarshal([]byte(stdout), &arranged); err != nil {
		t.Fatalf("decode Tool arrangement: %v\n%s", err, stdout)
	}
	positions := make(map[string]formations.LayoutNode, len(arranged.Nodes))
	counts := make(map[string]int, len(arranged.Nodes))
	for _, node := range arranged.Nodes {
		positions[node.ID] = node
		counts[node.ID]++
	}
	if len(arranged.Nodes) != 4 || len(positions) != 4 ||
		counts["mis_main"] != 1 || counts["tool_normalize"] != 1 || counts["tool_sink"] != 1 || counts["gate_review"] != 1 ||
		positions["tool_normalize"].X >= positions["gate_review"].X {
		t.Fatalf("arranged Tool topology nodes=%+v counts=%+v, want four exact board nodes with Tool left of connected Gate", arranged.Nodes, counts)
	}
	if got := readArchonFile(t, store.BoardPath("tool-parity")); got != boardBytesBeforeArrange {
		t.Fatalf("Tool arrange changed board bytes:\n%s", got)
	}
	if runner.liveSessionCalls != 0 || len(runner.spawned) != 0 || len(runner.attach) != 0 {
		t.Fatalf("Tool definition commands reached tmux: live=%d spawned=%v attach=%v", runner.liveSessionCalls, runner.spawned, runner.attach)
	}
}

func assertArchonToolParityProjection(t *testing.T, board formations.BoardDocument) {
	t.Helper()
	if board.Schema != formations.CurrentBoardSchema || len(board.Tools) != 2 {
		t.Fatalf("Tool board projection = schema %d tools %+v", board.Schema, board.Tools)
	}
	tool := board.Tools[0]
	if tool.ID != "tool_normalize" || tool.Title != "Normalize report" ||
		tool.ProfileID != "json.normalize" || tool.ProfileVersion != "1" ||
		len(tool.Params) != 1 || tool.Params["mode"] != "strict" ||
		len(tool.Inputs) != 1 || len(tool.Outputs) != 1 {
		t.Fatalf("Tool projection = %+v, want exact json.normalize@1 definition", tool)
	}
	input := tool.Inputs[0]
	if input.ID != "port_tool_in" || input.Name != "input" || input.Label != "Report" ||
		input.Direction != "input" || input.Kind != "work" ||
		len(input.AcceptedMediaTypes) != 1 || input.AcceptedMediaTypes[0] != "application/json" ||
		input.Required == nil || !*input.Required || input.Role == nil || *input.Role != "data" {
		t.Fatalf("Tool input projection = %+v", input)
	}
	output := tool.Outputs[0]
	if output.ID != "port_tool_out" || output.Name != "output" || output.Label != "Normalized report" ||
		output.Direction != "output" || output.Kind != "work" ||
		len(output.AcceptedMediaTypes) != 1 || output.AcceptedMediaTypes[0] != "application/json" ||
		output.Required != nil || output.Role != nil {
		t.Fatalf("Tool output projection = %+v", output)
	}
	sink := board.Tools[1]
	if sink.ID != "tool_sink" || sink.Title != "Receive normalized report" ||
		sink.ProfileID != "json.normalize" || sink.ProfileVersion != "1" ||
		len(sink.Params) != 1 || sink.Params["mode"] != "strict" ||
		len(sink.Inputs) != 1 || sink.Inputs[0].ID != "port_sink_in" || sink.Inputs[0].Name != "input" ||
		len(sink.Outputs) != 1 || sink.Outputs[0].ID != "port_sink_out" || sink.Outputs[0].Name != "output" {
		t.Fatalf("second Tool projection = %+v, want exact json.normalize@1 sink definition", sink)
	}
}

func archonToolParityBoardFixture() string {
	return `schema = 2
id = "brd_tool_parity"
slug = "tool-parity"
title = "Tool parity"
rev = 4
updatedBy = "agent:test"
updatedAt = "2026-07-20T00:00:00Z"

[[mission]]
id = "mis_main"
title = "Main"
goal = "Inspect the Tool"
beadId = "ctx-ug7.8.1"

[[tool]]
id = "tool_normalize"
title = "Normalize report"
profileId = "json.normalize"
profileVersion = "1"

[tool.params]
mode = "strict"

[[tool.input]]
id = "port_tool_in"
name = "input"
label = "Report"
direction = "input"
kind = "work"
acceptedMediaTypes = ["application/json"]
required = true
role = "data"

[[tool.output]]
id = "port_tool_out"
name = "output"
label = "Normalized report"
direction = "output"
kind = "work"
acceptedMediaTypes = ["application/json"]

[[tool]]
id = "tool_sink"
title = "Receive normalized report"
profileId = "json.normalize"
profileVersion = "1"

[tool.params]
mode = "strict"

[[tool.input]]
id = "port_sink_in"
name = "input"
label = "Report"
direction = "input"
kind = "work"
acceptedMediaTypes = ["application/json"]
required = true
role = "data"

[[tool.output]]
id = "port_sink_out"
name = "output"
label = "Normalized report"
direction = "output"
kind = "work"
acceptedMediaTypes = ["application/json"]

[[gate]]
id = "gate_review"
title = "Review"
kinds = ["human"]
criterion = "Normalized report is acceptable"
`
}

func archonToolParityLayoutFixture() string {
	return `schema = 1
boardId = "brd_tool_parity"
boardRev = 4
updatedAt = "2026-07-20T00:00:00Z"

[[node]]
id = "mis_main"
x = 700
y = 420

[[node]]
id = "tool_normalize"
x = 500
y = 420

[[node]]
id = "tool_sink"
x = 400
y = 420

[[node]]
id = "gate_review"
x = 300
y = 420
`
}

func archonInvalidToolValidationFixture() string {
	return `schema = 2
id = "brd_tool_invalid"
slug = "tool-invalid"
title = "Invalid Tool validation"
rev = 1

[[mission]]
id = "mis_invalid"
title = "Invalid Tool fixture"
goal = "Inspect one invalid Tool"
beadId = "ctx-ug7.8.1"

[[tool]]
id = "tool_invalid"
title = "Invalid profile version"
profileId = "json.normalize"
profileVersion = "999"

[tool.params]
mode = "strict"

[[tool.input]]
id = "port_invalid_in"
name = "input"
label = "Report"
direction = "input"
kind = "work"
acceptedMediaTypes = ["application/json"]
required = false
role = "data"

[[tool.output]]
id = "port_invalid_out"
name = "output"
label = "Normalized report"
direction = "output"
kind = "work"
acceptedMediaTypes = ["application/json"]
`
}

func snapshotArchonToolParityTree(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string)
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if entry.IsDir() {
			snapshot[relative] = "directory"
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snapshot[relative] = string(raw)
		return nil
	}); err != nil {
		t.Fatalf("snapshot Tool parity workspace: %v", err)
	}
	return snapshot
}
