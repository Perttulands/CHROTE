package formations

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCodeGateEvaluatorRequiresExactFrozenBinding(t *testing.T) {
	descriptor, ok := LookupCodeGateProfileDescriptor("output_contains", "1")
	if !ok {
		t.Fatal("missing output_contains@1 descriptor")
	}
	binding := newRunGateBinding("run_test", GateNode{
		ID:           "gate_lint",
		Check:        "output_contains",
		CheckVersion: "1",
		CheckValue:   "LINT OK",
	}, descriptor)
	tests := []struct {
		name   string
		mutate func(*RunGateBinding)
	}{
		{name: "profile content", mutate: func(binding *RunGateBinding) { binding.ProfileSHA256 = strings.Repeat("0", 64) }},
		{name: "evaluator content", mutate: func(binding *RunGateBinding) { binding.EvaluatorBundleSHA256 = strings.Repeat("0", 64) }},
		{name: "parameters", mutate: func(binding *RunGateBinding) { binding.Parameters["value"] = "different" }},
		{name: "unknown parameter", mutate: func(binding *RunGateBinding) { binding.Parameters["extra"] = "undeclared" }},
		{name: "policy", mutate: func(binding *RunGateBinding) { binding.PolicySHA256 = strings.Repeat("0", 64) }},
		{name: "determinism policy", mutate: func(binding *RunGateBinding) { binding.DeterminismPolicySHA256 = strings.Repeat("0", 64) }},
		{name: "result encoding", mutate: func(binding *RunGateBinding) { binding.ResultEncoding = "other" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			altered := binding
			altered.Parameters = map[string]string{"value": binding.Parameters["value"]}
			test.mutate(&altered)
			_, err := NewCodeGateEvaluator().EvaluateGate(GateEvaluation{
				RunID:        "run_test",
				GateID:       "gate_lint",
				Kinds:        []string{"code"},
				Check:        "output_contains",
				CheckVersion: "1",
				CheckValue:   "LINT OK",
				Input:        RunInputRef{Text: "lint clean — LINT OK"},
				Binding:      &altered,
			})
			if err == nil {
				t.Fatalf("evaluation accepted mismatched frozen %s binding", test.name)
			}
		})
	}
}

func TestCodeGateEvaluatorProducesStableCanonicalDecisionResult(t *testing.T) {
	descriptor, ok := LookupCodeGateProfileDescriptor("output_contains", "1")
	if !ok {
		t.Fatal("missing output_contains@1 descriptor")
	}
	gate := GateNode{
		ID:           "gate_lint",
		Kinds:        []string{"code"},
		Check:        "output_contains",
		CheckVersion: "1",
		CheckValue:   "LINT OK",
	}
	req := GateEvaluation{
		RunID:        "run_test",
		GateID:       gate.ID,
		Kinds:        gate.Kinds,
		Check:        gate.Check,
		CheckVersion: gate.CheckVersion,
		CheckValue:   gate.CheckValue,
		Input:        RunInputRef{Text: "lint clean — LINT OK"},
		Binding:      ptrRunGateBinding(newRunGateBinding("run_test", gate, descriptor)),
	}

	first, err := NewCodeGateEvaluator().EvaluateGate(req)
	if err != nil {
		t.Fatalf("first evaluation: %v", err)
	}
	second, err := NewCodeGateEvaluator().EvaluateGate(req)
	if err != nil {
		t.Fatalf("second evaluation: %v", err)
	}
	if first.ResultEncoding != CodeGateResultEncoding {
		t.Fatalf("result encoding = %q, want %q", first.ResultEncoding, CodeGateResultEncoding)
	}
	if first.CanonicalResult == "" || first.ResultSHA256 == "" {
		t.Fatalf("canonical result metadata is incomplete: %+v", first)
	}
	if first.CanonicalResult != second.CanonicalResult || first.ResultSHA256 != second.ResultSHA256 {
		t.Fatalf("repeated canonical results differ:\nfirst:  %+v\nsecond: %+v", first, second)
	}
	if got := codeGateSHA256(first.CanonicalResult); got != first.ResultSHA256 {
		t.Fatalf("result sha256 = %q, want canonical content hash %q", first.ResultSHA256, got)
	}
}

func TestCodeGateCanonicalBytesUseRFC8785StringEscaping(t *testing.T) {
	const value = "<line\u2028break>"
	if got, want := codeGateParametersCanonical(value), "{\"value\":\"<line\u2028break>\"}"; got != want {
		t.Fatalf("canonical parameters = %q, want RFC 8785 bytes %q", got, want)
	}
	got, err := canonicalCodeGateResult(
		"pass",
		value,
		[]GateEvidenceRef{{Kind: "text", Text: value}},
	)
	if err != nil {
		t.Fatalf("canonical result: %v", err)
	}
	want := "{\"evidence\":[{\"kind\":\"text\",\"text\":\"<line\u2028break>\"}],\"reason\":\"<line\u2028break>\",\"verdict\":\"pass\"}"
	if got != want {
		t.Fatalf("canonical result = %q, want RFC 8785 bytes %q", got, want)
	}
}

