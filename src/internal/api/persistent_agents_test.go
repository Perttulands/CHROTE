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
			args:        "/home/alice/.hermes/hermes-agent-current/venv/bin/python -m hermes_cli.main --profile scout --resume " + persistentTestHermesID,
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
			installFakeSystemctl(t)
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
	installFakeSystemctl(t)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/workspace/not-home")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	installProcessTable(t, []processInfo{{
		pid:  "42",
		ppid: "1",
		comm: "python",
		args: "/home/alice/.hermes/hermes-agent-current/venv/bin/python -m hermes_cli.main --profile scout --resume " + persistentTestHermesID,
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
	installFakeSystemctl(t)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	installProcessTable(t, []processInfo{{
		pid:  "42",
		ppid: "1",
		comm: "python",
		args: "/home/alice/.hermes/hermes-agent-current/venv/bin/python -m hermes_cli.main --profile scout",
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
		{pid: "52", ppid: "42", comm: "python", args: "/home/alice/.hermes/hermes-agent-current/venv/bin/python -m hermes_cli.main --profile scout --resume " + persistentTestHermesID},
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
	installFakeSystemctl(t)
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
	installFakeSystemctl(t)
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
	installFakeSystemctl(t)
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
	installFakeSystemctl(t)
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

func TestTmuxHandler_EnablePersistentAgentRejectsManagedStatusBeforeSideEffects(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	managedPath := filepath.Join(tmpDir, "tmux-recovery", "managed-status.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:node:/home/alice/project\n' ;;
esac
`)
	writeManagedStatusSeed(t, managedPath, "codex-alpha", "alice", "codex-alpha.service")
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	installFakeSystemctl(t)
	t.Setenv("CHROTE_MANAGED_RECOVERY_STATUS_PATH", managedPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := persistentAgentRequestJSON(t, map[string]any{
		"identity": "would incorrectly take over externally managed session",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if !strings.Contains(strings.ToLower(recorder.Body.String()), "external manager") {
		t.Fatalf("error body = %s, want external manager ownership conflict", recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want no tmux side effects before managed ownership conflict", got)
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
	installFakeSystemctl(t)
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
	installFakeSystemctl(t)
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
	installFakeSystemctl(t)
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
			installFakeSystemctl(t)
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
	installFakeSystemctl(t)
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

func TestTmuxHandler_EnablePersistentAgentRejectsExplicitDescriptorCWDDisagreement(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:node:/home/alice/project\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	installFakeSystemctl(t)
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
	installFakeSystemctl(t)
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

func TestTmuxHandler_ListSessionsReportsAStoreEntryWithNoUnitAsDegraded(t *testing.T) {
	// The pre-ADR-0014 supervisor kept a six-state ladder in this store. Those
	// states are gone -- health is the unit's -- but a record can still outlive
	// its unit: a failed enable, or a row written by the old supervisor before
	// the migration. Reporting "unlocked" for such a session would contradict the
	// persistent:true in the same payload, so it must read as degraded and say
	// why. This is also the projection an operator sees mid-migration.
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	installPersistentAgentScriptedTmux(t, `
case "$*" in
  *list-sessions*) printf '$1:codex-orphaned:1:0\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_AGENT_UNITS_DIR", filepath.Join(tmpDir, "agent-units"))
	installFakeSystemctl(t)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	{
		entry := persistentAgentRawEntry("codex-orphaned", "alice", RecoveryAgentCodex, persistentTestCodexID, "")
		writePersistentAgentRawSeed(t, persistentPath, []map[string]any{entry})

		handler := NewTmuxHandler()
		recorder := httptest.NewRecorder()
		handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
		var payload struct {
			Sessions []map[string]any `json:"sessions"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode sessions: %v", err)
		}
		var found map[string]any
		for _, session := range payload.Sessions {
			if session["name"] == "codex-orphaned" {
				found = session
			}
		}
		if found == nil {
			t.Fatalf("session was not projected: %+v", payload.Sessions)
		}
		if found["persistent"] != true {
			t.Fatalf("a stored entry must still project persistent:true, got %+v", found)
		}
		if found["persistentHealth"] != agentHealthDegraded {
			t.Fatalf("persistentHealth = %v, want %q", found["persistentHealth"], agentHealthDegraded)
		}
		if detail, _ := found["persistentDetail"].(string); !strings.Contains(detail, "no supervising unit") {
			t.Fatalf("degraded projection must say why, got %q", detail)
		}
		// The retired supervisor fields must not come back in the payload.
		for _, gone := range []string{
			"persistentState", "persistentResumeCommand", "persistentLastError",
			"persistentNextRetryAt", "persistentConsecutiveLaunchFailures",
			"persistentLastCheckAt", "persistentLastRestartAt",
		} {
			if _, present := found[gone]; present {
				t.Fatalf("retired supervisor field %q is still projected: %+v", gone, found)
			}
		}
	}
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
		command = "/home/alice/.hermes/hermes-agent-current/venv/bin/python -m hermes_cli.main --profile " + profile + " --resume " + sessionID
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

func TestTmuxHandler_EnablePersistentAgentRenameFailureRollsBackCleanly(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:node:/home/alice/project\n' ;;
  *rename-session*) echo 'rename exploded' >&2; exit 1 ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	installFakeSystemctl(t)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	installProcessTable(t, []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex resume --no-alt-screen " + persistentTestCodexID}})

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := persistentAgentRequestJSON(t, map[string]any{
		"identity":           "Maintains exact identity.",
		"newName":            "codex-beta",
		"recoveryDescriptor": persistentAgentTestDescriptor("codex-beta", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, expected 500; body=%s", recorder.Code, recorder.Body.String())
	}
	responseBody := recorder.Body.String()
	if !strings.Contains(responseBody, "rename exploded") {
		t.Fatalf("response should carry the tmux error: %s", responseBody)
	}
	if strings.Contains(responseBody, "stale persistent entry") {
		t.Fatalf("successful rollback must not report a stale entry: %s", responseBody)
	}
	raw, err := os.ReadFile(persistentPath)
	if err != nil {
		t.Fatalf("read persistent store: %v", err)
	}
	var entries []map[string]any
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("decode persistent store: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("rollback should leave no entries, got %#v", entries)
	}
}

func TestTmuxHandler_EnablePersistentAgentSurfacesFailedRollback(t *testing.T) {
	tmpDir := t.TempDir()
	storeDir := filepath.Join(tmpDir, "persistent-agents")
	persistentPath := filepath.Join(storeDir, "agents.json")
	t.Cleanup(func() { _ = os.Chmod(storeDir, 0o770) })
	// The fake rename makes the store directory read-only before failing, so
	// Upsert succeeds but the rollback Forget cannot write the store.
	installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:node:/home/alice/project\n' ;;
  *rename-session*) chmod 0500 '`+storeDir+`'; echo 'rename exploded' >&2; exit 1 ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	installFakeSystemctl(t)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	installProcessTable(t, []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex resume --no-alt-screen " + persistentTestCodexID}})

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := persistentAgentRequestJSON(t, map[string]any{
		"identity":           "Maintains exact identity.",
		"newName":            "codex-beta",
		"recoveryDescriptor": persistentAgentTestDescriptor("codex-beta", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, expected 500; body=%s", recorder.Code, recorder.Body.String())
	}
	responseBody := recorder.Body.String()
	if !strings.Contains(responseBody, "rename exploded") ||
		!strings.Contains(responseBody, "stale persistent entry remains") ||
		!strings.Contains(responseBody, "codex-beta") {
		t.Fatalf("failed rollback must surface both errors and the stale name: %s", responseBody)
	}
	_ = os.Chmod(storeDir, 0o770)
	entries := readPersistentAgentRawEntries(t, persistentPath)
	if len(entries) != 1 || entries[0]["name"] != "codex-beta" {
		t.Fatalf("the stale entry the response warned about should exist: %#v", entries)
	}
}
