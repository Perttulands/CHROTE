package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	osuser "os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type sendHarness struct {
	dropsDir  string
	tmuxLog   string
	submitLog string
	aclLog    string
	mux       *http.ServeMux
	handler   *TmuxHandler
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
	submitLog := filepath.Join(dir, "submit.log")
	aclLog := filepath.Join(dir, "setfacl.log")
	paneCount := filepath.Join(dir, "pane-count")
	captureCount := filepath.Join(dir, "capture-count")
	submitCount := filepath.Join(dir, "submit-count")
	payloadPath := filepath.Join(dir, "payload-path")
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
    switch_after="${TMUX_SEND_PANES_SWITCH_AFTER:-1}"
    if [ "$count" -le "$switch_after" ] || [ -z "${TMUX_SEND_PANES_NEXT:-}" ]; then
      printf '%s' "$TMUX_SEND_PANES"
    else
      printf '%s' "$TMUX_SEND_PANES_NEXT"
    fi
    ;;
  load-buffer)
    [ "${TMUX_SEND_FAIL_LOAD:-}" = 1 ] && exit 7
    for arg in "$@"; do payload_path="$arg"; done
    printf '%s' "$payload_path" > "$TMUX_SEND_PAYLOAD_PATH"
    ;;
  capture-pane)
    count=0
    if [ -f "$TMUX_SEND_CAPTURE_COUNT" ]; then count=$(cat "$TMUX_SEND_CAPTURE_COUNT"); fi
    count=$((count + 1)); printf '%s' "$count" > "$TMUX_SEND_CAPTURE_COUNT"
    if [ "${TMUX_SEND_CAPTURE_FAIL_AT:-}" = "$count" ]; then exit 12; fi
    adjusted_count="$count"
    submit_count=0
    if [ -f "$TMUX_SEND_SUBMIT_COUNT" ]; then submit_count=$(cat "$TMUX_SEND_SUBMIT_COUNT"); fi
    if { [ -n "${TMUX_SEND_CAPTURE_BEFORE:-}" ] || [ "${TMUX_SEND_CAPTURE_BEFORE_COLLAPSED_CODEX:-}" = 1 ]; } && [ "$submit_count" -eq 0 ]; then
      if [ "$count" -eq 1 ]; then
        if [ "${TMUX_SEND_CAPTURE_BEFORE_COLLAPSED_CODEX:-}" = 1 ]; then
          for before_payload in "$CHROTE_SESSION_DROPS_DIR"/*/payload.txt; do [ -f "$before_payload" ] && break; done
          before_size=$(wc -c < "$before_payload")
          printf '╭ OpenAI Codex (v0.147.0)\n› [Pasted Content %s chars]\n  gpt-5.6-sol xhigh' "$before_size"
        else
          printf '%s' "$TMUX_SEND_CAPTURE_BEFORE"
        fi
        exit 0
      fi
      adjusted_count=$((count - 1))
    fi
    if [ "${TMUX_SEND_CAPTURE_COLLAPSED_CODEX:-}" = 1 ]; then
      size=$(wc -c < "$(cat "$TMUX_SEND_PAYLOAD_PATH")")
      printf '╭ OpenAI Codex (v0.147.0)\n› [Pasted Content %s chars]\n  gpt-5.6-sol xhigh' "$size"
    elif [ "$adjusted_count" -eq 1 ]; then
      printf '%s' "${TMUX_SEND_CAPTURE_ONE:-}"
    else
      printf '%s' "${TMUX_SEND_CAPTURE_TWO:-${TMUX_SEND_CAPTURE_ONE:-}}"
    fi
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
        *" send-keys "*)
          count=0
          if [ -f "$TMUX_SEND_SUBMIT_COUNT" ]; then count=$(cat "$TMUX_SEND_SUBMIT_COUNT"); fi
          count=$((count + 1)); printf '%s' "$count" > "$TMUX_SEND_SUBMIT_COUNT"
          if [ "${TMUX_SEND_RETRY_TARGET_CHANGED:-}" = 1 ] && [ "$count" -gt 1 ]; then
            printf '%s\n' CHROTE_SEND_SUBMIT_TARGET_CHANGED
          else
            printf '%s\n' dispatched >> "$TMUX_SEND_SUBMIT_LOG"
            printf '%s\n' CHROTE_SEND_SUBMIT_KEY_DISPATCHED
          fi
          ;;
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
	t.Setenv("TMUX_SEND_SUBMIT_LOG", submitLog)
	t.Setenv("TMUX_SEND_ACL_LOG", aclLog)
	t.Setenv("TMUX_SEND_PANE_COUNT", paneCount)
	t.Setenv("TMUX_SEND_CAPTURE_COUNT", captureCount)
	t.Setenv("TMUX_SEND_SUBMIT_COUNT", submitCount)
	t.Setenv("TMUX_SEND_PAYLOAD_PATH", payloadPath)
	t.Setenv("TMUX_SEND_PANES", panes)
	t.Setenv("CHROTE_SESSION_DROPS_DIR", filepath.Join(dir, "drops"))
	t.Setenv("CHROTE_TERMINAL_USERS", "alice")
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", "alice=/tmp/tmux-a")
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", "alice=/home/alice")

	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return sendHarness{dropsDir: filepath.Join(dir, "drops"), tmuxLog: tmuxLog, submitLog: submitLog, aclLog: aclLog, mux: mux, handler: handler}
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
		!strings.Contains(log, "paste-buffer -p -d -b chrote-send-") ||
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
		!strings.Contains(log, "paste-buffer -p -d") ||
		strings.Count(log, "send-keys -t %41 Enter") != 1 ||
		!strings.Contains(log, "CHROTE_SEND_SUBMIT_KEY_DISPATCHED") {
		t.Fatalf("paste and submit key were not separately generation-guarded: %s", log)
	}
	if strings.Contains(log, "send-keys -t %41 C-m") {
		t.Fatalf("legacy C-m remained in submit path: %s", log)
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
	if strings.Count(readOptionalFile(t, h.tmuxLog), "send-keys -t %41 Enter") != 1 {
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
