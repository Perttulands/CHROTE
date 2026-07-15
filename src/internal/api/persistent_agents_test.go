package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	persistentTestCodexID  = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	persistentTestClaudeID = "9ed1181c-b2a3-4ef2-96ea-a84e51e79dc4"
	persistentTestHermesID = "hermes-session-20260715T100000Z"
)

func TestTmuxHandler_EnablePersistentAgentStoresCanonicalExplicitDescriptors(t *testing.T) {
	tests := []struct {
		name        string
		kind        string
		sessionID   string
		profile     string
		paneCommand string
		args        string
		wantCommand string
	}{
		{
			name:        "codex",
			kind:        RecoveryAgentCodex,
			sessionID:   persistentTestCodexID,
			paneCommand: "node",
			args:        "node /usr/bin/codex resume --no-alt-screen " + persistentTestCodexID,
			wantCommand: "codex resume " + persistentTestCodexID,
		},
		{
			name:        "claude",
			kind:        RecoveryAgentClaude,
			sessionID:   persistentTestClaudeID,
			paneCommand: "claude",
			args:        "claude --resume " + persistentTestClaudeID,
			wantCommand: "claude --resume " + persistentTestClaudeID,
		},
		{
			name:        "hermes",
			kind:        RecoveryAgentHermes,
			sessionID:   persistentTestHermesID,
			profile:     "scout",
			paneCommand: "python",
			args:        "/home/alice/.hermes/hermes-agent-current/venv/bin/python -m hermes_cli.main --profile scout --resume " + persistentTestHermesID + " --tui --yolo",
			wantCommand: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
			installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:`+tt.paneCommand+`:/home/alice/project\n' ;;
esac
`)
			t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
			t.Setenv("CHROTE_TERMINAL_USERS", "alice")
			t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
			t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
			t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
			installProcessTable(t, []processInfo{{pid: "42", ppid: "1", comm: tt.paneCommand, args: tt.args}})

			handler := NewTmuxHandler()
			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)
			body := persistentAgentRequestJSON(t, map[string]any{
				"identity":           "Maintains exact identity.",
				"recoveryDescriptor": persistentAgentTestDescriptor("codex-alpha", "alice", tt.kind, tt.sessionID, tt.profile),
			})
			req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			mux.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
			}
			var response map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response["agentKind"] != tt.kind || response["agentSessionId"] != tt.sessionID || response["resumeCommand"] != tt.wantCommand {
				t.Fatalf("persistent response = %#v", response)
			}
			rawEntries := readPersistentAgentRawEntries(t, persistentPath)
			desc, ok := rawEntries[0]["recoveryDescriptor"].(map[string]any)
			if !ok {
				t.Fatalf("stored entry missing recoveryDescriptor: %#v", rawEntries[0])
			}
			agent, ok := desc["agent"].(map[string]any)
			if !ok {
				t.Fatalf("stored descriptor missing agent: %#v", desc)
			}
			owner, ok := desc["owner"].(map[string]any)
			if !ok {
				t.Fatalf("stored descriptor missing owner: %#v", desc)
			}
			if desc["mode"] != RecoveryModeAgent || owner["kind"] != RecoveryOwnerPersistentAgent || owner["ref"] != "persistent:alice/codex-alpha" || owner["mayRestart"] != true {
				t.Fatalf("stored descriptor owner/mode = %#v", desc)
			}
			if agent["kind"] != tt.kind || agent["nativeSessionId"] != tt.sessionID {
				t.Fatalf("stored descriptor agent = %#v", agent)
			}
			if tt.kind == RecoveryAgentHermes && agent["hermesProfile"] != tt.profile {
				t.Fatalf("stored hermes profile = %#v, want %q", agent["hermesProfile"], tt.profile)
			}
		})
	}
}

func TestTmuxHandler_EnablePersistentAgentInfersHermesDescriptorFromProductionArgv(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:python:/home/alice/project\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/workspace/not-home")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	installProcessTable(t, []processInfo{{
		pid:  "42",
		ppid: "1",
		comm: "python",
		args: "/home/alice/.hermes/hermes-agent-current/venv/bin/python -m hermes_cli.main --profile scout --resume " + persistentTestHermesID + " --tui --yolo",
	}})

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/hermes-scout/persistence?unixUser=alice", bytes.NewBufferString(`{"identity":"Keeps Hermes scout alive."}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	wantCommand := ""
	if response["agentKind"] != RecoveryAgentHermes || response["agentSessionId"] != persistentTestHermesID || response["resumeCommand"] != wantCommand {
		t.Fatalf("persistent response = %#v", response)
	}
	rawEntries := readPersistentAgentRawEntries(t, persistentPath)
	desc, ok := rawEntries[0]["recoveryDescriptor"].(map[string]any)
	if !ok {
		t.Fatalf("stored entry missing recoveryDescriptor: %#v", rawEntries[0])
	}
	agent, ok := desc["agent"].(map[string]any)
	if !ok {
		t.Fatalf("stored descriptor missing agent: %#v", desc)
	}
	if agent["kind"] != RecoveryAgentHermes || agent["nativeSessionId"] != persistentTestHermesID || agent["hermesProfile"] != "scout" {
		t.Fatalf("stored hermes descriptor agent = %#v", agent)
	}
}

func TestTmuxHandler_EnablePersistentAgentRejectsPlainHermesWithoutUniqueResumeID(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:python:/home/alice/project\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	installProcessTable(t, []processInfo{{
		pid:  "42",
		ppid: "1",
		comm: "python",
		args: "/home/alice/.hermes/hermes-agent-current/venv/bin/python -m hermes_cli.main --profile scout --tui --yolo",
	}})

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/hermes-scout/persistence?unixUser=alice", bytes.NewBufferString(`{"identity":"Keeps Hermes scout alive."}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if !strings.Contains(strings.ToLower(recorder.Body.String()), "hermes resume id") {
		t.Fatalf("error body = %s, want precise Hermes resume id failure", recorder.Body.String())
	}
	if raw, err := os.ReadFile(persistentPath); err == nil && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("persistent store should be empty, got %s", raw)
	}
	got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	want := [][]string{{"-S", "/tmp/tmux-a", "display-message", "-p", "-t", "hermes-scout", "#{pane_pid}:#{pane_current_command}:#{pane_current_path}"}}
	if !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want only pane inspection %#v", got, want)
	}
}

