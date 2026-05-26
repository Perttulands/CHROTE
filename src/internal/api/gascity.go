package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chrote/server/internal/core"
)

const (
	defaultGasCityBaseURL         = "http://127.0.0.1:8372"
	defaultGasCityCityDir         = "/home/perttu/gascity"
	defaultGasCityPoemTarget      = "chrote-poem-pi"
	defaultGasCityPoemTemplate    = "pi-smoke"
	defaultGasCityMailRecipient   = "human"
	gasCityMailBodyLimit          = 4096
	gasCityMailStoreLimit         = 64 << 20
	gasCityMaxMailLimit           = 50
	gasCityPoemOutputLimit        = 4000
	gasCityPoemTopicLimit         = 80
	gasCityTranscriptDefaultLines = 120
	gasCityTranscriptMaxLines     = 500
	gasCityTranscriptOutputLimit  = 64 << 10
)

var (
	gasCityIdentityPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	gasCitySessionIDPattern = regexp.MustCompile(`^gc-[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	gasCityPoemTopicPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 .,!_?-]{0,79}$`)
	gasCityANSIPattern      = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

	errGasCityConfiguredCityUnavailable = errors.New("configured Gas City is not running")
	errGasCitySessionListUnavailable    = errors.New("Gas City session list unavailable")
	errGasCitySessionNotFound           = errors.New("Gas City session not found")
	errGasCitySessionAmbiguous          = errors.New("Gas City session id is ambiguous")
)

// GasCityConfig is the server-side configuration for CHROTE's bounded Gas City surface.
type GasCityConfig struct {
	BaseURL       string
	CityDir       string
	PoemTarget    string
	PoemTemplate  string
	MailRecipient string
}

// GasCityObserverResponse is a CHROTE-safe read model of the local Gas City supervisor.
type GasCityObserverResponse struct {
	Status         string                  `json:"status"`
	CheckedAt      string                  `json:"checkedAt"`
	Error          string                  `json:"error,omitempty"`
	Health         GasCityHealthSummary    `json:"health"`
	Cities         []GasCityCitySummary    `json:"cities"`
	Sessions       []GasCitySessionSummary `json:"sessions"`
	Mail           GasCityMailCounts       `json:"mail"`
	Work           GasCityWorkCounts       `json:"work"`
	Formulas       []GasCityFormulaSummary `json:"formulas"`
	Molecules      []GasCityWorkItem       `json:"molecules"`
	Wisps          []GasCityWorkItem       `json:"wisps"`
	Convoys        []GasCityWorkItem       `json:"convoys"`
	RecentEvents   []GasCityEventSummary   `json:"recentEvents"`
	UpstreamErrors []GasCityUpstreamError  `json:"upstreamErrors,omitempty"`
}

type GasCityHealthSummary struct {
	Status        string `json:"status"`
	Version       string `json:"version,omitempty"`
	BuildID       string `json:"buildId,omitempty"`
	UptimeSeconds int64  `json:"uptimeSeconds,omitempty"`
	CitiesTotal   int    `json:"citiesTotal"`
	CitiesRunning int    `json:"citiesRunning"`
	StartupReady  bool   `json:"startupReady"`
	StartupPhase  string `json:"startupPhase,omitempty"`
}

type GasCityCitySummary struct {
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Running bool   `json:"running"`
	Status  string `json:"status,omitempty"`
	Error   string `json:"error,omitempty"`
}

type GasCitySessionSummary struct {
	City        string `json:"city"`
	ID          string `json:"id"`
	Title       string `json:"title,omitempty"`
	Alias       string `json:"alias,omitempty"`
	Template    string `json:"template,omitempty"`
	State       string `json:"state,omitempty"`
	Provider    string `json:"provider,omitempty"`
	SessionName string `json:"sessionName,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	LastActive  string `json:"lastActive,omitempty"`
	Running     bool   `json:"running"`
	Attached    bool   `json:"attached"`
}

type GasCityMailCounts struct {
	Total  int `json:"total"`
	Unread int `json:"unread"`
}

type GasCityWorkCounts struct {
	Open       int `json:"open"`
	Ready      int `json:"ready"`
	InProgress int `json:"inProgress"`
	Routed     int `json:"routed"`
	Molecules  int `json:"molecules"`
	Wisps      int `json:"wisps"`
	Convoys    int `json:"convoys"`
}

type GasCityFormulaSummary struct {
	City        string `json:"city"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	RunCount    int    `json:"runCount"`
}

