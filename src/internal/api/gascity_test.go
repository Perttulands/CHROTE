package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
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
	if cfg.CityDir != defaultGasCityCityDir || cfg.PoemTarget != defaultGasCityPoemTarget || cfg.PoemTemplate != defaultGasCityPoemTemplate || cfg.MailRecipient != defaultGasCityMailRecipient {
		t.Fatalf("control defaults = %+v, want local city, Pi target, and human recipient", cfg)
	}
}

func TestGasCityHandlerObserverFetchesReadOnlySummary(t *testing.T) {
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
		case "/v0/city/testcity/status":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"name":       "testcity",
				"running":    2,
				"uptime_sec": 99,
				"work":       map[string]any{"open": 7, "ready": 3, "in_progress": 1},
				"mail":       map[string]any{"total": 5, "unread": 2},
			})
		case "/v0/city/testcity/sessions?limit=50&peek=false&state=active":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"items": []map[string]any{
					{
						"id":           "gc-1",
						"title":        "planner",
						"alias":        "planner",
						"template":     "planner",
						"state":        "active",
						"provider":     "./bin/mock-agent",
						"session_name": "planner",
						"created_at":   "2026-05-26T10:00:00Z",
						"last_active":  "2026-05-26T10:01:00Z",
						"running":      true,
					},
				},
				"total": 1,
			})
		case "/v0/city/testcity/mail/count":
			writeTestJSON(w, http.StatusOK, map[string]any{"total": 5, "unread": 2})
		case "/v0/city/testcity/formulas?scope_kind=city&scope_ref=testcity":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"items": []map[string]any{
					{"name": "plan-review-synthesis", "description": "Plan and review.", "version": "1", "run_count": 2},
				},
				"total": 1,
			})
		case "/v0/city/testcity/convoys":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"items": []map[string]any{
					{"id": "gc-30", "title": "sling-gc-29", "status": "open", "issue_type": "convoy"},
				},
				"total": 1,
			})
		case "/v0/city/testcity/beads?limit=50":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"items": []map[string]any{
					{"id": "gc-29", "title": "Routed task", "status": "open", "issue_type": "task", "metadata": map[string]string{"gc.routed_to": "planner"}},
					{"id": "gc-31", "title": "Review molecule", "status": "open", "issue_type": "molecule", "ref": "plan-review-synthesis"},
					{"id": "gc-32", "title": "Temporary workflow", "status": "open", "issue_type": "wisp", "ref": "mol-review-quorum"},
				},
				"total": 3,
			})
		case "/v0/events?limit=20":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"items": []map[string]any{
					{"seq": 101, "type": "session.woke", "ts": "2026-05-26T10:02:00Z", "actor": "controller", "subject": "planner", "city": "testcity"},
				},
				"total": 101,
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
	if data.Health.Status != "ok" || !data.Health.StartupReady {
		t.Fatalf("health = %+v, want ok startup-ready health", data.Health)
	}
	if len(data.Cities) != 1 || data.Cities[0].Name != "testcity" || !data.Cities[0].Running {
		t.Fatalf("cities = %+v, want running testcity", data.Cities)
	}
	if len(data.Sessions) != 1 || data.Sessions[0].ID != "gc-1" || !data.Sessions[0].Running {
		t.Fatalf("sessions = %+v, want running gc-1 session", data.Sessions)
	}
	if data.Mail.Total != 5 || data.Mail.Unread != 2 {
		t.Fatalf("mail = %+v, want total=5 unread=2", data.Mail)
	}
	if data.Work.Open != 7 || data.Work.Ready != 3 || data.Work.InProgress != 1 || data.Work.Routed != 1 {
		t.Fatalf("work = %+v, want status and routed counts", data.Work)
	}
	if len(data.Formulas) != 1 || data.Formulas[0].Name != "plan-review-synthesis" {
		t.Fatalf("formulas = %+v, want plan-review-synthesis", data.Formulas)
	}
	if len(data.Molecules) != 1 || data.Molecules[0].ID != "gc-31" {
		t.Fatalf("molecules = %+v, want gc-31", data.Molecules)
	}
	if len(data.Wisps) != 1 || data.Wisps[0].ID != "gc-32" {
		t.Fatalf("wisps = %+v, want gc-32", data.Wisps)
	}
	if len(data.Convoys) != 1 || data.Convoys[0].ID != "gc-30" {
		t.Fatalf("convoys = %+v, want gc-30", data.Convoys)
	}
	if len(data.RecentEvents) != 1 || data.RecentEvents[0].Type != "session.woke" {
		t.Fatalf("recent events = %+v, want session.woke event", data.RecentEvents)
	}

	sort.Strings(seen)
	for _, request := range seen {
		if strings.Contains(request, "/wake") || strings.Contains(request, "/sling") || strings.Contains(request, "/close") {
			t.Fatalf("observer made mutating-looking upstream request: %s", request)
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
	if len(response.Data.Sessions) != 0 || response.Data.Mail.Total != 0 || len(response.Data.RecentEvents) != 0 {
		t.Fatalf("unavailable observer should return empty read model, got %+v", response.Data)
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

func TestGasCityHandlerObserverDegradesWhenOptionalRoutesFail(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.RequestURI() {
		case "/health":
			writeTestJSON(w, http.StatusOK, map[string]any{"status": "ok"})
		case "/v0/cities":
			writeTestJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{{"name": "testcity", "running": true}}})
		case "/v0/city/testcity/status":
			writeTestJSON(w, http.StatusOK, map[string]any{"name": "testcity", "work": map[string]any{"open": 1}})
		case "/v0/city/testcity/sessions?limit=50&peek=false&state=active":
			writeTestJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{}})
		case "/v0/city/testcity/mail/count":
			writeTestJSON(w, http.StatusOK, map[string]any{"total": 0, "unread": 0})
		case "/v0/city/testcity/formulas?scope_kind=city&scope_ref=testcity":
			writeTestJSON(w, http.StatusBadRequest, map[string]any{"detail": "scope unavailable"})
		case "/v0/city/testcity/convoys":
			writeTestJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{}})
		case "/v0/city/testcity/beads?limit=50":
			writeTestJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{}})
		case "/v0/events?limit=20":
			writeTestJSON(w, http.StatusServiceUnavailable, map[string]any{"detail": "events unavailable"})
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
	if response.Data.Work.Open != 1 {
		t.Fatalf("work = %+v, want status data preserved despite optional failures", response.Data.Work)
	}
	if len(response.Data.Formulas) != 0 || len(response.Data.RecentEvents) != 0 {
		t.Fatalf("failed optional routes should return empty collections, got formulas=%+v events=%+v", response.Data.Formulas, response.Data.RecentEvents)
	}
	if len(response.Data.UpstreamErrors) != 2 {
		t.Fatalf("upstream errors = %+v, want formulas and events errors", response.Data.UpstreamErrors)
	}
}

