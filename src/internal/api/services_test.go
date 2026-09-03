package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newServicesHandlerWithClient(config ServiceConfig, client *http.Client) *ServicesHandler {
	handler := NewServicesHandler(config)
	if client == nil {
		return handler
	}
	handler.client = client
	streamClient := *client
	streamClient.Timeout = 0
	handler.streamClient = &streamClient
	return handler
}

// Unset service settings fall back to the local defaults, and set ones are taken
// with their surrounding whitespace and trailing slash removed, because an
// operator pasting a URL into a unit file brings both along.
func TestLoadServiceConfigFromEnv(t *testing.T) {
	token := strings.Join([]string{"fixture", "private", "value"}, "-")

	for _, testCase := range []struct {
		name           string
		ttsURL         string
		contextURL     string
		contextToken   string
		wantTTS        string
		wantContext    string
		wantTokenValue string
	}{
		{
			name:        "unset settings fall back to the local services",
			wantTTS:     "http://127.0.0.1:3100",
			wantContext: "http://127.0.0.1:3200",
		},
		{
			name:           "set settings are trimmed of space and a trailing slash",
			ttsURL:         " http://tts.internal:3100/ ",
			contextURL:     " http://context.internal:3200/ ",
			contextToken:   token,
			wantTTS:        "http://tts.internal:3100",
			wantContext:    "http://context.internal:3200",
			wantTokenValue: token,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("CHROTE_TTS_URL", testCase.ttsURL)
			t.Setenv("CHROTE_CONTEXT_API_URL", testCase.contextURL)
			t.Setenv("CHROTE_CONTEXT_API_TOKEN", testCase.contextToken)

			cfg := LoadServiceConfigFromEnv()

			if cfg.TTSBaseURL != testCase.wantTTS {
				t.Fatalf("TTSBaseURL = %q, want %q", cfg.TTSBaseURL, testCase.wantTTS)
			}
			if cfg.ContextBaseURL != testCase.wantContext {
				t.Fatalf("ContextBaseURL = %q, want %q", cfg.ContextBaseURL, testCase.wantContext)
			}
			if cfg.ContextToken != testCase.wantTokenValue {
				t.Fatal("ContextToken did not match the configured value")
			}
		})
	}
}

