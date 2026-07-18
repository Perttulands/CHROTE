package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chrote/server/internal/formations"
)

func TestFormationsRuntimeAPIRejectsBeforeWorkspaceMutationWithSafeTypedError(t *testing.T) {
	workspace := t.TempDir()
	privateRoot := filepath.Join(t.TempDir(), "host-private-missing")
	handler := NewFormationsHandlerWithStore(formations.NewRuntimeStore(workspace, privateRoot))
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	requests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/formations/runs", `{"board":"missing","missionId":"mission_missing"}`},
		{http.MethodPost, "/api/formations/runs/run_missing/resume", `{}`},
		{http.MethodPost, "/api/formations/runs/run_missing/abort", `{}`},
		{http.MethodPost, "/api/formations/runs/run_missing/gates/gate_missing/verdict", `{"verdict":"pass"}`},
	}
	for _, request := range requests {
		t.Run(request.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(request.method, request.path, strings.NewReader(request.body)))
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503: %s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), `"code":"RUNTIME_AUTHORITY_NON_AUTHORIZING"`) {
				t.Fatalf("response lacks stable authority code: %s", recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), privateRoot) || strings.Contains(recorder.Body.String(), workspace) || strings.Contains(recorder.Body.String(), "wsa_") {
				t.Fatalf("response leaked private authority identity: %s", recorder.Body.String())
			}
		})
	}

	definitions := httptest.NewRecorder()
	mux.ServeHTTP(definitions, httptest.NewRequest(http.MethodGet, "/api/formations/boards", nil))
	if definitions.Code != http.StatusOK {
		t.Fatalf("schema-1 definitions status = %d, want 200: %s", definitions.Code, definitions.Body.String())
	}
	if matches, err := filepath.Glob(filepath.Join(workspace, ".formations", "runs", "*")); err != nil || len(matches) != 0 {
		t.Fatalf("runtime rejection left workspace artifacts: matches=%v err=%v", matches, err)
	}
}
