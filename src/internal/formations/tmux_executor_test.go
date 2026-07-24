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

func TestTmuxExecutorAcceptsConfiguredCockpitSocket(t *testing.T) {
	// Owner ruling: Formations shares the cockpit socket. A real, stable,
	// non-symlink socket that is ALSO the configured cockpit socket must now
	// validate cleanly; safety moved from socket-refusal to session-scoping.
	cfg := tmuxTestConfig(t)
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "perttu="+cfg.Socket)
	t.Setenv("CHROTE_DEFAULT_TMUX_SOCKET", cfg.Socket)

	if err := newTmuxFormationExecutorWithClient(nil, nil, cfg, &fakeTmuxHarnessClient{}).validateConfiguredBoundary(); err != nil {
		t.Fatalf("configured cockpit socket validate error = %v, want accepted", err)
	}
}

func TestTmuxExecutorAllowsDisposableTempDogfood(t *testing.T) {
	if err := newTmuxFormationExecutorWithClient(nil, nil, tmuxTestConfig(t), &fakeTmuxHarnessClient{}).validateConfiguredBoundary(); err != nil {
		t.Fatalf("disposable temp dogfood validate error = %v, want allowed", err)
	}
}

func TestTmuxExecutorAcceptsProductionDedicatedSocketOutsideTemp(t *testing.T) {
	// Production-style config: a dedicated tmux socket and workspace cwd/roots
	// that live OUTSIDE /tmp. The executor must accept it now that the disposable
	// /tmp dogfood requirement is removed.
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_TMPDIR", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("CHROTE_DEFAULT_TMUX_SOCKET", "")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "")

	cfg := nonTempWorkspace(t)
	if err := newTmuxFormationExecutorWithClient(nil, nil, cfg, &fakeTmuxHarnessClient{}).validateConfiguredBoundary(); err != nil {
		t.Fatalf("production dedicated socket + workspace outside /tmp validate error = %v, want accepted", err)
	}
}

func TestTmuxExecutorRejectsSocketSymlink(t *testing.T) {
	// The socket-identity/stability check is retained: a socket that is a
	// symlink is refused so the executor cannot be pointed at a moving target.
	cfg := tmuxTestConfig(t)
	aliasSocket := filepath.Join(filepath.Dir(cfg.Socket), "outside.sock")
	if err := os.Symlink("/dev/null", aliasSocket); err != nil {
		t.Fatalf("symlink outside socket fixture: %v", err)
	}
	cfg.Socket = aliasSocket
	assertAttachmentAuditUnavailable(t, cfg)
}

func TestTmuxExecutorNonTempWorkspaceUsesOrdinaryValidationCodes(t *testing.T) {
	// The /tmp workspace boundary is removed: a missing non-/tmp cwd or root is
	// now an ordinary configuration error, not session_target_attachment_audit_unavailable.
	t.Run("missing cwd", func(t *testing.T) {
		cfg := tmuxTestConfig(t)
		cfg.Cwd = "/srv/path-that-does-not-exist"
		cfg.Roots = []string{cfg.Cwd}
		assertBoundaryCode(t, cfg, "unavailable_cwd")
	})

	t.Run("missing root", func(t *testing.T) {
		cfg := tmuxTestConfig(t)
		cfg.Roots = []string{cfg.Cwd, "/srv/path-that-does-not-exist"}
		assertBoundaryCode(t, cfg, "unavailable_root")
	})
}

func TestTmuxExecutorRefusesSocketRetargetBeforeSend(t *testing.T) {
	cfg := tmuxTestConfig(t)
	client := &fakeTmuxHarnessClient{
		sessions: []string{"tmux-scout"},
		pane:     tmuxPaneState{CurrentPath: cfg.Cwd},
	}
	client.afterCapture = func(call int) {
		if call != 1 {
			return
		}
		if err := os.Remove(cfg.Socket); err != nil {
			t.Fatalf("remove disposable socket fixture: %v", err)
		}
		if err := os.Symlink("/dev/null", cfg.Socket); err != nil {
			t.Fatalf("retarget disposable socket fixture: %v", err)
		}
	}

	status, events := runTmuxFormationForTestWithConfig(t, client, "", cfg)
	if status.Status != RunStatusBlocked {
		t.Fatalf("run status = %+v, want blocked", status)
	}
	errorEvent := eventOfType(t, events, RunEventError)
	if errorEvent.Data["code"] != "session_target_attachment_audit_unavailable" {
		t.Fatalf("run error code = %#v, want session_target_attachment_audit_unavailable", errorEvent.Data["code"])
	}
	if client.listCalls != 1 || client.describeCalls != 1 || client.captureCalls != 1 {
		t.Fatalf("pre-retarget client calls = list:%d describe:%d capture:%d, want 1/1/1", client.listCalls, client.describeCalls, client.captureCalls)
	}
	if client.sendCalls != 0 {
		t.Fatalf("send calls after socket retarget = %d, want zero", client.sendCalls)
	}
}

func assertAttachmentAuditUnavailable(t *testing.T, cfg TmuxExecutorConfig) {
	t.Helper()
	err := newTmuxFormationExecutorWithClient(nil, nil, cfg, &fakeTmuxHarnessClient{}).validateConfiguredBoundary()
	var executionErr *RunExecutionError
	if !errors.As(err, &executionErr) {
		t.Fatalf("disposable boundary error = %v, want RunExecutionError", err)
	}
	if executionErr.Code != "session_target_attachment_audit_unavailable" {
		t.Fatalf("disposable boundary code = %q, want session_target_attachment_audit_unavailable", executionErr.Code)
	}
}