func TestPersistentAgentProcessTreeRejectsMultipleIdentifiedAgentDescendants(t *testing.T) {
	infos := []processInfo{
		{pid: "42", ppid: "1", comm: "bash", args: "bash"},
		{pid: "50", ppid: "42", comm: "node", args: "node /usr/bin/codex resume " + persistentTestCodexID},
		{pid: "51", ppid: "42", comm: "claude", args: "claude --resume " + persistentTestClaudeID},
		{pid: "52", ppid: "42", comm: "python", args: "/home/alice/.hermes/hermes-agent-current/venv/bin/python -m hermes_cli.main --profile scout --resume " + persistentTestHermesID + " --tui --yolo"},
	}

	_, foundAgent, foundSessionID, err := inferPersistentAgentMetadataInTableForOwner(infos, "42", "", "/home/alice")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "multiple") {
		t.Fatalf("error = %v, want multiple identified agent identities", err)
	}
	if !foundAgent || foundSessionID {
		t.Fatalf("foundAgent=%v foundSessionID=%v, want ambiguous live agent without selected identity", foundAgent, foundSessionID)
	}
}

func TestPersistentAgentProcessTreeDedupesSameIdentityAcrossWrapperAndChild(t *testing.T) {
	infos := []processInfo{
		{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex resume " + persistentTestCodexID},
		{pid: "50", ppid: "42", comm: "node", args: "node /usr/bin/codex resume --no-alt-screen " + persistentTestCodexID},
	}

	metadata, foundAgent, foundSessionID, err := inferPersistentAgentMetadataInTableForOwner(infos, "42", "", "/home/alice")
	if err != nil {
		t.Fatalf("infer process tree metadata: %v", err)
	}
	if !foundAgent || !foundSessionID || metadata.Kind != RecoveryAgentCodex || metadata.SessionID != persistentTestCodexID {
		t.Fatalf("metadata=%+v foundAgent=%v foundSessionID=%v, want one de-duplicated Codex identity", metadata, foundAgent, foundSessionID)
	}
}

func TestTmuxHandler_EnablePersistentAgentExplicitCodexRequiresOwnerProbeWhenArgvLacksSessionID(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:node:/home/alice/project\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	installProcessTable(t, []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex --no-alt-screen"}})
	originalProbe := probePersistentAgentOwnerMetadata
	probeCalls := 0
	probePersistentAgentOwnerMetadata = func(ctx context.Context, h *TmuxHandler, target tmuxTarget, pane paneInspection, requestedKind string) (inferredPersistentAgentMetadata, error) {
		probeCalls++
		if target.unixUser != "alice" || target.ownerHome != "/home/alice" || pane.PID != "42" || requestedKind != RecoveryAgentCodex {
			t.Fatalf("probe args target=%+v pane=%+v requestedKind=%q", target, pane, requestedKind)
		}
		return inferredPersistentAgentMetadata{Kind: RecoveryAgentCodex, SessionID: persistentTestCodexID, Source: "owner-probe", Confidence: RecoveryConfidenceHigh}, nil
	}
	t.Cleanup(func() { probePersistentAgentOwnerMetadata = originalProbe })

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := persistentAgentRequestJSON(t, map[string]any{
		"recoveryDescriptor": persistentAgentTestDescriptor("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if probeCalls != 1 {
		t.Fatalf("owner probe calls = %d, want 1 to prove exact explicit Codex identity", probeCalls)
	}
}

func TestTmuxHandler_EnablePersistentAgentRejectsAmbiguousProcessTreeBeforeStoreOrRename(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:node:/home/alice/project\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	installProcessTable(t, []processInfo{
		{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex resume " + persistentTestCodexID},
		{pid: "51", ppid: "42", comm: "claude", args: "claude --resume " + persistentTestClaudeID},
	})

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := persistentAgentRequestJSON(t, map[string]any{
		"newName":            "codex-persist",
		"recoveryDescriptor": persistentAgentTestDescriptor("codex-persist", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if !strings.Contains(strings.ToLower(recorder.Body.String()), "multiple") {
		t.Fatalf("error body = %s, want process-tree ambiguity", recorder.Body.String())
	}
	if raw, err := os.ReadFile(persistentPath); err == nil && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("persistent store should be empty, got %s", raw)
	}
	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	for _, call := range calls {
		if containsArg(call, "rename-session") || containsArg(call, "new-session") {
			t.Fatalf("tmux mutation before ambiguous identity rejection: %#v", calls)
		}
	}
}

func TestTmuxHandler_EnablePersistentAgentExplicitCodexRejectsOwnerProbeMismatchBeforeStoreOrRename(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:node:/home/alice/project\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	installProcessTable(t, []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex --no-alt-screen"}})
	originalProbe := probePersistentAgentOwnerMetadata
	probePersistentAgentOwnerMetadata = func(context.Context, *TmuxHandler, tmuxTarget, paneInspection, string) (inferredPersistentAgentMetadata, error) {
		return inferredPersistentAgentMetadata{Kind: RecoveryAgentCodex, SessionID: "11111111-2222-4333-8444-555555555555", Source: "owner-probe", Confidence: RecoveryConfidenceHigh}, nil
	}
	t.Cleanup(func() { probePersistentAgentOwnerMetadata = originalProbe })

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := persistentAgentRequestJSON(t, map[string]any{
		"newName":            "codex-persist",
		"recoveryDescriptor": persistentAgentTestDescriptor("codex-persist", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if !strings.Contains(strings.ToLower(recorder.Body.String()), "wrong identity") {
		t.Fatalf("error body = %s, want wrong identity", recorder.Body.String())
	}
	if raw, err := os.ReadFile(persistentPath); err == nil && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("persistent store should be empty, got %s", raw)
	}
	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	for _, call := range calls {
		if containsArg(call, "rename-session") || containsArg(call, "new-session") {
			t.Fatalf("tmux mutation before exact identity proof: %#v", calls)
		}
	}
}

func TestParsePersistentAgentOwnerProbeOutputDedupesDuplicatesAndRejectsConflicts(t *testing.T) {
	duplicate := persistentAgentOwnerProbeResultPrefix + `{"kind":"codex","sessionId":"` + persistentTestCodexID + `","confidence":"high"}` + "\n" +
		persistentAgentOwnerProbeResultPrefix + `{"kind":"codex","sessionId":"` + persistentTestCodexID + `","confidence":"high"}`
	metadata, err := parsePersistentAgentOwnerProbeOutput(duplicate)
	if err != nil {
		t.Fatalf("parse duplicate owner probe output: %v", err)
	}
	if metadata.Kind != RecoveryAgentCodex || metadata.SessionID != persistentTestCodexID {
		t.Fatalf("metadata = %+v, want one de-duplicated Codex candidate", metadata)
	}

	conflicting := persistentAgentOwnerProbeResultPrefix + `{"kind":"codex","sessionId":"` + persistentTestCodexID + `","confidence":"high"}` + "\n" +
		persistentAgentOwnerProbeResultPrefix + `{"kind":"codex","sessionId":"11111111-2222-4333-8444-555555555555","confidence":"high"}`
	if _, err := parsePersistentAgentOwnerProbeOutput(conflicting); err == nil || !strings.Contains(strings.ToLower(err.Error()), "multiple") {
		t.Fatalf("conflicting owner probe error = %v, want multiple-candidate rejection", err)
	}
}

func TestTmuxHandler_EnablePersistentAgentRejectsSessionBankOwnedPlanBeforeSideEffects(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installPersistentAgentScriptedTmux(t, "")
	plan := []WorkloadRecoveryDescriptor{
		sessionBankAgentDescriptor("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, "", 0, 0, "%1", "agents", "b25f,80x24,0,0", "/home/alice/project"),
	}
	writeBankSeedRaw(t, bankPath, sessionBankEntryWithRecoveryPlanJSON(t, "codex-alpha", "alice", 1, plan))
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := persistentAgentRequestJSON(t, map[string]any{
		"recoveryDescriptor": persistentAgentTestDescriptor("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if !strings.Contains(strings.ToLower(recorder.Body.String()), "session bank") {
		t.Fatalf("error body = %s, want Session Bank ownership conflict", recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want no tmux side effects before ownership conflict", got)
	}
	if raw, err := os.ReadFile(persistentPath); err == nil && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("persistent store should be empty, got %s", raw)
	}
}

func TestTmuxHandler_EnablePersistentAgentRejectsSourceSessionBankOwnershipBeforeRename(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:node:/home/alice/project\n' ;;
esac
`)
	plan := []WorkloadRecoveryDescriptor{
		sessionBankAgentDescriptor("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, "", 0, 0, "%1", "agents", "b25f,80x24,0,0", "/home/alice/project"),
	}
	writeBankSeedRaw(t, bankPath, sessionBankEntryWithRecoveryPlanJSON(t, "codex-alpha", "alice", 1, plan))
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	installProcessTable(t, []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex resume " + persistentTestCodexID}})

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := persistentAgentRequestJSON(t, map[string]any{
		"newName":            "codex-persist",
		"recoveryDescriptor": persistentAgentTestDescriptor("codex-persist", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if !strings.Contains(strings.ToLower(recorder.Body.String()), "session bank") {
		t.Fatalf("error body = %s, want source Session Bank ownership conflict", recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want no tmux side effects before source ownership conflict", got)
	}
	if raw, err := os.ReadFile(persistentPath); err == nil && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("persistent store should be empty, got %s", raw)
	}
}

func TestTmuxHandler_EnablePersistentAgentRejectsPersistentTargetCollisionBeforeEffects(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:node:/home/alice/project\n' ;;
esac
`)
	existingID := "11111111-2222-4333-8444-555555555555"
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
		persistentAgentRawEntry("codex-persist", "alice", RecoveryAgentCodex, existingID, ""),
	})
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	installProcessTable(t, []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex resume " + persistentTestCodexID}})

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := persistentAgentRequestJSON(t, map[string]any{
		"newName":            "codex-persist",
		"identity":           "new target owner",
		"recoveryDescriptor": persistentAgentTestDescriptor("codex-persist", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if !strings.Contains(strings.ToLower(recorder.Body.String()), "persistent") {
		t.Fatalf("error body = %s, want persistent target collision", recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want no tmux side effects before persistent target collision", got)
	}
	rawEntries := readPersistentAgentRawEntries(t, persistentPath)
	if len(rawEntries) != 1 || rawEntries[0]["name"] != "codex-persist" || rawEntries[0]["agentSessionId"] != existingID || rawEntries[0]["identity"] != "Maintains exact identity." {
		t.Fatalf("persistent store mutated on target collision: %#v", rawEntries)
	}
}

func TestPersistentAgentStoreRenameRejectsExistingTarget(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	targetID := "11111111-2222-4333-8444-555555555555"
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
		persistentAgentRawEntry("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
		persistentAgentRawEntry("codex-target", "alice", RecoveryAgentCodex, targetID, ""),
	})
	store := newPersistentAgentStore(persistentPath)

	err := store.Rename("codex-alpha", "codex-target", "alice")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "exists") {
		t.Fatalf("rename error = %v, want existing target rejection", err)
	}
	rawEntries := readPersistentAgentRawEntries(t, persistentPath)
	if len(rawEntries) != 2 || rawEntries[0]["name"] != "codex-alpha" || rawEntries[1]["name"] != "codex-target" || rawEntries[1]["agentSessionId"] != targetID {
		t.Fatalf("persistent entries mutated after rejected rename: %#v", rawEntries)
	}
}

func TestTmuxHandler_EnablePersistentAgentRejectsPersistentSourceRenameBeforeEffects(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:node:/home/alice/project\n' ;;
esac
`)
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
		persistentAgentRawEntry("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	before, err := os.ReadFile(persistentPath)
	if err != nil {
		t.Fatalf("read persistent seed: %v", err)
	}
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	installProcessTable(t, []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex resume " + persistentTestCodexID}})

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := persistentAgentRequestJSON(t, map[string]any{
		"newName":            "codex-renamed",
		"recoveryDescriptor": persistentAgentTestDescriptor("codex-renamed", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want no tmux side effects for persistent source rename through Enable", got)
	}
	after, err := os.ReadFile(persistentPath)
	if err != nil {
		t.Fatalf("read persistent after rejection: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("persistent store mutated on rejected Enable rename:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestTmuxHandler_EnablePersistentAgentRejectsOversizedAndMultiJSONBodiesBeforeTmux(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{
			name:       "over limit valid prefix with trailing data",
			body:       `{"identity":"valid-prefix"}` + strings.Repeat(" ", 256*1024+1) + `x`,
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "second json value",
			body:       `{"identity":"one"} {"identity":"two"}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
			argsPath := installPersistentAgentScriptedTmux(t, "")
			t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
			t.Setenv("CHROTE_TERMINAL_USERS", "alice")
			t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
			t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
			t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

			handler := NewTmuxHandler()
			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)
			req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			mux.ServeHTTP(recorder, req)
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
				t.Fatalf("tmux calls = %#v, want body rejection before tmux inspection", got)
			}
			if raw, err := os.ReadFile(persistentPath); err == nil && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "[]" {
				t.Fatalf("persistent store should be empty, got %s", raw)
			}
		})
	}
}

func TestTmuxHandler_EnablePersistentAgentRejectsExternalManagedDescriptorBeforeSideEffects(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, "")
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	managed := persistentAgentTestDescriptor("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, "")
	managed.Mode = RecoveryModeManaged
	managed.Owner = WorkloadRecoveryOwner{Kind: RecoveryOwnerExternalManager, Ref: "systemd:user/hermes.service", MayRestart: false}
	managed.Agent = nil
	managed.WorkloadKind = RecoveryWorkloadManaged
	managed.EvidenceSource = RecoveryEvidenceManager
	body := persistentAgentRequestJSON(t, map[string]any{"recoveryDescriptor": managed})

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if !strings.Contains(strings.ToLower(recorder.Body.String()), "managed") {
		t.Fatalf("error body = %s, want managed descriptor rejection", recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want no tmux side effects before managed rejection", got)
	}
}

func TestTmuxHandler_ReconcilePersistentAgentsRejectsAmbiguousProcessTreeWithoutRestart(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *has-session*) exit 0 ;;
  *display-message*) printf '42:node:/home/alice/project\n' ;;
  *capture-pane*) printf 'Codex is ready\n' ;;
  *kill-session*|*new-session*) echo 'unexpected destructive reconcile' >&2; exit 1 ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
		persistentAgentRawEntry("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	installProcessTable(t, []processInfo{
		{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex resume " + persistentTestCodexID},
		{pid: "51", ppid: "42", comm: "claude", args: "claude --resume " + persistentTestClaudeID},
	})

	handler := NewTmuxHandler()
	results, err := handler.ReconcilePersistentAgents(context.Background())
	if err != nil {
		t.Fatalf("reconcile persistent agents: %v", err)
	}
	if len(results) != 1 || results[0].Action != PersistentAgentStateWrongIdentity || !strings.Contains(strings.ToLower(results[0].Error), "multiple") {
		t.Fatalf("reconcile results = %+v, want wrong_identity ambiguity", results)
	}
	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	for _, call := range calls {
		if containsArg(call, "kill-session") || containsArg(call, "new-session") {
			t.Fatalf("reconcile attempted recovery for ambiguous identity: %#v", calls)
		}
	}
}

func TestTmuxHandler_ReconcilePersistentAgentsDoesNotReviveWhenKillSessionFails(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *has-session*) exit 0 ;;
  *display-message*) printf '42:bash:/home/alice/project\n' ;;
  *capture-pane*) printf 'shell prompt\n' ;;
  *kill-session*) echo 'permission denied' >&2; exit 1 ;;
  *new-session*) echo 'unexpected revive after kill failure' >&2; exit 1 ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
		persistentAgentRawEntry("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	installProcessTable(t, nil)

	handler := NewTmuxHandler()
	results, err := handler.ReconcilePersistentAgents(context.Background())
	if err != nil {
		t.Fatalf("reconcile persistent agents: %v", err)
	}
	if len(results) != 1 || results[0].Action != "error" || !strings.Contains(strings.ToLower(results[0].Error), "permission") {
		t.Fatalf("reconcile results = %+v, want non-destructive kill error", results)
	}
	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	for _, call := range calls {
		if containsArg(call, "new-session") {
			t.Fatalf("reconcile revived after kill-session failure: %#v", calls)
		}
	}
	rawEntries := readPersistentAgentRawEntries(t, persistentPath)
	if rawEntries[0]["state"] == PersistentAgentStateBackoff || rawEntries[0]["consecutiveLaunchFailures"] != nil {
		t.Fatalf("kill error should not count as launch failure: %#v", rawEntries[0])
	}
}

func TestTmuxHandler_ReconcilePersistentAgentsDoesNotRestartOnTmuxTransportError(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *has-session*) echo 'error connecting to /tmp/tmux-a (Permission denied)' >&2; exit 1 ;;
  *new-session*|*kill-session*) echo 'unexpected destructive reconcile' >&2; exit 1 ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
		persistentAgentRawEntry("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})

	handler := NewTmuxHandler()
	results, err := handler.ReconcilePersistentAgents(context.Background())
	if err != nil {
		t.Fatalf("reconcile persistent agents: %v", err)
	}
	if len(results) != 1 || results[0].Action != "error" || !strings.Contains(strings.ToLower(results[0].Error), "permission") {
		t.Fatalf("reconcile results = %+v, want non-destructive transport error", results)
	}
	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	if len(calls) != 1 || !containsArg(calls[0], "has-session") {
		t.Fatalf("tmux calls = %#v, want only has-session transport check", calls)
	}
	rawEntries := readPersistentAgentRawEntries(t, persistentPath)
	if rawEntries[0]["state"] == PersistentAgentStateBackoff || rawEntries[0]["state"] == PersistentAgentStateFailed || rawEntries[0]["consecutiveLaunchFailures"] != nil || rawEntries[0]["nextRetryAt"] != nil {
		t.Fatalf("transport error should not count as launch failure: %#v", rawEntries[0])
	}
}

