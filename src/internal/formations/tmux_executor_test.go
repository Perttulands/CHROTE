package formations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfiguredFormationExecutorFromEnvSelectsTmuxOnlyWhenHarnessesSet(t *testing.T) {
	clearExecutorEnv(t)

	executor := NewConfiguredFormationExecutorFromEnv(nil, nil, "test-boundary")
	if _, ok := executor.(unavailableFormationExecutor); !ok {
		t.Fatalf("executor without harness env = %T, want unavailableFormationExecutor", executor)
	}

	t.Setenv("CHROTE_FORMATIONS_TMUX_HARNESSES", "openai-codex")
	executor = NewConfiguredFormationExecutorFromEnv(nil, nil, "test-boundary")
	if _, ok := executor.(*TmuxFormationExecutor); !ok {
		t.Fatalf("executor with tmux harness env = %T, want *TmuxFormationExecutor", executor)
	}

	t.Setenv("CHROTE_FORMATIONS_TMUX_HARNESSES", "")
	t.Setenv("CHROTE_FORMATIONS_LAB_HARNESSES", "openai-codex")
	executor = NewConfiguredFormationExecutorFromEnv(nil, nil, "test-boundary")
	if _, ok := executor.(*TmuxFormationExecutor); ok {
		t.Fatalf("executor with only lab harness env = %T, must not select tmux", executor)
	}
	if _, ok := executor.(*LabFormationExecutor); !ok {
		t.Fatalf("executor with lab harness env = %T, want *LabFormationExecutor", executor)
	}
}

func TestTmuxExecutorRefusesLiveChroteSocketUnlessProdSmoke(t *testing.T) {
	cfg := tmuxTestConfig(t)
	cfg.Socket = "/run/user/1000/chrote-tmux/tmux-1000/default"
	cfg.ProdSmoke = false

	err := newTmuxFormationExecutorWithClient(nil, nil, cfg, &fakeTmuxHarnessClient{}).validateConfiguredBoundary()
	var executionErr *RunExecutionError
	if !errors.As(err, &executionErr) {
		t.Fatalf("live socket error = %v, want RunExecutionError", err)
	}
	if executionErr.Code != "unsafe_socket" || executionErr.Boundary != "executor" {
		t.Fatalf("live socket error = %+v, want unsafe_socket/executor", executionErr)
	}

	cfg.ProdSmoke = true
	if err := newTmuxFormationExecutorWithClient(nil, nil, cfg, &fakeTmuxHarnessClient{}).validateConfiguredBoundary(); err != nil {
		t.Fatalf("prod-smoke live socket validate error = %v, want allowed", err)
	}

	clearExecutorEnv(t)
	t.Setenv("CHROTE_FORMATIONS_TMUX_PROD_SMOKE", "prod-smoke")
	if !TmuxExecutorConfigFromEnv().ProdSmoke {
		t.Fatal("CHROTE_FORMATIONS_TMUX_PROD_SMOKE=prod-smoke did not enable prod smoke config")
	}
}

func TestTmuxExecutorSessionFailuresRecordDurableBoundaryAndProvenance(t *testing.T) {
	for _, tc := range []struct {
		name     string
		client   *fakeTmuxHarnessClient
		wantCode string
	}{
		{
			name:     "missing session",
			client:   &fakeTmuxHarnessClient{sessions: nil},
			wantCode: "missing_session",
		},
		{
			name:     "ambiguous session",
			client:   &fakeTmuxHarnessClient{sessions: []string{"tmux-scout", "tmux-scout"}},
			wantCode: "ambiguous_session",
		},
		{
			name:     "dead pane",
			client:   &fakeTmuxHarnessClient{sessions: []string{"tmux-scout"}, pane: tmuxPaneState{Dead: true}},
			wantCode: "dead_pane",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, events := runTmuxFormationForTest(t, tc.client, "")
			if status.Status != RunStatusBlocked || !status.ResumeAllowed {
				t.Fatalf("status = %+v, want resumable blocked run", status)
			}
			errEvent := eventOfType(t, events, RunEventError)
			if errEvent.NodeID != "fmn_research" || errEvent.SlotID != "slot_research" {
				t.Fatalf("error provenance envelope = %+v, want node/slot", errEvent)
			}
			if errEvent.Data["code"] != tc.wantCode || errEvent.Data["boundary"] != "adapter" {
				t.Fatalf("error data = %#v, want code %s boundary adapter", errEvent.Data, tc.wantCode)
			}
			if errEvent.Data["nodeId"] != "fmn_research" || errEvent.Data["slotId"] != "slot_research" || errEvent.Data["recoverable"] != true {
				t.Fatalf("error data provenance = %#v, want nodeId/slotId/recoverable", errEvent.Data)
			}
			blocked := eventOfType(t, events, RunEventBlocked)
			if blocked.NodeID != "fmn_research" || blocked.SlotID != "slot_research" || blocked.Data["blockedNodeId"] != "fmn_research" {
				t.Fatalf("blocked event = %+v data=%#v, want durable blocked node/slot", blocked, blocked.Data)
			}
			if tc.client.sendCalls != 0 {
				t.Fatalf("send calls = %d, want no send after unsafe session state", tc.client.sendCalls)
			}
		})
	}
}

func TestRedactPromptAndSecretTokensFromDispatchAdapterFailureLedger(t *testing.T) {
	store, started := startS4DispatchRun(t)
	prompt := "brief: RAW-PROMPT-DISPATCH-DO-NOT-LOG api_key=sk-dispatchsecret123\ninput: hidden dispatch payload"
	dispatcher := NewSlotDispatcher(store, promptEchoDispatchAdapter{})

	_, err := dispatcher.DispatchSlot(started.RunID, SlotDispatchRequest{
		NodeID:      "fmn_work",
		SlotID:      "slot_work",
		AgentID:     "scout",
		Harness:     "openai-codex",
		SessionStem: "scout",
		SessionRef:  "tmux:scout",
		Prompt:      prompt,
		Attempt:     1,
	})
	if err == nil {
		t.Fatal("dispatch adapter failure returned nil error")
	}

	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	assertLedgerDataForTypesRedacted(t, events, []string{RunEventError, RunEventBlocked},
		"RAW-PROMPT-DISPATCH-DO-NOT-LOG", "hidden dispatch payload", "sk-dispatchsecret123", "sk-dispatchadapter123")
}

