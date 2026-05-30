package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLoadGasCityConfigFromEnvDefaultsToLoopbackSupervisor(t *testing.T) {
	t.Setenv("CHROTE_GASCITY_URL", "")

	cfg := LoadGasCityConfigFromEnv()

	if cfg.BaseURL != "http://127.0.0.1:8372" {
		t.Fatalf("BaseURL = %q, want default localhost supervisor", cfg.BaseURL)
	}
	if cfg.CityDir != defaultGasCityCityDir {
		t.Fatalf("CityDir = %q, want default local city", cfg.CityDir)
	}
}

// TestGasCityChildEnvPrependsExtraPathForGCInvocation encodes the deployed-502
// root cause: the chrote.service PATH resolves tmux to an older /usr/bin/tmux
// that cannot read the Gas City supervisor's tmux server, so gc session peek
// fails. gc invocations must run with the supervisor's tmux dir prepended to
// PATH. This test pins that the child PATH starts with the extra dir.
func TestGasCityChildEnvPrependsExtraPathForGCInvocation(t *testing.T) {
	sep := string(os.PathListSeparator)
	t.Setenv("PATH", "/usr/bin"+sep+"/bin")

	env := gasCityChildEnv("/home/linuxbrew/.linuxbrew/bin")

	var gotPath string
	pathCount := 0
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			gotPath = strings.TrimPrefix(kv, "PATH=")
			pathCount++
		}
	}
	if pathCount != 1 {
		t.Fatalf("child env has %d PATH entries, want exactly 1", pathCount)
	}
	want := "/home/linuxbrew/.linuxbrew/bin" + sep + "/usr/bin" + sep + "/bin"
	if gotPath != want {
		t.Fatalf("child PATH = %q, want extra dir prepended: %q", gotPath, want)
	}
}

func TestGasCityChildEnvEmptyExtraPathIsParentEnv(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	env := gasCityChildEnv("")
	for _, kv := range env {
		if kv == "PATH=/usr/bin" {
			return
		}
	}
	t.Fatalf("empty extra path should leave PATH unchanged; env=%v", env)
}

func TestGasCityExecRunnerDoesNotWaitForDescendantOutputHandles(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()

	start := time.Now()
	output, err := (gasCityExecRunner{}).Run(ctx, "/bin/sh", []string{"-c", `printf '{"ok":true}\n'; (sleep 2) &`})
	if err != nil {
		t.Fatalf("runner returned error before shell exited: %v; output=%q", err, output)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("runner waited %s for descendant-owned output handles; want prompt return after gc exits", elapsed)
	}
	if !strings.Contains(output, `"ok":true`) {
		t.Fatalf("output = %q, want captured command output", output)
	}
}

func TestMergePathPrependDeduplicates(t *testing.T) {
	sep := string(os.PathListSeparator)
	// The supervisor dir already present in PATH must not be duplicated, so the
	// child PATH does not grow on every restart.
	got := mergePathPrepend("/brew/bin", "/usr/bin"+sep+"/brew/bin"+sep+"/bin")
	want := "/brew/bin" + sep + "/usr/bin" + sep + "/bin"
	if got != want {
		t.Fatalf("merged PATH = %q, want deduplicated %q", got, want)
	}
}

func TestResolveGasCityGCExtraPathOffDisables(t *testing.T) {
	t.Setenv("CHROTE_GASCITY_GC_PATH", "off")
	if got := resolveGasCityGCExtraPath(); got != "" {
		t.Fatalf("extra path = %q, want empty when set to off", got)
	}
}

func TestResolveGasCityGCExtraPathHonorsExplicitOverride(t *testing.T) {
	t.Setenv("CHROTE_GASCITY_GC_PATH", "/custom/tmux/bin")
	if got := resolveGasCityGCExtraPath(); got != "/custom/tmux/bin" {
		t.Fatalf("extra path = %q, want explicit override", got)
	}
}