func TestPersistentAgentStoreRejectsDuplicateAndCorruptRecordsWithoutMutation(t *testing.T) {
	tests := []struct {
		name    string
		entries []map[string]any
		want    string
	}{
		{
			name: "duplicate key",
			entries: []map[string]any{
				persistentAgentRawEntry("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
				persistentAgentRawEntry("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
			},
			want: "duplicate",
		},
		{
			name: "corrupt legacy record",
			entries: []map[string]any{{
				"name":           "bad name",
				"unixUser":       "alice",
				"agentKind":      RecoveryAgentCodex,
				"agentSessionId": persistentTestCodexID,
				"resumeCommand":  "codex resume " + persistentTestCodexID,
				"createdAt":      "2026-07-15T00:00:00Z",
				"updatedAt":      "2026-07-15T00:00:00Z",
			}},
			want: "record 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
			argsPath := installPersistentAgentScriptedTmux(t, "")
			writePersistentAgentRawSeed(t, persistentPath, tt.entries)
			before, err := os.ReadFile(persistentPath)
			if err != nil {
				t.Fatalf("read persistent seed: %v", err)
			}
			t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
			t.Setenv("CHROTE_TERMINAL_USERS", "alice")
			t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
			t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
			t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

			handler := NewTmuxHandler()
			if _, err := handler.ReconcilePersistentAgents(context.Background()); err == nil || !strings.Contains(strings.ToLower(err.Error()), tt.want) {
				t.Fatalf("reconcile error = %v, want %q", err, tt.want)
			}
			if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
				t.Fatalf("tmux calls = %#v, want no reconcile side effects for invalid store", got)
			}
			after, err := os.ReadFile(persistentPath)
			if err != nil {
				t.Fatalf("read persistent after rejected reconcile: %v", err)
			}
			if !bytes.Equal(after, before) {
				t.Fatalf("persistent store mutated after invalid load:\nbefore=%s\nafter=%s", before, after)
			}
		})
	}
}