func TestTmuxPromptAndSecretsRedactedWhenSendOrCaptureFails(t *testing.T) {
	goal := "RAW-PROMPT-TMUX-DO-NOT-LOG api_key=sk-tmuxsecret123"
	board := tmuxRunBoardWithBrief(goal)

	t.Run("send failure", func(t *testing.T) {
		client := &fakeTmuxHarnessClient{
			sessions:          []string{"tmux-scout"},
			sendErrEchoPrompt: true,
		}
		status, events := runTmuxFormationForTest(t, client, board)
		if status.Status != RunStatusBlocked {
			t.Fatalf("status = %+v, want blocked", status)
		}
		assertLedgerDataForTypesRedacted(t, events, []string{RunEventError, RunEventBlocked},
			"RAW-PROMPT-TMUX-DO-NOT-LOG", "sk-tmuxsecret123", "sk-tmuxsendsecret123")
		if eventsContainType(events, RunEventAdapterSend) {
			t.Fatalf("adapter_send recorded after failed send: %v", eventTypes(events))
		}
	})

	t.Run("capture failure", func(t *testing.T) {
		client := &fakeTmuxHarnessClient{
			sessions:             []string{"tmux-scout"},
			captureErrEchoPrompt: true,
		}
		status, events := runTmuxFormationForTest(t, client, board)
		if status.Status != RunStatusBlocked {
			t.Fatalf("status = %+v, want blocked", status)
		}
		assertLedgerDataForTypesRedacted(t, events, []string{RunEventAdapterSend, RunEventError, RunEventBlocked},
			"RAW-PROMPT-TMUX-DO-NOT-LOG", "sk-tmuxsecret123", "sk-tmuxcapturesecret123")
		adapterSend := eventOfType(t, events, RunEventAdapterSend)
		if adapterSend.Data["promptSha256"] == "" || adapterSend.Data["sent"] != true {
			t.Fatalf("adapter_send data = %#v, want hash-only sent record", adapterSend.Data)
		}
	})
}

func TestTmuxAdapterHappyPathRecordsSendCompletionAndOutput(t *testing.T) {
	client := &fakeTmuxHarnessClient{
		sessions: []string{"tmux-scout"},
		artifact: "reports/tmux-happy.md",
	}
	status, events := runTmuxFormationForTest(t, client, "")
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("status = %+v, want succeeded final", status)
	}
	for _, eventType := range []string{RunEventSlotDispatch, RunEventAdapterSend, RunEventSlotResult, RunEventNodeOutput} {
		if !eventsContainType(events, eventType) {
			t.Fatalf("events = %v, want %s", eventTypes(events), eventType)
		}
	}
	if client.sendCalls != 1 || client.captureCalls != 2 {
		t.Fatalf("fake client calls send=%d capture=%d, want one send and two captures (preflight + completion)", client.sendCalls, client.captureCalls)
	}
	adapterSend := eventOfType(t, events, RunEventAdapterSend)
	if adapterSend.Data["adapter"] != "tmux" || adapterSend.Data["sessionRef"] != "tmux:tmux-scout" || adapterSend.Data["promptSha256"] == "" {
		t.Fatalf("adapter_send data = %#v, want tmux session and prompt hash", adapterSend.Data)
	}
	slotResult := eventOfType(t, events, RunEventSlotResult)
	sentinel, ok := slotResult.Data["sentinel"].(map[string]any)
	if !ok {
		t.Fatalf("slot_result sentinel = %#v, want object", slotResult.Data["sentinel"])
	}
	if sentinel["artifact"] != "reports/tmux-happy.md" || slotResult.Data["status"] != "ok" {
		t.Fatalf("slot_result data = %#v, want parsed completion sentinel", slotResult.Data)
	}
	output := eventOfType(t, events, RunEventNodeOutput)
	if !strings.Contains(fmt.Sprint(output.Data["text"]), "agent output") {
		t.Fatalf("node_output = %#v, want captured agent output text", output.Data)
	}
}

func TestTmuxExecutorParsesNamedOutputPayloadBlockForPortRouting(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4NamedOutputBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	cfg := tmuxTestConfig(t)
	client := &fakeTmuxHarnessClient{
		sessions: []string{"tmux-scout"},
		pane:     tmuxPaneState{CurrentPath: cfg.Cwd},
		captures: []string{
			"splitter produced two routed outputs\n```chrote-outputs\n{\"port_split_left\":{\"text\":\"LEFT-FROM-TMUX\"},\"port_split_right\":{\"text\":\"RIGHT-FROM-TMUX\"}}\n```\n<<<CHROTE-DONE run-id=run_missing status=ok artifact=split.md>>>",
			"left consumer saw LEFT-FROM-TMUX\n<<<CHROTE-DONE run-id=run_missing status=ok artifact=left.md>>>",
			"right consumer saw RIGHT-FROM-TMUX\n<<<CHROTE-DONE run-id=run_missing status=ok artifact=right.md>>>",
		},
	}
	executor := newTmuxFormationExecutorWithClient(store, personas, cfg, client)
	engine := NewRunEngine(store, personas, executor)
	status, err := engine.RunMission("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Limits:            RunLimits{MaxDispatch: 5, MaxAttempts: 1},
	})
	if err != nil {
		t.Fatalf("run mission: %v", err)
	}
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("status = %+v, want succeeded final", status)
	}
	if len(client.sentPrompts) == 0 || !strings.Contains(client.sentPrompts[0], "```chrote-outputs") {
		t.Fatalf("split prompt missing chrote-outputs contract:\n%s", client.sentPrompts[0])
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	splitOutput := findNodeOutputEvent(t, events, "fmn_split")
	outputs, ok := splitOutput.Data["outputs"].(map[string]any)
	if !ok {
		t.Fatalf("split output payloads = %#v, want map", splitOutput.Data["outputs"])
	}
	assertOutputPayloadText(t, outputs, "port_split_left", "LEFT-FROM-TMUX")
	assertOutputPayloadText(t, outputs, "port_split_right", "RIGHT-FROM-TMUX")
	if got, want := firstStartedInputText(t, events, "fmn_left"), "LEFT-FROM-TMUX"; got != want {
		t.Fatalf("left routed input = %q, want %q", got, want)
	}
	if got, want := firstStartedInputText(t, events, "fmn_right"), "RIGHT-FROM-TMUX"; got != want {
		t.Fatalf("right routed input = %q, want %q", got, want)
	}
}