type GasCityWorkItem struct {
	City      string `json:"city"`
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	Status    string `json:"status,omitempty"`
	IssueType string `json:"issueType,omitempty"`
	Ref       string `json:"ref,omitempty"`
	RoutedTo  string `json:"routedTo,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type GasCityEventSummary struct {
	City    string `json:"city,omitempty"`
	Seq     int64  `json:"seq"`
	Type    string `json:"type"`
	Time    string `json:"time,omitempty"`
	Actor   string `json:"actor,omitempty"`
	Subject string `json:"subject,omitempty"`
}

type GasCityUpstreamError struct {
	Route   string `json:"route"`
	Message string `json:"message"`
}

type GasCityMailListResponse struct {
	Recipient string               `json:"recipient"`
	Limit     int                  `json:"limit"`
	Messages  []GasCityMailMessage `json:"messages"`
}

type GasCityMailMessage struct {
	ID            string `json:"id"`
	From          string `json:"from,omitempty"`
	Recipient     string `json:"recipient"`
	Subject       string `json:"subject,omitempty"`
	Body          string `json:"body"`
	BodyTruncated bool   `json:"bodyTruncated"`
	Status        string `json:"status,omitempty"`
	IssueType     string `json:"issueType,omitempty"`
	Read          bool   `json:"read"`
	FromSessionID string `json:"fromSessionId,omitempty"`
	CreatedAt     string `json:"createdAt,omitempty"`
	UpdatedAt     string `json:"updatedAt,omitempty"`
	order         int
}

type GasCityPiPoemRequest struct {
	Topic string `json:"topic"`
}

type GasCityPiPoemResponse struct {
	Nonce           string `json:"nonce"`
	Subject         string `json:"subject"`
	Target          string `json:"target"`
	TargetAlias     string `json:"targetAlias,omitempty"`
	TargetTemplate  string `json:"targetTemplate,omitempty"`
	TargetSessionID string `json:"targetSessionId,omitempty"`
	Recipient       string `json:"recipient"`
	Output          string `json:"output"`
}

type GasCityTranscriptResponse struct {
	Source     string `json:"source"`
	SessionID  string `json:"sessionId"`
	Alias      string `json:"alias,omitempty"`
	Template   string `json:"template,omitempty"`
	State      string `json:"state,omitempty"`
	City       string `json:"city,omitempty"`
	Lines      int    `json:"lines"`
	LineCount  int    `json:"lineCount"`
	Transcript string `json:"transcript"`
	Truncated  bool   `json:"truncated"`
}

type GasCityAuditResponse struct {
	Entries []gasCityAuditEntry `json:"entries"`
}

// GasCityHandler handles CHROTE's bounded Gas City observer and control routes.
type GasCityHandler struct {
	config             GasCityConfig
	client             *http.Client
	configError        string
	controlConfigError string
	runner             gasCityCommandRunner
	nonce              func() string
	auditMu            sync.Mutex
	audit              []gasCityAuditEntry
}

type gasCityAuditEntry struct {
	Time            string `json:"time"`
	Action          string `json:"action"`
	Target          string `json:"target"`
	TargetAlias     string `json:"targetAlias,omitempty"`
	TargetTemplate  string `json:"targetTemplate,omitempty"`
	TargetSessionID string `json:"targetSessionId,omitempty"`
	Recipient       string `json:"recipient"`
	Subject         string `json:"subject"`
	Nonce           string `json:"nonce"`
	Success         bool   `json:"success"`
	Error           string `json:"error,omitempty"`
}

type gasCityCommandRunner interface {
	Run(ctx context.Context, name string, args []string) (string, error)
}

type gasCityExecRunner struct{}

func (gasCityExecRunner) Run(ctx context.Context, name string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// LoadGasCityConfigFromEnv loads Gas City config from process env.
func LoadGasCityConfigFromEnv() GasCityConfig {
	baseURL := cleanBaseURL(os.Getenv("CHROTE_GASCITY_URL"))
	if baseURL == "" {
		baseURL = defaultGasCityBaseURL
	}
	cityDir := strings.TrimSpace(os.Getenv("CHROTE_GASCITY_CITY_DIR"))
	if cityDir == "" {
		cityDir = defaultGasCityCityDir
	}
	target := strings.TrimSpace(os.Getenv("CHROTE_GASCITY_PI_POEM_TARGET"))
	if target == "" {
		target = defaultGasCityPoemTarget
	}
	template := strings.TrimSpace(os.Getenv("CHROTE_GASCITY_PI_POEM_TEMPLATE"))
	if template == "" {
		template = defaultGasCityPoemTemplate
	}
	recipient := strings.TrimSpace(os.Getenv("CHROTE_GASCITY_MAIL_RECIPIENT"))
	if recipient == "" {
		recipient = defaultGasCityMailRecipient
	}
	return GasCityConfig{
		BaseURL:       baseURL,
		CityDir:       cityDir,
		PoemTarget:    target,
		PoemTemplate:  template,
		MailRecipient: recipient,
	}
}

// NewGasCityHandler creates a Gas City handler with production defaults.
func NewGasCityHandler(config GasCityConfig) *GasCityHandler {
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = defaultGasCityBaseURL
	}
	if strings.TrimSpace(config.CityDir) == "" {
		config.CityDir = defaultGasCityCityDir
	}
	if strings.TrimSpace(config.PoemTarget) == "" {
		config.PoemTarget = defaultGasCityPoemTarget
	}
	if strings.TrimSpace(config.PoemTemplate) == "" {
		config.PoemTemplate = defaultGasCityPoemTemplate
	}
	if strings.TrimSpace(config.MailRecipient) == "" {
		config.MailRecipient = defaultGasCityMailRecipient
	}
	baseURL, configError := validateGasCityBaseURL(config.BaseURL)
	cityDir, cityDirError := validateGasCityCityDir(config.CityDir)
	target, targetError := validateGasCityIdentity("CHROTE_GASCITY_PI_POEM_TARGET", config.PoemTarget)
	template, templateError := validateGasCityIdentity("CHROTE_GASCITY_PI_POEM_TEMPLATE", config.PoemTemplate)
	recipient, recipientError := validateGasCityIdentity("CHROTE_GASCITY_MAIL_RECIPIENT", config.MailRecipient)
	return &GasCityHandler{
		config: GasCityConfig{
			BaseURL:       baseURL,
			CityDir:       cityDir,
			PoemTarget:    target,
			PoemTemplate:  template,
			MailRecipient: recipient,
		},
		client:             newGasCityHTTPClient(),
		configError:        configError,
		controlConfigError: firstNonEmpty(cityDirError, targetError, templateError, recipientError),
		runner:             gasCityExecRunner{},
		nonce:              newGasCityNonce,
		audit:              []gasCityAuditEntry{},
	}
}

// NewGasCityHandlerWithClient creates a Gas City observer handler with a testable HTTP client.
func NewGasCityHandlerWithClient(config GasCityConfig, client *http.Client) *GasCityHandler {
	handler := NewGasCityHandler(config)
	if client != nil {
		handler.client = client
	}
	return handler
}

// RegisterRoutes registers Gas City routes.
func (h *GasCityHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/gascity/observer", h.Observer)
	mux.HandleFunc("GET /api/gascity/mail", h.Mail)
	mux.HandleFunc("GET /api/gascity/audit", h.Audit)
	mux.HandleFunc("GET /api/gascity/sessions/{id}/transcript", h.Transcript)
	mux.HandleFunc("POST /api/gascity/requests/pi-poem", h.PiPoem)
}

// Observer handles GET /api/gascity/observer.
func (h *GasCityHandler) Observer(w http.ResponseWriter, r *http.Request) {
	core.WriteSuccess(w, h.snapshot(r.Context()))
}

// Mail handles GET /api/gascity/mail.
func (h *GasCityHandler) Mail(w http.ResponseWriter, r *http.Request) {
	if h.controlConfigError != "" {
		core.WriteError(w, http.StatusServiceUnavailable, "GASCITY_MISCONFIGURED", h.controlConfigError)
		return
	}
	recipient := strings.TrimSpace(r.URL.Query().Get("recipient"))
	if recipient == "" {
		recipient = h.config.MailRecipient
	}
	if recipient != h.config.MailRecipient {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Gas City mail recipient is fixed to "+h.config.MailRecipient)
		return
	}
	limit, err := parseGasCityMailLimit(r.URL.Query().Get("limit"))
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	messages, err := h.mailMessages(recipient, limit)
	if err != nil {
		core.WriteError(w, http.StatusServiceUnavailable, "GASCITY_STORE_UNAVAILABLE", sanitizeGasCityStoreError(err))
		return
	}
	core.WriteSuccess(w, GasCityMailListResponse{
		Recipient: recipient,
		Limit:     limit,
		Messages:  messages,
	})
}

// Audit handles GET /api/gascity/audit.
func (h *GasCityHandler) Audit(w http.ResponseWriter, r *http.Request) {
	h.auditMu.Lock()
	entries := append([]gasCityAuditEntry(nil), h.audit...)
	h.auditMu.Unlock()
	core.WriteSuccess(w, GasCityAuditResponse{Entries: entries})
}

// Transcript handles GET /api/gascity/sessions/{id}/transcript.
func (h *GasCityHandler) Transcript(w http.ResponseWriter, r *http.Request) {
	if h.controlConfigError != "" {
		core.WriteError(w, http.StatusServiceUnavailable, "GASCITY_MISCONFIGURED", h.controlConfigError)
		return
	}
	if h.configError != "" {
		core.WriteError(w, http.StatusServiceUnavailable, "GASCITY_MISCONFIGURED", h.configError)
		return
	}
	sessionID, err := validateGasCitySessionID(r.PathValue("id"))
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	lines, err := parseGasCityTranscriptLines(r.URL.Query().Get("lines"))
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	resolveCtx, resolveCancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer resolveCancel()
	session, err := h.resolveGasCityTranscriptSession(resolveCtx, sessionID)
	if err != nil {
		switch {
		case errors.Is(err, errGasCitySessionNotFound):
			core.WriteError(w, http.StatusNotFound, "GASCITY_SESSION_NOT_FOUND", "Gas City session not found")
		case errors.Is(err, errGasCitySessionAmbiguous):
			core.WriteError(w, http.StatusConflict, "GASCITY_SESSION_AMBIGUOUS", "Gas City session id is ambiguous")
		default:
			core.WriteError(w, http.StatusServiceUnavailable, "GASCITY_SESSION_UNAVAILABLE", err.Error())
		}
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	output, err := h.runner.Run(ctx, "gc", []string{
		"--city", h.config.CityDir,
		"session", "peek", session.ID,
		"--lines", strconv.Itoa(lines),
	})
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "GASCITY_TRANSCRIPT_UNAVAILABLE", "Gas City transcript peek failed")
		return
	}

	transcript, truncated := sanitizeGasCityTranscriptOutput(output)
	core.WriteSuccess(w, GasCityTranscriptResponse{
		Source:     "gc-session-peek",
		SessionID:  session.ID,
		Alias:      session.Alias,
		Template:   session.Template,
		State:      session.State,
		City:       session.CityName,
		Lines:      lines,
		LineCount:  countGasCityTranscriptLines(transcript),
		Transcript: transcript,
		Truncated:  truncated,
	})
}

// PiPoem handles POST /api/gascity/requests/pi-poem.
func (h *GasCityHandler) PiPoem(w http.ResponseWriter, r *http.Request) {
	if h.controlConfigError != "" {
		core.WriteError(w, http.StatusServiceUnavailable, "GASCITY_MISCONFIGURED", h.controlConfigError)
		return
	}
	if h.configError != "" {
		core.WriteError(w, http.StatusServiceUnavailable, "GASCITY_MISCONFIGURED", h.configError)
		return
	}
	var request GasCityPiPoemRequest
	body := http.MaxBytesReader(w, r.Body, 1024)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
		return
	}
	topic, err := validateGasCityPoemTopic(request.Topic)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	resolveCtx, resolveCancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer resolveCancel()
	session, err := h.resolveGasCityPoemSession(resolveCtx)
	if err != nil {
		core.WriteError(w, http.StatusConflict, "GASCITY_SESSION_UNAVAILABLE", err.Error())
		return
	}

	nonce := h.nonce()
	subject := "CHROTE Pi poem " + nonce
	command := buildGasCityPiPoemCommand(topic, nonce, subject, h.config.MailRecipient)
	args := []string{
		"--city", h.config.CityDir,
		"session", "nudge", session.ID,
		"--delivery", "immediate",
		command,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	output, err := h.runner.Run(ctx, "gc", args)
	success := err == nil
	errorMessage := ""
	if err != nil {
		errorMessage = "Gas City poem request failed"
	}
	h.recordAudit(gasCityAuditEntry{
		Time:            time.Now().UTC().Format(time.RFC3339),
		Action:          "pi-poem",
		Target:          h.config.PoemTarget,
		TargetAlias:     session.Alias,
		TargetTemplate:  session.Template,
		TargetSessionID: session.ID,
		Recipient:       h.config.MailRecipient,
		Subject:         subject,
		Nonce:           nonce,
		Success:         success,
		Error:           errorMessage,
	})
	log.Printf("gascity control action=pi-poem target_session=%s target_alias=%s target_template=%s recipient=%s nonce=%s success=%t", session.ID, session.Alias, session.Template, h.config.MailRecipient, nonce, success)
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, "GASCITY_REQUEST_FAILED", "Gas City poem request failed")
		return
	}

	core.WriteSuccess(w, GasCityPiPoemResponse{
		Nonce:           nonce,
		Subject:         subject,
		Target:          session.ID,
		TargetAlias:     session.Alias,
		TargetTemplate:  session.Template,
		TargetSessionID: session.ID,
		Recipient:       h.config.MailRecipient,
		Output:          sanitizeGasCityCLIOutput(output),
	})
}

func (h *GasCityHandler) snapshot(ctx context.Context) GasCityObserverResponse {
	response := newGasCityObserverResponse("ok")
	if h.configError != "" {
		response.Status = "misconfigured"
		response.Error = h.configError
		return response
	}

	var health gasCityHealthResponse
	if err := h.getJSON(ctx, "/health", &health); err != nil {
		response.Status = "unavailable"
		response.Error = "Gas City supervisor unavailable"
		response.UpstreamErrors = append(response.UpstreamErrors, GasCityUpstreamError{
			Route:   "/health",
			Message: sanitizeGasCityUpstreamError(err),
		})
		return response
	}
	response.Health = health.summary()

	var cities gasCityCitiesResponse
	if err := h.getJSON(ctx, "/v0/cities", &cities); err != nil {
		response.Status = "degraded"
		response.Error = "Gas City city list unavailable"
		response.UpstreamErrors = append(response.UpstreamErrors, GasCityUpstreamError{
			Route:   "/v0/cities",
			Message: sanitizeGasCityUpstreamError(err),
		})
		return response
	}

	for _, city := range cities.Items {
		response.Cities = append(response.Cities, GasCityCitySummary{
			Name:    city.Name,
			Path:    city.Path,
			Running: city.Running,
			Status:  city.Status,
			Error:   city.Error,
		})
		if city.Name == "" || !city.Running {
			continue
		}
		h.addCitySnapshot(ctx, city.Name, &response)
	}

	var events gasCityEventsResponse
	if err := h.getJSON(ctx, "/v0/events?limit=20", &events); err != nil {
		addGasCityUpstreamError(&response, "/v0/events", err)
	} else {
		for _, event := range events.Items {
			response.RecentEvents = append(response.RecentEvents, event.summary())
		}
	}

	response.Work.Molecules = len(response.Molecules)
	response.Work.Wisps = len(response.Wisps)
	response.Work.Convoys = len(response.Convoys)
	if len(response.UpstreamErrors) > 0 && response.Status == "ok" {
		response.Status = "degraded"
		response.Error = "Some Gas City observer fields are unavailable"
	}
	return response
}

func (h *GasCityHandler) addCitySnapshot(ctx context.Context, cityName string, response *GasCityObserverResponse) {
	escapedCity := url.PathEscape(cityName)

	var status gasCityCityStatusResponse
	statusOK := true
	if err := h.getJSON(ctx, "/v0/city/"+escapedCity+"/status", &status); err != nil {
		statusOK = false
		addGasCityUpstreamError(response, "/v0/city/{city}/status", err)
	} else {
		response.Work.Open += status.Work.Open
		response.Work.Ready += status.Work.Ready
		response.Work.InProgress += status.Work.InProgress
	}

	var sessions gasCitySessionsResponse
	sessionPath := "/v0/city/" + escapedCity + "/sessions?" + url.Values{
		"limit": []string{"50"},
		"peek":  []string{"false"},
		"state": []string{"active"},
	}.Encode()
	if err := h.getJSON(ctx, sessionPath, &sessions); err != nil {
		addGasCityUpstreamError(response, "/v0/city/{city}/sessions", err)
	} else {
		for _, session := range sessions.Items {
			response.Sessions = append(response.Sessions, session.summary(cityName))
		}
	}

	cityMail := status.Mail
	var mail GasCityMailCounts
	if err := h.getJSON(ctx, "/v0/city/"+escapedCity+"/mail/count", &mail); err != nil {
		addGasCityUpstreamError(response, "/v0/city/{city}/mail/count", err)
	} else {
		cityMail = mail
	}
	if statusOK || cityMail.Total != 0 || cityMail.Unread != 0 {
		response.Mail.Total += cityMail.Total
		response.Mail.Unread += cityMail.Unread
	}

	formulaPath := "/v0/city/" + escapedCity + "/formulas?" + url.Values{
		"scope_kind": []string{"city"},
		"scope_ref":  []string{cityName},
	}.Encode()
	var formulas gasCityFormulasResponse
	if err := h.getJSON(ctx, formulaPath, &formulas); err != nil {
		addGasCityUpstreamError(response, "/v0/city/{city}/formulas", err)
	} else {
		for _, formula := range formulas.Items {
			response.Formulas = append(response.Formulas, formula.summary(cityName))
		}
	}

	var convoys gasCityWorkItemsResponse
	if err := h.getJSON(ctx, "/v0/city/"+escapedCity+"/convoys", &convoys); err != nil {
		addGasCityUpstreamError(response, "/v0/city/{city}/convoys", err)
	} else {
		for _, convoy := range convoys.Items {
			response.Convoys = append(response.Convoys, convoy.summary(cityName))
		}
	}

	var beads gasCityWorkItemsResponse
	if err := h.getJSON(ctx, "/v0/city/"+escapedCity+"/beads?limit=50", &beads); err != nil {
		addGasCityUpstreamError(response, "/v0/city/{city}/beads", err)
		return
	}
	for _, bead := range beads.Items {
		item := bead.summary(cityName)
		switch bead.IssueType {
		case "molecule":
			response.Molecules = append(response.Molecules, item)
		case "wisp":
			response.Wisps = append(response.Wisps, item)
		}
		if item.RoutedTo != "" {
			response.Work.Routed++
		}
	}
}

type gasCityPoemSession struct {
	ID       string
	Alias    string
	Template string
	CityName string
}

type gasCityTranscriptSession struct {
	ID       string
	Alias    string
	Template string
	State    string
	CityName string
}

func (h *GasCityHandler) resolveGasCityPoemSession(ctx context.Context) (gasCityPoemSession, error) {
	var cities gasCityCitiesResponse
	if err := h.getJSON(ctx, "/v0/cities", &cities); err != nil {
		return gasCityPoemSession{}, errors.New("Gas City city list unavailable")
	}

	cityName := ""
	for _, city := range cities.Items {
		if !city.Running {
			continue
		}
		if filepath.Clean(city.Path) == h.config.CityDir {
			cityName = city.Name
			break
		}
	}
	if cityName == "" {
		return gasCityPoemSession{}, errors.New("configured Gas City is not running")
	}

	var sessions gasCitySessionsResponse
	sessionPath := "/v0/city/" + url.PathEscape(cityName) + "/sessions?" + url.Values{
		"limit": []string{"100"},
		"peek":  []string{"false"},
		"state": []string{"active"},
	}.Encode()
	if err := h.getJSON(ctx, sessionPath, &sessions); err != nil {
		return gasCityPoemSession{}, errors.New("Gas City session list unavailable")
	}

	var matches []gasCitySessionItem
	for _, session := range sessions.Items {
		if session.ID != h.config.PoemTarget && session.Alias != h.config.PoemTarget {
			continue
		}
		matches = append(matches, session)
	}
	if len(matches) == 0 {
		return gasCityPoemSession{}, errors.New("configured Pi target session is not active")
	}
	if len(matches) > 1 {
		return gasCityPoemSession{}, errors.New("configured Pi target session is ambiguous")
	}
	session := matches[0]
	if session.Template != h.config.PoemTemplate {
		return gasCityPoemSession{}, errors.New("configured Pi target session has unexpected template")
	}
	if !session.Running || session.State != "active" {
		return gasCityPoemSession{}, errors.New("configured Pi target session is not running")
	}
	if session.ID == "" {
		return gasCityPoemSession{}, errors.New("configured Pi target session has no stable id")
	}
	return gasCityPoemSession{
		ID:       session.ID,
		Alias:    session.Alias,
		Template: session.Template,
		CityName: cityName,
	}, nil
}

func (h *GasCityHandler) resolveGasCityTranscriptSession(ctx context.Context, sessionID string) (gasCityTranscriptSession, error) {
	var cities gasCityCitiesResponse
	if err := h.getJSON(ctx, "/v0/cities", &cities); err != nil {
		return gasCityTranscriptSession{}, errors.New("Gas City city list unavailable")
	}

	cityName := ""
	for _, city := range cities.Items {
		if !city.Running {
			continue
		}
		if filepath.Clean(city.Path) == h.config.CityDir {
			cityName = city.Name
			break
		}
	}
	if cityName == "" {
		return gasCityTranscriptSession{}, errGasCityConfiguredCityUnavailable
	}

	var sessions gasCitySessionsResponse
	sessionPath := "/v0/city/" + url.PathEscape(cityName) + "/sessions?" + url.Values{
		"limit": []string{"100"},
		"peek":  []string{"false"},
		"state": []string{"all"},
	}.Encode()
	if err := h.getJSON(ctx, sessionPath, &sessions); err != nil {
		return gasCityTranscriptSession{}, errGasCitySessionListUnavailable
	}

	var matches []gasCitySessionItem
	for _, session := range sessions.Items {
		if session.ID == sessionID {
			matches = append(matches, session)
		}
	}
	if len(matches) == 0 {
		return gasCityTranscriptSession{}, errGasCitySessionNotFound
	}
	if len(matches) > 1 {
		return gasCityTranscriptSession{}, errGasCitySessionAmbiguous
	}
	session := matches[0]
	return gasCityTranscriptSession{
		ID:       session.ID,
		Alias:    session.Alias,
		Template: session.Template,
		State:    session.State,
		CityName: cityName,
	}, nil
}

func (h *GasCityHandler) mailMessages(recipient string, limit int) ([]GasCityMailMessage, error) {
	storePath := filepath.Join(h.config.CityDir, ".gc", "beads.json")
	file, err := os.Open(storePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var store gasCityBeadsStore
	if err := json.NewDecoder(io.LimitReader(file, gasCityMailStoreLimit)).Decode(&store); err != nil {
		return nil, gasCityInvalidJSONError{}
	}

	messages := []GasCityMailMessage{}
	for i, bead := range store.Beads {
		if !isGasCityMailIssueType(bead.IssueType) {
			continue
		}
		messageRecipient := firstNonEmpty(
			bead.Assignee,
			metadataStringPath(bead.Metadata, "mail.to"),
			metadataStringPath(bead.Metadata, "mail.recipient"),
			metadataStringPath(bead.Metadata, "to"),
		)
		if messageRecipient != recipient {
			continue
		}
		body, truncated := truncateGasCityText(bead.Description, gasCityMailBodyLimit)
		messages = append(messages, GasCityMailMessage{
			ID:            bead.ID,
			From:          firstNonEmpty(metadataStringPath(bead.Metadata, "mail.from_display"), bead.From),
			Recipient:     messageRecipient,
			Subject:       bead.Title,
			Body:          body,
			BodyTruncated: truncated,
			Status:        bead.Status,
			IssueType:     bead.IssueType,
			Read:          metadataBoolPath(bead.Metadata, "mail.read"),
			FromSessionID: metadataStringPath(bead.Metadata, "mail.from_session_id"),
			CreatedAt:     bead.CreatedAt,
			UpdatedAt:     jsonStringValue(bead.UpdatedAt),
			order:         i,
		})
	}

	sort.SliceStable(messages, func(i, j int) bool {
		left, leftOK := parseGasCityTime(messages[i].CreatedAt)
		right, rightOK := parseGasCityTime(messages[j].CreatedAt)
		if leftOK && rightOK && !left.Equal(right) {
			return left.After(right)
		}
		if leftOK != rightOK {
			return leftOK
		}
		return messages[i].order > messages[j].order
	})
	if len(messages) > limit {
		messages = messages[:limit]
	}
	for i := range messages {
		messages[i].order = 0
	}
	return messages, nil
}

func parseGasCityMailLimit(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 20, nil
	}
	limit, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || limit < 1 || limit > gasCityMaxMailLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d", gasCityMaxMailLimit)
	}
	return limit, nil
}

func parseGasCityTranscriptLines(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return gasCityTranscriptDefaultLines, nil
	}
	lines, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || lines < 1 || lines > gasCityTranscriptMaxLines {
		return 0, fmt.Errorf("lines must be between 1 and %d", gasCityTranscriptMaxLines)
	}
	return lines, nil
}

func validateGasCitySessionID(raw string) (string, error) {
	sessionID := strings.TrimSpace(raw)
	if sessionID == "" {
		return "", errors.New("session id is required")
	}
	if !gasCitySessionIDPattern.MatchString(sessionID) {
		return "", errors.New("session id must be a stable Gas City gc-* id")
	}
	return sessionID, nil
}

func validateGasCityPoemTopic(raw string) (string, error) {
	topic := strings.TrimSpace(raw)
	if topic == "" {
		return "", errors.New("topic is required")
	}
	if len(topic) > gasCityPoemTopicLimit {
		return "", fmt.Errorf("topic must be %d characters or fewer", gasCityPoemTopicLimit)
	}
	if !gasCityPoemTopicPattern.MatchString(topic) {
		return "", errors.New("topic may use only letters, numbers, spaces, and .,!_?-")
	}
	return topic, nil
}

func buildGasCityPiPoemCommand(topic, nonce, subject, recipient string) string {
	prompt := fmt.Sprintf(
		"Write a two-line original poem about %s for Gas City mail. Include the exact nonce %s. Do not mention instructions or tools.",
		topic,
		nonce,
	)
	return strings.Join([]string{
		"!",
		"set -euo pipefail;",
		`tmp=$(mktemp);`,
		`trap 'rm -f "$tmp"' EXIT;`,
		"pi --no-tools --no-context-files --no-extensions --no-skills --no-prompt-templates --no-session --mode text --print " + shellSingleQuote(prompt) + ` > "$tmp";`,
		"gc mail send " + shellSingleQuote(recipient) + " -s " + shellSingleQuote(subject) + ` -m "$(cat "$tmp")"`,
	}, " ")
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func newGasCityNonce() string {
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "C4A-" + time.Now().UTC().Format("20060102-150405")
	}
	return "C4A-" + time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(random[:])
}

func sanitizeGasCityCLIOutput(output string) string {
	output = strings.TrimSpace(strings.ReplaceAll(output, "\x00", ""))
	truncated, _ := truncateGasCityText(output, gasCityPoemOutputLimit)
	return truncated
}

func sanitizeGasCityTranscriptOutput(output string) (string, bool) {
	output = strings.ReplaceAll(output, "\x00", "")
	output = gasCityANSIPattern.ReplaceAllString(output, "")
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")

	var builder strings.Builder
	builder.Grow(len(output))
	for _, r := range output {
		if r == '\n' || r == '\t' || (r >= 0x20 && r != 0x7f) {
			builder.WriteRune(r)
		}
	}
	return truncateGasCityText(builder.String(), gasCityTranscriptOutputLimit)
}

func countGasCityTranscriptLines(output string) int {
	output = strings.TrimRight(output, "\n")
	if output == "" {
		return 0
	}
	return strings.Count(output, "\n") + 1
}

func (h *GasCityHandler) recordAudit(entry gasCityAuditEntry) {
	h.auditMu.Lock()
	defer h.auditMu.Unlock()
	h.audit = append(h.audit, entry)
	if len(h.audit) > 20 {
		h.audit = h.audit[len(h.audit)-20:]
	}
}

func newGasCityObserverResponse(status string) GasCityObserverResponse {
	return GasCityObserverResponse{
		Status:       status,
		CheckedAt:    time.Now().UTC().Format(time.RFC3339),
		Cities:       []GasCityCitySummary{},
		Sessions:     []GasCitySessionSummary{},
		Formulas:     []GasCityFormulaSummary{},
		Molecules:    []GasCityWorkItem{},
		Wisps:        []GasCityWorkItem{},
		Convoys:      []GasCityWorkItem{},
		RecentEvents: []GasCityEventSummary{},
	}
}

func (h *GasCityHandler) getJSON(ctx context.Context, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.config.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return gasCityStatusError{status: resp.StatusCode}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(target); err != nil {
		return gasCityInvalidJSONError{}
	}
	return nil
}

func newGasCityHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

type gasCityStatusError struct {
	status int
}

func (e gasCityStatusError) Error() string {
	return fmt.Sprintf("upstream returned HTTP %d", e.status)
}

type gasCityInvalidJSONError struct{}

func (gasCityInvalidJSONError) Error() string {
	return "upstream returned invalid JSON"
}

func addGasCityUpstreamError(response *GasCityObserverResponse, route string, err error) {
	response.UpstreamErrors = append(response.UpstreamErrors, GasCityUpstreamError{
		Route:   route,
		Message: sanitizeGasCityUpstreamError(err),
	})
}

func sanitizeGasCityUpstreamError(err error) string {
	var statusErr gasCityStatusError
	if errors.As(err, &statusErr) {
		return statusErr.Error()
	}
	var invalidJSONErr gasCityInvalidJSONError
	if errors.As(err, &invalidJSONErr) {
		return invalidJSONErr.Error()
	}
	if os.IsTimeout(err) || errors.Is(err, context.DeadlineExceeded) {
		return "upstream service timed out"
	}
	return "upstream service unavailable"
}

func validateGasCityBaseURL(raw string) (string, string) {
	trimmed := cleanBaseURL(raw)
	if trimmed == "" {
		trimmed = defaultGasCityBaseURL
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "CHROTE_GASCITY_URL must be an http URL pointing to localhost or loopback"
	}
	if parsed.Scheme != "http" {
		return "", "CHROTE_GASCITY_URL must use http and point to localhost or loopback"
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "CHROTE_GASCITY_URL must not include credentials, query, or fragment values"
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", "CHROTE_GASCITY_URL must point to a supervisor root URL"
	}
	if port := parsed.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", "CHROTE_GASCITY_URL must include a valid TCP port"
		}
	}
	if !isGasCityLoopbackHost(parsed.Hostname()) {
		return "", "CHROTE_GASCITY_URL must point to localhost or loopback"
	}
	parsed.Path = ""
	return cleanBaseURL(parsed.String()), ""
}

func validateGasCityCityDir(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		trimmed = defaultGasCityCityDir
	}
	if strings.ContainsRune(trimmed, 0) {
		return "", "CHROTE_GASCITY_CITY_DIR must not contain NUL bytes"
	}
	if !filepath.IsAbs(trimmed) {
		return "", "CHROTE_GASCITY_CITY_DIR must be an absolute local path"
	}
	cleaned := filepath.Clean(trimmed)
	if cleaned == string(os.PathSeparator) {
		return "", "CHROTE_GASCITY_CITY_DIR must not point to the filesystem root"
	}
	return cleaned, ""
}

func validateGasCityIdentity(envName, raw string) (string, string) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", envName + " must not be empty"
	}
	if !gasCityIdentityPattern.MatchString(value) {
		return "", envName + " must contain only letters, numbers, dot, underscore, or dash"
	}
	return value, ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func isGasCityLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isGasCityMailIssueType(issueType string) bool {
	switch strings.ToLower(strings.TrimSpace(issueType)) {
	case "message", "mail":
		return true
	default:
		return false
	}
}

func parseGasCityTime(raw string) (time.Time, bool) {
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	return parsed, err == nil
}

func truncateGasCityText(value string, limit int) (string, bool) {
	if limit < 0 {
		limit = 0
	}
	if len([]rune(value)) <= limit {
		return value, false
	}
	var builder strings.Builder
	builder.Grow(limit)
	count := 0
	for _, r := range value {
		if count >= limit {
			break
		}
		builder.WriteRune(r)
		count++
	}
	return builder.String(), true
}

func sanitizeGasCityStoreError(err error) string {
	var invalidJSONErr gasCityInvalidJSONError
	if errors.As(err, &invalidJSONErr) {
		return "Gas City mail store returned invalid JSON"
	}
	return "Gas City mail store unavailable"
}

func metadataStringPath(metadata map[string]any, path string) string {
	if metadata == nil {
		return ""
	}
	if value, ok := metadata[path]; ok {
		return scalarString(value)
	}
	parts := strings.Split(path, ".")
	var value any = metadata
	for _, part := range parts {
		object, ok := value.(map[string]any)
		if !ok {
			return ""
		}
		value, ok = object[part]
		if !ok {
			return ""
		}
	}
	return scalarString(value)
}

func metadataBoolPath(metadata map[string]any, path string) bool {
	value := metadataStringPath(metadata, path)
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}

func scalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

func jsonStringValue(value any) string {
	if value == nil {
		return ""
	}
	return scalarString(value)
}

type gasCityBeadsStore struct {
	Beads []gasCityMailBead `json:"beads"`
}

type gasCityMailBead struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	IssueType   string         `json:"issue_type"`
	Status      string         `json:"status"`
	From        string         `json:"from"`
	Assignee    string         `json:"assignee"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   any            `json:"updated_at"`
	Metadata    map[string]any `json:"metadata"`
}