func TestPersistentAgentStorePreservesValidLegacyCodexMigration(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{{
		"name":           "codex-alpha",
		"unixUser":       "alice",
		"agentKind":      RecoveryAgentCodex,
		"agentSessionId": persistentTestCodexID,
		"resumeCommand":  "rm -rf /",
		"cwd":            "/home/alice/project",
		"createdAt":      "2026-07-15T00:00:00Z",
		"updatedAt":      "2026-07-15T00:00:00Z",
	}})

	entries, err := newPersistentAgentStore(persistentPath).Read()
	if err != nil {
		t.Fatalf("read valid legacy persistent record: %v", err)
	}
	if len(entries) != 1 || entries[0].ResumeCommand != "codex resume "+persistentTestCodexID {
		t.Fatalf("entries = %+v, want canonicalized legacy Codex record", entries)
	}
}

func TestTmuxHandler_ReconcilePersistentAgentsRequiresExactIdentity(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *has-session*) exit 0 ;;
  *display-message*) printf '42:node:/home/alice/project\n' ;;
  *capture-pane*) printf 'Codex is ready\n' ;;
  *kill-session*|*new-session*) echo 'unexpected destructive reconcile' >&2; exit 1 ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
		persistentAgentRawEntry("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	installProcessTable(t, []processInfo{{
		pid:  "42",
		ppid: "1",
		comm: "node",
		args: "node /usr/bin/codex resume --no-alt-screen 11111111-2222-4333-8444-555555555555",
	}})

	handler := NewTmuxHandler()
	results, err := handler.ReconcilePersistentAgents(context.Background())
	if err != nil {
		t.Fatalf("reconcile persistent agents: %v", err)
	}
	if len(results) != 1 || results[0].Action != "wrong_identity" || !strings.Contains(strings.ToLower(results[0].Error), "wrong identity") {
		t.Fatalf("reconcile results = %+v, want wrong_identity", results)
	}
	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	for _, call := range calls {
		if containsArg(call, "kill-session") || containsArg(call, "new-session") {
			t.Fatalf("reconcile attempted destructive recovery for wrong identity: %#v", calls)
		}
	}
}

