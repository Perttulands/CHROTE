package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	osuser "os/user"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/chrote/server/internal/core"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "chrote-tmux-tests-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("CHROTE_SESSION_BANK_PATH", filepath.Join(dir, "sessions.json"))
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestTmuxHandler_NewTmuxHandler(t *testing.T) {
	handler := NewTmuxHandler()

	if handler == nil {
		t.Fatal("NewTmuxHandler() returned nil")
	}
	if handler.cache == nil {
		t.Error("Handler cache is nil")
	}
	if handler.colorRegex == nil {
		t.Error("Handler colorRegex is nil")
	}
}

func TestTmuxHandler_ValidateColor(t *testing.T) {
	handler := NewTmuxHandler()

	tests := []struct {
		name    string
		color   string
		isValid bool
	}{
		{"hex 3 digit", "#fff", true},
		{"hex 6 digit", "#ff00ff", true},
		{"named color", "red", true},
		{"named color blue", "blue", true},
		{"default", "default", true},
		{"invalid hex", "#gggggg", false},
		{"invalid chars", "red@blue", false},
		{"empty is not matched by regex but handled separately", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.colorRegex.MatchString(tt.color)
			if result != tt.isValid {
				t.Errorf("colorRegex.MatchString(%q) = %v, expected %v", tt.color, result, tt.isValid)
			}
		})
	}
}