func assertBoundaryCode(t *testing.T, cfg TmuxExecutorConfig, wantCode string) {
	t.Helper()
	err := newTmuxFormationExecutorWithClient(nil, nil, cfg, &fakeTmuxHarnessClient{}).validateConfiguredBoundary()
	var executionErr *RunExecutionError
	if !errors.As(err, &executionErr) {
		t.Fatalf("boundary error = %v, want RunExecutionError with code %q", err, wantCode)
	}
	if executionErr.Code != wantCode {
		t.Fatalf("boundary code = %q, want %q", executionErr.Code, wantCode)
	}
}

func TestTmuxExecutorSessionFailuresRecordDurableBoundaryAndProvenance(t *testing.T) {
	for _, tc := range []struct {
		name     string
		client   *fakeTmuxHarnessClient
		wantCode string
	}{
		{
			name:     "spawn failure",
			client:   &fakeTmuxHarnessClient{createErr: errors.New("tmux new-session refused")},
			wantCode: "session_spawn_failed",
		},
		{
			name:     "dead pane",
			client:   &fakeTmuxHarnessClient{pane: tmuxPaneState{Dead: true}},
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
	if len(client.created) != 1 {
		t.Fatalf("created sessions = %v, want exactly one on-demand owned session", client.created)
	}
	ownedSession := client.created[0]
	adapterSend := eventOfType(t, events, RunEventAdapterSend)
	if adapterSend.Data["adapter"] != "tmux" || adapterSend.Data["sessionRef"] != "tmux:"+ownedSession || adapterSend.Data["promptSha256"] == "" {
		t.Fatalf("adapter_send data = %#v, want owned tmux session %q and prompt hash", adapterSend.Data, ownedSession)
	}
	if client.sendTargets[0] != ownedSession {
		t.Fatalf("send target = %q, want the owned session %q, never the foreign fixture", client.sendTargets[0], ownedSession)
	}
	if got, want := client.killed, []string{ownedSession}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("killed sessions = %v, want the one owned session torn down %v", got, want)
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

func TestTmuxExecutorOnlyCreatesAndKillsOwnedSessionsOnSharedSocket(t *testing.T) {
	// The shared cockpit socket also hosts the operator's interactive terminals
	// and other live agent sessions. Seed those as foreign fixtures and prove the
	// executor spawns exactly one uniquely-named session, only ever operates on
	// that session, tears down only that session, never issues kill-server, and
	// never disrupts a foreign session.
	foreign := []string{"sol", "terminal-1", "terminal-2", "mission-real-agent-smoke-0644", "0"}
	client := &fakeTmuxHarnessClient{
		sessions: append([]string(nil), foreign...),
		artifact: "reports/tmux-shared.md",
	}
	status, _ := runTmuxFormationForTest(t, client, "")
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("status = %+v, want succeeded final", status)
	}

	if len(client.created) != 1 {
		t.Fatalf("created sessions = %v, want exactly one owned on-demand session", client.created)
	}
	owned := client.created[0]
	if !safeTmuxSessionName(owned) {
		t.Fatalf("owned session name %q is not a safe tmux target", owned)
	}
	foreignSet := map[string]bool{}
	for _, name := range foreign {
		foreignSet[name] = true
	}
	if foreignSet[owned] {
		t.Fatalf("owned session name %q collides with a foreign session", owned)
	}

	// Every recorded tmux op either enumerates sessions (no target) or targets the
	// one owned session. There is structurally no kill-server, and no op may ever
	// name a foreign session.
	for _, op := range client.ops {
		if op.op == "kill-server" {
			t.Fatalf("executor issued kill-server on the shared socket")
		}
		if op.target == "" {
			continue
		}
		if op.target != owned {
			t.Fatalf("tmux op %q targeted %q; the executor must only ever touch the session it created (%q)", op.op, op.target, owned)
		}
	}
	if got, want := client.killed, []string{owned}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("killed sessions = %v, want only the owned session torn down %v", got, want)
	}

	// Every foreign session is still present and untouched after the run.
	live := map[string]bool{}
	for _, name := range client.liveSessions() {
		live[name] = true
	}
	for _, name := range foreign {
		if !live[name] {
			t.Fatalf("foreign session %q was disrupted; it must remain live and untouched", name)
		}
	}
}

func TestTmuxExecutorLazyStartsServerOnEmptySocket(t *testing.T) {
	// Owner ruling: Formations supports ANY configured tmux socket, including one
	// with no pre-existing server. Model that by removing the socket fixture and
	// reporting no live server: the executor must lazy-start a keeper (which, like
	// tmux, materializes the socket), then create/own/tear-down its run session
	// exactly as on an existing server — never kill-server, never tearing down the
	// keeper.
	cfg := tmuxTestConfig(t)
	if err := os.Remove(cfg.Socket); err != nil {
		t.Fatalf("remove socket fixture to model empty socket: %v", err)
	}
	client := &fakeTmuxHarnessClient{noServer: true, artifact: "reports/tmux-lazy.md"}
	status, _ := runTmuxFormationForTestWithConfig(t, client, "", cfg)
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("status = %+v, want succeeded final", status)
	}

	keeper := cfg.SessionPrefix + "keeper"
	if got, want := client.keepersStarted, []string{keeper}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("keepers started = %v, want exactly one lazy-start keeper %v", got, want)
	}
	if !safeTmuxSessionName(keeper) {
		t.Fatalf("keeper name %q is not a safe tmux target", keeper)
	}
	if len(client.created) != 1 {
		t.Fatalf("created sessions = %v, want exactly one owned run session", client.created)
	}
	run := client.created[0]
	if run == keeper {
		t.Fatalf("owned run session %q must be distinct from the keeper", run)
	}

	for _, op := range client.ops {
		if op.op == "kill-server" {
			t.Fatalf("executor issued kill-server on the lazy-started socket")
		}
	}
	// Only the owned run session is torn down; the keeper is infrastructure and is
	// never reclaimed by teardown.
	if got, want := client.killed, []string{run}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("killed sessions = %v, want only the owned run session %v (keeper must never be torn down)", got, want)
	}
	live := map[string]bool{}
	for _, name := range client.liveSessions() {
		live[name] = true
	}
	if !live[keeper] {
		t.Fatalf("keeper %q must remain live after the run; it is left running by design", keeper)
	}
}