// The catalogue says whether a token is configured and never what it is. A
// missing one degrades the service with words the operator can act on, rather
// than leaving a service that looks healthy and fails on first use.
func TestServicesHandlerCatalogReportsTheContextTokenWithoutExposingIt(t *testing.T) {
	token := strings.Join([]string{"fixture", "private", "value"}, "-")

	for _, testCase := range []struct {
		name          string
		token         string
		wantConfigure bool
		wantStatus    string
		wantMessage   string
	}{
		{
			name:          "a configured token is reported present and healthy",
			token:         token,
			wantConfigure: true,
		},
		{
			name:        "a missing token degrades the service and says why",
			wantStatus:  "degraded",
			wantMessage: "CHROTE_CONTEXT_API_TOKEN is not configured; Context Citadel document and integration operations are disabled.",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			handler := NewServicesHandler(ServiceConfig{
				TTSBaseURL:     "http://127.0.0.1:3100",
				ContextBaseURL: "http://127.0.0.1:3200",
				ContextToken:   testCase.token,
			})

			req := httptest.NewRequest(http.MethodGet, "/api/services", nil)
			rec := httptest.NewRecorder()
			handler.Catalog(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if strings.Contains(rec.Body.String(), token) {
				t.Fatal("catalog response exposed the context token")
			}

			var response struct {
				Success bool `json:"success"`
				Data    struct {
					Services []ServiceStatus `json:"services"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode catalog response: %v", err)
			}
			if !response.Success {
				t.Fatal("catalog response should use success envelope")
			}

			context := findServiceStatus(t, response.Data.Services, "context")
			if context.TokenConfigured != testCase.wantConfigure {
				t.Fatalf("tokenConfigured = %v, want %v", context.TokenConfigured, testCase.wantConfigure)
			}
			if testCase.wantStatus == "" {
				if context.Status == "degraded" {
					t.Fatal("context service should not be degraded when token is configured")
				}
				return
			}
			if context.Status != testCase.wantStatus {
				t.Fatalf("context status = %q, want %q", context.Status, testCase.wantStatus)
			}
			if context.Message != testCase.wantMessage {
				t.Fatalf("context message = %q, want %q", context.Message, testCase.wantMessage)
			}
		})
	}
}

func findServiceStatus(t *testing.T, services []ServiceStatus, id string) ServiceStatus {
	t.Helper()
	for _, service := range services {
		if service.ID == id {
			return service
		}
	}
	t.Fatalf("missing service status for %q", id)
	return ServiceStatus{}
}

func TestServicesHandlerTTSProxyForwardsJSONRequests(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/health":
			writeTestJSON(w, http.StatusOK, map[string]any{"status": "ok", "messages": 2})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/tts/messages":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"messages": []map[string]any{
					{"id": "abc123", "status": "ready", "text": "hello from chrote"},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/tts/enqueue":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "hello from chrote") {
				t.Fatalf("enqueue body was not forwarded: %s", string(body))
			}
			writeTestJSON(w, http.StatusAccepted, map[string]any{"id": "abc123", "status": "queued"})
		default:
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := NewServicesHandler(ServiceConfig{TTSBaseURL: upstream.URL})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/services/tts/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", rec.Code)
	}
	assertEnvelopeData(t, rec.Body.String(), "messages")

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/services/tts/messages", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("messages status = %d, want 200", rec.Code)
	}
	assertEnvelopeData(t, rec.Body.String(), "abc123")

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/services/tts/enqueue", strings.NewReader(`{"text":"hello from chrote"}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("enqueue status = %d, want 202", rec.Code)
	}
	assertEnvelopeData(t, rec.Body.String(), "abc123")
}

func TestServicesHandlerTTSAudioProxyStreamsRawAudio(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/tts/audio/abc123" {
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("mp3-bytes"))
	}))
	defer upstream.Close()

	handler := NewServicesHandler(ServiceConfig{TTSBaseURL: upstream.URL})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/services/tts/audio/abc123", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("audio status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "audio/mpeg" {
		t.Fatalf("audio content-type = %q, want audio/mpeg", rec.Header().Get("Content-Type"))
	}
	if rec.Body.String() != "mp3-bytes" {
		t.Fatalf("audio body = %q, want raw upstream bytes", rec.Body.String())
	}
}

func TestServicesHandlerTTSFeedFlushesAndClearsWriteDeadline(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/tts/feed" {
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: first\ndata: {}\n\n"))
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("event: second\ndata: {}\n\n"))
	}))
	defer upstream.Close()

	handler := NewServicesHandler(ServiceConfig{TTSBaseURL: upstream.URL})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := &streamingResponseRecorder{header: make(http.Header)}
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/services/tts/feed", nil))

	if rec.status != http.StatusOK {
		t.Fatalf("feed status = %d, want 200", rec.status)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("feed content-type = %q, want text/event-stream", got)
	}
	if !strings.Contains(rec.body.String(), "event: second") {
		t.Fatalf("feed body did not preserve upstream stream: %s", rec.body.String())
	}
	if rec.flushes == 0 {
		t.Fatal("feed stream did not flush after upstream chunks")
	}
	if !rec.writeDeadlineSet {
		t.Fatal("feed stream did not clear the handler write deadline")
	}
	if !rec.writeDeadline.IsZero() {
		t.Fatalf("feed write deadline = %v, want zero time", rec.writeDeadline)
	}
}