type gasCityHealthUpstream struct {
	Ready bool   `json:"ready"`
	Phase string `json:"phase"`
}

type gasCityHealthResponse struct {
	Status        string                `json:"status"`
	Version       string                `json:"version"`
	BuildID       string                `json:"build_id"`
	UptimeSeconds int64                 `json:"uptime_sec"`
	CitiesTotal   int                   `json:"cities_total"`
	CitiesRunning int                   `json:"cities_running"`
	Startup       gasCityHealthUpstream `json:"startup"`
}

func (h gasCityHealthResponse) summary() GasCityHealthSummary {
	return GasCityHealthSummary{
		Status:        h.Status,
		Version:       h.Version,
		BuildID:       h.BuildID,
		UptimeSeconds: h.UptimeSeconds,
		CitiesTotal:   h.CitiesTotal,
		CitiesRunning: h.CitiesRunning,
		StartupReady:  h.Startup.Ready,
		StartupPhase:  h.Startup.Phase,
	}
}

type gasCityCitiesResponse struct {
	Items []gasCityCityItem `json:"items"`
	Total int               `json:"total"`
}

type gasCityCityItem struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Running bool   `json:"running"`
	Status  string `json:"status"`
	Error   string `json:"error"`
}

type gasCityCityStatusResponse struct {
	Name    string            `json:"name"`
	Running int               `json:"running"`
	Work    gasCityWorkCounts `json:"work"`
	Mail    GasCityMailCounts `json:"mail"`
}