func TestGasCityHandlerMailListsHumanMessagesFromStore(t *testing.T) {
	cityDir := t.TempDir()
	longBody := strings.Repeat("a", gasCityMailBodyLimit+10)
	writeTestGasCityBeads(t, cityDir, map[string]any{
		"beads": []map[string]any{
			{
				"id":          "gc-task",
				"issue_type":  "task",
				"title":       "not mail",
				"description": "should not appear",
				"assignee":    "human",
				"created_at":  "2026-05-27T09:00:00Z",
			},
			{
				"id":          "gc-older",
				"issue_type":  "message",
				"title":       "Older reply",
				"description": "first body",
				"from":        "chrote-poem-pi",
				"assignee":    "human",
				"created_at":  "2026-05-27T09:01:00Z",
				"metadata": map[string]any{
					"mail.from_session_id": "gc-51923",
					"mail.read":            "true",
				},
			},
			{
				"id":          "gc-other",
				"issue_type":  "message",
				"title":       "Wrong recipient",
				"description": "not human",
				"from":        "planner",
				"assignee":    "planner",
				"created_at":  "2026-05-27T09:02:00Z",
			},
			{
				"id":          "gc-newer",
				"issue_type":  "message",
				"title":       "Newer reply",
				"description": longBody,
				"from":        "chrote-poem-pi",
				"assignee":    "human",
				"created_at":  "2026-05-27T09:03:00Z",
				"metadata": map[string]any{
					"mail.from_display":    "chrote-poem-pi",
					"mail.from_session_id": "gc-51923",
				},
			},
		},
		"deps": []any{},
		"seq":  4,
	})

	handler := NewGasCityHandler(GasCityConfig{CityDir: cityDir})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/gascity/mail?recipient=human&limit=20", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Success bool                    `json:"success"`
		Data    GasCityMailListResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode mail response: %v", err)
	}
	if !response.Success {
		t.Fatal("mail response should use success envelope")
	}
	if response.Data.Recipient != "human" {
		t.Fatalf("recipient = %q, want human", response.Data.Recipient)
	}
	if len(response.Data.Messages) != 2 {
		t.Fatalf("messages = %+v, want two human message beads", response.Data.Messages)
	}
	newest := response.Data.Messages[0]
	if newest.ID != "gc-newer" || newest.Subject != "Newer reply" || newest.From != "chrote-poem-pi" {
		t.Fatalf("newest = %+v, want newest human mail first", newest)
	}
	if !newest.BodyTruncated || len(newest.Body) != gasCityMailBodyLimit {
		t.Fatalf("newest body length/truncated = %d/%v, want limited body", len(newest.Body), newest.BodyTruncated)
	}
	older := response.Data.Messages[1]
	if older.ID != "gc-older" || !older.Read || older.FromSessionID != "gc-51923" {
		t.Fatalf("older = %+v, want read message with session metadata", older)
	}
}

