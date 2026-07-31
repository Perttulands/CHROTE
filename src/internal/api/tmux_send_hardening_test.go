package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	osuser "os/user"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

type sendHarness struct {
	dropsDir string
	tmuxLog  string
	aclLog   string
	mux      *http.ServeMux
	handler  *TmuxHandler
}

func newSendHarness(t *testing.T, panes string) sendHarness {
	t.Helper()
	originalLookupUser := tmuxLookupUser
	originalSendSleep := tmuxSendSleep
	tmuxSendSleep = func(context.Context, time.Duration) error { return nil }
	tmuxLookupUser = func(username string) (*osuser.User, error) {
		switch username {
		case "alice":
			return &osuser.User{Username: username, Uid: "1001", HomeDir: "/home/alice"}, nil
		case "bob":
			return &osuser.User{Username: username, Uid: "1002", HomeDir: "/home/bob"}, nil
		default:
			return originalLookupUser(username)
		}
	}
	t.Cleanup(func() {
		tmuxLookupUser = originalLookupUser
		tmuxSendSleep = originalSendSleep
	})
	dir := t.TempDir()
	tmuxLog := filepath.Join(dir, "tmux.log")
	aclLog := filepath.Join(dir, "setfacl.log")
	paneCount := filepath.Join(dir, "pane-count")
	tmuxScript := `#!/bin/sh
for arg in "$@"; do printf '%s\n' "$arg" >> "$TMUX_SEND_LOG"; done
printf '%s\n' '---' >> "$TMUX_SEND_LOG"
while [ "$#" -gt 0 ]; do
  case "$1" in
    -S) shift 2 ;;
    *) break ;;
  esac
done
case "${1:-}" in
  list-panes)
    count=0
    if [ -f "$TMUX_SEND_PANE_COUNT" ]; then count=$(cat "$TMUX_SEND_PANE_COUNT"); fi
    count=$((count + 1)); printf '%s' "$count" > "$TMUX_SEND_PANE_COUNT"
    if [ "$count" -eq 1 ] || [ -z "${TMUX_SEND_PANES_NEXT:-}" ]; then
      printf '%s' "$TMUX_SEND_PANES"
    else
      printf '%s' "$TMUX_SEND_PANES_NEXT"
    fi
    ;;
  load-buffer)
    [ "${TMUX_SEND_FAIL_LOAD:-}" = 1 ] && exit 7
    ;;
  if-shell)
    if [ "${TMUX_SEND_ATOMIC_CHANGED:-}" = 1 ]; then
      printf '%s\n' CHROTE_SEND_TARGET_CHANGED
    elif [ "${TMUX_SEND_SUBMIT_TARGET_CHANGED:-}" = 1 ] && echo " $* " | grep -q " send-keys "; then
      printf '%s\n' CHROTE_SEND_SUBMIT_TARGET_CHANGED
    elif [ "${TMUX_SEND_FAIL_PASTE:-}" = 1 ]; then
      exit 8
    elif [ "${TMUX_SEND_FAIL_SUBMIT:-}" = 1 ] && echo " $* " | grep -q " send-keys "; then
      exit 10
    elif [ -n "${TMUX_SEND_REQUIRE_SETTLE:-}" ] && echo " $* " | grep -q " send-keys " && [ ! -f "$TMUX_SEND_REQUIRE_SETTLE" ]; then
      printf '%s\n' CHROTE_SEND_SUBMIT_NOT_SETTLED
    else
      case " $* " in
        *" send-keys "*) printf '%s\n' CHROTE_SEND_SUBMIT_KEY_DISPATCHED ;;
        *) printf '%s\n' CHROTE_SEND_PASTED ;;
      esac
    fi
    ;;
  delete-buffer)
    [ "${TMUX_SEND_FAIL_DELETE:-}" = 1 ] && exit 9
    ;;
esac
exit 0
`
	aclScript := `#!/bin/sh
for arg in "$@"; do printf '%s\n' "$arg" >> "$TMUX_SEND_ACL_LOG"; done
printf '%s\n' '---' >> "$TMUX_SEND_ACL_LOG"
if [ "${TMUX_SEND_BLOCK_ACL:-}" = 1 ]; then
  block_acl=1
  if [ -n "${TMUX_SEND_BLOCK_ACL_MATCH:-}" ]; then
    block_acl=0
    for arg in "$@"; do
      [ "$arg" = "$TMUX_SEND_BLOCK_ACL_MATCH" ] && block_acl=1
    done
  fi
  if [ "$block_acl" = 1 ]; then
    : > "$TMUX_SEND_ACL_STARTED"
    exec sleep 30
  fi
fi
[ "${TMUX_SEND_FAIL_ACL:-}" = 1 ] && exit 11
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(tmuxScript), 0o700); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "setfacl"), []byte(aclScript), 0o700); err != nil {
		t.Fatalf("write fake setfacl: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMUX_SEND_LOG", tmuxLog)
	t.Setenv("TMUX_SEND_ACL_LOG", aclLog)
	t.Setenv("TMUX_SEND_PANE_COUNT", paneCount)
	t.Setenv("TMUX_SEND_PANES", panes)
	t.Setenv("CHROTE_SESSION_DROPS_DIR", filepath.Join(dir, "drops"))
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return sendHarness{dropsDir: filepath.Join(dir, "drops"), tmuxLog: tmuxLog, aclLog: aclLog, mux: mux, handler: handler}
}

func (h sendHarness) send(t *testing.T, session string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	return h.sendWithQuery(t, session, "", fields)
}

func (h sendHarness) sendWithQuery(t *testing.T, session, rawQuery string, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write %s field: %v", key, err)
		}
	}
	if _, ok := fields["unixUser"]; !ok {
		if err := writer.WriteField("unixUser", "alice"); err != nil {
			t.Fatalf("write unixUser: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	path := "/api/tmux/sessions/" + session + "/send"
	if rawQuery != "" {
		path += "?" + rawQuery
	}
	req := httptest.NewRequest(http.MethodPost, path, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	h.mux.ServeHTTP(recorder, req)
	return recorder
}

func readOptionalFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

func dropDirectoryCount(t *testing.T, root string) int {
	t.Helper()
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatalf("read drop root: %v", err)
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			count++
		}
	}
	return count
}

func assertNoSendSideEffects(t *testing.T, h sendHarness) {
	t.Helper()
	if log := readOptionalFile(t, h.tmuxLog); log != "" {
		t.Fatalf("unexpected tmux calls:\n%s", log)
	}
	if got := dropDirectoryCount(t, h.dropsDir); got != 0 {
		t.Fatalf("unexpected drop directories: %d", got)
	}
}

func TestSendToSessionRequiresExplicitUnixUserWhenMultipleAreConfigured(t *testing.T) {
	h := newSendHarness(t, "$7	one	%41	111	9001	@3	work	/home/alice	bash	1\n")
	t.Setenv("CHROTE_TERMINAL_USERS", "alice,bob")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a,bob=/tmp/tmux-b")

	panes := httptest.NewRecorder()
	h.mux.ServeHTTP(panes, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions/one/panes", nil))
	if panes.Code != http.StatusBadRequest || !strings.Contains(panes.Body.String(), "Unix user is required") {
		t.Fatalf("bare panes response = %d %s", panes.Code, panes.Body.String())
	}
	send := h.send(t, "one", map[string]string{"text": "must not route", "unixUser": ""})
	if send.Code != http.StatusBadRequest || !strings.Contains(send.Body.String(), "Unix user is required") {
		t.Fatalf("bare send response = %d %s", send.Code, send.Body.String())
	}
	assertNoSendSideEffects(t, h)
}

func TestSendToSessionRejectsConflictingQueryAndBodyUnixUsers(t *testing.T) {
	h := newSendHarness(t, "$7	one	%41	111	9001	@3	work	/home/alice	bash	1\n")
	t.Setenv("CHROTE_TERMINAL_USERS", "alice,bob")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a,bob=/tmp/tmux-b")

	recorder := h.sendWithQuery(t, "one", "unixUser=alice", map[string]string{"text": "must not route", "unixUser": "bob"})
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "conflicting Unix users") {
		t.Fatalf("conflicting send response = %d %s", recorder.Code, recorder.Body.String())
	}
	assertNoSendSideEffects(t, h)
}

func TestSendToSessionRejectsTmuxPrefixMatchBeforePersisting(t *testing.T) {
	h := newSendHarness(t, "$7\talpha-long\t%41\t111\t9001\n")
	recorder := h.send(t, "alpha", map[string]string{"text": "do not misroute"})
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
	if got := dropDirectoryCount(t, h.dropsDir); got != 0 {
		t.Fatalf("drop directories = %d, want 0", got)
	}
	if strings.Contains(readOptionalFile(t, h.tmuxLog), "paste-buffer") {
		t.Fatalf("prefix-matched request reached paste-buffer")
	}
}

func TestSendToSessionRequiresPaneForMultiPaneSession(t *testing.T) {
	h := newSendHarness(t, "$7	multi	%41	111	9001\n$7	multi	%42	222	9001\n")
	recorder := h.send(t, "multi", map[string]string{"text": "ambiguous"})
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if got := dropDirectoryCount(t, h.dropsDir); got != 0 {
		t.Fatalf("drop directories = %d, want 0", got)
	}
}

func TestSendToSessionRejectsStalePaneBeforeCreatingDrop(t *testing.T) {
	h := newSendHarness(t, "$7	one	%41	111	9001\n")
	recorder := h.send(t, "one", map[string]string{"text": "stale pane", "pane": "%99"})
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "PANE_NOT_IN_SESSION") {
		t.Fatalf("status/body = %d %s, want PANE_NOT_IN_SESSION conflict", recorder.Code, recorder.Body.String())
	}
	if got := dropDirectoryCount(t, h.dropsDir); got != 0 {
		t.Fatalf("drop directories = %d, want 0", got)
	}
}

func TestListSessionPanesReturnsExactPaneIdentities(t *testing.T) {
	h := newSendHarness(t, "$7	multi	%41	111	9001	@3	work	/home/alice/app	bash	1\n$7	multi	%42	222	9001	@4	logs	/home/alice/app	tail	0\n$8	multi-long	%43	333	9001	@5	other	/tmp	bash	1\n")
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions/multi/panes?unixUser=alice", nil)
	recorder := httptest.NewRecorder()
	h.mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response struct {
		Session string           `json:"session"`
		Panes   []sendPaneTarget `json:"panes"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode panes response: %v", err)
	}
	if response.Session != "multi" || len(response.Panes) != 2 {
		t.Fatalf("pane response = %#v", response)
	}
	if response.Panes[0].PaneID != "%41" || response.Panes[0].WindowID != "@3" || response.Panes[0].CurrentPath != "/home/alice/app" || response.Panes[0].CurrentCommand != "bash" || !response.Panes[0].Active {
		t.Fatalf("first pane identity = %#v", response.Panes[0])
	}
}