type gasCityWorkCounts struct {
	Open       int `json:"open"`
	Ready      int `json:"ready"`
	InProgress int `json:"in_progress"`
	Routed     int `json:"routed"`
	Molecules  int `json:"molecules"`
	Wisps      int `json:"wisps"`
	Convoys    int `json:"convoys"`
}

type gasCitySessionsResponse struct {
	Items []gasCitySessionItem `json:"items"`
	Total int                  `json:"total"`
}

type gasCitySessionItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Alias       string `json:"alias"`
	Template    string `json:"template"`
	State       string `json:"state"`
	Provider    string `json:"provider"`
	SessionName string `json:"session_name"`
	CreatedAt   string `json:"created_at"`
	LastActive  string `json:"last_active"`
	Running     bool   `json:"running"`
	Attached    bool   `json:"attached"`
}

func (s gasCitySessionItem) summary(cityName string) GasCitySessionSummary {
	return GasCitySessionSummary{
		City:        cityName,
		ID:          s.ID,
		Title:       s.Title,
		Alias:       s.Alias,
		Template:    s.Template,
		State:       s.State,
		Provider:    s.Provider,
		SessionName: s.SessionName,
		CreatedAt:   s.CreatedAt,
		LastActive:  s.LastActive,
		Running:     s.Running,
		Attached:    s.Attached,
	}
}