func TestServicesHandlerTTSFeedIgnoresFiniteClientTimeout(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/tts/feed" {
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: first\ndata: {}\n\n"))
		w.(http.Flusher).Flush()

		time.Sleep(40 * time.Millisecond)
		_, _ = w.Write([]byte("event: second\ndata: {}\n\n"))
	}))
	defer upstream.Close()

	handler := newServicesHandlerWithClient(
		ServiceConfig{TTSBaseURL: upstream.URL},
		&http.Client{Timeout: 10 * time.Millisecond},
	)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/services/tts/feed", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("feed status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "event: second") {
		t.Fatalf("feed ended before delayed event, body: %s", rec.Body.String())
	}
}

func TestServicesHandlerContextProxyInjectsServerTokenOnly(t *testing.T) {
	token := strings.Join([]string{"fixture", "owner", "value"}, "-")
	var upstreamAuth string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodGet || r.URL.Path != "/v1/context" {
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.Path)
		}
		writeTestJSON(w, http.StatusOK, map[string]any{
			"docs":          []string{"profile.md"},
			"authorization": upstreamAuth,
		})
	}))
	defer upstream.Close()

	handler := NewServicesHandler(ServiceConfig{
		ContextBaseURL: upstream.URL,
		ContextToken:   token,
	})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/services/context/docs", nil)
	req.Header.Set("Authorization", "Bearer browser-supplied-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if upstreamAuth != "Bearer "+token {
		t.Fatalf("upstream Authorization = %q, want server token", upstreamAuth)
	}
	if strings.Contains(rec.Body.String(), token) || strings.Contains(rec.Body.String(), "browser-supplied-token") {
		t.Fatal("context proxy response exposed a bearer token")
	}
	assertEnvelopeData(t, rec.Body.String(), "profile.md")
}