func TestSendToSessionRejectsChooserGenerationReuseBeforeCreatingDrop(t *testing.T) {
	h := newSendHarness(t, "$7\tone\t%41\t111\t9001\t@3\twork\t/home/alice\tbash\t1\n")
	t.Setenv("TMUX_SEND_PANES_NEXT", "$9\tone\t%41\t777\t9900\t@8\treused\t/tmp\tsh\t1\n")
	req := httptest.NewRequest(http.MethodGet, "/api/tmux/sessions/one/panes?unixUser=alice", nil)
	recorder := httptest.NewRecorder()
	h.mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("pane discovery status = %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = h.send(t, "one", map[string]string{
		"text":      "must not reach reused pane",
		"pane":      "%41",
		"sessionId": "$7",
		"panePid":   "111",
		"serverPid": "9001",
	})
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "TARGET_CHANGED") {
		t.Fatalf("status/body = %d %s, want TARGET_CHANGED conflict", recorder.Code, recorder.Body.String())
	}
	if got := dropDirectoryCount(t, h.dropsDir); got != 0 {
		t.Fatalf("drop directories = %d, want 0", got)
	}
}

func TestSendToSessionRejectsEmptyPayloadAsBadRequest(t *testing.T) {
	h := newSendHarness(t, "$7	one	%41	111	9001\n")
	recorder := h.send(t, "one", map[string]string{})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if got := dropDirectoryCount(t, h.dropsDir); got != 0 {
		t.Fatalf("drop directories = %d, want 0", got)
	}
}

