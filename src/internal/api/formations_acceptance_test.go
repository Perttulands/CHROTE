package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chrote/server/internal/formations"
)

func TestFormationsRunProjectionParity(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	personas := formations.NewPersonaStore(t.TempDir())
	personas.Now = fixedFormationsAPIClock()
	if _, err := personas.CreatePersona(formations.CreatePersonaRequest{ID: "scout", Kind: "specialist", Harness: "openai-codex"}); err != nil {
		t.Fatalf("create persona: %v", err)
	}
	writeFormationsAPIFixture(t, store.BoardPath("human-search"), formationsAPIS5HumanGateBoardFixture())
	handler := NewFormationsHandlerWithStores(store, personas)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	board, err := store.ReadBoard("human-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	engine := formations.NewRunEngine(store, personas, apiTestRunExecutor{})
	engine.SetGateEvaluator(apiTestGateEvaluator{verdict: "pass"})
	started, err := engine.RunMission("human-search", formations.RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
		Limits:            formations.RunLimits{MaxDispatch: 5, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("start human waiting run: %v", err)
	}
	if started.Status != formations.RunStatusRunning || started.Final {
		t.Fatalf("started = %+v, want running while human gate waits", started)
	}
	if _, err := store.RecordEscalationFromCapture(started.RunID, "fmn_work", "<<<CHROTE-ESCALATE run-id="+started.RunID+" severity=needs-attention reason='operator taste needed'>>>"); err != nil {
		t.Fatalf("record escalation: %v", err)
	}

	verdictRec := httptest.NewRecorder()
	mux.ServeHTTP(verdictRec, httptest.NewRequest(http.MethodPost, "/api/formations/runs/"+started.RunID+"/gates/gate_review/verdict", bytes.NewBufferString(`{"verdict":"pass","reason":"ship it","actor":"human:perttu"}`)))
	if verdictRec.Code != http.StatusOK {
		t.Fatalf("verdict status = %d, want %d: %s", verdictRec.Code, http.StatusOK, verdictRec.Body.String())
	}
	approved := decodeFormationsStatusResponse(t, verdictRec.Body.Bytes())
	if approved.Status != formations.RunStatusBlocked || approved.Final || !approved.ResumeAllowed {
		t.Fatalf("approved = %+v, want resumable block after public verdict", approved)
	}

	statusRec := httptest.NewRecorder()
	mux.ServeHTTP(statusRec, httptest.NewRequest(http.MethodGet, "/api/formations/runs/"+started.RunID, nil))
	if statusRec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d: %s", statusRec.Code, http.StatusOK, statusRec.Body.String())
	}
	status := decodeFormationsStatusResponse(t, statusRec.Body.Bytes())
	if status.RunID != started.RunID || status.Status != approved.Status || status.EventCount != approved.EventCount {
		t.Fatalf("status projection = %+v, want same truth as verdict response %+v", status, approved)
	}

	resumeRec := httptest.NewRecorder()
	mux.ServeHTTP(resumeRec, httptest.NewRequest(http.MethodPost, "/api/formations/runs/"+started.RunID+"/resume", bytes.NewBufferString(`{"reason":"gate approved","actor":"agent:test"}`)))
	if resumeRec.Code != http.StatusOK {
		t.Fatalf("resume status = %d, want %d: %s", resumeRec.Code, http.StatusOK, resumeRec.Body.String())
	}
	resumed := decodeFormationsStatusResponse(t, resumeRec.Body.Bytes())
	if resumed.Status != formations.RunStatusBlocked || resumed.Final {
		t.Fatalf("resumed = %+v, want blocked when resume reaches missing executor", resumed)
	}

	eventsRec := httptest.NewRecorder()
	mux.ServeHTTP(eventsRec, httptest.NewRequest(http.MethodGet, "/api/formations/runs/"+started.RunID+"/events", nil))
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("events status = %d, want %d: %s", eventsRec.Code, http.StatusOK, eventsRec.Body.String())
	}
	var eventsResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Events []formations.RunEvent `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(eventsRec.Body.Bytes(), &eventsResponse); err != nil {
		t.Fatalf("decode events: %v\n%s", err, eventsRec.Body.String())
	}
	if len(eventsResponse.Data.Events) != resumed.EventCount {
		t.Fatalf("events len = %d, resumed eventCount = %d", len(eventsResponse.Data.Events), resumed.EventCount)
	}
	if !apiEventsContain(eventsResponse.Data.Events, formations.RunEventNodeOutput, formations.RunEventHumanInputRequested, formations.RunEventEscalationRaised, formations.RunEventHumanVerdictRecorded, formations.RunEventGateVerdict, formations.RunEventBlocked, formations.RunEventResumed, formations.RunEventError) {
		t.Fatalf("events = %v, want ledger truth for output/escalation/human verdict/gate/resume/error/block", apiEventTypes(eventsResponse.Data.Events))
	}

	escalationsRec := httptest.NewRecorder()
	mux.ServeHTTP(escalationsRec, httptest.NewRequest(http.MethodGet, "/api/formations/runs/"+started.RunID+"/escalations", nil))
	if escalationsRec.Code != http.StatusOK {
		t.Fatalf("escalations status = %d, want %d: %s", escalationsRec.Code, http.StatusOK, escalationsRec.Body.String())
	}
	var escalationsResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Escalations []formations.OpenEscalation `json:"escalations"`
		} `json:"data"`
	}
	if err := json.Unmarshal(escalationsRec.Body.Bytes(), &escalationsResponse); err != nil {
		t.Fatalf("decode escalations: %v\n%s", err, escalationsRec.Body.String())
	}
	if len(escalationsResponse.Data.Escalations) != 1 || escalationsResponse.Data.Escalations[0].Reason != "operator taste needed" {
		t.Fatalf("escalations = %+v, want same escalation reason as ledger event", escalationsResponse.Data.Escalations)
	}
}

type apiTestRunExecutor struct{}

func (apiTestRunExecutor) ExecuteFormation(req formations.FormationExecution) (formations.FormationExecutionResult, error) {
	return formations.FormationExecutionResult{
		Status:    "done",
		ReportRef: "refs/" + req.NodeID + ".md",
		Text:      "api test output " + req.NodeID,
	}, nil
}

type apiTestGateEvaluator struct {
	verdict string
}

func (e apiTestGateEvaluator) EvaluateGate(formations.GateEvaluation) (formations.GateEvaluationResult, error) {
	verdict := e.verdict
	if verdict == "" {
		verdict = "pass"
	}
	return formations.GateEvaluationResult{Verdict: verdict, Reason: "api test " + verdict}, nil
}

func apiEventsContain(events []formations.RunEvent, want ...string) bool {
	seen := map[string]bool{}
	for _, event := range events {
		seen[event.Type] = true
	}
	for _, eventType := range want {
		if !seen[eventType] {
			return false
		}
	}
	return true
}

func apiEventTypes(events []formations.RunEvent) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}