// Every Context Citadel route CHROTE proxies reaches upstream under the server's
// own token, with the browser's Authorization header discarded, and neither
// token comes back in the response. Read, save, history, ask, grants, ingestion
// and audit share one table because they share one proxy: a route added without
// the token injection is the whole failure this pins.
func TestServicesHandlerContextProxyForwardsEveryRouteUnderTheServerToken(t *testing.T) {
	token := strings.Join([]string{"fixture", "owner", "value"}, "-")
	rawGrantToken := "ctx_live_fixturehandle_fixturesecretvalue"
	var seen []string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Fatalf("upstream Authorization = %q, want server token", r.Header.Get("Authorization"))
		}
		seen = append(seen, r.Method+" "+r.URL.RequestURI())

		switch {
		case r.Method == http.MethodGet && r.URL.RequestURI() == "/v1/context/identity/communication.md":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"path":    "identity/communication.md",
				"content": "# Communication\n",
			})
		case r.Method == http.MethodPut && r.URL.RequestURI() == "/v1/context/identity/communication.md":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "updated content") {
				t.Fatalf("save body was not forwarded: %s", string(body))
			}
			writeTestJSON(w, http.StatusOK, map[string]any{"ok": true, "path": "identity/communication.md"})
		case r.Method == http.MethodGet && r.URL.RequestURI() == "/v1/context/identity/communication.md/history":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"path": "identity/communication.md",
				"history": []map[string]any{
					{"hash": "abc123", "message": "PUT identity/communication.md"},
				},
			})
		case r.Method == http.MethodPost && r.URL.RequestURI() == "/v1/ask":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "How should agents speak?") {
				t.Fatalf("ask body was not forwarded: %s", string(body))
			}
			writeTestJSON(w, http.StatusOK, map[string]any{
				"answer": "Use short status updates.",
				"sources": []map[string]any{
					{"path": "identity/communication.md", "snippet": "Keep it brief."},
				},
			})
		case r.Method == http.MethodGet && r.URL.RequestURI() == "/v1/grants":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"grants": []map[string]any{{"id": "grant_1", "name": "ChatGPT", "status": "active"}},
			})
		case r.Method == http.MethodPost && r.URL.RequestURI() == "/v1/grants":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"token": rawGrantToken,
				"grant": map[string]any{"id": "grant_1", "name": "ChatGPT", "status": "active"},
			})
		case r.Method == http.MethodPost && r.URL.RequestURI() == "/v1/grants/grant_1/revoke":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"grant": map[string]any{"id": "grant_1", "status": "revoked"},
			})
		case r.Method == http.MethodPost && r.URL.RequestURI() == "/v1/grants/grant_1/rotate":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"token": rawGrantToken,
				"grant": map[string]any{"id": "grant_1", "status": "active"},
			})
		case r.Method == http.MethodPost && r.URL.RequestURI() == "/v1/grants/preview":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "grant_1") {
				t.Fatalf("preview body was not forwarded: %s", string(body))
			}
			writeTestJSON(w, http.StatusOK, map[string]any{
				"preview": map[string]any{
					"egress_plan": map[string]any{"allowed": true},
					"chunks":      []map[string]any{{"canonical_path": "world/idea.md"}},
				},
			})
		case r.Method == http.MethodGet && r.URL.RequestURI() == "/v1/ingestion/queue":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"items": []map[string]any{{"path": "inbox/candidates/idea.md", "lifecycle": "candidate"}},
			})
		case r.Method == http.MethodPost && r.URL.RequestURI() == "/v1/ingestion/candidates/inbox/candidates/idea.md/approve":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"item": map[string]any{"path": "inbox/candidates/idea.md", "review_status": "approved"},
			})
		case r.Method == http.MethodPost && r.URL.RequestURI() == "/v1/ingestion/candidates/inbox/candidates/risky.md/reject":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "Not reliable enough") {
				t.Fatalf("reject body was not forwarded: %s", string(body))
			}
			writeTestJSON(w, http.StatusOK, map[string]any{
				"item": map[string]any{"path": "inbox/candidates/risky.md", "review_status": "rejected"},
			})
		case r.Method == http.MethodGet && r.URL.RequestURI() == "/v1/audit?limit=25":
			writeTestJSON(w, http.StatusOK, map[string]any{
				"events": []map[string]any{{"type": "grant.created", "actor": "owner"}},
			})
		default:
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer upstream.Close()

	handler := NewServicesHandler(ServiceConfig{ContextBaseURL: upstream.URL, ContextToken: token})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	requests := []struct {
		method   string
		path     string
		body     string
		wantData string
		wantSeen string
	}{
		{method: http.MethodGet, path: "/api/services/context/docs/identity/communication.md", wantData: "Communication", wantSeen: "GET /v1/context/identity/communication.md"},
		{method: http.MethodPut, path: "/api/services/context/docs/identity/communication.md", body: `{"content":"updated content"}`, wantData: "identity/communication.md", wantSeen: "PUT /v1/context/identity/communication.md"},
		{method: http.MethodGet, path: "/api/services/context/history/identity/communication.md", wantData: "abc123", wantSeen: "GET /v1/context/identity/communication.md/history"},
		{method: http.MethodPost, path: "/api/services/context/ask", body: `{"question":"How should agents speak?"}`, wantData: "Use short status updates.", wantSeen: "POST /v1/ask"},
		{method: http.MethodGet, path: "/api/services/context/grants", wantSeen: "GET /v1/grants"},
		{method: http.MethodPost, path: "/api/services/context/grants", body: `{"name":"ChatGPT"}`, wantSeen: "POST /v1/grants"},
		{method: http.MethodPost, path: "/api/services/context/grants/grant_1/revoke", wantSeen: "POST /v1/grants/grant_1/revoke"},
		{method: http.MethodPost, path: "/api/services/context/grants/grant_1/rotate", wantSeen: "POST /v1/grants/grant_1/rotate"},
		{method: http.MethodPost, path: "/api/services/context/grants/preview", body: `{"grant_id":"grant_1"}`, wantSeen: "POST /v1/grants/preview"},
		{method: http.MethodGet, path: "/api/services/context/ingestion/queue", wantSeen: "GET /v1/ingestion/queue"},
		{method: http.MethodPost, path: "/api/services/context/ingestion/candidates/inbox/candidates/idea.md/approve", wantSeen: "POST /v1/ingestion/candidates/inbox/candidates/idea.md/approve"},
		{method: http.MethodPost, path: "/api/services/context/ingestion/candidates/inbox/candidates/risky.md/reject", body: `{"reason":"Not reliable enough."}`, wantSeen: "POST /v1/ingestion/candidates/inbox/candidates/risky.md/reject"},
		{method: http.MethodGet, path: "/api/services/context/audit?limit=25", wantSeen: "GET /v1/audit?limit=25"},
	}

	wantSeen := make([]string, 0, len(requests))
	for _, request := range requests {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(request.method, request.path, strings.NewReader(request.body))
		if request.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", "Bearer browser-supplied-token")
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s status = %d, want 200; body: %s", request.method, request.path, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), token) || strings.Contains(rec.Body.String(), "browser-supplied-token") {
			t.Fatalf("%s %s exposed an upstream/browser token: %s", request.method, request.path, rec.Body.String())
		}
		if request.wantData != "" {
			assertEnvelopeData(t, rec.Body.String(), request.wantData)
		}
		wantSeen = append(wantSeen, request.wantSeen)
	}

	if got, want := strings.Join(seen, ","), strings.Join(wantSeen, ","); got != want {
		t.Fatalf("upstream calls = %q, want %q", got, want)
	}
}