func TestGasCityHandlerMailReadsCurrentSizedStore(t *testing.T) {
	cityDir := t.TempDir()
	writeTestGasCityBeads(t, cityDir, map[string]any{
		"beads": []map[string]any{
			{
				"id":          "gc-large-task",
				"issue_type":  "task",
				"title":       "large non-mail history",
				"description": strings.Repeat("x", 21<<20),
				"assignee":    "planner",
				"created_at":  "2026-05-27T09:00:00Z",
			},
			{
				"id":          "gc-live-mail",
				"issue_type":  "message",
				"title":       "Latest reply",
				"description": "current live-sized store still parses",
				"from":        "chrote-poem-pi",
				"assignee":    "human",
				"created_at":  "2026-05-27T09:03:00Z",
			},
		},
		"deps": []any{},
		"seq":  2,
	})

	handler := NewGasCityHandler(GasCityConfig{CityDir: cityDir})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/gascity/mail?recipient=human&limit=20", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for live-sized store; body: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data GasCityMailListResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode mail response: %v", err)
	}
	if len(response.Data.Messages) != 1 || response.Data.Messages[0].ID != "gc-live-mail" {
		t.Fatalf("messages = %+v, want live-sized store mail", response.Data.Messages)
	}
}

func TestGasCityHandlerPiPoemRejectsUnsafeTopic(t *testing.T) {
	runner := &fakeGasCityRunner{}
	handler := NewGasCityHandler(GasCityConfig{CityDir: t.TempDir()})
	handler.runner = runner
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/gascity/requests/pi-poem", strings.NewReader(`{"topic":"mail; rm -rf /"}`)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want no Gas City mutation for unsafe topic", runner.calls)
	}
}

func TestGasCityHandlerPiPoemRejectsRawCommandFields(t *testing.T) {
	runner := &fakeGasCityRunner{}
	handler := NewGasCityHandler(GasCityConfig{CityDir: t.TempDir()})
	handler.runner = runner
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/gascity/requests/pi-poem", strings.NewReader(`{"topic":"mail routes","command":"gc session stop gc-1"}`)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want no Gas City mutation when browser sends raw command text", runner.calls)
	}
}

