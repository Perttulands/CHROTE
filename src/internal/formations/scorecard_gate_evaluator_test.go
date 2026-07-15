package formations

import (
	"strings"
	"testing"
)

func TestScorecardGateEvaluatorRecomputesWeightedScoreAndPasses(t *testing.T) {
	evaluator := ScorecardGateEvaluator{}
	result, err := evaluator.EvaluateGate(GateEvaluation{
		Kinds:             []string{"scorecard"},
		ScoreThreshold:    8.0,
		RequireNoMustFix:  true,
		RequiredReviewers: []string{"critic", "brand", "a11y", "copy"},
		ReviewerWeights:   []string{"critic=0.4", "brand=0.2", "a11y=0.2", "copy=0.2"},
		Input: RunInputRef{ArtifactRef: "site/index.html", Text: `{
			"schema": 1,
			"claimedComposite": 1.0,
			"artifactRef": "site/index.html",
			"reviews": [
				{"reviewer":"critic","score":8.5,"evidence":["Hierarchy is explicit"],"mustFix":[]},
				{"reviewer":"brand","score":8.0,"evidence":["Tokens match DESIGN.md"],"mustFix":[]},
				{"reviewer":"a11y","score":8.0,"evidence":["Keyboard path verified"],"mustFix":[]},
				{"reviewer":"copy","score":8.0,"evidence":["CTA is specific"],"mustFix":[]}
			]
		}`},
	})
	if err != nil {
		t.Fatalf("evaluate scorecard: %v", err)
	}
	if result.Verdict != "pass" || result.PerKind["scorecard"] != "pass" {
		t.Fatalf("result = %+v, want pass", result)
	}
	if !strings.Contains(result.Reason, "8.20") || !strings.Contains(result.Reason, "mustFix=0") {
		t.Fatalf("reason = %q, want recomputed 8.20 and mustFix count", result.Reason)
	}
	if strings.Contains(result.Reason, "1.00") {
		t.Fatalf("reason trusted claimed composite: %q", result.Reason)
	}
}

func TestScorecardGateEvaluatorFailsThresholdMustFixAndMissingEvidence(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "threshold",
			text: `{"schema":1,"artifactRef":"site/index.html","reviews":[{"reviewer":"critic","score":7.9,"evidence":["Flat hierarchy"],"mustFix":[]}]}`,
			want: "below threshold",
		},
		{
			name: "must fix",
			text: `{"schema":1,"artifactRef":"site/index.html","reviews":[{"reviewer":"critic","score":9.0,"evidence":["Button inspected"],"mustFix":["Repair invisible focus"]}]}`,
			want: "mustFix=1",
		},
		{
			name: "missing evidence",
			text: `{"schema":1,"artifactRef":"site/index.html","reviews":[{"reviewer":"critic","score":9.0,"evidence":[],"mustFix":[]}]}`,
			want: "evidence",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := (ScorecardGateEvaluator{}).EvaluateGate(GateEvaluation{
				Kinds:             []string{"scorecard"},
				ScoreThreshold:    8,
				RequireNoMustFix:  true,
				RequiredReviewers: []string{"critic"},
				ReviewerWeights:   []string{"critic=1.0"},
				Input:             RunInputRef{ArtifactRef: "site/index.html", Text: tt.text},
			})
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			if result.Verdict != "fail" || !strings.Contains(result.Reason, tt.want) {
				t.Fatalf("result = %+v, want fail containing %q", result, tt.want)
			}
		})
	}
}