func TestTmuxHandler_CreateSession_InvalidJSON(t *testing.T) {
	handler := NewTmuxHandler()

	// Test with invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", bytes.NewBufferString("{invalid}"))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateSession(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestTmuxHandler_CreateSession_InvalidName(t *testing.T) {
	handler := NewTmuxHandler()

	body := CreateSessionRequest{Name: "invalid name with spaces"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateSession(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}

	var response map[string]interface{}
	json.Unmarshal(recorder.Body.Bytes(), &response)

	if response["success"] != false {
		t.Error("Response should indicate failure")
	}
}

func TestTmuxHandler_DeleteSession_InvalidName(t *testing.T) {
	handler := NewTmuxHandler()

	req := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/invalid@name", nil)
	req.SetPathValue("name", "invalid@name")
	recorder := httptest.NewRecorder()

	handler.DeleteSession(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestTmuxHandler_DeleteAllSessions_NoConfirmHeader(t *testing.T) {
	handler := NewTmuxHandler()

	req := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/all", nil)
	// Intentionally NOT setting X-Nuke-Confirm header
	recorder := httptest.NewRecorder()

	handler.DeleteAllSessions(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Errorf("Status code = %d, expected %d (Forbidden)", recorder.Code, http.StatusForbidden)
	}

	var response map[string]interface{}
	json.Unmarshal(recorder.Body.Bytes(), &response)

	if response["success"] != false {
		t.Error("Response should indicate failure")
	}
}

func TestTmuxHandler_RenameSession_InvalidNewName(t *testing.T) {
	handler := NewTmuxHandler()

	body := RenameSessionRequest{NewName: "invalid name!"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/oldsession", bytes.NewBuffer(bodyBytes))
	req.SetPathValue("name", "oldsession")
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.RenameSession(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestTmuxHandler_RenameSessionRejectsPersistentTargetCollisionBeforeTmux(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, "")
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
		persistentAgentRawEntry("codex-target", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	body := RenameSessionRequest{NewName: "codex-target"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/codex-alpha?unixUser=alice", bytes.NewBuffer(bodyBytes))
	req.SetPathValue("name", "codex-alpha")
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.RenameSession(recorder, req)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want no tmux side effects before persistent target collision", got)
	}
	rawEntries := readPersistentAgentRawEntries(t, persistentPath)
	if len(rawEntries) != 1 || rawEntries[0]["name"] != "codex-target" {
		t.Fatalf("persistent store mutated on rename collision: %#v", rawEntries)
	}
}

func TestTmuxHandler_RenameSessionRejectsSessionBankOwnedSourceBeforeTmux(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installPersistentAgentScriptedTmux(t, "")
	plan := []WorkloadRecoveryDescriptor{
		sessionBankAgentDescriptor("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, "", 0, 0, "%1", "agents", "b25f,80x24,0,0", "/home/alice/project"),
	}
	bankSeed := sessionBankEntryWithRecoveryPlanJSON(t, "codex-alpha", "alice", 1, plan)
	writeBankSeedRaw(t, bankPath, bankSeed)
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
		persistentAgentRawEntry("codex-other", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	persistentBefore, err := os.ReadFile(persistentPath)
	if err != nil {
		t.Fatalf("read persistent seed: %v", err)
	}
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	body := RenameSessionRequest{NewName: "codex-renamed"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/codex-alpha?unixUser=alice", bytes.NewBuffer(bodyBytes))
	req.SetPathValue("name", "codex-alpha")
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.RenameSession(recorder, req)
	assertRecoveryOwnershipError(t, recorder, http.StatusConflict, "PERSISTENT_AGENT_OWNERSHIP_CONFLICT", RecoveryOwnerSessionBank, sessionBankOwnerRef("alice", "codex-alpha"))
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want no tmux side effects before source ownership conflict", got)
	}
	bankAfter, err := os.ReadFile(bankPath)
	if err != nil {
		t.Fatalf("read bank after rename rejection: %v", err)
	}
	persistentAfter, err := os.ReadFile(persistentPath)
	if err != nil {
		t.Fatalf("read persistent after rename rejection: %v", err)
	}
	if !bytes.Equal(bankAfter, bankSeed) {
		t.Fatalf("bank store mutated on rejected source rename:\nbefore=%s\nafter=%s", bankSeed, bankAfter)
	}
	if !bytes.Equal(persistentAfter, persistentBefore) {
		t.Fatalf("persistent store mutated on rejected source rename:\nbefore=%s\nafter=%s", persistentBefore, persistentAfter)
	}
}

func TestTmuxHandler_RenameSessionRejectsSessionBankOwnedTargetBeforeTmux(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installPersistentAgentScriptedTmux(t, "")
	plan := []WorkloadRecoveryDescriptor{
		sessionBankAgentDescriptor("codex-renamed", "alice", RecoveryAgentCodex, persistentTestCodexID, "", 0, 0, "%1", "agents", "b25f,80x24,0,0", "/home/alice/project"),
	}
	bankSeed := sessionBankEntryWithRecoveryPlanJSON(t, "codex-renamed", "alice", 1, plan)
	writeBankSeedRaw(t, bankPath, bankSeed)
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
		persistentAgentRawEntry("codex-other", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	persistentBefore, err := os.ReadFile(persistentPath)
	if err != nil {
		t.Fatalf("read persistent seed: %v", err)
	}
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	body := RenameSessionRequest{NewName: "codex-renamed"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/codex-alpha?unixUser=alice", bytes.NewBuffer(bodyBytes))
	req.SetPathValue("name", "codex-alpha")
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.RenameSession(recorder, req)
	assertRecoveryOwnershipError(t, recorder, http.StatusConflict, "PERSISTENT_AGENT_OWNERSHIP_CONFLICT", RecoveryOwnerSessionBank, sessionBankOwnerRef("alice", "codex-renamed"))
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want no tmux side effects before target ownership conflict", got)
	}
	bankAfter, err := os.ReadFile(bankPath)
	if err != nil {
		t.Fatalf("read bank after rename rejection: %v", err)
	}
	persistentAfter, err := os.ReadFile(persistentPath)
	if err != nil {
		t.Fatalf("read persistent after rename rejection: %v", err)
	}
	if !bytes.Equal(bankAfter, bankSeed) {
		t.Fatalf("bank store mutated on rejected target rename:\nbefore=%s\nafter=%s", bankSeed, bankAfter)
	}
	if !bytes.Equal(persistentAfter, persistentBefore) {
		t.Fatalf("persistent store mutated on rejected target rename:\nbefore=%s\nafter=%s", persistentBefore, persistentAfter)
	}
}

func TestTmuxHandler_RenamePersistentSessionStoreFailureDoesNotCallTmux(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, "")
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
		persistentAgentRawEntry("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	before, err := os.ReadFile(persistentPath)
	if err != nil {
		t.Fatalf("read persistent seed: %v", err)
	}
	persistentDir := filepath.Dir(persistentPath)
	if err := os.Chmod(persistentDir, 0o500); err != nil {
		t.Fatalf("chmod persistent dir read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(persistentDir, 0o700) })
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	body := RenameSessionRequest{NewName: "codex-renamed"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/codex-alpha?unixUser=alice", bytes.NewBuffer(bodyBytes))
	req.SetPathValue("name", "codex-alpha")
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.RenameSession(recorder, req)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want no tmux rename after persistent store write failure", got)
	}
	after, err := os.ReadFile(persistentPath)
	if err != nil {
		t.Fatalf("read persistent after rejected rename: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("persistent store mutated after failed store write:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestTmuxHandler_RenamePersistentSessionReportsRollbackFailureWhenTmuxRenameFails(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	persistentDir := filepath.Dir(persistentPath)
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *rename-session*)
    chmod 500 "$PERSISTENT_DIR"
    echo 'rename failed' >&2
    exit 1
    ;;
esac
`)
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
		persistentAgentRawEntry("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	t.Setenv("PERSISTENT_DIR", persistentDir)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	t.Cleanup(func() { _ = os.Chmod(persistentDir, 0o700) })

	handler := NewTmuxHandler()
	body := RenameSessionRequest{NewName: "codex-renamed"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/codex-alpha?unixUser=alice", bytes.NewBuffer(bodyBytes))
	req.SetPathValue("name", "codex-alpha")
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.RenameSession(recorder, req)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
	if !strings.Contains(strings.ToLower(recorder.Body.String()), "rollback") {
		t.Fatalf("error body = %s, want combined rollback failure", recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) == 0 || !containsArg(got[0], "rename-session") {
		t.Fatalf("tmux calls = %#v, want attempted rename-session", got)
	}
}

func TestTmuxHandler_ReconcileWaitsForPersistentRenameTransaction(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	startedPath := filepath.Join(tmpDir, "rename-started")
	releasePath := filepath.Join(tmpDir, "rename-release")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *rename-session*)
    : > "$RENAME_STARTED"
    while [ ! -f "$RENAME_RELEASE" ]; do sleep 0.02; done
    exit 0
    ;;
  *"has-session -t codex-alpha"*)
    if [ -f "$RENAME_STARTED" ] && [ ! -f "$RENAME_RELEASE" ]; then
      echo "can't find session: codex-alpha" >&2
      exit 1
    fi
    exit 0
    ;;
  *"has-session -t codex-renamed"*)
    if [ -f "$RENAME_RELEASE" ]; then exit 0; fi
    echo "can't find session: codex-renamed" >&2
    exit 1
    ;;
  *display-message*) printf '42:node:/home/alice/project\n' ;;
  *capture-pane*) printf 'Codex ready\n' ;;
  *new-session*) echo 'unexpected early recreate' >&2; exit 1 ;;
esac
`)
	writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
		persistentAgentRawEntry("codex-alpha", "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	t.Setenv("RENAME_STARTED", startedPath)
	t.Setenv("RENAME_RELEASE", releasePath)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	installProcessTable(t, []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex resume " + persistentTestCodexID}})

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := RenameSessionRequest{NewName: "codex-renamed"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/codex-alpha?unixUser=alice", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	renameDone := make(chan int, 1)
	go func() {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, req)
		renameDone <- recorder.Code
	}()
	waitForTestFile(t, startedPath)

	reconcileDone := make(chan []PersistentAgentReconcileResult, 1)
	go func() {
		results, err := handler.ReconcilePersistentAgents(context.Background())
		if err != nil {
			t.Errorf("reconcile persistent agents: %v", err)
		}
		reconcileDone <- results
	}()

	select {
	case results := <-reconcileDone:
		t.Fatalf("reconcile completed before rename transaction released: %+v", results)
	case <-time.After(100 * time.Millisecond):
	}
	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	for _, call := range calls {
		if containsArg(call, "new-session") {
			t.Fatalf("reconcile recreated during blocked rename transaction: %#v", calls)
		}
	}
	if err := os.WriteFile(releasePath, []byte("go"), 0o600); err != nil {
		t.Fatalf("release rename: %v", err)
	}
	if code := <-renameDone; code != http.StatusOK {
		t.Fatalf("rename status code = %d, expected %d", code, http.StatusOK)
	}
	results := <-reconcileDone
	if len(results) != 1 || results[0].Action != "ok" {
		t.Fatalf("reconcile results = %+v, want ok after rename release", results)
	}
}

func TestTmuxHandler_ListSessionsFiltersReservedProbeSessionsFromPublicAndBank(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	installPersistentAgentScriptedTmux(t, `
case "$*" in
  *list-sessions*) printf '$1:chrote-probe-123:1:0\n$2:work:1:0\n' ;;
esac
`)
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	recorder := httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) != 1 || response.Sessions[0].Name != "work" {
		t.Fatalf("sessions = %+v, want only non-reserved work session", response.Sessions)
	}
	if len(response.Banked) != 1 || response.Banked[0].Name != "work" {
		t.Fatalf("banked = %+v, want only non-reserved work session", response.Banked)
	}
	raw, err := os.ReadFile(bankPath)
	if err != nil {
		t.Fatalf("read bank snapshot: %v", err)
	}
	if bytes.Contains(raw, []byte("chrote-probe-123")) {
		t.Fatalf("reserved probe session leaked into bank snapshot: %s", raw)
	}
}

func TestTmuxHandler_ListSessionsProjectsManagedStatusSeparatelyAndSkipsBankOwnership(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	managedPath := filepath.Join(tmpDir, "tmux-recovery", "managed-status.json")
	writeManagedStatusSeed(t, managedPath, "systemd-worker", "alice", "worker.service")
	installPersistentAgentScriptedTmux(t, `
case "$*" in
  *list-sessions*) printf '$1:systemd-worker:1:0\n$2:shell-owned:1:0\n' ;;
esac
`)
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_MANAGED_RECOVERY_STATUS_PATH", managedPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	recorder := httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Managed) != 1 {
		t.Fatalf("managed = %+v, want one read-only registry entry", response.Managed)
	}
	managed := response.Managed[0]
	if managed.Name != "systemd-worker" || managed.SessionName != "systemd-worker" || managed.UnixUser != "alice" {
		t.Fatalf("managed identity = %+v", managed)
	}
	if managed.Owner.Kind != RecoveryOwnerExternalManager || managed.Owner.Ref != "systemd:user/worker.service" || managed.Owner.MayRestart {
		t.Fatalf("managed owner = %+v", managed.Owner)
	}
	if managed.ManagerKind != "systemd-user" || managed.ManagerRef != "worker.service" || !managed.Status.OK || managed.Status.ActiveState != "active" {
		t.Fatalf("managed status = %+v", managed)
	}
	for _, session := range response.Sessions {
		if session.Name == "systemd-worker" {
			t.Fatalf("managed session leaked into ordinary sessions: %+v", response.Sessions)
		}
	}
	for group, sessions := range response.Grouped {
		for _, session := range sessions {
			if session.Name == "systemd-worker" {
				t.Fatalf("managed session leaked into grouped[%s]: %+v", group, sessions)
			}
		}
	}
	if len(response.Banked) != 1 || response.Banked[0].Name != "shell-owned" {
		t.Fatalf("banked = %+v, want only CHROTE-owned shell session", response.Banked)
	}
	raw, err := os.ReadFile(bankPath)
	if err != nil {
		t.Fatalf("read bank snapshot: %v", err)
	}
	if bytes.Contains(raw, []byte("systemd-worker")) {
		t.Fatalf("managed session leaked into Session Bank snapshot: %s", raw)
	}
}

func TestTmuxHandler_ListSessionsReportsMalformedManagedStatusWithoutFailingSessions(t *testing.T) {
	tmpDir := t.TempDir()
	managedPath := filepath.Join(tmpDir, "tmux-recovery", "managed-status.json")
	if err := os.MkdirAll(filepath.Dir(managedPath), 0o755); err != nil {
		t.Fatalf("mkdir managed status: %v", err)
	}
	if err := os.WriteFile(managedPath, []byte(`{"not":"a-list"}`), 0o600); err != nil {
		t.Fatalf("write malformed managed status: %v", err)
	}
	installPersistentAgentScriptedTmux(t, `
case "$*" in
  *list-sessions*) printf '$1:shell-owned:1:0\n' ;;
esac
`)
	t.Setenv("CHROTE_MANAGED_RECOVERY_STATUS_PATH", managedPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	recorder := httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) != 1 || response.Sessions[0].Name != "shell-owned" {
		t.Fatalf("sessions = %+v, want live session despite malformed managed status", response.Sessions)
	}
	if len(response.Managed) != 0 {
		t.Fatalf("managed = %+v, want empty on malformed registry", response.Managed)
	}
	if !strings.Contains(strings.ToLower(response.Error), "managed status") {
		t.Fatalf("error = %q, want managed status validation failure", response.Error)
	}
}

func TestTmuxHandler_CreateSessionRejectsManagedNameBeforeTmux(t *testing.T) {
	tmpDir := t.TempDir()
	managedPath := filepath.Join(tmpDir, "tmux-recovery", "managed-status.json")
	writeManagedStatusSeed(t, managedPath, "systemd-worker", "alice", "worker.service")
	argsPath := installArgvRecordingTmux(t)
	t.Setenv("CHROTE_MANAGED_RECOVERY_STATUS_PATH", managedPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", bytes.NewBufferString(`{"name":"systemd-worker","unixUser":"alice"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateSession(recorder, req)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want none before managed create conflict", got)
	}
}

func TestTmuxHandler_DeleteSessionRejectsManagedNameBeforeTmux(t *testing.T) {
	tmpDir := t.TempDir()
	managedPath := filepath.Join(tmpDir, "tmux-recovery", "managed-status.json")
	writeManagedStatusSeed(t, managedPath, "systemd-worker", "alice", "worker.service")
	argsPath := installArgvRecordingTmux(t)
	t.Setenv("CHROTE_MANAGED_RECOVERY_STATUS_PATH", managedPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/systemd-worker?unixUser=alice", nil)
	req.SetPathValue("name", "systemd-worker")
	recorder := httptest.NewRecorder()

	handler.DeleteSession(recorder, req)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want none before managed delete conflict", got)
	}
}

func TestTmuxHandler_RenameSessionRejectsManagedSourceOrDestinationBeforeTmux(t *testing.T) {
	tests := []struct {
		name       string
		oldName    string
		newName    string
		managed    string
		managerRef string
	}{
		{name: "source", oldName: "systemd-worker", newName: "shell-owned", managed: "systemd-worker", managerRef: "worker.service"},
		{name: "destination", oldName: "shell-owned", newName: "systemd-worker", managed: "systemd-worker", managerRef: "worker.service"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			managedPath := filepath.Join(tmpDir, "tmux-recovery", "managed-status.json")
			writeManagedStatusSeed(t, managedPath, tt.managed, "alice", tt.managerRef)
			argsPath := installArgvRecordingTmux(t)
			t.Setenv("CHROTE_MANAGED_RECOVERY_STATUS_PATH", managedPath)
			t.Setenv("CHROTE_TERMINAL_USERS", "alice")
			t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
			t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
			t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

			handler := NewTmuxHandler()
			req := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/"+tt.oldName+"?unixUser=alice", bytes.NewBufferString(`{"newName":"`+tt.newName+`"}`))
			req.Header.Set("Content-Type", "application/json")
			req.SetPathValue("name", tt.oldName)
			recorder := httptest.NewRecorder()

			handler.RenameSession(recorder, req)
			assertRecoveryOwnershipError(t, recorder, http.StatusConflict, "SESSION_OWNERSHIP_CONFLICT", RecoveryOwnerExternalManager, "systemd:user/"+tt.managerRef)
			if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
				t.Fatalf("tmux calls = %#v, want none before managed rename conflict", got)
			}
		})
	}
}

func TestTmuxHandler_DeleteAllSessionsPreservesManagedRegistryEntries(t *testing.T) {
	tmpDir := t.TempDir()
	managedPath := filepath.Join(tmpDir, "tmux-recovery", "managed-status.json")
	writeManagedStatusSeed(t, managedPath, "systemd-worker", "alice", "worker.service")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *list-sessions*) printf 'systemd-worker\nshell-owned\n' ;;
esac
`)
	t.Setenv("CHROTE_MANAGED_RECOVERY_STATUS_PATH", managedPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/all?unixUser=alice", nil)
	req.Header.Set("X-Nuke-Confirm", "DASHBOARD-NUKE-CONFIRMED")
	recorder := httptest.NewRecorder()

	handler.DeleteAllSessions(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response struct {
		Killed    int      `json:"killed"`
		Sessions  []string `json:"sessions"`
		Protected []string `json:"protected"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Killed != 1 || len(response.Sessions) != 1 || response.Sessions[0] != "shell-owned" {
		t.Fatalf("nuke response = %+v, want only shell-owned killed", response)
	}
	if len(response.Protected) != 1 || response.Protected[0] != "systemd-worker" {
		t.Fatalf("protected = %+v, want managed systemd-worker", response.Protected)
	}
	for _, call := range normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)) {
		if containsArg(call, "kill-session") && containsArg(call, "systemd-worker") {
			t.Fatalf("managed session received kill-session call: %#v", call)
		}
	}
}

func TestTmuxHandler_CreateAndRenameRejectReservedInternalSessionNames(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, "")
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	createBody := bytes.NewBufferString(`{"name":"chrote-probe-user","unixUser":"alice"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", createBody)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.CreateSession(createRec, createReq)
	if createRec.Code != http.StatusBadRequest {
		t.Fatalf("create status code = %d, expected %d; body=%s", createRec.Code, http.StatusBadRequest, createRec.Body.String())
	}

	renameBody := RenameSessionRequest{NewName: "chrote-probe-renamed"}
	renameBytes, _ := json.Marshal(renameBody)
	renameReq := httptest.NewRequest(http.MethodPatch, "/api/tmux/sessions/work?unixUser=alice", bytes.NewBuffer(renameBytes))
	renameReq.SetPathValue("name", "work")
	renameReq.Header.Set("Content-Type", "application/json")
	renameRec := httptest.NewRecorder()
	handler.RenameSession(renameRec, renameReq)
	if renameRec.Code != http.StatusBadRequest {
		t.Fatalf("rename status code = %d, expected %d; body=%s", renameRec.Code, http.StatusBadRequest, renameRec.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want reserved-name rejection before tmux", got)
	}
}

func TestTmuxHandler_ApplyAppearance_InvalidColor(t *testing.T) {
	handler := NewTmuxHandler()

	body := AppearanceRequest{
		StatusBg: "invalidcolor@#$",
		StatusFg: "red",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/tmux/appearance", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.ApplyAppearance(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestTmuxHandler_RegisterRoutes(t *testing.T) {
	handler := NewTmuxHandler()
	mux := http.NewServeMux()

	// This should not panic
	handler.RegisterRoutes(mux)
}

func TestTmuxHandler_ListSessions_ReturnsValidJSON(t *testing.T) {
	handler := NewTmuxHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	recorder := httptest.NewRecorder()

	handler.ListSessions(recorder, req)

	// Should return valid JSON even if tmux isn't running
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("Response is not valid JSON: %v", err)
	}

	// Should have a timestamp
	if response.Timestamp == "" {
		t.Error("Response should include timestamp")
	}

	// Sessions should be initialized (not nil)
	if response.Sessions == nil {
		t.Error("Sessions should be initialized slice, not nil")
	}

	// Grouped should be initialized (not nil)
	if response.Grouped == nil {
		t.Error("Grouped should be initialized map, not nil")
	}
}

func TestTmuxHandler_DefaultProfileUsesConfiguredSocketAndWorkDir(t *testing.T) {
	_, argsPath := installFakeTmux(t)
	t.Setenv("CHROTE_DEFAULT_TMUX_SOCKET", "/tmp/tmux-1001/default")
	t.Setenv("CHROTE_DEFAULT_TMUX_WORKDIR", "/srv/terminal-three")

	handler := NewTmuxHandler()
	body := CreateSessionRequest{Name: "terminal-three-smoke"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateSession(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	got := normalizeFakeTmuxCreationTokens(readFakeCommandCalls(t, argsPath))
	want := []string{
		"-S /tmp/tmux-1001/default new-session -d -P -F #{session_id} -e CHROTE_CREATION_TOKEN=<token> -s terminal-three-smoke -c /srv/terminal-three",
		"-S /tmp/tmux-1001/default set-option -g mouse on",
		"-S /tmp/tmux-1001/default unbind-key -q -n MouseDown3Pane",
		"-S /tmp/tmux-1001/default unbind-key -q -n MouseDown3Status",
		"-S /tmp/tmux-1001/default unbind-key -q -n MouseDown3StatusLeft",
		"-S /tmp/tmux-1001/default unbind-key -q -n M-MouseDown3Pane",
		"-S /tmp/tmux-1001/default unbind-key -q -n M-MouseDown3Status",
		"-S /tmp/tmux-1001/default unbind-key -q -n M-MouseDown3StatusLeft",
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("tmux calls = %#v, want %#v", got, want)
	}
}

func TestTmuxHandler_CurrentUnixUserHonorsConfiguredDefaultTarget(t *testing.T) {
	current, err := osuser.Current()
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	_, argsPath := installFakeTmux(t)
	t.Setenv("CHROTE_DEFAULT_TMUX_SOCKET", "/configured/current-user.sock")
	t.Setenv("CHROTE_DEFAULT_TMUX_WORKDIR", "/srv/current-user")
	t.Setenv("CHROTE_TERMINAL_USERS", current.Username)

	handler := NewTmuxHandler()
	body := CreateSessionRequest{Name: "current-user-smoke", UnixUser: current.Username}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateSession(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	got := normalizeFakeTmuxCreationTokens(readFakeCommandCalls(t, argsPath))
	want := []string{
		"-S /configured/current-user.sock new-session -d -P -F #{session_id} -e CHROTE_CREATION_TOKEN=<token> -s current-user-smoke -c /srv/current-user",
		"-S /configured/current-user.sock set-option -g mouse on",
		"-S /configured/current-user.sock unbind-key -q -n MouseDown3Pane",
		"-S /configured/current-user.sock unbind-key -q -n MouseDown3Status",
		"-S /configured/current-user.sock unbind-key -q -n MouseDown3StatusLeft",
		"-S /configured/current-user.sock unbind-key -q -n M-MouseDown3Pane",
		"-S /configured/current-user.sock unbind-key -q -n M-MouseDown3Status",
		"-S /configured/current-user.sock unbind-key -q -n M-MouseDown3StatusLeft",
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("tmux calls = %#v, want %#v", got, want)
	}
}

func TestTmuxHandler_CurrentUnixUserCarriesTrustedHomeSeparateFromConfiguredWorkDir(t *testing.T) {
	current, err := osuser.Current()
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	if current.HomeDir == "" {
		t.Skip("current user has no OS home to verify")
	}
	t.Setenv("CHROTE_DEFAULT_TMUX_SOCKET", "/configured/current-user.sock")
	t.Setenv("CHROTE_DEFAULT_TMUX_WORKDIR", filepath.Join(current.HomeDir, "project"))
	t.Setenv("CHROTE_TERMINAL_USERS", current.Username)

	handler := NewTmuxHandler()
	target, err := handler.targetForUnixUser(current.Username)
	if err != nil {
		t.Fatalf("targetForUnixUser(%q): %v", current.Username, err)
	}
	if target.workDir == target.ownerHome {
		t.Fatalf("workdir and ownerHome collapsed to %q; configured startup cwd must not define descriptor containment", target.workDir)
	}
	if target.workDir != filepath.Join(current.HomeDir, "project") {
		t.Fatalf("workDir = %q, want configured startup cwd", target.workDir)
	}
	if target.ownerHome != current.HomeDir {
		t.Fatalf("ownerHome = %q, want OS user home %q", target.ownerHome, current.HomeDir)
	}
}

func TestTmuxHandler_CreateSessionUsesSelectedUnixUserTarget(t *testing.T) {
	_, argsPath := installFakeTmux(t)
	t.Setenv("CHROTE_TERMINAL_USERS", "perttu,tavern")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "perttu=/run/user/1000/chrote-tmux/tmux-1000/default,tavern=/tmp/tmux-1001/default")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "perttu=/home/operator,tavern=/home/secondary")

	handler := NewTmuxHandler()
	bodyBytes := []byte(`{"name":"tavern-shell","unixUser":"tavern","mouseScroll":false}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.CreateSession(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	got := normalizeFakeTmuxCreationTokens(readFakeCommandCalls(t, argsPath))
	want := []string{
		"-S /tmp/tmux-1001/default new-session -d -P -F #{session_id} -e CHROTE_CREATION_TOKEN=<token> -s tavern-shell -c /home/secondary",
		"-S /tmp/tmux-1001/default set-option -g mouse off",
		"-S /tmp/tmux-1001/default unbind-key -q -n MouseDown3Pane",
		"-S /tmp/tmux-1001/default unbind-key -q -n MouseDown3Status",
		"-S /tmp/tmux-1001/default unbind-key -q -n MouseDown3StatusLeft",
		"-S /tmp/tmux-1001/default unbind-key -q -n M-MouseDown3Pane",
		"-S /tmp/tmux-1001/default unbind-key -q -n M-MouseDown3Status",
		"-S /tmp/tmux-1001/default unbind-key -q -n M-MouseDown3StatusLeft",
	}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("tmux calls = %#v, want %#v", got, want)
	}
}

func TestTmuxHandler_ListSessionsAggregatesConfiguredTerminalUsers(t *testing.T) {
	tmpDir := t.TempDir()
	fakeTmux := filepath.Join(tmpDir, "tmux")
	script := `#!/bin/sh
case "$*" in
  *"/tmp/tmux-p"*) printf 'perttu-shell:1:0\n' ;;
  *"/tmp/tmux-t"*) printf 'tavern-shell:2:1\n' ;;
  *) printf 'unexpected:1:0\n' ;;
esac
`
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", tmpDir)
	t.Setenv("CHROTE_TERMINAL_USERS", "perttu,tavern")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "perttu=/tmp/tmux-p,tavern=/tmp/tmux-t")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "perttu=/home/operator,tavern=/home/secondary")

	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	recorder := httptest.NewRecorder()

	handler.ListSessions(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) != 2 {
		t.Fatalf("sessions = %+v, want two configured-user sessions", response.Sessions)
	}
	usersBySession := map[string]string{}
	for _, session := range response.Sessions {
		usersBySession[session.Name] = session.UnixUser
	}
	if usersBySession["perttu-shell"] != "perttu" || usersBySession["tavern-shell"] != "tavern" {
		t.Fatalf("session users = %#v, want perttu-shell/perttu and tavern-shell/tavern", usersBySession)
	}
	wantUsers := []string{"perttu", "tavern"}
	if strings.Join(response.TerminalUsers, ",") != strings.Join(wantUsers, ",") {
		t.Fatalf("terminalUsers = %#v, want %#v", response.TerminalUsers, wantUsers)
	}
}

func TestTmuxHandler_ListSessionsReturnsTrimmedDedupedTerminalUsers(t *testing.T) {
	installFakeTmux(t)
	t.Setenv("CHROTE_TERMINAL_USERS", " alice, bob , alice ,,bob ")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a,bob=/tmp/tmux-b")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice,bob=/home/bob")

	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	recorder := httptest.NewRecorder()

	handler.ListSessions(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := []string{"alice", "bob"}
	if strings.Join(response.TerminalUsers, ",") != strings.Join(want, ",") {
		t.Fatalf("terminalUsers = %#v, want %#v", response.TerminalUsers, want)
	}
}

func TestTmuxHandler_ListSessionsDoesNotAdvertiseImplicitCurrentUser(t *testing.T) {
	installFakeTmux(t)
	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	recorder := httptest.NewRecorder()

	handler.ListSessions(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.TerminalUsers) != 0 {
		t.Fatalf("terminalUsers = %#v, want empty when CHROTE_TERMINAL_USERS is unset", response.TerminalUsers)
	}
	for _, session := range response.Sessions {
		if session.UnixUser != "" {
			t.Fatalf("session %q UnixUser = %q, want bare default-session identity", session.Name, session.UnixUser)
		}
	}
}

func TestTmuxHandler_DeleteAllSessionsReportsListErrors(t *testing.T) {
	installFailingTmux(t, "error connecting to /tmp/tmux-1001/default (Permission denied)")
	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/all", nil)
	req.Header.Set("X-Nuke-Confirm", "DASHBOARD-NUKE-CONFIRMED")
	recorder := httptest.NewRecorder()

	handler.DeleteAllSessions(recorder, req)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "Permission denied") {
		t.Fatalf("body = %q, want permission error to fail loud", recorder.Body.String())
	}
}

func TestTmuxHandler_ApplyAppearanceTargetsConfiguredTerminalUsers(t *testing.T) {
	tmpDir := t.TempDir()
	callsPath := filepath.Join(tmpDir, "tmux.calls")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> " + callsPath + "\nprintf '%s\\n' '---' >> " + callsPath + "\n"
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", tmpDir)
	t.Setenv("CHROTE_TERMINAL_USERS", "perttu,tavern")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "perttu=/tmp/tmux-p,tavern=/tmp/tmux-t")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "perttu=/home/operator,tavern=/home/secondary")

	handler := NewTmuxHandler()
	bodyBytes := []byte(`{"statusBg":"default","statusFg":"#ffffff","paneBorderActive":"#ff00ff"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/appearance", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.ApplyAppearance(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatalf("read fake tmux calls: %v", err)
	}
	got := string(calls)
	if strings.Count(got, "-S\n/tmp/tmux-p\nset\n-g\n") != 2 {
		t.Fatalf("perttu appearance calls = %q, want two commands on /tmp/tmux-p", got)
	}
	if strings.Count(got, "-S\n/tmp/tmux-t\nset\n-g\n") != 2 {
		t.Fatalf("tavern appearance calls = %q, want two commands on /tmp/tmux-t", got)
	}
	if strings.Contains(got, "statusBg") {
		t.Fatalf("tmux calls leaked JSON keys instead of set args: %q", got)
	}
}

func TestTmuxHandler_SetMouseModeTargetsConfiguredTerminalUsers(t *testing.T) {
	tmpDir := t.TempDir()
	callsPath := filepath.Join(tmpDir, "tmux.calls")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> " + callsPath + "\nprintf '%s\\n' '---' >> " + callsPath + "\n"
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", tmpDir)
	t.Setenv("CHROTE_TERMINAL_USERS", "perttu,tavern")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "perttu=/tmp/tmux-p,tavern=/tmp/tmux-t")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "perttu=/home/operator,tavern=/home/secondary")

	handler := NewTmuxHandler()
	bodyBytes := []byte(`{"enabled":false}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/mouse", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.SetMouseMode(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["mouse"] != "off" || response["applied"] != float64(2) || response["total"] != float64(2) {
		t.Fatalf("response = %#v, want mouse off applied/total 2", response)
	}
	calls, err := os.ReadFile(callsPath)
	if err != nil {
		t.Fatalf("read fake tmux calls: %v", err)
	}
	got := string(calls)
	if strings.Count(got, "-S\n/tmp/tmux-p\nset-option\n-g\nmouse\noff\n") != 1 {
		t.Fatalf("perttu mouse calls = %q, want mouse off command on /tmp/tmux-p", got)
	}
	if strings.Count(got, "-S\n/tmp/tmux-t\nset-option\n-g\nmouse\noff\n") != 1 {
		t.Fatalf("tavern mouse calls = %q, want mouse off command on /tmp/tmux-t", got)
	}
}

func TestTmuxHandler_SetMouseModeRemovesTmuxRightClickMenus(t *testing.T) {
	_, argsPath := installFakeTmux(t)
	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/mouse", bytes.NewBufferString(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.SetMouseMode(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	want := []string{
		"set-option -g mouse on",
		"unbind-key -q -n MouseDown3Pane",
		"unbind-key -q -n MouseDown3Status",
		"unbind-key -q -n MouseDown3StatusLeft",
		"unbind-key -q -n M-MouseDown3Pane",
		"unbind-key -q -n M-MouseDown3Status",
		"unbind-key -q -n M-MouseDown3StatusLeft",
	}
	if got := normalizeFakeTmuxCreationTokens(readFakeCommandCalls(t, argsPath)); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("tmux calls = %#v, want mouse mode plus no right-click menus %#v", got, want)
	}
}