func TestTmuxExecutorReusesExistingServer(t *testing.T) {
	// When a server is already running on the configured socket, the executor must
	// use it as-is: no second keeper is lazy-started and the existing server is
	// left untouched. Seed foreign sessions to model a live shared server.
	foreign := []string{"sol", "terminal-1"}
	client := &fakeTmuxHarnessClient{
		sessions: append([]string(nil), foreign...),
		artifact: "reports/tmux-existing.md",
	}
	status, _ := runTmuxFormationForTest(t, client, "")
	if status.Status != RunStatusSucceeded || !status.Final {
		t.Fatalf("status = %+v, want succeeded final", status)
	}

	if len(client.keepersStarted) != 0 {
		t.Fatalf("keepers started = %v, want none when a server already exists", client.keepersStarted)
	}
	for _, op := range client.ops {
		if op.op == "start-keeper" {
			t.Fatalf("executor lazy-started a keeper despite an existing server")
		}
		if op.op == "kill-server" {
			t.Fatalf("executor issued kill-server on the existing server")
		}
	}
	if len(client.created) != 1 {
		t.Fatalf("created sessions = %v, want exactly one owned run session", client.created)
	}
	if got, want := client.killed, []string{client.created[0]}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("killed sessions = %v, want only the owned run session %v", got, want)
	}
	// Foreign sessions on the existing server remain live and untouched.
	live := map[string]bool{}
	for _, name := range client.liveSessions() {
		live[name] = true
	}
	for _, name := range foreign {
		if !live[name] {
			t.Fatalf("foreign session %q was disrupted; the existing server must be left untouched", name)
		}
	}
}