func TestServicesHandlerContextAskUsesLongerTimeoutThanGenericProxy(t *testing.T) {
	token := strings.Join([]string{"fixture", "owner", "value"}, "-")

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/ask" {
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Fatalf("upstream Authorization = %q, want server token", r.Header.Get("Authorization"))
		}
		time.Sleep(75 * time.Millisecond)
		writeTestJSON(w, http.StatusOK, map[string]any{
			"answer":  "Use the context repository.",
			"sources": []map[string]any{{"path": "identity/bio.md"}},
		})
	}))
	defer upstream.Close()

	handler := newServicesHandlerWithClient(
		ServiceConfig{ContextBaseURL: upstream.URL, ContextToken: token},
		&http.Client{Timeout: 10 * time.Millisecond},
	)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	askReq := httptest.NewRequest(http.MethodPost, "/api/services/context/ask", strings.NewReader(`{"question":"What is this context repository for?"}`))
	askReq.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, askReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("ask status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	assertEnvelopeData(t, rec.Body.String(), "Use the context repository.")
}

// A document path with spaces in it reaches upstream percent-encoded, whether
// the browser left the separating slash literal or encoded it too. Both forms
// arrive from real navigation, and either one taken verbatim would ask upstream
// for a path that does not exist.
func TestServicesHandlerContextProxyEscapesNestedPathsWithSpaces(t *testing.T) {
	token := strings.Join([]string{"fixture", "owner", "value"}, "-")

	for _, testCase := range []struct {
		name      string
		requested string
	}{
		{
			name:      "the separating slash is left literal",
			requested: "/api/services/context/docs/team%20notes/agent%20profile.md",
		},
		{
			name:      "the separating slash is encoded too",
			requested: "/api/services/context/docs/team%20notes%2Fagent%20profile.md",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+token {
					t.Fatalf("upstream Authorization = %q, want server token", r.Header.Get("Authorization"))
				}
				if got, want := r.URL.EscapedPath(), "/v1/context/team%20notes/agent%20profile.md"; got != want {
					t.Fatalf("upstream path = %q, want %q", got, want)
				}
				writeTestJSON(w, http.StatusOK, map[string]any{
					"path":    "team notes/agent profile.md",
					"content": "# Agent Profile\n",
				})
			}))
			defer upstream.Close()

			handler := NewServicesHandler(ServiceConfig{ContextBaseURL: upstream.URL, ContextToken: token})
			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, testCase.requested, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("read status = %d, want 200", rec.Code)
			}
			assertEnvelopeData(t, rec.Body.String(), "Agent Profile")
		})
	}
}