func TestTmuxHandler_SetMouseModeDoesNotReportPartialPolicyAsApplied(t *testing.T) {
	t.Setenv("CHROTE_TERMINAL_USERS", "")
	t.Setenv("CHROTE_DEFAULT_TMUX_SOCKET", "")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *"unbind-key -q -n MouseDown3Status"*) echo 'unbind failed' >&2; exit 1 ;;
esac
`)
	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/mouse", bytes.NewBufferString(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.SetMouseMode(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Applied int  `json:"applied"`
		Total   int  `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Success || response.Applied != 0 || response.Total != 1 {
		t.Fatalf("response = %+v, want failed policy application with applied=0 total=1", response)
	}
	want := [][]string{
		{"set-option", "-g", "mouse", "on"},
		{"unbind-key", "-q", "-n", "MouseDown3Pane"},
		{"unbind-key", "-q", "-n", "MouseDown3Status"},
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want fail-fast policy calls %#v", got, want)
	}
}

func TestTmuxHandler_CreateOwnedTmuxSessionCleansAmbiguousCreationByMarker(t *testing.T) {
	t.Setenv("CHROTE_TERMINAL_USERS", "")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *new-session*)
    for arg in "$@"; do
      case "$arg" in
        CHROTE_CREATION_TOKEN=*) printf '%s' "$arg" > "$TMUX_SESSION_MARKER_FILE" ;;
      esac
    done
    echo 'context deadline exceeded after server-side creation' >&2
    exit 1
    ;;
esac
`)
	handler := NewTmuxHandler()

	_, err := handler.createOwnedTmuxSession(context.Background(), "", "ambiguous-smoke", "/tmp")
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("create error = %v, want original ambiguous creation error", err)
	}
	want := [][]string{
		{"new-session", "-d", "-P", "-F", "#{session_id}", "-e", "CHROTE_CREATION_TOKEN=<token>", "-s", "ambiguous-smoke", "-c", "/tmp"},
		{"if-shell", "-F", "-t", "ambiguous-smoke", "#{==:#{CHROTE_CREATION_TOKEN},<token>}", "kill-session -t ambiguous-smoke", "display-message -p CHROTE_OWNERSHIP_MISMATCH"},
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want marker-owned ambiguous cleanup %#v", got, want)
	}
}

func TestTmuxHandler_CreateOwnedTmuxSessionCleansMalformedIDByOwnedName(t *testing.T) {
	t.Setenv("CHROTE_TERMINAL_USERS", "")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *new-session*)
    for arg in "$@"; do
      case "$arg" in
        CHROTE_CREATION_TOKEN=*) printf '%s' "$arg" > "$TMUX_SESSION_MARKER_FILE" ;;
      esac
    done
    printf 'not-a-session-id\n'
    exit 0
    ;;
esac
`)
	handler := NewTmuxHandler()

	_, err := handler.createOwnedTmuxSession(context.Background(), "", "malformed-id-smoke", "/tmp")
	if err == nil || !strings.Contains(err.Error(), "without a valid session ID") {
		t.Fatalf("create error = %v, want malformed session ID error", err)
	}
	want := [][]string{
		{"new-session", "-d", "-P", "-F", "#{session_id}", "-e", "CHROTE_CREATION_TOKEN=<token>", "-s", "malformed-id-smoke", "-c", "/tmp"},
		{"if-shell", "-F", "-t", "malformed-id-smoke", "#{==:#{CHROTE_CREATION_TOKEN},<token>}", "kill-session -t malformed-id-smoke", "display-message -p CHROTE_OWNERSHIP_MISMATCH"},
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want name fallback for malformed ID %#v", got, want)
	}
}

func TestTmuxHandler_CreateOwnedTmuxSessionRefusesToCleanUnownedName(t *testing.T) {
	t.Setenv("CHROTE_TERMINAL_USERS", "")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *new-session*) echo 'duplicate session' >&2; exit 1 ;;
  *if-shell*) printf 'CHROTE_OWNERSHIP_MISMATCH\n'; exit 0 ;;
esac
`)
	handler := NewTmuxHandler()

	_, err := handler.createOwnedTmuxSession(context.Background(), "", "existing-smoke", "/tmp")
	if err == nil || !strings.Contains(err.Error(), "creation token does not match") {
		t.Fatalf("create error = %v, want ownership mismatch joined to create failure", err)
	}
	got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	if len(got) != 2 || got[0][0] != "new-session" || got[1][0] != "if-shell" {
		t.Fatalf("tmux calls = %#v, want create plus ownership check and no kill", got)
	}
}

func TestTmuxHandler_CleanupOwnedTmuxSessionJoinsKillFailure(t *testing.T) {
	t.Setenv("CHROTE_TERMINAL_USERS", "")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *if-shell*) echo 'kill denied' >&2; exit 1 ;;
esac
`)
	handler := NewTmuxHandler()
	cause := errors.New("policy failed")

	err := handler.cleanupOwnedTmuxSessionAfterError("", ownedTmuxSession{ID: "$42", Name: "owned-smoke", Token: "0123456789abcdef01234567"}, cause)
	if err == nil || !strings.Contains(err.Error(), "policy failed") || !strings.Contains(err.Error(), "kill denied") {
		t.Fatalf("cleanup error = %v, want original and cleanup failures joined", err)
	}
	want := [][]string{
		{"if-shell", "-F", "-t", "$42", "#{==:#{CHROTE_CREATION_TOKEN},<token>}", "kill-session -t $42", "display-message -p CHROTE_OWNERSHIP_MISMATCH"},
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want ID-based verified cleanup %#v", got, want)
	}
}

func TestTmuxHandler_SetMouseModeRejectsInvalidJSON(t *testing.T) {
	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/mouse", bytes.NewBufferString("{invalid}"))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.SetMouseMode(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestTmuxHandler_SetMouseModeRejectsMissingEnabled(t *testing.T) {
	_, argsPath := installFakeTmux(t)
	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/mouse", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	handler.SetMouseMode(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if got := normalizeFakeTmuxCreationTokens(readFakeCommandCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want no side effect for missing enabled", got)
	}
}

func TestTmuxHandler_RegisterRoutesWiresMouseMode(t *testing.T) {
	_, argsPath := installFakeTmux(t)
	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/mouse", bytes.NewBufferString(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	want := []string{
		"set-option -g mouse on",
		"unbind-key -q -n MouseDown3Pane",
		"unbind-key -q -n MouseDown3Status",
		"unbind-key -q -n MouseDown3StatusLeft",
		"unbind-key -q -n M-MouseDown3Pane",
		"unbind-key -q -n M-MouseDown3Status",
		"unbind-key -q -n M-MouseDown3StatusLeft",
	}
	if got := normalizeFakeTmuxCreationTokens(readFakeCommandCalls(t, argsPath)); strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("tmux calls = %#v, want %#v", got, want)
	}
}

func TestTmuxHandler_SessionBankDoesNotMutateOnFilteredScans(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	script := `#!/bin/sh
case "$*" in
  *"/tmp/tmux-a"*list-sessions*) printf '$1:codex-alpha:1:0\n' ;;
  *"/tmp/tmux-b"*list-sessions*) printf '$2:claude-beta:1:0\n' ;;
esac
`
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", tmpDir)
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice,bob")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a,bob=/tmp/tmux-b")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice,bob=/home/bob")

	handler := NewTmuxHandler()
	recorder := httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("full scan status = %d; body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions?unixUser=alice", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("filtered scan status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	var filtered SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("decode filtered response: %v", err)
	}
	var bob *SessionBankEntry
	for i := range filtered.Banked {
		if filtered.Banked[i].Name == "claude-beta" && filtered.Banked[i].UnixUser == "bob" {
			bob = &filtered.Banked[i]
			break
		}
	}
	if bob == nil {
		t.Fatalf("filtered bank = %+v, want existing bob entry preserved", filtered.Banked)
	}
	if !bob.Live {
		t.Fatalf("filtered bank bob entry = %+v, want live unchanged by alice-only scan", *bob)
	}
}

func TestTmuxHandler_SessionBankKeepsRestartResumeHints(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "offline")
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	script := "#!/bin/sh\ncase \"$*\" in\n  *list-sessions*)\n    if [ ! -f " + statePath + " ]; then printf '$7:codex-alpha:1:0\\n'; fi\n    ;;\nesac\n"
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", tmpDir)
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	recorder := httptest.NewRecorder()
	handler.ListSessions(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var liveResponse SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &liveResponse); err != nil {
		t.Fatalf("decode live response: %v", err)
	}
	if len(liveResponse.Banked) != 1 || !liveResponse.Banked[0].Live {
		t.Fatalf("banked live response = %+v, want one live banked session", liveResponse.Banked)
	}
	if liveResponse.Banked[0].ID != "$7" {
		t.Fatalf("banked session id = %q, want tmux id", liveResponse.Banked[0].ID)
	}
	if liveResponse.Banked[0].ResumeCommand != "/resume codex-alpha" {
		t.Fatalf("resume command = %q, want /resume codex-alpha", liveResponse.Banked[0].ResumeCommand)
	}

	rawBank, err := os.ReadFile(bankPath)
	if err != nil {
		t.Fatalf("read bank file: %v", err)
	}
	tampered := strings.Replace(string(rawBank), `"resumeCommand": "/resume codex-alpha"`, `"resumeCommand": "rm -rf /"`, 1)
	if tampered == string(rawBank) {
		t.Fatalf("bank fixture did not contain expected resume command: %s", rawBank)
	}
	if err := os.WriteFile(bankPath, []byte(tampered), 0o660); err != nil {
		t.Fatalf("tamper bank file: %v", err)
	}

	if err := os.WriteFile(statePath, []byte("offline"), 0o644); err != nil {
		t.Fatalf("write offline state: %v", err)
	}
	handler = NewTmuxHandler()
	recorder = httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	var offlineResponse SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &offlineResponse); err != nil {
		t.Fatalf("decode offline response: %v", err)
	}
	if len(offlineResponse.Sessions) != 0 {
		t.Fatalf("live sessions = %+v, want none after restart/offline scan", offlineResponse.Sessions)
	}
	if len(offlineResponse.Banked) != 1 || offlineResponse.Banked[0].Live {
		t.Fatalf("banked offline response = %+v, want one offline banked session", offlineResponse.Banked)
	}
	if offlineResponse.Banked[0].LastSeen == "" || offlineResponse.Banked[0].FirstSeen == "" {
		t.Fatalf("bank timestamps missing: %+v", offlineResponse.Banked[0])
	}
	if offlineResponse.Banked[0].ResumeCommand != "/resume codex-alpha" {
		t.Fatalf("offline resume command = %q, want sanitized /resume codex-alpha", offlineResponse.Banked[0].ResumeCommand)
	}
}

func TestTmuxHandler_EnablePersistentAgentRenamesStoresAndAnnotatesLiveSession(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:node:/home/alice/project\n' ;;
  *list-sessions*) printf '$7:codex-vw-codex1:1:0\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	const sessionID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	installProcessTable(t, []processInfo{{
		pid:  "42",
		ppid: "1",
		comm: "node",
		args: "node /usr/bin/codex resume " + sessionID,
	}})

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := bytes.NewBufferString(`{"newName":"codex-vw-codex1","identity":"Maintains the VW Codex lane.","agentKind":"codex","agentSessionId":"` + sessionID + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", body)
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
	if response["success"] != true || response["session"] != "codex-vw-codex1" || response["persistent"] != true || response["agentKind"] != "codex" || response["agentSessionId"] != sessionID || response["resumeCommand"] != "codex resume "+sessionID || response["cwd"] != "/home/alice/project" || response["identity"] != "Maintains the VW Codex lane." {
		t.Fatalf("persistent response = %#v", response)
	}
	wantPrefix := [][]string{
		{"-S", "/tmp/tmux-a", "display-message", "-p", "-t", "codex-alpha", "#{pane_pid}:#{pane_current_command}:#{pane_current_path}"},
		{"-S", "/tmp/tmux-a", "rename-session", "-t", "codex-alpha", "codex-vw-codex1"},
	}
	got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	if len(got) < len(wantPrefix) || !equalArgvCalls(got[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("tmux calls prefix = %#v, want %#v", got, wantPrefix)
	}

	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	var sessions SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &sessions); err != nil {
		t.Fatalf("decode sessions: %v", err)
	}
	if len(sessions.Sessions) != 1 {
		t.Fatalf("sessions = %+v, want one", sessions.Sessions)
	}
	session := sessions.Sessions[0]
	if !session.Persistent || session.PersistentIdentity != "Maintains the VW Codex lane." || session.PersistentAgentKind != "codex" || session.PersistentAgentSessionID != sessionID || session.PersistentResumeCommand != "codex resume "+sessionID {
		t.Fatalf("persistent session metadata = %+v", session)
	}
}

func TestTmuxHandler_EnablePersistentAgentInfersCodexMetadataFromLiveProcess(t *testing.T) {
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
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	const sessionID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	originalReadProcessTable := readPersistentAgentProcessTable
	readPersistentAgentProcessTable = func(context.Context) ([]processInfo, error) {
		return []processInfo{{
			pid:  "42",
			ppid: "1",
			comm: "node",
			args: "node /usr/bin/codex resume --no-alt-screen -C /home/alice/project " + sessionID,
		}}, nil
	}
	t.Cleanup(func() { readPersistentAgentProcessTable = originalReadProcessTable })

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := bytes.NewBufferString(`{"identity":"Maintains the VW Codex lane."}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", body)
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
	if response["agentKind"] != "codex" || response["agentSessionId"] != sessionID || response["resumeCommand"] != "codex resume "+sessionID || response["cwd"] != "/home/alice/project" {
		t.Fatalf("persistent response = %#v", response)
	}
}

func TestTmuxHandler_EnablePersistentAgentFallsBackToOwnerProbeWhenArgsLackResumeID(t *testing.T) {
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
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	const sessionID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	originalReadProcessTable := readPersistentAgentProcessTable
	readPersistentAgentProcessTable = func(context.Context) ([]processInfo, error) {
		return []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex --no-alt-screen"}}, nil
	}
	t.Cleanup(func() { readPersistentAgentProcessTable = originalReadProcessTable })
	originalProbe := probePersistentAgentOwnerMetadata
	probePersistentAgentOwnerMetadata = func(ctx context.Context, h *TmuxHandler, target tmuxTarget, pane paneInspection, requestedKind string) (inferredPersistentAgentMetadata, error) {
		if target.unixUser != "alice" || target.socket != "/tmp/tmux-a" || pane.PID != "42" || pane.CWD != "/home/alice/project" || requestedKind != "" {
			t.Fatalf("probe args target=%+v pane=%+v requestedKind=%q", target, pane, requestedKind)
		}
		return inferredPersistentAgentMetadata{Kind: "codex", SessionID: sessionID, Source: "owner-probe"}, nil
	}
	t.Cleanup(func() { probePersistentAgentOwnerMetadata = originalProbe })

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := bytes.NewBufferString(`{"identity":"Maintains the VW Codex lane."}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", body)
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
	if response["agentKind"] != "codex" || response["agentSessionId"] != sessionID || response["resumeCommand"] != "codex resume "+sessionID {
		t.Fatalf("persistent response = %#v", response)
	}
}

func TestTmuxHandler_EnablePersistentAgentFailsClearlyWhenSessionIDCannotBeInferred(t *testing.T) {
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
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	originalReadProcessTable := readPersistentAgentProcessTable
	readPersistentAgentProcessTable = func(context.Context) ([]processInfo, error) {
		return []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex --no-alt-screen"}}, nil
	}
	t.Cleanup(func() { readPersistentAgentProcessTable = originalReadProcessTable })
	originalProbe := probePersistentAgentOwnerMetadata
	probePersistentAgentOwnerMetadata = func(context.Context, *TmuxHandler, tmuxTarget, paneInspection, string) (inferredPersistentAgentMetadata, error) {
		return inferredPersistentAgentMetadata{}, context.Canceled
	}
	t.Cleanup(func() { probePersistentAgentOwnerMetadata = originalProbe })

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewBufferString(`{"identity":"Maintains the VW Codex lane."}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "Could not infer Codex/Claude session id") {
		t.Fatalf("error body = %s", recorder.Body.String())
	}
	if raw, err := os.ReadFile(persistentPath); err == nil && strings.TrimSpace(string(raw)) != "[]" && strings.TrimSpace(string(raw)) != "" {
		t.Fatalf("persistent store should be empty, got %s", raw)
	}
}

func TestTmuxHandler_ListSessionsDoesNotWritePersistentLiveSessionsIntoBank(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	installPersistentAgentScriptedTmux(t, `
case "$*" in
  *list-sessions*) printf '$7:codex-alpha:1:0\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	writePersistentAgentSeed(t, persistentPath, []PersistentAgentEntry{{
		Name:           "codex-alpha",
		UnixUser:       "alice",
		Identity:       "Maintains the repo.",
		AgentKind:      "codex",
		AgentSessionID: "019f45ec-f88b-7f70-88dc-b5b99a9e94c6",
		CWD:            "/home/alice/project",
	}})

	handler := NewTmuxHandler()
	recorder := httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode sessions: %v", err)
	}
	if len(response.Sessions) != 1 || !response.Sessions[0].Persistent {
		t.Fatalf("sessions = %+v, want one persistent live session", response.Sessions)
	}
	if len(response.Banked) != 0 {
		t.Fatalf("banked = %+v, want persistent live session excluded from bank", response.Banked)
	}
	entries, err := handler.bank.Read()
	if err != nil {
		t.Fatalf("read bank: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("bank store entries = %+v, want none for persistent live session", entries)
	}
}

