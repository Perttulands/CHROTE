package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	originalSendSleep := tmuxSendSleep
	tmuxSendSleep = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() {
		tmuxSendSleep = originalSendSleep
	})
	dir := t.TempDir()
	tmuxLog := filepath.Join(dir, "tmux.log")
	aclLog := filepath.Join(dir, "setfacl.log")
	paneCount := filepath.Join(dir, "pane-count")
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
	for arg in "$@"; do payload_path="$arg"; done
    printf '%s' "$payload_path" > "$TMUX_SEND_PAYLOAD_PATH"
    ;;
  if-shell)
    if [ "${TMUX_SEND_ATOMIC_CHANGED:-}" = 1 ]; then
      printf '%s\n' CHROTE_SEND_TARGET_CHANGED
	elif [ "${TMUX_SEND_FAIL_PASTE:-}" = 1 ]; then
	  exit 8
	elif [ -n "${TMUX_SEND_REQUIRE_SETTLE:-}" ] && echo " $* " | grep -q " send-keys " && [ ! -f "$TMUX_SEND_REQUIRE_SETTLE" ]; then
      printf '%s\n' CHROTE_SEND_SUBMIT_NOT_SETTLED
    else
      case " $* " in
        *" send-keys "*)
          count=0
          if [ -f "$TMUX_SEND_SUBMIT_COUNT" ]; then count=$(cat "$TMUX_SEND_SUBMIT_COUNT"); fi
          count=$((count + 1)); printf '%s' "$count" > "$TMUX_SEND_SUBMIT_COUNT"
		  printf '%s\n' CHROTE_SEND_SUBMIT_KEY_DISPATCHED
          ;;
        *) printf '%s\n' CHROTE_SEND_PASTED ;;
      esac
    fi
    ;;
  delete-buffer)
	;;
esac
exit 0
`
	aclScript := `#!/bin/sh
for arg in "$@"; do printf '%s\n' "$arg" >> "$TMUX_SEND_ACL_LOG"; done
printf '%s\n' '---' >> "$TMUX_SEND_ACL_LOG"
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
	t.Setenv("TMUX_SEND_SUBMIT_COUNT", submitCount)
	t.Setenv("TMUX_SEND_PAYLOAD_PATH", payloadPath)
	t.Setenv("TMUX_SEND_PANES", panes)
	t.Setenv("CHROTE_SESSION_DROPS_DIR", filepath.Join(dir, "drops"))
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a")
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
	t.Setenv("CHROTE_TMUX_SOCKET", "alice=/tmp/tmux-a,bob=/tmp/tmux-b")

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

	// Two answers are worse than none: naming one user in the query and another
	// in the body must not be resolved by preferring either.
	conflicting := h.sendWithQuery(t, "one", "unixUser=alice", map[string]string{"text": "must not route", "unixUser": "bob"})
	if conflicting.Code != http.StatusBadRequest || !strings.Contains(conflicting.Body.String(), "conflicting Unix users") {
		t.Fatalf("conflicting send response = %d %s", conflicting.Code, conflicting.Body.String())
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

// A send CHROTE cannot address exactly is refused before anything is written.
// Each of these would otherwise land keystrokes in a pane the operator did not
// choose, so the empty drops directory is as much of the contract as the status.
func TestSendToSessionRefusesASendItCannotAddressExactly(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		panes      string
		session    string
		fields     map[string]string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "a session with more than one pane needs the pane named",
			panes:      "$7\tmulti\t%41\t111\t9001\n$7\tmulti\t%42\t222\t9001\n",
			session:    "multi",
			fields:     map[string]string{"text": "ambiguous"},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "a pane that has left the session is not sent to",
			panes:      "$7\tone\t%41\t111\t9001\n",
			session:    "one",
			fields:     map[string]string{"text": "stale pane", "pane": "%99"},
			wantStatus: http.StatusConflict,
			wantBody:   "PANE_NOT_IN_SESSION",
		},
		{
			name:       "a send with nothing in it is not a send",
			panes:      "$7\tone\t%41\t111\t9001\n",
			session:    "one",
			fields:     map[string]string{},
			wantStatus: http.StatusBadRequest,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			h := newSendHarness(t, testCase.panes)
			recorder := h.send(t, testCase.session, testCase.fields)
			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, testCase.wantStatus, recorder.Body.String())
			}
			if testCase.wantBody != "" && !strings.Contains(recorder.Body.String(), testCase.wantBody) {
				t.Fatalf("body = %s, want it to name %q", recorder.Body.String(), testCase.wantBody)
			}
			if got := dropDirectoryCount(t, h.dropsDir); got != 0 {
				t.Fatalf("drop directories = %d, want 0", got)
			}
		})
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

