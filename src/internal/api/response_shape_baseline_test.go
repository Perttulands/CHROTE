package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// Intentionally flat endpoints pinned by home-idhj.14:
// /api/health, /api/version, and tmux endpoints that dashboard code reads
// directly without a top-level data envelope.
func TestResponseShapeBaseline_FlatHealthEndpointsDoNotUseDataEnvelope(t *testing.T) {
	handler := NewHealthHandlerWithVersion("test-version")

	tests := []struct {
		name     string
		path     string
		call     func(http.ResponseWriter, *http.Request)
		wantKeys []string
	}{
		{
			name:     "health",
			path:     "/api/health",
			call:     handler.Health,
			wantKeys: []string{"commit", "status", "timestamp", "version"},
		},
		{
			name:     "version",
			path:     "/api/version",
			call:     handler.Version,
			wantKeys: []string{"version"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			tt.call(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			response := decodeJSONMap(t, rec)
			assertTopLevelKeys(t, response, tt.wantKeys)
			assertNoTopLevelKey(t, response, "data")
		})
	}
}

func TestResponseShapeBaseline_BeadsHealthUsesSuccessDataEnvelope(t *testing.T) {
	resetBeadsTestEnv(t)
	makeFakeBdCommand(t, "bd version 1.2.3\n")

	handler := NewBeadsHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/beads/health", nil)
	rec := httptest.NewRecorder()

	handler.Health(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	response := decodeJSONMap(t, rec)
	assertTopLevelKeys(t, response, []string{"data", "success", "timestamp"})
	if response["success"] != true {
		t.Fatalf("success = %v, want true", response["success"])
	}

	data, ok := response["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data = %T, want object", response["data"])
	}
	assertTopLevelKeys(t, data, []string{"allowedRoots", "bdVersion", "configuredWorkspaces", "status"})
}

func TestResponseShapeBaseline_FlatTmuxEndpointsDoNotUseDataEnvelope(t *testing.T) {
	installFakeTmux(t)
	handler := NewTmuxHandler()

	tests := []struct {
		name      string
		method    string
		path      string
		body      string
		pathName  string
		headerKey string
		headerVal string
		call      func(http.ResponseWriter, *http.Request)
		wantKeys  []string
	}{
		{
			name:     "list sessions",
			method:   http.MethodGet,
			path:     "/api/tmux/sessions",
			call:     handler.ListSessions,
			wantKeys: []string{"banked", "grouped", "managed", "recoveryEvidence", "sessions", "sources", "terminalUsers", "timestamp"},
		},
		{
			name:     "create session",
			method:   http.MethodPost,
			path:     "/api/tmux/sessions",
			body:     `{"name":"baseline-session"}`,
			call:     handler.CreateSession,
			wantKeys: []string{"session", "success", "timestamp"},
		},
		{
			name:     "delete session",
			method:   http.MethodDelete,
			path:     "/api/tmux/sessions/baseline-session",
			pathName: "baseline-session",
			call:     handler.DeleteSession,
			wantKeys: []string{"killed", "success", "timestamp"},
		},
		{
			name:      "delete all sessions",
			method:    http.MethodDelete,
			path:      "/api/tmux/sessions/all",
			headerKey: "X-Nuke-Confirm",
			headerVal: "DASHBOARD-NUKE-CONFIRMED",
			call:      handler.DeleteAllSessions,
			wantKeys:  []string{"killed", "protected", "sessions", "success", "timestamp"},
		},
		{
			name:     "rename session",
			method:   http.MethodPatch,
			path:     "/api/tmux/sessions/old-session",
			pathName: "old-session",
			body:     `{"newName":"new-session"}`,
			call:     handler.RenameSession,
			wantKeys: []string{"newName", "oldName", "success", "timestamp"},
		},
		{
			name:     "capture pane",
			method:   http.MethodGet,
			path:     "/api/tmux/sessions/baseline-session/capture",
			pathName: "baseline-session",
			call:     handler.CapturePane,
			wantKeys: []string{"content", "session"},
		},
		{
			name:     "apply appearance",
			method:   http.MethodPost,
			path:     "/api/tmux/appearance",
			body:     `{"statusBg":"black","statusFg":"white","paneBorderActive":"green"}`,
			call:     handler.ApplyAppearance,
			wantKeys: []string{"applied", "success", "timestamp", "total"},
		},
		{
			name:     "set mouse mode",
			method:   http.MethodPost,
			path:     "/api/tmux/mouse",
			body:     `{"enabled":true}`,
			call:     handler.SetMouseMode,
			wantKeys: []string{"applied", "mouse", "success", "timestamp", "total"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			if tt.pathName != "" {
				req.SetPathValue("name", tt.pathName)
			}
			if tt.headerKey != "" {
				req.Header.Set(tt.headerKey, tt.headerVal)
			}
			rec := httptest.NewRecorder()

			tt.call(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			response := decodeJSONMap(t, rec)
			assertTopLevelKeys(t, response, tt.wantKeys)
			assertNoTopLevelKey(t, response, "data")
		})
	}
}

func TestTmuxHandler_DeleteAllSessionsRequiresExactNukeConfirmationHeader(t *testing.T) {
	_, argsPath := installFakeTmux(t)
	handler := NewTmuxHandler()

	tests := []struct {
		name        string
		headerValue string
		wantStatus  int
		wantCalls   []string
	}{
		{
			name:       "missing confirmation",
			wantStatus: http.StatusForbidden,
			wantCalls:  nil,
		},
		{
			name:        "wrong confirmation",
			headerValue: "DASHBOARD-NUKE",
			wantStatus:  http.StatusForbidden,
			wantCalls:   nil,
		},
		{
			name:        "wrong case confirmation",
			headerValue: "dashboard-nuke-confirmed",
			wantStatus:  http.StatusForbidden,
			wantCalls:   nil,
		},
		{
			name:        "confirmation with trailing space",
			headerValue: "DASHBOARD-NUKE-CONFIRMED ",
			wantStatus:  http.StatusForbidden,
			wantCalls:   nil,
		},
		{
			name:        "exact confirmation",
			headerValue: "DASHBOARD-NUKE-CONFIRMED",
			wantStatus:  http.StatusOK,
			wantCalls: []string{
				"list-sessions -F #{session_name}",
				"kill-session -t alpha",
				"kill-session -t beta",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(argsPath, nil, 0600); err != nil {
				t.Fatalf("reset tmux args: %v", err)
			}
			req := httptest.NewRequest(http.MethodDelete, "/api/tmux/sessions/all", nil)
			if tt.headerValue != "" {
				req.Header.Set("X-Nuke-Confirm", tt.headerValue)
			}
			rec := httptest.NewRecorder()

			handler.DeleteAllSessions(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if gotCalls := readFakeCommandCalls(t, argsPath); !reflect.DeepEqual(gotCalls, tt.wantCalls) {
				t.Fatalf("tmux calls = %#v, want %#v", gotCalls, tt.wantCalls)
			}
		})
	}
}

func decodeJSONMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()

	var response map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode JSON response: %v\nbody: %s", err, rec.Body.String())
	}
	return response
}

func assertTopLevelKeys(t *testing.T, response map[string]interface{}, want []string) {
	t.Helper()

	got := make([]string, 0, len(response))
	for key := range response {
		got = append(got, key)
	}
	sort.Strings(got)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("top-level keys = %#v, want %#v", got, want)
	}
}