func TestTmuxHandler_EnablePersistentAgentRejectsNonAgentPaneWithoutPersisting(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf 'bash:/home/alice/project\n' ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewBufferString(`{"agentKind":"codex","agentSessionId":"019f45ec-f88b-7f70-88dc-b5b99a9e94c6"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	want := [][]string{{"-S", "/tmp/tmux-a", "display-message", "-p", "-t", "codex-alpha", "#{pane_pid}:#{pane_current_command}:#{pane_current_path}"}}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want %#v", got, want)
	}
	if raw, err := os.ReadFile(persistentPath); err == nil && strings.TrimSpace(string(raw)) != "[]" && strings.TrimSpace(string(raw)) != "" {
		t.Fatalf("persistent store should be empty, got %s", raw)
	}
}

func TestPersistentAgentProcessDetectionAcceptsNodeWrappedCodex(t *testing.T) {
	if !processLooksLikeAgent("node", "node /usr/bin/codex --no-alt-screen", "codex") {
		t.Fatal("node-wrapped Codex process should count as live codex")
	}
	if processLooksLikeAgent("python", "python /tmp/codex-helper.py", "codex") {
		t.Fatal("non-node helper mentioning codex should not count as live codex")
	}
	if processLooksLikeAgent("node", "node /usr/bin/claude", "codex") {
		t.Fatal("node-wrapped Claude process should not count as live codex")
	}
}

func TestPersistentAgentProcessTreeChecksPaneRootProcess(t *testing.T) {
	infos := []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex"}}
	if !processTreeContainsAgentInTable(infos, "42", "codex") {
		t.Fatal("pane root node-wrapped Codex process should count as live codex")
	}
}

func TestPersistentAgentMetadataInferenceParsesClaudeResumeArgs(t *testing.T) {
	const sessionID = "9ed1181c-b2a3-4ef2-96ea-a84e51e79dc4"
	infos := []processInfo{{pid: "50", ppid: "1", comm: "claude", args: "claude --dangerously-skip-permissions --resume " + sessionID + " READ-ONLY"}}
	metadata, foundAgent, foundSessionID := inferPersistentAgentMetadataInTable(infos, "50", "")
	if !foundAgent || !foundSessionID || metadata.Kind != "claude" || metadata.SessionID != sessionID {
		t.Fatalf("metadata=%+v foundAgent=%v foundSessionID=%v", metadata, foundAgent, foundSessionID)
	}
}

func TestTmuxHandler_ReconcilePersistentAgentsRecreatesMissingCodexSession(t *testing.T) {
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
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	const sessionID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	writePersistentAgentSeed(t, persistentPath, []PersistentAgentEntry{{
		Name:           "codex-alpha",
		UnixUser:       "alice",
		Identity:       "Maintains the repo.",
		AgentKind:      "codex",
		AgentSessionID: sessionID,
		ResumeCommand:  "rm -rf /",
		CWD:            "/home/alice/project",
	}})

	handler := NewTmuxHandler()
	results, err := handler.ReconcilePersistentAgents(context.Background())
	if err != nil {
		t.Fatalf("reconcile persistent agents: %v", err)
	}
	if len(results) != 1 || results[0].Action != "recreated" || results[0].Error != "" {
		t.Fatalf("reconcile results = %+v", results)
	}
	want := [][]string{
		{"-S", "/tmp/tmux-a", "has-session", "-t", "codex-alpha"},
		{"-S", "/tmp/tmux-a", "new-session", "-d", "-P", "-F", "#{session_id}", "-e", "CHROTE_CREATION_TOKEN=<token>", "-s", "codex-alpha", "-c", "/home/alice/project"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "MouseDown3Pane"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "MouseDown3Status"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "MouseDown3StatusLeft"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "M-MouseDown3Pane"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "M-MouseDown3Status"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "M-MouseDown3StatusLeft"},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "$42", "-l", "--", "codex resume " + sessionID},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "$42", "Enter"},
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want %#v", got, want)
	}
}

func TestTmuxHandler_ReconcilePersistentAgentsFailsAndCleansUpWhenMenuRemovalFails(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *has-session*) echo 'no server running on /tmp/tmux-a' >&2; exit 1 ;;
  *"unbind-key -q -n MouseDown3Pane"*) echo 'unbind failed' >&2; exit 1 ;;
esac
`)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	const sessionID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	writePersistentAgentSeed(t, persistentPath, []PersistentAgentEntry{{
		Name:           "codex-alpha",
		UnixUser:       "alice",
		AgentKind:      "codex",
		AgentSessionID: sessionID,
		CWD:            "/home/alice/project",
	}})

	handler := NewTmuxHandler()
	results, err := handler.ReconcilePersistentAgents(context.Background())
	if err != nil {
		t.Fatalf("reconcile persistent agents: %v", err)
	}
	if len(results) != 1 || results[0].Action != "backoff" || !strings.Contains(results[0].Error, "MouseDown3Pane") {
		t.Fatalf("reconcile results = %+v, want menu-removal backoff", results)
	}
	want := [][]string{
		{"-S", "/tmp/tmux-a", "has-session", "-t", "codex-alpha"},
		{"-S", "/tmp/tmux-a", "new-session", "-d", "-P", "-F", "#{session_id}", "-e", "CHROTE_CREATION_TOKEN=<token>", "-s", "codex-alpha", "-c", "/home/alice/project"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "MouseDown3Pane"},
		{"-S", "/tmp/tmux-a", "if-shell", "-F", "-t", "$42", "#{==:#{CHROTE_CREATION_TOKEN},<token>}", "kill-session -t $42", "display-message -p CHROTE_OWNERSHIP_MISMATCH"},
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want cleanup calls %#v", got, want)
	}
}

func TestTmuxHandler_DisablePersistentAgentRemovesDesiredStateWithoutCallingTmux(t *testing.T) {
	tmpDir := t.TempDir()
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	argsPath := installArgvRecordingTmux(t)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	writePersistentAgentSeed(t, persistentPath, []PersistentAgentEntry{{
		Name:           "codex-alpha",
		UnixUser:       "alice",
		AgentKind:      "codex",
		AgentSessionID: "019f45ec-f88b-7f70-88dc-b5b99a9e94c6",
		CWD:            "/home/alice/project",
	}})

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want none", got)
	}
	entries, err := handler.persistent.Read()
	if err != nil {
		t.Fatalf("read persistent store: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("persistent entries = %+v, want empty", entries)
	}
}

func TestTmuxHandler_ListSessionsPreservesAgentRecoveryMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installArgvRecordingTmux(t)
	const sessionID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	seed := []SessionBankEntry{{
		Name:           "codex-alpha",
		UnixUser:       "alice",
		Group:          "codex",
		Windows:        1,
		Attached:       false,
		Live:           true,
		FirstSeen:      "2026-07-09T00:00:00Z",
		LastSeen:       "2026-07-09T00:00:00Z",
		AgentKind:      "codex",
		AgentSessionID: sessionID,
		ResumeCommand:  "rm -rf /",
		CWD:            "/home/alice/project",
		TranscriptPath: "/home/alice/.codex/sessions/rollout-" + sessionID + ".jsonl",
	}}
	raw, err := json.Marshal(seed)
	if err != nil {
		t.Fatalf("marshal bank seed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(bankPath), 0o755); err != nil {
		t.Fatalf("mkdir bank: %v", err)
	}
	if err := os.WriteFile(bankPath, raw, 0o660); err != nil {
		t.Fatalf("write bank seed: %v", err)
	}
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")

	handler := NewTmuxHandler()
	recorder := httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) != 0 {
		t.Fatalf("live sessions = %+v, want none", response.Sessions)
	}
	if len(response.Banked) != 1 {
		t.Fatalf("banked = %+v, want one entry", response.Banked)
	}
	entry := response.Banked[0]
	if entry.Live {
		t.Fatalf("entry.Live = true, want offline after empty tmux scan: %+v", entry)
	}
	if entry.AgentKind != "codex" || entry.AgentSessionID != sessionID || entry.CWD != "/home/alice/project" || entry.TranscriptPath == "" {
		t.Fatalf("agent recovery metadata not preserved: %+v", entry)
	}
	if entry.RecoveryKind != "agent" {
		t.Fatalf("recovery kind = %q, want agent", entry.RecoveryKind)
	}
	if entry.ResumeCommand != "codex resume "+sessionID {
		t.Fatalf("resume command = %q, want canonical codex command", entry.ResumeCommand)
	}
	if calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(calls) != 1 {
		t.Fatalf("tmux calls = %#v, want one list-sessions call", calls)
	}
}

func TestSessionBankCanonicalAgentResumeCommand(t *testing.T) {
	const codexID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	const claudeID = "9ed1181c-b2a3-4ef2-96ea-a84e51e79dc4"
	tests := []struct {
		name string
		kind string
		id   string
		want string
		ok   bool
	}{
		{name: "codex", kind: "codex", id: codexID, want: "codex resume " + codexID, ok: true},
		{name: "claude", kind: "claude", id: claudeID, want: "claude --resume " + claudeID, ok: true},
		{name: "normalizes kind", kind: " Codex ", id: codexID, want: "codex resume " + codexID, ok: true},
		{name: "unknown kind", kind: "pi", id: codexID, ok: false},
		{name: "empty id", kind: "codex", id: "", ok: false},
		{name: "space in id", kind: "codex", id: codexID + " extra", ok: false},
		{name: "semicolon in id", kind: "codex", id: codexID + ";touch-pwn", ok: false},
		{name: "newline in id", kind: "codex", id: codexID + "\nwhoami", ok: false},
		{name: "dollar in id", kind: "codex", id: "$(whoami)", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := canonicalAgentResumeCommand(tt.kind, tt.id)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("canonicalAgentResumeCommand(%q, %q) = %q, %v; want %q, %v", tt.kind, tt.id, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestTmuxHandler_RecoverBankedCodexSessionCreatesShellAndSendsResumeCommandLiterally(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installSessionBankRecoveryTmux(t, `
case "$*" in
  *has-session*) echo 'no server running on /tmp/tmux-a' >&2; exit 1 ;;
esac
`)
	const sessionID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	writeBankSeed(t, bankPath, []SessionBankEntry{{
		Name:           "codex-alpha",
		UnixUser:       "alice",
		Group:          "codex",
		Windows:        1,
		FirstSeen:      "2026-07-09T00:00:00Z",
		LastSeen:       "2026-07-09T00:00:00Z",
		AgentKind:      "codex",
		AgentSessionID: sessionID,
		ResumeCommand:  "rm -rf /",
		CWD:            "/home/alice/project",
	}})
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/codex-alpha/recover?unixUser=alice", bytes.NewBufferString(`{"mouseScroll":true}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	wantCalls := [][]string{
		{"-S", "/tmp/tmux-a", "has-session", "-t", "codex-alpha"},
		{"-S", "/tmp/tmux-a", "new-session", "-d", "-P", "-F", "#{session_id}", "-e", "CHROTE_CREATION_TOKEN=<token>", "-s", "codex-alpha", "-c", "/home/alice/project"},
		{"-S", "/tmp/tmux-a", "set-option", "-g", "mouse", "on"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "MouseDown3Pane"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "MouseDown3Status"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "MouseDown3StatusLeft"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "M-MouseDown3Pane"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "M-MouseDown3Status"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "M-MouseDown3StatusLeft"},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "$42", "-l", "codex resume " + sessionID},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "$42", "Enter"},
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); !equalArgvCalls(got, wantCalls) {
		t.Fatalf("tmux calls = %#v, want %#v", got, wantCalls)
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["success"] != true || response["session"] != "codex-alpha" || response["agentKind"] != "codex" || response["agentSessionId"] != sessionID || response["resumeCommand"] != "codex resume "+sessionID {
		t.Fatalf("recover response = %#v", response)
	}
	if _, hasData := response["data"]; hasData {
		t.Fatalf("recover response should be flat JSON, got data envelope: %#v", response)
	}
}

func TestTmuxHandler_SendToSessionStoresDropAndPastesViaBuffer(t *testing.T) {
	h := newSendHarness(t, "$7\talice-shell\t%42\t111\t9001\n")
	dropsDir := h.dropsDir
	argsPath := h.tmuxLog

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("text", "Please inspect this screenshot."); err != nil {
		t.Fatalf("write text field: %v", err)
	}
	if err := writer.WriteField("submit", "true"); err != nil {
		t.Fatalf("write submit field: %v", err)
	}
	if err := writer.WriteField("unixUser", "alice"); err != nil {
		t.Fatalf("write unixUser field: %v", err)
	}
	fileWriter, err := writer.CreateFormFile("files", "../clipboard image.png")
	if err != nil {
		t.Fatalf("create file field: %v", err)
	}
	if _, err := fileWriter.Write([]byte("fake png")); err != nil {
		t.Fatalf("write file payload: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/alice-shell/send", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["success"] != true || response["session"] != "alice-shell" || response["unixUser"] != "alice" || response["pane"] != "%42" || response["submitted"] != true {
		t.Fatalf("send response = %#v", response)
	}
	dropPath, _ := response["dropPath"].(string)
	if dropPath == "" || !strings.HasPrefix(dropPath, dropsDir) {
		t.Fatalf("dropPath = %q, want under %q", dropPath, dropsDir)
	}
	for _, rel := range []string{"manifest.json", "text.txt", "payload.txt", filepath.Join("files", "clipboard-image.png")} {
		if _, err := os.Stat(filepath.Join(dropPath, rel)); err != nil {
			t.Fatalf("expected drop file %s: %v", rel, err)
		}
	}
	payload, err := os.ReadFile(filepath.Join(dropPath, "payload.txt"))
	if err != nil {
		t.Fatalf("read payload: %v", err)
	}
	filePath := filepath.Join(dropPath, "files", "clipboard-image.png")
	payloadText := string(payload)
	if !strings.Contains(payloadText, "Please inspect this screenshot.") || !strings.Contains(payloadText, "CHROTE stored this send at:") || !strings.Contains(payloadText, dropPath) || !strings.Contains(payloadText, "Files:") || !strings.Contains(payloadText, filePath) {
		t.Fatalf("payload = %q, want text, drop path %q, and stored file path %q", payloadText, dropPath, filePath)
	}
	if strings.HasSuffix(payloadText, "\n") {
		t.Fatalf("payload has trailing newline; submit=false must not press Enter implicitly: %q", payloadText)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat stored file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("stored file owner-only base mode = %o, want 600 with fake ACL", info.Mode().Perm())
	}

	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	joined := make([]string, 0, len(calls))
	for _, call := range calls {
		joined = append(joined, strings.Join(call, "\x00"))
	}
	wantSnippets := []string{
		strings.Join([]string{"-S", "/tmp/tmux-a", "load-buffer"}, "\x00"),
		strings.Join([]string{"-S", "/tmp/tmux-a", "if-shell", "-F", "-t", "%42"}, "\x00"),
		"paste-buffer -d -b chrote-send-",
		"send-keys -t %42 Enter",
		atomicSendSubmittedMarker,
	}
	for _, snippet := range wantSnippets {
		found := false
		for _, call := range joined {
			if strings.Contains(call, snippet) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("tmux calls = %#v, missing %q", calls, snippet)
		}
	}
	for _, call := range joined {
		if strings.Contains(call, "Please inspect this screenshot") {
			t.Fatalf("bulk text leaked into tmux argv instead of buffer file: %#v", calls)
		}
	}
}

func TestTmuxHandler_UpdateBankedRecoveryMetadataMakesLiveSessionRecoverableAfterRestart(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "offline")
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	fakeTmux := filepath.Join(tmpDir, "tmux")
	script := "#!/bin/sh\ncase \"$*\" in\n  *list-sessions*)\n    if [ ! -f " + statePath + " ]; then printf '$7:codex-alpha:1:0\\n'; fi\n    ;;\nesac\n"
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	t.Setenv("PATH", tmpDir)
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	const sessionID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"

	handler := NewTmuxHandler()
	recorder := httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("initial list status = %d; body=%s", recorder.Code, recorder.Body.String())
	}

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := bytes.NewBufferString(`{"agentKind":"codex","agentSessionId":"` + sessionID + `","cwd":"/home/alice/project","transcriptPath":"/home/alice/.codex/sessions/rollout-` + sessionID + `.jsonl"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/codex-alpha/recovery?unixUser=alice", body)
	req.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("recovery metadata status = %d; body=%s", recorder.Code, recorder.Body.String())
	}

	if err := os.WriteFile(statePath, []byte("offline"), 0o644); err != nil {
		t.Fatalf("write offline marker: %v", err)
	}
	handler = NewTmuxHandler()
	recorder = httptest.NewRecorder()
	handler.ListSessions(recorder, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("offline list status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode offline response: %v", err)
	}
	if len(response.Banked) != 1 {
		t.Fatalf("banked = %+v, want one recoverable bank entry", response.Banked)
	}
	entry := response.Banked[0]
	if entry.RecoveryKind != "agent" || entry.AgentKind != "codex" || entry.AgentSessionID != sessionID || entry.ResumeCommand != "codex resume "+sessionID || entry.CWD != "/home/alice/project" {
		t.Fatalf("offline recovery entry = %+v, want persisted agent metadata", entry)
	}
}

func TestTmuxHandler_UpdateBankedRecoveryRejectsManagedOwnerBeforeStoring(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	managedPath := filepath.Join(tmpDir, "tmux-recovery", "managed-status.json")
	writeManagedStatusSeed(t, managedPath, "codex-alpha", "alice", "codex-alpha.service")
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_MANAGED_RECOVERY_STATUS_PATH", managedPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := bytes.NewBufferString(`{"agentKind":"codex","agentSessionId":"019f45ec-f88b-7f70-88dc-b5b99a9e94c6","cwd":"/home/alice/project"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/codex-alpha/recovery?unixUser=alice", body)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if !strings.Contains(strings.ToLower(recorder.Body.String()), "external manager") {
		t.Fatalf("error body = %s, want external manager ownership conflict", recorder.Body.String())
	}
	if raw, err := os.ReadFile(bankPath); err == nil && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("session bank should remain empty, got %s", raw)
	}
}

func TestTmuxHandler_RecoverBankedRecoveryRejectsManagedOwnerBeforeTmux(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	managedPath := filepath.Join(tmpDir, "tmux-recovery", "managed-status.json")
	argsPath := installSessionBankRecoveryTmux(t, "")
	const sessionID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	writeBankSeed(t, bankPath, []SessionBankEntry{{
		Name:           "codex-alpha",
		UnixUser:       "alice",
		Group:          "codex",
		Windows:        1,
		FirstSeen:      "2026-07-09T00:00:00Z",
		LastSeen:       "2026-07-09T00:00:00Z",
		AgentKind:      "codex",
		AgentSessionID: sessionID,
		CWD:            "/home/alice/project",
	}})
	writeManagedStatusSeed(t, managedPath, "codex-alpha", "alice", "codex-alpha.service")
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_MANAGED_RECOVERY_STATUS_PATH", managedPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/codex-alpha/recover?unixUser=alice", bytes.NewBufferString(`{"mouseScroll":true}`))
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
}

func TestSessionBankEntryPresentEmptyRecoveryPlanRoundTripStaysUnsafe(t *testing.T) {
	raw := `[{
		"name":"empty-plan",
		"unixUser":"alice",
		"group":"codex",
		"windows":1,
		"attached":false,
		"live":false,
		"firstSeen":"2026-07-09T00:00:00Z",
		"lastSeen":"2026-07-09T00:00:00Z",
		"agentKind":"codex",
		"agentSessionId":"019f45ec-f88b-7f70-88dc-b5b99a9e94c6",
		"resumeCommand":"codex resume 019f45ec-f88b-7f70-88dc-b5b99a9e94c6",
		"cwd":"/home/alice/project",
		"recoveryPlan":[]
	}]`
	var entries []SessionBankEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		t.Fatalf("unmarshal present-empty recovery plan: %v", err)
	}
	entries = sanitizeSessionBankEntries(entries)
	if len(entries) != 1 {
		t.Fatalf("entries = %+v, want one sanitized entry", entries)
	}
	if entries[0].RecoveryKind != RecoveryModeUnresolved || entries[0].AgentKind != "" || entries[0].AgentSessionID != "" || entries[0].ResumeCommand != "" {
		t.Fatalf("sanitized entry = %+v, want unresolved with no legacy resume metadata", entries[0])
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal entries: %v", err)
	}
	if !bytes.Contains(encoded, []byte(`"recoveryPlan":[]`)) {
		t.Fatalf("encoded entry = %s, want present empty recoveryPlan preserved", encoded)
	}
	if bytes.Contains(encoded, []byte(`"agentKind"`)) || bytes.Contains(encoded, []byte(`codex resume`)) {
		t.Fatalf("encoded entry = %s, want no legacy resume metadata", encoded)
	}
}