func TestPickOwnedSessionNameFailsClosedOnForeignCollision(t *testing.T) {
	// A name collision with a pre-existing (possibly foreign) session must never
	// reuse or touch it: the executor regenerates a fresh unique name instead.
	cfg := tmuxTestConfig(t)
	origNonce := newSessionNonce
	t.Cleanup(func() { newSessionNonce = origNonce })
	var calls int
	newSessionNonce = func() string {
		calls++
		if calls == 1 {
			return "dup" // first candidate deliberately collides
		}
		return "unique"
	}
	runID, slotID := "run_x", "slot_y"
	collidingForeign := cfg.SessionPrefix + runID + "-" + slotID + "-dup"
	client := &fakeTmuxHarnessClient{sessions: []string{collidingForeign, "sol", "terminal-1"}}
	executor := newTmuxFormationExecutorWithClient(nil, nil, cfg, client)
	if err := executor.validateConfiguredBoundary(); err != nil {
		t.Fatalf("validate boundary: %v", err)
	}

	name, err := executor.pickOwnedSessionName(context.Background(), runID, slotID)
	if err != nil {
		t.Fatalf("pick owned session name: %v", err)
	}
	if name == collidingForeign {
		t.Fatalf("owned name = %q, must never reuse the colliding foreign session", name)
	}
	if want := cfg.SessionPrefix + runID + "-" + slotID + "-unique"; name != want {
		t.Fatalf("owned name = %q, want regenerated unique name %q", name, want)
	}
	// Name selection is read-only: it enumerates sessions but never creates,
	// kills, or dispatches to anything.
	if len(client.created) != 0 || len(client.killed) != 0 || client.sendCalls != 0 {
		t.Fatalf("pick mutated tmux: created=%v killed=%v sends=%d", client.created, client.killed, client.sendCalls)
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

func TestTmuxExecutorReadsOutputRefArtifactForPortRouting(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4NamedOutputBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	leftArtifact := filepath.Join(store.Workspace, ".formations", "artifacts", "left-long.md")
	leftArtifactRef, err := filepath.Rel(store.Workspace, leftArtifact)
	if err != nil {
		t.Fatalf("relative artifact ref: %v", err)
	}
	leftArtifactRef = filepath.ToSlash(leftArtifactRef)
	longLeft := "LEFT-ARTIFACT-BEGIN\n" + strings.Repeat("long routed artifact line with preserved spacing 0123456789\n", 80) + "LEFT-ARTIFACT-END"
	writeFixture(t, leftArtifact, longLeft)
	payloads := map[string]FormationOutputPayload{
		"port_split_left":  {Text: "LEFT-SUMMARY", Ref: leftArtifactRef},
		"port_split_right": {Text: "RIGHT-FROM-TMUX"},
	}
	rawPayloads, err := json.Marshal(payloads)
	if err != nil {
		t.Fatalf("marshal output payloads: %v", err)
	}
	cfg := tmuxTestConfig(t)
	cfg.Cwd = store.Workspace
	cfg.Roots = []string{store.Workspace}
	client := &fakeTmuxHarnessClient{
		sessions: []string{"tmux-scout"},
		pane:     tmuxPaneState{CurrentPath: cfg.Cwd},
		captures: []string{
			"splitter wrote long payload artifact\n```chrote-outputs\n" + string(rawPayloads) + "\n```\n<<<CHROTE-DONE run-id=run_missing status=ok artifact=split.md>>>",
			"left consumer saw artifact payload\n<<<CHROTE-DONE run-id=run_missing status=ok artifact=left.md>>>",
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
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	splitOutput := findNodeOutputEvent(t, events, "fmn_split")
	outputs, ok := splitOutput.Data["outputs"].(map[string]any)
	if !ok {
		t.Fatalf("split output payloads = %#v, want map", splitOutput.Data["outputs"])
	}
	assertOutputPayloadText(t, outputs, "port_split_left", longLeft)
	assertOutputPayloadText(t, outputs, "port_split_right", "RIGHT-FROM-TMUX")
	leftPayload, ok := outputs["port_split_left"].(map[string]any)
	if !ok || leftPayload["ref"] != leftArtifactRef {
		t.Fatalf("left payload = %#v, want ref %q", outputs["port_split_left"], leftArtifactRef)
	}
	if got := firstStartedInputText(t, events, "fmn_left"); got != longLeft {
		t.Fatalf("left routed input length=%d, want artifact length=%d", len(got), len(longLeft))
	}
	if len(client.sentPrompts) < 2 || !strings.Contains(client.sentPrompts[1], longLeft) {
		t.Fatalf("left consumer prompt did not receive hydrated artifact body; prompts=%d", len(client.sentPrompts))
	}
}

func TestTmuxExecutorBlocksInvalidOutputRefArtifacts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		setupRef func(t *testing.T, store *Store) string
		wantCode string
	}{
		{
			name: "missing_ref_file",
			setupRef: func(t *testing.T, store *Store) string {
				t.Helper()
				return filepath.Join(store.Workspace, ".formations", "artifacts", "missing-left.md")
			},
			wantCode: "unavailable_output_ref",
		},
		{
			name: "non_regular_ref_directory",
			setupRef: func(t *testing.T, store *Store) string {
				t.Helper()
				dir := filepath.Join(store.Workspace, ".formations", "artifacts", "directory-ref")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("create directory ref fixture: %v", err)
				}
				return dir
			},
			wantCode: "invalid_output_ref",
		},
		{
			name: "outside_configured_roots",
			setupRef: func(t *testing.T, store *Store) string {
				t.Helper()
				outside := filepath.Join(t.TempDir(), "outside-left.md")
				writeFixture(t, outside, "SHOULD-NOT-ROUTE")
				return outside
			},
			wantCode: "output_ref_outside_root",
		},
		{
			name: "symlink_escape",
			setupRef: func(t *testing.T, store *Store) string {
				t.Helper()
				outside := filepath.Join(t.TempDir(), "outside-left.md")
				writeFixture(t, outside, "SHOULD-NOT-ROUTE")
				insideLink := filepath.Join(store.Workspace, ".formations", "artifacts", "linked-outside.md")
				if err := os.MkdirAll(filepath.Dir(insideLink), 0o755); err != nil {
					t.Fatalf("mkdir symlink parent: %v", err)
				}
				if err := os.Symlink(outside, insideLink); err != nil {
					t.Fatalf("create symlink escape fixture: %v", err)
				}
				return insideLink
			},
			wantCode: "output_ref_outside_root",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, personas := s4RunFixture(t)
			store.Now = fixedClock()
			personas.Now = fixedClock()
			createS4Persona(t, personas, "scout")
			writeFixture(t, store.BoardPath("session-search"), s4NamedOutputBoardFixture())
			board, err := store.ReadBoard("session-search")
			if err != nil {
				t.Fatalf("read board: %v", err)
			}
			badRef := tc.setupRef(t, store)
			payloads := map[string]FormationOutputPayload{
				"port_split_left":  {Text: "LEFT-SUMMARY", Ref: badRef},
				"port_split_right": {Text: "RIGHT-FROM-TMUX"},
			}
			rawPayloads, err := json.Marshal(payloads)
			if err != nil {
				t.Fatalf("marshal output payloads: %v", err)
			}
			cfg := tmuxTestConfig(t)
			cfg.Cwd = store.Workspace
			cfg.Roots = []string{store.Workspace}
			client := &fakeTmuxHarnessClient{
				sessions: []string{"tmux-scout"},
				pane:     tmuxPaneState{CurrentPath: cfg.Cwd},
				captures: []string{
					"splitter referenced invalid artifact\n```chrote-outputs\n" + string(rawPayloads) + "\n```\n<<<CHROTE-DONE run-id=run_missing status=ok artifact=split.md>>>",
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
			if status.Status != RunStatusBlocked || status.Final {
				t.Fatalf("status = %+v, want blocked non-final", status)
			}
			events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
			errEvent := eventOfType(t, events, RunEventError)
			if errEvent.NodeID != "fmn_split" || errEvent.Data["code"] != tc.wantCode || errEvent.Data["boundary"] != "executor" {
				t.Fatalf("error event = %+v data=%#v, want %s on fmn_split/executor", errEvent, errEvent.Data, tc.wantCode)
			}
			if eventsContainNodeStart(events, "fmn_left") {
				t.Fatalf("events = %v, invalid ref must not route to fmn_left", eventTypesOf(events))
			}
		})
	}
}

func eventsContainNodeStart(events []RunEvent, nodeID string) bool {
	for _, event := range events {
		if event.Type == RunEventNodeStarted && event.NodeID == nodeID {
			return true
		}
	}
	return false
}

func TestTmuxExecutorReattachesCompletedDispatchFromPaneCapture(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4RunBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	started, err := store.StartRun("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	dispatcher := NewSlotDispatcher(store, &fakeDispatchAdapter{})
	lease, err := dispatcher.DispatchSlot(started.RunID, SlotDispatchRequest{
		NodeID:      "fmn_research",
		SlotID:      "slot_research",
		AgentID:     "scout",
		Harness:     "openai-codex",
		SessionStem: "scout",
		SessionRef:  "tmux:tmux-scout",
		Prompt:      "Do the work",
		Attempt:     1,
	})
	if err != nil {
		t.Fatalf("dispatch slot: %v", err)
	}
	if err := dispatcher.CompleteFromCapture(started.RunID, lease.DispatchID, "still working"); !errors.Is(err, ErrDispatchTimeout) {
		t.Fatalf("complete without sentinel error = %v, want ErrDispatchTimeout", err)
	}
	if _, err := store.ResumeRun(started.RunID, RunResumeRequest{Actor: "agent:test", Mode: "reattach", Reason: "recover completed pane"}); err != nil {
		t.Fatalf("record resume: %v", err)
	}
	cfg := tmuxTestConfig(t)
	client := &fakeTmuxHarnessClient{paneText: fmt.Sprintf("reattached pane output\n```chrote-outputs\n{\"port_research_out\":{\"text\":\"reattached from tmux\"}}\n```\n<<<CHROTE-DONE run-id=%s status=ok artifact=done.md>>>", started.RunID)}
	executor := newTmuxFormationExecutorWithClient(store, personas, cfg, client)
	result, err := executor.ReattachFormationDispatch(FormationReattachRequest{
		RunID:      started.RunID,
		DispatchID: lease.DispatchID,
		NodeID:     "fmn_research",
		SlotID:     "slot_research",
		Formation:  board.Formations[0],
	})
	if err != nil {
		t.Fatalf("reattach dispatch: %v", err)
	}
	if client.sendCalls != 0 || client.captureCalls != 1 {
		t.Fatalf("client send/capture calls = %d/%d, want no resend and one capture", client.sendCalls, client.captureCalls)
	}
	if result.Outputs["port_research_out"].Text != "reattached from tmux" {
		t.Fatalf("result outputs = %#v, want reattached payload", result.Outputs)
	}
	events := readRunEvents(t, findOnlyRunLedger(t, store, "session-search"))
	if eventOfType(t, events, RunEventSlotResult).Data["dispatchId"] != lease.DispatchID {
		t.Fatalf("events = %#v, want slot_result for original dispatch", events)
	}
}

func TestTmuxExecutorReattachRejectsOversizedCapture(t *testing.T) {
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	writeFixture(t, store.BoardPath("session-search"), s4RunBoardFixture())
	board, err := store.ReadBoard("session-search")
	if err != nil {
		t.Fatalf("read board: %v", err)
	}
	started, err := store.StartRun("session-search", RunStartRequest{
		MissionID:         "mis_showcase",
		Actor:             "agent:test",
		ExpectedBoardETag: board.ETag,
		ExpectedBoardRev:  board.Rev,
		Personas:          personas,
	})
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	dispatcher := NewSlotDispatcher(store, &fakeDispatchAdapter{})
	lease, err := dispatcher.DispatchSlot(started.RunID, SlotDispatchRequest{
		NodeID:      "fmn_research",
		SlotID:      "slot_research",
		AgentID:     "scout",
		Harness:     "openai-codex",
		SessionStem: "scout",
		SessionRef:  "tmux:tmux-scout",
		Prompt:      "Do the work",
		Attempt:     1,
	})
	if err != nil {
		t.Fatalf("dispatch slot: %v", err)
	}
	cfg := tmuxTestConfig(t)
	cfg.OutputCapBytes = 64
	client := &fakeTmuxHarnessClient{paneText: strings.Repeat("x", 65) + "\n<<<CHROTE-DONE run-id=" + started.RunID + " status=ok artifact=done.md>>>"}
	executor := newTmuxFormationExecutorWithClient(store, personas, cfg, client)
	_, err = executor.ReattachFormationDispatch(FormationReattachRequest{
		RunID:      started.RunID,
		DispatchID: lease.DispatchID,
		NodeID:     "fmn_research",
		SlotID:     "slot_research",
		Formation:  board.Formations[0],
	})
	var executionErr *RunExecutionError
	if !errors.As(err, &executionErr) || executionErr.Code != "oversized_output" || executionErr.DispatchID != lease.DispatchID {
		t.Fatalf("reattach oversized error = %#v, want oversized_output with dispatch provenance", err)
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
	if len(client.created) != 2 {
		t.Fatalf("created sessions = %v, want one owned session per peer", client.created)
	}
	ownedA, ownedB := client.created[0], client.created[1]
	if got, want := client.sendTargets, []string{ownedA, ownedB, ownedA}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("send targets = %v, want peer-turn/turn/facilitator on owned sessions %v (facilitator reuses peer A)", got, want)
	}
	for _, foreign := range []string{"tmux-peer-a", "tmux-peer-b"} {
		for _, target := range client.sendTargets {
			if target == foreign {
				t.Fatalf("send targeted foreign session %q; executor must only dispatch to sessions it created", foreign)
			}
		}
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

func TestPeerPlaneAppendStaysOnPinnedRunDirectoryAfterPathSwap(t *testing.T) {
	executor, req, runDirectory := peerPlaneSecurityFixture(t)
	plane, err := executor.seedPeerPlane(req, nil)
	if err != nil {
		t.Fatalf("seed peer plane: %v", err)
	}
	defer plane.close()
	if got, want := plane.relativePath, runArtifactPath("session-search", req.RunID, ".peer.md"); got != want {
		t.Fatalf("peer plane relative path = %q, want %q", got, want)
	}
	detachedDirectory := runDirectory + ".detached"
	if err := os.Rename(runDirectory, detachedDirectory); err != nil {
		t.Fatalf("detach pinned run directory: %v", err)
	}
	if err := os.MkdirAll(runDirectory, 0o770); err != nil {
		t.Fatalf("create replacement run directory: %v", err)
	}
	replacementPlane := filepath.Join(runDirectory, req.RunID+".peer.md")
	const replacementBefore = "replacement directory must not receive peer output\n"
	writeFixture(t, replacementPlane, replacementBefore)

	if err := executor.appendPeerPlaneOutput(plane, tmuxSlotOutput{
		SlotID: "slot_peer", AgentID: "peer-a", Phase: "peer-turn", Text: "PINNED-PEER-CONTRIBUTION",
	}); err != nil {
		t.Fatalf("append through pinned run directory: %v", err)
	}
	if got := readFile(t, replacementPlane); got != replacementBefore {
		t.Fatalf("replacement run directory was mutated: %q", got)
	}
	detachedPlane := filepath.Join(detachedDirectory, req.RunID+".peer.md")
	if got := readFile(t, detachedPlane); !strings.Contains(got, "PINNED-PEER-CONTRIBUTION") {
		t.Fatalf("pinned peer plane missing contribution:\n%s", got)
	}
}

func TestPeerPlaneAppendRejectsSymlinkAndHardlinkSubstitution(t *testing.T) {
	tests := []struct {
		name       string
		substitute func(t *testing.T, victimPath, planePath string)
	}{
		{
			name: "symlink",
			substitute: func(t *testing.T, victimPath, planePath string) {
				t.Helper()
				if err := os.Symlink(victimPath, planePath); err != nil {
					t.Fatalf("substitute peer plane symlink: %v", err)
				}
			},
		},
		{
			name: "hardlink",
			substitute: func(t *testing.T, victimPath, planePath string) {
				t.Helper()
				if err := os.Link(victimPath, planePath); err != nil {
					t.Fatalf("substitute peer plane hardlink: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executor, req, runDirectory := peerPlaneSecurityFixture(t)
			plane, err := executor.seedPeerPlane(req, nil)
			if err != nil {
				t.Fatalf("seed peer plane: %v", err)
			}
			defer plane.close()
			planePath := filepath.Join(runDirectory, req.RunID+".peer.md")
			if err := os.Remove(planePath); err != nil {
				t.Fatalf("remove seeded peer plane: %v", err)
			}
			victimPath := filepath.Join(t.TempDir(), "peer-plane-victim")
			const victimBefore = "private victim content\n"
			writeFixture(t, victimPath, victimBefore)
			test.substitute(t, victimPath, planePath)

			err = executor.appendPeerPlaneOutput(plane, tmuxSlotOutput{
				SlotID: "slot_peer", AgentID: "peer-a", Phase: "peer-turn", Text: "must be rejected",
			})
			if !errors.Is(err, ErrRunLedgerInvalid) {
				t.Fatalf("append through %s error = %v, want ErrRunLedgerInvalid", test.name, err)
			}
			if got := readFile(t, victimPath); got != victimBefore {
				t.Fatalf("rejected %s append mutated victim: %q", test.name, got)
			}
		})
	}
}

func TestPeerPlaneAppendRejectsAggregateByteOverflowWithoutMutation(t *testing.T) {
	executor, req, runDirectory := peerPlaneSecurityFixture(t)
	plane, err := executor.seedPeerPlane(req, nil)
	if err != nil {
		t.Fatalf("seed peer plane: %v", err)
	}
	defer plane.close()
	planePath := filepath.Join(runDirectory, req.RunID+".peer.md")
	before := strings.Repeat("x", peerPlaneMaxBytes)
	writeFixture(t, planePath, before)

	err = executor.appendPeerPlaneOutput(plane, tmuxSlotOutput{
		SlotID: "slot_peer", AgentID: "peer-a", Phase: "peer-turn", Text: "one byte too many",
	})
	if !errors.Is(err, ErrRunLedgerInvalid) {
		t.Fatalf("append beyond peer plane byte cap error = %v, want ErrRunLedgerInvalid", err)
	}
	if got := readFile(t, planePath); got != before {
		t.Fatalf("rejected oversized peer append mutated plane")
	}
}

func peerPlaneSecurityFixture(t *testing.T) (*TmuxFormationExecutor, FormationExecution, string) {
	t.Helper()
	workspace := t.TempDir()
	store := NewStore(workspace)
	runID := newPrefixedID("run")
	ledgerPath := filepath.Join(workspace, runArtifactPath("session-search", runID, ".ndjson"))
	writeFixture(t, ledgerPath, string(testRunLedgerBytes(t, testRunStartedEvent(runID, "session-search"))))
	executor := newTmuxFormationExecutorWithClient(store, nil, tmuxTestConfig(t), &fakeTmuxHarnessClient{})
	return executor, FormationExecution{
		RunID: runID, NodeID: "fmn_peer", Brief: FormationBrief{Goal: "Coordinate safely"},
	}, filepath.Dir(ledgerPath)
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
	// created order follows binding resolution: controller, worker A, worker B.
	if len(client.created) != 3 {
		t.Fatalf("created sessions = %v, want one owned session for the leader and each worker", client.created)
	}
	ownedLead, ownedWorkerA := client.created[0], client.created[1]
	if got, want := client.sendTargets, []string{ownedLead}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("send targets = %v, want only the leader's owned session %v", got, want)
	}
	for _, foreign := range []string{"tmux-lead", "tmux-worker-a", "tmux-worker-b"} {
		if client.sendTargets[0] == foreign {
			t.Fatalf("leader dispatch targeted foreign session %q; executor must only dispatch to sessions it created", foreign)
		}
	}
	if len(client.sentPrompts) != 1 {
		t.Fatalf("sent prompts = %d, want 1 leader prompt", len(client.sentPrompts))
	}
	leaderPrompt := client.sentPrompts[0]
	for _, want := range []string{
		"orchestration phase: leader-agentic",
		"formation team packet:",
		"tmux socket: " + cfg.Socket,
		"- slot slot_worker_a label=\"Worker A\" agent=\"worker-a\" harness=\"openai-codex\" session=\"" + ownedWorkerA + "\"",
		"tmux -S " + cfg.Socket + " capture-pane -t " + ownedWorkerA + " -p -S -120",
		"tmux -S " + cfg.Socket + " send-keys -t " + ownedWorkerA + " C-u",
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

func TestRealTmuxHarnessClientSendPromptPastesThenEntersUntilWorking(t *testing.T) {
	fakeDir := t.TempDir()
	logPath := filepath.Join(fakeDir, "tmux.log")
	fakeTmuxPath := filepath.Join(fakeDir, "tmux")
	// The fake reports the agent as working ("esc to interrupt") on the first
	// capture-pane, so the submit loop stops after a single Enter.
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
if [[ " $* " == *" capture-pane "* ]]; then
  printf '  ⏵⏵ bypass permissions on · esc to interrupt\n'
fi
`
	if err := os.WriteFile(fakeTmuxPath, []byte(fakeTmux), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("TMUX_FAKE_LOG", logPath)
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	oldDelay := tmuxSubmitDelay
	oldSettle := tmuxPasteSettleDelay
	oldSleep := tmuxSleep
	t.Cleanup(func() {
		tmuxSubmitDelay = oldDelay
		tmuxPasteSettleDelay = oldSettle
		tmuxSleep = oldSleep
	})
	tmuxSubmitDelay = 37 * time.Millisecond
	tmuxPasteSettleDelay = 71 * time.Millisecond
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
		{"-S", socket, "send-keys", "-t", target, "Enter"},
		{"-S", socket, "capture-pane", "-p", "-J", "-t", target, "-S", "-6"},
		{"-S", socket, "delete-buffer", "-b", "chrote-dispatch-123"},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("tmux command sequence = %#v, want %#v", got, want)
	}
	// One settle sleep after the paste (before any Enter), then one submit sleep
	// after the single Enter (before the capture confirms working).
	wantSleeps := []sleepSnapshot{
		{Delay: tmuxPasteSettleDelay, CommandCount: 3},
		{Delay: tmuxSubmitDelay, CommandCount: 4},
	}
	if fmt.Sprint(sleeps) != fmt.Sprint(wantSleeps) {
		t.Fatalf("tmux submit pacing = %#v, want %#v", sleeps, wantSleeps)
	}
}

func TestRealTmuxHarnessClientSubmitsPromptWithRepeatedEnter(t *testing.T) {
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
    printf '❯ worker prompt still sitting unsent in the input box\n'
  else
    printf '  ⏵⏵ bypass permissions on · esc to interrupt · ← for history\n'
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
	// The prompt is submitted by pressing Enter until the pane shows the agent
	// working ("esc to interrupt"): one Enter while it still sits unsent, a second
	// once working is detected, then the loop stops. The old ENTER/C-m keys are
	// gone — a single "Enter" key per attempt.
	enterCount := strings.Count(string(raw), "send-keys\t-t\t"+target+"\tEnter")
	legacyKeys := strings.Count(string(raw), "\tENTER") + strings.Count(string(raw), "\tC-m")
	captureCount := strings.Count(string(raw), "capture-pane	-p	-J	-t	"+target)
	if enterCount != 2 || captureCount != 2 || legacyKeys != 0 {
		t.Fatalf("submit counts enter=%d capture=%d legacy=%d, want enter=2 capture=2 legacy=0\nlog:\n%s", enterCount, captureCount, legacyKeys, raw)
	}
}

func TestTmuxPaneShowsAgentWorking(t *testing.T) {
	idle := strings.Join([]string{
		"❯ prompt still sitting unsent in the input box",
		"────────────────────────────────",
		"  ⏵⏵ bypass permissions on (shift+tab to cycle)",
	}, "\n")
	if tmuxPaneShowsAgentWorking(idle) {
		t.Fatalf("idle input box was misread as an actively working turn")
	}

	working := strings.Join([]string{
		"  Working through the brief…",
		"  ⏵⏵ bypass permissions on · esc to interrupt · ← for history",
	}, "\n")
	if !tmuxPaneShowsAgentWorking(working) {
		t.Fatalf("active turn (esc to interrupt) was not detected as working")
	}
}

func TestRealTmuxHarnessClientCapturePaneJoinsWrappedChroteOutputs(t *testing.T) {
	fakeDir := t.TempDir()
	logPath := filepath.Join(fakeDir, "tmux.log")
	joinedPath := filepath.Join(fakeDir, "joined.txt")
	wrappedPath := filepath.Join(fakeDir, "wrapped.txt")
	fakeTmuxPath := filepath.Join(fakeDir, "tmux")

	fence := "```"
	joined := "done thinking\n" + fence + "chrote-outputs\n" +
		`{"port_solo_out":{"text":"SOLO-REAL-ANSWER=399 padded so this json line is wider than a normal terminal pane"}}` + "\n" +
		fence + "\n<<<CHROTE-DONE run-id=run_o25k status=ok artifact=solo.md>>>\n"
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

	fakeTmux := `#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
{
  printf 'ARGS'
  for arg in "$@"; do
    printf '\t%s' "$arg"
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
	if !strings.Contains(payload.Text, "SOLO-REAL-ANSWER=399") || strings.Contains(payload.Text, "\n") {
		t.Fatalf("captured payload text = %q, want model answer intact and unwrapped", payload.Text)
	}

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake tmux log: %v", err)
	}
	if !strings.Contains(string(raw), "\t-J") {
		t.Fatalf("capture-pane did not request -J to join wrapped lines:\n%s", raw)
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
	return runTmuxFormationForTestWithConfig(t, client, board, tmuxTestConfig(t))
}

func runTmuxFormationForTestWithConfig(t *testing.T, client *fakeTmuxHarnessClient, board string, cfg TmuxExecutorConfig) (*RunStatusProjection, []RunEvent) {
	t.Helper()
	store, personas := s4RunFixture(t)
	store.Now = fixedClock()
	personas.Now = fixedClock()
	createS4Persona(t, personas, "scout")
	if board == "" {
		board = s4RunBoardFixture()
	}
	writeFixture(t, store.BoardPath("session-search"), board)
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
	socket := filepath.Join(root, "tmux.sock")
	if err := os.WriteFile(socket, []byte("disposable tmux socket identity fixture"), 0o600); err != nil {
		t.Fatalf("write disposable tmux socket fixture: %v", err)
	}
	return TmuxExecutorConfig{
		Harnesses:      []string{"openai-codex"},
		Socket:         socket,
		Cwd:            root,
		Roots:          []string{root},
		SessionPrefix:  "tmux-",
		OutputCapBytes: defaultTmuxOutputCapBytes,
		TimeoutSeconds: 1,
	}
}

// nonTempWorkspace builds a production-style config whose dedicated socket and
// workspace cwd/roots live OUTSIDE /tmp, mirroring the /srv + /run production
// layout. The base directory is created under the package directory (which is
// never /tmp) and removed on cleanup.
func nonTempWorkspace(t *testing.T) TmuxExecutorConfig {
	t.Helper()
	packageRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("get package root: %v", err)
	}
	base, err := os.MkdirTemp(packageRoot, "chrote-prod-")
	if err != nil {
		t.Fatalf("create non-temp workspace base: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(base); err != nil {
			t.Errorf("remove non-temp workspace base: %v", err)
		}
	})
	workspace := filepath.Join(base, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatalf("create non-temp workspace: %v", err)
	}
	socketDir := filepath.Join(base, "formations-tmux")
	if err := os.MkdirAll(socketDir, 0o700); err != nil {
		t.Fatalf("create non-temp socket dir: %v", err)
	}
	socket := filepath.Join(socketDir, "default")
	if err := os.WriteFile(socket, []byte("dedicated formations tmux socket fixture"), 0o600); err != nil {
		t.Fatalf("write non-temp socket fixture: %v", err)
	}
	return TmuxExecutorConfig{
		Harnesses:      []string{"openai-codex"},
		Socket:         socket,
		Cwd:            workspace,
		Roots:          []string{workspace},
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
		"CHROTE_FORMATIONS_TMUX_DEDICATED",
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

type fakeTmuxOp struct {
	op     string
	target string
}

type fakeTmuxHarnessClient struct {
	sessions             []string
	pane                 tmuxPaneState
	noServer             bool
	hasServerErr         error
	startKeeperErr       error
	keepersStarted       []string
	listErr              error
	describeErr          error
	sendErr              error
	captureErr           error
	createErr            error
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
	listCalls            int
	describeCalls        int
	created              []string
	killed               []string
	ops                  []fakeTmuxOp
	afterCapture         func(call int)
}

// liveSessions models the sessions currently visible on the socket: the
// pre-existing (foreign) fixtures plus every session the executor created and
// has not yet torn down.
func (f *fakeTmuxHarnessClient) liveSessions() []string {
	killed := make(map[string]int, len(f.killed))
	for _, name := range f.killed {
		killed[name]++
	}
	live := append([]string(nil), f.sessions...)
	for _, name := range f.created {
		if killed[name] > 0 {
			killed[name]--
			continue
		}
		live = append(live, name)
	}
	return live
}

func (f *fakeTmuxHarnessClient) HasServer(_ context.Context, _ string) (bool, error) {
	// Probe op carries no session target so safety assertions that scan ops for
	// foreign-target touches skip it.
	f.ops = append(f.ops, fakeTmuxOp{op: "has-server"})
	if f.hasServerErr != nil {
		return false, f.hasServerErr
	}
	return !f.noServer, nil
}

func (f *fakeTmuxHarnessClient) StartKeeper(_ context.Context, socket, keeper string) error {
	f.ops = append(f.ops, fakeTmuxOp{op: "start-keeper", target: keeper})
	if f.startKeeperErr != nil {
		return f.startKeeperErr
	}
	// Model tmux materializing the socket file when it starts the server, so the
	// executor's subsequent pinTmuxSocketIdentity (a real os.Lstat) succeeds.
	if strings.TrimSpace(socket) != "" {
		_ = os.WriteFile(socket, []byte("lazy-started tmux socket identity fixture"), 0o600)
	}
	f.keepersStarted = append(f.keepersStarted, keeper)
	f.sessions = append(f.sessions, keeper)
	f.noServer = false
	return nil
}

func (f *fakeTmuxHarnessClient) ListSessions(context.Context, string) ([]string, error) {
	f.listCalls++
	f.ops = append(f.ops, fakeTmuxOp{op: "list-sessions"})
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.liveSessions(), nil
}

func (f *fakeTmuxHarnessClient) CreateSession(_ context.Context, _, name, _, _ string) error {
	f.ops = append(f.ops, fakeTmuxOp{op: "new-session", target: name})
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, name)
	return nil
}

func (f *fakeTmuxHarnessClient) KillSession(_ context.Context, _, name string) error {
	f.ops = append(f.ops, fakeTmuxOp{op: "kill-session", target: name})
	f.killed = append(f.killed, name)
	return nil
}

func (f *fakeTmuxHarnessClient) DescribeActivePane(_ context.Context, _, target string) (tmuxPaneState, error) {
	f.describeCalls++
	f.ops = append(f.ops, fakeTmuxOp{op: "describe", target: target})
	if f.describeErr != nil {
		return tmuxPaneState{}, f.describeErr
	}
	return f.pane, nil
}

func (f *fakeTmuxHarnessClient) SendPrompt(_ context.Context, _, target, dispatchID, prompt string) error {
	f.sendCalls++
	f.ops = append(f.ops, fakeTmuxOp{op: "send", target: target})
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

func (f *fakeTmuxHarnessClient) CapturePane(_ context.Context, _, target string, _ int) (string, error) {
	f.captureCalls++
	f.ops = append(f.ops, fakeTmuxOp{op: "capture", target: target})
	if f.afterCapture != nil {
		f.afterCapture(f.captureCalls)
	}
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
	seen := map[string]bool{}
	add := func(port string) {
		port = strings.TrimSpace(port)
		if port == "" || seen[port] {
			return
		}
		seen[port] = true
		ports = append(ports, port)
	}
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
				add(fields[0])
			}
			continue
		case inList && trimmed != "":
			inList = false
		}
		for cursor := 0; ; {
			idx := strings.Index(trimmed[cursor:], "\"port_")
			if idx < 0 {
				break
			}
			start := cursor + idx + 1
			endRel := strings.Index(trimmed[start:], "\"")
			if endRel < 0 {
				break
			}
			add(trimmed[start : start+endRel])
			cursor = start + endRel + 1
			if cursor >= len(trimmed) {
				break
			}
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
