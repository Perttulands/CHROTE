package formations

import (
	"strings"
	"testing"
)

// runAmbiguousFixture starts a mission and returns its blocked/terminal events.
func runAmbiguousMission(t *testing.T, store *Store, personas *PersonaStore, engine *RunEngine) (*RunStatusProjection, []RunEvent) {
	t.Helper()
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 8, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	return status, events
}

// TestJudgeGateAmbiguousVerdictBlocksLoudly locks the A1 fix: a judge formation
// whose final output is not exactly "pass" or "fail" must block the run loudly
// with the ambiguous_gate_verdict code instead of silently routing pass.
func TestJudgeGateAmbiguousVerdictBlocksLoudly(t *testing.T) {
	cases := []struct {
		name   string
		output string
	}{
		{"prose", "the work looks good to me"},
		{"near miss passed", "passed"},
		{"json", `{"verdict":"pass"}`},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, personas := s4RunFixture(t)
			store.Now = fixedClock()
			personas.Now = fixedClock()
			createS4Persona(t, personas, "scout")
			writeFixture(t, store.BoardPath("session-search"), s4JudgeChainRunBoardFixture())
			executor := &fakeRunExecutor{outputs: map[string]string{
				"fmn_j1": "review notes",
				"fmn_j2": tc.output,
			}}
			engine := NewRunEngine(store, personas, executor)

			status, events := runAmbiguousMission(t, store, personas, engine)
			if status.Status != RunStatusBlocked || status.Final {
				t.Fatalf("status = %+v, want blocked non-final on ambiguous judge verdict", status)
			}
			// Must NOT have routed pass: ship never runs.
			for _, id := range executor.nodeIDs() {
				if id == "fmn_ship" {
					t.Fatalf("ambiguous verdict routed pass: executor ran %v", executor.nodeIDs())
				}
			}
			// No gate verdict should be recorded for an unrecognized verdict.
			for _, ev := range events {
				if ev.Type == RunEventGateVerdict {
					t.Fatalf("gate verdict recorded for ambiguous output: %+v", ev)
				}
			}
			block := lastEventOfType(t, events, RunEventBlocked)
			if block.Data["code"] != "ambiguous_gate_verdict" {
				t.Fatalf("block code = %v, want ambiguous_gate_verdict (block=%+v)", block.Data["code"], block.Data)
			}
			if block.GateID != "gate_review" {
				t.Fatalf("block gateId = %q, want gate_review", block.GateID)
			}
			reason, _ := block.Data["reason"].(string)
			if !strings.Contains(reason, "gate_review") || !strings.Contains(reason, `expected exactly "pass" or "fail"`) {
				t.Fatalf("block reason = %q, want precise expected-token message naming the gate", reason)
			}
			if boolFromEventData(block, "resumeAllowed") {
				t.Fatalf("ambiguous gate block is resumable; want non-resumable (block=%+v)", block.Data)
			}
		})
	}
}

// TestJudgeGateExactVerdictsStillRoute proves the strict parser keeps the two
// canonical verdicts working: exact "fail" routes the fail wire, exact "pass"
// routes the pass wire.
func TestJudgeGateExactVerdictsStillRoute(t *testing.T) {
	t.Run("exact fail routes fail wire", func(t *testing.T) {
		store, personas := s4RunFixture(t)
		store.Now = fixedClock()
		personas.Now = fixedClock()
		createS4Persona(t, personas, "scout")
		writeFixture(t, store.BoardPath("session-search"), s4JudgeChainRunBoardFixture(true))
		executor := &fakeRunExecutor{outputs: map[string]string{
			"fmn_j1": "review notes",
			"fmn_j2": "fail",
		}}
		engine := NewRunEngine(store, personas, executor)

		status, events := runAmbiguousMission(t, store, personas, engine)
		if status.Status != RunStatusSucceeded {
			t.Fatalf("status = %+v, want succeeded via wired fail branch", status)
		}
		verdict := eventOfType(t, events, RunEventGateVerdict)
		if verdict.Data["verdict"] != "fail" || verdict.Data["routePort"] != "fail" {
			t.Fatalf("gate verdict = %+v, want fail route", verdict)
		}
	})

	t.Run("exact pass routes pass wire", func(t *testing.T) {
		store, personas := s4RunFixture(t)
		store.Now = fixedClock()
		personas.Now = fixedClock()
		createS4Persona(t, personas, "scout")
		writeFixture(t, store.BoardPath("session-search"), s4JudgeChainRunBoardFixture())
		executor := &fakeRunExecutor{outputs: map[string]string{
			"fmn_j1": "review notes",
			"fmn_j2": "pass",
		}}
		engine := NewRunEngine(store, personas, executor)

		status, events := runAmbiguousMission(t, store, personas, engine)
		if status.Status != RunStatusSucceeded {
			t.Fatalf("status = %+v, want succeeded via pass route", status)
		}
		verdict := eventOfType(t, events, RunEventGateVerdict)
		if verdict.Data["verdict"] != "pass" || verdict.Data["routePort"] != "pass" {
			t.Fatalf("gate verdict = %+v, want pass route", verdict)
		}
	})
}

// TestGateEvaluatorAmbiguousVerdictBlocksLoudly covers the non-judge gate
// evaluator path (run_engine.go:1205): an evaluator returning an unrecognized
// verdict must block, not silently route pass.
func TestGateEvaluatorAmbiguousVerdictBlocksLoudly(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4GateBoardFixture(false))
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)
	engine.SetGateEvaluator(&fakeGateEvaluator{verdicts: []string{"approved"}})

	status, events := runAmbiguousMission(t, store, personas, engine)
	if status.Status != RunStatusBlocked || status.Final {
		t.Fatalf("status = %+v, want blocked on ambiguous evaluator verdict", status)
	}
	for _, id := range executor.nodeIDs() {
		if id == "fmn_ship" {
			t.Fatalf("ambiguous evaluator verdict routed pass: executor ran %v", executor.nodeIDs())
		}
	}
	block := lastEventOfType(t, events, RunEventBlocked)
	if block.Data["code"] != "ambiguous_gate_verdict" {
		t.Fatalf("block code = %v, want ambiguous_gate_verdict (block=%+v)", block.Data["code"], block.Data)
	}
}

