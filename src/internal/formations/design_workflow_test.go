package formations

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestOpenDesignWebWorkflowRunsIntegrityAndQualityRefinementLoops(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	store.Now = fixedClock()
	packRoot := openDesignPackRoot(t)

	created, err := store.InstantiateWorkflowPack(packRoot, WorkflowInstantiateRequest{
		Slug:      "control-plane",
		Title:     "Control Plane",
		Goal:      "Design and build a calm operator control plane with an obvious primary action.",
		UpdatedBy: "agent:test",
	})
	if err != nil {
		t.Fatalf("instantiate open design workflow: %v", err)
	}
	if created.Pack.ID != "open-design-web" || created.Layout == nil {
		t.Fatalf("created workflow = %+v, want open-design-web with layout", created)
	}
	if strings.Contains(created.Board.TOML, workflowPackRootToken) {
		t.Fatalf("instantiated board retained pack root token:\n%s", created.Board.TOML)
	}
	if report := ValidateBoard(created.Board); len(report.Errors) > 0 {
		t.Fatalf("instantiated board validation errors: %+v", report.Errors)
	}
	if _, err := os.Stat(filepath.Join(workspace, created.InstalledRoot, "LICENSE")); err != nil {
		t.Fatalf("installed pack license: %v", err)
	}

	executor := &openDesignTestExecutor{t: t, workspace: workspace}
	engine := NewRunEngine(store, nil, executor)
	engine.SetGateEvaluator(ScriptGateEvaluator{Workspace: workspace, Timeout: 5 * time.Second, OutputCapBytes: 16 * 1024})
	missionID := created.Board.Missions[0].ID
	status, err := engine.RunMission(created.Board.Slug, RunStartRequest{
		MissionID:         missionID,
		Actor:             "agent:test",
		ExpectedBoardETag: created.Board.ETag,
		ExpectedBoardRev:  created.Board.Rev,
		Limits:            RunLimits{MaxDispatch: 30, MaxAttempts: 4, WallClockSeconds: 60},
	})
	if err != nil {
		t.Fatalf("run to direction gate: %v", err)
	}
	if status.Status != RunStatusRunning || status.Final || status.ResumeAllowed {
		t.Fatalf("direction gate status = %+v, want running with an open human input request", status)
	}
	if got, want := executor.titles(), []string{"Creative Direction"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pre-approval execution order = %v, want %v", got, want)
	}

	directionGate := gateByTitle(t, created.Board, "Direction Approval")
	if _, err := engine.RecordHumanGateVerdict(status.RunID, HumanGateVerdictRequest{
		GateID:  directionGate.ID,
		Verdict: "pass",
		Reason:  "direction is coherent and specific",
		Actor:   "human:test",
	}); err != nil {
		t.Fatalf("approve direction: %v", err)
	}
	status, err = engine.ResumeRun(status.RunID, RunResumeRequest{
		Actor:  "agent:test",
		Mode:   "reattach",
		Reason: "direction approved",
	})
	if err != nil {
		t.Fatalf("resume design workflow: %v", err)
	}
	if status.Status != RunStatusSucceeded || !status.Final {
		events := readRunEvents(t, findOnlyRunLedger(t, store, created.Board.Slug))
		var verdicts []map[string]any
		for _, event := range events {
			if event.Type == RunEventGateVerdict {
				verdicts = append(verdicts, event.Data)
			}
		}
		t.Fatalf("final status = %+v, want succeeded; events=%v verdicts=%v", status, eventTypes(events), verdicts)
	}
	wantOrder := []string{
		"Creative Direction",
		"Prototype Builder",
		"Prototype Refiner",
		"Design Jury",
		"Prototype Refiner",
		"Design Jury",
		"Design Handoff",
	}
	if got := executor.titles(); !reflect.DeepEqual(got, wantOrder) {
		events := readRunEvents(t, findOnlyRunLedger(t, store, created.Board.Slug))
		var verdicts []map[string]any
		for _, event := range events {
			if event.Type == RunEventGateVerdict {
				verdicts = append(verdicts, event.Data)
			}
		}
		t.Fatalf("execution order = %v, want %v; events=%v verdicts=%v status=%+v", got, wantOrder, eventTypes(events), verdicts, status)
	}
	if executor.refinerInputs[0].GateFeedback == "" || !strings.Contains(executor.refinerInputs[0].GateFeedback, "placeholder") {
		t.Fatalf("first refiner feedback = %q, want validator evidence", executor.refinerInputs[0].GateFeedback)
	}
	if executor.refinerInputs[1].GateFeedback == "" || !strings.Contains(executor.refinerInputs[1].GateFeedback, "below threshold") {
		t.Fatalf("second refiner feedback = %q, want authoritative score failure", executor.refinerInputs[1].GateFeedback)
	}
	if _, err := os.Stat(filepath.Join(workspace, "prototype", "HANDOFF.md")); err != nil {
		t.Fatalf("handoff artifact: %v", err)
	}

	events := readRunEvents(t, findOnlyRunLedger(t, store, created.Board.Slug))
	var integrityFails, qualityFails, qualityPasses int
	integrityID := gateByTitle(t, created.Board, "Artifact Integrity").ID
	qualityID := gateByTitle(t, created.Board, "Design Quality").ID
	for _, event := range events {
		if event.Type != RunEventGateVerdict {
			continue
		}
		verdict := stringFromEventData(event, "verdict")
		switch {
		case event.GateID == integrityID && verdict == "fail":
			integrityFails++
		case event.GateID == qualityID && verdict == "fail":
			qualityFails++
		case event.GateID == qualityID && verdict == "pass":
			qualityPasses++
		}
	}
	if integrityFails != 1 || qualityFails != 1 || qualityPasses != 1 {
		t.Fatalf("gate verdict counts integrityFail=%d qualityFail=%d qualityPass=%d", integrityFails, qualityFails, qualityPasses)
	}
}

