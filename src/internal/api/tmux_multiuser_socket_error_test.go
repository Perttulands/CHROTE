package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chrote/server/internal/core"
)

// installSelectiveTmux writes a fake tmux that fails only for sockets whose path
// contains failFor, and otherwise reports one session named after the socket.
//
// The pre-existing baseline test installs a tmux that fails for EVERY call, which
// cannot express the case that matters on a multi-user host: one user's socket
// unreadable while another's is fine.
func installSelectiveTmux(t *testing.T, failFor string, stderr string) {
	t.Helper()

	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"sock=\"\"\n" +
		"while [ $# -gt 0 ]; do\n" +
		"  case \"$1\" in\n" +
		"    -S) sock=\"$2\"; shift 2 ;;\n" +
		"    *) shift ;;\n" +
		"  esac\n" +
		"done\n" +
		"case \"$sock\" in\n" +
		"  *" + failFor + "*) printf '%s\\n' \"$TMUX_STDERR\" >&2; exit 1 ;;\n" +
		"esac\n" +
		"printf '%s	%s	%s	%s	%s	%s	%s	%s\\n' '9001' '1700000000' \"$sock\" '$7' 'healthy-session' '1' '0' '/workspaces/healthy'\n" +
		"exit 0\n"
	scriptPath := filepath.Join(dir, "tmux")
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		t.Fatalf("write selective fake tmux: %v", err)
	}
	t.Setenv("TMUX_STDERR", stderr)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CHROTE_SESSION_BANK_PATH", filepath.Join(dir, "session-bank.json"))
	t.Setenv("CHROTE_MANAGED_RECOVERY_STATUS_PATH", filepath.Join(dir, "managed-status.json"))
}

// The production path on a multi-user host is the CHROTE_TERMINAL_USERS loop, and
// it is the ONLY place a socket error is prefixed with the unix user. Without
// CHROTE_TERMINAL_USERS set, configuredTerminalUsers() returns empty and
// ListSessions takes the single-target branch instead -- which is what every
// pre-existing test exercised, so the branch that actually runs in production had
// no coverage and "the error names the effective user" was unproven.
func TestTmuxHandler_ListSessionsNamesTheUnixUserWhoseSocketFailed(t *testing.T) {
	installSelectiveTmux(t, "denied", "error connecting to /tmp/chrote-tmux-test/build.sock (Permission denied)")
	t.Setenv("CHROTE_TERMINAL_USERS", "alice,build")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/fixture-alice/ok.sock,build=/tmp/fixture-build/denied.sock")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/workspaces/alice,build=/workspaces/build")

	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	rec := httptest.NewRecorder()

	handler.ListSessions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response SessionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !strings.Contains(response.Error, "build: tmux source permission denied") {
		t.Fatalf("error = %q, want the redacted permission failure to be visible", response.Error)
	}
	// The whole point: which user is broken must be identifiable. An unattributed
	// permission error on a host with several users is not actionable.
	//
	// Fake users are paired with fake workdirs so targetForUnixUser can exercise the
	// configured multi-user branch without relying on host accounts.
	if !strings.Contains(response.Error, "build") {
		t.Fatalf("error = %q, want it to name the unix user whose socket failed", response.Error)
	}
	if strings.Contains(response.Error, "alice") {
		t.Fatalf("error = %q, want the healthy user not to be blamed", response.Error)
	}
}

