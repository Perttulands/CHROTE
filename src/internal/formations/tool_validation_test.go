package formations

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestToolStructuralSchemaOneInspectionPreservesSourceAndLayoutBeforeRejection(t *testing.T) {
	store := NewStore(t.TempDir())
	raw := strings.Replace(toolStructuralConnectedBoardFixture(), "schema = 2", "schema = 1", 1)
	layoutRaw := toolStructuralLayoutFixture()
	boardPath := store.BoardPath("tool-structural")
	layoutPath := store.LayoutPath("tool-structural")
	writeFixture(t, boardPath, raw)
	writeFixture(t, layoutPath, layoutRaw)
	wantBoardIdentity := operativeFileIdentityForTest(t, boardPath)
	wantLayoutIdentity := operativeFileIdentityForTest(t, layoutPath)

	board, err := store.ReadBoard("tool-structural")
	if err != nil {
		t.Fatalf("inspect schema-1 board containing a Tool: %v", err)
	}
	wantProjection, err := json.Marshal(board)
	if err != nil {
		t.Fatalf("marshal schema-1 Tool projection before validation: %v", err)
	}
	if report := ValidateBoard(board); len(report.Errors) == 0 {
		t.Fatal("structural validation accepted a Tool under board schema 1")
	}
	gotProjection, err := json.Marshal(board)
	if err != nil {
		t.Fatalf("marshal schema-1 Tool projection after validation: %v", err)
	}
	if string(gotProjection) != string(wantProjection) {
		t.Fatalf("structural validation mutated the inspected board:\n got %s\nwant %s", gotProjection, wantProjection)
	}
	assertToolStructuralFileIdentity(t, boardPath, raw, wantBoardIdentity)
	assertToolStructuralFileIdentity(t, layoutPath, layoutRaw, wantLayoutIdentity)
}

func TestToolStructuralValidSchemaTwoAndUnrelatedBoardsRemainAccepted(t *testing.T) {
	assertToolStructuralBoardAccepted(t, toolStructuralConnectedBoardFixture(), "valid schema-2 json.normalize@1 board")
	assertToolStructuralBoardAccepted(t, cleanValidateBoardFixture(), "unrelated schema-1 Tool-free board")
}

func TestToolStructuralRegistryTupleMustResolveExactly(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
	}{
		{name: "unknown profile", old: `profileId = "json.normalize"`, new: `profileId = "json.unknown"`},
		{name: "case-changed profile", old: `profileId = "json.normalize"`, new: `profileId = "JSON.normalize"`},
		{name: "profile alias", old: `profileId = "json.normalize"`, new: `profileId = "json.normalise"`},
		{name: "unknown version", old: `profileVersion = "1"`, new: `profileVersion = "2"`},
		{name: "case-changed version", old: `profileVersion = "1"`, new: `profileVersion = "ONE"`},
		{name: "version alias", old: `profileVersion = "1"`, new: `profileVersion = "01"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := replaceToolStructuralFixture(t, toolStructuralConnectedBoardFixture(), tt.old, tt.new)
			assertToolStructuralBoardRejected(t, raw, tt.name)
		})
	}
}

func TestToolStructuralJSONNormalizePortsBindFrozenShapeButNotLabels(t *testing.T) {
	labelsChanged := strings.NewReplacer(
		`label = "Report"`, `label = "Operator-selected source"`,
		`label = "Normalized report"`, `label = "Operator-selected result"`,
	).Replace(toolStructuralConnectedBoardFixture())
	assertToolStructuralBoardAccepted(t, labelsChanged, "display-only Tool labels")

	tests := []struct {
		name string
		old  string
		new  string
	}{
		{
			name: "missing input port",
			old: `[[tool.input]]
id = "port_tool_in"
name = "input"
label = "Report"
direction = "input"
kind = "work"
acceptedMediaTypes = ["application/json"]
required = true
role = "data"

`,
		},
		{
			name: "extra port changes descriptor sequence",
			old:  `[[tool.output]]`,
			new: `[[tool.input]]
id = "port_tool_extra"
name = "extra"
label = "Extra"
direction = "input"
kind = "work"
acceptedMediaTypes = ["application/json"]
required = false
role = "data"

[[tool.output]]`,
		},
		{name: "semantic name", old: `name = "input"`, new: `name = "source"`},
		{name: "direction", old: `direction = "input"`, new: `direction = "output"`},
		{name: "kind", old: `kind = "work"`, new: `kind = "gate_feedback"`},
		{name: "media value", old: `acceptedMediaTypes = ["application/json"]`, new: `acceptedMediaTypes = ["text/plain"]`},
		{name: "media cardinality", old: `acceptedMediaTypes = ["application/json"]`, new: `acceptedMediaTypes = ["application/json", "text/plain"]`},
		{name: "required presence", old: "required = true\n", new: ""},
		{name: "required value", old: `required = true`, new: `required = false`},
		{name: "role presence", old: "role = \"data\"\n", new: ""},
		{name: "role value", old: `role = "data"`, new: `role = "retry_control"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := replaceToolStructuralFixture(t, toolStructuralConnectedBoardFixture(), tt.old, tt.new)
			assertToolStructuralBoardRejected(t, raw, tt.name)
		})
	}
}

