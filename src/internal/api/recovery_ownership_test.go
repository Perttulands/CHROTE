package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chrote/server/internal/core"
)

const recoveryOwnershipTestSession = "codex-alpha"

func TestTmuxHandler_SessionBankPathsRejectPersistentOwnerBeforeMutation(t *testing.T) {
	const unixUser = "alice"
	restoreBody := fmt.Sprintf(`{
		"name":%q,
		"unixUser":%q,
		"group":"codex",
		"windows":1,
		"attached":false,
		"live":false,
		"firstSeen":"2026-07-09T00:00:00Z",
		"lastSeen":"2026-07-09T00:00:00Z",
		"agentKind":"codex",
		"agentSessionId":%q,
		"resumeCommand":%q,
		"cwd":"/home/alice/project"
	}`, recoveryOwnershipTestSession, unixUser, persistentTestCodexID, "codex resume "+persistentTestCodexID)

	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		seedBank []SessionBankEntry
	}{
		{
			name:   "update recovery metadata",
			method: http.MethodPost,
			path:   "/api/tmux/session-bank/codex-alpha/recovery?unixUser=alice",
			body:   fmt.Sprintf(`{"agentKind":"codex","agentSessionId":%q,"cwd":"/home/alice/project"}`, persistentTestCodexID),
		},
		{
			name:   "recover banked session",
			method: http.MethodPost,
			path:   "/api/tmux/session-bank/codex-alpha/recover?unixUser=alice",
			body:   `{"mouseScroll":true}`,
			seedBank: []SessionBankEntry{{
				Name:           recoveryOwnershipTestSession,
				UnixUser:       unixUser,
				Group:          "codex",
				Windows:        1,
				FirstSeen:      "2026-07-09T00:00:00Z",
				LastSeen:       "2026-07-09T00:00:00Z",
				AgentKind:      RecoveryAgentCodex,
				AgentSessionID: persistentTestCodexID,
				CWD:            "/home/alice/project",
			}},
		},
		{
			name:   "restore rollback entry",
			method: http.MethodPut,
			path:   "/api/tmux/session-bank/codex-alpha/entry?unixUser=alice",
			body:   restoreBody,
			seedBank: []SessionBankEntry{{
				Name:      recoveryOwnershipTestSession,
				UnixUser:  unixUser,
				Group:     "codex",
				Windows:   1,
				FirstSeen: "2026-07-08T00:00:00Z",
				LastSeen:  "2026-07-08T00:00:00Z",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
			persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
			argsPath := installArgvRecordingTmux(t)
			writeBankSeed(t, bankPath, tt.seedBank)
			writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
				persistentAgentRawEntry(recoveryOwnershipTestSession, unixUser, RecoveryAgentCodex, persistentTestCodexID, ""),
			})
			beforeBank, err := os.ReadFile(bankPath)
			if err != nil {
				t.Fatalf("read bank before request: %v", err)
			}
			configureRecoveryOwnershipTest(t, bankPath, persistentPath, "")

			handler := NewTmuxHandler()
			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			mux.ServeHTTP(recorder, req)
			assertRecoveryOwnershipError(t, recorder, http.StatusConflict, "SESSION_BANK_OWNERSHIP_CONFLICT", RecoveryOwnerPersistentAgent, persistentAgentOwnerRef(unixUser, recoveryOwnershipTestSession))
			afterBank, err := os.ReadFile(bankPath)
			if err != nil {
				t.Fatalf("read bank after request: %v", err)
			}
			if !bytes.Equal(afterBank, beforeBank) {
				t.Fatalf("Session Bank changed before ownership rejection:\nbefore=%s\nafter=%s", beforeBank, afterBank)
			}
			if got := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(got) != 0 {
				t.Fatalf("tmux calls = %#v, want no fake-tmux side effects before ownership rejection", got)
			}
		})
	}
}