// A broken socket for one user must not hide another user's live sessions. The API
// deliberately returns partial success -- healthy sessions plus an error naming the
// failures -- because cross-user ACL breakage is the headline failure mode here, and
// an empty list is indistinguishable from "no sessions".
func TestTmuxHandler_ListSessionsStillReturnsHealthyUsersSessionsWhenOneSocketFails(t *testing.T) {
	installSelectiveTmux(t, "denied", "error connecting to /tmp/chrote-tmux-test/build.sock (Permission denied)")
	t.Setenv("CHROTE_TERMINAL_USERS", "alice,build")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/fixture-alice/ok.sock,build=/tmp/fixture-build/denied.sock")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/workspaces/alice,build=/workspaces/build")

	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	rec := httptest.NewRecorder()

	handler.ListSessions(rec, req)

	var response SessionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Sessions) == 0 {
		t.Fatalf("sessions = empty, want the healthy user's sessions preserved alongside the error %q", response.Error)
	}
	for _, session := range response.Sessions {
		if session.UnixUser == "build" {
			t.Fatalf("sessions contain the failed user %q: %+v", session.UnixUser, session)
		}
	}
	payload := decodeJSONMap(t, rec)
	if partial, ok := payload["partial"].(bool); !ok || !partial {
		t.Fatalf("partial = %#v, want true for healthy sessions plus a per-user error", payload["partial"])
	}
}

// When every configured user's socket fails, none of the returned session data
// is authoritative. The partial marker must stay absent so dashboard clients
// preserve their last-known-good state.
func TestTmuxHandler_ListSessionsDoesNotMarkTotalMultiUserFailurePartial(t *testing.T) {
	installSelectiveTmux(t, "fixture-", "error connecting to /tmp/chrote-tmux-test/socket (Permission denied)")
	t.Setenv("CHROTE_TERMINAL_USERS", "alice,build")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/fixture-alice/denied.sock,build=/tmp/fixture-build/denied.sock")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/workspaces/alice,build=/workspaces/build")

	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	rec := httptest.NewRecorder()

	handler.ListSessions(rec, req)

	payload := decodeJSONMap(t, rec)
	if _, ok := payload["partial"]; ok {
		t.Fatalf("partial = %#v, want the marker omitted when every configured user failed", payload["partial"])
	}
	if sessions, ok := payload["sessions"].([]interface{}); !ok || len(sessions) != 0 {
		t.Fatalf("sessions = %#v, want none when every configured user failed", payload["sessions"])
	}
	errorText, _ := payload["error"].(string)
	if !strings.Contains(errorText, "alice:") || !strings.Contains(errorText, "build:") {
		t.Fatalf("error = %q, want both failed fake users named", errorText)
	}
}

// A per-user partial marker covers only tmux socket failures. If another
// sessions-response subsystem also fails, the combined payload is not
// authoritative and must retain total-failure semantics.
func TestTmuxHandler_ListSessionsDoesNotMarkGlobalFailurePartial(t *testing.T) {
	tests := []struct {
		name          string
		pathEnv       string
		errorFragment string
	}{
		{name: "managed status", pathEnv: "CHROTE_MANAGED_RECOVERY_STATUS_PATH", errorFragment: "managed status:"},
		{name: "session bank", pathEnv: "CHROTE_SESSION_BANK_PATH", errorFragment: "session bank:"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			installSelectiveTmux(t, "denied", "error connecting to /tmp/chrote-tmux-test/build.sock (Permission denied)")
			t.Setenv("CHROTE_TERMINAL_USERS", "alice,build")
			t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/fixture-alice/ok.sock,build=/tmp/fixture-build/denied.sock")
			t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/workspaces/alice,build=/workspaces/build")
			t.Setenv(test.pathEnv, t.TempDir())

			handler := NewTmuxHandler()
			req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
			rec := httptest.NewRecorder()

			handler.ListSessions(rec, req)

			payload := decodeJSONMap(t, rec)
			if _, ok := payload["partial"]; ok {
				t.Fatalf("partial = %#v, want the marker omitted when a global subsystem also failed", payload["partial"])
			}
			errorText, _ := payload["error"].(string)
			if !strings.Contains(errorText, test.errorFragment) || !strings.Contains(errorText, "build:") {
				t.Fatalf("error = %q, want both the global and per-user failures preserved", errorText)
			}
		})
	}
}