func TestTmuxHandler_EnablePersistentAgentRejectsExplicitDescriptorCWDDisagreement(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:node:/home/alice/project\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	installProcessTable(t, []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex resume " + persistentTestCodexID}})

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := persistentAgentRequestJSON(t, map[string]any{
		"cwd":                "/home/alice/other",
		"recoveryDescriptor": persistentAgentTestDescriptor("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if !strings.Contains(strings.ToLower(recorder.Body.String()), "cwd") && !strings.Contains(strings.ToLower(recorder.Body.String()), "working directory") {
		t.Fatalf("error body = %s, want CWD disagreement", recorder.Body.String())
	}
	if raw, err := os.ReadFile(persistentPath); err == nil && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("persistent store should be empty, got %s", raw)
	}
	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	for _, call := range calls {
		if containsArg(call, "rename-session") || containsArg(call, "new-session") {
			t.Fatalf("tmux mutation after explicit CWD disagreement: %#v", calls)
		}
	}
}

func TestTmuxHandler_EnablePersistentAgentRejectsExplicitDescriptorOutsideOwnerHome(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:node:/srv/outside\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	installProcessTable(t, []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex resume " + persistentTestCodexID}})
	desc := persistentAgentTestDescriptor("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, "")
	desc.Topology.PaneCurrentPath = "/srv/outside"

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := persistentAgentRequestJSON(t, map[string]any{"recoveryDescriptor": desc})
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if !strings.Contains(strings.ToLower(recorder.Body.String()), "owner home") && !strings.Contains(strings.ToLower(recorder.Body.String()), "owner-home") {
		t.Fatalf("error body = %s, want owner-home containment failure", recorder.Body.String())
	}
	if raw, err := os.ReadFile(persistentPath); err == nil && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("persistent store should be empty, got %s", raw)
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want descriptor rejection before tmux inspection", got)
	}
}

func TestTmuxHandler_ReconcilePersistentAgentsRevivesFromDescriptorCWDNotLegacyEntryCWD(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *has-session*) echo 'no server running on /tmp/tmux-a' >&2; exit 1 ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/legacy")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	entry := persistentAgentRawEntry("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, "")
	entry["cwd"] = "/home/alice/legacy"
	desc := entry["recoveryDescriptor"].(WorkloadRecoveryDescriptor)
	desc.Topology.PaneCurrentPath = "/home/alice/project"
	entry["recoveryDescriptor"] = desc
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{entry})

	handler := NewTmuxHandler()
	results, err := handler.ReconcilePersistentAgents(context.Background())
	if err != nil {
		t.Fatalf("reconcile persistent agents: %v", err)
	}
	if len(results) != 1 || results[0].Action != "recreated" {
		t.Fatalf("reconcile results = %+v, want recreated", results)
	}
	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	for _, call := range calls {
		if !containsArg(call, "new-session") {
			continue
		}
		for i := 0; i+1 < len(call); i++ {
			if call[i] == "-c" {
				if call[i+1] != "/home/alice/project" {
					t.Fatalf("new-session cwd = %q, want descriptor topology cwd; call=%#v", call[i+1], call)
				}
				return
			}
		}
		t.Fatalf("new-session call missing -c cwd: %#v", call)
	}
	t.Fatalf("tmux calls missing new-session: %#v", calls)
}