func TestToolStructuralIDsAreUniqueOnlyAtTheirAuthorityScope(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "empty port id",
			raw:  replaceToolStructuralFixture(t, toolStructuralIsolatedNodesFixture(), `id = "port_tool_in"`, `id = ""`),
		},
		{
			name: "duplicate port id within Tool",
			raw:  replaceToolStructuralFixture(t, toolStructuralIsolatedNodesFixture(), `id = "port_tool_out"`, `id = "port_tool_in"`),
		},
		{
			name: "duplicate Tool id",
			raw: toolStructuralIsolatedNodesFixture() + `
[[tool]]
id = "tool_normalize"
title = "Duplicate"
profileId = "json.normalize"
profileVersion = "1"

[tool.params]
mode = "strict"

[[tool.input]]
id = "port_duplicate_in"
name = "input"
label = "Report"
direction = "input"
kind = "work"
acceptedMediaTypes = ["application/json"]
required = true
role = "data"

[[tool.output]]
id = "port_duplicate_out"
name = "output"
label = "Normalized report"
direction = "output"
kind = "work"
acceptedMediaTypes = ["application/json"]
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertToolStructuralBoardRejected(t, tt.raw, tt.name)
		})
	}

	for _, collisionID := range []string{"mis_main", "fmn_worker", "gate_review"} {
		t.Run("cross-kind node id "+collisionID, func(t *testing.T) {
			raw := replaceToolStructuralFixture(t, toolStructuralIsolatedNodesFixture(), `id = "tool_normalize"`, `id = "`+collisionID+`"`)
			assertToolStructuralBoardRejected(t, raw, "cross-kind node id")
		})
	}

	portIDsReusedByAnotherTool := toolStructuralIsolatedNodesFixture() + `
[[tool]]
id = "tool_archive"
title = "Archive normalization"
profileId = "json.normalize"
profileVersion = "1"

[tool.params]
mode = "strict"

[[tool.input]]
id = "port_tool_in"
name = "input"
label = "Archive"
direction = "input"
kind = "work"
acceptedMediaTypes = ["application/json"]
required = true
role = "data"

[[tool.output]]
id = "port_tool_out"
name = "output"
label = "Normalized archive"
direction = "output"
kind = "work"
acceptedMediaTypes = ["application/json"]
`
	assertToolStructuralBoardAccepted(t, portIDsReusedByAnotherTool, "port instance ids reused by a different Tool")
}

func TestToolStructuralJSONNormalizeParametersAreCompleteAndStrict(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
	}{
		{name: "missing required mode", old: "mode = \"strict\"\n", new: ""},
		{name: "unknown parameter", old: "mode = \"strict\"\n", new: "mode = \"strict\"\nextra = true\n"},
		{name: "wrong boolean scalar", old: `mode = "strict"`, new: `mode = true`},
		{name: "wrong integer scalar", old: `mode = "strict"`, new: `mode = 1`},
		{name: "out of domain", old: `mode = "strict"`, new: `mode = "lenient"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := replaceToolStructuralFixture(t, toolStructuralConnectedBoardFixture(), tt.old, tt.new)
			assertToolStructuralBoardRejected(t, raw, tt.name)
		})
	}
}