// A genuinely absent server is not an error. Same loop, so this asserts the
// no-server path stays quiet in the multi-user branch too -- otherwise a user who
// simply has no tmux running would raise a permanent cockpit error.
func TestTmuxHandler_ListSessionsMultiUserNoServerIsNotAnError(t *testing.T) {
	installSelectiveTmux(t, "empty", "no server running on /tmp/fixture-build/empty.sock")
	t.Setenv("CHROTE_TERMINAL_USERS", "alice,build")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/fixture-alice/ok.sock,build=/tmp/fixture-build/empty.sock")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/workspaces/alice,build=/workspaces/build")

	handler := NewTmuxHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil)
	rec := httptest.NewRecorder()

	handler.ListSessions(rec, req)

	var response SessionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Error != "" {
		t.Fatalf("error = %q, want no user-visible error when a user simply has no server", response.Error)
	}
	if len(response.Sessions) == 0 {
		t.Fatalf("sessions = empty, want the running user's sessions listed")
	}
}

func TestTmuxHandler_ListSessionsPublishesQualifiedRecoverySources(t *testing.T) {
	installSelectiveTmux(t, "denied", "error connecting to /tmp/chrote-tmux-test/build.sock (Permission denied)")
	t.Setenv("CHROTE_TERMINAL_USERS", "alice,build")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/fixture-alice/ok.sock,build=/tmp/fixture-build/denied.sock")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/workspaces/alice,build=/workspaces/build")

	rec := httptest.NewRecorder()
	NewTmuxHandler().ListSessions(rec, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	payload := decodeJSONMap(t, rec)
	sources, ok := payload["sources"].([]interface{})
	if !ok || len(sources) != 2 {
		t.Fatalf("sources = %#v, want one qualified result per configured Unix user", payload["sources"])
	}
	byUser := map[string]map[string]interface{}{}
	for _, raw := range sources {
		source, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("source = %T, want object", raw)
		}
		byUser[source["unixUser"].(string)] = source
	}
	if got := byUser["alice"]["status"]; got != "complete" {
		t.Fatalf("alice status = %#v, want complete", got)
	}
	if generation, _ := byUser["alice"]["generation"].(string); generation == "" {
		t.Fatalf("alice generation = %#v, want a stale-precondition token", byUser["alice"]["generation"])
	}
	if got := byUser["build"]["status"]; got != "failed" {
		t.Fatalf("build status = %#v, want failed", got)
	}
	if got := byUser["build"]["errorCode"]; got != "TMUX_SOURCE_UNAVAILABLE" {
		t.Fatalf("build errorCode = %#v, want TMUX_SOURCE_UNAVAILABLE", got)
	}

	evidence, ok := payload["recoveryEvidence"].([]interface{})
	if !ok || len(evidence) == 0 {
		t.Fatalf("recoveryEvidence = %#v, want bounded live/offline evidence", payload["recoveryEvidence"])
	}
}