func TestResolveGasCityGCExtraPathPicksDirWithDistinctTmux(t *testing.T) {
	t.Setenv("CHROTE_GASCITY_GC_PATH", "")
	dir := t.TempDir()
	tmuxPath := filepath.Join(dir, "tmux")
	if err := os.WriteFile(tmuxPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}
	// A dir without tmux must be skipped; the dir with an executable tmux wins.
	emptyDir := t.TempDir()
	orig := gasCityTmuxCandidateDirs
	gasCityTmuxCandidateDirs = []string{emptyDir, dir}
	t.Cleanup(func() { gasCityTmuxCandidateDirs = orig })

	if got := resolveGasCityGCExtraPath(); got != dir {
		t.Fatalf("extra path = %q, want candidate dir with executable tmux %q", got, dir)
	}
}

func TestResolveGasCityGCExtraPathSkipsServiceTmuxDir(t *testing.T) {
	t.Setenv("CHROTE_GASCITY_GC_PATH", "")
	// If the only candidate is the service tmux dir itself, add nothing (it is
	// the incompatible 3.4 build we must NOT prefer).
	orig := gasCityTmuxCandidateDirs
	gasCityTmuxCandidateDirs = []string{filepath.Dir(gasCityServiceTmux)}
	t.Cleanup(func() { gasCityTmuxCandidateDirs = orig })

	if got := resolveGasCityGCExtraPath(); got != "" {
		t.Fatalf("extra path = %q, want empty when only candidate is the service tmux dir", got)
	}
}

func TestGasCityHandlerObserverFetchesReadOnlySessionMetadata(t *testing.T) {
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected upstream mutation %s %s", r.Method, r.URL.RequestURI())
		}
		seen = append(seen, r.Method+" "+r.URL.RequestURI())

		switch r.URL.RequestURI() {
		case "/health":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"status":         "ok",
				"version":        "dev",
				"uptime_sec":     42,
				"cities_total":   1,
				"cities_running": 1,
				"startup": map[string]any{
					"ready": true,
					"phase": "running",
				},
			})
		case "/v0/cities":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"items": []map[string]any{
					{"name": "testcity", "path": "/tmp/testcity", "running": true},
				},
				"total": 1,
			})
		case "/v0/city/testcity/sessions?limit=100&peek=false&state=all":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"items": []map[string]any{
					{
						"id":           "gc-1",
						"title":        "Planner",
						"alias":        "planner",
						"template":     "planner",
						"state":        "active",
						"provider":     "./bin/mock-agent",
						"session_name": "planner",
						"created_at":   "2026-05-26T10:00:00Z",
						"last_active":  "2026-05-26T10:01:00Z",
						"running":      true,
						"attached":     true,
					},
				},
				"total": 1,
			})
		default:
			t.Fatalf("unexpected upstream request %s", r.URL.RequestURI())
		}
	}))
	defer upstream.Close()

	handler := NewGasCityHandler(GasCityConfig{BaseURL: upstream.URL})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/gascity/observer", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Success bool                    `json:"success"`
		Data    GasCityObserverResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode observer response: %v", err)
	}
	if !response.Success {
		t.Fatal("observer response should use success envelope")
	}
	data := response.Data
	if data.Status != "ok" {
		t.Fatalf("observer status = %q, want ok; errors=%+v", data.Status, data.UpstreamErrors)
	}
	if len(data.Sessions) != 1 {
		t.Fatalf("sessions = %+v, want one Gas City session", data.Sessions)
	}
	session := data.Sessions[0]
	if session.Source != "gascity" || session.ID != "gc-1" || session.Name != "planner" || session.Title != "Planner" || session.Alias != "planner" || session.Template != "planner" || session.Status != "active" || session.AttachTarget != "gc:gc-1" || !session.Running || !session.Attached {
		t.Fatalf("session = %+v, want read-only identity metadata with stable gc attach target", session)
	}
	wantSeen := strings.Join([]string{
		"GET /health",
		"GET /v0/cities",
		"GET /v0/city/testcity/sessions?limit=100&peek=false&state=all",
	}, "\n")
	if gotSeen := strings.Join(seen, "\n"); gotSeen != wantSeen {
		t.Fatalf("upstream requests = %q, want only read-only session metadata requests %q", gotSeen, wantSeen)
	}

	var raw struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw observer response: %v", err)
	}
	for _, forbidden := range []string{"health", "cities", "mail", "work", "formulas", "molecules", "wisps", "convoys", "recentEvents"} {
		if _, ok := raw.Data[forbidden]; ok {
			t.Fatalf("observer response retained %q hidden surface: %s", forbidden, rec.Body.String())
		}
	}
}