func TestTmuxHandler_SessionBankOwnershipLookupCases(t *testing.T) {
	tests := []struct {
		name           string
		requestUser    string
		persistentUser string
		persistentBad  bool
		managed        bool
		wantStatus     int
		wantCode       string
		wantOwnerKind  string
		wantOwnerRef   string
	}{
		{name: "unowned", requestUser: "alice", wantStatus: http.StatusOK},
		{name: "different user", requestUser: "bob", persistentUser: "alice", wantStatus: http.StatusOK},
		{name: "persistent store read error", requestUser: "alice", persistentBad: true, wantStatus: http.StatusInternalServerError, wantCode: "SESSION_BANK_ERROR"},
		{name: "managed owner", requestUser: "alice", managed: true, wantStatus: http.StatusConflict, wantCode: "SESSION_BANK_OWNERSHIP_CONFLICT", wantOwnerKind: RecoveryOwnerExternalManager, wantOwnerRef: "systemd:user/codex-alpha.service"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
			persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
			managedPath := filepath.Join(tmpDir, "managed", "status.json")
			writeBankSeed(t, bankPath, nil)
			if tt.persistentBad {
				if err := os.MkdirAll(filepath.Dir(persistentPath), 0o755); err != nil {
					t.Fatalf("create persistent store directory: %v", err)
				}
				if err := os.WriteFile(persistentPath, []byte(`{"name":`), 0o600); err != nil {
					t.Fatalf("write malformed persistent store: %v", err)
				}
			} else if tt.persistentUser != "" {
				writePersistentAgentRawSeed(t, persistentPath, []map[string]any{
					persistentAgentRawEntry(recoveryOwnershipTestSession, tt.persistentUser, RecoveryAgentCodex, persistentTestCodexID, ""),
				})
			}
			if tt.managed {
				writeManagedStatusSeed(t, managedPath, recoveryOwnershipTestSession, tt.requestUser, "codex-alpha.service")
			}
			beforeBank, err := os.ReadFile(bankPath)
			if err != nil {
				t.Fatalf("read bank before request: %v", err)
			}
			configureRecoveryOwnershipTest(t, bankPath, persistentPath, managedPath)

			handler := NewTmuxHandler()
			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)
			body := fmt.Sprintf(`{"agentKind":"codex","agentSessionId":%q,"cwd":"/home/%s/project"}`, persistentTestCodexID, tt.requestUser)
			req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/codex-alpha/recovery?unixUser="+tt.requestUser, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			mux.ServeHTTP(recorder, req)
			if tt.wantOwnerKind != "" {
				assertRecoveryOwnershipError(t, recorder, tt.wantStatus, tt.wantCode, tt.wantOwnerKind, tt.wantOwnerRef)
			} else if recorder.Code != tt.wantStatus {
				t.Fatalf("status code = %d, want %d; body=%s", recorder.Code, tt.wantStatus, recorder.Body.String())
			} else if tt.wantCode != "" {
				assertAPIErrorCode(t, recorder, tt.wantCode)
			}
			if tt.wantStatus != http.StatusOK {
				afterBank, err := os.ReadFile(bankPath)
				if err != nil {
					t.Fatalf("read bank after rejected request: %v", err)
				}
				if !bytes.Equal(afterBank, beforeBank) {
					t.Fatalf("Session Bank changed after failed ownership lookup:\nbefore=%s\nafter=%s", beforeBank, afterBank)
				}
			}
		})
	}
}