func TestTmuxHandler_ListSessionsPartialFailurePreservesFailedSourceEvidence(t *testing.T) {
	installSelectiveTmux(t, "denied", "error connecting to /tmp/chrote-tmux-test/build.sock (Permission denied)")
	t.Setenv("CHROTE_TERMINAL_USERS", "alice,build")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/fixture-alice/ok.sock,build=/tmp/fixture-build/denied.sock")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/workspaces/alice,build=/workspaces/build")

	bankPath := os.Getenv("CHROTE_SESSION_BANK_PATH")
	seed := []SessionBankEntry{
		{Name: "alice-old", UnixUser: "alice", Group: "shell", Live: true, FirstSeen: "2026-08-25T10:00:00Z", LastSeen: "2026-08-25T11:00:00Z", CWD: "/workspaces/alice"},
		{Name: "build-agent", UnixUser: "build", Group: "", Live: true, FirstSeen: "2026-08-25T09:00:00Z", LastSeen: "2026-08-25T11:30:00Z", AgentKind: "codex", AgentSessionID: "019f4baa-e368-7ea0-8912-fb2c6f99785c", RecoveryKind: "legacy-kind", ResumeCommand: "legacy unsafe command", CWD: "../legacy-build"},
	}
	raw, err := json.Marshal(seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bankPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	NewTmuxHandler().ListSessions(rec, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	payload := decodeJSONMap(t, rec)
	evidence, ok := payload["recoveryEvidence"].([]interface{})
	if !ok {
		t.Fatalf("recoveryEvidence = %#v, want array", payload["recoveryEvidence"])
	}
	stateByKey := map[string]string{}
	for _, raw := range evidence {
		entry := raw.(map[string]interface{})
		stateByKey[entry["unixUser"].(string)+"/"+entry["name"].(string)] = entry["state"].(string)
	}
	if got := stateByKey["alice/alice-old"]; got != "offline" {
		t.Fatalf("alice/alice-old state = %q, want offline from its complete source", got)
	}
	if got := stateByKey["build/build-agent"]; got != "stale" {
		t.Fatalf("build/build-agent state = %q, want stale from its failed source", got)
	}

	persistedRaw, err := os.ReadFile(bankPath)
	if err != nil {
		t.Fatal(err)
	}
	var rawEntries []SessionBankEntry
	if err := json.Unmarshal(persistedRaw, &rawEntries); err != nil {
		t.Fatal(err)
	}
	for _, entry := range rawEntries {
		if entry.UnixUser == "build" && entry.Name == "build-agent" && !reflect.DeepEqual(entry, seed[1]) {
			t.Fatalf("failed-source raw entry mutated = %+v, want exact last-known record %+v", entry, seed[1])
		}
	}

	persisted, err := newSessionBankStore(bankPath).Read()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range persisted {
		if entry.UnixUser == "build" && entry.Name == "build-agent" {
			if !entry.Live || entry.LastSeen != "2026-08-25T11:30:00Z" {
				t.Fatalf("failed-source entry mutated = %+v, want last-known-good fields preserved", entry)
			}
			return
		}
	}
	t.Fatal("missing persisted build/build-agent evidence")
}

func TestTmuxHandler_ListSessionsMalformedInventoryCannotOfflineEvidence(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "tmux")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nprintf 'truncated-row\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHROTE_TMUX_BIN", scriptPath)
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/fixture-alice/tmux.sock")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/workspaces/alice")
	t.Setenv("CHROTE_SESSION_BANK_PATH", filepath.Join(dir, "session-bank.json"))
	t.Setenv("CHROTE_MANAGED_RECOVERY_STATUS_PATH", filepath.Join(dir, "managed-status.json"))
	seed := []SessionBankEntry{{Name: "important", UnixUser: "alice", Group: "agents", Live: true, FirstSeen: "2026-08-25T10:00:00Z", LastSeen: "2026-08-25T11:00:00Z"}}
	raw, _ := json.Marshal(seed)
	if err := os.WriteFile(os.Getenv("CHROTE_SESSION_BANK_PATH"), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	NewTmuxHandler().ListSessions(rec, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	var response SessionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Sources) != 1 || response.Sources[0].Status != tmuxSourceFailed || !strings.Contains(response.Sources[0].Error, "protocol") {
		t.Fatalf("sources = %+v, want failed protocol evidence", response.Sources)
	}
	if len(response.RecoveryEvidence) != 1 || response.RecoveryEvidence[0].State != recoveryEvidenceStale {
		t.Fatalf("recoveryEvidence = %+v, want stale last-known evidence", response.RecoveryEvidence)
	}
	persisted, err := newSessionBankStore(os.Getenv("CHROTE_SESSION_BANK_PATH")).Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted) != 1 || !persisted[0].Live || persisted[0].LastSeen != "2026-08-25T11:00:00Z" {
		t.Fatalf("persisted evidence mutated after malformed inventory: %+v", persisted)
	}
}

func TestTmuxSourceGenerationIncludesServerIdentity(t *testing.T) {
	sessions := []core.Session{{ID: "$7", Name: "one", UnixUser: "alice", Windows: 1, CWD: "/workspaces/one"}}
	one := tmuxSourceGeneration("alice", sessions, "9001@/tmp/tmux-a")
	two := tmuxSourceGeneration("alice", sessions, "9002@/tmp/tmux-a")
	if one == two {
		t.Fatalf("generation = %q for distinct tmux server identities", one)
	}
}