func TestTmuxHandler_RecoverBankedPresentEmptyPlanRejectsBeforeTmux(t *testing.T) {
	for _, body := range []string{"{}", `{"topologyOnly":true}`} {
		t.Run(body, func(t *testing.T) {
			tmpDir := t.TempDir()
			bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
			argsPath := installSessionBankRecoveryTmux(t, "")
			writeBankSeedRaw(t, bankPath, []byte(`[{
				"name":"empty-plan",
				"unixUser":"alice",
				"group":"codex",
				"windows":1,
				"attached":false,
				"live":false,
				"firstSeen":"2026-07-09T00:00:00Z",
				"lastSeen":"2026-07-09T00:00:00Z",
				"agentKind":"codex",
				"agentSessionId":"019f45ec-f88b-7f70-88dc-b5b99a9e94c6",
				"resumeCommand":"codex resume 019f45ec-f88b-7f70-88dc-b5b99a9e94c6",
				"cwd":"/home/alice/project",
				"recoveryPlan":[]
			}]`))
			t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
			t.Setenv("CHROTE_TERMINAL_USERS", "alice")
			t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
			t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
			t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

			handler := NewTmuxHandler()
			req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/empty-plan/recover?unixUser=alice", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			handler.RecoverBankedSession(recorder, req)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
				t.Fatalf("tmux calls = %#v, want none before empty plan rejection", got)
			}
		})
	}
}

func TestTmuxHandler_UpdateBankedRecoveryRejectsPresentEmptyOrNullPlanEvenWithLegacyAgent(t *testing.T) {
	tests := map[string]string{
		"empty": `{"agentKind":"codex","agentSessionId":"019f45ec-f88b-7f70-88dc-b5b99a9e94c6","cwd":"/home/alice/project","recoveryPlan":[]}`,
		"null":  `{"agentKind":"codex","agentSessionId":"019f45ec-f88b-7f70-88dc-b5b99a9e94c6","cwd":"/home/alice/project","recoveryPlan":null}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			tmpDir := t.TempDir()
			bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
			t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
			t.Setenv("CHROTE_TERMINAL_USERS", "alice")
			t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
			t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
			t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

			handler := NewTmuxHandler()
			req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/empty-plan/recovery?unixUser=alice", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			handler.UpdateBankedRecovery(recorder, req)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if raw, err := os.ReadFile(bankPath); err == nil && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "[]" {
				t.Fatalf("bank should remain empty after rejected present-empty plan, got %s", raw)
			}
		})
	}
}

func TestTmuxHandler_RecoverBankedSessionDropsUnsafeTmuxFormatCWD(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installSessionBankRecoveryTmux(t, `
case "$*" in
  *has-session*) echo 'no server running on /tmp/tmux-a' >&2; exit 1 ;;
esac
`)
	const sessionID = "019f45ec-f88b-7f70-88dc-b5b99a9e94c6"
	writeBankSeed(t, bankPath, []SessionBankEntry{{
		Name:           "codex-alpha",
		UnixUser:       "alice",
		Group:          "codex",
		FirstSeen:      "2026-07-09T00:00:00Z",
		LastSeen:       "2026-07-09T00:00:00Z",
		AgentKind:      "codex",
		AgentSessionID: sessionID,
		CWD:            "/tmp/#(touch /tmp/chrote-pwned)",
	}})
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/codex-alpha/recover?unixUser=alice", bytes.NewBufferString(`{"mouseScroll":true}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	if len(calls) < 2 || strings.Join(calls[0], "\x00") != strings.Join([]string{"-S", "/tmp/tmux-a", "has-session", "-t", "codex-alpha"}, "\x00") || strings.Join(calls[1], "\x00") != strings.Join([]string{"-S", "/tmp/tmux-a", "new-session", "-d", "-P", "-F", "#{session_id}", "-e", "CHROTE_CREATION_TOKEN=<token>", "-s", "codex-alpha", "-c", "/home/alice"}, "\x00") {
		t.Fatalf("first tmux call = %#v, want unsafe cwd dropped and configured workdir used", calls)
	}
	for _, call := range calls {
		if strings.Contains(strings.Join(call, " "), "#(") {
			t.Fatalf("tmux calls include unsafe format cwd: %#v", calls)
		}
	}
}

func TestTmuxHandler_RecoverBankedSessionRejectsUnsafeAgentMetadataWithoutTmuxSideEffects(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installArgvRecordingTmux(t)
	writeBankSeed(t, bankPath, []SessionBankEntry{{
		Name:           "codex-alpha",
		UnixUser:       "alice",
		Group:          "codex",
		FirstSeen:      "2026-07-09T00:00:00Z",
		LastSeen:       "2026-07-09T00:00:00Z",
		AgentKind:      "codex",
		AgentSessionID: "019f45ec-f88b-7f70-88dc-b5b99a9e94c6;touch-pwn",
		ResumeCommand:  "rm -rf /",
	}})
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/codex-alpha/recover?unixUser=alice", nil)
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want none for unsafe recovery metadata", got)
	}
}

func TestTmuxHandler_RecoverBankedVelisPlanRecreatesTwoWindowsAndLaunchesTypedWorkloads(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installSessionBankRecoveryTmux(t, `
case "$*" in
  *has-session*) echo 'no server running on /tmp/tmux-a' >&2; exit 1 ;;
esac
`)
	const (
		velisLayoutAgents = "b25f,120x40,0,0[120x13,0,0,1,120x13,0,14,2,120x12,0,28,3]"
		velisLayoutServer = "7f91,120x40,0,0[60x40,0,0,4,59x40,61,0,5]"
	)
	plan := []WorkloadRecoveryDescriptor{
		sessionBankAgentDescriptor("velis", "alice", RecoveryAgentCodex, recoveryTestCodexID, "", 0, 0, "%1", "agents", velisLayoutAgents, "/home/alice/velis"),
		sessionBankAgentDescriptor("velis", "alice", RecoveryAgentClaude, recoveryTestClaudeID, "", 0, 1, "%2", "agents", velisLayoutAgents, "/home/alice/velis"),
		sessionBankAgentDescriptor("velis", "alice", RecoveryAgentHermes, recoveryTestHermesID, "scout", 0, 2, "%3", "agents", velisLayoutAgents, "/home/alice/velis"),
		sessionBankPythonDescriptor("velis", "alice", 8088, "/home/alice/velis/public", 1, 0, "%4", "server", velisLayoutServer, "/home/alice/velis/server"),
		sessionBankTopologyDescriptor("velis", "alice", 1, 1, "%5", "server", velisLayoutServer, "/home/alice/velis"),
	}
	writeBankSeedRaw(t, bankPath, sessionBankEntryWithRecoveryPlanJSON(t, "velis", "alice", 2, plan))
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/velis/recover?unixUser=alice", bytes.NewBufferString(`{"mouseScroll":true}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	wantCalls := [][]string{
		{"-S", "/tmp/tmux-a", "has-session", "-t", "velis"},
		{"-S", "/tmp/tmux-a", "new-session", "-d", "-P", "-F", "#{session_id}\t#{window_id}\t#{pane_id}", "-e", "CHROTE_CREATION_TOKEN=<token>", "-s", "velis", "-c", "/home/alice/velis", "-n", "agents"},
		{"-S", "/tmp/tmux-a", "new-window", "-d", "-P", "-F", "#{window_id}\t#{pane_id}", "-t", "$42", "-n", "server", "-c", "/home/alice/velis/server"},
		{"-S", "/tmp/tmux-a", "split-window", "-d", "-P", "-F", "#{pane_id}", "-t", "%24", "-c", "/home/alice/velis"},
		{"-S", "/tmp/tmux-a", "split-window", "-d", "-P", "-F", "#{pane_id}", "-t", "%24", "-c", "/home/alice/velis"},
		{"-S", "/tmp/tmux-a", "split-window", "-d", "-P", "-F", "#{pane_id}", "-t", "%31", "-c", "/home/alice/velis"},
		{"-S", "/tmp/tmux-a", "select-layout", "-t", "@17", velisLayoutAgents},
		{"-S", "/tmp/tmux-a", "select-layout", "-t", "@18", velisLayoutServer},
		{"-S", "/tmp/tmux-a", "set-option", "-g", "mouse", "on"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "MouseDown3Pane"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "MouseDown3Status"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "MouseDown3StatusLeft"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "M-MouseDown3Pane"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "M-MouseDown3Status"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "M-MouseDown3StatusLeft"},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "%24", "-l", "codex resume " + recoveryTestCodexID},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "%24", "Enter"},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "%44", "-l", "claude --resume " + recoveryTestClaudeID},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "%44", "Enter"},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "%45", "-l", "/home/alice/.hermes/hermes-agent-current/venv/bin/python -m hermes_cli.main --profile scout --resume " + recoveryTestHermesID},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "%45", "Enter"},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "%31", "-l", "python3 -m http.server 8088 --bind 127.0.0.1 --directory /home/alice/velis/public"},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "%31", "Enter"},
	}
	got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	if !equalArgvCalls(got, wantCalls) {
		t.Fatalf("tmux calls = %#v, want %#v", got, wantCalls)
	}
	assertNoLogicalTmuxIndexTargets(t, got)
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["success"] != true || response["action"] != "recovered" || response["session"] != "velis" || response["launched"] != float64(4) || response["topologyOnly"] != false {
		t.Fatalf("recover response = %#v", response)
	}
}