func TestGasCityHandlerObserverReportsUnavailableWithoutRunningSupervisor(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	upstream.Close()

	handler := NewGasCityHandlerWithClient(
		GasCityConfig{BaseURL: upstream.URL},
		&http.Client{Timeout: 10 * time.Millisecond},
	)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/gascity/observer", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data GasCityObserverResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode observer response: %v", err)
	}
	if response.Data.Status != "unavailable" {
		t.Fatalf("status = %q, want unavailable", response.Data.Status)
	}
	if response.Data.Error == "" {
		t.Fatal("unavailable observer should include a clear error")
	}
	if len(response.Data.Sessions) != 0 {
		t.Fatalf("unavailable observer should return no sessions, got %+v", response.Data.Sessions)
	}
	body := rec.Body.String()
	if strings.Contains(body, upstream.URL) || strings.Contains(body, "127.0.0.1") || strings.Contains(body, "connect:") {
		t.Fatalf("unavailable response exposed internal transport details: %s", body)
	}
}

func TestGasCityHandlerObserverBlocksNonLoopbackConfig(t *testing.T) {
	handler := NewGasCityHandler(GasCityConfig{BaseURL: "http://example.com:8372"})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/gascity/observer", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data GasCityObserverResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode observer response: %v", err)
	}
	if response.Data.Status != "misconfigured" {
		t.Fatalf("status = %q, want misconfigured", response.Data.Status)
	}
	if !strings.Contains(response.Data.Error, "localhost or loopback") {
		t.Fatalf("error = %q, want localhost-only guidance", response.Data.Error)
	}
}

func TestGasCityHandlerObserverDegradesWhenSessionRouteFails(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.RequestURI() {
		case "/health":
			writeTestJSON(w, http.StatusOK, map[string]any{"status": "ok"})
		case "/v0/cities":
			writeTestJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{{"name": "testcity", "running": true}}})
		case "/v0/city/testcity/sessions?limit=100&peek=false&state=all":
			writeTestJSON(w, http.StatusServiceUnavailable, map[string]any{"detail": "sessions unavailable"})
		default:
			t.Fatalf("unexpected upstream request %s", r.URL.RequestURI())
		}
	}))
	defer upstream.Close()

	handler := NewGasCityHandler(GasCityConfig{BaseURL: upstream.URL})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/gascity/observer", nil))

	var response struct {
		Data GasCityObserverResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode observer response: %v", err)
	}
	if response.Data.Status != "degraded" {
		t.Fatalf("status = %q, want degraded", response.Data.Status)
	}
	if len(response.Data.Sessions) != 0 {
		t.Fatalf("sessions = %+v, want empty sessions when route fails", response.Data.Sessions)
	}
	if len(response.Data.UpstreamErrors) != 1 || response.Data.UpstreamErrors[0].Route != "/v0/city/{city}/sessions" {
		t.Fatalf("upstream errors = %+v, want session-route error only", response.Data.UpstreamErrors)
	}
}