func TestTmuxHandler_ReconcilePersistentAgentsTreatsProcessOnlyPresenceAsWrongIdentity(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *has-session*) exit 0 ;;
  *display-message*) printf '42:node:/home/alice/project\n' ;;
  *capture-pane*) printf 'Codex is ready\n' ;;
  *kill-session*|*new-session*) echo 'unexpected destructive reconcile' >&2; exit 1 ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
		persistentAgentRawEntry("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	installProcessTable(t, []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex --no-alt-screen"}})

	handler := NewTmuxHandler()
	results, err := handler.ReconcilePersistentAgents(context.Background())
	if err != nil {
		t.Fatalf("reconcile persistent agents: %v", err)
	}
	if len(results) != 1 || results[0].Action != "wrong_identity" || !strings.Contains(strings.ToLower(results[0].Error), "unknown identity") {
		t.Fatalf("reconcile results = %+v, want wrong_identity for process-only presence", results)
	}
	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	for _, call := range calls {
		if containsArg(call, "kill-session") || containsArg(call, "new-session") {
			t.Fatalf("reconcile attempted destructive recovery for process-only presence: %#v", calls)
		}
	}
}

func TestTmuxHandler_ReconcilePersistentAgentsBlocksInteractionPromptsWithoutRestart(t *testing.T) {
	tests := []struct {
		name string
		tail string
		want string
	}{
		{name: "update", tail: "Update available. Run codex update to continue.", want: "update"},
		{name: "hook approval", tail: "Hook approval required: allow this hook? [y/N]", want: "hook"},
		{name: "trust", tail: "Do you trust this workspace before running Codex?", want: "trust"},
		{name: "migration", tail: "First-run migration required before Hermes can continue.", want: "migration"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
			argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *has-session*) exit 0 ;;
  *display-message*) printf '42:bash:/home/alice/project\n' ;;
  *capture-pane*) printf '`+tt.tail+`\n' ;;
  *kill-session*|*new-session*) echo 'unexpected destructive reconcile' >&2; exit 1 ;;