func TestCodeGateEvaluatorRejectsBoundedInputExhaustion(t *testing.T) {
	descriptor, ok := LookupCodeGateProfileDescriptor("output_contains", "1")
	if !ok {
		t.Fatal("missing output_contains@1 descriptor")
	}
	gate := GateNode{
		ID:           "gate_lint",
		Kinds:        []string{"code"},
		Check:        "output_contains",
		CheckVersion: "1",
		CheckValue:   "LINT OK",
	}
	_, err := NewCodeGateEvaluator().EvaluateGate(GateEvaluation{
		RunID:        "run_test",
		GateID:       gate.ID,
		Kinds:        gate.Kinds,
		Check:        gate.Check,
		CheckVersion: gate.CheckVersion,
		CheckValue:   gate.CheckValue,
		Input:        RunInputRef{Text: strings.Repeat("x", descriptor.MaxInputBytes+1)},
		Binding:      ptrRunGateBinding(newRunGateBinding("run_test", gate, descriptor)),
	})
	if err == nil || !strings.Contains(err.Error(), "maxInputBytes") {
		t.Fatalf("input exhaustion error = %v, want bounded maxInputBytes rejection", err)
	}
}

func TestCodeGateRejectsTamperedCanonicalResultBeforeDurabilityOrRoute(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), codeGateLintBoardFixture("output_contains", "LINT OK"))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &attemptOutputExecutor{textByAttempt: map[string]map[int]string{
		"fmn_work": {1: "LINT OK"},
	}}
	engine := NewRunEngine(store, personas, executor)
	engine.SetGateEvaluator(tamperedCanonicalGateEvaluator{})

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 8, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusBlocked {
		t.Fatalf("status = %+v, want blocked on canonical result mismatch", status)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	errEvent := eventOfType(t, events, RunEventError)
	if errEvent.Data["code"] != "gate_evaluator_error" {
		t.Fatalf("error = %+v, want gate_evaluator_error", errEvent)
	}
	for _, event := range events {
		if event.Type == RunEventGateKindResult || event.Type == RunEventGateVerdict {
			t.Fatalf("tampered canonical result became durable or routed: %+v", event)
		}
	}
	if got := executor.nodeIDs(); !reflect.DeepEqual(got, []string{"fmn_work"}) {
		t.Fatalf("executor nodes = %v, want no downstream route", got)
	}
}

type tamperedCanonicalGateEvaluator struct{}

func (tamperedCanonicalGateEvaluator) EvaluateGate(req GateEvaluation) (GateEvaluationResult, error) {
	canonical, err := canonicalCodeGateResult("pass", "tampered", nil)
	if err != nil {
		return GateEvaluationResult{}, err
	}
	return GateEvaluationResult{
		Verdict:         "pass",
		Reason:          "tampered",
		ResultEncoding:  CodeGateResultEncoding,
		ResultSHA256:    strings.Repeat("0", 64),
		CanonicalResult: canonical,
		GateBindingID:   req.Binding.GateBindingID,
	}, nil
}

func TestCodeGatePassRoutesExactSelectedToolOutputAndProvenance(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), codeGateLintBoardFixture("output_contains", "LINT OK"))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	started, err := store.StartRun("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	gate := board.Gates[0]
	input := RunInputRef{
		EdgeID:     "edge_tool_gate",
		FromNodeID: "tool_lint",
		FromPortID: "normalized",
		ToPortID:   "in",
		OutputSeq:  4,
		Ref:        "artifact://tool_lint/output/4",
		Text:       "normalized lint output — LINT OK",
		ReportRef:  "reports/tool-lint.md",
	}
	binding, err := store.readRunGateBinding(started.RunID, gate.ID)
	if err != nil {
		t.Fatalf("read Gate binding: %v", err)
	}
	result, err := NewCodeGateEvaluator().EvaluateGate(GateEvaluation{
		RunID:        started.RunID,
		GateID:       gate.ID,
		Kinds:        gate.Kinds,
		Check:        gate.Check,
		CheckVersion: gate.CheckVersion,
		CheckValue:   gate.CheckValue,
		Input:        input,
		Binding:      binding,
	})
	if err != nil {
		t.Fatalf("evaluate exact Tool output: %v", err)
	}
	engine := NewRunEngine(store, personas, &fakeRunExecutor{})
	ready := map[string]map[string]RunInputRef{}
	queued := map[string]bool{}
	queue := []string{}
	if err := engine.routeGateEvaluation(
		started.RunID,
		board,
		map[string]GateNode{gate.ID: gate},
		gate,
		input,
		"pass",
		result,
		RunLimits{},
		ready,
		queued,
		&queue,
	); err != nil {
		t.Fatalf("route Gate pass: %v", err)
	}
	got := ready["fmn_ship"]["port_ship_in"]
	want := input
	want.ToPortID = "port_ship_in"
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("routed input = %+v, want exact selected Tool output/provenance %+v", got, want)
	}
}