func TestTmuxPeerFormationUsesSharedPlaneAndFacilitatorSynthesis(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	for _, id := range []string{"peer-a", "peer-b"} {
		createS4Persona(t, personas, id)
	}
	writeFixture(t, store.BoardPath("session-search"), tmuxPeerBoardFixture())
	cfg := tmuxTestConfig(t)
	client := &fakeTmuxHarnessClient{
		sessions: []string{"tmux-peer-a", "tmux-peer-b"},
		pane:     tmuxPaneState{CurrentPath: cfg.Cwd},
		captures: []string{
			"PEER-A-BOUNDARY: first peer contribution about runtime boundary\n<<<CHROTE-DONE run-id=run_missing status=ok artifact=peer-a.md>>>",
			"PEER-B-EVIDENCE: second peer read PEER-A-BOUNDARY and added evidence\n<<<CHROTE-DONE run-id=run_missing status=ok artifact=peer-b.md>>>",
			"PEER-COLLABORATED: synthesis uses PEER-A-BOUNDARY and PEER-B-EVIDENCE from the shared plane\n<<<CHROTE-DONE run-id=run_missing status=ok artifact=peer-final.md>>>",
		},
	}
	executor := newTmuxFormationExecutorWithClient(store, personas, cfg, client)
	engine := NewRunEngine(store, personas, executor)
	status, err := engine.RunFormation("session-search", "fmn_peer", FormationRunRequest{
		Actor:  "agent:test",
		Limits: RunLimits{MaxDispatch: 5, MaxAttempts: 1},
	})
	if err != nil {
		t.Fatalf("run peer formation: %v", err)
	}
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("status = %+v, want succeeded final", status)
	}
	if got, want := client.sendTargets, []string{"tmux-peer-a", "tmux-peer-b", "tmux-peer-a"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("send targets = %v, want peer turn/turn/facilitator %v", got, want)
	}
	if len(client.sentPrompts) != 3 {
		t.Fatalf("sent prompts = %d, want 3 peer prompts", len(client.sentPrompts))
	}
	for i, prompt := range client.sentPrompts {
		for _, want := range []string{"shared peer plane:", "Read the shared peer plane before deciding your next move", "tmux -S " + cfg.Socket + " capture-pane"} {
			if !strings.Contains(prompt, want) {
				t.Fatalf("prompt %d missing %q:\n%s", i, want, prompt)
			}
		}
	}
	if !strings.Contains(client.sentPrompts[2], "peer phase: facilitator") || !strings.Contains(client.sentPrompts[2], "temporary facilitator") {
		t.Fatalf("facilitator prompt missing facilitator instructions:\n%s", client.sentPrompts[2])
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	planeEvent := eventOfType(t, events, "peer_plane")
	planeRel, ok := planeEvent.Data["path"].(string)
	if !ok || planeRel == "" {
		t.Fatalf("peer_plane event data = %#v, want path", planeEvent.Data)
	}
	planeRaw, err := os.ReadFile(filepath.Join(store.Workspace, filepath.FromSlash(planeRel)))
	if err != nil {
		t.Fatalf("read shared peer plane %q: %v", planeRel, err)
	}
	plane := string(planeRaw)
	for _, want := range []string{"# Peer Plane", "peer-a", "peer-b", "PEER-A-BOUNDARY", "PEER-B-EVIDENCE", "PEER-COLLABORATED"} {
		if !strings.Contains(plane, want) {
			t.Fatalf("shared peer plane missing %q:\n%s", want, plane)
		}
	}
	var phases []string
	for _, event := range events {
		if event.Type == RunEventSlotDispatch {
			phases = append(phases, fmt.Sprint(event.Data["phase"]))
		}
	}
	if got, want := phases, []string{"peer-turn", "peer-turn", "peer-facilitator"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("dispatch phases = %v, want %v", got, want)
	}
	output := eventOfType(t, events, RunEventNodeOutput)
	text := fmt.Sprint(output.Data["text"])
	if !strings.Contains(text, "PEER-COLLABORATED") || !strings.Contains(text, "PEER-A-BOUNDARY") || !strings.Contains(text, "PEER-B-EVIDENCE") {
		t.Fatalf("node_output = %#v, want peer synthesis grounded in both contributions", output.Data)
	}
	if strings.Contains(text, "tmux harness completed") {
		t.Fatalf("node_output = %#v, want synthesis not independent harness summaries", output.Data)
	}
}

func TestTmuxOrchestratedFormationGivesLeaderToolPacketWithoutPreDispatchingWorkers(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	for _, id := range []string{"lead", "worker-a", "worker-b"} {
		createS4Persona(t, personas, id)
	}
	writeFixture(t, store.BoardPath("session-search"), tmuxOrchestratedBoardFixture())
	cfg := tmuxTestConfig(t)
	client := &fakeTmuxHarnessClient{
		sessions: []string{"tmux-lead", "tmux-worker-a", "tmux-worker-b"},
		pane:     tmuxPaneState{CurrentPath: cfg.Cwd},
		captures: []string{
			"FINAL-SYNTHESIS: leader used tmux to prompt worker-a and inspect worker-b before finishing\n<<<CHROTE-DONE run-id=run_missing status=ok artifact=final.md>>>",
		},
	}
	executor := newTmuxFormationExecutorWithClient(store, personas, cfg, client)
	engine := NewRunEngine(store, personas, executor)
	status, err := engine.RunFormation("session-search", "fmn_orch", FormationRunRequest{
		Actor:  "agent:test",
		Limits: RunLimits{MaxDispatch: 6, MaxAttempts: 1},
	})
	if err != nil {
		t.Fatalf("run orchestrated formation: %v", err)
	}
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("status = %+v, want succeeded final", status)
	}
	if got, want := client.sendTargets, []string{"tmux-lead"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("send targets = %v, want only leader dispatch %v", got, want)
	}
	if len(client.sentPrompts) != 1 {
		t.Fatalf("sent prompts = %d, want 1 leader prompt", len(client.sentPrompts))
	}
	leaderPrompt := client.sentPrompts[0]
	for _, want := range []string{
		"orchestration phase: leader-agentic",
		"formation team packet:",
		"tmux socket: " + cfg.Socket,
		"- slot slot_worker_a label=\"Worker A\" agent=\"worker-a\" harness=\"openai-codex\" session=\"tmux-worker-a\"",
		"tmux -S " + cfg.Socket + " capture-pane -t tmux-worker-a -p -S -120",
		"tmux -S " + cfg.Socket + " send-keys -t tmux-worker-a C-u",
		"Use native tmux/shell tools to steer the worker sessions yourself",
	} {
		if !strings.Contains(leaderPrompt, want) {
			t.Fatalf("leader prompt missing %q:\n%s", want, leaderPrompt)
		}
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	var phases []string
	var teamEvents int
	for _, event := range events {
		if event.Type == RunEventSlotDispatch {
			phases = append(phases, fmt.Sprint(event.Data["phase"]))
		}
		if event.Type == RunEventOrchestrationTeam {
			teamEvents++
		}
	}
	if got, want := phases, []string{"leader-agentic"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("dispatch phases = %v, want %v", got, want)
	}
	if teamEvents != 1 {
		t.Fatalf("orchestration team events = %d, want 1", teamEvents)
	}
	output := eventOfType(t, events, RunEventNodeOutput)
	if !strings.Contains(fmt.Sprint(output.Data["text"]), "FINAL-SYNTHESIS") {
		t.Fatalf("node_output = %#v, want leader final output", output.Data)
	}
}