func TestTmuxHandler_UpdateAndRecoverPlanNormalizesSourceIndexesButPreservesStoredEvidence(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installSessionBankRecoveryTmux(t, `
case "$*" in
  *has-session*) echo 'no server running on /tmp/tmux-a' >&2; exit 1 ;;
esac
`)
	const (
		layoutAgents = "b25f,100x30,0,0[100x15,0,0,4,100x14,0,16,5]"
		layoutServer = "7f91,100x30,0,0[50x30,0,0,4,49x30,51,0,5]"
	)
	plan := []WorkloadRecoveryDescriptor{
		sessionBankAgentDescriptor("indexed", "alice", RecoveryAgentCodex, recoveryTestCodexID, "", 7, 4, "%74", "agents", layoutAgents, "/home/alice/indexed"),
		sessionBankAgentDescriptor("indexed", "alice", RecoveryAgentClaude, recoveryTestClaudeID, "", 7, 5, "%75", "agents", layoutAgents, "/home/alice/indexed"),
		sessionBankPythonDescriptor("indexed", "alice", 8088, "/home/alice/indexed/public", 8, 4, "%84", "server", layoutServer, "/home/alice/indexed/server"),
		sessionBankTopologyDescriptor("indexed", "alice", 8, 5, "%85", "server", layoutServer, "/home/alice/indexed"),
	}
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := bytes.NewBuffer(sessionBankRecoveryPlanRequestJSON(t, plan))
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/indexed/recovery?unixUser=alice", body)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	entries, err := handler.bank.Read()
	if err != nil {
		t.Fatalf("read bank: %v", err)
	}
	if len(entries) != 1 || len(entries[0].RecoveryPlan) != len(plan) {
		t.Fatalf("bank entries = %+v, want stored recovery plan", entries)
	}
	for i, desc := range entries[0].RecoveryPlan {
		if desc.Topology.WindowIndex != plan[i].Topology.WindowIndex || desc.Topology.PaneIndex != plan[i].Topology.PaneIndex {
			t.Fatalf("stored descriptor %d indexes = %d.%d, want original evidence %d.%d", i, desc.Topology.WindowIndex, desc.Topology.PaneIndex, plan[i].Topology.WindowIndex, plan[i].Topology.PaneIndex)
		}
	}

	req = httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/indexed/recover?unixUser=alice", nil)
	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("recover status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	wantCalls := [][]string{
		{"-S", "/tmp/tmux-a", "has-session", "-t", "indexed"},
		{"-S", "/tmp/tmux-a", "new-session", "-d", "-P", "-F", "#{session_id}\t#{window_id}\t#{pane_id}", "-e", "CHROTE_CREATION_TOKEN=<token>", "-s", "indexed", "-c", "/home/alice/indexed", "-n", "agents"},
		{"-S", "/tmp/tmux-a", "new-window", "-d", "-P", "-F", "#{window_id}\t#{pane_id}", "-t", "$42", "-n", "server", "-c", "/home/alice/indexed/server"},
		{"-S", "/tmp/tmux-a", "split-window", "-d", "-P", "-F", "#{pane_id}", "-t", "%24", "-c", "/home/alice/indexed"},
		{"-S", "/tmp/tmux-a", "split-window", "-d", "-P", "-F", "#{pane_id}", "-t", "%31", "-c", "/home/alice/indexed"},
		{"-S", "/tmp/tmux-a", "select-layout", "-t", "@17", layoutAgents},
		{"-S", "/tmp/tmux-a", "select-layout", "-t", "@18", layoutServer},
		{"-S", "/tmp/tmux-a", "set-option", "-g", "mouse", "on"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "MouseDown3Pane"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "MouseDown3Status"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "MouseDown3StatusLeft"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "M-MouseDown3Pane"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "M-MouseDown3Status"},
		{"-S", "/tmp/tmux-a", "unbind-key", "-q", "-n", "M-MouseDown3StatusLeft"},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "%24", "-l", "codex resume " + recoveryTestCodexID},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "%24", "Enter"},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "%44", "-l", "claude --resume " + recoveryTestClaudeID},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "%44", "Enter"},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "%31", "-l", "python3 -m http.server 8088 --bind 127.0.0.1 --directory /home/alice/indexed/public"},
		{"-S", "/tmp/tmux-a", "send-keys", "-t", "%31", "Enter"},
	}
	got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	if !equalArgvCalls(got, wantCalls) {
		t.Fatalf("tmux calls = %#v, want %#v", got, wantCalls)
	}
	assertNoLogicalTmuxIndexTargets(t, got)
}

func TestTmuxHandler_RecoverBankedPlanUsesTrustedHomeSeparateFromStartupWorkdir(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installSessionBankRecoveryTmux(t, `
case "$*" in
  *has-session*) echo 'no server running on /tmp/tmux-a' >&2; exit 1 ;;
esac
`)
	plan := []WorkloadRecoveryDescriptor{
		sessionBankPythonDescriptor("velis", "alice", 8088, "/home/alice/shared-assets", 0, 0, "%1", "server", "b25f,80x24,0,0", "/home/alice/velis"),
	}
	writeBankSeedRaw(t, bankPath, sessionBankEntryWithRecoveryPlanJSON(t, "velis", "alice", 1, plan))
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/velis/recover?unixUser=alice", nil)
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	wantCommand := []string{"-S", "/tmp/tmux-a", "send-keys", "-t", "%24", "-l", "python3 -m http.server 8088 --bind 127.0.0.1 --directory /home/alice/shared-assets"}
	if !containsArgvCall(calls, wantCommand) {
		t.Fatalf("tmux calls = %#v, want canonical command under trusted home %#v", calls, wantCommand)
	}
}

func TestTmuxHandler_RecoverBankedPlanRejectsOutsideTrustedHomeAndMissingHomeBeforeTmux(t *testing.T) {
	tests := []struct {
		name       string
		homeMap    string
		directory  string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "outside trusted home",
			homeMap:    "alice=/home/alice",
			directory:  "/home/bob/shared-assets",
			wantStatus: http.StatusBadRequest,
			wantBody:   "path must stay under owner home",
		},
		{
			name:       "missing trusted home",
			directory:  "/home/alice/project/public",
			wantStatus: http.StatusBadRequest,
			wantBody:   "trusted owner home",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
			argsPath := installSessionBankRecoveryTmux(t, `
case "$*" in
  *has-session*) echo 'no server running on /tmp/tmux-a' >&2; exit 1 ;;
esac
`)
			plan := []WorkloadRecoveryDescriptor{
				sessionBankPythonDescriptor("velis", "alice", 8088, tt.directory, 0, 0, "%1", "server", "b25f,80x24,0,0", "/home/alice/project"),
			}
			writeBankSeedRaw(t, bankPath, sessionBankEntryWithRecoveryPlanJSON(t, "velis", "alice", 1, plan))
			t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
			t.Setenv("CHROTE_TERMINAL_USERS", "alice")
			t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
			t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
			if tt.homeMap != "" {
				t.Setenv("CHROTE_TERMINAL_USER_HOMES", tt.homeMap)
			}

			handler := NewTmuxHandler()
			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)
			req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/velis/recover?unixUser=alice", nil)
			recorder := httptest.NewRecorder()

			mux.ServeHTTP(recorder, req)
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), tt.wantBody) {
				t.Fatalf("body = %s, want %q", recorder.Body.String(), tt.wantBody)
			}
			if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
				t.Fatalf("tmux calls = %#v, want none before trusted-home validation succeeds", got)
			}
		})
	}
}

func TestTmuxHandler_UpdateBankedRecoveryPlanRequiresTrustedHomeBeforeStoring(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	desc := sessionBankPythonDescriptor("velis", "alice", 8088, "/home/alice/project/public", 0, 0, "%1", "server", "b25f,80x24,0,0", "/home/alice/project")
	body := bytes.NewBuffer(sessionBankRecoveryPlanRequestJSON(t, []WorkloadRecoveryDescriptor{desc}))

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/velis/recovery?unixUser=alice", body)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "trusted owner home") {
		t.Fatalf("body = %s, want trusted owner home failure", recorder.Body.String())
	}
	if raw, err := os.ReadFile(bankPath); err == nil && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("bank should not store descriptor without trusted home, got %s", raw)
	}
}

func TestTmuxHandler_UpdateAndRecoverBankedPlanEmptyUnixUserUsesCurrentHome(t *testing.T) {
	current, err := osuser.Current()
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	ownerHome := strings.TrimSpace(current.HomeDir)
	if ownerHome == "" {
		t.Skip("current user has no OS home to verify")
	}
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installSessionBankRecoveryTmux(t, `
case "$*" in
  *has-session*) echo 'no server running on /tmp/tmux-default' >&2; exit 1 ;;
esac
`)
	sessionName := "currenthome"
	workloadCWD := filepath.Join(ownerHome, "ctx-sh7-workload")
	directory := filepath.Join(ownerHome, "ctx-sh7-shared-assets")
	plan := []WorkloadRecoveryDescriptor{
		sessionBankPythonDescriptor(sessionName, "", 8091, directory, 0, 0, "%1", "server", "b25f,80x24,0,0", workloadCWD),
	}
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_DEFAULT_TMUX_SOCKET", "/tmp/tmux-default")
	t.Setenv("CHROTE_DEFAULT_TMUX_WORKDIR", filepath.Join(ownerHome, "ctx-sh7-startup-project"))

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := bytes.NewBuffer(sessionBankRecoveryPlanRequestJSON(t, plan))
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/"+sessionName+"/recovery", body)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	entries, err := handler.bank.Read()
	if err != nil {
		t.Fatalf("read bank: %v", err)
	}
	if len(entries) != 1 || entries[0].UnixUser != "" {
		t.Fatalf("bank entries = %+v, want legacy empty unixUser bank key", entries)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/"+sessionName+"/recover", nil)
	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("recover status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	wantCommand := []string{"-S", "/tmp/tmux-default", "send-keys", "-t", "%24", "-l", "python3 -m http.server 8091 --bind 127.0.0.1 --directory " + directory}
	if !containsArgvCall(calls, wantCommand) {
		t.Fatalf("tmux calls = %#v, want current-home-bounded command %#v", calls, wantCommand)
	}
	for _, call := range calls {
		if containsArg(call, filepath.Join(ownerHome, "ctx-sh7-startup-project")) {
			t.Fatalf("descriptor recovery used startup workdir as containment or launch cwd: %#v", calls)
		}
	}
}

func TestTmuxHandler_EmptyUnixUserDescriptorRecoveryFailsClearlyWithoutCurrentHome(t *testing.T) {
	originalCurrentUser := tmuxCurrentUser
	tmuxCurrentUser = func() (*osuser.User, error) {
		return &osuser.User{Username: "current", HomeDir: ""}, nil
	}
	t.Cleanup(func() { tmuxCurrentUser = originalCurrentUser })

	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installSessionBankRecoveryTmux(t, `
case "$*" in
  *has-session*) echo 'no server running on /tmp/tmux-default' >&2; exit 1 ;;
esac
`)
	sessionName := "currenthome"
	plan := []WorkloadRecoveryDescriptor{
		sessionBankPythonDescriptor(sessionName, "", 8091, "/home/current/public", 0, 0, "%1", "server", "b25f,80x24,0,0", "/home/current/project"),
	}
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_DEFAULT_TMUX_SOCKET", "/tmp/tmux-default")
	t.Setenv("CHROTE_DEFAULT_TMUX_WORKDIR", "/home/current/startup")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := bytes.NewBuffer(sessionBankRecoveryPlanRequestJSON(t, plan))
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/"+sessionName+"/recovery", body)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("update status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "trusted owner home") {
		t.Fatalf("update body = %s, want trusted owner home failure", recorder.Body.String())
	}
	if raw, err := os.ReadFile(bankPath); err == nil && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("bank should not store descriptor without current-user home, got %s", raw)
	}

	writeBankSeedRaw(t, bankPath, sessionBankEntryWithRecoveryPlanJSON(t, sessionName, "", 1, plan))
	req = httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/"+sessionName+"/recover", nil)
	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("recover status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "trusted owner home") {
		t.Fatalf("recover body = %s, want trusted owner home failure", recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want none without current-user home", got)
	}
}

func TestTmuxHandler_RecoverBankedPlanRejectsUnsafeAndConflictingDescriptorsBeforeTmux(t *testing.T) {
	tests := []struct {
		name string
		plan []WorkloadRecoveryDescriptor
		want string
	}{
		{
			name: "duplicate pane target",
			plan: []WorkloadRecoveryDescriptor{
				sessionBankAgentDescriptor("velis", "alice", RecoveryAgentCodex, recoveryTestCodexID, "", 0, 0, "%1", "agents", "b25f,80x24,0,0", "/home/alice/velis"),
				sessionBankAgentDescriptor("velis", "alice", RecoveryAgentClaude, recoveryTestClaudeID, "", 0, 0, "%2", "agents", "b25f,80x24,0,0", "/home/alice/velis"),
			},
			want: "duplicate recovery pane target",
		},
		{
			name: "duplicate pane id",
			plan: []WorkloadRecoveryDescriptor{
				sessionBankAgentDescriptor("velis", "alice", RecoveryAgentCodex, recoveryTestCodexID, "", 0, 0, "%1", "agents", "b25f,80x24,0,0", "/home/alice/velis"),
				sessionBankAgentDescriptor("velis", "alice", RecoveryAgentClaude, recoveryTestClaudeID, "", 0, 1, "%1", "agents", "b25f,80x24,0,0", "/home/alice/velis"),
			},
			want: "duplicate recovery pane id",
		},
		{
			name: "conflicting owner",
			plan: []WorkloadRecoveryDescriptor{
				sessionBankAgentDescriptor("velis", "alice", RecoveryAgentCodex, recoveryTestCodexID, "", 0, 0, "%1", "agents", "b25f,80x24,0,0", "/home/alice/velis"),
				func() WorkloadRecoveryDescriptor {
					desc := sessionBankAgentDescriptor("velis", "alice", RecoveryAgentClaude, recoveryTestClaudeID, "", 0, 1, "%2", "agents", "b25f,80x24,0,0", "/home/alice/velis")
					desc.Owner.Ref = "bob/velis"
					return desc
				}(),
			},
			want: "recovery owner ref",
		},
		{
			name: "persistent agent owner",
			plan: []WorkloadRecoveryDescriptor{
				func() WorkloadRecoveryDescriptor {
					desc := sessionBankAgentDescriptor("velis", "alice", RecoveryAgentCodex, recoveryTestCodexID, "", 0, 0, "%1", "agents", "b25f,80x24,0,0", "/home/alice/velis")
					desc.Owner = WorkloadRecoveryOwner{Kind: RecoveryOwnerPersistentAgent, Ref: "persistent:alice/velis", MayRestart: true}
					return desc
				}(),
			},
			want: "must be session_bank-owned",
		},
		{
			name: "external manager owner",
			plan: []WorkloadRecoveryDescriptor{
				func() WorkloadRecoveryDescriptor {
					desc := sessionBankBaseDescriptor("velis", "alice", 0, 0, "%1", "agents", "b25f,80x24,0,0", "/home/alice/velis")
					desc.Mode = RecoveryModeManaged
					desc.Owner = WorkloadRecoveryOwner{Kind: RecoveryOwnerExternalManager, Ref: "systemd:user/velis.service", MayRestart: false}
					desc.WorkloadKind = RecoveryWorkloadManaged
					desc.EvidenceSource = RecoveryEvidenceManager
					desc.Confidence = RecoveryConfidenceHigh
					return desc
				}(),
			},
			want: "must be session_bank-owned",
		},
		{
			name: "unsafe cwd",
			plan: []WorkloadRecoveryDescriptor{
				sessionBankAgentDescriptor("velis", "alice", RecoveryAgentCodex, recoveryTestCodexID, "", 0, 0, "%1", "agents", "b25f,80x24,0,0", "/home/alice/#(touch pwn)"),
			},
			want: "unsafe",
		},
		{
			name: "window index gap",
			plan: []WorkloadRecoveryDescriptor{
				sessionBankTopologyDescriptor("velis", "alice", 7, 4, "%74", "agents", "b25f,80x24,0,0", "/home/alice/velis"),
				sessionBankTopologyDescriptor("velis", "alice", 9, 4, "%94", "server", "7f91,80x24,0,0", "/home/alice/velis"),
			},
			want: "recovery windows must be contiguous",
		},
		{
			name: "pane index gap",
			plan: []WorkloadRecoveryDescriptor{
				sessionBankTopologyDescriptor("velis", "alice", 7, 4, "%74", "agents", "b25f,80x24,0,0", "/home/alice/velis"),
				sessionBankTopologyDescriptor("velis", "alice", 7, 6, "%76", "agents", "b25f,80x24,0,0", "/home/alice/velis"),
			},
			want: "recovery panes for window 7 must be contiguous",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
			argsPath := installSessionBankRecoveryTmux(t, "")
			writeBankSeedRaw(t, bankPath, sessionBankEntryWithRecoveryPlanJSON(t, "velis", "alice", 1, tt.plan))
			t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
			t.Setenv("CHROTE_TERMINAL_USERS", "alice")
			t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
			t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
			t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

			handler := NewTmuxHandler()
			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)
			req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/velis/recover?unixUser=alice", nil)
			recorder := httptest.NewRecorder()

			mux.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), tt.want) {
				t.Fatalf("error body = %s, want %q", recorder.Body.String(), tt.want)
			}
			if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
				t.Fatalf("tmux calls = %#v, want none before validation succeeds", got)
			}
		})
	}
}

func TestTmuxHandler_RecoverBankedUnresolvedPlanRequiresTopologyOnlyAndNeverLaunches(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	unresolved := sessionBankUnresolvedDescriptor("velis", "alice", 0, 0, "%8", "worker", "b25f,80x24,0,0", "/home/alice/velis")
	writeBankSeedRaw(t, bankPath, sessionBankEntryWithRecoveryPlanJSON(t, "velis", "alice", 1, []WorkloadRecoveryDescriptor{unresolved}))
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	argsPath := installSessionBankRecoveryTmux(t, "")
	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/velis/recover?unixUser=alice", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "unresolved") {
		t.Fatalf("error body = %s, want unresolved failure", recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want none before topology-only opt-in", got)
	}

	argsPath = installSessionBankRecoveryTmux(t, `
case "$*" in
  *has-session*) echo 'no server running on /tmp/tmux-a' >&2; exit 1 ;;
esac
`)
	handler = NewTmuxHandler()
	mux = http.NewServeMux()
	handler.RegisterRoutes(mux)
	req = httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/velis/recover?unixUser=alice", bytes.NewBufferString(`{"topologyOnly":true}`))
	req.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	if len(calls) == 0 {
		t.Fatalf("tmux calls = %#v, want topology creation", calls)
	}
	for _, call := range calls {
		if len(call) > 2 && call[2] == "send-keys" {
			t.Fatalf("topology-only recovery launched a process: %#v", calls)
		}
	}
}