// TestVerificationAmbiguousVerdictBlocksLoudly locks the verification-path half
// of A1: an inline verification verdict that is not exactly pass/fail must block
// with ambiguous_verification_verdict instead of silently passing.
func TestVerificationAmbiguousVerdictBlocksLoudly(t *testing.T) {
	cases := []struct {
		name   string
		output string
	}{
		{"prose", "the implementation seems fine overall"},
		{"near miss", "passed"},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, personas := s4RunFixture(t)
			store.Now = fixedClock()
			personas.Now = fixedClock()
			createS4Persona(t, personas, "scout")
			writeFixture(t, store.BoardPath("session-search"), s4VerificationBoardFixture("block"))
			executor := &fakeRunExecutor{}
			engine := NewRunEngine(store, personas, executor)
			engine.SetVerificationEvaluator(&fakeVerificationEvaluator{verdicts: []string{tc.output}})

			status, events := runAmbiguousMission(t, store, personas, engine)
			if status.Status != RunStatusBlocked || status.Final {
				t.Fatalf("status = %+v, want blocked on ambiguous verification verdict", status)
			}
			for _, id := range executor.nodeIDs() {
				if id == "fmn_ship" {
					t.Fatalf("ambiguous verification verdict routed pass: executor ran %v", executor.nodeIDs())
				}
			}
			// No verification_verdict event is recorded for an unrecognized verdict.
			for _, ev := range events {
				if ev.Type == RunEventVerificationVerdict {
					t.Fatalf("verification verdict recorded for ambiguous output: %+v", ev)
				}
			}
			block := lastEventOfType(t, events, RunEventBlocked)
			if block.Data["code"] != "ambiguous_verification_verdict" {
				t.Fatalf("block code = %v, want ambiguous_verification_verdict (block=%+v)", block.Data["code"], block.Data)
			}
			if block.NodeID != "fmn_work" {
				t.Fatalf("block nodeId = %q, want fmn_work", block.NodeID)
			}
			reason, _ := block.Data["reason"].(string)
			if !strings.Contains(reason, "ver_work") || !strings.Contains(reason, `expected exactly "pass" or "fail"`) {
				t.Fatalf("block reason = %q, want precise expected-token message naming the verification", reason)
			}
		})
	}
}

// TestVerificationExactVerdictsStillRoute proves strict parsing keeps the two
// canonical verification verdicts working.
func TestVerificationExactVerdictsStillRoute(t *testing.T) {
	t.Run("exact pass continues to downstream", func(t *testing.T) {
		store, personas := s4RunFixture(t)
		store.Now = fixedClock()
		personas.Now = fixedClock()
		createS4Persona(t, personas, "scout")
		writeFixture(t, store.BoardPath("session-search"), s4VerificationBoardFixture("block"))
		executor := &fakeRunExecutor{}
		engine := NewRunEngine(store, personas, executor)
		engine.SetVerificationEvaluator(&fakeVerificationEvaluator{verdicts: []string{"pass"}})

		status, events := runAmbiguousMission(t, store, personas, engine)
		if status.Status != RunStatusSucceeded {
			t.Fatalf("status = %+v, want succeeded on pass verification", status)
		}
		verdict := eventOfType(t, events, RunEventVerificationVerdict)
		if verdict.Data["verdict"] != "pass" {
			t.Fatalf("verification verdict = %+v, want pass", verdict)
		}
	})

	t.Run("exact fail blocks", func(t *testing.T) {
		store, personas := s4RunFixture(t)
		store.Now = fixedClock()
		personas.Now = fixedClock()
		createS4Persona(t, personas, "scout")
		writeFixture(t, store.BoardPath("session-search"), s4VerificationBoardFixture("block"))
		executor := &fakeRunExecutor{}
		engine := NewRunEngine(store, personas, executor)
		engine.SetVerificationEvaluator(&fakeVerificationEvaluator{verdicts: []string{"fail"}})

		status, events := runAmbiguousMission(t, store, personas, engine)
		if status.Status != RunStatusBlocked {
			t.Fatalf("status = %+v, want blocked on fail verification", status)
		}
		verdict := eventOfType(t, events, RunEventVerificationVerdict)
		if verdict.Data["verdict"] != "fail" {
			t.Fatalf("verification verdict = %+v, want fail", verdict)
		}
	})
}

// TestParseStrictVerdict locks the single source of truth for the accepted
// verdict token set: exactly the trimmed, lower-cased strings "pass" and "fail".
func TestParseStrictVerdict(t *testing.T) {
	recognized := map[string]string{
		"pass":   "pass",
		"fail":   "fail",
		" pass ": "pass",
		"PASS":   "pass",
		"Fail":   "fail",
	}
	for in, want := range recognized {
		got, ok := parseStrictVerdict(in)
		if !ok || got != want {
			t.Fatalf("parseStrictVerdict(%q) = (%q,%v), want (%q,true)", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "passed", "failed", "the work failed", "approve", "no", `{"verdict":"pass"}`} {
		if got, ok := parseStrictVerdict(in); ok {
			t.Fatalf("parseStrictVerdict(%q) = (%q,true), want not recognized", in, got)
		}
	}
}
