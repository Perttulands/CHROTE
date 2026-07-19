package formations

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type toolStructuralPortMutationCase struct {
	name   string
	mutate func(*testing.T, string) string
}

func TestToolStructuralSchemaOneInspectionPreservesSourceAndLayoutBeforeRejection(t *testing.T) {
	store := NewStore(t.TempDir())
	raw := replaceToolStructuralFixture(t, toolStructuralDraftBoardFixture(), "schema = 2", "schema = 1")
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
	assertToolStructuralDefinitionAccepted(t, toolStructuralDraftBoardFixture(), "valid schema-2 json.normalize@1 draft")
	assertToolStructuralBoardAccepted(t, toolStructuralDraftBoardFixture(), "valid schema-2 json.normalize@1 draft board")
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
			raw := mutateToolStructuralPrimaryToolBlock(t, toolStructuralDraftBoardFixture(), tt.old, tt.new)
			assertToolStructuralBoardRejected(t, raw, tt.name)
		})
	}
}

func TestToolStructuralJSONNormalizePortsBindFrozenShapeButNotLabels(t *testing.T) {
	labelsChanged := mutateToolStructuralPortBlock(
		t,
		toolStructuralDraftBoardFixture(),
		toolStructuralPrimaryInputBlock,
		`label = "Report"`,
		`label = "Operator-selected source"`,
	)
	labelsChanged = mutateToolStructuralPortBlock(
		t,
		labelsChanged,
		toolStructuralPrimaryOutputBlock,
		`label = "Normalized report"`,
		`label = "Operator-selected result"`,
	)
	assertToolStructuralDefinitionAccepted(t, labelsChanged, "display-only Tool labels")

	tests := []toolStructuralPortMutationCase{
		{
			name: "missing input port",
			mutate: func(t *testing.T, raw string) string {
				return replaceToolStructuralFixture(t, raw, toolStructuralPrimaryInputBlock, "")
			},
		},
		{
			name: "extra input port",
			mutate: func(t *testing.T, raw string) string {
				return replaceToolStructuralFixture(t, raw, toolStructuralPrimaryInputBlock, toolStructuralPrimaryInputBlock+`[[tool.input]]
id = "port_tool_extra"
name = "extra"
label = "Extra"
direction = "input"
kind = "work"
acceptedMediaTypes = ["application/json"]
required = false
role = "data"

				`)
			},
		},
		{
			name: "missing output port",
			mutate: func(t *testing.T, raw string) string {
				return replaceToolStructuralFixture(t, raw, toolStructuralPrimaryOutputBlock, "")
			},
		},
		{
			name: "extra output port",
			mutate: func(t *testing.T, raw string) string {
				return replaceToolStructuralFixture(t, raw, toolStructuralPrimaryOutputBlock, toolStructuralPrimaryOutputBlock+`[[tool.output]]
id = "port_tool_extra_out"
name = "extra_output"
label = "Extra output"
direction = "output"
kind = "work"
acceptedMediaTypes = ["application/json"]

`)
			},
		},
		toolStructuralPortMutation("input semantic name", toolStructuralPrimaryInputBlock, `name = "input"`, `name = "source"`),
		toolStructuralPortMutation("input direction", toolStructuralPrimaryInputBlock, `direction = "input"`, `direction = "output"`),
		toolStructuralPortMutation("input kind", toolStructuralPrimaryInputBlock, `kind = "work"`, `kind = "gate_feedback"`),
		toolStructuralPortMutation("input media value", toolStructuralPrimaryInputBlock, `acceptedMediaTypes = ["application/json"]`, `acceptedMediaTypes = ["text/plain"]`),
		toolStructuralPortMutation("input media cardinality", toolStructuralPrimaryInputBlock, `acceptedMediaTypes = ["application/json"]`, `acceptedMediaTypes = ["application/json", "text/plain"]`),
		toolStructuralPortMutation("input required presence", toolStructuralPrimaryInputBlock, "required = true\n", ""),
		toolStructuralPortMutation("input required value", toolStructuralPrimaryInputBlock, `required = true`, `required = false`),
		toolStructuralPortMutation("input role presence", toolStructuralPrimaryInputBlock, "role = \"data\"\n", ""),
		toolStructuralPortMutation("input role value", toolStructuralPrimaryInputBlock, `role = "data"`, `role = "retry_control"`),
		toolStructuralPortMutation("output semantic name", toolStructuralPrimaryOutputBlock, `name = "output"`, `name = "result"`),
		toolStructuralPortMutation("output direction", toolStructuralPrimaryOutputBlock, `direction = "output"`, `direction = "input"`),
		toolStructuralPortMutation("output kind", toolStructuralPrimaryOutputBlock, `kind = "work"`, `kind = "gate_feedback"`),
		toolStructuralPortMutation("output media value", toolStructuralPrimaryOutputBlock, `acceptedMediaTypes = ["application/json"]`, `acceptedMediaTypes = ["text/plain"]`),
		toolStructuralPortMutation("output media cardinality", toolStructuralPrimaryOutputBlock, `acceptedMediaTypes = ["application/json"]`, `acceptedMediaTypes = ["application/json", "text/plain"]`),
		toolStructuralPortMutation("output required presence", toolStructuralPrimaryOutputBlock, "acceptedMediaTypes = [\"application/json\"]\n", "acceptedMediaTypes = [\"application/json\"]\nrequired = false\n"),
		toolStructuralPortMutation("output role presence", toolStructuralPrimaryOutputBlock, "acceptedMediaTypes = [\"application/json\"]\n", "acceptedMediaTypes = [\"application/json\"]\nrole = \"data\"\n"),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := tt.mutate(t, toolStructuralDraftBoardFixture())
			assertToolStructuralDefinitionRejected(t, raw, tt.name)
		})
	}
}