func TestOpenDesignWebValidatorRejectsEscapes(t *testing.T) {
	workspace := t.TempDir()
	pack, err := LoadWorkflowPack(openDesignPackRoot(t))
	if err != nil {
		t.Fatalf("load pack: %v", err)
	}
	evaluator := ScriptGateEvaluator{Workspace: workspace, Timeout: 5 * time.Second}
	result, err := evaluator.EvaluateGate(GateEvaluation{
		Kinds:       []string{"script"},
		CommandArgv: []string{"python3", filepath.Join(pack.Root, "evaluators", "validate_web.py"), "{{input.artifactRef}}"},
		Input:       RunInputRef{ArtifactRef: "../outside.html"},
	})
	if err != nil {
		t.Fatalf("evaluate escape: %v", err)
	}
	if result.Verdict != "fail" || !strings.Contains(result.Reason, "outside workspace") {
		t.Fatalf("escape result = %+v, want fail-closed workspace boundary", result)
	}
}

func TestOpenDesignWebWorkflowRunsDirectPassWithoutRefinement(t *testing.T) {
	workspace := t.TempDir()
	store := NewStore(workspace)
	store.Now = fixedClock()
	created, err := store.InstantiateWorkflowPack(openDesignPackRoot(t), WorkflowInstantiateRequest{
		Slug:      "direct-design",
		Title:     "Direct Design",
		Goal:      "Build an accessible operations console",
		UpdatedBy: "test:design",
	})
	if err != nil {
		t.Fatalf("instantiate direct workflow: %v", err)
	}

	executor := &openDesignTestExecutor{t: t, workspace: workspace, directPass: true}
	engine := NewRunEngine(store, nil, executor)
	engine.SetGateEvaluator(ScriptGateEvaluator{Workspace: workspace, Timeout: 5 * time.Second, OutputCapBytes: 16 * 1024})
	status, err := engine.RunMission(created.Board.Slug, RunStartRequest{
		MissionID:         created.Board.Missions[0].ID,
		Actor:             "agent:test",
		ExpectedBoardETag: created.Board.ETag,
		ExpectedBoardRev:  created.Board.Rev,
		Limits:            RunLimits{MaxDispatch: 20, MaxAttempts: 4, WallClockSeconds: 60},
	})
	if err != nil {
		t.Fatalf("run direct workflow to direction gate: %v", err)
	}
	directionGate := gateByTitle(t, created.Board, "Direction Approval")
	if _, err := engine.RecordHumanGateVerdict(status.RunID, HumanGateVerdictRequest{
		GateID:  directionGate.ID,
		Verdict: "pass",
		Reason:  "Approved direct direction",
		Actor:   "human:test",
	}); err != nil {
		t.Fatalf("approve direct direction: %v", err)
	}
	status, err = engine.ResumeRun(status.RunID, RunResumeRequest{Actor: "agent:test", Mode: "reattach", Reason: "direct pass"})
	if err != nil {
		t.Fatalf("resume direct workflow: %v", err)
	}
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("direct workflow status = %+v, want succeeded", status)
	}
	wantOrder := []string{"Creative Direction", "Prototype Builder", "Design Jury", "Design Handoff"}
	if got := executor.titles(); !reflect.DeepEqual(got, wantOrder) {
		t.Fatalf("direct execution order = %v, want %v", got, wantOrder)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, created.Board.Slug))
	for _, event := range events {
		if event.Type == RunEventGateVerdict && stringFromEventData(event, "verdict") == "fail" {
			t.Fatalf("direct workflow emitted unexpected failed verdict: %+v", event.Data)
		}
	}
}

type openDesignTestExecutor struct {
	t             *testing.T
	workspace     string
	directPass    bool
	calls         []FormationExecution
	juryRuns      int
	refinerInputs []RunInputRef
}