func TestServicesHandlerContextProxyPreservesUpstreamAuthorizationAndNotFound(t *testing.T) {
	token := strings.Join([]string{"fixture", "wrong", "value"}, "-")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Fatalf("upstream Authorization = %q, want server token", r.Header.Get("Authorization"))
		}
		writeTestJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid token " + r.Header.Get("Authorization")})
	}))
	defer upstream.Close()

	handler := NewServicesHandler(ServiceConfig{ContextBaseURL: upstream.URL, ContextToken: token})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/services/context/docs/identity/missing.md", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if strings.Contains(rec.Body.String(), token) {
		t.Fatal("context proxy upstream error response exposed the server token")
	}
	if !strings.Contains(rec.Body.String(), "SERVICE_UPSTREAM_ERROR") || !strings.Contains(rec.Body.String(), "invalid token") {
		t.Fatalf("response did not preserve structured upstream authorization error: %s", rec.Body.String())
	}
}

func TestServicesHandlerContextProxyMissingTokenReturnsStructuredError(t *testing.T) {
	handler := NewServicesHandler(ServiceConfig{ContextBaseURL: "http://127.0.0.1:3200"})
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/services/context/docs", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "MISSING_CONTEXT_TOKEN") {
		t.Fatalf("missing structured error code in response: %s", rec.Body.String())
	}
}

func TestServicesHandlerUpstreamUnavailableReturnsStructuredError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	upstream.Close()

	handler := newServicesHandlerWithClient(
		ServiceConfig{TTSBaseURL: upstream.URL},
		&http.Client{Timeout: 50 * time.Millisecond},
	)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/services/tts/health", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "SERVICE_UPSTREAM_ERROR") {
		t.Fatalf("missing upstream error code in response: %s", rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, upstream.URL) || strings.Contains(body, "127.0.0.1") || strings.Contains(body, "connect:") {
		t.Fatalf("transport error response exposed internal upstream details: %s", body)
	}
}

func TestServicesHandlerUpstreamTimeoutReturnsStructuredGatewayTimeout(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(75 * time.Millisecond)
		writeTestJSON(w, http.StatusOK, map[string]any{"status": "late"})
	}))
	defer upstream.Close()

	handler := newServicesHandlerWithClient(
		ServiceConfig{TTSBaseURL: upstream.URL},
		&http.Client{Timeout: 10 * time.Millisecond},
	)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/services/tts/health", nil))

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "SERVICE_UPSTREAM_TIMEOUT") {
		t.Fatalf("missing timeout error code in response: %s", rec.Body.String())
	}
}

func writeTestJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func assertEnvelopeData(t *testing.T, raw string, needle string) {
	t.Helper()
	if !strings.Contains(raw, `"success":true`) {
		t.Fatalf("response is not a success envelope: %s", raw)
	}
	if !strings.Contains(raw, needle) {
		t.Fatalf("response does not contain %q: %s", needle, raw)
	}
}

type streamingResponseRecorder struct {
	header           http.Header
	body             strings.Builder
	status           int
	flushes          int
	writeDeadline    time.Time
	writeDeadlineSet bool
}

func (r *streamingResponseRecorder) Header() http.Header {
	return r.header
}

func (r *streamingResponseRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	return r.body.Write(p)
}

func (r *streamingResponseRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
}

func (r *streamingResponseRecorder) Flush() {
	r.flushes++
}

func (r *streamingResponseRecorder) SetWriteDeadline(deadline time.Time) error {
	r.writeDeadline = deadline
	r.writeDeadlineSet = true
	return nil
}