func TestToolStructuralGenericParameterTypesAndConstraints(t *testing.T) {
	descriptor := toolStructuralScalarDescriptor()
	if err := validateToolProfileDescriptor(descriptor); err != nil {
		t.Fatalf("generic scalar descriptor is not contract-valid: %v", err)
	}
	valid := map[string]any{
		"text":    "data",
		"mode":    "strict",
		"enabled": true,
		"limit":   int64(2),
	}
	if err := validateToolNodeAgainstDescriptor(toolStructuralGenericNode(descriptor, valid), descriptor); err != nil {
		t.Fatalf("valid generic scalar parameters rejected: %v", err)
	}

	tests := []struct {
		name  string
		key   string
		value any
	}{
		{name: "string type", key: "text", value: true},
		{name: "string minimum bytes", key: "text", value: "x"},
		{name: "string maximum bytes", key: "text", value: "abcde"},
		{name: "string enum", key: "mode", value: "loose"},
		{name: "boolean type", key: "enabled", value: "true"},
		{name: "integer type", key: "limit", value: "2"},
		{name: "integer minimum", key: "limit", value: int64(-3)},
		{name: "integer maximum", key: "limit", value: int64(3)},
		{name: "non-scalar value", key: "text", value: []string{"data"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := cloneToolStructuralParams(valid)
			params[tt.key] = tt.value
			if err := validateToolNodeAgainstDescriptor(toolStructuralGenericNode(descriptor, params), descriptor); err == nil {
				t.Fatalf("generic descriptor accepted invalid %s parameter %#v", tt.key, tt.value)
			}
		})
	}
}

func TestToolStructuralGlobalParameterLimits(t *testing.T) {
	t.Run("maximum parameter count", func(t *testing.T) {
		descriptor := ToolProfileDescriptor{
			ProfileID:      "test.limit",
			ProfileVersion: "1",
			DisplayName:    "Parameter count boundary",
		}
		params := make(map[string]any)
		for index := 0; index < 16; index++ {
			name := fmt.Sprintf("p%02d", index)
			descriptor.Parameters = append(descriptor.Parameters, ToolParameterSpec{Name: name, Label: name, Type: "boolean", Required: true})
			params[name] = true
		}
		if err := validateToolNodeAgainstDescriptor(toolStructuralGenericNode(descriptor, params), descriptor); err != nil {
			t.Fatalf("Tool with exactly 16 complete parameters rejected: %v", err)
		}
		descriptor.Parameters = append(descriptor.Parameters, ToolParameterSpec{Name: "p16", Label: "p16", Type: "boolean", Required: true})
		params["p16"] = true
		if err := validateToolNodeAgainstDescriptor(toolStructuralGenericNode(descriptor, params), descriptor); err == nil {
			t.Fatal("Tool with 17 parameters passed the frozen global maximum of 16")
		}
	})

	t.Run("maximum canonical scalar-object bytes", func(t *testing.T) {
		descriptor := ToolProfileDescriptor{
			ProfileID:      "test.size",
			ProfileVersion: "1",
			DisplayName:    "Canonical parameter size boundary",
			Parameters: []ToolParameterSpec{{
				Name:     "payload",
				Label:    "Payload",
				Type:     "string",
				Required: true,
				MaxBytes: toolDescriptorInteger(5000),
			}},
		}
		const scalarObjectOverhead = len(`{"payload":""}`)
		atLimit := strings.Repeat("x", 4096-scalarObjectOverhead)
		if err := validateToolNodeAgainstDescriptor(toolStructuralGenericNode(descriptor, map[string]any{"payload": atLimit}), descriptor); err != nil {
			t.Fatalf("4096-byte RFC8785 scalar parameter object rejected: %v", err)
		}
		overLimit := atLimit + "x"
		if err := validateToolNodeAgainstDescriptor(toolStructuralGenericNode(descriptor, map[string]any{"payload": overLimit}), descriptor); err == nil {
			t.Fatal("4097-byte RFC8785 scalar parameter object passed the frozen 4096-byte maximum")
		}
	})
}