func TestWaitForCompletionAcceptsSentinelAfterTurnMarkerWhenPriorSentinelScrolledOut(t *testing.T) {
	prompt := strings.Join([]string{
		"run: run_marker",
		"slot: slot_peer_a",
		"turn marker: turn_current",
	}, "\n")
	captured := strings.Join([]string{
		"turn marker: turn_current",
		"final answer",
		"<<<CHROTE-DONE run-id=run_marker status=ok artifact=final.md>>>",
	}, "\n")
	if !hasNewCompletionSentinel(captured, prompt, "run_marker", 1) {
		t.Fatal("new sentinel after current turn marker was rejected when prior sentinel count had scrolled out")
	}

	stale := strings.Join([]string{
		"turn marker: turn_old",
		"old answer",
		"<<<CHROTE-DONE run-id=run_marker status=ok artifact=old.md>>>",
	}, "\n")
	if hasNewCompletionSentinel(stale, prompt, "run_marker", 1) {
		t.Fatal("stale sentinel before/missing current turn marker was accepted")
	}
}

func TestExtractCapturedSlotTextRemovesWrappedPromptEcho(t *testing.T) {
	captured := strings.Join([]string{
		"run: run_test",
		"node: fmn_orch",
		"slot: slot_lead",
		"orchestration phase: controller-synthesis",
		"brief: hidden prompt body",
		"When complete, emit exactly one sentinel line using the run value above: <<<CHROTE-DONE run-id=<the-run-value-above> status=ok artifact=<pat",
		"h-or-ref>>>",
		"FINAL-SYNTHESIS: safe final answer",
		"<<<CHROTE-DONE run-id=run_test status=ok artifact=artifacts/final.md>>>",
	}, "\n")

	text := extractCapturedSlotText(captured, "", "run_test")
	if text != "FINAL-SYNTHESIS: safe final answer" {
		t.Fatalf("extracted slot text = %q, want only final answer without prompt echo", text)
	}
}

func TestExtractCapturedSlotTextKeepsFinalTurnAfterTranscriptNoise(t *testing.T) {
	captured := strings.Join([]string{
		"controller plan:",
		"• Worker A: identify boundary",
		"When complete, emit exactly one sentinel line using the run value above: <<<CHROTE-DONE run-id=<the-run-value-above> status=ok artifact=<path-or-ref>>>",
		"worker outputs:",
		"• API/runtime boundary: Archon/Formations to tmux Codex.",
		"────────────────────────────────────────────────────────────────────────────────────────────────",
		"",
		"• Runtime-mediated orchestrated control works at the smoke level.",
		"  Worker dispatch and final synthesis returned through Codex sentinels.",
		"<<<CHROTE-DONE run-id=run_final status=ok artifact=final-synthesis>>>",
	}, "\n")

	text := extractCapturedSlotText(captured, "", "run_final")
	want := "• Runtime-mediated orchestrated control works at the smoke level.\n  Worker dispatch and final synthesis returned through Codex sentinels."
	if text != want {
		t.Fatalf("extracted slot text = %q, want %q", text, want)
	}
}

func TestTmuxRenderedPromptDoesNotContainParseableActualRunSentinel(t *testing.T) {
	runID := "run_prompt_echo_regression"
	executor := &TmuxFormationExecutor{config: TmuxExecutorConfig{Cwd: "/tmp/chrote-test"}}
	prompt := executor.renderPrompt(FormationExecution{
		RunID:  runID,
		NodeID: "fmn_research",
		Brief:  FormationBrief{Goal: "verify prompt echo cannot complete dispatch"},
		Inputs: []RunInputRef{{Text: "context"}},
	}, FormationSlot{ID: "slot_research"}, PersonaCard{ID: "scout"}, HarnessVariant{ID: "openai-codex"})

	if !strings.Contains(prompt, "run: "+runID+"\n") {
		t.Fatalf("rendered prompt = %q, want run line with actual run id", prompt)
	}
	if forbidden := "<<<CHROTE-DONE run-id=" + runID; strings.Contains(prompt, forbidden) {
		t.Fatalf("rendered prompt contains parseable actual-run sentinel prefix %q: %q", forbidden, prompt)
	}
	if sentinel, ok := ParseCompletionSentinel(prompt, runID); ok {
		t.Fatalf("prompt echo parsed as completion sentinel: %+v", sentinel)
	}

	actual := fmt.Sprintf("agent output\n<<<CHROTE-DONE run-id=%s status=ok artifact=reports/actual.md>>>\n", runID)
	sentinel, ok := ParseCompletionSentinel(actual, runID)
	if !ok || sentinel.RunID != runID || sentinel.Status != "ok" || sentinel.Artifact != "reports/actual.md" {
		t.Fatalf("actual emitted sentinel parsed = %+v ok=%v, want matching completion", sentinel, ok)
	}
}

