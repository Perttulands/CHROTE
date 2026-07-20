package formations

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolExecutionPreflightResolvesTheSelectedRootBeforeMigration(t *testing.T) {
	board := toolExecutionPreflightHeader() +
		toolExecutionPreflightFormation("fmn_legacy", true) +
		toolExecutionPreflightTool("tool_unrelated", "1")
	tests := []struct {
		name  string
		start func(*Store, *RunEngine) error
	}{
		{
			name: "direct Store missing Mission",
			start: func(store *Store, _ *RunEngine) error {
				_, err := store.StartRun("tool-preflight", RunStartRequest{MissionID: "mis_missing"})
				return err
			},
		},
		{
			name: "engine missing Mission",
			start: func(_ *Store, engine *RunEngine) error {
				_, err := engine.RunMission("tool-preflight", RunStartRequest{MissionID: "mis_missing"})
				return err
			},
		},
		{
			name: "engine missing isolated Formation",
			start: func(_ *Store, engine *RunEngine) error {
				_, err := engine.RunFormation("tool-preflight", "fmn_missing", FormationRunRequest{})
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, engine, executor, evaluator := newToolExecutionPreflightHarness(t, board)
			err := test.start(store, engine)
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("start error = %v, want exact-root ErrNotFound", err)
			}
			if errors.Is(err, ErrLegacyInlineVerificationRequiresMigration) ||
				errors.Is(err, ErrToolExecutionUnavailable) ||
				errors.Is(err, ErrRuntimeAuthorityNonAuthorizing) {
				t.Fatalf("later preflight boundary masked exact root: %v", err)
			}
			assertToolExecutionPreflightNoEffects(t, store.Workspace, executor, evaluator)
		})
	}
}

func TestToolExecutionPreflightUsesApprovedSelectedMissionOrder(t *testing.T) {
	validTool := toolExecutionPreflightTool("tool_valid", "1")
	tests := []struct {
		name        string
		board       string
		directStore bool
		want        error
		wantText    string
	}{
		{
			name: "reachable inline migration precedes Tool",
			board: toolExecutionPreflightHeader() +
				toolExecutionPreflightFormation("fmn_legacy", true) +
				validTool +
				toolExecutionPreflightConnection("edge_start", "workflow", "mis_main:out", "fmn_legacy:port_fmn_legacy_in") +
				toolExecutionPreflightConnection("edge_tool", "workflow", "fmn_legacy:port_fmn_legacy_out", "tool_valid:port_tool_valid_in"),
			want: ErrLegacyInlineVerificationRequiresMigration,
		},
		{
			name: "reachable script Gate migration precedes Tool",
			board: toolExecutionPreflightHeader() +
				toolExecutionPreflightGate("gate_legacy", true, "code") +
				validTool +
				toolExecutionPreflightConnection("edge_start", "workflow", "mis_main:out", "gate_legacy:in") +
				toolExecutionPreflightConnection("edge_tool", "workflow", "gate_legacy:pass", "tool_valid:port_tool_valid_in"),
			want: ErrLegacyScriptGateRequiresFencedMigration,
		},
		{
			name: "all reachable descriptors validate before unavailable",
			board: toolExecutionPreflightHeader() +
				validTool +
				toolExecutionPreflightTool("tool_invalid", "999") +
				toolExecutionPreflightConnection("edge_valid_first", "workflow", "mis_main:out", "tool_valid:port_tool_valid_in") +
				toolExecutionPreflightConnection("edge_invalid_second", "workflow", "mis_main:out", "tool_invalid:port_tool_invalid_in"),
			wantText: `unknown profile tuple "json.normalize"@"999"`,
		},
		{
			name: "known descriptor parameters validate before unavailable",
			board: toolExecutionPreflightHeader() +
				validTool +
				strings.Replace(toolExecutionPreflightTool("tool_invalid", "1"), `mode = "strict"`, `mode = "lenient"`, 1) +
				toolExecutionPreflightConnection("edge_valid_first", "workflow", "mis_main:out", "tool_valid:port_tool_valid_in") +
				toolExecutionPreflightConnection("edge_invalid_second", "workflow", "mis_main:out", "tool_invalid:port_tool_invalid_in"),
			wantText: `Tool "tool_invalid" parameter "mode": is outside the allowed enum`,
		},
		{
			name: "engine capability fence precedes media and authority",
			board: toolExecutionPreflightHeader() +
				validTool +
				toolExecutionPreflightConnection("edge_tool", "workflow", "mis_main:out", "tool_valid:port_tool_valid_in"),
			want: ErrToolExecutionUnavailable,
		},
		{
			name: "direct Store cannot bypass capability fence",
			board: toolExecutionPreflightHeader() +
				validTool +
				toolExecutionPreflightConnection("edge_tool", "workflow", "mis_main:out", "tool_valid:port_tool_valid_in"),
			directStore: true,
			want:        ErrToolExecutionUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, engine, executor, evaluator := newToolExecutionPreflightHarness(t, test.board)
			var err error
			if test.directStore {
				_, err = store.StartRun("tool-preflight", RunStartRequest{MissionID: "mis_main"})
			} else {
				_, err = engine.RunMission("tool-preflight", RunStartRequest{MissionID: "mis_main"})
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("start error = %v, want %v", err, test.want)
			}
			if test.wantText != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantText) {
					t.Fatalf("start error = %v, want descriptor detail %q", err, test.wantText)
				}
				if errors.Is(err, ErrToolExecutionUnavailable) || errors.Is(err, ErrRuntimeAuthorityNonAuthorizing) {
					t.Fatalf("later fence masked invalid reachable descriptor: %v", err)
				}
			}
			assertToolExecutionPreflightNoEffects(t, store.Workspace, executor, evaluator)
		})
	}
}

