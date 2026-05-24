package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/chrote/server/internal/core"
)

const (
	defaultTTSBaseURL     = "http://127.0.0.1:3100"
	defaultContextBaseURL = "http://127.0.0.1:3200"
)

// ServiceConfig is the server-side runtime configuration for /srv adapters.
type ServiceConfig struct {
	TTSBaseURL     string
	ContextBaseURL string
	ContextToken   string
}

// ServiceStatus is safe to return to browser clients.
type ServiceStatus struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Status          string   `json:"status"`
	Message         string   `json:"message,omitempty"`
	Configured      bool     `json:"configured"`
	TokenConfigured bool     `json:"tokenConfigured"`
	Capabilities    []string `json:"capabilities"`
}

// ServicesCatalogResponse describes configured CHROTE service adapters.
type ServicesCatalogResponse struct {
	Services []ServiceStatus `json:"services"`
}

// ServicesHandler handles CHROTE-owned service adapter routes.
type ServicesHandler struct {
	config           ServiceConfig
	client           *http.Client
	streamClient     *http.Client
	contextAskClient *http.Client
}

// LoadServiceConfigFromEnv loads service adapter config from process env.
func LoadServiceConfigFromEnv() ServiceConfig {
	return ServiceConfig{
		TTSBaseURL:     envURL("CHROTE_TTS_URL", defaultTTSBaseURL),
		ContextBaseURL: envURL("CHROTE_CONTEXT_API_URL", defaultContextBaseURL),
		ContextToken:   strings.TrimSpace(os.Getenv("CHROTE_CONTEXT_API_TOKEN")),
	}
}