func TestToolStructuralIDsAreUniqueOnlyAtTheirAuthorityScope(t *testing.T) {
	definitionTests := []struct {
		name string
		raw  string
	}{
		{
			name: "empty Tool id",
			raw: mutateToolStructuralPrimaryToolBlock(
				t,
				toolStructuralDraftBoardFixture(),
				`id = "tool_normalize"`,
				`id = ""`,
			),
		},
		{
			name: "empty input port id",
			raw: mutateToolStructuralPortBlock(
				t,
				toolStructuralDraftBoardFixture(),
				toolStructuralPrimaryInputBlock,
				`id = "port_tool_in"`,
				`id = ""`,
			),
		},
		{
			name: "empty output port id",
			raw: mutateToolStructuralPortBlock(
				t,
				toolStructuralDraftBoardFixture(),
				toolStructuralPrimaryOutputBlock,
				`id = "port_tool_out"`,
				`id = ""`,
			),
		},
		{
			name: "duplicate port id within Tool",
			raw: mutateToolStructuralPortBlock(
				t,
				toolStructuralDraftBoardFixture(),
				toolStructuralPrimaryOutputBlock,
				`id = "port_tool_out"`,
				`id = "port_tool_in"`,
			),
		},
	}
	for _, tt := range definitionTests {
		t.Run(tt.name, func(t *testing.T) {
			assertToolStructuralDefinitionRejected(t, tt.raw, tt.name)
		})
	}

	boardTests := []struct {
		name string
		raw  string
	}{
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
	for _, tt := range boardTests {
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
		name  string
		block string
	}{
		{name: "missing required mode", block: "[tool.params]\n\n"},
		{name: "unknown parameter", block: "[tool.params]\nmode = \"strict\"\nextra = true\n\n"},
		{name: "wrong boolean scalar", block: "[tool.params]\nmode = true\n\n"},
		{name: "wrong integer scalar", block: "[tool.params]\nmode = 1\n\n"},
		{name: "out of domain", block: "[tool.params]\nmode = \"lenient\"\n\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := replaceToolStructuralFixture(t, toolStructuralDraftBoardFixture(), toolStructuralPrimaryParamsBlock, tt.block)
			assertToolStructuralDefinitionRejected(t, raw, tt.name)
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
	validTests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "optional parameter omitted", mutate: func(map[string]any) {}},
		{name: "boolean false", mutate: func(params map[string]any) { params["enabled"] = false }},
		{name: "exact string minimum", mutate: func(params map[string]any) { params["text"] = "xy" }},
		{name: "exact string maximum", mutate: func(params map[string]any) { params["text"] = "data" }},
		{name: "exact integer minimum", mutate: func(params map[string]any) { params["limit"] = int64(-2) }},
		{name: "exact integer maximum", mutate: func(params map[string]any) { params["limit"] = int64(2) }},
	}
	for _, tt := range validTests {
		t.Run(tt.name, func(t *testing.T) {
			params := cloneToolStructuralParams(valid)
			tt.mutate(params)
			if err := validateToolNodeAgainstDescriptor(toolStructuralGenericNode(descriptor, params), descriptor); err != nil {
				t.Fatalf("valid generic scalar parameters rejected: %v", err)
			}
		})
	}

	tests := []struct {
		name  string
		key   string
		value any
	}{
		{name: "string type", key: "text", value: true},
		{name: "string minimum bytes", key: "text", value: "x"},
		{name: "string maximum bytes", key: "text", value: "abcde"},
		{name: "string NUL within byte limits", key: "text", value: "a\x00"},
		{name: "string invalid UTF-8 within byte limits", key: "text", value: string([]byte{'a', 0xff})},
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
		// RFC 8785 leaves '<' unescaped. encoding/json's default HTML escaping
		// expands it to "\\u003c", so this boundary catches the wrong encoder.
		atLimit := strings.Repeat("<", 4096-scalarObjectOverhead)
		if err := validateToolNodeAgainstDescriptor(toolStructuralGenericNode(descriptor, map[string]any{"payload": atLimit}), descriptor); err != nil {
			t.Fatalf("4096-byte RFC8785 scalar parameter object rejected: %v", err)
		}
		overLimit := atLimit + "<"
		if err := validateToolNodeAgainstDescriptor(toolStructuralGenericNode(descriptor, map[string]any{"payload": overLimit}), descriptor); err == nil {
			t.Fatal("4097-byte RFC8785 scalar parameter object passed the frozen 4096-byte maximum")
		}
	})
}