func TestToolExecutionPreflightTraversesEveryMissionBranchButNotIsolatedDownstream(t *testing.T) {
	validTool := toolExecutionPreflightTool("tool_valid", "1")
	tests := []struct {
		name      string
		board     string
		formation string
		want      error
	}{
		{
			name: "Gate fail branch reaches Tool",
			board: toolExecutionPreflightHeader() +
				toolExecutionPreflightGate("gate_review", false, "human") +
				validTool +
				toolExecutionPreflightConnection("edge_start", "workflow", "mis_main:out", "gate_review:in") +
				toolExecutionPreflightConnection("edge_fail", "workflow", "gate_review:fail", "tool_valid:port_tool_valid_in"),
			want: ErrToolExecutionUnavailable,
		},
		{
			name: "Gate judge branch reaches Tool",
			board: toolExecutionPreflightHeader() +
				toolExecutionPreflightGate("gate_review", false, "formation") +
				toolExecutionPreflightFormation("fmn_judge", false) +
				validTool +
				toolExecutionPreflightConnection("edge_start", "workflow", "mis_main:out", "gate_review:in") +
				toolExecutionPreflightConnection("edge_judge_send", "judge", "gate_review:judge", "fmn_judge:port_fmn_judge_in") +
				toolExecutionPreflightConnection("edge_judge_return", "judge", "fmn_judge:port_fmn_judge_out", "gate_review:judge") +
				toolExecutionPreflightConnection("edge_tool", "workflow", "fmn_judge:port_fmn_judge_out", "tool_valid:port_tool_valid_in"),
			want: ErrToolExecutionUnavailable,
		},
		{
			name: "disconnected invalid Tool is outside Mission root",
			board: toolExecutionPreflightHeader() +
				toolExecutionPreflightFormation("fmn_work", false) +
				toolExecutionPreflightTool("tool_valid", "999") +
				toolExecutionPreflightConnection("edge_start", "workflow", "mis_main:out", "fmn_work:port_fmn_work_in"),
			want: ErrRuntimeAuthorityNonAuthorizing,
		},
		{
			name: "unrelated inline verification is outside Mission root",
			board: toolExecutionPreflightHeader() +
				toolExecutionPreflightFormation("fmn_work", false) +
				toolExecutionPreflightFormation("fmn_legacy", true) +
				toolExecutionPreflightConnection("edge_start", "workflow", "mis_main:out", "fmn_work:port_fmn_work_in"),
			want: ErrRuntimeAuthorityNonAuthorizing,
		},
		{
			name: "downstream invalid Tool is outside isolated Formation root",
			board: toolExecutionPreflightHeader() +
				toolExecutionPreflightIsolatedFormation("fmn_work") +
				toolExecutionPreflightTool("tool_valid", "999") +
				toolExecutionPreflightConnection("edge_tool", "workflow", "fmn_work:port_fmn_work_out", "tool_valid:port_tool_valid_in"),
			formation: "fmn_work",
			want:      ErrRuntimeAuthorityNonAuthorizing,
		},
		{
			name: "unrelated inline verification is outside isolated Formation root",
			board: toolExecutionPreflightHeader() +
				toolExecutionPreflightIsolatedFormation("fmn_work") +
				toolExecutionPreflightFormation("fmn_legacy", true),
			formation: "fmn_work",
			want:      ErrRuntimeAuthorityNonAuthorizing,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, engine, executor, evaluator := newToolExecutionPreflightHarness(t, test.board)
			var err error
			if test.formation != "" {
				_, err = engine.RunFormation("tool-preflight", test.formation, FormationRunRequest{})
			} else {
				_, err = engine.RunMission("tool-preflight", RunStartRequest{MissionID: "mis_main"})
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("start error = %v, want %v", err, test.want)
			}
			assertToolExecutionPreflightNoEffects(t, store.Workspace, executor, evaluator)
		})
	}
}