func TestRealTmuxHarnessClientSendPromptClearsPaneAndPacesUppercaseEnterAndControlM(t *testing.T) {
	fakeDir := t.TempDir()
	logPath := filepath.Join(fakeDir, "tmux.log")
	fakeTmuxPath := filepath.Join(fakeDir, "tmux")
	fakeTmux := `#!/usr/bin/env bash
set -euo pipefail
stdin="$(cat)"
{
  printf 'ARGS'
  for arg in "$@"; do
    printf '	%s' "$arg"
  done
  printf '\n'
  if [ -n "$stdin" ]; then
    printf 'STDIN_BEGIN\n%s\nSTDIN_END\n' "$stdin"
  fi
} >> "${TMUX_FAKE_LOG:?}"
`
	if err := os.WriteFile(fakeTmuxPath, []byte(fakeTmux), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("TMUX_FAKE_LOG", logPath)
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	oldDelay := tmuxSubmitDelay
	oldSleep := tmuxSleep
	t.Cleanup(func() {
		tmuxSubmitDelay = oldDelay
		tmuxSleep = oldSleep
	})
	tmuxSubmitDelay = 37 * time.Millisecond
	type sleepSnapshot struct {
		Delay        time.Duration
		CommandCount int
	}
	var sleeps []sleepSnapshot
	tmuxSleep = func(d time.Duration) {
		sleeps = append(sleeps, sleepSnapshot{Delay: d, CommandCount: countFakeTmuxCommands(t, logPath)})
	}

	ctx := context.Background()
	socket := "/tmp/chrote-test-tmux.sock"
	target := "tmux-scout"
	if err := (realTmuxHarnessClient{}).SendPrompt(ctx, socket, target, "dispatch-123", "run: test\n"); err != nil {
		t.Fatalf("send prompt with fake tmux: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake tmux log: %v", err)
	}
	var got [][]string
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(line, "ARGS	") {
			continue
		}
		got = append(got, strings.Split(line, "	")[1:])
	}
	want := [][]string{
		{"-S", socket, "load-buffer", "-b", "chrote-dispatch-123", "-"},
		{"-S", socket, "send-keys", "-t", target, "C-u"},
		{"-S", socket, "paste-buffer", "-t", target, "-b", "chrote-dispatch-123"},
		{"-S", socket, "send-keys", "-t", target, "ENTER"},
		{"-S", socket, "send-keys", "-t", target, "C-m"},
		{"-S", socket, "capture-pane", "-p", "-J", "-t", target, "-S", "-40"},
		{"-S", socket, "delete-buffer", "-b", "chrote-dispatch-123"},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("tmux command sequence = %#v, want %#v", got, want)
	}
	wantSleeps := []sleepSnapshot{
		{Delay: tmuxSubmitDelay, CommandCount: 3},
		{Delay: tmuxSubmitDelay, CommandCount: 4},
		{Delay: tmuxSubmitDelay, CommandCount: 5},
	}
	if fmt.Sprint(sleeps) != fmt.Sprint(wantSleeps) {
		t.Fatalf("tmux submit pacing = %#v, want %#v", sleeps, wantSleeps)
	}
}

func TestRealTmuxHarnessClientRetriesWhenPastedPromptStillPending(t *testing.T) {
	fakeDir := t.TempDir()
	logPath := filepath.Join(fakeDir, "tmux.log")
	statePath := filepath.Join(fakeDir, "capture-count")
	fakeTmuxPath := filepath.Join(fakeDir, "tmux")
	fakeTmux := `#!/usr/bin/env bash
set -euo pipefail
stdin="$(cat)"
{
  printf 'ARGS'
  for arg in "$@"; do
    printf '\t%s' "$arg"
  done
  printf '\n'
  if [ -n "$stdin" ]; then
    printf 'STDIN_BEGIN\n%s\nSTDIN_END\n' "$stdin"
  fi
} >> "${TMUX_FAKE_LOG:?}"
if [[ " $* " == *" capture-pane "* ]]; then
  count=0
  if [ -f "${TMUX_FAKE_STATE:?}" ]; then
    count="$(cat "${TMUX_FAKE_STATE:?}")"
  fi
  count=$((count + 1))
  printf '%s' "$count" > "${TMUX_FAKE_STATE:?}"
  if [ "$count" -eq 1 ]; then
    printf '› [Pasted Content 1900 chars]\n'
  else
    printf 'submitted\n'
  fi
fi
`
	if err := os.WriteFile(fakeTmuxPath, []byte(fakeTmux), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("TMUX_FAKE_LOG", logPath)
	t.Setenv("TMUX_FAKE_STATE", statePath)
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	oldDelay := tmuxSubmitDelay
	oldSleep := tmuxSleep
	t.Cleanup(func() {
		tmuxSubmitDelay = oldDelay
		tmuxSleep = oldSleep
	})
	tmuxSubmitDelay = time.Millisecond
	tmuxSleep = func(time.Duration) {}

	ctx := context.Background()
	socket := "/tmp/chrote-test-tmux.sock"
	target := "tmux-worker"
	if err := (realTmuxHarnessClient{}).SendPrompt(ctx, socket, target, "dispatch-retry", strings.Repeat("worker prompt\n", 80)); err != nil {
		t.Fatalf("send prompt with fake tmux: %v", err)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake tmux log: %v", err)
	}
	enterCount := strings.Count(string(raw), "send-keys\t-t\t"+target+"\tENTER")
	controlMCount := strings.Count(string(raw), "send-keys\t-t\t"+target+"\tC-m")
	captureCount := strings.Count(string(raw), "capture-pane\t-p\t-J\t-t\t"+target)
	if enterCount != 2 || controlMCount != 2 || captureCount != 2 {
		t.Fatalf("retry counts enter=%d c-m=%d capture=%d, want 2/2/2\nlog:\n%s", enterCount, controlMCount, captureCount, raw)
	}
}

func TestTmuxPaneLooksLikePendingPastedInputHandlesBulletsInsidePrompt(t *testing.T) {
	pending := strings.Join([]string{
		"› [Pasted Content 8664 chars]────────",
		"",
		"  • Worker A: identify the API/runtime boundary.",
		"  • Worker B: identify the verification evidence.",
		"  gpt-5.5 xhigh · /tmp/workspace",
	}, "\n")
	if !tmuxPaneLooksLikePendingPastedInput(pending) {
		t.Fatalf("pending pasted prompt with bullet lines was not detected")
	}

	placeholder := pending + "\nWhen complete, emit exactly one sentinel line using the run value above: <<<CHROTE-DONE run-id=<the-run-value-above> status=ok artifact=<path-or-ref>>>"
	if !tmuxPaneLooksLikePendingPastedInput(placeholder) {
		t.Fatalf("placeholder sentinel inside pasted prompt was misclassified as completion")
	}

	working := pending + "\n\n• Working (1s • esc to interrupt)"
	if tmuxPaneLooksLikePendingPastedInput(working) {
		t.Fatalf("active Codex turn was misclassified as pending pasted input")
	}

	done := pending + "\n\n• Done\n<<<CHROTE-DONE run-id=run_x status=ok artifact=inline>>>"
	if tmuxPaneLooksLikePendingPastedInput(done) {
		t.Fatalf("completed Codex turn was misclassified as pending pasted input")
	}
}

func countFakeTmuxCommands(t *testing.T, logPath string) int {
	t.Helper()
	raw, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("read fake tmux log for command count: %v", err)
	}
	count := 0
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "ARGS	") {
			count++
		}
	}
	return count
}