func TestToolStructuralEndpointsHonorToolPortDirection(t *testing.T) {
	raw := []byte(toolStructuralConnectedBoardFixture())
	tests := []struct {
		name      string
		endpoint  string
		direction string
		want      bool
	}{
		{name: "input accepts incoming edge", endpoint: "tool_normalize:port_tool_in", direction: FormationPortInput, want: true},
		{name: "input cannot produce", endpoint: "tool_normalize:port_tool_in", direction: FormationPortOutput},
		{name: "output produces outgoing edge", endpoint: "tool_normalize:port_tool_out", direction: FormationPortOutput, want: true},
		{name: "output cannot consume", endpoint: "tool_normalize:port_tool_out", direction: FormationPortInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeID, ok := endpointAllowsDirection(raw, tt.endpoint, tt.direction)
			if ok != tt.want {
				t.Fatalf("endpointAllowsDirection(%q, %q) allowed = %v, want %v", tt.endpoint, tt.direction, ok, tt.want)
			}
			if ok && nodeID != "tool_normalize" {
				t.Fatalf("recognized Tool endpoint node = %q, want tool_normalize", nodeID)
			}
		})
	}
}

func TestToolStructuralSecondProducerToToolInputRejectsCandidateAndWholeBoard(t *testing.T) {
	existing := []BoardConnection{{ID: "edge_first", From: "mis_main:out", To: "tool_normalize:port_tool_in"}}
	candidate := BoardConnection{ID: "edge_second", From: "fmn_source:port_source_out", To: "tool_normalize:port_tool_in"}
	if exists, err := validateConnectionCandidate(existing, candidate); err == nil || exists {
		t.Fatalf("second producer candidate = exists %v, err %v; want structural conflict", exists, err)
	}

	raw := toolStructuralConnectedBoardFixture() + `
[[formation]]
id = "fmn_source"
type = "solo"
title = "Independent source"

[[formation.output]]
id = "port_source_out"
label = "Output"

[[formation.slot]]
id = "slot_source"
label = "Source"
agentId = "scout"
harness = "openai-codex"
controller = true

[[connection]]
id = "edge_second_tool_producer"
from = "fmn_source:port_source_out"
to = "tool_normalize:port_tool_in"
`
	assertToolStructuralBoardRejected(t, raw, "whole board with two producers for one Tool input")
}

func assertToolStructuralBoardAccepted(t *testing.T, raw, name string) {
	t.Helper()
	board, err := parseBoard([]byte(raw))
	if err != nil {
		t.Fatalf("parse valid %s: %v", name, err)
	}
	if report := ValidateBoard(board); len(report.Errors) != 0 {
		t.Fatalf("structural validation rejected %s: %+v", name, report.Errors)
	}
}

func assertToolStructuralBoardRejected(t *testing.T, raw, name string) {
	t.Helper()
	board, err := parseBoard([]byte(raw))
	if err != nil {
		t.Fatalf("inspection parser rejected %s before structural validation: %v", name, err)
	}
	if report := ValidateBoard(board); len(report.Errors) == 0 {
		t.Fatalf("structural validation accepted %s", name)
	}
}