func TestReadLinuxTmuxServerProcessIdentityResolvesDefaultToCurrentUser(t *testing.T) {
	identity, err := readLinuxTmuxServerProcessIdentity(fmt.Sprint(os.Getpid()), "default")
	if err != nil || !strings.Contains(identity, "uid=") {
		t.Fatalf("default source process identity = %q, %v", identity, err)
	}
}

func TestParseAuthoritativeSessionsOutputIsBoundedAndBindsPhysicalServerIdentity(t *testing.T) {
	if _, _, err := parseAuthoritativeSessionsOutput(strings.Repeat("x", tmuxInventoryMaxBytes+1), "alice", "/tmp/tmux-a"); err == nil || !strings.Contains(err.Error(), "bounded output") {
		t.Fatalf("oversized inventory error = %v", err)
	}
	longCWD := "/" + strings.Repeat("x", tmuxInventoryMaxCWD)
	row := "9001\t1700000000\t/tmp/tmux-a\t$7\tone\t1\t0\t" + longCWD + "\n"
	if _, _, err := parseAuthoritativeSessionsOutput(row, "alice", "/tmp/tmux-a"); err == nil || !strings.Contains(err.Error(), "invalid cwd") {
		t.Fatalf("oversized cwd error = %v", err)
	}

	original := readTmuxServerProcessIdentity
	identity := "boot=a;start=1;uid=1000"
	readTmuxServerProcessIdentity = func(_, _ string) (string, error) { return identity, nil }
	t.Cleanup(func() { readTmuxServerProcessIdentity = original })
	valid := "9001\t1700000000\t/tmp/tmux-a\t$7\tone\t1\t0\t/workspaces/one\n"
	sessions, firstIdentity, err := parseAuthoritativeSessionsOutput(valid, "alice", "/tmp/tmux-a")
	if err != nil {
		t.Fatal(err)
	}
	identity = "boot=b;start=1;uid=1000"
	_, secondIdentity, err := parseAuthoritativeSessionsOutput(valid, "alice", "/tmp/tmux-a")
	if err != nil {
		t.Fatal(err)
	}
	if firstIdentity == secondIdentity || tmuxSourceGeneration("alice", sessions, firstIdentity) == tmuxSourceGeneration("alice", sessions, secondIdentity) {
		t.Fatalf("physical server replacement did not change generation: %q %q", firstIdentity, secondIdentity)
	}
}

func TestNativeEvidenceUsesItsOwnObservationTimestamp(t *testing.T) {
	entry := SessionBankEntry{
		Name: "agent", AgentKind: "codex", AgentSessionID: "11111111-1111-4111-8111-111111111111",
		LastSeen: "2026-08-26T03:00:00Z", NativeEvidenceObservedAt: "2026-08-25T02:00:00Z",
	}
	evidence := nativeEvidenceFromBank(entry)
	if len(evidence) != 1 || evidence[0].ObservedAt != entry.NativeEvidenceObservedAt {
		t.Fatalf("native evidence = %+v, want stable native timestamp", evidence)
	}
	entry.NativeEvidenceObservedAt = ""
	if evidence := nativeEvidenceFromBank(entry); len(evidence) != 1 || evidence[0].ObservedAt != "" {
		t.Fatalf("legacy native evidence = %+v, want no fabricated timestamp", evidence)
	}
}

func TestParseAuthoritativeSessionsOutputRejectsDuplicateIdentity(t *testing.T) {
	output := "9001	1700000000	/tmp/tmux-a	$7	one	1	0	/workspaces/one\n9001	1700000000	/tmp/tmux-a	$7	two	1	0	/workspaces/two\n"
	if _, _, err := parseAuthoritativeSessionsOutput(output, "alice", "/tmp/tmux-a"); err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("error = %v, want duplicate identity rejection", err)
	}
}

func TestParseAuthoritativeSessionsOutputRejectsEmptySuccess(t *testing.T) {
	if _, _, err := parseAuthoritativeSessionsOutput("", "alice", "/tmp/tmux-a"); err == nil || !strings.Contains(err.Error(), "empty output") {
		t.Fatalf("empty authoritative output error = %v", err)
	}
}