func TestTmuxHandler_RecoverBankedMixedSameOwnerUnresolvedPlanTopologyOnlyCreatesNoLaunches(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	plan := []WorkloadRecoveryDescriptor{
		sessionBankAgentDescriptor("velis", "alice", RecoveryAgentCodex, recoveryTestCodexID, "", 0, 0, "%1", "agents", "b25f,80x24,0,0[80x12,0,0,1,80x11,0,13,2]", "/home/alice/velis"),
		sessionBankUnresolvedDescriptor("velis", "alice", 0, 1, "%2", "agents", "b25f,80x24,0,0[80x12,0,0,1,80x11,0,13,2]", "/home/alice/velis/unknown"),
	}
	writeBankSeedRaw(t, bankPath, sessionBankEntryWithRecoveryPlanJSON(t, "velis", "alice", 1, plan))
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	argsPath := installSessionBankRecoveryTmux(t, "")
	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/velis/recover?unixUser=alice", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "unresolved") {
		t.Fatalf("error body = %s, want unresolved failure before tmux", recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want none before unresolved plan opt-in", got)
	}

	argsPath = installSessionBankRecoveryTmux(t, `
case "$*" in
  *has-session*) echo 'no server running on /tmp/tmux-a' >&2; exit 1 ;;
esac
`)
	handler = NewTmuxHandler()
	mux = http.NewServeMux()
	handler.RegisterRoutes(mux)
	req = httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/velis/recover?unixUser=alice", bytes.NewBufferString(`{"topologyOnly":true}`))
	req.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	if len(calls) == 0 {
		t.Fatalf("tmux calls = %#v, want topology creation", calls)
	}
	for _, call := range calls {
		if len(call) > 2 && call[2] == "send-keys" {
			t.Fatalf("topology-only mixed plan launched a process: %#v", calls)
		}
	}
}

func TestTmuxHandler_RecoverBankedPlanSkipsExistingLiveSessionIdempotently(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installSessionBankRecoveryTmux(t, "")
	plan := []WorkloadRecoveryDescriptor{
		sessionBankAgentDescriptor("velis", "alice", RecoveryAgentCodex, recoveryTestCodexID, "", 0, 0, "%1", "agents", "b25f,80x24,0,0", "/home/alice/velis"),
	}
	writeBankSeedRaw(t, bankPath, sessionBankEntryWithRecoveryPlanJSON(t, "velis", "alice", 1, plan))
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/velis/recover?unixUser=alice", nil)
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["success"] != true || response["action"] != "skip-live" || response["session"] != "velis" {
		t.Fatalf("recover response = %#v", response)
	}
	want := [][]string{{"-S", "/tmp/tmux-a", "has-session", "-t", "velis"}}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); !equalArgvCalls(got, want) {
		t.Fatalf("tmux calls = %#v, want only live-session check %#v", got, want)
	}
}

func TestTmuxHandler_RecoverBankedPlanCleansOnlyCreatedSessionAfterPartialFailure(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installSessionBankRecoveryTmux(t, `
case "$*" in
  *has-session*) echo 'no server running on /tmp/tmux-a' >&2; exit 1 ;;
  *broken-layout*) echo 'bad layout' >&2; exit 1 ;;
esac
`)
	plan := []WorkloadRecoveryDescriptor{
		sessionBankAgentDescriptor("velis", "alice", RecoveryAgentCodex, recoveryTestCodexID, "", 0, 0, "%1", "agents", "b25f,80x24,0,0", "/home/alice/velis"),
		sessionBankPythonDescriptor("velis", "alice", 8088, "/home/alice/velis/public", 1, 0, "%2", "server", "broken-layout", "/home/alice/velis/server"),
	}
	writeBankSeedRaw(t, bankPath, sessionBankEntryWithRecoveryPlanJSON(t, "velis", "alice", 2, plan))
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/velis/recover?unixUser=alice", nil)
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
	wantSuffix := []string{"-S", "/tmp/tmux-a", "if-shell", "-F", "-t", "$42", "#{==:#{CHROTE_CREATION_TOKEN},<token>}", "kill-session -t $42", "display-message -p CHROTE_OWNERSHIP_MISMATCH"}
	calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
	if len(calls) == 0 || strings.Join(calls[len(calls)-1], "\x00") != strings.Join(wantSuffix, "\x00") {
		t.Fatalf("tmux calls = %#v, want owned-session cleanup suffix %#v", calls, wantSuffix)
	}
	for _, call := range calls {
		if len(call) > 0 && call[len(call)-1] == "external-extra" {
			t.Fatalf("cleanup targeted unrelated session: %#v", calls)
		}
	}
}

func TestTmuxHandler_UpdateBankedRecoveryRejectsNonSessionBankDescriptorWithoutStoring(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	desc := sessionBankAgentDescriptor("velis", "alice", RecoveryAgentCodex, recoveryTestCodexID, "", 0, 0, "%1", "agents", "b25f,80x24,0,0", "/home/alice/velis")
	desc.Owner = WorkloadRecoveryOwner{Kind: RecoveryOwnerPersistentAgent, Ref: "persistent:alice/velis", MayRestart: true}
	body := bytes.NewBuffer(sessionBankRecoveryPlanRequestJSON(t, []WorkloadRecoveryDescriptor{desc}))

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/velis/recovery?unixUser=alice", body)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if raw, err := os.ReadFile(bankPath); err == nil && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("bank should not store rejected descriptor, got %s", raw)
	}
}

func TestSessionBankRecoveryPlanBoundsAcceptExactLimits(t *testing.T) {
	plan := sessionBankTopologyPlan("bounds", "alice", 4, 32, 0, 0)
	got, err := validateSessionBankRecoveryPlan("bounds", "alice", "/home/alice", plan, true)
	if err != nil {
		t.Fatalf("validate exact descriptor/pane bounds: %v", err)
	}
	if len(got.Descriptors) != 128 || len(got.Windows) != 4 {
		t.Fatalf("validated plan has %d descriptors and %d windows, want 128 descriptors and 4 windows", len(got.Descriptors), len(got.Windows))
	}

	plan = sessionBankTopologyPlan("bounds", "alice", 32, 1, 0, 0)
	got, err = validateSessionBankRecoveryPlan("bounds", "alice", "/home/alice", plan, true)
	if err != nil {
		t.Fatalf("validate exact window bounds: %v", err)
	}
	if len(got.Windows) != 32 {
		t.Fatalf("validated windows = %d, want 32", len(got.Windows))
	}
}

func TestTmuxHandler_UpdateBankedRecoveryRejectsOverBoundSubmittedPlanWithoutStoring(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	plan := sessionBankTopologyPlan("bounds", "alice", 5, 26, 0, 0)
	body := bytes.NewBuffer(sessionBankRecoveryPlanRequestJSON(t, plan))

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/bounds/recovery?unixUser=alice", body)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "recovery plan descriptors") {
		t.Fatalf("body = %s, want descriptor bound failure", recorder.Body.String())
	}
	if raw, err := os.ReadFile(bankPath); err == nil && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("bank should not store over-bound descriptor plan, got %s", raw)
	}
}

func TestTmuxHandler_RecoverBankedPlanRejectsStoredOverBoundsBeforeTmux(t *testing.T) {
	tests := []struct {
		name string
		plan []WorkloadRecoveryDescriptor
		want string
	}{
		{
			name: "too many windows",
			plan: sessionBankTopologyPlan("bounds", "alice", 33, 1, 0, 0),
			want: "recovery plan windows",
		},
		{
			name: "too many panes in window",
			plan: sessionBankTopologyPlan("bounds", "alice", 1, 33, 0, 0),
			want: "recovery panes for window 0",
		},
		{
			name: "too many descriptors",
			plan: sessionBankTopologyPlan("bounds", "alice", 5, 26, 0, 0),
			want: "recovery plan descriptors",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
			argsPath := installSessionBankRecoveryTmux(t, `
case "$*" in
  *has-session*) echo 'no server running on /tmp/tmux-a' >&2; exit 1 ;;
esac
`)
			writeBankSeedRaw(t, bankPath, sessionBankEntryWithRecoveryPlanJSON(t, "bounds", "alice", 1, tt.plan))
			t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
			t.Setenv("CHROTE_TERMINAL_USERS", "alice")
			t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
			t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
			t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

			handler := NewTmuxHandler()
			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)
			req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/bounds/recover?unixUser=alice", nil)
			recorder := httptest.NewRecorder()

			mux.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), tt.want) {
				t.Fatalf("body = %s, want %q", recorder.Body.String(), tt.want)
			}
			if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
				t.Fatalf("tmux calls = %#v, want none before over-bound stored plan is rejected", got)
			}
		})
	}
}

func TestTmuxHandler_UpdateBankedRecoveryRejectsOversizedBodyWithoutStoring(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	body := `{"agentKind":"codex","agentSessionId":"` + recoveryTestCodexID + `","padding":"` + strings.Repeat("x", 1<<20) + `"}`

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/oversized/recovery", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusRequestEntityTooLarge, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "1048576") {
		t.Fatalf("body = %s, want precise recovery request byte limit", recorder.Body.String())
	}
	if raw, err := os.ReadFile(bankPath); err == nil && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("bank should not store oversized request, got %s", raw)
	}
}

func TestTmuxHandler_UpdateBankedRecoveryRejectsTrailingOverLimitWhitespaceWithoutStoring(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	body := `{"agentKind":"codex","agentSessionId":"` + recoveryTestCodexID + `"}` + strings.Repeat(" ", int(sessionBankRecoveryMaxRequestBytes))

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/trailing/recovery", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusRequestEntityTooLarge, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "1048576") {
		t.Fatalf("body = %s, want precise recovery request byte limit", recorder.Body.String())
	}
	if raw, err := os.ReadFile(bankPath); err == nil && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("bank should not store trailing over-limit request, got %s", raw)
	}
}

func TestTmuxHandler_UpdateBankedRecoveryRejectsSecondJSONValueWithoutStoring(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	body := `{"agentKind":"codex","agentSessionId":"` + recoveryTestCodexID + `"}` +
		`{"agentKind":"codex","agentSessionId":"` + recoveryTestCodexID + `"}`

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/trailing/recovery", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if raw, err := os.ReadFile(bankPath); err == nil && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "[]" {
		t.Fatalf("bank should not store request with a second JSON value, got %s", raw)
	}
}

func TestTmuxHandler_RecoverBankedSessionRejectsOversizedBodyBeforeTmux(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installSessionBankRecoveryTmux(t, `
case "$*" in
  *has-session*) echo 'no server running on /tmp/tmux-a' >&2; exit 1 ;;
esac
`)
	plan := []WorkloadRecoveryDescriptor{
		sessionBankTopologyDescriptor("oversized", "alice", 0, 0, "%1", "server", "b25f,80x24,0,0", "/home/alice/oversized"),
	}
	writeBankSeedRaw(t, bankPath, sessionBankEntryWithRecoveryPlanJSON(t, "oversized", "alice", 1, plan))
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	body := `{"topologyOnly":true,"padding":"` + strings.Repeat("x", 1<<20) + `"}`

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/oversized/recover?unixUser=alice", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusRequestEntityTooLarge, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "1048576") {
		t.Fatalf("body = %s, want precise recovery request byte limit", recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want none before oversized body is rejected", got)
	}
}

func TestTmuxHandler_RecoverBankedSessionRejectsTrailingOverLimitDataBeforeTmux(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installSessionBankRecoveryTmux(t, `
case "$*" in
  *has-session*) echo 'no server running on /tmp/tmux-a' >&2; exit 1 ;;
esac
`)
	plan := []WorkloadRecoveryDescriptor{
		sessionBankTopologyDescriptor("trailing", "alice", 0, 0, "%1", "server", "b25f,80x24,0,0", "/home/alice/trailing"),
	}
	writeBankSeedRaw(t, bankPath, sessionBankEntryWithRecoveryPlanJSON(t, "trailing", "alice", 1, plan))
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	body := `{"topologyOnly":true}` + strings.Repeat("x", int(sessionBankRecoveryMaxRequestBytes))

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/trailing/recover?unixUser=alice", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusRequestEntityTooLarge, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "1048576") {
		t.Fatalf("body = %s, want precise recovery request byte limit", recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want none before trailing over-limit body is rejected", got)
	}
}

func TestTmuxHandler_ForgetBankedAgentSessionDoesNotCallTmux(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installArgvRecordingTmux(t)
	writeBankSeed(t, bankPath, []SessionBankEntry{{
		Name:           "codex-alpha",
		UnixUser:       "alice",
		Group:          "codex",
		FirstSeen:      "2026-07-09T00:00:00Z",
		LastSeen:       "2026-07-09T00:00:00Z",
		AgentKind:      "codex",
		AgentSessionID: "019f45ec-f88b-7f70-88dc-b5b99a9e94c6",
		ResumeCommand:  "codex resume 019f45ec-f88b-7f70-88dc-b5b99a9e94c6",
	}})
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodDelete, "/api/tmux/session-bank/codex-alpha?unixUser=alice", nil)
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want metadata-only forget", got)
	}
}

func TestSessionBankForgetRemovesExactUserEntry(t *testing.T) {
	store := newSessionBankStore(filepath.Join(t.TempDir(), "session-bank", "sessions.json"))
	_, err := store.Snapshot([]core.Session{
		{Name: "codex-alpha", ID: "$7", UnixUser: "alice", Group: "codex", Windows: 1},
		{Name: "codex-alpha", ID: "$8", UnixUser: "bob", Group: "codex", Windows: 1},
	})
	if err != nil {
		t.Fatalf("snapshot bank: %v", err)
	}

	removed, err := store.Forget("codex-alpha", "alice")
	if err != nil {
		t.Fatalf("forget bank entry: %v", err)
	}
	if !removed {
		t.Fatal("removed = false, want true")
	}

	entries, err := store.Read()
	if err != nil {
		t.Fatalf("read bank: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "codex-alpha" || entries[0].UnixUser != "bob" {
		t.Fatalf("bank entries after forget = %+v, want only bob/codex-alpha", entries)
	}

	removed, err = store.Forget("missing", "alice")
	if err != nil {
		t.Fatalf("forget missing bank entry: %v", err)
	}
	if removed {
		t.Fatal("removed missing = true, want false")
	}
}

func TestTmuxHandler_RegisterRoutesWiresSessionBankForget(t *testing.T) {
	bankPath := filepath.Join(t.TempDir(), "session-bank", "sessions.json")
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)

	handler := NewTmuxHandler()
	_, err := handler.bank.Snapshot([]core.Session{{Name: "codex-alpha", ID: "$7", UnixUser: "alice", Group: "codex", Windows: 1}})
	if err != nil {
		t.Fatalf("snapshot bank: %v", err)
	}

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodDelete, "/api/tmux/session-bank/codex-alpha?unixUser=alice", nil)
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode forget response: %v", err)
	}
	if response["removed"] != true {
		t.Fatalf("removed response = %#v, want true", response["removed"])
	}
	entries, err := handler.bank.Read()
	if err != nil {
		t.Fatalf("read bank: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("bank entries = %+v, want empty after forget", entries)
	}
}