func runTmuxFormationForTest(t *testing.T, client *fakeTmuxHarnessClient, board string) (*RunStatusProjection, []RunEvent) {
	t.Helper()
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	if board == "" {
		board = s4RunBoardFixture()
	}
	writeFixture(t, store.BoardPath("session-search"), board)
	cfg := tmuxTestConfig(t)
	if client.pane.CurrentPath == "" && !client.pane.Dead {
		client.pane.CurrentPath = cfg.Cwd
	}
	executor := newTmuxFormationExecutorWithClient(store, personas, cfg, client)
	engine := NewRunEngine(store, personas, executor)
	status, err := engine.RunFormation("session-search", "fmn_research", FormationRunRequest{
		Actor:  "agent:test",
		Limits: RunLimits{MaxDispatch: 5, MaxAttempts: 1},
	})
	if err != nil {
		t.Fatalf("run tmux formation: %v", err)
	}
	return status, readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
}

func tmuxTestConfig(t *testing.T) TmuxExecutorConfig {
	t.Helper()
	root := t.TempDir()
	return TmuxExecutorConfig{
		Harnesses:      []string{"openai-codex"},
		Socket:         filepath.Join(root, "tmux.sock"),
		Cwd:            root,
		Roots:          []string{root},
		SessionPrefix:  "tmux-",
		OutputCapBytes: defaultTmuxOutputCapBytes,
		TimeoutSeconds: 1,
	}
}

func clearExecutorEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"CHROTE_FORMATIONS_LAB_HARNESSES",
		"CHROTE_FORMATIONS_LAB_CWD",
		"CHROTE_FORMATIONS_LAB_ROOTS",
		"CHROTE_FORMATIONS_TMUX_HARNESSES",
		"CHROTE_FORMATIONS_TMUX_SOCKET",
		"CHROTE_FORMATIONS_TMUX_CWD",
		"CHROTE_FORMATIONS_TMUX_ROOTS",
		"CHROTE_FORMATIONS_TMUX_SESSION_PREFIX",
		"CHROTE_FORMATIONS_TMUX_PROD_SMOKE",
	} {
		t.Setenv(key, "")
	}
}

func tmuxRunBoardWithBrief(goal string) string {
	return strings.Replace(s4RunBoardFixture(), "title = \"Research\"\n\n[[formation.input]]", "title = \"Research\"\n\n[formation.brief]\ngoal = "+renderString(goal)+"\nbeadId = \"home-7kc4.7\"\n\n[[formation.input]]", 1)
}

func tmuxOrchestratedBoardFixture() string {
	return `schema = 1
id = "brd_orch"
slug = "session-search"
title = "Orchestrated Smoke"
rev = 1
updatedBy = "agent:test"
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_orch"
type = "orchestrated"
title = "Proposal crew"

[formation.brief]
goal = "Produce a tiny implementation proposal with API and test sections"
beadId = "home-9hjn"

[[formation.input]]
id = "port_orch_in"
label = "Input"

[[formation.output]]
id = "port_orch_out"
label = "Output"

[[formation.slot]]
id = "slot_lead"
label = "Lead"
controller = true
agentId = "lead"
harness = "openai-codex"

[[formation.slot]]
id = "slot_worker_a"
label = "Worker A"
controller = false
agentId = "worker-a"
harness = "openai-codex"

[[formation.slot]]
id = "slot_worker_b"
label = "Worker B"
controller = false
agentId = "worker-b"
harness = "openai-codex"
`
}

func tmuxPeerBoardFixture() string {
	return `schema = 1
id = "brd_peer"
slug = "session-search"
title = "Peer Smoke"
rev = 1
updatedBy = "agent:test"
updatedAt = "2026-06-03T16:00:00Z"

[[formation]]
id = "fmn_peer"
type = "peer"
title = "Peer proof pair"

[formation.brief]
goal = "Produce a peer synthesis from shared-plane contributions"
beadId = "home-21p5"

[[formation.input]]
id = "port_peer_in"
label = "Input"

[[formation.output]]
id = "port_peer_out"
label = "Output"

[[formation.slot]]
id = "slot_peer_a"
label = "Peer A"
controller = false
agentId = "peer-a"
harness = "openai-codex"

[[formation.slot]]
id = "slot_peer_b"
label = "Peer B"
controller = false
agentId = "peer-b"
harness = "openai-codex"
`
}

type fakeTmuxHarnessClient struct {
	sessions             []string
	pane                 tmuxPaneState
	listErr              error
	describeErr          error
	sendErr              error
	captureErr           error
	sendErrEchoPrompt    bool
	captureErrEchoPrompt bool
	captures             []string
	artifact             string
	lastPrompt           string
	lastTarget           string
	lastDispatchID       string
	sentPrompts          []string
	sendTargets          []string
	paneText             string
	awaitingCapture      bool
	sendCalls            int
	captureCalls         int
}

func (f *fakeTmuxHarnessClient) ListSessions(context.Context, string) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]string(nil), f.sessions...), nil
}

func (f *fakeTmuxHarnessClient) DescribeActivePane(context.Context, string, string) (tmuxPaneState, error) {
	if f.describeErr != nil {
		return tmuxPaneState{}, f.describeErr
	}
	return f.pane, nil
}

func (f *fakeTmuxHarnessClient) SendPrompt(_ context.Context, _, target, dispatchID, prompt string) error {
	f.sendCalls++
	f.lastDispatchID = dispatchID
	f.lastPrompt = prompt
	f.lastTarget = target
	f.sentPrompts = append(f.sentPrompts, prompt)
	f.sendTargets = append(f.sendTargets, target)
	f.awaitingCapture = true
	if f.sendErrEchoPrompt {
		return fmt.Errorf("tmux send failed with prompt %s token=sk-tmu...t123", prompt)
	}
	return f.sendErr
}