esac
`)
			t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
			t.Setenv("CHROTE_TERMINAL_USERS", "alice")
			t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
			t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
			t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
			writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
				persistentAgentRawEntry("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
			})
			installProcessTable(t, nil)

			handler := NewTmuxHandler()
			results, err := handler.ReconcilePersistentAgents(context.Background())
			if err != nil {
				t.Fatalf("reconcile persistent agents: %v", err)
			}
			if len(results) != 1 || results[0].Action != "needs_interaction" || !strings.Contains(strings.ToLower(results[0].Error), tt.want) {
				t.Fatalf("reconcile results = %+v, want needs_interaction containing %q", results, tt.want)
			}
			calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
			for _, call := range calls {
				if containsArg(call, "kill-session") || containsArg(call, "new-session") || (containsArg(call, "send-keys") && containsArg(call, "Enter")) {
					t.Fatalf("reconcile attempted automatic recovery for interaction prompt: %#v", calls)
				}
			}
		})
	}
}

func TestTmuxHandler_ReconcilePersistentAgentsBackoffPreventsRepeatedLaunchFailuresAndResetClearsFailure(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *has-session*) echo 'no server running on /tmp/tmux-a' >&2; exit 1 ;;
  *new-session*) echo 'launch failed' >&2; exit 1 ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
		persistentAgentRawEntry("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})

	handler := NewTmuxHandler()
	results, err := handler.ReconcilePersistentAgents(context.Background())
	if err != nil {
		t.Fatalf("first reconcile persistent agents: %v", err)
	}
	if len(results) != 1 || results[0].Action != "backoff" || !strings.Contains(results[0].Error, "launch failed") {
		t.Fatalf("first reconcile results = %+v, want backoff launch failure", results)
	}
	firstCalls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	results, err = handler.ReconcilePersistentAgents(context.Background())
	if err != nil {
		t.Fatalf("second reconcile persistent agents: %v", err)
	}
	if len(results) != 1 || results[0].Action != "backoff" {
		t.Fatalf("second reconcile results = %+v, want backoff without retry", results)
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != len(firstCalls) {
		t.Fatalf("second reconcile made tmux calls during backoff: before=%#v after=%#v", firstCalls, got)
	}

	forcePersistentFailureRetry(t, persistentPath, 2)
	results, err = handler.ReconcilePersistentAgents(context.Background())
	if err != nil {
		t.Fatalf("failed-threshold reconcile persistent agents: %v", err)
	}
	if len(results) != 1 || results[0].Action != "failed" {
		t.Fatalf("failed-threshold reconcile results = %+v, want failed", results)
	}
	callsAtFailed := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	results, err = handler.ReconcilePersistentAgents(context.Background())
	if err != nil {
		t.Fatalf("post-failed reconcile persistent agents: %v", err)
	}
	if len(results) != 1 || results[0].Action != "failed" {
		t.Fatalf("post-failed reconcile results = %+v, want failed without retry", results)
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != len(callsAtFailed) {
		t.Fatalf("failed state made tmux calls: before=%#v after=%#v", callsAtFailed, got)
	}

	installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:node:/home/alice/project\n' ;;
esac
`)
	installProcessTable(t, []processInfo{{
		pid:  "42",
		ppid: "1",
		comm: "node",
		args: "node /usr/bin/codex resume " + persistentTestCodexID,
	}})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := persistentAgentRequestJSON(t, map[string]any{
		"recoveryDescriptor": persistentAgentTestDescriptor("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("reset status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	rawEntries := readPersistentAgentRawEntries(t, persistentPath)
	if rawEntries[0]["state"] == "failed" || rawEntries[0]["consecutiveLaunchFailures"] != nil || rawEntries[0]["nextRetryAt"] != nil {
		t.Fatalf("Make Persistent did not reset failure metadata: %#v", rawEntries[0])
	}
}

func TestPersistentAgentFilterBankedRemovesOnlyExactUserSession(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
		persistentAgentRawEntry("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	store := newPersistentAgentStore(persistentPath)
	filtered, err := store.FilterBanked([]SessionBankEntry{
		{Name: "codex-alpha", UnixUser: "alice"},
		{Name: "codex-alpha", UnixUser: "bob"},
		{Name: "claude-alpha", UnixUser: "alice"},
	})
	if err != nil {
		t.Fatalf("filter banked: %v", err)
	}
	if len(filtered) != 2 || filtered[0].UnixUser != "bob" || filtered[1].Name != "claude-alpha" {
		t.Fatalf("filtered banked = %+v, want only exact alice/codex-alpha removed", filtered)
	}
}

func TestTmuxHandler_ListSessionsProjectsPersistentSupervisorStateJSON(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	installPersistentAgentScriptedTmux(t, `
case "$*" in
  *list-sessions*) printf '$1:codex-healthy:1:0\n$2:claude-backoff:1:0\n$3:codex-needs:1:0\n$4:hermes-wrong:1:0\n$5:hermes-failed:1:0\n$6:mortal-shell:1:0\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_SESSION_BANK_PATH", filepath.Join(tmpDir, "session-bank", "sessions.json"))
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	healthy := persistentAgentRawEntry("codex-healthy", "alice", RecoveryAgentCodex, persistentTestCodexID, "")
	healthy["state"] = PersistentAgentStateHealthy
	healthy["lastCheckAt"] = "2026-07-15T10:00:00Z"
	healthy["lastRestartAt"] = "2026-07-15T09:55:00Z"
	backoff := persistentAgentRawEntry("claude-backoff", "alice", RecoveryAgentClaude, persistentTestClaudeID, "")
	backoff["state"] = PersistentAgentStateBackoff
	backoff["consecutiveLaunchFailures"] = 2
	backoff["nextRetryAt"] = "2026-07-15T10:05:00Z"
	backoff["lastCheckAt"] = "2026-07-15T10:01:00Z"
	backoff["lastError"] = "launch failed: exit status 1"
	needsInteraction := persistentAgentRawEntry("codex-needs", "alice", RecoveryAgentCodex, persistentTestCodexID, "")
	needsInteraction["state"] = PersistentAgentStateNeedsInteraction
	needsInteraction["lastCheckAt"] = "2026-07-15T10:02:00Z"
	needsInteraction["lastError"] = "blocked-needs-interaction: migration"
	wrongIdentity := persistentAgentRawEntry("hermes-wrong", "alice", RecoveryAgentHermes, persistentTestHermesID, "scout")
	wrongIdentity["state"] = PersistentAgentStateWrongIdentity
	wrongIdentity["lastCheckAt"] = "2026-07-15T10:03:00Z"
	wrongIdentity["lastError"] = "wrong identity: expected hermes scout"
	failed := persistentAgentRawEntry("hermes-failed", "alice", RecoveryAgentHermes, "hermes-session-20260715T110000Z", "operator")
	failed["state"] = PersistentAgentStateFailed
	failed["consecutiveLaunchFailures"] = 3
	failed["lastCheckAt"] = "2026-07-15T10:04:00Z"
	failed["lastError"] = "launch failed permanently"
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{healthy, backoff, needsInteraction, wrongIdentity, failed})

	handler := NewTmuxHandler()
	recorder := httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	rawSessions, ok := response["sessions"].([]any)
	if !ok {
		t.Fatalf("sessions payload = %#v, want array", response["sessions"])
	}
	byName := map[string]map[string]any{}
	for _, rawSession := range rawSessions {
		session, ok := rawSession.(map[string]any)
		if !ok {
			t.Fatalf("session payload = %#v, want object", rawSession)
		}
		name, _ := session["name"].(string)
		byName[name] = session
	}

	assertPersistent := func(name, state string) map[string]any {
		t.Helper()
		session, ok := byName[name]
		if !ok {
			t.Fatalf("missing session %q in %#v", name, byName)
		}
		if session["persistent"] != true || session["persistentState"] != state {
			t.Fatalf("%s persistent state payload = %#v", name, session)
		}
		return session
	}
	healthySession := assertPersistent("codex-healthy", PersistentAgentStateHealthy)
	if healthySession["persistentLastCheckAt"] != "2026-07-15T10:00:00Z" || healthySession["persistentLastRestartAt"] != "2026-07-15T09:55:00Z" {
		t.Fatalf("healthy timestamps = %#v", healthySession)
	}
	if _, ok := healthySession["persistentHermesProfile"]; ok {
		t.Fatalf("non-Hermes session exposed profile: %#v", healthySession)
	}
	backoffSession := assertPersistent("claude-backoff", PersistentAgentStateBackoff)
	if backoffSession["persistentConsecutiveLaunchFailures"] != float64(2) || backoffSession["persistentNextRetryAt"] != "2026-07-15T10:05:00Z" || backoffSession["persistentLastError"] != "launch failed: exit status 1" {
		t.Fatalf("backoff metadata = %#v", backoffSession)
	}
	assertPersistent("codex-needs", PersistentAgentStateNeedsInteraction)
	needsSession := byName["codex-needs"]
	if needsSession["persistentLastError"] != "blocked-needs-interaction: migration" {
		t.Fatalf("needs-interaction error = %#v", needsSession)
	}
	wrongSession := assertPersistent("hermes-wrong", PersistentAgentStateWrongIdentity)
	if wrongSession["persistentHermesProfile"] != "scout" || wrongSession["persistentLastError"] != "wrong identity: expected hermes scout" {
		t.Fatalf("wrong-identity Hermes metadata = %#v", wrongSession)
	}
	if _, ok := wrongSession["persistentAgentProfile"]; ok {
		t.Fatalf("wrong-identity Hermes session exposed generic profile: %#v", wrongSession)
	}
	failedSession := assertPersistent("hermes-failed", PersistentAgentStateFailed)
	if failedSession["persistentHermesProfile"] != "operator" || failedSession["persistentConsecutiveLaunchFailures"] != float64(3) || failedSession["persistentLastError"] != "launch failed permanently" {
		t.Fatalf("failed Hermes metadata = %#v", failedSession)
	}
	if _, ok := failedSession["persistentAgentProfile"]; ok {
		t.Fatalf("failed Hermes session exposed generic profile: %#v", failedSession)
	}

	mortal, ok := byName["mortal-shell"]
	if !ok {
		t.Fatalf("missing mortal session in %#v", byName)
	}
	for _, field := range []string{
		"persistent",
		"persistentState",
		"persistentConsecutiveLaunchFailures",
		"persistentNextRetryAt",
		"persistentLastCheckAt",
		"persistentLastRestartAt",
		"persistentLastError",
		"persistentHermesProfile",
		"persistentAgentProfile",
	} {
		if _, ok := mortal[field]; ok {
			t.Fatalf("mortal session included %s: %#v", field, mortal)
		}
	}
}

func TestPersistentAgentDescriptorBackedHermesIgnoresStoredResumeCommand(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *list-sessions*) printf '$1:hermes-scout:1:0\n' ;;
  *has-session*) echo 'no server running on /tmp/tmux-a' >&2; exit 1 ;;
esac
`)
	entry := persistentAgentRawEntry("hermes-scout", "alice", RecoveryAgentHermes, persistentTestHermesID, "scout")
	entry["resumeCommand"] = "/tmp/.hermes/hermes-agent-current/venv/bin/python -m hermes_cli.main --profile scout --resume " + persistentTestHermesID + " --tui --yolo"
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{entry})
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	recorder := httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions?unixUser=alice", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(response.Sessions) != 1 || !response.Sessions[0].Persistent {
		t.Fatalf("sessions = %+v, want one persistent Hermes session", response.Sessions)
	}
	if response.Sessions[0].PersistentResumeCommand != "" {
		t.Fatalf("persistent Hermes resume command = %q, want empty read-side command", response.Sessions[0].PersistentResumeCommand)
	}

	results, err := handler.ReconcilePersistentAgents(context.Background())
	if err != nil {
		t.Fatalf("reconcile persistent agents: %v", err)
	}
	if len(results) != 1 || results[0].Action != "recreated" {
		t.Fatalf("reconcile results = %+v, want recreated", results)
	}
	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	wantCommand := "/home/alice/.hermes/hermes-agent-current/venv/bin/python -m hermes_cli.main --profile scout --resume " + persistentTestHermesID + " --tui --yolo"
	for _, call := range calls {
		for _, arg := range call {
			if strings.Contains(arg, "hermes_cli.main") {
				if arg != wantCommand {
					t.Fatalf("Hermes launch command = %q, want trusted owner-home command %q", arg, wantCommand)
				}
				return
			}
		}
	}
	t.Fatalf("tmux calls missing Hermes launch command: %#v", calls)
}

func persistentAgentTestDescriptor(sessionName, unixUser, kind, sessionID, profile string) WorkloadRecoveryDescriptor {
	return WorkloadRecoveryDescriptor{
		Mode: RecoveryModeAgent,
		Owner: WorkloadRecoveryOwner{
			Kind:       RecoveryOwnerPersistentAgent,
			Ref:        "persistent:" + sessionBankOwnerRef(unixUser, sessionName),
			MayRestart: true,
		},
		Topology: WorkloadRecoveryTopology{
			SessionName:     sessionName,
			WindowIndex:     0,
			WindowName:      "agents",
			WindowLayout:    "b25f,80x24,0,0",
			PaneIndex:       0,
			PaneID:          "%1",
			PaneCurrentPath: "/home/alice/project",
		},
		WorkloadKind: kind,
		Agent: &WorkloadRecoveryAgent{
			Kind:            kind,
			NativeSessionID: sessionID,
			HermesProfile:   profile,
		},
		EvidenceSource: RecoveryEvidenceArgv,
		Confidence:     RecoveryConfidenceHigh,
	}
}

func persistentAgentRawEntry(sessionName, unixUser, kind, sessionID, profile string) map[string]any {
	command := "codex resume " + sessionID
	if kind == RecoveryAgentClaude {
		command = "claude --resume " + sessionID
	}
	if kind == RecoveryAgentHermes {
		command = "/home/alice/.hermes/hermes-agent-current/venv/bin/python -m hermes_cli.main --profile " + profile + " --resume " + sessionID + " --tui --yolo"
	}
	return map[string]any{
		"name":               sessionName,
		"unixUser":           unixUser,
		"identity":           "Maintains exact identity.",
		"agentKind":          kind,
		"agentSessionId":     sessionID,
		"resumeCommand":      "rm -rf /",
		"cwd":                "/home/alice/project",
		"createdAt":          "2026-07-15T00:00:00Z",
		"updatedAt":          "2026-07-15T00:00:00Z",
		"recoveryDescriptor": persistentAgentTestDescriptor(sessionName, unixUser, kind, sessionID, profile),
		"expectedCommand":    command,
	}
}

func writePersistentAgentRawSeed(t *testing.T, path string, entries []map[string]any) {
	t.Helper()
	for _, entry := range entries {
		delete(entry, "expectedCommand")
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal raw persistent seed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir persistent dir: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o660); err != nil {
		t.Fatalf("write raw persistent seed: %v", err)
	}
}

func readPersistentAgentRawEntries(t *testing.T, path string) []map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persistent agents: %v", err)
	}
	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("decode persistent agents: %v; raw=%s", err, raw)
	}
	if len(entries) == 0 {
		t.Fatalf("persistent entries empty; raw=%s", raw)
	}
	return entries
}

func persistentAgentRequestJSON(t *testing.T, value map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal persistent request: %v", err)
	}
	return raw
}

func installProcessTable(t *testing.T, infos []processInfo) {
	t.Helper()
	original := readPersistentAgentProcessTable
	readPersistentAgentProcessTable = func(context.Context) ([]processInfo, error) {
		return infos, nil
	}
	t.Cleanup(func() { readPersistentAgentProcessTable = original })
}

func forcePersistentFailureRetry(t *testing.T, path string, consecutiveFailures int) {
	t.Helper()
	entries := readPersistentAgentRawEntries(t, path)
	entries[0]["state"] = "backoff"
	entries[0]["consecutiveLaunchFailures"] = consecutiveFailures
	entries[0]["nextRetryAt"] = "2000-01-01T00:00:00Z"
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal forced persistent failure retry: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o660); err != nil {
		t.Fatalf("write forced persistent failure retry: %v", err)
	}
}