func TestTmuxHandler_RestoreBankedSessionEntryReplacesExactLegacyEntryWithoutTmux(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installArgvRecordingTmux(t)
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")
	writeBankSeedRaw(t, bankPath, []byte(`[{
		"name":"codex-alpha",
		"unixUser":"alice",
		"group":"codex",
		"windows":1,
		"attached":false,
		"live":false,
		"firstSeen":"2026-07-09T00:00:00Z",
		"lastSeen":"2026-07-09T00:00:00Z",
		"recoveryPlan":[]
	}]`))

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := `{
		"name":"codex-alpha",
		"unixUser":"alice",
		"group":"codex",
		"windows":1,
		"attached":false,
		"live":false,
		"firstSeen":"2026-07-09T00:00:00Z",
		"lastSeen":"2026-07-09T00:00:00Z",
		"agentKind":"codex",
		"agentSessionId":"019f45ec-f88b-7f70-88dc-b5b99a9e94c6",
		"resumeCommand":"codex resume 019f45ec-f88b-7f70-88dc-b5b99a9e94c6",
		"cwd":"/home/alice/project"
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/tmux/session-bank/codex-alpha/entry?unixUser=alice", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want exact bank restore with no tmux side effects", got)
	}
	raw, err := os.ReadFile(bankPath)
	if err != nil {
		t.Fatalf("read bank: %v", err)
	}
	if bytes.Contains(raw, []byte(`"recoveryPlan"`)) {
		t.Fatalf("restored absent-plan legacy entry should not gain recoveryPlan: %s", raw)
	}
	var entries []SessionBankEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("decode bank: %v", err)
	}
	if len(entries) != 1 || entries[0].RecoveryKind != "agent" || entries[0].ResumeCommand != "codex resume 019f45ec-f88b-7f70-88dc-b5b99a9e94c6" {
		t.Fatalf("entries = %+v, want exact legacy agent metadata restored", entries)
	}
}

func TestTmuxHandler_RestoreBankedSessionEntryPreservesPresentEmptyUnsafePlan(t *testing.T) {
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	argsPath := installArgvRecordingTmux(t)
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	body := `{
		"name":"empty-plan",
		"unixUser":"alice",
		"group":"codex",
		"windows":1,
		"attached":false,
		"live":false,
		"firstSeen":"2026-07-09T00:00:00Z",
		"lastSeen":"2026-07-09T00:00:00Z",
		"agentKind":"codex",
		"agentSessionId":"019f45ec-f88b-7f70-88dc-b5b99a9e94c6",
		"resumeCommand":"codex resume 019f45ec-f88b-7f70-88dc-b5b99a9e94c6",
		"cwd":"/home/alice/project",
		"recoveryPlan":[]
	}`
	req := httptest.NewRequest(http.MethodPut, "/api/tmux/session-bank/empty-plan/entry?unixUser=alice", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, expected %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
		t.Fatalf("tmux calls = %#v, want exact bank restore with no tmux side effects", got)
	}
	raw, err := os.ReadFile(bankPath)
	if err != nil {
		t.Fatalf("read bank: %v", err)
	}
	if !bytes.Contains(raw, []byte(`"recoveryPlan": []`)) && !bytes.Contains(raw, []byte(`"recoveryPlan":[]`)) {
		t.Fatalf("restored present-empty entry lost recoveryPlan presence: %s", raw)
	}
	if bytes.Contains(raw, []byte(`"agentKind"`)) || bytes.Contains(raw, []byte(`codex resume`)) {
		t.Fatalf("present-empty unsafe entry retained legacy resume metadata: %s", raw)
	}
}

func writeBankSeed(t *testing.T, path string, entries []SessionBankEntry) {
	t.Helper()
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal bank seed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir bank dir: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o660); err != nil {
		t.Fatalf("write bank seed: %v", err)
	}
}

func writeBankSeedRaw(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir bank dir: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o660); err != nil {
		t.Fatalf("write raw bank seed: %v", err)
	}
}

func TestManagedRecoveryStatusRejectsCrossFieldContradictions(t *testing.T) {
	valid := ManagedRecoveryStatusEntry{
		Name:        "managed-worker",
		SessionName: "managed-worker",
		UnixUser:    "alice",
		Owner: WorkloadRecoveryOwner{
			Kind:       RecoveryOwnerExternalManager,
			Ref:        "systemd:user/managed-worker.service",
			MayRestart: false,
		},
		ManagerKind: "systemd-user",
		ManagerRef:  "managed-worker.service",
		Status: ManagedRecoveryHealthStatus{
			OK:          true,
			ActiveState: "active",
			CheckedAt:   "2026-07-15T10:00:00Z",
		},
		StorageKind: "managed-status",
		SourceKind:  "restore",
	}

	tests := map[string]func(*ManagedRecoveryStatusEntry){
		"missing unix user":             func(entry *ManagedRecoveryStatusEntry) { entry.UnixUser = "" },
		"owner ref mismatch":            func(entry *ManagedRecoveryStatusEntry) { entry.Owner.Ref = "systemd:user/other.service" },
		"wrong storage kind":            func(entry *ManagedRecoveryStatusEntry) { entry.StorageKind = "arbitrary" },
		"active state marked unhealthy": func(entry *ManagedRecoveryStatusEntry) { entry.Status.OK = false },
		"inactive state marked healthy": func(entry *ManagedRecoveryStatusEntry) { entry.Status.ActiveState = "inactive" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			entry := valid
			mutate(&entry)
			if _, err := normalizeManagedRecoveryStatusEntry(entry, 0); err == nil {
				t.Fatal("expected contradictory managed status entry to be rejected")
			}
		})
	}
}

func TestManagedRecoveryStatusReadRejectsUntrustedFilesystemObjects(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string) string
		want  string
	}{
		{
			name: "symlink",
			setup: func(t *testing.T, dir string) string {
				target := filepath.Join(dir, "target.json")
				writeManagedStatusSeed(t, target, "managed-worker", "alice", "managed-worker.service")
				link := filepath.Join(dir, "managed-status.json")
				if err := os.Symlink(target, link); err != nil {
					t.Fatalf("symlink managed status: %v", err)
				}
				return link
			},
			want: "symlink",
		},
		{
			name: "group-readable",
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "managed-status.json")
				writeManagedStatusSeed(t, path, "managed-worker", "alice", "managed-worker.service")
				if err := os.Chmod(path, 0o640); err != nil {
					t.Fatalf("chmod managed status: %v", err)
				}
				return path
			},
			want: "permissions",
		},
		{
			name: "directory",
			setup: func(t *testing.T, dir string) string {
				path := filepath.Join(dir, "managed-status.json")
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatalf("mkdir managed status path: %v", err)
				}
				return path
			},
			want: "regular file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t, t.TempDir())
			_, err := newManagedRecoveryStatusStore(path).Read()
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tt.want) {
				t.Fatalf("Read() error = %v, want %q trust failure", err, tt.want)
			}
		})
	}
}

func writeManagedStatusSeed(t *testing.T, path, name, unixUser, unit string) {
	t.Helper()
	raw, err := json.Marshal([]map[string]any{{
		"name":        name,
		"sessionName": name,
		"unixUser":    unixUser,
		"owner": map[string]any{
			"kind":       RecoveryOwnerExternalManager,
			"ref":        "systemd:user/" + unit,
			"mayRestart": false,
		},
		"managerKind": "systemd-user",
		"managerRef":  unit,
		"status": map[string]any{
			"ok":          true,
			"activeState": "active",
			"checkedAt":   "2026-07-15T10:00:00Z",
		},
		"storageKind": "managed-status",
		"sourceKind":  "restore",
	}})
	if err != nil {
		t.Fatalf("marshal managed status seed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir managed status dir: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write managed status seed: %v", err)
	}
}

func sessionBankEntryWithRecoveryPlanJSON(t *testing.T, name, unixUser string, windows int, plan []WorkloadRecoveryDescriptor) []byte {
	t.Helper()
	raw, err := json.Marshal([]map[string]any{{
		"name":         name,
		"unixUser":     unixUser,
		"group":        core.CategorizeSession(name),
		"windows":      windows,
		"attached":     false,
		"live":         false,
		"firstSeen":    "2026-07-09T00:00:00Z",
		"lastSeen":     "2026-07-09T00:00:00Z",
		"recoveryPlan": plan,
	}})
	if err != nil {
		t.Fatalf("marshal raw bank seed: %v", err)
	}
	return raw
}

func sessionBankRecoveryPlanRequestJSON(t *testing.T, plan []WorkloadRecoveryDescriptor) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"recoveryPlan": plan})
	if err != nil {
		t.Fatalf("marshal recovery plan request: %v", err)
	}
	return raw
}

func sessionBankTopologyPlan(sessionName, unixUser string, windows, panesPerWindow, startWindow, startPane int) []WorkloadRecoveryDescriptor {
	plan := make([]WorkloadRecoveryDescriptor, 0, windows*panesPerWindow)
	paneID := 1
	for w := 0; w < windows; w++ {
		for p := 0; p < panesPerWindow; p++ {
			plan = append(plan, sessionBankTopologyDescriptor(
				sessionName,
				unixUser,
				startWindow+w,
				startPane+p,
				fmt.Sprintf("%%%d", paneID),
				fmt.Sprintf("w%d", startWindow+w),
				fmt.Sprintf("layout-%d", startWindow+w),
				fmt.Sprintf("/home/alice/%s/w%d/p%d", sessionName, startWindow+w, startPane+p),
			))
			paneID++
		}
	}
	return plan
}

func sessionBankAgentDescriptor(sessionName, unixUser, kind, sessionID, profile string, windowIndex, paneIndex int, paneID, windowName, layout, cwd string) WorkloadRecoveryDescriptor {
	desc := sessionBankBaseDescriptor(sessionName, unixUser, windowIndex, paneIndex, paneID, windowName, layout, cwd)
	desc.Mode = RecoveryModeAgent
	desc.WorkloadKind = kind
	desc.Agent = &WorkloadRecoveryAgent{
		Kind:            kind,
		NativeSessionID: sessionID,
		HermesProfile:   profile,
	}
	desc.EvidenceSource = RecoveryEvidenceArgv
	desc.Confidence = RecoveryConfidenceHigh
	return desc
}

func sessionBankPythonDescriptor(sessionName, unixUser string, port int, directory string, windowIndex, paneIndex int, paneID, windowName, layout, cwd string) WorkloadRecoveryDescriptor {
	desc := sessionBankBaseDescriptor(sessionName, unixUser, windowIndex, paneIndex, paneID, windowName, layout, cwd)
	desc.Mode = RecoveryModeCommand
	desc.WorkloadKind = RecoveryWorkloadPythonHTTPServer
	desc.Command = &WorkloadRecoveryCommand{
		Kind: RecoveryCommandPythonHTTPServer,
		PythonHTTPServer: &PythonHTTPServerRecoveryCommand{
			Bind:      "127.0.0.1",
			Port:      port,
			Directory: directory,
		},
	}
	desc.EvidenceSource = RecoveryEvidenceArgv
	desc.Confidence = RecoveryConfidenceHigh
	return desc
}

func sessionBankTopologyDescriptor(sessionName, unixUser string, windowIndex, paneIndex int, paneID, windowName, layout, cwd string) WorkloadRecoveryDescriptor {
	desc := sessionBankBaseDescriptor(sessionName, unixUser, windowIndex, paneIndex, paneID, windowName, layout, cwd)
	desc.Mode = RecoveryModeTopology
	desc.WorkloadKind = RecoveryWorkloadShell
	desc.EvidenceSource = RecoveryEvidenceTopology
	desc.Confidence = RecoveryConfidenceMedium
	return desc
}

func sessionBankUnresolvedDescriptor(sessionName, unixUser string, windowIndex, paneIndex int, paneID, windowName, layout, cwd string) WorkloadRecoveryDescriptor {
	desc := sessionBankBaseDescriptor(sessionName, unixUser, windowIndex, paneIndex, paneID, windowName, layout, cwd)
	desc.Mode = RecoveryModeUnresolved
	desc.Owner.MayRestart = false
	desc.WorkloadKind = RecoveryWorkloadUnknown
	desc.UnresolvedReason = RecoveryUnresolvedUnknownProcess
	desc.EvidenceSource = RecoveryEvidenceProcess
	desc.Confidence = RecoveryConfidenceLow
	return desc
}

func sessionBankBaseDescriptor(sessionName, unixUser string, windowIndex, paneIndex int, paneID, windowName, layout, cwd string) WorkloadRecoveryDescriptor {
	return WorkloadRecoveryDescriptor{
		Owner: WorkloadRecoveryOwner{
			Kind:       RecoveryOwnerSessionBank,
			Ref:        sessionBankOwnerRef(unixUser, sessionName),
			MayRestart: true,
		},
		Topology: WorkloadRecoveryTopology{
			SessionName:     sessionName,
			WindowIndex:     windowIndex,
			WindowName:      windowName,
			WindowLayout:    layout,
			PaneIndex:       paneIndex,
			PaneID:          paneID,
			PaneCurrentPath: cwd,
		},
	}
}

func installArgvRecordingTmux(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "tmux-argv.txt")
	scriptPath := filepath.Join(dir, "tmux")
	markerPath := filepath.Join(dir, "tmux-session-marker.txt")
	script := `#!/bin/sh
for arg in "$@"; do
  printf '%s\n' "$arg" >> "$TMUX_ARGS_FILE"
done
printf '%s\n' '---' >> "$TMUX_ARGS_FILE"
case "$*" in
  *list-sessions*) printf '' ;;
  *new-session*) printf '$42\n' ;;
esac
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	if err := os.WriteFile(argsPath, nil, 0o600); err != nil {
		t.Fatalf("write args log: %v", err)
	}
	t.Setenv("TMUX_ARGS_FILE", argsPath)
	t.Setenv("TMUX_SESSION_MARKER_FILE", markerPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsPath
}

func installSessionBankRecoveryTmux(t *testing.T, behavior string) string {
	t.Helper()
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "tmux-argv.txt")
	scriptPath := filepath.Join(dir, "tmux")
	markerPath := filepath.Join(dir, "tmux-session-marker.txt")
	splitCountPath := filepath.Join(dir, "tmux-split-count.txt")
	script := `#!/bin/sh
for arg in "$@"; do
  printf '%s\n' "$arg" >> "$TMUX_ARGS_FILE"
done
printf '%s\n' '---' >> "$TMUX_ARGS_FILE"
` + behavior + `
case "$*" in
  *new-session*)
    for arg in "$@"; do
      case "$arg" in
        CHROTE_CREATION_TOKEN=*) printf '%s' "$arg" > "$TMUX_SESSION_MARKER_FILE" ;;
      esac
    done
    case "$*" in
      *window_id*) printf '$42\t@17\t%%24\n' ;;
      *) printf '$42\n' ;;
    esac
    ;;
  *new-window*)
    printf '@18\t%%31\n'
    ;;
  *split-window*)
    count=0
    if [ -f "$TMUX_SPLIT_COUNT_FILE" ]; then
      count=$(cat "$TMUX_SPLIT_COUNT_FILE")
    fi
    count=$((count + 1))
    printf '%s' "$count" > "$TMUX_SPLIT_COUNT_FILE"
    case "$count" in
      1) printf '%%44\n' ;;
      2) printf '%%45\n' ;;
      3) printf '%%46\n' ;;
      *) printf '%%99\n' ;;
    esac
    ;;
  *if-shell*)
    if [ -f "$TMUX_SESSION_MARKER_FILE" ]; then
      rm -f "$TMUX_SESSION_MARKER_FILE"
    else
      echo "can't find session" >&2
      exit 1
    fi
    ;;
esac
exit 0
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	if err := os.WriteFile(argsPath, nil, 0o600); err != nil {
		t.Fatalf("write args log: %v", err)
	}
	t.Setenv("TMUX_ARGS_FILE", argsPath)
	t.Setenv("TMUX_SESSION_MARKER_FILE", markerPath)
	t.Setenv("TMUX_SPLIT_COUNT_FILE", splitCountPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsPath
}

func readArgvRecordingTmuxCalls(t *testing.T, argsPath string) [][]string {
	t.Helper()
	raw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read tmux argv log: %v", err)
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	lines := strings.Split(trimmed, "\n")
	calls := [][]string{}
	current := []string{}
	for _, line := range lines {
		if line == "---" {
			if len(current) > 0 {
				calls = append(calls, current)
				current = []string{}
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		calls = append(calls, current)
	}
	return calls
}

func equalArgvCalls(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.Join(a[i], "\x00") != strings.Join(b[i], "\x00") {
			return false
		}
	}
	return true
}

func containsArgvCall(calls [][]string, want []string) bool {
	for _, call := range calls {
		if strings.Join(call, "\x00") == strings.Join(want, "\x00") {
			return true
		}
	}
	return false
}

func containsArg(call []string, want string) bool {
	for _, arg := range call {
		if arg == want {
			return true
		}
	}
	return false
}

func assertNoLogicalTmuxIndexTargets(t *testing.T, calls [][]string) {
	t.Helper()
	for _, call := range calls {
		for i := 0; i < len(call)-1; i++ {
			if call[i] != "-t" {
				continue
			}
			target := call[i+1]
			if strings.Contains(target, ":0") || strings.Contains(target, ":1") {
				t.Fatalf("tmux target %q uses logical window/pane index instead of captured ID in calls %#v", target, calls)
			}
		}
	}
}

var (
	tmuxCreationTokenTestPattern = regexp.MustCompile(`CHROTE_CREATION_TOKEN=[0-9a-f]+`)
	tmuxRawTokenTestPattern      = regexp.MustCompile(`\b[0-9a-f]{24}\b`)
)

func normalizeTmuxCreationToken(value string) string {
	value = tmuxCreationTokenTestPattern.ReplaceAllString(value, "CHROTE_CREATION_TOKEN=<token>")
	return tmuxRawTokenTestPattern.ReplaceAllString(value, "<token>")
}

func normalizeFakeTmuxCreationTokens(calls []string) []string {
	normalized := make([]string, len(calls))
	for i, call := range calls {
		normalized[i] = normalizeTmuxCreationToken(call)
	}
	return normalized
}

func normalizeArgvTmuxCreationTokens(calls [][]string) [][]string {
	normalized := make([][]string, len(calls))
	for i, call := range calls {
		normalized[i] = make([]string, len(call))
		for j, arg := range call {
			normalized[i][j] = normalizeTmuxCreationToken(arg)
		}
	}
	return normalized
}

func waitForTestFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func writePersistentAgentSeed(t *testing.T, path string, entries []PersistentAgentEntry) {
	t.Helper()
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal persistent seed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir persistent dir: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o660); err != nil {
		t.Fatalf("write persistent seed: %v", err)
	}
}

func installPersistentAgentScriptedTmux(t *testing.T, behavior string) string {
	t.Helper()
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "tmux-argv.txt")
	scriptPath := filepath.Join(dir, "tmux")
	markerPath := filepath.Join(dir, "tmux-session-marker.txt")
	script := `#!/bin/sh
for arg in "$@"; do
  printf '%s\n' "$arg" >> "$TMUX_ARGS_FILE"
done
printf '%s\n' '---' >> "$TMUX_ARGS_FILE"
` + behavior + `
case "$*" in
  *new-session*)
    for arg in "$@"; do
      case "$arg" in
        CHROTE_CREATION_TOKEN=*) printf '%s' "$arg" > "$TMUX_SESSION_MARKER_FILE" ;;
      esac
    done
    printf '$42\n'
    ;;
  *if-shell*)
    if [ -f "$TMUX_SESSION_MARKER_FILE" ]; then
      rm -f "$TMUX_SESSION_MARKER_FILE"
    else
      echo "can't find session" >&2
      exit 1
    fi
    ;;
esac
exit 0
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	if err := os.WriteFile(argsPath, nil, 0o600); err != nil {
		t.Fatalf("write args log: %v", err)
	}
	t.Setenv("TMUX_ARGS_FILE", argsPath)
	t.Setenv("TMUX_SESSION_MARKER_FILE", markerPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsPath
}