func assertNoTopLevelKey(t *testing.T, response map[string]interface{}, key string) {
	t.Helper()

	if _, ok := response[key]; ok {
		t.Fatalf("unexpected top-level %q key in response %#v", key, response)
	}
}

func installFakeTmux(t *testing.T) (string, string) {
	t.Helper()

	dir := t.TempDir()
	argsPath := filepath.Join(dir, "tmux-args.txt")
	scriptPath := filepath.Join(dir, "tmux")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$TMUX_ARGS_FILE"
case "$*" in
  "list-sessions -F #{session_name}:#{session_windows}:#{session_attached}")
    printf 'alpha:1:0\nbeta:2:1\n'
    ;;
  "list-sessions -F #{session_name}")
    printf 'alpha\nbeta\n'
    ;;
  capture-pane*)
    printf 'line one\nline two\n'
    ;;
  *new-session*)
    printf '$42\n'
    ;;
  *)
    printf ''
    ;;
esac
`
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		t.Fatalf("write fake tmux command: %v", err)
	}
	if err := os.WriteFile(argsPath, nil, 0600); err != nil {
		t.Fatalf("write fake tmux args file: %v", err)
	}

	t.Setenv("TMUX_ARGS_FILE", argsPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return scriptPath, argsPath
}

func readFakeCommandCalls(t *testing.T, argsPath string) []string {
	t.Helper()

	raw, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read fake command calls: %v", err)
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil
	}
	return strings.Split(string(raw), "\n")
}