func TestSendToSessionDeliveryContract(t *testing.T) {
	var settleDelays []time.Duration
	tests := []struct {
		name       string
		panes      string
		session    string
		fields     map[string]string
		wantStatus int
		setup      func(*testing.T)
		assert     func(*testing.T, sendHarness, map[string]any)
	}{
		{
			name:    "targets the exact pane generation",
			panes:   "$7\tmulti\t%41\t111\t9001\n$7\tmulti\t%42\t222\t9001\n",
			session: "multi",
			fields: map[string]string{
				"text":      "safe payload",
				"pane":      "%42",
				"sessionId": "$7",
				"panePid":   "222",
				"serverPid": "9001",
			},
			wantStatus: http.StatusOK,
			assert: func(t *testing.T, h sendHarness, response map[string]any) {
				if response["pane"] != "%42" || response["submitKeyDispatched"] != false || response["transport"] != "pasted" || response["bufferCleaned"] != true {
					t.Fatalf("response = %#v, want exact pane %%42, pasted, cleaned, and no submit key", response)
				}
				log := readOptionalFile(t, h.tmuxLog)
				if !strings.Contains(log, "if-shell\n-F\n-t\n%42\n") ||
					!strings.Contains(log, "#{==:#{session_id},$7}") ||
					!strings.Contains(log, "#{==:#{pane_id},%42}") ||
					!strings.Contains(log, "#{==:#{pane_pid},222}") ||
					!strings.Contains(log, "#{==:#{pid},9001}") ||
					!strings.Contains(log, "paste-buffer -p -d -b chrote-send-") ||
					!strings.Contains(log, "-t %42 ; display-message -p CHROTE_SEND_PASTED") ||
					strings.Contains(log, "send-keys") {
					t.Fatalf("tmux log does not contain one exact guarded paste to %%42: %s", log)
				}
				entries, err := os.ReadDir(h.dropsDir)
				if err != nil || len(entries) != 1 {
					t.Fatalf("read persisted drop: entries=%v err=%v", entries, err)
				}
				dropPath := filepath.Join(h.dropsDir, entries[0].Name())
				rootInfo, err := os.Stat(h.dropsDir)
				if err != nil || rootInfo.Mode().Perm() != 0o711 {
					t.Fatalf("drop root mode = %v err=%v, want 0711 traversal without listing", rootInfo, err)
				}
				payloadInfo, err := os.Stat(filepath.Join(dropPath, "payload.txt"))
				if err != nil || payloadInfo.Mode().Perm() != 0o600 {
					t.Fatalf("payload base mode = %v err=%v, want 0600 before ACL semantics", payloadInfo, err)
				}
				wantACL := "-P\n-R\n-m\nu:alice:r-X\n--\n" + dropPath + "\n---\n"
				if got := readOptionalFile(t, h.aclLog); got != wantACL {
					t.Fatalf("write-time ACL grant = %q, want exactly one recursive target grant %q", got, wantACL)
				}
			},
		},
		{
			name:       "never partially pastes after generation change",
			panes:      "$7\tone\t%41\t111\t9001\n",
			session:    "one",
			fields:     map[string]string{"text": "race"},
			wantStatus: http.StatusConflict,
			setup: func(t *testing.T) {
				t.Setenv("TMUX_SEND_ATOMIC_CHANGED", "1")
			},
			assert: func(t *testing.T, h sendHarness, _ map[string]any) {
				if got := dropDirectoryCount(t, h.dropsDir); got != 0 {
					t.Fatalf("drop directories = %d after rejected generation, want 0", got)
				}
				log := readOptionalFile(t, h.tmuxLog)
				if strings.Count(log, "if-shell\n-F\n-t\n%41\n") != 1 || !strings.Contains(log, "delete-buffer") || strings.Contains(log, "\npaste-buffer\n") {
					t.Fatalf("generation rejection was not one guarded all-or-nothing paste with cleanup: %s", log)
				}
			},
		},
		{
			name:       "settles then submits exactly once",
			panes:      "$7\tone\t%41\t111\t9001\n",
			session:    "one",
			fields:     map[string]string{"text": "submit exactly once safely", "submit": "true"},
			wantStatus: http.StatusOK,
			setup: func(t *testing.T) {
				settled := filepath.Join(t.TempDir(), "paste-settled")
				t.Setenv("TMUX_SEND_REQUIRE_SETTLE", settled)
				settleDelays = nil
				tmuxSendSleep = func(_ context.Context, delay time.Duration) error {
					settleDelays = append(settleDelays, delay)
					if delay == tmuxSendSubmitSettleDelay {
						if err := os.WriteFile(settled, nil, 0o600); err != nil {
							t.Fatalf("record paste settle: %v", err)
						}
					}
					return nil
				}
			},
			assert: func(t *testing.T, h sendHarness, response map[string]any) {
				if response["transport"] != "pasted" || response["submissionRequested"] != true || response["submitKeyDispatched"] != true || response["bufferCleaned"] != true {
					t.Fatalf("guarded submit response = %#v", response)
				}
				if len(settleDelays) == 0 || settleDelays[0] != tmuxSendSubmitSettleDelay {
					t.Fatalf("initial settle delays = %v, want %s", settleDelays, tmuxSendSubmitSettleDelay)
				}
				log := readOptionalFile(t, h.tmuxLog)
				if strings.Count(log, "if-shell\n-F\n-t\n%41\n") != 2 || strings.Count(log, "send-keys -t %41 Enter") != 1 || !strings.Contains(log, "CHROTE_SEND_SUBMIT_KEY_DISPATCHED") {
					t.Fatalf("paste and exactly one Enter were not separately generation-guarded: %s", log)
				}
				if _, legacy := response["submitted"]; legacy {
					t.Fatalf("response claims application submission: %#v", response)
				}
			},
		},
		{
			name:       "reports an honest unknown transport outcome",
			panes:      "$7\tone\t%41\t111\t9001\n",
			session:    "one",
			fields:     map[string]string{"text": "uncertain transport"},
			wantStatus: http.StatusAccepted,
			setup: func(t *testing.T) {
				t.Setenv("TMUX_SEND_FAIL_PASTE", "1")
			},
			assert: func(t *testing.T, h sendHarness, response map[string]any) {
				if response["success"] != false || response["transport"] != "unknown" || response["retryable"] != false || response["deliveryConfirmed"] != false || response["targetVerified"] != false || response["bufferCleaned"] != true {
					t.Fatalf("unknown outcome response overclaims delivery: %#v", response)
				}
				if got := dropDirectoryCount(t, h.dropsDir); got != 1 {
					t.Fatalf("unknown outcome retained drops = %d, want 1 for inspection", got)
				}
				if !strings.Contains(readOptionalFile(t, h.tmuxLog), "delete-buffer") {
					t.Fatalf("unknown outcome did not attempt buffer cleanup: %s", readOptionalFile(t, h.tmuxLog))
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := newSendHarness(t, test.panes)
			if test.setup != nil {
				test.setup(t)
			}
			recorder := h.send(t, test.session, test.fields)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			response := map[string]any{}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			test.assert(t, h, response)
		})
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

// A send routed through the mux stores its own drop and reaches the pane through
// a tmux buffer, never through argv. Text on the command line would be visible
// to every process on the host and would break on any shell metacharacter, so
// the absence of the prompt text in the recorded argv is the point.
func TestSendToSessionStoresDropAndPastesViaBuffer(t *testing.T) {
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
	if response["success"] != true || response["session"] != "alice-shell" || response["unixUser"] != "alice" || response["pane"] != "%42" || response["submitKeyDispatched"] != true {
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
		"paste-buffer -p -d -b chrote-send-",
		"send-keys -t %42 Enter",
		atomicSendSubmitKeyMarker,
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