func (e *openDesignTestExecutor) ExecuteFormation(req FormationExecution) (FormationExecutionResult, error) {
	e.calls = append(e.calls, req)
	if len(req.Formation.Outputs) != 1 {
		return FormationExecutionResult{}, fmt.Errorf("test executor expected one output for %s", req.Formation.Title)
	}
	port := req.Formation.Outputs[0].ID
	payload := FormationOutputPayload{}
	switch req.Formation.Title {
	case "Creative Direction":
		payload.Text = "Lane: calm technical instrument; hierarchy: status, intervention, evidence; motion: restrained; keyboard path required."
	case "Prototype Builder":
		artifact := filepath.Join("prototype", "index.html")
		artifactHTML := invalidOpenDesignHTML()
		if e.directPass {
			artifactHTML = validOpenDesignHTML(1)
		}
		writeFixture(e.t, filepath.Join(e.workspace, artifact), artifactHTML)
		payload = FormationOutputPayload{Ref: artifact, ArtifactRef: artifact, Text: "Initial prototype with routed artifact."}
	case "Prototype Refiner":
		if len(req.Inputs) != 1 {
			return FormationExecutionResult{}, fmt.Errorf("refiner inputs = %d, want 1", len(req.Inputs))
		}
		e.refinerInputs = append(e.refinerInputs, req.Inputs[0])
		artifact := req.Inputs[0].ArtifactRef
		if artifact == "" {
			artifact = filepath.Join("prototype", "index.html")
		}
		writeFixture(e.t, filepath.Join(e.workspace, artifact), validOpenDesignHTML(len(e.refinerInputs)))
		payload = FormationOutputPayload{Ref: artifact, ArtifactRef: artifact, Text: fmt.Sprintf("Refined prototype revision %d.", len(e.refinerInputs))}
	case "Design Jury":
		e.juryRuns++
		artifact := req.Inputs[0].ArtifactRef
		if e.juryRuns == 1 && !e.directPass {
			payload = FormationOutputPayload{ArtifactRef: artifact, Text: openDesignScorecard(6.5, []string{"Primary action lacks enough visual emphasis"})}
		} else {
			payload = FormationOutputPayload{ArtifactRef: artifact, Text: openDesignScorecard(8.5, nil)}
		}
	case "Design Handoff":
		handoff := filepath.Join("prototype", "HANDOFF.md")
		writeFixture(e.t, filepath.Join(e.workspace, handoff), "# Handoff\n\nValidated prototype: prototype/index.html\nFinal jury: passing.\n")
		payload = FormationOutputPayload{Ref: handoff, ArtifactRef: filepath.Join("prototype", "index.html"), Text: "Handoff packaged with evidence."}
	default:
		return FormationExecutionResult{}, fmt.Errorf("unexpected formation %q", req.Formation.Title)
	}
	return FormationExecutionResult{Status: "done", Outputs: map[string]FormationOutputPayload{port: payload}}, nil
}

func (e *openDesignTestExecutor) titles() []string {
	values := make([]string, 0, len(e.calls))
	for _, call := range e.calls {
		values = append(values, call.Formation.Title)
	}
	return values
}

func gateByTitle(t *testing.T, board *BoardDocument, title string) GateNode {
	t.Helper()
	for _, gate := range board.Gates {
		if gate.Title == title {
			return gate
		}
	}
	t.Fatalf("gate %q not found", title)
	return GateNode{}
}

func openDesignPackRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "formation-packs", "open-design-web"))
}

func openDesignScorecard(score float64, mustFix []string) string {
	mustFixJSON := "[]"
	if len(mustFix) > 0 {
		mustFixJSON = fmt.Sprintf("[%q]", mustFix[0])
	}
	return fmt.Sprintf(`{"schema":1,"artifactRef":"prototype/index.html","summary":"Independent jury evidence","reviews":[{"reviewer":"critic","score":%.1f,"evidence":["Hierarchy inspected"],"mustFix":%s},{"reviewer":"brand","score":%.1f,"evidence":["Token system inspected"],"mustFix":[]},{"reviewer":"a11y","score":%.1f,"evidence":["Keyboard and semantics inspected"],"mustFix":[]},{"reviewer":"copy","score":%.1f,"evidence":["Labels inspected"],"mustFix":[]}]}`, score, mustFixJSON, score, score, score)
}

func invalidOpenDesignHTML() string {
	return `<!doctype html><html lang="en"><head><meta name="viewport" content="width=device-width"><title>Control plane</title><style>:focus-visible{outline:2px solid blue}</style></head><body><main><h1>Lorem ipsum</h1><button>Act</button></main></body></html>`
}

func validOpenDesignHTML(revision int) string {
	return fmt.Sprintf(`<!doctype html><html lang="en"><head><meta name="viewport" content="width=device-width,initial-scale=1"><title>Control plane</title><style>body{font-family:system-ui;margin:0;background:#0b1020;color:#eef2ff}main{max-width:68rem;margin:auto;padding:4rem 1.5rem}.status{color:#8ee6c0}button{padding:.8rem 1rem;background:#8ee6c0;color:#071018;border:0}button:focus-visible{outline:3px solid #fff;outline-offset:3px}@media(max-width:40rem){main{padding-top:2rem}}</style></head><body><main><p class="status">Systems nominal · revision %d</p><h1>Intervene with context, not guesswork.</h1><p>Inspect the evidence trail before changing the active formation.</p><button type="button">Open intervention brief</button></main></body></html>`, revision)
}