func (f *fakeTmuxHarnessClient) CapturePane(context.Context, string, string, int) (string, error) {
	f.captureCalls++
	if !f.awaitingCapture {
		return f.paneText, nil
	}
	if f.captureErrEchoPrompt {
		return "", fmt.Errorf("tmux capture failed after prompt %s token=sk-tmu...t123", f.lastPrompt)
	}
	if f.captureErr != nil {
		return "", f.captureErr
	}
	var captured string
	if len(f.captures) > 0 {
		captured = strings.ReplaceAll(f.captures[0], "run_missing", runIDFromPrompt(f.lastPrompt))
		f.captures = f.captures[1:]
	} else {
		artifact := f.artifact
		if artifact == "" {
			artifact = "reports/tmux.md"
		}
		captured = fmt.Sprintf("agent output\n<<<CHROTE-DONE run-id=%s status=ok artifact=%s>>>\n", runIDFromPrompt(f.lastPrompt), artifact)
	}
	captured = withPromptOutputContract(captured, f.lastPrompt)
	f.awaitingCapture = false
	if f.paneText != "" {
		f.paneText += "\n"
	}
	f.paneText += captured
	return f.paneText, nil
}

func runIDFromPrompt(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		if runID, ok := strings.CutPrefix(strings.TrimSpace(line), "run: "); ok {
			return strings.TrimSpace(runID)
		}
	}
	return "run_missing"
}

func withPromptOutputContract(captured, prompt string) string {
	if !strings.Contains(prompt, "formation output contract:") || strings.Contains(captured, "```chrote-outputs") {
		return captured
	}
	ports := outputPortsFromPrompt(prompt)
	if len(ports) == 0 {
		return captured
	}
	payloads := make(map[string]FormationOutputPayload, len(ports))
	text := strings.TrimSpace(captured)
	if sentinelAt := strings.Index(text, "<<<CHROTE-DONE "); sentinelAt >= 0 {
		text = strings.TrimSpace(text[:sentinelAt])
	}
	for _, portID := range ports {
		payloads[portID] = FormationOutputPayload{Text: text}
	}
	raw, err := json.Marshal(payloads)
	if err != nil {
		return captured
	}
	block := "```chrote-outputs\n" + string(raw) + "\n```\n"
	if sentinelAt := strings.Index(captured, "<<<CHROTE-DONE "); sentinelAt >= 0 {
		return strings.TrimRight(captured[:sentinelAt], "\n") + "\n" + block + captured[sentinelAt:]
	}
	return strings.TrimRight(captured, "\n") + "\n" + block
}

func outputPortsFromPrompt(prompt string) []string {
	var ports []string
	inList := false
	for _, line := range strings.Split(prompt, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "Use all and only these output port ids:":
			inList = true
			continue
		case inList && strings.HasPrefix(trimmed, "- "):
			fields := strings.Fields(strings.TrimPrefix(trimmed, "- "))
			if len(fields) > 0 {
				ports = append(ports, fields[0])
			}
			continue
		case inList:
			return ports
		}
	}
	return ports
}

type promptEchoDispatchAdapter struct{}

func (promptEchoDispatchAdapter) SendSlotDispatch(payload SlotDispatchPayload) error {
	return fmt.Errorf("adapter refused prompt %s token=sk-dispatchadapter123", payload.Prompt)
}

func assertLedgerDataForTypesRedacted(t *testing.T, events []RunEvent, eventTypes []string, forbidden ...string) {
	t.Helper()
	want := make(map[string]bool, len(eventTypes))
	seen := make(map[string]bool, len(eventTypes))
	for _, eventType := range eventTypes {
		want[eventType] = true
	}
	for _, event := range events {
		if !want[event.Type] {
			continue
		}
		seen[event.Type] = true
		raw, err := json.Marshal(event.Data)
		if err != nil {
			t.Fatalf("marshal event data for %s: %v", event.Type, err)
		}
		data := string(raw)
		for _, value := range forbidden {
			if strings.Contains(data, value) {
				t.Fatalf("%s data leaked %q: %s", event.Type, value, data)
			}
		}
	}
	for _, eventType := range eventTypes {
		if !seen[eventType] {
			t.Fatalf("events = %v, want redaction-checked event %s", eventTypesOf(events), eventType)
		}
	}
}

func firstStartedInputText(t *testing.T, events []RunEvent, nodeID string) string {
	t.Helper()
	for _, event := range events {
		if event.Type != RunEventNodeStarted || event.NodeID != nodeID {
			continue
		}
		inputs, ok := event.Data["inputRefs"].([]any)
		if !ok || len(inputs) == 0 {
			t.Fatalf("node_started %s inputRefs = %#v, want non-empty slice", nodeID, event.Data["inputRefs"])
		}
		return runInputRefFromAny(inputs[0]).Text
	}
	t.Fatalf("missing node_started for %s", nodeID)
	return ""
}

func eventTypesOf(events []RunEvent) []string {
	values := make([]string, 0, len(events))
	for _, event := range events {
		values = append(values, event.Type)
	}
	return values
}