func TestSendToSessionPinsExactPaneAndDefaultsToNoSubmit(t *testing.T) {
	h := newSendHarness(t, "$7\tmulti\t%41\t111\t9001\n$7\tmulti\t%42\t222\t9001\n")
	recorder := h.send(t, "multi", map[string]string{
		"text":      "safe payload",
		"pane":      "%42",
		"sessionId": "$7",
		"panePid":   "222",
		"serverPid": "9001",
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["pane"] != "%42" || response["submitKeyDispatched"] != false || response["transport"] != "pasted" {
		t.Fatalf("response = %#v, want pane %%42, no submit key, transport pasted", response)
	}
	log := readOptionalFile(t, h.tmuxLog)
	if !strings.Contains(log, "if-shell\n-F\n-t\n%42\n") ||
		!strings.Contains(log, "#{==:#{session_id},$7}") ||
		!strings.Contains(log, "#{==:#{pane_pid},222}") ||
		!strings.Contains(log, "#{==:#{pid},9001}") ||
		!strings.Contains(log, "paste-buffer -d -b chrote-send-") ||
		!strings.Contains(log, "-t %42 ; display-message -p CHROTE_SEND_PASTED") {
		t.Fatalf("tmux log does not atomically guard and paste to %%42: %s", log)
	}
	if strings.Contains(log, "send-keys") {
		t.Fatalf("omitted submit unexpectedly queued Enter: %s", log)
	}
	if strings.Contains(log, "delete-buffer") {
		t.Fatalf("successful paste-buffer -d unexpectedly needed fallback deletion: %s", log)
	}
	if response["bufferCleaned"] != true {
		t.Fatalf("atomic paste did not report buffer cleanup: %#v", response)
	}
	if got := dropDirectoryCount(t, h.dropsDir); got != 1 {
		t.Fatalf("drop directories = %d, want 1", got)
	}
	entries, err := os.ReadDir(h.dropsDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("read persisted drop: entries=%v err=%v", entries, err)
	}
	payload := filepath.Join(h.dropsDir, entries[0].Name(), "payload.txt")
	info, err := os.Stat(payload)
	if err != nil {
		t.Fatalf("stat payload: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("payload owner-only base mode = %o, want 600 with fake ACL", info.Mode().Perm())
	}
	if !strings.Contains(readOptionalFile(t, h.aclLog), "u:alice:r--") {
		t.Fatalf("target-user ACL was not applied: %s", readOptionalFile(t, h.aclLog))
	}
}

func TestSendToSessionAtomicGuardRejectsTargetChangeAndRemovesDrop(t *testing.T) {
	h := newSendHarness(t, "$7	one	%41	111	9001\n")
	t.Setenv("TMUX_SEND_ATOMIC_CHANGED", "1")
	recorder := h.send(t, "one", map[string]string{"text": "race"})
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if got := dropDirectoryCount(t, h.dropsDir); got != 0 {
		t.Fatalf("drop directories = %d after stale target, want 0", got)
	}
	log := readOptionalFile(t, h.tmuxLog)
	if !strings.Contains(log, "if-shell") || !strings.Contains(log, "delete-buffer") {
		t.Fatalf("atomic target rejection did not clean the retained buffer: %s", log)
	}
}

func TestSendToSessionAtomicClientFailureRetainsDropAsNonRetryableUnknown(t *testing.T) {
	h := newSendHarness(t, "$7	one	%41	111	9001\n")
	t.Setenv("TMUX_SEND_FAIL_PASTE", "1")
	recorder := h.send(t, "one", map[string]string{"text": "outcome unknown"})
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusAccepted, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["success"] != false || response["transport"] != "unknown" || response["retryable"] != false || response["bufferCleaned"] != true {
		t.Fatalf("unknown delivery response = %#v", response)
	}
	if got := dropDirectoryCount(t, h.dropsDir); got != 1 {
		t.Fatalf("drop directories = %d after ambiguous send, want retained drop", got)
	}
	if !strings.Contains(readOptionalFile(t, h.tmuxLog), "delete-buffer") {
		t.Fatalf("ambiguous send did not attempt tmux buffer cleanup: %s", readOptionalFile(t, h.tmuxLog))
	}
}

func TestSendToSessionSettlesThenDispatchesOneGuardedSubmitKey(t *testing.T) {
	h := newSendHarness(t, "$7	one	%41	111	9001\n")
	var settleDelay time.Duration
	settled := filepath.Join(t.TempDir(), "paste-settled")
	t.Setenv("TMUX_SEND_REQUIRE_SETTLE", settled)
	tmuxSendSleep = func(_ context.Context, delay time.Duration) error {
		settleDelay = delay
		if err := os.WriteFile(settled, nil, 0o600); err != nil {
			t.Fatalf("record paste settle: %v", err)
		}
		return nil
	}
	recorder := h.send(t, "one", map[string]string{"text": "submit after settling", "submit": "true"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["transport"] != "pasted" || response["submissionRequested"] != true || response["submitKeyDispatched"] != true || response["bufferCleaned"] != true {
		t.Fatalf("guarded submit response = %#v", response)
	}
	if _, legacy := response["submitted"]; legacy {
		t.Fatalf("response still claims application submission: %#v", response)
	}
	if settleDelay != tmuxSendSubmitSettleDelay {
		t.Fatalf("settle delay = %s, want %s", settleDelay, tmuxSendSubmitSettleDelay)
	}
	log := readOptionalFile(t, h.tmuxLog)
	if strings.Count(log, "if-shell\n-F\n-t\n%41\n") != 2 ||
		!strings.Contains(log, "paste-buffer -d") ||
		strings.Count(log, "send-keys -t %41 C-m") != 1 ||
		!strings.Contains(log, "CHROTE_SEND_SUBMIT_KEY_DISPATCHED") {
		t.Fatalf("paste and submit key were not separately generation-guarded: %s", log)
	}
	if strings.Contains(log, "send-keys -t %41 Enter") {
		t.Fatalf("legacy immediate Enter remained in submit path: %s", log)
	}
}

func TestSendToSessionDoesNotDispatchSubmitKeyAfterGenerationChanges(t *testing.T) {
	h := newSendHarness(t, "$7	one	%41	111	9001\n")
	t.Setenv("TMUX_SEND_SUBMIT_TARGET_CHANGED", "1")
	recorder := h.send(t, "one", map[string]string{"text": "paste only after race", "submit": "true"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	warning, _ := response["warning"].(string)
	if response["transport"] != "pasted" || response["submitKeyDispatched"] != false || !strings.Contains(warning, "submit key was not dispatched") {
		t.Fatalf("generation-race response = %#v", response)
	}
	if strings.Count(readOptionalFile(t, h.tmuxLog), "send-keys -t %41 C-m") != 1 {
		t.Fatalf("guarded submit command was retried: %s", readOptionalFile(t, h.tmuxLog))
	}
}

func TestSendToSessionACLFailureRemovesPartialDrop(t *testing.T) {
	h := newSendHarness(t, "$7	one	%41	111	9001\n")
	t.Setenv("TMUX_SEND_FAIL_ACL", "1")
	recorder := h.send(t, "one", map[string]string{"text": "private"})
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
	if got := dropDirectoryCount(t, h.dropsDir); got != 0 {
		t.Fatalf("drop directories = %d after ACL failure, want 0", got)
	}
}

func TestMaintainSessionDropsBadEntryDoesNotStrandLaterRetainedUsers(t *testing.T) {
	h := newSendHarness(t, "")
	for name, unixUser := range map[string]string{"a-alice": "alice", "z-bob": "bob"} {
		drop := filepath.Join(h.dropsDir, name)
		if err := os.MkdirAll(drop, 0o755); err != nil {
			t.Fatalf("create %s drop: %v", unixUser, err)
		}
		manifest, err := json.Marshal(sessionDropManifest{UnixUser: unixUser})
		if err != nil {
			t.Fatalf("marshal %s manifest: %v", unixUser, err)
		}
		if err := os.WriteFile(filepath.Join(drop, "manifest.json"), manifest, 0o644); err != nil {
			t.Fatalf("write %s manifest: %v", unixUser, err)
		}
	}
	if err := syscall.Mkfifo(filepath.Join(h.dropsDir, "m-bad"), 0o600); err != nil {
		t.Fatalf("create bad middle entry: %v", err)
	}

	err := maintainSessionDrops(h.dropsDir, time.Now())
	if err == nil || !strings.Contains(err.Error(), "unsupported entry") {
		t.Fatalf("maintenance error = %v, want unsupported entry report", err)
	}
	aclLog := readOptionalFile(t, h.aclLog)
	for _, expected := range []string{"u:alice:--x", "u:bob:--x", "u:bob:r-x", "u:bob:r--"} {
		if !strings.Contains(aclLog, expected) {
			t.Fatalf("ACL log missing %q after bad middle entry:\n%s", expected, aclLog)
		}
	}
	bobManifest := filepath.Join(h.dropsDir, "z-bob", "manifest.json")
	info, err := os.Stat(bobManifest)
	if err != nil {
		t.Fatalf("stat Bob manifest: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("Bob manifest owner-only base mode = %o, want 600 with fake ACL", info.Mode().Perm())
	}
}

func TestMaintainSessionDropsDeletedAccountDoesNotBlockOtherUsersOrExpiration(t *testing.T) {
	h := newSendHarness(t, "")
	t.Setenv("CHROTE_SESSION_DROPS_RETENTION", "1h")
	now := time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC)
	fixtures := map[string]string{
		"a-alice":   "alice",
		"m-deleted": "chrote_deleted_user_zz",
		"z-bob":     "bob",
	}
	for name, unixUser := range fixtures {
		drop := filepath.Join(h.dropsDir, name)
		if err := os.MkdirAll(drop, 0o755); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		manifest, err := json.Marshal(sessionDropManifest{UnixUser: unixUser})
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(drop, "manifest.json"), manifest, 0o644); err != nil {
			t.Fatalf("write %s manifest: %v", name, err)
		}
	}
	expired := filepath.Join(h.dropsDir, "20260701T000000Z-121212121212121212121212")
	if err := os.MkdirAll(expired, 0o755); err != nil {
		t.Fatalf("create expired drop: %v", err)
	}
	old := now.Add(-24 * time.Hour)
	if err := os.Chtimes(expired, old, old); err != nil {
		t.Fatalf("age expired drop: %v", err)
	}

	err := maintainSessionDrops(h.dropsDir, now)
	if err == nil || !strings.Contains(err.Error(), "resolve session drop Unix user \"chrote_deleted_user_zz\"") {
		t.Fatalf("maintenance error = %v, want deleted-account report", err)
	}
	if _, err := os.Stat(expired); !os.IsNotExist(err) {
		t.Fatalf("expired drop survived deleted-account manifest: %v", err)
	}
	aclLog := readOptionalFile(t, h.aclLog)
	for _, expected := range []string{"u:alice:--x", "u:bob:--x", "g::---", "o::---"} {
		if !strings.Contains(aclLog, expected) {
			t.Fatalf("ACL log missing %q:\n%s", expected, aclLog)
		}
	}
	if strings.Contains(aclLog, "u:chrote_deleted_user_zz:") {
		t.Fatalf("deleted account was included in ACL grants:\n%s", aclLog)
	}
	for _, name := range []string{"m-deleted", "z-bob"} {
		info, statErr := os.Stat(filepath.Join(h.dropsDir, name, "manifest.json"))
		if statErr != nil {
			t.Fatalf("stat %s hardened manifest: %v", name, statErr)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s manifest owner-only base mode = %o, want 600 with fake ACL", name, info.Mode().Perm())
		}
	}
}

func TestMaintainSessionDropsExpiresOldDropsAndHardensRetainedDrops(t *testing.T) {
	h := newSendHarness(t, "")
	t.Setenv("CHROTE_SESSION_DROPS_RETENTION", "24h")
	now := time.Date(2026, 7, 18, 15, 0, 0, 0, time.UTC)
	oldPath := filepath.Join(h.dropsDir, "20260701T000000Z-aaaaaaaaaaaaaaaaaaaaaaaa")
	keptPath := filepath.Join(h.dropsDir, "20260718T000000Z-bbbbbbbbbbbbbbbbbbbbbbbb")
	legacyPath := filepath.Join(h.dropsDir, "legacy-drop")
	for _, path := range []string{oldPath, keptPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create fixture drop: %v", err)
		}
		manifest, err := json.Marshal(sessionDropManifest{UnixUser: "alice"})
		if err != nil {
			t.Fatalf("marshal fixture manifest: %v", err)
		}
		if err := os.WriteFile(filepath.Join(path, "manifest.json"), manifest, 0o644); err != nil {
			t.Fatalf("write fixture manifest: %v", err)
		}
		if err := os.WriteFile(filepath.Join(path, "payload.txt"), []byte("payload"), 0o644); err != nil {
			t.Fatalf("write fixture payload: %v", err)
		}
	}
	oldTime := now.Add(-48 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("age old fixture: %v", err)
	}
	if err := os.Chtimes(keptPath, now, now); err != nil {
		t.Fatalf("date kept fixture: %v", err)
	}
	if err := os.MkdirAll(legacyPath, 0o755); err != nil {
		t.Fatalf("create legacy fixture: %v", err)
	}
	legacyPayload := filepath.Join(legacyPath, "payload.txt")
	if err := os.WriteFile(legacyPayload, []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write legacy payload: %v", err)
	}
	loosePayload := filepath.Join(h.dropsDir, "legacy-payload.txt")
	if err := os.WriteFile(loosePayload, []byte("loose"), 0o644); err != nil {
		t.Fatalf("write loose payload: %v", err)
	}

	if err := maintainSessionDrops(h.dropsDir, now); err != nil {
		t.Fatalf("maintain session drops: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expired drop still exists: %v", err)
	}
	for path, want := range map[string]os.FileMode{
		h.dropsDir:                               0o700,
		keptPath:                                 0o700,
		filepath.Join(keptPath, "manifest.json"): 0o600,
		filepath.Join(keptPath, "payload.txt"):   0o600,
		legacyPath:                               0o700,
		legacyPayload:                            0o600,
		loosePayload:                             0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if info.Mode().Perm() != want {
			t.Fatalf("mode %s = %o, want %o", path, info.Mode().Perm(), want)
		}
	}
	aclLog := readOptionalFile(t, h.aclLog)
	if !strings.Contains(aclLog, "u:alice:r--") || !strings.Contains(aclLog, "u:alice:r-x") {
		t.Fatalf("retained drop ACLs missing: %s", aclLog)
	}
}

func TestSessionDropJanitorExpiresDropsAtStartupAndWithoutAnotherSend(t *testing.T) {
	h := newSendHarness(t, "")
	t.Setenv("CHROTE_SESSION_DROPS_RETENTION", "10ms")
	t.Setenv("CHROTE_SESSION_DROPS_MAINTENANCE_INTERVAL", "100ms")

	createExpiredDrop := func(id string) string {
		t.Helper()
		path := filepath.Join(h.dropsDir, id)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create expired drop: %v", err)
		}
		manifest, err := json.Marshal(sessionDropManifest{UnixUser: "alice"})
		if err != nil {
			t.Fatalf("marshal expired manifest: %v", err)
		}
		if err := os.WriteFile(filepath.Join(path, "manifest.json"), manifest, 0o644); err != nil {
			t.Fatalf("write expired manifest: %v", err)
		}
		old := time.Now().Add(-time.Hour)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatalf("age expired drop: %v", err)
		}
		return path
	}

	startupDrop := createExpiredDrop("20260701T000000Z-cccccccccccccccccccccccc")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errorsSeen := make(chan error, 1)
	handler := NewTmuxHandler()
	done, err := handler.StartSessionDropJanitor(ctx, func(err error) {
		select {
		case errorsSeen <- err:
		default:
		}
	})
	if err != nil {
		t.Fatalf("start drop janitor: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("janitor did not stop after cancellation")
		}
	})
	if _, err := os.Stat(startupDrop); !os.IsNotExist(err) {
		t.Fatalf("startup maintenance left expired drop: %v", err)
	}

	periodicDrop := createExpiredDrop("20260702T000000Z-dddddddddddddddddddddddd")
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	for {
		if _, err := os.Stat(periodicDrop); os.IsNotExist(err) {
			break
		}
		select {
		case err := <-errorsSeen:
			t.Fatalf("janitor reported error: %v", err)
		case <-deadline.C:
			t.Fatalf("periodic janitor left expired drop %s", periodicDrop)
		case <-poll.C:
		}
	}
}

func TestSessionDropJanitorCancellationInterruptsLockContention(t *testing.T) {
	h := newSendHarness(t, "")
	if err := h.handler.lockSessionDrops(context.Background()); err != nil {
		t.Fatalf("hold session drop lock: %v", err)
	}
	defer h.handler.unlockSessionDrops()
	ctx, cancel := context.WithCancel(context.Background())
	type startResult struct {
		done <-chan struct{}
		err  error
	}
	result := make(chan startResult, 1)
	go func() {
		done, err := h.handler.StartSessionDropJanitor(ctx, nil)
		result <- startResult{done: done, err: err}
	}()
	cancel()
	select {
	case started := <-result:
		if !errors.Is(started.err, context.Canceled) {
			t.Fatalf("janitor start error = %v, want context cancellation", started.err)
		}
		select {
		case <-started.done:
		default:
			t.Fatal("cancelled janitor completion channel remained open")
		}
	case <-time.After(time.Second):
		t.Fatal("janitor remained queued on the session drop lock after cancellation")
	}
	if log := readOptionalFile(t, h.aclLog); log != "" {
		t.Fatalf("cancelled queued janitor changed ACLs:\n%s", log)
	}
}

func TestSessionDropJanitorCancellationKillsInProgressRootACLCommand(t *testing.T) {
	h := newSendHarness(t, "")
	startedPath := filepath.Join(t.TempDir(), "acl-started")
	t.Setenv("TMUX_SEND_BLOCK_ACL", "1")
	t.Setenv("TMUX_SEND_ACL_STARTED", startedPath)
	ctx, cancel := context.WithCancel(context.Background())
	type startResult struct {
		done <-chan struct{}
		err  error
	}
	result := make(chan startResult, 1)
	go func() {
		done, err := h.handler.StartSessionDropJanitor(ctx, nil)
		result <- startResult{done: done, err: err}
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(startedPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("blocking root setfacl command did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case started := <-result:
		if !errors.Is(started.err, context.Canceled) {
			t.Fatalf("janitor start error = %v, want context cancellation", started.err)
		}
		select {
		case <-started.done:
		default:
			t.Fatal("cancelled root-ACL janitor completion channel remained open")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("janitor did not kill the blocking root setfacl command")
	}
}

func TestSessionDropJanitorCancellationKillsInProgressPerTreeACLCommand(t *testing.T) {
	h := newSendHarness(t, "")
	startedPath := filepath.Join(t.TempDir(), "acl-started")
	retainedDrop := filepath.Join(h.dropsDir, "retained-drop")
	if err := os.MkdirAll(retainedDrop, 0o755); err != nil {
		t.Fatalf("create retained drop: %v", err)
	}
	if err := os.WriteFile(filepath.Join(retainedDrop, "manifest.json"), []byte(`{"unixUser":"alice"}`), 0o644); err != nil {
		t.Fatalf("write retained manifest: %v", err)
	}
	t.Setenv("TMUX_SEND_BLOCK_ACL", "1")
	t.Setenv("TMUX_SEND_BLOCK_ACL_MATCH", retainedDrop)
	t.Setenv("TMUX_SEND_ACL_STARTED", startedPath)
	ctx, cancel := context.WithCancel(context.Background())
	type startResult struct {
		done <-chan struct{}
		err  error
	}
	result := make(chan startResult, 1)
	go func() {
		done, err := h.handler.StartSessionDropJanitor(ctx, nil)
		result <- startResult{done: done, err: err}
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(startedPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("blocking setfacl command did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case started := <-result:
		if !errors.Is(started.err, context.Canceled) {
			t.Fatalf("janitor start error = %v, want context cancellation", started.err)
		}
		select {
		case <-started.done:
		default:
			t.Fatal("cancelled in-progress janitor completion channel remained open")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("janitor did not kill the blocking setfacl command")
	}
	before := readOptionalFile(t, h.aclLog)
	if !strings.Contains(before, "u:alice:--x") {
		t.Fatalf("root traversal ACL was not coherently rebuilt before cancellation:\n%s", before)
	}
	time.Sleep(50 * time.Millisecond)
	if after := readOptionalFile(t, h.aclLog); after != before {
		t.Fatalf("ACL log changed after janitor completion:\nbefore=%q\nafter=%q", before, after)
	}
}

func TestSessionDropJanitorInvalidIntervalFallsBackWithoutDisablingMaintenance(t *testing.T) {
	h := newSendHarness(t, "")
	t.Setenv("CHROTE_SESSION_DROPS_RETENTION", "1h")
	t.Setenv("CHROTE_SESSION_DROPS_MAINTENANCE_INTERVAL", "not-a-duration")
	drop := filepath.Join(h.dropsDir, "20260701T000000Z-999999999999999999999999")
	if err := os.MkdirAll(drop, 0o755); err != nil {
		t.Fatalf("create expired drop: %v", err)
	}
	old := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(drop, old, old); err != nil {
		t.Fatalf("age expired drop: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reported := make(chan error, 1)
	done, err := h.handler.StartSessionDropJanitor(ctx, func(err error) { reported <- err })
	if err != nil {
		t.Fatalf("janitor safe fallback returned error: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("janitor did not stop after cancellation")
		}
	})
	if _, err := os.Stat(drop); !os.IsNotExist(err) {
		t.Fatalf("startup maintenance left expired drop: %v", err)
	}
	select {
	case err := <-reported:
		if !strings.Contains(err.Error(), "using 1h0m0s") {
			t.Fatalf("fallback report = %v", err)
		}
	default:
		t.Fatal("invalid interval was not reported")
	}
}

func TestMaintainSessionDropsInvalidRetentionStillHardensRetainedDrops(t *testing.T) {
	h := newSendHarness(t, "")
	t.Setenv("CHROTE_SESSION_DROPS_RETENTION", "invalid")
	drop := filepath.Join(h.dropsDir, "20260718T120000Z-999999999999999999999999")
	if err := os.MkdirAll(drop, 0o755); err != nil {
		t.Fatalf("create retained drop: %v", err)
	}
	manifest, err := json.Marshal(sessionDropManifest{UnixUser: "alice"})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	manifestPath := filepath.Join(drop, "manifest.json")
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := maintainSessionDrops(h.dropsDir, time.Now()); err == nil || !strings.Contains(err.Error(), "invalid CHROTE_SESSION_DROPS_RETENTION") {
		t.Fatalf("maintenance error = %v, want invalid retention report", err)
	}
	for path, want := range map[string]os.FileMode{drop: 0o700, manifestPath: 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat hardened path %s: %v", path, err)
		}
		if info.Mode().Perm() != want {
			t.Fatalf("mode %s = %o, want %o", path, info.Mode().Perm(), want)
		}
	}
	aclLog, err := os.ReadFile(h.aclLog)
	if err != nil {
		t.Fatalf("read ACL log: %v", err)
	}
	for _, expected := range []string{"u:alice:--x", "u:alice:r-x", "u:alice:r--"} {
		if !strings.Contains(string(aclLog), expected) {
			t.Fatalf("ACL log missing %q:\n%s", expected, aclLog)
		}
	}
}

func TestMaintainSessionDropsRejectsSymlinkRoot(t *testing.T) {
	base := t.TempDir()
	realRoot := filepath.Join(base, "real")
	if err := os.Mkdir(realRoot, 0o755); err != nil {
		t.Fatalf("create real root: %v", err)
	}
	linkRoot := filepath.Join(base, "drops")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Fatalf("create root symlink: %v", err)
	}
	if err := maintainSessionDrops(linkRoot, time.Now()); err == nil {
		t.Fatal("maintainSessionDrops accepted a symlink root")
	}
	info, err := os.Stat(realRoot)
	if err != nil {
		t.Fatalf("stat real root: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("symlink target mode changed to %o", info.Mode().Perm())
	}
}

func TestMaintainSessionDropsRejectsNestedSpecialFiles(t *testing.T) {
	h := newSendHarness(t, "")
	dropPath := filepath.Join(h.dropsDir, "20260718T120000Z-eeeeeeeeeeeeeeeeeeeeeeee")
	if err := os.MkdirAll(dropPath, 0o755); err != nil {
		t.Fatalf("create drop: %v", err)
	}
	fifoPath := filepath.Join(dropPath, "payload.fifo")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Fatalf("create FIFO: %v", err)
	}
	if err := maintainSessionDrops(h.dropsDir, time.Now()); err == nil {
		t.Fatal("maintainSessionDrops accepted a nested FIFO")
	}
}

func TestMaintainSessionDropsRejectsManifestSymlinkWithoutFollowing(t *testing.T) {
	h := newSendHarness(t, "")
	dropPath := filepath.Join(h.dropsDir, "20260718T120000Z-bbbbbbbbbbbbbbbbbbbbbbbb")
	if err := os.MkdirAll(dropPath, 0o755); err != nil {
		t.Fatalf("create drop: %v", err)
	}
	external := filepath.Join(t.TempDir(), "external.json")
	if err := os.WriteFile(external, []byte(`{"unixUser":"alice"}`), 0o666); err != nil {
		t.Fatalf("write external manifest: %v", err)
	}
	externalInfo, err := os.Stat(external)
	if err != nil {
		t.Fatalf("stat external manifest before maintenance: %v", err)
	}
	externalMode := externalInfo.Mode().Perm()
	if err := os.Symlink(external, filepath.Join(dropPath, "manifest.json")); err != nil {
		t.Fatalf("create manifest symlink: %v", err)
	}
	if err := maintainSessionDrops(h.dropsDir, time.Now()); err == nil || !strings.Contains(err.Error(), "without following links") {
		t.Fatalf("manifest symlink error = %v", err)
	}
	info, err := os.Stat(external)
	if err != nil {
		t.Fatalf("stat external manifest: %v", err)
	}
	if info.Mode().Perm() != externalMode {
		t.Fatalf("external manifest mode changed from %o to %o", externalMode, info.Mode().Perm())
	}
}

func TestMaintainSessionDropsDoesNotExpireImpossibleTimestamp(t *testing.T) {
	h := newSendHarness(t, "")
	t.Setenv("CHROTE_SESSION_DROPS_RETENTION", "1h")
	invalidPath := filepath.Join(h.dropsDir, "20269999T999999Z-ffffffffffffffffffffffff")
	if err := os.MkdirAll(invalidPath, 0o755); err != nil {
		t.Fatalf("create invalid timestamp drop: %v", err)
	}
	old := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(invalidPath, old, old); err != nil {
		t.Fatalf("age invalid timestamp drop: %v", err)
	}
	if err := maintainSessionDrops(h.dropsDir, time.Now()); err != nil {
		t.Fatalf("maintain drops: %v", err)
	}
	if _, err := os.Stat(invalidPath); err != nil {
		t.Fatalf("invalid timestamp entry should be retained and hardened: %v", err)
	}
}

func TestSecureSessionDropTreeRebuildsACLsFromKnownBase(t *testing.T) {
	if _, err := exec.LookPath("setfacl"); err != nil {
		t.Skip("setfacl is not installed")
	}
	if _, err := exec.LookPath("getfacl"); err != nil {
		t.Skip("getfacl is not installed")
	}
	root := filepath.Join(t.TempDir(), "drops")
	drop := filepath.Join(root, "20260718T120000Z-aaaaaaaaaaaaaaaaaaaaaaaa")
	filesDir := filepath.Join(drop, "files")
	file := filepath.Join(drop, "payload.txt")
	manifestPath := filepath.Join(drop, "manifest.json")
	upload := filepath.Join(filesDir, "upload.txt")
	if err := os.MkdirAll(filesDir, 0o750); err != nil {
		t.Fatalf("create drop: %v", err)
	}
	if err := os.WriteFile(file, []byte("payload"), 0o640); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if err := os.WriteFile(upload, []byte("upload"), 0o640); err != nil {
		t.Fatalf("write upload: %v", err)
	}
	manifest, err := json.Marshal(sessionDropManifest{UnixUser: "nobody"})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, manifest, 0o640); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	for _, command := range [][]string{
		{"setfacl", "-m", "u:daemon:--x,d:u:daemon:r-x", root},
		{"setfacl", "-m", "u:daemon:rwx,d:u:daemon:rwx", drop},
		{"setfacl", "-m", "u:daemon:rwx,d:u:daemon:rwx", filesDir},
		{"setfacl", "-m", "u:daemon:rw-", file},
		{"setfacl", "-m", "u:daemon:rw-", manifestPath},
		{"setfacl", "-m", "u:daemon:rw-", upload},
	} {
		if output, err := exec.Command(command[0], command[1:]...).CombinedOutput(); err != nil {
			t.Skipf("cannot seed ACL fixture: %v: %s", err, output)
		}
	}
	if err := maintainSessionDrops(root, time.Date(2026, 7, 18, 13, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("maintain drop tree: %v", err)
	}
	expectedPermissions := map[string]string{
		root:         "--x",
		drop:         "r-x",
		filesDir:     "r-x",
		manifestPath: "r--",
		file:         "r--",
		upload:       "r--",
	}
	for path, expected := range expectedPermissions {
		output, err := exec.Command("getfacl", "-cp", path).CombinedOutput()
		if err != nil {
			t.Fatalf("getfacl %s: %v: %s", path, err, output)
		}
		acl := string(output)
		if strings.Contains(acl, "user:daemon:") || strings.Contains(acl, "default:user:daemon:") {
			t.Fatalf("stale daemon ACL survived on %s:\n%s", path, acl)
		}
		if !strings.Contains(acl, "user:nobody:"+expected) {
			t.Fatalf("target nobody ACL %s missing on %s:\n%s", expected, path, acl)
		}
		if !strings.Contains(acl, "group::---") || !strings.Contains(acl, "other::---") {
			t.Fatalf("owning group or other retained access on %s:\n%s", path, acl)
		}
	}
}
