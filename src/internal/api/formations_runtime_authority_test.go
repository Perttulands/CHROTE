package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrote/server/internal/formations"
)

func TestFormationsRuntimeAPIStartDefinitionErrorsPrecedeUnavailableAuthority(t *testing.T) {
	tests := []struct {
		name       string
		slug       string
		board      string
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "mission missing board",
			body:       `{"board":"missing","missionId":"mis_missing"}`,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
		{
			name:       "formation missing board",
			body:       `{"board":"missing","formationId":"fmn_missing"}`,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
		{
			name:       "mission missing root",
			slug:       "session-search",
			board:      formationsAPIS5CascadeBoardFixture(),
			body:       `{"board":"session-search","missionId":"mis_missing"}`,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
		{
			name:       "formation missing root",
			slug:       "session-search",
			board:      formationsAPIS5CascadeBoardFixture(),
			body:       `{"board":"session-search","formationId":"fmn_missing"}`,
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
		{
			name:       "mission reachable legacy script gate",
			slug:       "session-search",
			board:      formationsAPILegacyScriptGateBoardFixture(),
			body:       `{"board":"session-search","missionId":"mis_showcase"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   formations.LegacyScriptGateMigrationCode,
		},
		{
			name:       "mission legacy inline verification",
			slug:       "legacy-inline",
			board:      formationsAPILegacyInlineVerificationFixture(),
			body:       `{"board":"legacy-inline","missionId":"mis_main"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   formations.LegacyInlineVerificationMigrationCode,
		},
		{
			name:       "formation legacy inline verification",
			slug:       "legacy-inline",
			board:      formationsAPILegacyInlineVerificationFixture(),
			body:       `{"board":"legacy-inline","formationId":"fmn_work"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   formations.LegacyInlineVerificationMigrationCode,
		},
		{
			name:       "Mission reaches non-executing Tool",
			slug:       "tool-parity",
			board:      formationsAPIRuntimeToolBoardFixture(),
			body:       `{"board":"tool-parity","missionId":"mis_main"}`,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "tool_execution_unavailable",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			privateRoot := filepath.Join(t.TempDir(), "wsa_private_authority")
			store := formations.NewRuntimeStore(workspace, privateRoot)
			if test.board != "" {
				writeFormationsAPIFixture(t, store.BoardPath(test.slug), test.board)
			}
			tmuxCapture := installRuntimeAuthorityAPITmuxTripwire(t, workspace)
			handler := NewFormationsHandlerWithStore(store)
			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)

			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/formations/runs", strings.NewReader(test.body)))
			if recorder.Code != test.wantStatus {
				t.Errorf("status = %d, want %d: %s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			body := recorder.Body.String()
			if !strings.Contains(body, `"code":"`+test.wantCode+`"`) {
				t.Errorf("response lacks selected-definition code %q: %s", test.wantCode, body)
			}
			if strings.Contains(body, `"code":"RUNTIME_AUTHORITY_NON_AUTHORIZING"`) {
				t.Errorf("runtime authority masked selected-definition error: %s", body)
			}
			assertRuntimeAuthorityAPIResponseIsPrivate(t, body, workspace, privateRoot)
			assertNoRuntimeAuthorityAPIEffects(t, workspace, tmuxCapture)
		})
	}
}

func formationsAPIRuntimeToolBoardFixture() string {
	return formationsAPIToolParityBoardFixture() + `
[[connection]]
id = "edge_mission_tool"
channel = "workflow"
from = "mis_main:out"
to = "tool_normalize:port_tool_in"
`
}

func TestFormationsRuntimeAPIResumeAbortAndVerdictRemainAuthorityFirst(t *testing.T) {
	workspace := t.TempDir()
	privateRoot := filepath.Join(t.TempDir(), "wsa_private_authority")
	tmuxCapture := installRuntimeAuthorityAPITmuxTripwire(t, workspace)
	handler := NewFormationsHandlerWithStore(formations.NewRuntimeStore(workspace, privateRoot))
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	requests := []struct {
		name string
		path string
		body string
	}{
		{"resume", "/api/formations/runs/run_missing/resume", `{}`},
		{"abort", "/api/formations/runs/run_missing/abort", `{}`},
		{"verdict", "/api/formations/runs/run_missing/gates/gate_missing/verdict", `{"verdict":"pass"}`},
	}
	for _, request := range requests {
		t.Run(request.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, request.path, strings.NewReader(request.body)))
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503: %s", recorder.Code, recorder.Body.String())
			}
			body := recorder.Body.String()
			if !strings.Contains(body, `"code":"RUNTIME_AUTHORITY_NON_AUTHORIZING"`) {
				t.Fatalf("response lacks stable authority code: %s", body)
			}
			assertRuntimeAuthorityAPIResponseIsPrivate(t, body, workspace, privateRoot)
			assertNoRuntimeAuthorityAPIEffects(t, workspace, tmuxCapture)
		})
	}

	definitions := httptest.NewRecorder()
	mux.ServeHTTP(definitions, httptest.NewRequest(http.MethodGet, "/api/formations/boards", nil))
	if definitions.Code != http.StatusOK {
		t.Fatalf("schema-1 definitions status = %d, want 200: %s", definitions.Code, definitions.Body.String())
	}
}

func installRuntimeAuthorityAPITmuxTripwire(t *testing.T, workspace string) string {
	t.Helper()
	binDir := t.TempDir()
	capturePath := filepath.Join(t.TempDir(), "tmux-called")
	fakeTmux := filepath.Join(binDir, "tmux")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$FORMATIONS_RUNTIME_AUTHORITY_TMUX_CAPTURE\"\nexit 99\n"
	if err := os.WriteFile(fakeTmux, []byte(script), 0o755); err != nil {
		t.Fatalf("write tmux tripwire: %v", err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("FORMATIONS_RUNTIME_AUTHORITY_TMUX_CAPTURE", capturePath)
	t.Setenv("CHROTE_FORMATIONS_LAB_HARNESSES", "")
	t.Setenv("CHROTE_FORMATIONS_TMUX_HARNESSES", "openai-codex")
	t.Setenv("CHROTE_FORMATIONS_TMUX_SOCKET", filepath.Join(t.TempDir(), "default"))
	t.Setenv("CHROTE_FORMATIONS_TMUX_CWD", workspace)
	t.Setenv("CHROTE_FORMATIONS_TMUX_ROOTS", workspace)
	return capturePath
}

func assertRuntimeAuthorityAPIResponseIsPrivate(t *testing.T, body, workspace, privateRoot string) {
	t.Helper()
	if strings.Contains(body, workspace) || strings.Contains(body, privateRoot) || strings.Contains(body, "wsa_") {
		t.Fatalf("response leaked private authority identity: %s", body)
	}
}

func assertNoRuntimeAuthorityAPIEffects(t *testing.T, workspace, tmuxCapture string) {
	t.Helper()
	if matches, err := filepath.Glob(filepath.Join(workspace, ".formations", "runs", "*")); err != nil || len(matches) != 0 {
		t.Fatalf("runtime rejection left workspace artifacts: matches=%v err=%v", matches, err)
	}
	if raw, err := os.ReadFile(tmuxCapture); err == nil {
		t.Fatalf("runtime rejection reached tmux: %s", raw)
	} else if !os.IsNotExist(err) {
		t.Fatalf("inspect tmux tripwire: %v", err)
	}
}