func TestGasCityHandlerPiPoemNudgesConfiguredTargetWithBoundedCommand(t *testing.T) {
	runner := &fakeGasCityRunner{output: "Nudged chrote-poem-pi\n"}
	cityDir := t.TempDir()
	upstream := newTestGasCitySupervisor(t, cityDir, []map[string]any{
		{
			"id":           "gc-51923",
			"alias":        "chrote-poem-pi",
			"template":     "pi-smoke",
			"state":        "active",
			"running":      true,
			"session_name": "s-gc-51923",
			"title":        "pi-smoke",
		},
	})
	defer upstream.Close()
	handler := NewGasCityHandler(GasCityConfig{BaseURL: upstream.URL, CityDir: cityDir})
	handler.runner = runner
	handler.nonce = func() string { return "C4A-TEST-NONCE" }
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/gascity/requests/pi-poem", strings.NewReader(`{"topic":"mail routes"}`)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want one bounded gc invocation", runner.calls)
	}
	if runner.name != "gc" {
		t.Fatalf("runner name = %q, want gc", runner.name)
	}
	wantPrefix := []string{"--city", cityDir, "session", "nudge", "gc-51923", "--delivery", "immediate"}
	if len(runner.args) != len(wantPrefix)+1 {
		t.Fatalf("args = %#v, want prefix plus one bounded shell command", runner.args)
	}
	for i, want := range wantPrefix {
		if runner.args[i] != want {
			t.Fatalf("args[%d] = %q, want %q; args=%#v", i, runner.args[i], want, runner.args)
		}
	}
	command := runner.args[len(runner.args)-1]
	for _, required := range []string{
		"! set -euo pipefail;",
		"pi --no-tools --no-context-files --no-extensions --no-skills --no-prompt-templates --no-session --mode text --print",
		"mail routes",
		"C4A-TEST-NONCE",
		"gc mail send 'human'",
		"CHROTE Pi poem C4A-TEST-NONCE",
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("bounded command missing %q: %s", required, command)
		}
	}
	for _, forbidden := range []string{"gc session stop", "gc --city", "CHROTE_GASCITY", "CONTEXT_API_TOKEN"} {
		if strings.Contains(command, forbidden) {
			t.Fatalf("bounded command exposed forbidden text %q: %s", forbidden, command)
		}
	}

	var response struct {
		Success bool                  `json:"success"`
		Data    GasCityPiPoemResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode poem response: %v", err)
	}
	if !response.Success {
		t.Fatal("poem response should use success envelope")
	}
	if response.Data.Nonce != "C4A-TEST-NONCE" || response.Data.Target != "gc-51923" || response.Data.TargetAlias != "chrote-poem-pi" || response.Data.TargetTemplate != "pi-smoke" || response.Data.TargetSessionID != "gc-51923" || response.Data.Recipient != "human" {
		t.Fatalf("poem response = %+v, want fixed nonce/resolved target/recipient", response.Data)
	}
	if response.Data.Output != "Nudged chrote-poem-pi" {
		t.Fatalf("output = %q, want sanitized runner output", response.Data.Output)
	}
	if len(handler.audit) != 1 || handler.audit[0].Nonce != "C4A-TEST-NONCE" || handler.audit[0].TargetSessionID != "gc-51923" || !handler.audit[0].Success {
		t.Fatalf("audit = %+v, want one successful bounded request entry", handler.audit)
	}

	auditRec := httptest.NewRecorder()
	mux.ServeHTTP(auditRec, httptest.NewRequest(http.MethodGet, "/api/gascity/audit", nil))
	if auditRec.Code != http.StatusOK {
		t.Fatalf("audit status = %d, want 200; body: %s", auditRec.Code, auditRec.Body.String())
	}
	var auditResponse struct {
		Data GasCityAuditResponse `json:"data"`
	}
	if err := json.Unmarshal(auditRec.Body.Bytes(), &auditResponse); err != nil {
		t.Fatalf("decode audit response: %v", err)
	}
	if len(auditResponse.Data.Entries) != 1 || auditResponse.Data.Entries[0].TargetSessionID != "gc-51923" {
		t.Fatalf("audit response = %+v, want inspectable resolved target audit", auditResponse.Data)
	}
}

func TestGasCityHandlerPiPoemRejectsUnexpectedTargetTemplate(t *testing.T) {
	runner := &fakeGasCityRunner{}
	cityDir := t.TempDir()
	upstream := newTestGasCitySupervisor(t, cityDir, []map[string]any{
		{
			"id":       "gc-51923",
			"alias":    "chrote-poem-pi",
			"template": "planner",
			"state":    "active",
			"running":  true,
		},
	})
	defer upstream.Close()
	handler := NewGasCityHandler(GasCityConfig{BaseURL: upstream.URL, CityDir: cityDir})
	handler.runner = runner
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/gascity/requests/pi-poem", strings.NewReader(`{"topic":"mail routes"}`)))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body: %s", rec.Code, rec.Body.String())
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want no Gas City mutation for unexpected target template", runner.calls)
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
		{output: "   \n"},                              // post-restart empty pane
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

func writeTestGasCityBeads(t *testing.T, cityDir string, store map[string]any) {
	t.Helper()
	gcDir := filepath.Join(cityDir, ".gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatalf("mkdir .gc: %v", err)
	}
	body, err := json.Marshal(store)
	if err != nil {
		t.Fatalf("marshal store fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gcDir, "beads.json"), body, 0o644); err != nil {
		t.Fatalf("write store fixture: %v", err)
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
	results []struct {
		output string
		err    error
	}
}

func (r *scriptedGasCityRunner) Run(_ context.Context, _ string, _ []string) (string, error) {
	idx := r.calls
	r.calls++
	if idx >= len(r.results) {
		idx = len(r.results) - 1
	}
	return r.results[idx].output, r.results[idx].err
}