// TestRealTmuxHarnessClientCapturePaneJoinsWrappedChroteOutputs locks the
// home-o25k regression: a real agent emits the chrote-outputs contract as a
// single long JSON line. At a normal pane width tmux soft-wraps that line, and
// capture-pane *without* -J returns the wrap as a hard newline injected inside
// the JSON string value. parseChroteOutputs then fails with an invalid-JSON
// error and the run blocks with invalid_output_payloads even though dispatch,
// send, and capture all succeeded. CapturePane must ask tmux to join wrapped
// lines (-J) so the captured payload survives intact and parses.
func TestRealTmuxHarnessClientCapturePaneJoinsWrappedChroteOutputs(t *testing.T) {
	fakeDir := t.TempDir()
	logPath := filepath.Join(fakeDir, "tmux.log")
	joinedPath := filepath.Join(fakeDir, "joined.txt")
	wrappedPath := filepath.Join(fakeDir, "wrapped.txt")
	fakeTmuxPath := filepath.Join(fakeDir, "tmux")

	fence := "```"
	// Joined output: the single-line JSON a real agent emits, as -J would return it.
	joined := "done thinking\n" + fence + "chrote-outputs\n" +
		`{"port_solo_out":{"text":"SOLO-REAL-ANSWER=399 padded so this json line is wider than a normal terminal pane"}}` + "\n" +
		fence + "\n<<<CHROTE-DONE run-id=run_o25k status=ok artifact=solo.md>>>\n"
	// Wrapped output: what an 80-column pane shows without -J — a hard newline
	// landing inside the JSON string value, which is invalid JSON.
	wrapped := "done thinking\n" + fence + "chrote-outputs\n" +
		`{"port_solo_out":{"text":"SOLO-REAL-ANSWER=399 padded so this json line is wider th` + "\n" +
		`an a normal terminal pane"}}` + "\n" +
		fence + "\n<<<CHROTE-DONE run-id=run_o25k status=ok artifact=solo.md>>>\n"
	if err := os.WriteFile(joinedPath, []byte(joined), 0o644); err != nil {
		t.Fatalf("write joined fixture: %v", err)
	}
	if err := os.WriteFile(wrappedPath, []byte(wrapped), 0o644); err != nil {
		t.Fatalf("write wrapped fixture: %v", err)
	}

	// The fake tmux models pane-width wrapping: capture-pane returns the wrapped
	// (broken) fixture unless -J is requested, in which case it returns the
	// joined (intact) fixture.
	fakeTmux := `#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
{
  printf 'ARGS'
  for arg in "$@"; do
    printf '	%s' "$arg"
  done
  printf '\n'
} >> "${TMUX_FAKE_LOG:?}"
if [[ " $* " == *" capture-pane "* ]]; then
  if [[ " $* " == *" -J "* ]]; then
    cat "${TMUX_FAKE_JOINED:?}"
  else
    cat "${TMUX_FAKE_WRAPPED:?}"
  fi
fi
`
	if err := os.WriteFile(fakeTmuxPath, []byte(fakeTmux), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("TMUX_FAKE_LOG", logPath)
	t.Setenv("TMUX_FAKE_JOINED", joinedPath)
	t.Setenv("TMUX_FAKE_WRAPPED", wrappedPath)
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	captured, err := (realTmuxHarnessClient{}).CapturePane(context.Background(), "/tmp/chrote-test-o25k.sock", "tmux-solo", 1<<20)
	if err != nil {
		t.Fatalf("capture pane with fake tmux: %v", err)
	}

	clean, outputs, err := parseChroteOutputs(captured)
	if err != nil {
		t.Fatalf("captured chrote-outputs failed to parse (home-o25k wrap regression): %v\ncaptured=%q", err, captured)
	}
	payload, ok := outputs["port_solo_out"]
	if !ok {
		t.Fatalf("captured outputs missing declared port port_solo_out: %#v (clean=%q)", outputs, clean)
	}
	if !strings.Contains(payload.Text, "SOLO-REAL-ANSWER=399") {
		t.Fatalf("captured payload text = %q, want it to contain the model answer intact", payload.Text)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake tmux log: %v", err)
	}
	if !strings.Contains(string(raw), "\t-J") {
		t.Fatalf("capture-pane did not request -J to join wrapped lines:\n%s", raw)
	}
}

// TestFormationResultRecoversBarePortJSONFromRealAgentCapture locks the
// home-3b7m finding: real coding-agent TUIs (Claude Code, Codex) emit the
// chrote-outputs payload as bare JSON with no literal ```chrote-outputs fence
// (the markdown fence is rendered away/omitted), while the captured pane still
// contains the echoed prompt's placeholder example fence
// ({"port_id":{"text":"payload for that output"}}). The fence-only parser then
// latches onto that placeholder and the run blocks with missing_output_payload
// even though the agent emitted the correct declared port id and payload. The
// deterministic bash responder hid this by echoing a literal fence keyed by the
// real port id. The executor must recover the declared-port-keyed JSON the
// agent actually emitted, while still failing loud on unknown/missing ports.
func TestFormationResultRecoversBarePortJSONFromRealAgentCapture(t *testing.T) {
	port := "port_01KV09ZAXN949Y9N1GGW1KDAHT"
	fence := "```"
	// Mirrors a real Claude Code / Codex capture: echoed contract (with the
	// placeholder example fence), then the agent's bare single-line JSON keyed by
	// the real declared port id, then the completion sentinel.
	captured := strings.Join([]string{
		"brief: SOLO real-LLM smoke. Compute 19*21 ...",
		"formation output contract:",
		fence + "chrote-outputs",
		`{"port_id":{"text":"payload for that output"}}`,
		fence,
		"Use all and only these output port ids:",
		"- " + port + ` label="Output"`,
		"turn marker: turn_01KV09ZAYR0DJ1MK2AEZWN6ENE",
		"When complete, emit exactly one sentinel line ...",
		`{"` + port + `":{"text":"SOLO-REAL-ANSWER=399"}}`,
		"<<<CHROTE-DONE run-id=run_01KV09ZAYC status=ok artifact=none>>>",
	}, "\n")

	exec := &TmuxFormationExecutor{config: TmuxExecutorConfig{OutputCapBytes: 1 << 20}}
	req := FormationExecution{
		NodeID:    "fmn_solo",
		Formation: FormationNode{Outputs: []FormationPort{{ID: port, Label: "Output"}}},
	}

	res, err := exec.formationResultFromText(req, "report", captured)
	if err != nil {
		t.Fatalf("formationResultFromText errored on real-agent capture (home-3b7m regression): %v", err)
	}
	payload, ok := res.Outputs[port]
	if !ok {
		t.Fatalf("declared port %q not recovered from bare-JSON real-agent capture; outputs=%#v", port, res.Outputs)
	}
	if !strings.Contains(payload.Text, "SOLO-REAL-ANSWER=399") {
		t.Fatalf("recovered payload = %q, want the agent's real computed answer", payload.Text)
	}
	if _, ok := res.Outputs["port_id"]; ok {
		t.Fatalf("placeholder example port_id leaked into outputs: %#v", res.Outputs)
	}

	// Fail-loud preserved: a bare JSON object keyed by a port that is NOT declared
	// must not be routed; the declared port is still missing -> the run blocks.
	unknownCapture := strings.Join([]string{
		"turn marker: turn_x",
		`{"port_not_declared":{"text":"SOLO-REAL-ANSWER=399"}}`,
		"<<<CHROTE-DONE run-id=run_x status=ok artifact=none>>>",
	}, "\n")
	if _, err := exec.formationResultFromText(req, "report", unknownCapture); err == nil {
		t.Fatalf("expected missing_output_payload for undeclared bare port, got success")
	}
}
