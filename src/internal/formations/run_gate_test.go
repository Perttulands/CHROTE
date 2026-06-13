package formations

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestS4GateRoutesPassAndUnwiredFailBlocks(t *testing.T) {
	t.Run("pass routes through pass wire", func(t *testing.T) {
		store, personas := s4RunFixture(t)
		store.Now = fixedClock()
		personas.Now = fixedClock()
		createS4Persona(t, personas, "scout")
		writeFixture(t, store.BoardPath("session-search"), s4GateBoardFixture(false))
		board, err := store.ReadBoard("session-search")
		if err != nil {
			t.Fatalf("read board: %v", err)
		}
		executor := &fakeRunExecutor{}
		engine := NewRunEngine(store, personas, executor)
		engine.SetGateEvaluator(&fakeGateEvaluator{verdicts: []string{"pass"}})

		status, err := engine.RunMission("session-search", RunStartRequest{
			MissionID:         "mis_showcase",
			Actor:             "agent:test",
			ExpectedBoardETag: board.ETag,
			ExpectedBoardRev:  board.Rev,
			Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 2},
		})
		if err != nil {
			t.Fatalf("run mission: %v", err)
		}
		if status.Status != RunStatusSucceeded {
			t.Fatalf("status = %+v, want succeeded", status)
		}
		if got, want := executor.nodeIDs(), []string{"fmn_work", "fmn_ship"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("executor nodes = %v, want %v", got, want)
		}
		events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
		verdict := eventOfType(t, events, RunEventGateVerdict)
		if verdict.GateID != "gate_review" || verdict.Data["verdict"] != "pass" || verdict.Data["routePort"] != "pass" {
			t.Fatalf("gate verdict = %+v, want pass through pass route", verdict)
		}
	})

	t.Run("unwired fail records run_blocked", func(t *testing.T) {
		store, personas := s4RunFixture(t)
		store.Now = fixedClock()
		personas.Now = fixedClock()
		createS4Persona(t, personas, "scout")
		writeFixture(t, store.BoardPath("session-search"), s4GateBoardFixture(false))
		board, err := store.ReadBoard("session-search")
		if err != nil {
			t.Fatalf("read board: %v", err)
		}
		executor := &fakeRunExecutor{}
		engine := NewRunEngine(store, personas, executor)
		engine.SetGateEvaluator(&fakeGateEvaluator{verdicts: []string{"fail"}})

		status, err := engine.RunMission("session-search", RunStartRequest{
			MissionID:         "mis_showcase",
			Actor:             "agent:test",
			ExpectedBoardETag: board.ETag,
			ExpectedBoardRev:  board.Rev,
			Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 2},
		})
		if err != nil {
			t.Fatalf("run mission: %v", err)
		}
		if status.Status != RunStatusBlocked || status.Final {
			t.Fatalf("status = %+v, want blocked non-final", status)
		}
		if got, want := executor.nodeIDs(), []string{"fmn_work"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("executor nodes = %v, want only pre-gate work", got)
		}
		events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
		if events[len(events)-1].Type != RunEventBlocked {
			t.Fatalf("last event = %s, want run_blocked", events[len(events)-1].Type)
		}
		verdict := eventOfType(t, events, RunEventGateVerdict)
		if verdict.Data["verdict"] != "fail" || verdict.Data["routePort"] != "none" {
			t.Fatalf("gate verdict = %+v, want unwired fail route none", verdict)
		}
	})
}

func TestS4GateFailWirePushesBackWithAttemptLimit(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4GateBoardFixture(true))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)
	engine.SetGateEvaluator(&fakeGateEvaluator{verdicts: []string{"fail", "fail"}})

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusBlocked {
		t.Fatalf("status = %+v, want blocked after revise exhaustion", status)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_work", "fmn_work"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("executor nodes = %v, want two work attempts", got)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	attempts := nodeStartedAttempts(events, "fmn_work")
	if !reflect.DeepEqual(attempts, []int{1, 2}) {
		t.Fatalf("work attempts = %v, want [1 2]", attempts)
	}
	errEvent := eventOfType(t, events, RunEventError)
	if errEvent.Data["reason"] != "revise loop exhausted" {
		t.Fatalf("error data = %#v, want revise loop exhausted", errEvent.Data)
	}
}