func TestRunEngineRecomputesScorecardProducedByJudgeChain(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Now = fixedClock()
	writeFixture(t, store.BoardPath("scorecard-chain"), `schema = 1
id = "brd_scorecard_chain"
slug = "scorecard-chain"
title = "Scorecard chain"
rev = 1

[[mission]]
id = "mis_scorecard_chain"
title = "Scorecard chain"
goal = "Prove authoritative scoring"
`)
	persisted, err := store.ReadBoard("scorecard-chain")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	started, err := store.StartRun("scorecard-chain", RunStartRequest{
		MissionID:         "mis_scorecard_chain",
		Actor:             "test:scorecard",
		ExpectedBoardETag: persisted.ETag,
		ExpectedBoardRev:  persisted.Rev,
		Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 2, WallClockSeconds: 60},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}

	passingScorecard := `{"schema":1,"claimedComposite":1,"artifactRef":"site/index.html","reviews":[{"reviewer":"critic","score":9,"evidence":["render"]},{"reviewer":"brand","score":9,"evidence":["tokens"]}]}`
	executor := &fakeRunExecutor{
		outputs: map[string]string{"fmn_judge": "top-level commentary must not decide the gate"},
		outputsByPort: map[string]map[string]FormationOutputPayload{
			"fmn_judge": {"port_judge_out": {Text: passingScorecard, ArtifactRef: "site/index.html"}},
		},
	}
	engine := NewRunEngine(store, nil, executor)
	gate := GateNode{ID: "gate_score", Kinds: []string{"scorecard"}}
	board := &BoardDocument{
		Gates: []GateNode{gate},
		Formations: []FormationNode{{
			ID:      "fmn_judge",
			Title:   "Scorecard judge",
			Inputs:  []FormationPort{{ID: "port_judge_in"}},
			Outputs: []FormationPort{{ID: "port_judge_out"}},
		}},
		Connections: []BoardConnection{
			{ID: "edge_gate_judge", From: "gate_score:judge", To: "fmn_judge:port_judge_in"},
			{ID: "edge_judge_gate", From: "fmn_judge:port_judge_out", To: "gate_score:judge"},
		},
	}
	result, err := engine.evaluateGateResult(board, gate, GateEvaluation{
		RunID:             started.RunID,
		GateID:            gate.ID,
		Kinds:             gate.Kinds,
		ScoreThreshold:    8,
		RequiredReviewers: []string{"critic", "brand"},
		ReviewerWeights:   []string{"critic=0.5", "brand=0.5"},
		RequireNoMustFix:  true,
		Input:             RunInputRef{ArtifactRef: "site/index.html", Text: "artifact"},
	}, RunLimits{MaxDispatch: 5, MaxAttempts: 2, WallClockSeconds: 60})
	if err != nil {
		t.Fatalf("evaluate chained scorecard: %v", err)
	}
	if result.Verdict != "pass" || !strings.Contains(result.Reason, "authoritative score 9.00") {
		t.Fatalf("chained scorecard result = %+v, want recomputed pass", result)
	}
}

func TestScorecardGateEvaluatorRejectsMalformedOrUntrustedPolicy(t *testing.T) {
	tests := []struct {
		name    string
		req     GateEvaluation
		wantErr string
	}{
		{name: "wrong kind", req: GateEvaluation{Kinds: []string{"lint"}}, wantErr: "scorecard kind"},
		{name: "missing threshold", req: GateEvaluation{Kinds: []string{"scorecard"}, RequiredReviewers: []string{"critic"}, ReviewerWeights: []string{"critic=1"}, Input: RunInputRef{Text: `{}`}}, wantErr: "threshold"},
		{name: "weights do not sum", req: GateEvaluation{Kinds: []string{"scorecard"}, ScoreThreshold: 8, RequiredReviewers: []string{"critic"}, ReviewerWeights: []string{"critic=0.5"}, Input: RunInputRef{Text: `{}`}}, wantErr: "sum to 1"},
		{name: "duplicate reviewer", req: GateEvaluation{Kinds: []string{"scorecard"}, ScoreThreshold: 8, RequiredReviewers: []string{"critic"}, ReviewerWeights: []string{"critic=1"}, Input: RunInputRef{ArtifactRef: "site/index.html", Text: `{"schema":1,"artifactRef":"site/index.html","reviews":[{"reviewer":"critic","score":8,"evidence":["one"]},{"reviewer":"critic","score":8,"evidence":["two"]}]}`}}, wantErr: "duplicate reviewer"},
		{name: "unknown reviewer", req: GateEvaluation{Kinds: []string{"scorecard"}, ScoreThreshold: 8, RequiredReviewers: []string{"critic"}, ReviewerWeights: []string{"critic=1"}, Input: RunInputRef{ArtifactRef: "site/index.html", Text: `{"schema":1,"artifactRef":"site/index.html","reviews":[{"reviewer":"sales","score":10,"evidence":["trust me"]}]}`}}, wantErr: "unexpected reviewer"},
		{name: "artifact mismatch", req: GateEvaluation{Kinds: []string{"scorecard"}, ScoreThreshold: 8, RequiredReviewers: []string{"critic"}, ReviewerWeights: []string{"critic=1"}, Input: RunInputRef{ArtifactRef: "site/other.html", Text: `{"schema":1,"artifactRef":"site/index.html","reviews":[{"reviewer":"critic","score":9,"evidence":["render"]}]}`}}, wantErr: "does not match"},
		{name: "malformed json", req: GateEvaluation{Kinds: []string{"scorecard"}, ScoreThreshold: 8, RequiredReviewers: []string{"critic"}, ReviewerWeights: []string{"critic=1"}, Input: RunInputRef{Text: `{nope`}}, wantErr: "invalid scorecard"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := (ScorecardGateEvaluator{}).EvaluateGate(tt.req)
			if err != nil {
				t.Fatalf("evaluate returned transport error: %v", err)
			}
			if result.Verdict != "fail" || !strings.Contains(result.Reason, tt.wantErr) {
				t.Fatalf("result = %+v, want fail containing %q", result, tt.wantErr)
			}
		})
	}
}