func TestGasCityHandlerDoesNotRegisterHiddenProductSurfaceRoutes(t *testing.T) {
	handler := NewGasCityHandler(GasCityConfig{BaseURL: "http://127.0.0.1:8372"})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/gascity/requests/pi-poem", body: `{"topic":"mail routes"}`},
		{method: http.MethodGet, path: "/api/gascity/workflows/review-quorum/capability"},
		{method: http.MethodPost, path: "/api/gascity/workflows/review-quorum", body: `{"subject":"home-123"}`},
		{method: http.MethodGet, path: "/api/gascity/mail"},
		{method: http.MethodGet, path: "/api/gascity/audit"},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s %s status = %d, want 404 after removing hidden Gas City product surfaces; body: %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestGasCityHandlerCreateSessionRunsNativeGCSessionNew(t *testing.T) {
	cityDir := t.TempDir()
	runner := &fakeGasCityRunner{output: `{
		"schema_version":"gascity.session.new.result.v1",
		"ok":true,
		"session_id":"ga-9001",
		"session_name":"codxia",
		"template":"codex-smoke",
		"transport":"",
		"work_dir":"/tmp/codxia",
		"deferred_start":true,
		"attached":false,
		"alias":"codxia"
	}`}
	handler := NewGasCityHandler(GasCityConfig{CityDir: cityDir})
	handler.runner = runner
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/gascity/sessions", strings.NewReader(`{
		"name":"codxia",
		"template":"codex-smoke",
		"title":"Codxia agent"
	}`))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if runner.calls != 1 || runner.name != "gc" {
		t.Fatalf("runner calls/name = %d/%q, want one gc invocation", runner.calls, runner.name)
	}
	wantArgs := []string{"--city", cityDir, "session", "new", "codex-smoke", "--alias", "codxia", "--title", "Codxia agent", "--no-attach", "--json"}
	if strings.Join(runner.args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want native gc session new argv %#v", runner.args, wantArgs)
	}

	var response struct {
		Success bool                         `json:"success"`
		Data    GasCityCreateSessionResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if !response.Success {
		t.Fatal("create response should use success envelope")
	}
	if response.Data.Source != "gascity" || response.Data.ID != "ga-9001" || response.Data.AttachTarget != "gc:ga-9001" {
		t.Fatalf("response = %+v, want Gas City source with gc attach target", response.Data)
	}
	if response.Data.Name != "codxia" || response.Data.SessionName != "codxia" || response.Data.Template != "codex-smoke" || response.Data.Transport != "" || response.Data.WorkDir != "/tmp/codxia" {
		t.Fatalf("response = %+v, want parsed gc session new result fields", response.Data)
	}
	if response.Data.Attached || !response.Data.DeferredStart {
		t.Fatalf("response booleans = attached %v deferred %v, want no-attach deferred result", response.Data.Attached, response.Data.DeferredStart)
	}
}

func TestGasCityHandlerCreateSessionDefaultsEmptyTitleToAliasInArgv(t *testing.T) {
	cityDir := t.TempDir()
	runner := &fakeGasCityRunner{output: `{
		"schema_version":"gascity.session.new.result.v1",
		"ok":true,
		"session_id":"ga-9002",
		"session_name":"codxia",
		"template":"planner",
		"transport":"tmux",
		"work_dir":"/tmp/codxia",
		"deferred_start":false,
		"attached":false,
		"alias":"codxia"
	}`}
	handler := NewGasCityHandler(GasCityConfig{CityDir: cityDir})
	handler.runner = runner
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/gascity/sessions", strings.NewReader(`{"name":"codxia","template":"planner"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	wantArgs := []string{"--city", cityDir, "session", "new", "planner", "--alias", "codxia", "--title", "codxia", "--no-attach", "--json"}
	if strings.Join(runner.args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want title defaulted explicitly in argv %#v", runner.args, wantArgs)
	}
}

func TestGasCityHandlerCreateSessionValidatesRequestBeforeGC(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "empty name", body: `{"name":"","template":"planner"}`},
		{name: "unsafe name", body: `{"name":"bad name","template":"planner"}`},
		{name: "empty template", body: `{"name":"codxia","template":""}`},
		{name: "unsafe template", body: `{"name":"codxia","template":"../planner"}`},
		{name: "control title", body: "{\"name\":\"codxia\",\"template\":\"planner\",\"title\":\"bad\\ncaption\"}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeGasCityRunner{}
			handler := NewGasCityHandler(GasCityConfig{CityDir: t.TempDir()})
			handler.runner = runner
			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/gascity/sessions", strings.NewReader(tc.body)))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
			}
			if runner.calls != 0 {
				t.Fatalf("runner calls = %d, want no gc invocation for invalid request", runner.calls)
			}
			var response struct {
				Success bool `json:"success"`
				Error   struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response.Success || response.Error.Code != "BAD_REQUEST" {
				t.Fatalf("response = %+v, want BAD_REQUEST envelope", response)
			}
		})
	}
}