func newToolExecutionPreflightHarness(t *testing.T, board string) (*Store, *RunEngine, *countingFormationExecutor, *countingGateEvaluator) {
	t.Helper()
	workspace := t.TempDir()
	store := NewRuntimeStore(workspace, filepath.Join(t.TempDir(), "missing-formations-authority"))
	writeFixture(t, store.BoardPath("tool-preflight"), board)
	executor := &countingFormationExecutor{}
	evaluator := &countingGateEvaluator{}
	engine := NewRunEngine(store, nil, executor)
	engine.SetGateEvaluator(evaluator)
	return store, engine, executor, evaluator
}

func assertToolExecutionPreflightNoEffects(t *testing.T, workspace string, executor *countingFormationExecutor, evaluator *countingGateEvaluator) {
	t.Helper()
	if executor.calls != 0 || evaluator.calls != 0 {
		t.Fatalf("preflight effects = executor:%d evaluator:%d, want zero", executor.calls, evaluator.calls)
	}
	if matches := mustGlob(t, filepath.Join(workspace, ".formations", "runs", "*")); len(matches) != 0 {
		t.Fatalf("preflight rejection created run artifacts: %v", matches)
	}
	runsRoot := filepath.Join(workspace, ".formations", "runs")
	if _, err := os.Stat(runsRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preflight rejection created durable runs root %q: %v", runsRoot, err)
	}
}

func toolExecutionPreflightHeader() string {
	return `schema = 2
id = "brd_tool_preflight"
slug = "tool-preflight"
title = "Tool execution preflight"
rev = 1

[[mission]]
id = "mis_main"
title = "Main"
goal = "Prove Tool preflight ordering"
beadId = "ctx-ug7.8.1"

`
}

func toolExecutionPreflightFormation(id string, legacyVerification bool) string {
	verification := ""
	if legacyVerification {
		verification = `
[formation.verification]
id = "ver_legacy"
kinds = ["code"]
criterion = "Legacy inline check"
onFail = "block"
`
	}
	return fmt.Sprintf(`[[formation]]
id = %q
type = "solo"
title = "Formation"

[[formation.input]]
id = %q
label = "Input"
direction = "input"
kind = "work"
acceptedMediaTypes = ["text/plain", "text/markdown", "application/json"]
required = true
role = "data"

[[formation.output]]
id = %q
label = "Output"
direction = "output"
kind = "work"
acceptedMediaTypes = ["text/plain", "text/markdown", "application/json"]
%s
`, id, "port_"+id+"_in", "port_"+id+"_out", verification)
}

func toolExecutionPreflightIsolatedFormation(id string) string {
	return fmt.Sprintf(`[[formation]]
id = %q
type = "solo"
title = "Isolated Formation"

[formation.brief]
goal = "Execute one isolated Formation"
beadId = "ctx-ug7.8.1"

[[formation.input]]
id = %q
label = "Input"
direction = "input"
kind = "work"
acceptedMediaTypes = ["application/json"]
required = true
role = "data"

[[formation.output]]
id = %q
label = "Output"
direction = "output"
kind = "work"
acceptedMediaTypes = ["text/plain", "text/markdown", "application/json"]

`, id, "port_"+id+"_in", "port_"+id+"_out")
}

func toolExecutionPreflightGate(id string, legacyScript bool, kind string) string {
	command := ""
	if legacyScript {
		command = "commandArgv = [\"npm\", \"run\", \"lint\"]\n"
	}
	return fmt.Sprintf(`[[gate]]
id = %q
title = "Review"
kinds = [%q]
criterion = "Review the work"
%s
`, id, kind, command)
}

func toolExecutionPreflightTool(id, profileVersion string) string {
	return fmt.Sprintf(`[[tool]]
id = %q
title = "Normalize JSON"
profileId = "json.normalize"
profileVersion = %q

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

`, id, profileVersion, "port_"+id+"_in", "port_"+id+"_out")
}

func toolExecutionPreflightConnection(id, channel, from, to string) string {
	return fmt.Sprintf(`[[connection]]
id = %q
channel = %q
from = %q
to = %q

`, id, channel, from, to)
}