// NewServicesHandler creates a Services handler with explicit runtime config.
func NewServicesHandler(config ServiceConfig) *ServicesHandler {
	if strings.TrimSpace(config.TTSBaseURL) == "" {
		config.TTSBaseURL = defaultTTSBaseURL
	}
	if strings.TrimSpace(config.ContextBaseURL) == "" {
		config.ContextBaseURL = defaultContextBaseURL
	}
	config.TTSBaseURL = cleanBaseURL(config.TTSBaseURL)
	config.ContextBaseURL = cleanBaseURL(config.ContextBaseURL)
	config.ContextToken = strings.TrimSpace(config.ContextToken)

	return &ServicesHandler{
		config:           config,
		client:           &http.Client{Timeout: 15 * time.Second},
		streamClient:     &http.Client{},
		contextAskClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// NewServicesHandlerWithClient creates a Services handler with a testable HTTP client.
func NewServicesHandlerWithClient(config ServiceConfig, client *http.Client) *ServicesHandler {
	handler := NewServicesHandler(config)
	if client != nil {
		handler.client = client
		handler.streamClient = clientWithoutTimeout(client)
	}
	return handler
}

// RegisterRoutes registers service adapter routes.
func (h *ServicesHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/services", h.Catalog)
	mux.HandleFunc("GET /api/services/tts/health", h.TTSHealth)
	mux.HandleFunc("GET /api/services/tts/messages", h.TTSMessages)
	mux.HandleFunc("POST /api/services/tts/enqueue", h.TTSEnqueue)
	mux.HandleFunc("GET /api/services/tts/audio/{id}", h.TTSAudio)
	mux.HandleFunc("GET /api/services/tts/feed", h.TTSFeed)
	mux.HandleFunc("GET /api/services/context/health", h.ContextHealth)
	mux.HandleFunc("GET /api/services/context/docs", h.ContextDocs)
	mux.HandleFunc("GET /api/services/context/docs/{path...}", h.ContextRead)
	mux.HandleFunc("PUT /api/services/context/docs/{path...}", h.ContextSave)
	mux.HandleFunc("GET /api/services/context/history/{path...}", h.ContextHistory)
	mux.HandleFunc("POST /api/services/context/ask", h.ContextAsk)
	mux.HandleFunc("GET /api/services/context/grants", h.ContextGrants)
	mux.HandleFunc("POST /api/services/context/grants", h.ContextCreateGrant)
	mux.HandleFunc("POST /api/services/context/grants/preview", h.ContextGrantPreview)
	mux.HandleFunc("POST /api/services/context/grants/{id}/revoke", h.ContextRevokeGrant)
	mux.HandleFunc("POST /api/services/context/grants/{id}/rotate", h.ContextRotateGrant)
	mux.HandleFunc("GET /api/services/context/ingestion/queue", h.ContextIngestionQueue)
	mux.HandleFunc("POST /api/services/context/ingestion/candidates/{path...}", h.ContextReviewIngestionCandidate)
	mux.HandleFunc("GET /api/services/context/audit", h.ContextAudit)
}

// Catalog handles GET /api/services.
func (h *ServicesHandler) Catalog(w http.ResponseWriter, r *http.Request) {
	core.WriteSuccess(w, ServicesCatalogResponse{
		Services: []ServiceStatus{
			{
				ID:         "tts",
				Name:       "TTS Gateway",
				Status:     "configured",
				Configured: true,
				Capabilities: []string{
					"health",
					"messages",
					"enqueue",
					"audio",
					"feed",
				},
			},
			h.contextStatus(),
		},
	})
}

func (h *ServicesHandler) contextStatus() ServiceStatus {
	status := ServiceStatus{
		ID:              "context",
		Name:            "Context Citadel",
		Status:          "configured",
		Configured:      true,
		TokenConfigured: h.config.ContextToken != "",
		Capabilities: []string{
			"list",
			"read",
			"save",
			"history",
			"ask",
			"grants",
			"preview",
			"ingestion_queue",
			"ingestion_review",
			"audit",
		},
	}
	if !status.TokenConfigured {
		status.Status = "degraded"
		status.Message = "CHROTE_CONTEXT_API_TOKEN is not configured; Context Citadel document and integration operations are disabled."
	}
	return status
}

// TTSHealth handles GET /api/services/tts/health.
func (h *ServicesHandler) TTSHealth(w http.ResponseWriter, r *http.Request) {
	h.proxyJSON(w, r, h.config.TTSBaseURL, "/health", "")
}

// TTSMessages handles GET /api/services/tts/messages.
func (h *ServicesHandler) TTSMessages(w http.ResponseWriter, r *http.Request) {
	h.proxyJSON(w, r, h.config.TTSBaseURL, "/v1/tts/messages", "")
}

// TTSEnqueue handles POST /api/services/tts/enqueue.
func (h *ServicesHandler) TTSEnqueue(w http.ResponseWriter, r *http.Request) {
	h.proxyJSON(w, r, h.config.TTSBaseURL, "/v1/tts/enqueue", "")
}

// TTSAudio handles GET /api/services/tts/audio/{id}.
func (h *ServicesHandler) TTSAudio(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.proxyRaw(w, r, h.config.TTSBaseURL, "/v1/tts/audio/"+url.PathEscape(id), "")
}

// TTSFeed handles GET /api/services/tts/feed.
func (h *ServicesHandler) TTSFeed(w http.ResponseWriter, r *http.Request) {
	h.proxySSE(w, r, h.streamClient, h.config.TTSBaseURL, "/v1/tts/feed", "")
}

// ContextHealth handles GET /api/services/context/health.
func (h *ServicesHandler) ContextHealth(w http.ResponseWriter, r *http.Request) {
	h.proxyJSON(w, r, h.config.ContextBaseURL, "/health", "")
}

// ContextDocs handles GET /api/services/context/docs.
func (h *ServicesHandler) ContextDocs(w http.ResponseWriter, r *http.Request) {
	h.proxyContextJSON(w, r, "/v1/context")
}

// ContextRead handles GET /api/services/context/docs/{path...}.
func (h *ServicesHandler) ContextRead(w http.ResponseWriter, r *http.Request) {
	h.proxyContextJSON(w, r, "/v1/context/"+escapePathValue(r.PathValue("path")))
}

// ContextSave handles PUT /api/services/context/docs/{path...}.
func (h *ServicesHandler) ContextSave(w http.ResponseWriter, r *http.Request) {
	h.proxyContextJSON(w, r, "/v1/context/"+escapePathValue(r.PathValue("path")))
}

// ContextHistory handles GET /api/services/context/history/{path...}.
func (h *ServicesHandler) ContextHistory(w http.ResponseWriter, r *http.Request) {
	h.proxyContextJSON(w, r, "/v1/context/"+escapePathValue(r.PathValue("path"))+"/history")
}

// ContextAsk handles POST /api/services/context/ask.
func (h *ServicesHandler) ContextAsk(w http.ResponseWriter, r *http.Request) {
	h.proxyContextJSONWithClient(w, r, h.contextAskClient, "/v1/ask")
}

// ContextGrants handles GET /api/services/context/grants.
func (h *ServicesHandler) ContextGrants(w http.ResponseWriter, r *http.Request) {
	h.proxyContextJSON(w, r, "/v1/grants")
}

// ContextCreateGrant handles POST /api/services/context/grants.
func (h *ServicesHandler) ContextCreateGrant(w http.ResponseWriter, r *http.Request) {
	h.proxyContextJSON(w, r, "/v1/grants")
}

// ContextGrantPreview handles POST /api/services/context/grants/preview.
func (h *ServicesHandler) ContextGrantPreview(w http.ResponseWriter, r *http.Request) {
	h.proxyContextJSON(w, r, "/v1/grants/preview")
}

// ContextRevokeGrant handles POST /api/services/context/grants/{id}/revoke.
func (h *ServicesHandler) ContextRevokeGrant(w http.ResponseWriter, r *http.Request) {
	h.proxyContextJSON(w, r, "/v1/grants/"+url.PathEscape(r.PathValue("id"))+"/revoke")
}

// ContextRotateGrant handles POST /api/services/context/grants/{id}/rotate.
func (h *ServicesHandler) ContextRotateGrant(w http.ResponseWriter, r *http.Request) {
	h.proxyContextJSON(w, r, "/v1/grants/"+url.PathEscape(r.PathValue("id"))+"/rotate")
}

// ContextIngestionQueue handles GET /api/services/context/ingestion/queue.
func (h *ServicesHandler) ContextIngestionQueue(w http.ResponseWriter, r *http.Request) {
	h.proxyContextJSON(w, r, "/v1/ingestion/queue")
}

// ContextReviewIngestionCandidate handles candidate approve/reject review actions.
func (h *ServicesHandler) ContextReviewIngestionCandidate(w http.ResponseWriter, r *http.Request) {
	pathValue := r.PathValue("path")
	for _, action := range []string{"approve", "reject"} {
		suffix := "/" + action
		if strings.HasSuffix(pathValue, suffix) {
			candidatePath := strings.TrimSuffix(pathValue, suffix)
			if candidatePath == "" {
				core.WriteError(w, http.StatusBadRequest, "INVALID_CONTEXT_PATH", "candidate path is required")
				return
			}
			h.proxyContextJSON(w, r, "/v1/ingestion/candidates/"+escapePathValue(candidatePath)+suffix)
			return
		}
	}
	core.WriteError(w, http.StatusNotFound, "CONTEXT_ROUTE_NOT_FOUND", "Context ingestion review action not found")
}

// ContextAudit handles GET /api/services/context/audit.
func (h *ServicesHandler) ContextAudit(w http.ResponseWriter, r *http.Request) {
	h.proxyContextJSON(w, r, "/v1/audit")
}

func (h *ServicesHandler) proxyContextJSON(w http.ResponseWriter, r *http.Request, path string) {
	h.proxyContextJSONWithClient(w, r, h.client, path)
}

func (h *ServicesHandler) proxyContextJSONWithClient(w http.ResponseWriter, r *http.Request, client *http.Client, path string) {
	if h.config.ContextToken == "" {
		core.WriteError(w, http.StatusServiceUnavailable, "MISSING_CONTEXT_TOKEN", "CHROTE_CONTEXT_API_TOKEN is not configured")
		return
	}
	h.proxyJSONWithClient(w, r, client, h.config.ContextBaseURL, path, h.config.ContextToken)
}

func (h *ServicesHandler) proxyJSON(w http.ResponseWriter, r *http.Request, baseURL, path, bearerToken string) {
	h.proxyJSONWithClient(w, r, h.client, baseURL, path, bearerToken)
}

func (h *ServicesHandler) proxyJSONWithClient(w http.ResponseWriter, r *http.Request, client *http.Client, baseURL, path, bearerToken string) {
	if client == nil {
		client = h.client
	}
	resp, err := h.forwardWithClient(r, client, baseURL, path, bearerToken)
	if err != nil {
		writeForwardError(w, err)
		return
	}
	defer resp.Body.Close()

	var payload any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		core.WriteError(w, http.StatusBadGateway, "SERVICE_INVALID_RESPONSE", "upstream returned invalid JSON")
		return
	}
	payload = redactSecrets(payload, bearerToken)

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		core.WriteJSON(w, resp.StatusCode, core.NewErrorResponse("SERVICE_UPSTREAM_ERROR", upstreamErrorMessage(payload)))
		return
	}

	core.WriteJSON(w, resp.StatusCode, core.NewSuccessResponse(payload))
}