func TestGasCityHandlerCreateSessionReturnsFailureEnvelopeFromGCErrorJSON(t *testing.T) {
	runner := &fakeGasCityRunner{
		output: `{
			"schema_version":"gascity.session.new.result.v1",
			"ok":false,
			"error":{"code":"session_create_failed","message":"beads creation timed out","exit_code":124}
		}`,
		err: errors.New("exit status 124"),
	}
	handler := NewGasCityHandler(GasCityConfig{CityDir: t.TempDir()})
	handler.runner = runner
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/gascity/sessions", strings.NewReader(`{"name":"codxia","template":"planner"}`)))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Success || response.Error.Code != "GASCITY_SESSION_CREATE_FAILED" {
		t.Fatalf("response = %+v, want Gas City create failure envelope", response)
	}
	if !strings.Contains(response.Error.Message, "beads creation timed out") {
		t.Fatalf("error message = %q, want gc failure message", response.Error.Message)
	}
}

func TestGasCityHandlerCreateSessionRejectsInvalidGCJSON(t *testing.T) {
	runner := &fakeGasCityRunner{output: `{"ok":true,"session_id":""}`}
	handler := NewGasCityHandler(GasCityConfig{CityDir: t.TempDir()})
	handler.runner = runner
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/gascity/sessions", strings.NewReader(`{"name":"codxia","template":"planner"}`)))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Success || response.Error.Code != "GASCITY_INVALID_RESPONSE" {
		t.Fatalf("response = %+v, want invalid-response envelope", response)
	}
}

func TestGasCityHandlerTranscriptPeeksActiveMockSessionByID(t *testing.T) {
	runner := &fakeGasCityRunner{output: "\x1b[31mplanner ready\x1b[0m\r\nlatest mock output\x00\n"}
	cityDir := t.TempDir()
	upstream := newTestGasCitySupervisor(t, cityDir, []map[string]any{
		{
			"id":           "gc-4171",
			"alias":        "planner",
			"template":     "planner",
			"state":        "active",
			"running":      true,
			"session_name": "planner",
			"title":        "planner",
			"provider":     "./bin/mock-agent",
		},
	})
	defer upstream.Close()
	handler := NewGasCityHandler(GasCityConfig{BaseURL: upstream.URL, CityDir: cityDir})
	handler.runner = runner
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/gascity/sessions/gc-4171/transcript?lines=3", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want one read-only gc peek", runner.calls)
	}
	if runner.name != "gc" {
		t.Fatalf("runner name = %q, want gc", runner.name)
	}
	wantArgs := []string{"--city", cityDir, "session", "peek", "gc-4171", "--lines", "3"}
	if strings.Join(runner.args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want %#v", runner.args, wantArgs)
	}

	var response struct {
		Success bool                      `json:"success"`
		Data    GasCityTranscriptResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode transcript response: %v", err)
	}
	if !response.Success {
		t.Fatal("transcript response should use success envelope")
	}
	if response.Data.Source != "gc-session-peek" || response.Data.SessionID != "gc-4171" || response.Data.Alias != "planner" || response.Data.Template != "planner" || response.Data.State != "active" {
		t.Fatalf("metadata = %+v, want gc-session-peek active planner metadata", response.Data)
	}
	if response.Data.Lines != 3 || response.Data.LineCount != 2 {
		t.Fatalf("line metadata = lines %d count %d, want request lines and sanitized output count", response.Data.Lines, response.Data.LineCount)
	}
	if response.Data.Transcript != "planner ready\nlatest mock output\n" || response.Data.Truncated {
		t.Fatalf("transcript = %q truncated=%v, want sanitized untruncated pane output", response.Data.Transcript, response.Data.Truncated)
	}
	if strings.ContainsAny(response.Data.Transcript, "\x00\x1b") {
		t.Fatalf("transcript leaked terminal control bytes: %q", response.Data.Transcript)
	}
}