func assertToolStructuralFileIdentity(t *testing.T, path, wantRaw string, wantIdentity [2]uint64) {
	t.Helper()
	if got := readFile(t, path); got != wantRaw {
		t.Fatalf("structural validation changed %s:\n got %q\nwant %q", path, got, wantRaw)
	}
	if got := operativeFileIdentityForTest(t, path); got != wantIdentity {
		t.Fatalf("structural validation replaced operative identity for %s = %v, want %v", path, got, wantIdentity)
	}
}

func replaceToolStructuralFixture(t *testing.T, raw, old, replacement string) string {
	t.Helper()
	if count := strings.Count(raw, old); count != 1 {
		t.Fatalf("fixture replacement target %q count = %d, want 1", old, count)
	}
	return strings.Replace(raw, old, replacement, 1)
}

func cloneToolStructuralParams(params map[string]any) map[string]any {
	clone := make(map[string]any, len(params))
	for name, value := range params {
		clone[name] = value
	}
	return clone
}

func toolStructuralGenericNode(descriptor ToolProfileDescriptor, params map[string]any) ToolNode {
	return ToolNode{
		ID:             "tool_generic",
		Title:          "Generic validation fixture",
		ProfileID:      descriptor.ProfileID,
		ProfileVersion: descriptor.ProfileVersion,
		Params:         params,
	}
}

func toolStructuralScalarDescriptor() ToolProfileDescriptor {
	return ToolProfileDescriptor{
		ProfileID:      "test.scalar",
		ProfileVersion: "1",
		DisplayName:    "Scalar validation fixture",
		Parameters: []ToolParameterSpec{
			{Name: "text", Label: "Text", Type: "string", Required: true, MinBytes: toolDescriptorInteger(2), MaxBytes: toolDescriptorInteger(4)},
			{Name: "mode", Label: "Mode", Type: "string", Required: true, Enum: []string{"strict"}},
			{Name: "enabled", Label: "Enabled", Type: "boolean", Required: true},
			{Name: "limit", Label: "Limit", Type: "integer", Required: true, Minimum: toolDescriptorInteger(-2), Maximum: toolDescriptorInteger(2)},
		},
	}
}

func toolStructuralConnectedBoardFixture() string {
	return `schema = 2
id = "brd_tool_structural"
slug = "tool-structural"
title = "Tool structural validation"
rev = 4
updatedBy = "agent:test"
updatedAt = "2026-07-19T10:00:00Z"

[[mission]]
id = "mis_main"
title = "Main"
goal = "Normalize the report"
beadId = "home-test"

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

[[formation]]
id = "fmn_sink"
type = "solo"
title = "Use result"

[[formation.input]]
id = "port_sink_in"
label = "Input"

[[formation.output]]
id = "port_sink_out"
label = "Output"

[[formation.slot]]
id = "slot_sink"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[connection]]
id = "edge_mission_tool"
from = "mis_main:out"
to = "tool_normalize:port_tool_in"

[[connection]]
id = "edge_tool_sink"
from = "tool_normalize:port_tool_out"
to = "fmn_sink:port_sink_in"
`
}

func toolStructuralIsolatedNodesFixture() string {
	return `schema = 2
id = "brd_tool_structural"
slug = "tool-structural"
title = "Tool structural validation"
rev = 4

[[mission]]
id = "mis_main"
title = "Main"
goal = "Inspect collisions"
beadId = "home-test"

[[formation]]
id = "fmn_worker"
type = "solo"
title = "Worker"

[[formation.input]]
id = "port_worker_in"
label = "Input"

[[formation.output]]
id = "port_worker_out"
label = "Output"

[[formation.slot]]
id = "slot_worker"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[gate]]
id = "gate_review"
title = "Review"
kinds = ["human"]
criterion = "Confirm"

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
`
}

func toolStructuralLayoutFixture() string {
	return `schema = 1
boardId = "brd_tool_structural"
boardRev = 4

[[node]]
id = "tool_normalize"
x = 300
y = 160
`
}