func (h *ServicesHandler) proxyRaw(w http.ResponseWriter, r *http.Request, baseURL, path, bearerToken string) {
	h.proxyRawWithClient(w, r, h.client, baseURL, path, bearerToken)
}

func (h *ServicesHandler) proxyRawWithClient(w http.ResponseWriter, r *http.Request, client *http.Client, baseURL, path, bearerToken string) {
	resp, err := h.forwardWithClient(r, client, baseURL, path, bearerToken)
	if err != nil {
		writeForwardError(w, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		core.WriteError(w, resp.StatusCode, "SERVICE_UPSTREAM_ERROR", "upstream returned an error")
		return
	}

	copyHeader(w.Header(), resp.Header, "Content-Type")
	copyHeader(w.Header(), resp.Header, "Content-Length")
	copyHeader(w.Header(), resp.Header, "Cache-Control")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (h *ServicesHandler) proxySSE(w http.ResponseWriter, r *http.Request, client *http.Client, baseURL, path, bearerToken string) {
	resp, err := h.forwardWithClient(r, client, baseURL, path, bearerToken)
	if err != nil {
		writeForwardError(w, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		core.WriteError(w, resp.StatusCode, "SERVICE_UPSTREAM_ERROR", "upstream returned an error")
		return
	}

	controller := http.NewResponseController(w)
	_ = controller.SetWriteDeadline(time.Time{})
	copyHeader(w.Header(), resp.Header, "Content-Type")
	copyHeader(w.Header(), resp.Header, "Cache-Control")
	w.WriteHeader(resp.StatusCode)
	_ = controller.Flush()
	copyAndFlush(w, resp.Body, controller)
}

func (h *ServicesHandler) forward(r *http.Request, baseURL, path, bearerToken string) (*http.Response, error) {
	return h.forwardWithClient(r, h.client, baseURL, path, bearerToken)
}

func (h *ServicesHandler) forwardWithClient(r *http.Request, client *http.Client, baseURL, path, bearerToken string) (*http.Response, error) {
	target := baseURL + path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		return nil, err
	}
	if contentType := r.Header.Get("Content-Type"); contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if accept := r.Header.Get("Accept"); accept != "" {
		req.Header.Set("Accept", accept)
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	return client.Do(req)
}

func clientWithoutTimeout(client *http.Client) *http.Client {
	clone := *client
	clone.Timeout = 0
	return &clone
}

func writeForwardError(w http.ResponseWriter, err error) {
	if os.IsTimeout(err) {
		core.WriteError(w, http.StatusGatewayTimeout, "SERVICE_UPSTREAM_TIMEOUT", "upstream service timed out")
		return
	}
	core.WriteError(w, http.StatusBadGateway, "SERVICE_UPSTREAM_ERROR", "upstream service unavailable")
}

func upstreamErrorMessage(payload any) string {
	if body, ok := payload.(map[string]any); ok {
		if msg, ok := body["error"].(string); ok && msg != "" {
			return msg
		}
	}
	return "upstream returned an error"
}

func redactSecrets(value any, secrets ...string) any {
	filtered := make([]string, 0, len(secrets)*2)
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		filtered = append(filtered, "Bearer "+secret, secret)
	}
	if len(filtered) == 0 {
		return value
	}
	return redactValue(value, filtered)
}

func redactValue(value any, secrets []string) any {
	switch typed := value.(type) {
	case string:
		for _, secret := range secrets {
			typed = strings.ReplaceAll(typed, secret, "[redacted]")
		}
		return typed
	case []any:
		redacted := make([]any, len(typed))
		for i, item := range typed {
			redacted[i] = redactValue(item, secrets)
		}
		return redacted
	case map[string]any:
		redacted := make(map[string]any, len(typed))
		for key, item := range typed {
			redactedKey := redactValue(key, secrets).(string)
			redacted[redactedKey] = redactValue(item, secrets)
		}
		return redacted
	default:
		return value
	}
}

func escapePathValue(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}

func copyHeader(dst, src http.Header, name string) {
	if value := src.Get(name); value != "" {
		dst.Set(name, value)
	}
}

func copyAndFlush(w http.ResponseWriter, r io.Reader, controller *http.ResponseController) {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return
			}
			_ = controller.Flush()
		}
		if err != nil {
			return
		}
	}
}

func envURL(name, fallback string) string {
	if value := cleanBaseURL(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func cleanBaseURL(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func (c ServiceConfig) String() string {
	return fmt.Sprintf("tts=%s context=%s contextTokenConfigured=%t", c.TTSBaseURL, c.ContextBaseURL, c.ContextToken != "")
}