func TestTmuxHandler_PersistentAgentOwnershipLookupCases(t *testing.T) {
	tests := []struct {
		name          string
		bankOwnerUser string
		legacyBank    bool
		bankBad       bool
		managed       bool
		wantStatus    int
		wantCode      string
		wantOwnerKind string
		wantOwnerRef  string
	}{
		{name: "unowned", wantStatus: http.StatusOK},
		{name: "different user", bankOwnerUser: "bob", wantStatus: http.StatusOK},
		{name: "session bank descriptor owner", bankOwnerUser: "alice", wantStatus: http.StatusConflict, wantCode: "PERSISTENT_AGENT_OWNERSHIP_CONFLICT", wantOwnerKind: RecoveryOwnerSessionBank, wantOwnerRef: "alice/codex-alpha"},
		{name: "legacy session bank owner", bankOwnerUser: "alice", legacyBank: true, wantStatus: http.StatusConflict, wantCode: "PERSISTENT_AGENT_OWNERSHIP_CONFLICT", wantOwnerKind: RecoveryOwnerSessionBank, wantOwnerRef: "alice/codex-alpha"},
		{name: "session bank store read error", bankBad: true, wantStatus: http.StatusInternalServerError, wantCode: "PERSISTENT_AGENT_ERROR"},
		{name: "managed owner", managed: true, wantStatus: http.StatusConflict, wantCode: "PERSISTENT_AGENT_OWNERSHIP_CONFLICT", wantOwnerKind: RecoveryOwnerExternalManager, wantOwnerRef: "systemd:user/codex-alpha.service"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
			persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
			managedPath := filepath.Join(tmpDir, "managed", "status.json")
			argsPath := installPersistentAgentScriptedTmux(t, `
case "$*" in
  *display-message*) printf '42:node:/home/alice/project\n' ;;
esac
`)
			if tt.bankBad {
				if err := os.MkdirAll(filepath.Dir(bankPath), 0o755); err != nil {
					t.Fatalf("create bank directory: %v", err)
				}
				if err := os.WriteFile(bankPath, []byte(`{"name":`), 0o600); err != nil {
					t.Fatalf("write malformed bank store: %v", err)
				}
			} else if tt.bankOwnerUser == "" {
				writeBankSeed(t, bankPath, nil)
			} else if tt.legacyBank {
				writeBankSeed(t, bankPath, []SessionBankEntry{{
					Name:           recoveryOwnershipTestSession,
					UnixUser:       tt.bankOwnerUser,
					Group:          "codex",
					Windows:        1,
					FirstSeen:      "2026-07-09T00:00:00Z",
					LastSeen:       "2026-07-09T00:00:00Z",
					AgentKind:      RecoveryAgentCodex,
					AgentSessionID: persistentTestCodexID,
					CWD:            "/home/alice/project",
				}})
			} else {
				plan := []WorkloadRecoveryDescriptor{
					sessionBankAgentDescriptor(recoveryOwnershipTestSession, tt.bankOwnerUser, RecoveryAgentCodex, persistentTestCodexID, "", 0, 0, "%1", "agents", "b25f,80x24,0,0", "/home/alice/project"),
				}
				writeBankSeedRaw(t, bankPath, sessionBankEntryWithRecoveryPlanJSON(t, recoveryOwnershipTestSession, tt.bankOwnerUser, 1, plan))
			}
			if tt.managed {
				writeManagedStatusSeed(t, managedPath, recoveryOwnershipTestSession, "alice", "codex-alpha.service")
			}
			configureRecoveryOwnershipTest(t, bankPath, persistentPath, managedPath)
			installProcessTable(t, []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex resume " + persistentTestCodexID}})

			handler := NewTmuxHandler()
			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)
			body := persistentAgentRequestJSON(t, map[string]any{
				"recoveryDescriptor": persistentAgentTestDescriptor(recoveryOwnershipTestSession, "alice", RecoveryAgentCodex, persistentTestCodexID, ""),
			})
			req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			mux.ServeHTTP(recorder, req)
			if tt.wantOwnerKind != "" {
				assertRecoveryOwnershipError(t, recorder, tt.wantStatus, tt.wantCode, tt.wantOwnerKind, tt.wantOwnerRef)
			} else if recorder.Code != tt.wantStatus {
				t.Fatalf("status code = %d, want %d; body=%s", recorder.Code, tt.wantStatus, recorder.Body.String())
			} else if tt.wantCode != "" {
				assertAPIErrorCode(t, recorder, tt.wantCode)
			}
			calls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath))
			if tt.wantStatus != http.StatusOK && len(calls) != 0 {
				t.Fatalf("fake-tmux calls = %#v, want no side effects before ownership rejection", calls)
			}
			if tt.wantStatus != http.StatusOK {
				if raw, err := os.ReadFile(persistentPath); err == nil && strings.TrimSpace(string(raw)) != "" && strings.TrimSpace(string(raw)) != "[]" {
					t.Fatalf("persistent store mutated after rejected claim: %s", raw)
				}
			}
		})
	}
}