func TestToolStructuralGenericPortBindingPreservesSameDirectionOrder(t *testing.T) {
	descriptor := toolStructuralOrderedPortDescriptor()
	if err := validateToolProfileDescriptor(descriptor); err != nil {
		t.Fatalf("ordered-port descriptor is not contract-valid: %v", err)
	}
	node := toolStructuralGenericNode(descriptor, map[string]any{})
	node.Inputs = []ToolPort{
		toolStructuralPortFromDescriptor("port_primary", descriptor.Ports[0]),
		toolStructuralPortFromDescriptor("port_context", descriptor.Ports[1]),
	}
	node.Outputs = []ToolPort{
		toolStructuralPortFromDescriptor("port_result", descriptor.Ports[2]),
	}
	if err := validateToolNodeAgainstDescriptor(node, descriptor); err != nil {
		t.Fatalf("descriptor-ordered generic Tool ports rejected: %v", err)
	}

	reordered := node
	reordered.Inputs = append([]ToolPort(nil), node.Inputs...)
	reordered.Inputs[0], reordered.Inputs[1] = reordered.Inputs[1], reordered.Inputs[0]
	if err := validateToolNodeAgainstDescriptor(reordered, descriptor); err == nil {
		t.Fatal("Tool inputs reordered against the descriptor were accepted")
	}
}