func TestS4ScriptGateExplicitCommandRoutesPassOutput(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeExecutableScript(t, store.Workspace, "checks/pass.sh", `#!/bin/sh
printf 'script says pass\n'
`)
	writeFixture(t, store.BoardPath("session-search"), s4ScriptGateBoardFixture(`
scriptRoot = "."
scriptCwd = "."
scriptCommand = ["./checks/pass.sh"]
scriptTimeoutSeconds = 5
scriptOutputLimitBytes = 4096
`, false))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusSucceeded {
		t.Fatalf("status = %+v, want succeeded from script pass", status)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_work", "fmn_ship"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("executor nodes = %v, want %v", got, want)
	}
	shipInputs := callsByNode(executor.calls)["fmn_ship"][0].Inputs
	if got, want := shipInputs[0].Text, "script says pass"; got != want {
		t.Fatalf("ship input text = %q, want script stdout %q", got, want)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	verdict := eventOfType(t, events, RunEventGateVerdict)
	if verdict.Data["verdict"] != "pass" || verdict.Data["routePort"] != "pass" {
		t.Fatalf("gate verdict = %+v, want pass route", verdict)
	}
	script := mapFromAny(t, verdict.Data["script"], "script evidence")
	if script["status"] != "pass" || intFromAny(script["exitCode"]) != 0 {
		t.Fatalf("script evidence = %#v, want exit 0 pass", script)
	}
	outputs := mapFromAny(t, verdict.Data["outputs"], "gate outputs")
	pass := mapFromAny(t, outputs["pass"], "pass output")
	if pass["text"] != "script says pass" {
		t.Fatalf("pass output = %#v, want script stdout", pass)
	}
}

func TestS4ScriptGateNonZeroRoutesFailWithRedactedEvidence(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeExecutableScript(t, store.Workspace, "checks/fail.sh", `#!/bin/sh
printf 'token=super-secret-value\n'
exit 7
`)
	writeFixture(t, store.BoardPath("session-search"), s4ScriptGateBoardFixture(`
scriptRoot = "."
scriptCwd = "."
scriptCommand = ["./checks/fail.sh"]
scriptTimeoutSeconds = 5
scriptOutputLimitBytes = 4096
`, true))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusSucceeded {
		t.Fatalf("status = %+v, want succeeded via wired fail branch", status)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_work", "fmn_revise"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("executor nodes = %v, want %v", got, want)
	}
	reviseInputs := callsByNode(executor.calls)["fmn_revise"][0].Inputs
	if got, want := reviseInputs[0].FromPortID, "fail"; got != want {
		t.Fatalf("revise input from port = %q, want %q", got, want)
	}
	if strings.Contains(reviseInputs[0].Text, "super-secret-value") {
		t.Fatalf("fail branch input leaked secret: %q", reviseInputs[0].Text)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	verdict := eventOfType(t, events, RunEventGateVerdict)
	if verdict.Data["verdict"] != "fail" || verdict.Data["routePort"] != "fail" {
		t.Fatalf("gate verdict = %+v, want fail route", verdict)
	}
	script := mapFromAny(t, verdict.Data["script"], "script evidence")
	if script["status"] != "exit" || intFromAny(script["exitCode"]) != 7 {
		t.Fatalf("script evidence = %#v, want exit 7 fail", script)
	}
	stdout, _ := script["stdout"].(string)
	if strings.Contains(stdout, "super-secret-value") || !strings.Contains(stdout, "[REDACTED]") {
		t.Fatalf("script stdout evidence = %q, want redacted secret", stdout)
	}
}

func TestS4ScriptGateTimeoutBlocksWithDurableEvidence(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeExecutableScript(t, store.Workspace, "checks/slow.sh", `#!/bin/sh
sleep 2
printf 'late\n'
`)
	writeFixture(t, store.BoardPath("session-search"), s4ScriptGateBoardFixture(`
scriptRoot = "."
scriptCwd = "."
scriptCommand = ["./checks/slow.sh"]
scriptTimeoutSeconds = 1
scriptOutputLimitBytes = 4096
`, false))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusBlocked || status.Final {
		t.Fatalf("status = %+v, want blocked non-final from unwired timeout fail", status)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_work"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("executor nodes = %v, want only pre-gate work", got)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	verdict := eventOfType(t, events, RunEventGateVerdict)
	if verdict.Data["verdict"] != "fail" || verdict.Data["routePort"] != "none" {
		t.Fatalf("gate verdict = %+v, want timeout fail with no route", verdict)
	}
	script := mapFromAny(t, verdict.Data["script"], "script evidence")
	if script["status"] != "timeout" || script["timedOut"] != true || intFromAny(script["timeoutSeconds"]) != 1 {
		t.Fatalf("script evidence = %#v, want timeout evidence", script)
	}
	if events[len(events)-1].Type != RunEventBlocked {
		t.Fatalf("last event = %s, want run_blocked", events[len(events)-1].Type)
	}
}

func TestS4ScriptGateOutputCapLimitsLedgerAndRoutedPayload(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeExecutableScript(t, store.Workspace, "checks/chatty.sh", `#!/bin/sh
printf 'abcdefghijklmnopqrstuvwxyz\n'
`)
	writeFixture(t, store.BoardPath("session-search"), s4ScriptGateBoardFixture(`
scriptRoot = "."
scriptCwd = "."
scriptCommand = ["./checks/chatty.sh"]
scriptTimeoutSeconds = 5
scriptOutputLimitBytes = 8
`, false))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusSucceeded {
		t.Fatalf("status = %+v, want succeeded from capped script pass", status)
	}
	shipInputs := callsByNode(executor.calls)["fmn_ship"][0].Inputs
	if got, want := shipInputs[0].Text, "abcdefgh"; got != want {
		t.Fatalf("ship input text = %q, want capped stdout %q", got, want)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	verdict := eventOfType(t, events, RunEventGateVerdict)
	script := mapFromAny(t, verdict.Data["script"], "script evidence")
	if script["stdout"] != "abcdefgh" || script["stdoutTruncated"] != true {
		t.Fatalf("script evidence = %#v, want capped stdout with truncation flag", script)
	}
}

func TestS4ScriptGateCriterionIsNeverExecutedAsShell(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	pwned := filepath.Join(store.Workspace, "criterion-ran")
	criterion := "touch " + pwned
	writeFixture(t, store.BoardPath("session-search"), s4ScriptGateBoardFixtureWithCriterion(criterion, "", false))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusBlocked {
		t.Fatalf("status = %+v, want blocked because script config is missing", status)
	}
	if _, err := os.Stat(pwned); !os.IsNotExist(err) {
		t.Fatalf("criterion text was executed or file check failed: %v", err)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	errEvent := eventOfType(t, events, RunEventError)
	if errEvent.Data["code"] != "missing_script_config" {
		t.Fatalf("run error = %#v, want missing_script_config", errEvent.Data)
	}
}

func TestS4ScriptGateRejectsInlineShellCommandConfig(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	pwned := filepath.Join(store.Workspace, "inline-shell-ran")
	writeFixture(t, store.BoardPath("session-search"), s4ScriptGateBoardFixture(`
scriptRoot = "."
scriptCwd = "."
scriptCommand = ["sh", "-c", "touch `+pwned+`"]
scriptTimeoutSeconds = 5
scriptOutputLimitBytes = 4096
`, false))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusBlocked {
		t.Fatalf("status = %+v, want blocked because inline shell config is disallowed", status)
	}
	if _, err := os.Stat(pwned); !os.IsNotExist(err) {
		t.Fatalf("inline shell config was executed or file check failed: %v", err)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	errEvent := eventOfType(t, events, RunEventError)
	if errEvent.Data["code"] != "invalid_script_config" {
		t.Fatalf("run error = %#v, want invalid_script_config", errEvent.Data)
	}
}

func TestS4ScriptGateRejectsCwdOutsideConfiguredRoot(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeExecutableScript(t, store.Workspace, "checks/pass.sh", `#!/bin/sh
printf 'should not run\n'
`)
	writeFixture(t, store.BoardPath("session-search"), s4ScriptGateBoardFixture(`
scriptRoot = "checks"
scriptCwd = ".."
scriptCommand = ["./checks/pass.sh"]
scriptTimeoutSeconds = 5
scriptOutputLimitBytes = 4096
`, false))
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 2},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusBlocked {
		t.Fatalf("status = %+v, want blocked because script cwd escapes root", status)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	errEvent := eventOfType(t, events, RunEventError)
	if errEvent.Data["code"] != "invalid_script_config" || !strings.Contains(errEvent.Data["message"].(string), "scriptCwd escapes scriptRoot") {
		t.Fatalf("run error = %#v, want invalid cwd constraint evidence", errEvent.Data)
	}
}

func TestS4RunLimitsRecordAndStop(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4CascadeBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	executor := &fakeRunExecutor{}
	engine := NewRunEngine(store, personas, executor)

	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 1},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusBlocked {
		t.Fatalf("status = %+v, want blocked when max dispatch is exceeded", status)
	}
	if got, want := executor.nodeIDs(), []string{"fmn_frame"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("executor nodes = %v, want only first dispatch before limit", got)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	errEvent := eventOfType(t, events, RunEventError)
	if errEvent.Data["code"] != "max_dispatch_exceeded" {
		t.Fatalf("error data = %#v, want max_dispatch_exceeded", errEvent.Data)
	}
	if events[len(events)-1].Type != RunEventBlocked {
		t.Fatalf("last event = %s, want run_blocked", events[len(events)-1].Type)
	}
}

type fakeGateEvaluator struct {
	verdicts []string
	calls    []GateEvaluation
}

func (f *fakeGateEvaluator) EvaluateGate(req GateEvaluation) (GateEvaluationResult, error) {
	f.calls = append(f.calls, req)
	verdict := "pass"
	if len(f.verdicts) > 0 {
		verdict = f.verdicts[0]
		f.verdicts = f.verdicts[1:]
	}
	return GateEvaluationResult{Verdict: verdict, Reason: "fake " + verdict}, nil
}

func eventOfType(t *testing.T, events []RunEvent, eventType string) RunEvent {
	t.Helper()
	for _, event := range events {
		if event.Type == eventType {
			return event
		}
	}
	t.Fatalf("missing event type %s in %#v", eventType, events)
	return RunEvent{}
}

func nodeStartedAttempts(events []RunEvent, nodeID string) []int {
	var attempts []int
	for _, event := range events {
		if event.Type == RunEventNodeStarted && event.NodeID == nodeID {
			attempts = append(attempts, event.Attempt)
		}
	}
	return attempts
}

func s4GateBoardFixture(pushback bool) string {
	failWire := ""
	if pushback {
		failWire = `
[[connection]]
id = "edge_gate_fail_work"
from = "gate_review:fail"
to = "fmn_work:port_work_in"
`
	}
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
id = "gate_review"
title = "Review"
kinds = ["code"]
criterion = "Good enough to ship"

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
to = "gate_review:in"

[[connection]]
id = "edge_gate_pass_ship"
from = "gate_review:pass"
to = "fmn_ship:port_ship_in"
` + failWire
}

func s4ScriptGateBoardFixture(gateConfig string, failWire bool) string {
	return s4ScriptGateBoardFixtureWithCriterion("Run the configured script gate", gateConfig, failWire)
}

func s4ScriptGateBoardFixtureWithCriterion(criterion, gateConfig string, failWire bool) string {
	failWireTOML := ""
	failFormation := ""
	if failWire {
		failFormation = `
[[formation]]
id = "fmn_revise"
type = "solo"
title = "Revise"

[[formation.input]]
id = "port_revise_in"
label = "Input"

[[formation.output]]
id = "port_revise_out"
label = "Output"

[[formation.slot]]
id = "slot_revise"
label = "Worker"
agentId = "scout"
harness = "openai-codex"
controller = true
`
		failWireTOML = `
[[connection]]
id = "edge_gate_fail_revise"
from = "gate_review:fail"
to = "fmn_revise:port_revise_in"
`
	}
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
id = "gate_review"
title = "Script review"
kinds = ["script"]
criterion = "` + criterion + `"
` + gateConfig + `
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
` + failFormation + `
[[connection]]
id = "edge_mission_work"
from = "mis_showcase:out"
to = "fmn_work:port_work_in"

[[connection]]
id = "edge_work_gate"
from = "fmn_work:port_work_out"
to = "gate_review:in"

[[connection]]
id = "edge_gate_pass_ship"
from = "gate_review:pass"
to = "fmn_ship:port_ship_in"
` + failWireTOML
}

func writeExecutableScript(t *testing.T, workspace, rel, content string) {
	t.Helper()
	path := filepath.Join(workspace, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir script: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}
}

func mapFromAny(t *testing.T, value any, label string) map[string]any {
	t.Helper()
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want object", label, value)
	}
	return m
}

func intFromAny(value any) int {
	switch n := value.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}