func TestGasCityHandlerTranscriptPeeksAsleepSessionByID(t *testing.T) {
	runner := &fakeGasCityRunner{output: "before suspend\nresumed evidence\n"}
	cityDir := t.TempDir()
	upstream := newTestGasCitySupervisor(t, cityDir, []map[string]any{
		{
			"id":           "gc-4170",
			"alias":        "pi-smoke",
			"template":     "pi-smoke",
			"state":        "asleep",
			"running":      false,
			"session_name": "pi-smoke",
			"title":        "pi-smoke",
		},
	})
	defer upstream.Close()
	handler := NewGasCityHandler(GasCityConfig{BaseURL: upstream.URL, CityDir: cityDir})
	handler.runner = runner
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/gascity/sessions/gc-4170/transcript", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for supervisor-known asleep session; body: %s", rec.Code, rec.Body.String())
	}
	wantArgs := []string{"--city", cityDir, "session", "peek", "gc-4170", "--lines", "120"}
	if strings.Join(runner.args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want default-lines peek args %#v", runner.args, wantArgs)
	}
	var response struct {
		Data GasCityTranscriptResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode transcript response: %v", err)
	}
	if response.Data.SessionID != "gc-4170" || response.Data.State != "asleep" || response.Data.Lines != gasCityTranscriptDefaultLines {
		t.Fatalf("response = %+v, want asleep session metadata and default line bound", response.Data)
	}
	if response.Data.Transcript != "before suspend\nresumed evidence\n" || response.Data.LineCount != 2 {
		t.Fatalf("transcript = %q lineCount=%d, want recovered asleep/resumed evidence", response.Data.Transcript, response.Data.LineCount)
	}
}

func TestGasCityHandlerTranscriptRejectsAliasTarget(t *testing.T) {
	runner := &fakeGasCityRunner{}
	handler := NewGasCityHandler(GasCityConfig{CityDir: t.TempDir()})
	handler.runner = runner
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/gascity/sessions/planner/transcript?lines=20", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for alias/raw-session target; body: %s", rec.Code, rec.Body.String())
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want no gc invocation for alias target", runner.calls)
	}
}

// TestGasCityHandlerTranscriptRecoversArchivedPeekAfterSupervisorRestart is the
// core home-5ubb guarantee: once a live peek has been captured, an operator can
// still retrieve that session transcript after the supervisor is gone, beyond a
// single bounded live peek. Without the archive, a restart-volatile tmux pane
// leaves CHROTE with no transcript at all.
func TestGasCityHandlerTranscriptRecoversArchivedPeekAfterSupervisorRestart(t *testing.T) {
	cityDir := t.TempDir()
	archiveDir := t.TempDir()
	upstream := newTestGasCitySupervisor(t, cityDir, []map[string]any{
		{
			"id":       "gc-4171",
			"alias":    "planner",
			"template": "planner",
			"state":    "active",
			"running":  true,
			"provider": "./bin/mock-agent",
		},
	})
	runner := &fakeGasCityRunner{output: "planner ready\nDURABLE_MARKER_7\n"}
	handler := NewGasCityHandler(GasCityConfig{BaseURL: upstream.URL, CityDir: cityDir, TranscriptArchiveDir: archiveDir})
	handler.runner = runner
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1) Live peek while the supervisor is up: fresh source, archived to disk.
	liveRec := httptest.NewRecorder()
	mux.ServeHTTP(liveRec, httptest.NewRequest(http.MethodGet, "/api/gascity/sessions/gc-4171/transcript?lines=40", nil))
	if liveRec.Code != http.StatusOK {
		t.Fatalf("live status = %d, want 200; body: %s", liveRec.Code, liveRec.Body.String())
	}
	var liveResp struct {
		Data GasCityTranscriptResponse `json:"data"`
	}
	if err := json.Unmarshal(liveRec.Body.Bytes(), &liveResp); err != nil {
		t.Fatalf("decode live transcript: %v", err)
	}
	if liveResp.Data.Source != "gc-session-peek" || liveResp.Data.Stale {
		t.Fatalf("live response = %+v, want fresh non-stale gc-session-peek", liveResp.Data)
	}
	if !strings.Contains(liveResp.Data.Transcript, "DURABLE_MARKER_7") {
		t.Fatalf("live transcript = %q, want captured marker", liveResp.Data.Transcript)
	}

	// 2) Supervisor restart: it can no longer resolve the session.
	upstream.Close()

	recRec := httptest.NewRecorder()
	mux.ServeHTTP(recRec, httptest.NewRequest(http.MethodGet, "/api/gascity/sessions/gc-4171/transcript?lines=40", nil))
	if recRec.Code != http.StatusOK {
		t.Fatalf("recovery status = %d, want 200 from archive; body: %s", recRec.Code, recRec.Body.String())
	}
	var recResp struct {
		Data GasCityTranscriptResponse `json:"data"`
	}
	if err := json.Unmarshal(recRec.Body.Bytes(), &recResp); err != nil {
		t.Fatalf("decode recovery transcript: %v", err)
	}
	if recResp.Data.Source != "chrote-archive" || !recResp.Data.Stale {
		t.Fatalf("recovery response = %+v, want stale chrote-archive source", recResp.Data)
	}
	if !strings.Contains(recResp.Data.Transcript, "DURABLE_MARKER_7") {
		t.Fatalf("recovery transcript = %q, want archived marker after restart", recResp.Data.Transcript)
	}
	if recResp.Data.CapturedAt == "" {
		t.Fatal("recovery response should report when the archived peek was captured")
	}
	if recResp.Data.SessionID != "gc-4171" {
		t.Fatalf("recovery session id = %q, want gc-4171", recResp.Data.SessionID)
	}
}