func TestToolStructuralEndpointsHonorToolPortDirection(t *testing.T) {
	raw := []byte(toolStructuralDraftBoardFixture())
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

func TestToolStructuralMissionOutputMediaMustBeAcceptedByToolInput(t *testing.T) {
	raw := toolStructuralDraftBoardFixture() + `
[[connection]]
id = "edge_mission_tool_media_mismatch"
from = "mis_main:out"
to = "tool_normalize:port_tool_in"
`
	board := mustParseValidateBoardFixture(t, raw)
	report := ValidateBoard(board)
	if dangling := findBoardFindings(report.Errors, FindingDanglingConnection); len(dangling) != 0 {
		t.Fatalf("known Mission and Tool endpoints produced dangling findings: %+v", dangling)
	}
	mediaFindings := findBoardFindings(report.Errors, FindingIncompatibleMedia)
	if len(mediaFindings) != 1 || len(report.Errors) != 1 {
		t.Fatalf("Mission text/markdown to application/json-only Tool findings = %+v, want one stable media-incompatibility error", report.Errors)
	}
}

func TestToolStructuralRoutingRequiresCompleteProducerMediaSubset(t *testing.T) {
	baseRaw := toolStructuralDraftBoardFixture() + `
[[gate]]
id = "gate_media"
title = "Inspect media"
kinds = ["human"]
criterion = "Confirm the payload"
`

	t.Run("overlap without subset rejects", func(t *testing.T) {
		raw := baseRaw + `
[[connection]]
id = "edge_gate_tool_media_overlap"
from = "gate_media:pass"
to = "tool_normalize:port_tool_in"
`
		report := ValidateBoard(mustParseValidateBoardFixture(t, raw))
		if dangling := findBoardFindings(report.Errors, FindingDanglingConnection); len(dangling) != 0 {
			t.Fatalf("known Gate and Tool endpoints produced dangling findings: %+v", dangling)
		}
		mediaFindings := findBoardFindings(report.Errors, FindingIncompatibleMedia)
		if len(mediaFindings) != 1 || len(report.Errors) != 1 {
			t.Fatalf("Gate full-set output to JSON-only Tool findings = %+v, want one stable media-incompatibility error", report.Errors)
		}
	})

	t.Run("proper subset accepts", func(t *testing.T) {
		raw := baseRaw + `
[[connection]]
id = "edge_tool_gate_media_subset"
from = "tool_normalize:port_tool_out"
to = "gate_media:in"
`
		assertToolStructuralBoardAccepted(t, raw, "Tool JSON output routed to full-set Gate input")
	})
}

func TestToolStructuralToolInputRejectsGateFeedbackKind(t *testing.T) {
	raw := toolStructuralDraftBoardFixture() + `
[[gate]]
id = "gate_feedback"
title = "Inspect feedback"
kinds = ["human"]
criterion = "Confirm the payload"

[[connection]]
id = "edge_gate_feedback_tool_work"
from = "gate_feedback:fail"
to = "tool_normalize:port_tool_in"
`
	report := ValidateBoard(mustParseValidateBoardFixture(t, raw))
	if dangling := findBoardFindings(report.Errors, FindingDanglingConnection); len(dangling) != 0 {
		t.Fatalf("known Gate fail and Tool input endpoints produced dangling findings: %+v", dangling)
	}
	kindFindings := findBoardFindings(report.Errors, FindingIncompatiblePayloadKind)
	if len(kindFindings) != 1 || len(report.Errors) != 1 {
		t.Fatalf("Gate feedback to Tool work input findings = %+v, want one stable payload-kind incompatibility error", report.Errors)
	}
}

func TestToolStructuralJudgeRelationshipRejectsToolCrossUse(t *testing.T) {
	tests := []struct {
		name       string
		companion  string
		connection string
	}{
		{
			name: "Tool output into Gate judge",
			companion: `[[connection]]
id = "edge_gate_judge_formation"
from = "gate_judge:judge"
to = "fmn_judge:port_judge_in"
`,
			connection: `[[connection]]
id = "edge_tool_gate_judge"
from = "tool_normalize:port_tool_out"
to = "gate_judge:judge"
`,
		},
		{
			name: "Gate judge into Tool input",
			companion: `[[connection]]
id = "edge_gate_judge_formation"
from = "gate_judge:judge"
to = "fmn_judge:port_judge_in"

[[connection]]
id = "edge_formation_gate_judge"
from = "fmn_judge:port_judge_out"
to = "gate_judge:judge"
`,
			connection: `[[connection]]
id = "edge_gate_judge_tool"
from = "gate_judge:judge"
to = "tool_normalize:port_tool_in"
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := ValidateBoard(mustParseValidateBoardFixture(t, toolStructuralWriterBoardFixture()+"\n"+tt.companion+"\n"+tt.connection))
			if dangling := findBoardFindings(report.Errors, FindingDanglingConnection); len(dangling) != 0 {
				t.Fatalf("known Tool and Gate judge endpoints produced dangling findings: %+v", dangling)
			}
			judgeFindings := findBoardFindings(report.Errors, FindingInvalidJudgeRelationship)
			if len(judgeFindings) != 1 {
				t.Fatalf("Tool/Gate judge cross-use findings = %+v, want one invalid-judge-relationship error", report.Errors)
			}
			if wrongKind := findBoardFindings(report.Errors, FindingIncompatiblePayloadKind); len(wrongKind) != 0 {
				t.Fatalf("judge control was misreported as a payload-kind mismatch: %+v", wrongKind)
			}
			if wrongMedia := findBoardFindings(report.Errors, FindingIncompatibleMedia); len(wrongMedia) != 0 {
				t.Fatalf("judge control was misreported as a media mismatch: %+v", wrongMedia)
			}
		})
	}
}

func TestToolStructuralWritersRejectIncompatibleConnectionsWithoutPublication(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		from       string
		previousTo string
		to         string
	}{
		{
			name: "wire media mismatch",
			raw:  toolStructuralWriterBoardFixture(),
			from: "mis_main:out",
			to:   "tool_normalize:port_tool_in",
		},
		{
			name: "wire payload kind mismatch",
			raw:  toolStructuralWriterBoardFixture(),
			from: "gate_primary:fail",
			to:   "tool_normalize:port_tool_in",
		},
		{
			name: "wire Tool output into Gate judge",
			raw:  toolStructuralWriterBoardFixture(),
			from: "tool_normalize:port_tool_out",
			to:   "gate_judge:judge",
		},
		{
			name: "wire Gate judge into Tool input",
			raw:  toolStructuralWriterBoardFixture(),
			from: "gate_judge:judge",
			to:   "tool_normalize:port_tool_in",
		},
		{
			name: "rewire media mismatch",
			raw: toolStructuralWriterBoardFixture() + `
[[connection]]
id = "edge_mission_gate"
from = "mis_main:out"
to = "gate_primary:in"
`,
			from:       "mis_main:out",
			previousTo: "gate_primary:in",
			to:         "tool_normalize:port_tool_in",
		},
		{
			name: "rewire Tool output into Gate judge",
			raw: toolStructuralWriterBoardFixture() + `
[[connection]]
id = "edge_tool_gate"
from = "tool_normalize:port_tool_out"
to = "gate_primary:in"
`,
			from:       "tool_normalize:port_tool_out",
			previousTo: "gate_primary:in",
			to:         "gate_judge:judge",
		},
		{
			name: "rewire Gate judge into Tool input",
			raw: toolStructuralWriterBoardFixture() + `
[[connection]]
id = "edge_gate_judge_formation"
from = "gate_judge:judge"
to = "fmn_judge:port_judge_in"
`,
			from:       "gate_judge:judge",
			previousTo: "fmn_judge:port_judge_in",
			to:         "tool_normalize:port_tool_in",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertToolStructuralWriterRejectedWithoutPublication(t, tt.raw, tt.from, tt.previousTo, tt.to)
		})
	}
}

func TestToolStructuralWritersKeepCompatibleToolConnections(t *testing.T) {
	t.Run("wire proper media subset", func(t *testing.T) {
		store, before := toolStructuralWriterStore(t, toolStructuralWriterBoardFixture())
		after, err := store.WireFormationPorts("tool-structural", FormationWireRequest{
			From: "tool_normalize:port_tool_out", To: "gate_primary:in", UpdatedBy: "agent:test",
		}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
		if err != nil {
			t.Fatalf("wire compatible Tool output: %v", err)
		}
		if !hasConnection(after.Connections, "tool_normalize:port_tool_out", "gate_primary:in") {
			t.Fatalf("compatible Tool connection was not published: %+v", after.Connections)
		}
	})

	t.Run("rewire proper media subset", func(t *testing.T) {
		raw := toolStructuralWriterBoardFixture() + `
[[connection]]
id = "edge_tool_gate"
from = "tool_normalize:port_tool_out"
to = "gate_primary:in"
`
		store, before := toolStructuralWriterStore(t, raw)
		after, err := store.RewireFormationTarget("tool-structural", FormationRewireRequest{
			From: "tool_normalize:port_tool_out", PreviousTo: "gate_primary:in", To: "gate_secondary:in", UpdatedBy: "agent:test",
		}, WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev})
		if err != nil {
			t.Fatalf("rewire compatible Tool output: %v", err)
		}
		if hasConnection(after.Connections, "tool_normalize:port_tool_out", "gate_primary:in") ||
			!hasConnection(after.Connections, "tool_normalize:port_tool_out", "gate_secondary:in") {
			t.Fatalf("compatible Tool rewire was not published exactly once: %+v", after.Connections)
		}
	})
}

func TestToolStructuralValidateBoardChecksEveryToolDefinition(t *testing.T) {
	baselineRaw := toolStructuralDuplicateProducerBoardFixture(false)
	targetBlock := toolStructuralJSONNormalizeToolBlock("tool_target", "Target", "port_target_in", "port_target_out")
	invalidTargetBlock := replaceToolStructuralFixture(t, targetBlock, `mode = "strict"`, `mode = "lenient"`)
	invalidRaw := replaceToolStructuralFixture(t, baselineRaw, targetBlock, invalidTargetBlock)

	baselineReport := ValidateBoard(mustParseValidateBoardFixture(t, baselineRaw))
	if len(baselineReport.Errors) != 0 {
		t.Fatalf("otherwise-valid three-Tool baseline has structural errors: %+v", baselineReport.Errors)
	}
	report := ValidateBoard(mustParseValidateBoardFixture(t, invalidRaw))
	if dangling := findBoardFindings(report.Errors, FindingDanglingConnection); len(dangling) != 0 {
		t.Fatalf("descriptor-invalid third Tool introduced unrelated dangling errors: %+v", dangling)
	}
	if len(report.Errors) <= len(baselineReport.Errors) {
		t.Fatalf("descriptor-invalid third Tool added no whole-board error: baseline=%+v invalid=%+v", baselineReport.Errors, report.Errors)
	}
}

func TestToolStructuralSecondProducerToToolInputRejectsCandidateAndWholeBoard(t *testing.T) {
	existing := []BoardConnection{{ID: "edge_first", From: "tool_source_a:port_source_a_out", To: "tool_target:port_target_in"}}
	candidate := BoardConnection{ID: "edge_second", From: "tool_source_b:port_source_b_out", To: "tool_target:port_target_in"}
	if exists, err := validateConnectionCandidate(existing, candidate); err == nil || exists {
		t.Fatalf("second producer candidate = exists %v, err %v; want structural conflict", exists, err)
	}

	baselineRaw := toolStructuralDuplicateProducerBoardFixture(false)
	raw := toolStructuralDuplicateProducerBoardFixture(true)
	for _, candidateRaw := range []string{baselineRaw, raw} {
		report := ValidateBoard(mustParseValidateBoardFixture(t, candidateRaw))
		if dangling := findBoardFindings(report.Errors, FindingDanglingConnection); len(dangling) != 0 {
			t.Fatalf("Tool-output candidate endpoints were not structurally recognized: %+v", dangling)
		}
	}
	baselineReport := ValidateBoard(mustParseValidateBoardFixture(t, baselineRaw))
	report := ValidateBoard(mustParseValidateBoardFixture(t, raw))
	if len(report.Errors) <= len(baselineReport.Errors) {
		t.Fatalf("second Tool producer added no whole-board error: baseline=%+v duplicate=%+v", baselineReport.Errors, report.Errors)
	}
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

func assertToolStructuralDefinitionAccepted(t *testing.T, raw, name string) {
	t.Helper()
	tool, descriptor := mustToolStructuralDefinition(t, raw, name)
	if err := validateToolNodeAgainstDescriptor(tool, descriptor); err != nil {
		t.Fatalf("structural validation rejected %s: %v", name, err)
	}
}

func assertToolStructuralDefinitionRejected(t *testing.T, raw, name string) {
	t.Helper()
	tool, descriptor := mustToolStructuralDefinition(t, raw, name)
	if err := validateToolNodeAgainstDescriptor(tool, descriptor); err == nil {
		t.Fatalf("structural validation accepted %s", name)
	}
}

func mustToolStructuralDefinition(t *testing.T, raw, name string) (ToolNode, ToolProfileDescriptor) {
	t.Helper()
	board, err := parseBoard([]byte(raw))
	if err != nil {
		t.Fatalf("inspection parser rejected %s before structural validation: %v", name, err)
	}
	if len(board.Tools) != 1 {
		t.Fatalf("%s parsed Tool count = %d, want 1", name, len(board.Tools))
	}
	tool := board.Tools[0]
	descriptor, ok := LookupToolProfileDescriptor(tool.ProfileID, tool.ProfileVersion)
	if !ok {
		t.Fatalf("%s uses missing descriptor %q@%q", name, tool.ProfileID, tool.ProfileVersion)
	}
	return tool, descriptor
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

func assertToolStructuralWriterRejectedWithoutPublication(
	t *testing.T,
	raw string,
	from string,
	previousTo string,
	to string,
) {
	t.Helper()
	store, before := toolStructuralWriterStore(t, raw)
	path := store.BoardPath("tool-structural")
	wantRaw := readFile(t, path)
	wantIdentity := operativeFileIdentityForTest(t, path)
	opts := WriteOptions{ExpectedETag: before.ETag, ExpectedRev: before.Rev}
	var err error
	if previousTo == "" {
		_, err = store.WireFormationPorts("tool-structural", FormationWireRequest{
			From: from, To: to, UpdatedBy: "agent:test",
		}, opts)
	} else {
		_, err = store.RewireFormationTarget("tool-structural", FormationRewireRequest{
			From: from, PreviousTo: previousTo, To: to, UpdatedBy: "agent:test",
		}, opts)
	}
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("incompatible Tool connection error = %v, want ErrConflict", err)
	}
	assertToolStructuralFileIdentity(t, path, wantRaw, wantIdentity)
}

func toolStructuralWriterStore(t *testing.T, raw string) (*Store, *BoardDocument) {
	t.Helper()
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("tool-structural"), raw)
	before, err := store.ReadBoard("tool-structural")
	if err != nil {
		t.Fatalf("read Tool writer fixture: %v", err)
	}
	return store, before
}

func replaceToolStructuralFixture(t *testing.T, raw, old, replacement string) string {
	t.Helper()
	if count := strings.Count(raw, old); count != 1 {
		t.Fatalf("fixture replacement target %q count = %d, want 1", old, count)
	}
	return strings.Replace(raw, old, replacement, 1)
}

func mutateToolStructuralPrimaryToolBlock(t *testing.T, raw, old, replacement string) string {
	t.Helper()
	block := toolStructuralPrimaryToolBlock()
	mutated := replaceToolStructuralFixture(t, block, old, replacement)
	return replaceToolStructuralFixture(t, raw, block, mutated)
}

func mutateToolStructuralPortBlock(t *testing.T, raw, block, old, replacement string) string {
	t.Helper()
	mutated := replaceToolStructuralFixture(t, block, old, replacement)
	return replaceToolStructuralFixture(t, raw, block, mutated)
}

func toolStructuralPortMutation(name, block, old, replacement string) toolStructuralPortMutationCase {
	return toolStructuralPortMutationCase{
		name: name,
		mutate: func(t *testing.T, raw string) string {
			return mutateToolStructuralPortBlock(t, raw, block, old, replacement)
		},
	}
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

func toolStructuralPortFromDescriptor(id string, descriptor ToolPortDescriptor) ToolPort {
	return ToolPort{
		ID:                 id,
		Name:               descriptor.Name,
		Label:              descriptor.Label,
		Direction:          descriptor.Direction,
		Kind:               descriptor.Kind,
		AcceptedMediaTypes: append([]string(nil), descriptor.AcceptedMediaTypes...),
		Required:           cloneToolBool(descriptor.Required),
		Role:               cloneToolString(descriptor.Role),
	}
}

func toolStructuralOrderedPortDescriptor() ToolProfileDescriptor {
	return ToolProfileDescriptor{
		ProfileID:      "test.ordered",
		ProfileVersion: "1",
		DisplayName:    "Ordered port validation fixture",
		Ports: []ToolPortDescriptor{
			{
				Name:               "primary",
				Label:              "Primary",
				Direction:          "input",
				Kind:               "work",
				AcceptedMediaTypes: []string{"application/json"},
				Required:           toolDescriptorBool(true),
				Role:               toolDescriptorString("data"),
			},
			{
				Name:               "context",
				Label:              "Context",
				Direction:          "input",
				Kind:               "work",
				AcceptedMediaTypes: []string{"application/json"},
				Required:           toolDescriptorBool(false),
				Role:               toolDescriptorString("data"),
			},
			{
				Name:               "result",
				Label:              "Result",
				Direction:          "output",
				Kind:               "work",
				AcceptedMediaTypes: []string{"application/json"},
			},
		},
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
			{Name: "note", Label: "Note", Type: "string", Required: false, MaxBytes: toolDescriptorInteger(4)},
		},
	}
}

const toolStructuralPrimaryParamsBlock = `[tool.params]
mode = "strict"

`

const toolStructuralPrimaryInputBlock = `[[tool.input]]
id = "port_tool_in"
name = "input"
label = "Report"
direction = "input"
kind = "work"
acceptedMediaTypes = ["application/json"]
required = true
role = "data"

`

const toolStructuralPrimaryOutputBlock = `[[tool.output]]
id = "port_tool_out"
name = "output"
label = "Normalized report"
direction = "output"
kind = "work"
acceptedMediaTypes = ["application/json"]
`

func toolStructuralPrimaryToolBlock() string {
	return `[[tool]]
id = "tool_normalize"
title = "Normalize report"
profileId = "json.normalize"
profileVersion = "1"

` + toolStructuralPrimaryParamsBlock + toolStructuralPrimaryInputBlock + toolStructuralPrimaryOutputBlock
}

func toolStructuralDraftBoardFixture() string {
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

` + toolStructuralPrimaryToolBlock()
}

func toolStructuralWriterBoardFixture() string {
	return toolStructuralDraftBoardFixture() + `
[[formation]]
id = "fmn_judge"
type = "solo"
title = "Judge"

[[formation.input]]
id = "port_judge_in"
label = "Input"

[[formation.output]]
id = "port_judge_out"
label = "Output"

[[gate]]
id = "gate_primary"
title = "Primary gate"
kinds = ["human"]
criterion = "Confirm the payload"

[[gate]]
id = "gate_secondary"
title = "Secondary gate"
kinds = ["human"]
criterion = "Confirm the payload"

[[gate]]
id = "gate_judge"
title = "Formation judge"
kinds = ["formation"]
criterion = "Judge the payload"
`
}

func toolStructuralDuplicateProducerBoardFixture(includeSecondProducer bool) string {
	raw := `schema = 2
id = "brd_tool_duplicate_producer"
slug = "tool-duplicate-producer"
title = "Tool duplicate producer validation"
rev = 1

[[mission]]
id = "mis_main"
title = "Main"
goal = "Inspect duplicate Tool producers"
beadId = "home-test"

`
	raw += toolStructuralJSONNormalizeToolBlock("tool_source_a", "Source A", "port_source_a_in", "port_source_a_out")
	raw += toolStructuralJSONNormalizeToolBlock("tool_source_b", "Source B", "port_source_b_in", "port_source_b_out")
	raw += toolStructuralJSONNormalizeToolBlock("tool_target", "Target", "port_target_in", "port_target_out")
	raw += `
[[connection]]
id = "edge_first"
from = "tool_source_a:port_source_a_out"
to = "tool_target:port_target_in"
`
	if includeSecondProducer {
		raw += `
[[connection]]
id = "edge_second"
from = "tool_source_b:port_source_b_out"
to = "tool_target:port_target_in"
`
	}
	return raw
}

func toolStructuralJSONNormalizeToolBlock(id, title, inputID, outputID string) string {
	return fmt.Sprintf(`[[tool]]
id = %q
title = %q
profileId = "json.normalize"
profileVersion = "1"

[tool.params]
mode = "strict"

[[tool.input]]
id = %q
name = "input"
label = "Report"
direction = "input"
kind = "work"
acceptedMediaTypes = ["application/json"]
required = true
role = "data"

[[tool.output]]
id = %q
name = "output"
label = "Normalized report"
direction = "output"
kind = "work"
acceptedMediaTypes = ["application/json"]

`, id, title, inputID, outputID)
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