func TestTmuxHandler_ConcurrentSessionBankAndPersistentClaimsHaveOneWinner(t *testing.T) {
	const unixUser = "alice"
	tmpDir := t.TempDir()
	bankPath := filepath.Join(tmpDir, "session-bank", "sessions.json")
	persistentPath := filepath.Join(tmpDir, "persistent-agents", "agents.json")
	barrierReadyPath := filepath.Join(tmpDir, "persistent-preflight-complete")
	barrierReleasePath := filepath.Join(tmpDir, "release-persistent-claim")
	writeBankSeed(t, bankPath, nil)
	argsPath := installPersistentAgentScriptedTmux(t, fmt.Sprintf(`
case "$*" in
  *display-message*)
    : > %q
    while [ ! -f %q ]; do sleep 0.01; done
    printf '42:node:/home/alice/project\n'
    ;;
esac
`, barrierReadyPath, barrierReleasePath))
	configureRecoveryOwnershipTest(t, bankPath, persistentPath, "")
	installProcessTable(t, []processInfo{{pid: "42", ppid: "1", comm: "node", args: "node /usr/bin/codex resume " + persistentTestCodexID}})

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	type claimResult struct {
		kind string
		code int
		body string
	}
	results := make(chan claimResult, 2)
	persistentBody := persistentAgentRequestJSON(t, map[string]any{
		"recoveryDescriptor": persistentAgentTestDescriptor(recoveryOwnershipTestSession, unixUser, RecoveryAgentCodex, persistentTestCodexID, ""),
	})
	go func() {
		req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/codex-alpha/persistence?unixUser=alice", bytes.NewReader(persistentBody))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, req)
		results <- claimResult{kind: RecoveryOwnerPersistentAgent, code: recorder.Code, body: recorder.Body.String()}
	}()
	waitForTestFile(t, barrierReadyPath)

	bankStarted := make(chan struct{})
	go func() {
		close(bankStarted)
		body := fmt.Sprintf(`{"agentKind":"codex","agentSessionId":%q,"cwd":"/home/alice/project"}`, persistentTestCodexID)
		req := httptest.NewRequest(http.MethodPost, "/api/tmux/session-bank/codex-alpha/recovery?unixUser=alice", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, req)
		results <- claimResult{kind: RecoveryOwnerSessionBank, code: recorder.Code, body: recorder.Body.String()}
	}()
	<-bankStarted
	if err := os.WriteFile(barrierReleasePath, []byte("release"), 0o600); err != nil {
		t.Fatalf("release persistent claim barrier: %v", err)
	}

	got := make([]claimResult, 0, 2)
	for len(got) < 2 {
		select {
		case result := <-results:
			got = append(got, result)
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for concurrent claims; results=%+v", got)
		}
	}
	winners := 0
	conflicts := 0
	for _, result := range got {
		switch result.code {
		case http.StatusOK:
			winners++
		case http.StatusConflict:
			conflicts++
		default:
			t.Fatalf("%s claim status = %d; body=%s", result.kind, result.code, result.body)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("claim results = %+v, want exactly one winner and one ownership conflict", got)
	}

	persistentEntries, err := handler.persistent.Read()
	if err != nil {
		t.Fatalf("read persistent entries: %v", err)
	}
	bankEntries, err := handler.bank.Read()
	if err != nil {
		t.Fatalf("read bank entries: %v", err)
	}
	if len(persistentEntries)+len(bankEntries) != 1 {
		t.Fatalf("persistent entries = %+v, bank entries = %+v; want exactly one stored recovery owner", persistentEntries, bankEntries)
	}
	if gotCalls := normalizeArgvTmuxCreationTokens(readArgvRecordingTmuxCalls(t, argsPath)); len(gotCalls) != 1 {
		t.Fatalf("fake-tmux calls = %#v, want only the persistent claim inspection", gotCalls)
	}
}

func configureRecoveryOwnershipTest(t *testing.T, bankPath, persistentPath, managedPath string) {
	t.Helper()
	t.Setenv("CHROTE_SESSION_BANK_PATH", bankPath)
	t.Setenv("CHROTE_PERSISTENT_AGENTS_PATH", persistentPath)
	t.Setenv("CHROTE_MANAGED_RECOVERY_STATUS_PATH", managedPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice,bob")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a,bob=/tmp/tmux-b")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice/project,bob=/home/bob/project")
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", "alice=/home/alice,bob=/home/bob")
}

func assertRecoveryOwnershipError(t *testing.T, recorder *httptest.ResponseRecorder, wantStatus int, wantCode, wantOwnerKind, wantOwnerRef string) {
	t.Helper()
	if recorder.Code != wantStatus {
		t.Fatalf("status code = %d, want %d; body=%s", recorder.Code, wantStatus, recorder.Body.String())
	}
	response := decodeRecoveryOwnershipAPIResponse(t, recorder)
	if response.Error == nil || response.Error.Code != wantCode {
		t.Fatalf("error = %#v, want code %q", response.Error, wantCode)
	}
	if !strings.Contains(response.Error.Message, wantOwnerKind) || !strings.Contains(response.Error.Message, wantOwnerRef) {
		t.Fatalf("error message = %q, want non-sensitive owner kind %q and ref %q", response.Error.Message, wantOwnerKind, wantOwnerRef)
	}
}

func assertAPIErrorCode(t *testing.T, recorder *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	response := decodeRecoveryOwnershipAPIResponse(t, recorder)
	if response.Error == nil || response.Error.Code != wantCode {
		t.Fatalf("error = %#v, want code %q", response.Error, wantCode)
	}
}

func decodeRecoveryOwnershipAPIResponse(t *testing.T, recorder *httptest.ResponseRecorder) core.APIResponse {
	t.Helper()
	var response core.APIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode API response: %v", err)
	}
	return response
}