func TestMixedCodeFormationGateRunsCodeFirstAndRecordsBothKindResults(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	raw := strings.Replace(s4JudgeChainRunBoardFixture(), `checkValue = "output from"`, `checkValue = "LINT OK"`, 1)
	writeFixture(t, store.BoardPath("session-search"), raw)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{outputs: map[string]string{
		"fmn_work": "LINT OK",
		"fmn_j1":   "review notes",
		"fmn_j2":   "pass",
	}}
	evaluator := &countingCodeGateEvaluator{}
	engine := NewRunEngine(store, personas, executor)
	engine.SetGateEvaluator(evaluator)

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 8, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusSucceeded {
		t.Fatalf("status = %+v, want succeeded", status)
	}
	if evaluator.calls != 1 {
		t.Fatalf("code evaluator calls = %d, want one before the judge chain", evaluator.calls)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	var kindResults []RunEvent
	for _, event := range events {
		if event.Type == "gate_kind_result" {
			kindResults = append(kindResults, event)
		}
	}
	if len(kindResults) != 2 {
		t.Fatalf("Gate kind results = %+v, want durable code then formation results", kindResults)
	}
	if kindResults[0].Data["kind"] != "code" || kindResults[0].Data["verdict"] != "pass" {
		t.Fatalf("first Gate kind result = %+v, want code pass", kindResults[0])
	}
	if kindResults[0].Data["resultEncoding"] != CodeGateResultEncoding ||
		kindResults[0].Data["resultSha256"] == "" ||
		kindResults[0].Data["gateBindingId"] == "" {
		t.Fatalf("code kind result lacks frozen canonical provenance: %+v", kindResults[0])
	}
	if kindResults[1].Data["kind"] != "formation" || kindResults[1].Data["verdict"] != "pass" {
		t.Fatalf("second Gate kind result = %+v, want formation pass", kindResults[1])
	}
	verdict := eventOfType(t, events, RunEventGateVerdict)
	if got, want := verdict.Data["perKind"], map[string]any{"code": "pass", "formation": "pass"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("aggregate perKind = %#v, want %#v", got, want)
	}
	if verdict.Data["resultSha256"] != kindResults[0].Data["resultSha256"] {
		t.Fatalf("aggregate code result hash = %#v, want durable code result %#v", verdict.Data["resultSha256"], kindResults[0].Data["resultSha256"])
	}
}

func TestMixedCodeHumanGateReusesDurableCodeResultInAggregateVerdict(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	raw := strings.Replace(s5HumanGateBoardFixture(), `kinds = ["human"]`, `kinds = ["code", "human"]`, 1)
	raw = strings.Replace(
		raw,
		`criterion = "Good enough to ship"`,
		`criterion = "Good enough to ship"`+"\n"+
			`check = "output_contains"`+"\n"+
			`checkVersion = "1"`+"\n"+
			`checkValue = "LINT OK"`,
		1,
	)
	writeFixture(t, store.BoardPath("session-search"), raw)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{outputs: map[string]string{"fmn_work": "LINT OK"}}
	evaluator := &countingCodeGateEvaluator{}
	engine := NewRunEngine(store, personas, executor)
	engine.SetGateEvaluator(evaluator)

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	codeResult := eventOfType(t, events, "gate_kind_result")
	request := eventOfType(t, events, RunEventHumanInputRequested)
	if request.Data["resultSha256"] != codeResult.Data["resultSha256"] ||
		request.Data["gateBindingId"] != codeResult.Data["gateBindingId"] ||
		intFromRunEventData(request.Data["codeResultSeq"]) != codeResult.Seq {
		t.Fatalf("human request did not freeze the durable code result: request=%+v code=%+v", request, codeResult)
	}

	status, err = engine.RecordHumanGateVerdict(status.RunID, HumanGateVerdictRequest{
		GateID:  "gate_review",
		Verdict: "pass",
		Actor:   "human:test",
	})
	if err != nil {
		t.Fatalf("record human verdict: %v", err)
	}
	if evaluator.calls != 1 {
		t.Fatalf("code evaluator calls = %d, want durable result reused without reevaluation", evaluator.calls)
	}
	events = readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	verdict := eventOfType(t, events, RunEventGateVerdict)
	if got, want := verdict.Data["perKind"], map[string]any{"code": "pass", "human": "pass"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("aggregate perKind = %#v, want %#v", got, want)
	}
	if verdict.Data["resultSha256"] != codeResult.Data["resultSha256"] ||
		verdict.Data["resultEncoding"] != codeResult.Data["resultEncoding"] ||
		verdict.Data["gateBindingId"] != codeResult.Data["gateBindingId"] ||
		!reflect.DeepEqual(verdict.Data["evidence"], codeResult.Data["evidence"]) {
		t.Fatalf("aggregate verdict lost durable code result provenance: verdict=%+v code=%+v", verdict, codeResult)
	}
}

func TestMixedCodeFormationHumanGateFreezesBothPriorKindResultSequences(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	raw := strings.Replace(s4JudgeChainRunBoardFixture(), `kinds = ["code", "formation"]`, `kinds = ["code", "formation", "human"]`, 1)
	raw = strings.Replace(raw, `checkValue = "output from"`, `checkValue = "LINT OK"`, 1)
	writeFixture(t, store.BoardPath("session-search"), raw)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{outputs: map[string]string{
		"fmn_work": "LINT OK",
		"fmn_j1":   "review notes",
		"fmn_j2":   "pass",
	}}
	engine := NewRunEngine(store, personas, executor)
	engine.SetGateEvaluator(NewCodeGateEvaluator())

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 8, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusRunning {
		t.Fatalf("status = %+v, want waiting for human after code and formation pass", status)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	var kindResults []RunEvent
	for _, event := range events {
		if event.Type == RunEventGateKindResult {
			kindResults = append(kindResults, event)
		}
	}
	if len(kindResults) != 2 {
		t.Fatalf("kind results = %+v, want code and formation", kindResults)
	}
	request := eventOfType(t, events, RunEventHumanInputRequested)
	seqs, ok := request.Data["kindResultSeqs"].(map[string]any)
	if !ok ||
		intFromRunEventData(seqs["code"]) != kindResults[0].Seq ||
		intFromRunEventData(seqs["formation"]) != kindResults[1].Seq {
		t.Fatalf("human request kindResultSeqs = %#v, want code:%d formation:%d", request.Data["kindResultSeqs"], kindResults[0].Seq, kindResults[1].Seq)
	}
}

func TestResumeReusesDurableCodeKindResultAfterCrashWindow(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	raw := strings.Replace(codeGateLintBoardFixture("output_contains", "LINT OK"), `kinds = ["code"]`, `kinds = ["code", "human"]`, 1)
	writeFixture(t, store.BoardPath("session-search"), raw)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	started, err := store.StartRun("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
		Limits:            RunLimits{MaxDispatch: 8, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	input := RunInputRef{
		EdgeID:     "edge_work_gate",
		FromNodeID: "fmn_work",
		FromPortID: "port_work_out",
		ToPortID:   "in",
		OutputSeq:  1,
		Ref:        "ledger://work/1",
		Text:       "LINT OK",
	}
	for _, event := range []RunEvent{
		{Type: RunEventNodeStarted, NodeID: "fmn_work", Attempt: 1, Data: map[string]any{"nodeKind": "formation"}},
		{Type: RunEventNodeOutput, NodeID: "fmn_work", Data: formationOutputEventData(FormationExecutionResult{
			Status: "done",
			Text:   input.Text,
			Outputs: map[string]FormationOutputPayload{
				"port_work_out": {Ref: input.Ref, Text: input.Text},
			},
		})},
		{Type: RunEventGateEvaluating, GateID: "gate_lint", NodeID: "gate_lint", Data: map[string]any{"inputRef": input}},
	} {
		if err := store.AppendRunEvent(started.RunID, event); err != nil {
			t.Fatalf("append %s: %v", event.Type, err)
		}
	}
	binding, err := store.readRunGateBinding(started.RunID, "gate_lint")
	if err != nil {
		t.Fatalf("read binding: %v", err)
	}
	result, err := NewCodeGateEvaluator().EvaluateGate(GateEvaluation{
		RunID:        started.RunID,
		GateID:       "gate_lint",
		Kinds:        []string{"code"},
		Check:        "output_contains",
		CheckVersion: "1",
		CheckValue:   "LINT OK",
		Input:        input,
		Binding:      binding,
	})
	if err != nil {
		t.Fatalf("evaluate code result: %v", err)
	}
	setupEngine := NewRunEngine(store, personas, &fakeRunExecutor{})
	codeSeq, err := setupEngine.appendGateKindResult(started.RunID, board.Gates[0], "code", input, result)
	if err != nil {
		t.Fatalf("append code kind result: %v", err)
	}
	if err := store.AppendRunEvent(started.RunID, RunEvent{
		Type:   RunEventBlocked,
		GateID: "gate_lint",
		NodeID: "gate_lint",
		Data: map[string]any{
			"reason":        "crashed after code result",
			"resumeAllowed": true,
			"resumePolicy":  "explicit",
		},
	}); err != nil {
		t.Fatalf("append crash block: %v", err)
	}

	evaluator := &countingCodeGateEvaluator{}
	engine := NewRunEngine(store, personas, &fakeRunExecutor{})
	engine.SetGateEvaluator(evaluator)
	status, err := engine.ResumeRun(started.RunID, RunResumeRequest{
		Actor:  "agent:test",
		Mode:   "reattach",
		Reason: "recover Gate",
	})
	if err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if status.Status != RunStatusRunning || status.Final {
		t.Fatalf("status = %+v, want waiting human with reused code result", status)
	}
	if evaluator.calls != 0 {
		t.Fatalf("code evaluator calls = %d, want durable result reuse", evaluator.calls)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	if got := countEventsForNode(events, RunEventGateKindResult, "gate_lint"); got != 1 {
		t.Fatalf("code kind result count = %d, want exactly one", got)
	}
	request := eventOfType(t, events, RunEventHumanInputRequested)
	if intFromRunEventData(request.Data["codeResultSeq"]) != codeSeq {
		t.Fatalf("human request codeResultSeq = %#v, want %d", request.Data["codeResultSeq"], codeSeq)
	}
}

func TestHumanVerdictRejectsTamperedPriorCodeResultBeforeMutation(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	raw := strings.Replace(codeGateLintBoardFixture("output_contains", "LINT OK"), `kinds = ["code"]`, `kinds = ["code", "human"]`, 1)
	writeFixture(t, store.BoardPath("session-search"), raw)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	engine := NewRunEngine(store, personas, &fakeRunExecutor{outputs: map[string]string{"fmn_work": "LINT OK"}})
	engine.SetGateEvaluator(NewCodeGateEvaluator())
	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 8, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	ledger := findOnlyRunLedger(t, store, "session-search")
	events := readRunEvents(t, ledger)
	codeResult := eventOfType(t, events, RunEventGateKindResult)
	resultHash := stringFromAny(codeResult.Data["resultSha256"])
	ledgerRaw, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	tampered := strings.Replace(string(ledgerRaw), resultHash, strings.Repeat("0", 64), 1)
	if tampered == string(ledgerRaw) {
		t.Fatal("test did not tamper the durable code result")
	}
	if err := os.WriteFile(ledger, []byte(tampered), 0o600); err != nil {
		t.Fatalf("tamper ledger: %v", err)
	}

	before := readRunEvents(t, ledger)
	_, err = engine.RecordHumanGateVerdict(status.RunID, HumanGateVerdictRequest{
		GateID:  "gate_lint",
		Verdict: "pass",
		Actor:   "human:test",
	})
	if !errors.Is(err, ErrRunLedgerInvalid) {
		t.Fatalf("record human verdict error = %v, want ErrRunLedgerInvalid", err)
	}
	after := readRunEvents(t, ledger)
	if len(after) != len(before) {
		t.Fatalf("tampered result rejection appended events: before=%d after=%d", len(before), len(after))
	}
}

func TestResumeRejectsTamperedDurableCodeResultBeforeMutation(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	raw := strings.Replace(codeGateLintBoardFixture("output_contains", "LINT OK"), `kinds = ["code"]`, `kinds = ["code", "human"]`, 1)
	writeFixture(t, store.BoardPath("session-search"), raw)
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	engine := NewRunEngine(store, personas, &fakeRunExecutor{outputs: map[string]string{"fmn_work": "LINT OK"}})
	engine.SetGateEvaluator(NewCodeGateEvaluator())
	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 8, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if err := store.AppendRunEvent(status.RunID, RunEvent{
		Type: RunEventBlocked,
		Data: map[string]any{
			"reason":        "crash recovery",
			"resumeAllowed": true,
			"resumePolicy":  "explicit",
		},
	}); err != nil {
		t.Fatalf("append recovery block: %v", err)
	}
	ledger := findOnlyRunLedger(t, store, "session-search")
	events := readRunEvents(t, ledger)
	codeResult := eventOfType(t, events, RunEventGateKindResult)
	resultHash := stringFromAny(codeResult.Data["resultSha256"])
	ledgerRaw, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	tampered := strings.Replace(string(ledgerRaw), resultHash, strings.Repeat("0", 64), 1)
	if err := os.WriteFile(ledger, []byte(tampered), 0o600); err != nil {
		t.Fatalf("tamper ledger: %v", err)
	}

	before := readRunEvents(t, ledger)
	_, err = engine.ResumeRun(status.RunID, RunResumeRequest{Actor: "agent:test", Reason: "recover"})
	if !errors.Is(err, ErrRunLedgerInvalid) {
		t.Fatalf("resume error = %v, want ErrRunLedgerInvalid", err)
	}
	after := readRunEvents(t, ledger)
	if len(after) != len(before) {
		t.Fatalf("tampered result rejection appended events: before=%d after=%d", len(before), len(after))
	}
}

type countingCodeGateEvaluator struct {
	calls int
}

func (e *countingCodeGateEvaluator) EvaluateGate(req GateEvaluation) (GateEvaluationResult, error) {
	e.calls++
	return NewCodeGateEvaluator().EvaluateGate(req)
}

func TestCodeGateProfileAdmissionRejectsProcessAndEffectfulDescriptors(t *testing.T) {
	descriptor, ok := LookupCodeGateProfileDescriptor("output_contains", "1")
	if !ok {
		t.Fatal("missing output_contains@1 descriptor")
	}
	tests := []struct {
		name   string
		mutate func(*CodeGateProfileDescriptor)
	}{
		{name: "process", mutate: func(d *CodeGateProfileDescriptor) { d.ExecutionClass = "process" }},
		{name: "effectful", mutate: func(d *CodeGateProfileDescriptor) { d.EffectPolicy = "network" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			altered := descriptor
			test.mutate(&altered)
			if err := validateCodeGateProfileDescriptor(altered); err == nil {
				t.Fatalf("admission accepted %s profile descriptor", test.name)
			}
		})
	}
}

func TestCodeGateProfileIdentityChangesWithEvaluatorOperation(t *testing.T) {
	contains := newCodeGateProfileDefinition(
		"test_profile",
		"1",
		"Test profile",
		"Value",
		codeGateOperationContains,
	)
	absent := newCodeGateProfileDefinition(
		"test_profile",
		"1",
		"Test profile",
		"Value",
		codeGateOperationAbsent,
	)
	if contains.descriptor.EvaluatorBundleSHA256 == absent.descriptor.EvaluatorBundleSHA256 {
		t.Fatal("evaluator operation revision did not change the frozen bundle identity")
	}
	if contains.descriptor.ProfileSHA256 == absent.descriptor.ProfileSHA256 {
		t.Fatal("evaluator operation revision did not change the frozen profile identity")
	}
}

func TestCodeGateEvaluatorImplementationDigestMatchesSource(t *testing.T) {
	raw, err := os.ReadFile("code_gate.go")
	if err != nil {
		t.Fatalf("read code_gate.go: %v", err)
	}
	const (
		startMarker = "// BEGIN CODE GATE EVALUATOR BUNDLE"
		endMarker   = "// END CODE GATE EVALUATOR BUNDLE"
	)
	start := bytes.Index(raw, []byte(startMarker))
	end := bytes.Index(raw, []byte(endMarker))
	if start == -1 || end == -1 || end < start {
		t.Fatalf("evaluator bundle markers are missing or out of order")
	}
	end += len(endMarker)
	if got := codeGateSHA256(string(raw[start:end])); got != codeGateEvaluatorImplementationSHA256 {
		t.Fatalf("evaluator implementation digest = %s, want frozen %s; semantic changes require a new bundle identity", got, codeGateEvaluatorImplementationSHA256)
	}
}

func ptrRunGateBinding(binding RunGateBinding) *RunGateBinding {
	return &binding
}

// codeGateLintBoardFixture wires mission -> work -> machine gate, with the gate
// pass route to ship and the fail route pushed back to work. The gate declares
// an explicit, operator-authored machine check (never the free-text criterion).
func codeGateLintBoardFixture(check, checkValue string) string {
	return s4MissionOnlyBoardFixture() + `
[[formation]]
id = "fmn_work"
type = "solo"
title = "Work"

[[formation.input]]
id = "port_work_in"
label = "Input"

[[formation.output]]
id = "port_work_out"
label = "Output"

[[formation.slot]]
id = "slot_work"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[gate]]
id = "gate_lint"
title = "Lint"
kinds = ["code"]
criterion = "touch should-not-run"
check = "` + check + `"
checkVersion = "1"
checkValue = "` + checkValue + `"

[[formation]]
id = "fmn_ship"
type = "solo"
title = "Ship"

[[formation.input]]
id = "port_ship_in"
label = "Input"

[[formation.output]]
id = "port_ship_out"
label = "Output"

[[formation.slot]]
id = "slot_ship"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true

[[connection]]
id = "edge_mission_work"
from = "mis_showcase:out"
to = "fmn_work:port_work_in"

[[connection]]
id = "edge_work_gate"
from = "fmn_work:port_work_out"
to = "gate_lint:in"

[[connection]]
id = "edge_gate_pass_ship"
from = "gate_lint:pass"
to = "fmn_ship:port_ship_in"

[[connection]]
id = "edge_gate_fail_work"
from = "gate_lint:fail"
to = "fmn_work:port_work_in"
`
}

// attemptOutputExecutor returns different node output per attempt so a machine
// gate can be exercised through a real fail->revise->pass loop deterministically
// without any agent process.
type attemptOutputExecutor struct {
	calls         []FormationExecution
	textByAttempt map[string]map[int]string
}

func (e *attemptOutputExecutor) ExecuteFormation(req FormationExecution) (FormationExecutionResult, error) {
	e.calls = append(e.calls, req)
	text := "output from " + req.NodeID
	if byAttempt, ok := e.textByAttempt[req.NodeID]; ok {
		if t, ok := byAttempt[req.Attempt]; ok {
			text = t
		}
	}
	return FormationExecutionResult{
		Status:    "done",
		ReportRef: "refs/" + req.NodeID + ".md",
		Text:      text,
		Outputs:   payloadsForFormationOutputs(req.Formation, text, "refs/"+req.NodeID+".md"),
	}, nil
}

func (e *attemptOutputExecutor) nodeIDs() []string {
	ids := make([]string, 0, len(e.calls))
	for _, call := range e.calls {
		ids = append(ids, call.NodeID)
	}
	return ids
}

// A machine (code) gate loops fail->revise->pass deterministically against an
// explicit output check, with no human and no agent process.
func TestCodeGateEvaluatorLoopsUntilLintPasses(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), codeGateLintBoardFixture("output_contains", "LINT OK"))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &attemptOutputExecutor{textByAttempt: map[string]map[int]string{
		"fmn_work": {
			1: "lint: 3 problems (3 errors, 0 warnings)",
			2: "lint clean — LINT OK",
		},
	}}
	engine := NewRunEngine(store, personas, executor)
	engine.SetGateEvaluator(NewCodeGateEvaluator())

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
	if status.Status != RunStatusSucceeded {
		t.Fatalf("status = %+v, want succeeded after lint passes", status)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_work", "fmn_work", "fmn_ship"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("executor nodes = %v, want two work attempts then ship", got)
	}
	shipInput := executor.calls[2].Inputs[0]
	if shipInput.FromNodeID != "fmn_work" ||
		shipInput.FromPortID != "port_work_out" ||
		shipInput.EdgeID != "edge_work_gate" ||
		shipInput.Text != "lint clean — LINT OK" {
		t.Fatalf("pass route input = %+v, want exact selected work output and source provenance", shipInput)
	}

	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	verdicts := gateVerdicts(events, "gate_lint")
	if got, want := verdicts, []string{"fail", "pass"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("gate verdicts = %v, want fail then pass", got)
	}
	first := firstGateVerdictEvent(t, events, "gate_lint")
	if reason, _ := first.Data["reason"].(string); !strings.Contains(reason, "output_contains") || !strings.Contains(reason, "LINT OK") {
		t.Fatalf("fail verdict reason = %q, want evidence citing the check and value", first.Data["reason"])
	}
}

type panicGateEvaluator struct{}

func (panicGateEvaluator) EvaluateGate(GateEvaluation) (GateEvaluationResult, error) {
	panic("broken evaluator")
}

func TestGateEvaluatorPanicBecomesErrorWithoutVerdictOrRoute(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), codeGateLintBoardFixture("output_contains", "LINT OK"))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	engine := NewRunEngine(store, personas, &fakeRunExecutor{})
	engine.SetGateEvaluator(panicGateEvaluator{})

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 8, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusBlocked {
		t.Fatalf("status = %+v, want blocked after evaluator panic", status)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	errEvent := eventOfType(t, events, RunEventError)
	if errEvent.Data["code"] != "gate_evaluator_error" {
		t.Fatalf("error code = %#v, want gate_evaluator_error", errEvent.Data["code"])
	}
	for _, event := range events {
		if event.Type == RunEventGateVerdict {
			t.Fatalf("panic emitted Gate verdict or route: %+v", event)
		}
	}
}

func TestCodeGateExhaustionBecomesErrorWithoutVerdictOrRoute(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), codeGateLintBoardFixture("output_contains", "LINT OK"))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	descriptor, ok := LookupCodeGateProfileDescriptor("output_contains", "1")
	if !ok {
		t.Fatal("missing output_contains@1 descriptor")
	}
	executor := &attemptOutputExecutor{textByAttempt: map[string]map[int]string{
		"fmn_work": {1: strings.Repeat("x", descriptor.MaxInputBytes+1)},
	}}
	engine := NewRunEngine(store, personas, executor)
	engine.SetGateEvaluator(NewCodeGateEvaluator())

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 8, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusBlocked {
		t.Fatalf("status = %+v, want blocked after evaluator exhaustion", status)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	errEvent := eventOfType(t, events, RunEventError)
	if errEvent.Data["code"] != "gate_evaluator_error" {
		t.Fatalf("error code = %#v, want gate_evaluator_error", errEvent.Data["code"])
	}
	for _, event := range events {
		if event.Type == RunEventGateVerdict {
			t.Fatalf("exhaustion emitted Gate verdict or route: %+v", event)
		}
	}
	if got := executor.nodeIDs(); !reflect.DeepEqual(got, []string{"fmn_work"}) {
		t.Fatalf("executor nodes = %v, want no downstream dispatch after exhaustion", got)
	}
}

// A selected code Gate without an exact profile tuple must be rejected before
// run admission so upstream work cannot execute under an ambiguous evaluator.
func TestCodeGateAdmissionRequiresExplicitProfileTuple(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	// No check/checkValue declared: only the free-text criterion is present.
	writeFixture(t, store.BoardPath("session-search"), strings.Replace(
		codeGateLintBoardFixture("output_contains", "LINT OK"),
		"check = \"output_contains\"\ncheckVersion = \"1\"\ncheckValue = \"LINT OK\"\n",
		"",
		1,
	))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{}
	evaluator := &countingGateEvaluator{}
	engine := NewRunEngine(store, personas, executor)
	engine.SetGateEvaluator(evaluator)

	_, err = engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 8, MaxAttempts: 2},
	})
	if err == nil || !strings.Contains(err.Error(), FindingInvalidCodeGateProfile) {
		t.Fatalf("run mission error = %v, want %s", err, FindingInvalidCodeGateProfile)
	}
	if len(executor.calls) != 0 || evaluator.calls != 0 {
		t.Fatalf("rejected start effects = executor:%d evaluator:%d, want zero", len(executor.calls), evaluator.calls)
	}
	if runs := mustGlob(t, filepath.Join(store.Workspace, ".formations", "runs", "*")); len(runs) != 0 {
		t.Fatalf("missing profile tuple created run artifacts: %v", runs)
	}
}