type gasCityFormulasResponse struct {
	Items []gasCityFormulaItem `json:"items"`
	Total int                  `json:"total"`
}

type gasCityFormulaItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	RunCount    int    `json:"run_count"`
}

func (f gasCityFormulaItem) summary(cityName string) GasCityFormulaSummary {
	return GasCityFormulaSummary{
		City:        cityName,
		Name:        f.Name,
		Description: f.Description,
		Version:     f.Version,
		RunCount:    f.RunCount,
	}
}

type gasCityWorkItemsResponse struct {
	Items []gasCityWorkItem `json:"items"`
	Total int               `json:"total"`
}

type gasCityWorkItem struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Status    string         `json:"status"`
	IssueType string         `json:"issue_type"`
	Ref       string         `json:"ref"`
	CreatedAt string         `json:"created_at"`
	Metadata  map[string]any `json:"metadata"`
}

func (w gasCityWorkItem) summary(cityName string) GasCityWorkItem {
	return GasCityWorkItem{
		City:      cityName,
		ID:        w.ID,
		Title:     w.Title,
		Status:    w.Status,
		IssueType: w.IssueType,
		Ref:       w.Ref,
		RoutedTo:  metadataString(w.Metadata, "gc.routed_to"),
		CreatedAt: w.CreatedAt,
	}
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

type gasCityEventsResponse struct {
	Items []gasCityEventItem `json:"items"`
	Total int                `json:"total"`
}

type gasCityEventItem struct {
	City    string `json:"city"`
	Seq     int64  `json:"seq"`
	Type    string `json:"type"`
	Time    string `json:"ts"`
	Actor   string `json:"actor"`
	Subject string `json:"subject"`
}

func (e gasCityEventItem) summary() GasCityEventSummary {
	return GasCityEventSummary{
		City:    e.City,
		Seq:     e.Seq,
		Type:    e.Type,
		Time:    e.Time,
		Actor:   e.Actor,
		Subject: e.Subject,
	}
}