// TestGasCityHandlerTranscriptRecoversArchiveWhenPeekReturnsEmptyPane covers the
// post-restart case where the supervisor is back but the tmux pane was recreated
// empty: a misleading blank live peek should fall back to the archived snapshot.
func TestGasCityHandlerTranscriptRecoversArchiveWhenPeekReturnsEmptyPane(t *testing.T) {
	cityDir := t.TempDir()
	archiveDir := t.TempDir()
	upstream := newTestGasCitySupervisor(t, cityDir, []map[string]any{
		{"id": "gc-4171", "alias": "planner", "template": "planner", "state": "active", "running": true},
	})
	defer upstream.Close()
	runner := &scriptedGasCityRunner{results: []struct {
		output string
		err    error
	}{
		{output: "planner ready\nDURABLE_MARKER_7\n"}, // first live peek populates archive
		{output: "   \n"}, // post-restart empty pane
	}}
	handler := NewGasCityHandler(GasCityConfig{BaseURL: upstream.URL, CityDir: cityDir, TranscriptArchiveDir: archiveDir})
	handler.runner = runner
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	first := httptest.NewRecorder()
	mux.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/gascity/sessions/gc-4171/transcript", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200; body: %s", first.Code, first.Body.String())
	}

	second := httptest.NewRecorder()
	mux.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/api/gascity/sessions/gc-4171/transcript", nil))
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d, want 200; body: %s", second.Code, second.Body.String())
	}
	var resp struct {
		Data GasCityTranscriptResponse `json:"data"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode second transcript: %v", err)
	}
	if resp.Data.Source != "chrote-archive" || !resp.Data.Stale {
		t.Fatalf("empty-pane response = %+v, want stale archive fallback", resp.Data)
	}
	if !strings.Contains(resp.Data.Transcript, "DURABLE_MARKER_7") {
		t.Fatalf("empty-pane transcript = %q, want archived content not blank pane", resp.Data.Transcript)
	}
}

// TestGasCityHandlerTranscriptWithoutArchivePreservesErrorAfterRestart proves the
// recovery is additive: with archiving disabled (or nothing captured yet), an
// unresolvable session still returns the original not-found error, not a 200.
func TestGasCityHandlerTranscriptWithoutArchivePreservesErrorAfterRestart(t *testing.T) {
	cityDir := t.TempDir()
	upstream := newTestGasCitySupervisor(t, cityDir, []map[string]any{}) // no sessions
	defer upstream.Close()
	runner := &fakeGasCityRunner{}
	handler := NewGasCityHandler(GasCityConfig{BaseURL: upstream.URL, CityDir: cityDir, TranscriptArchiveDir: ""})
	handler.runner = runner
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/gascity/sessions/gc-4171/transcript", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when no archive snapshot exists; body: %s", rec.Code, rec.Body.String())
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want no peek for unresolvable session", runner.calls)
	}
}

func TestGasCityTranscriptArchiveSaveLoadAndDisabled(t *testing.T) {
	// Disabled archive is inert: save is a no-op, load misses.
	disabled := newGasCityTranscriptArchive("")
	if err := disabled.save(gasCityTranscriptSnapshot{SessionID: "gc-1", Transcript: "x"}); err != nil {
		t.Fatalf("disabled save error = %v, want nil no-op", err)
	}
	if _, ok := disabled.load("gc-1"); ok {
		t.Fatal("disabled archive should never load a snapshot")
	}

	dir := t.TempDir()
	archive := newGasCityTranscriptArchive(dir)
	if err := archive.save(gasCityTranscriptSnapshot{SessionID: "gc-9", Alias: "planner", Transcript: "captured output", LineCount: 1}); err != nil {
		t.Fatalf("save error = %v", err)
	}
	got, ok := archive.load("gc-9")
	if !ok {
		t.Fatal("expected to load saved snapshot")
	}
	if got.Transcript != "captured output" || got.Alias != "planner" || got.CapturedAt == "" {
		t.Fatalf("loaded snapshot = %+v, want captured fields and timestamp", got)
	}

	// File is created private to the user.
	info, err := os.Stat(filepath.Join(dir, "gc-9.json"))
	if err != nil {
		t.Fatalf("stat snapshot: %v", err)
	}
	if info.Mode().Perm() != gasCityArchiveFilePerm {
		t.Fatalf("snapshot perm = %v, want %v", info.Mode().Perm(), gasCityArchiveFilePerm)
	}

	if _, ok := archive.load("gc-missing"); ok {
		t.Fatal("missing session should not load")
	}
}

func TestGasCityTranscriptArchiveEvictsOldestPastCap(t *testing.T) {
	dir := t.TempDir()
	archive := newGasCityTranscriptArchive(dir)
	base := time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC)
	total := gasCityArchiveMaxSessions + 5
	for i := 0; i < total; i++ {
		i := i
		archive.now = func() time.Time { return base.Add(time.Duration(i) * time.Minute) }
		id := "gc-" + strconv.Itoa(i)
		if err := archive.save(gasCityTranscriptSnapshot{SessionID: id, Transcript: "t"}); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
		// Stagger file mtimes so eviction order is deterministic.
		stamp := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(filepath.Join(dir, id+".json"), stamp, stamp); err != nil {
			t.Fatalf("chtimes %s: %v", id, err)
		}
		archive.evictLocked()
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	jsonCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			jsonCount++
		}
	}
	if jsonCount != gasCityArchiveMaxSessions {
		t.Fatalf("archived snapshots = %d, want cap %d", jsonCount, gasCityArchiveMaxSessions)
	}
	// Oldest (gc-0) evicted; newest (last) retained.
	if _, ok := archive.load("gc-0"); ok {
		t.Fatal("oldest snapshot gc-0 should have been evicted")
	}
	if _, ok := archive.load("gc-" + strconv.Itoa(total-1)); !ok {
		t.Fatal("newest snapshot should be retained")
	}
}

func newTestGasCitySupervisor(t *testing.T, cityDir string, sessions []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.RequestURI() {
		case "/v0/cities":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"items": []map[string]any{
					{"name": "gascity", "path": cityDir, "running": true},
				},
				"total": 1,
			})
		case "/v0/city/gascity/sessions?limit=100&peek=false&state=active":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"items": sessions,
				"total": len(sessions),
			})
		case "/v0/city/gascity/sessions?limit=100&peek=false&state=all":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"items": sessions,
				"total": len(sessions),
			})
		default:
			t.Fatalf("unexpected upstream request %s", r.URL.RequestURI())
		}
	}))
}

type fakeGasCityRunner struct {
	calls  int
	name   string
	args   []string
	output string
	err    error
}

func (r *fakeGasCityRunner) Run(_ context.Context, name string, args []string) (string, error) {
	r.calls++
	r.name = name
	r.args = append([]string(nil), args...)
	return r.output, r.err
}

// scriptedGasCityRunner returns a different (output, err) per call so a test can
// model a live peek followed by a post-restart peek failure or empty pane.
type scriptedGasCityRunner struct {
	calls   int
	names   []string
	args    [][]string
	results []struct {
		output string
		err    error
	}
}

func (r *scriptedGasCityRunner) Run(_ context.Context, name string, args []string) (string, error) {
	idx := r.calls
	r.calls++
	r.names = append(r.names, name)
	r.args = append(r.args, append([]string(nil), args...))
	if idx >= len(r.results) {
		idx = len(r.results) - 1
	}
	return r.results[idx].output, r.results[idx].err
}