// An unknown check profile is rejected before run admission, not evaluated or
// silently passed.
func TestCodeGateEvaluatorUnknownProfileBlocks(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), codeGateLintBoardFixture("no_such_profile", "LINT OK"))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	engine := NewRunEngine(store, personas, &fakeRunExecutor{})
	engine.SetGateEvaluator(NewCodeGateEvaluator())

	_, err = engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 8, MaxAttempts: 2},
	})
	if err == nil || !strings.Contains(err.Error(), FindingInvalidCodeGateProfile) {
		t.Fatalf("run mission error = %v, want %s", err, FindingInvalidCodeGateProfile)
	}
	if runs := mustGlob(t, filepath.Join(store.Workspace, ".formations", "runs", "*")); len(runs) != 0 {
		t.Fatalf("unknown profile created run artifacts: %v", runs)
	}
}

// The output_absent profile passes only when a forbidden token is absent.
func TestCodeGateEvaluatorOutputAbsentProfile(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), codeGateLintBoardFixture("output_absent", "error"))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &attemptOutputExecutor{textByAttempt: map[string]map[int]string{
		"fmn_work": {
			1: "test run: 1 error remaining",
			2: "test run: all green",
		},
	}}
	engine := NewRunEngine(store, personas, executor)
	engine.SetGateEvaluator(NewCodeGateEvaluator())

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
	if status.Status != RunStatusSucceeded {
		t.Fatalf("status = %+v, want succeeded once the forbidden token is absent", status)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	if got, want := gateVerdicts(events, "gate_lint"), []string{"fail", "pass"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("gate verdicts = %v, want fail then pass", got)
	}
}

func gateVerdicts(events []RunEvent, gateID string) []string {
	var verdicts []string
	for _, event := range events {
		if event.Type == RunEventGateVerdict && event.GateID == gateID {
			if v, ok := event.Data["verdict"].(string); ok {
				verdicts = append(verdicts, v)
			}
		}
	}
	return verdicts
}

func firstGateVerdictEvent(t *testing.T, events []RunEvent, gateID string) RunEvent {
	t.Helper()
	for _, event := range events {
		if event.Type == RunEventGateVerdict && event.GateID == gateID {
			return event
		}
	}
	t.Fatalf("no gate verdict for %s", gateID)
	return RunEvent{}
}
