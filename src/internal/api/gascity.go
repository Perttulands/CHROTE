package api

import (
	"context"
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
	"strconv"
	"strings"
	"time"

	"github.com/chrote/server/internal/core"
)

const (
	defaultGasCityBaseURL         = "http://127.0.0.1:8372"
	defaultGasCityCityDir         = "/home/perttu/gascity"
	gasCityTranscriptDefaultLines = 120
	gasCityTranscriptMaxLines     = 500
	gasCityTranscriptOutputLimit  = 64 << 10
)

var (
	gasCitySessionIDPattern = regexp.MustCompile(`^gc-[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	gasCityANSIPattern      = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

	errGasCityConfiguredCityUnavailable = errors.New("configured Gas City is not running")
	errGasCitySessionListUnavailable    = errors.New("Gas City session list unavailable")
	errGasCitySessionNotFound           = errors.New("Gas City session not found")
	errGasCitySessionAmbiguous          = errors.New("Gas City session id is ambiguous")
)

// GasCityConfig is the server-side configuration for CHROTE's bounded Gas City surface.
type GasCityConfig struct {
	BaseURL string
	CityDir string
	// TranscriptArchiveDir is the CHROTE-owned directory where successful
	// session peeks are archived so transcripts survive a supervisor restart.
	// Empty disables archiving (live peek only).
	TranscriptArchiveDir string
	// GCExtraPath is prepended to PATH for gc CLI invocations so gc resolves the
	// same tmux binary the Gas City supervisor used to create session sockets
	// (the service PATH otherwise picks an incompatible older tmux).
	GCExtraPath string
}

// GasCityObserverResponse is a CHROTE-safe read model of the local Gas City supervisor.
type GasCityObserverResponse struct {
	Status         string                  `json:"status"`
	CheckedAt      string                  `json:"checkedAt"`
	Error          string                  `json:"error,omitempty"`
	Sessions       []GasCitySessionSummary `json:"sessions"`
	UpstreamErrors []GasCityUpstreamError  `json:"upstreamErrors,omitempty"`
}

type GasCitySessionSummary struct {
	Source       string `json:"source"`
	City         string `json:"city"`
	ID           string `json:"id"`
	Name         string `json:"name,omitempty"`
	Title        string `json:"title,omitempty"`
	Alias        string `json:"alias,omitempty"`
	Template     string `json:"template,omitempty"`
	Status       string `json:"status,omitempty"`
	State        string `json:"state,omitempty"`
	AttachTarget string `json:"attachTarget,omitempty"`
	CreatedAt    string `json:"createdAt,omitempty"`
	LastActive   string `json:"lastActive,omitempty"`
	Running      bool   `json:"running"`
	Attached     bool   `json:"attached"`
}

type GasCityUpstreamError struct {
	Route   string `json:"route"`
	Message string `json:"message"`
}

type GasCityTranscriptResponse struct {
	// Source is gc-session-peek for a fresh live peek, or chrote-archive when
	// the live peek was unavailable and CHROTE served the last archived peek.
	Source string `json:"source"`
	// Stale is true when the response is served from the archive rather than a
	// fresh live peek (for example after a supervisor restart).
	Stale     bool   `json:"stale"`
	SessionID string `json:"sessionId"`
	Alias     string `json:"alias,omitempty"`
	Template  string `json:"template,omitempty"`
	State     string `json:"state,omitempty"`
	City      string `json:"city,omitempty"`
	Lines     int    `json:"lines"`
	LineCount int    `json:"lineCount"`
	// CapturedAt is the archive capture time (RFC3339) when Source is
	// chrote-archive; empty for a fresh live peek.
	CapturedAt string `json:"capturedAt,omitempty"`
	Transcript string `json:"transcript"`
	Truncated  bool   `json:"truncated"`
}

// GasCityHandler handles CHROTE's bounded Gas City observer and transcript routes.
type GasCityHandler struct {
	config             GasCityConfig
	client             *http.Client
	configError        string
	controlConfigError string
	runner             gasCityCommandRunner
	transcriptArchive  *gasCityTranscriptArchive
}

type gasCityCommandRunner interface {
	Run(ctx context.Context, name string, args []string) (string, error)
}

// gasCityExecRunner runs the gc CLI (gc shells out to tmux itself via PATH; this
// runner never execs tmux directly). extraPath is prepended to the child PATH so
// gc resolves a tmux build compatible with the Gas City server's sockets. The
// chrote.service PATH resolves tmux to an older /usr/bin/tmux that cannot read
// the supervisor's newer tmux server, so without this `gc session peek` fails
// and the transcript route 502s. See resolveGasCityGCExtraPath for the details.
type gasCityExecRunner struct {
	extraPath string
}

func (r gasCityExecRunner) Run(ctx context.Context, name string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = gasCityChildEnv(r.extraPath)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// gasCityChildEnv returns the parent environment with extraPath prepended to
// PATH. extraPath entries already present in PATH are not duplicated.
func gasCityChildEnv(extraPath string) []string {
	env := os.Environ()
	extraPath = strings.TrimSpace(extraPath)
	if extraPath == "" {
		return env
	}
	current := os.Getenv("PATH")
	merged := mergePathPrepend(extraPath, current)
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			out = append(out, "PATH="+merged)
			replaced = true
			continue
		}
		out = append(out, kv)
	}
	if !replaced {
		out = append(out, "PATH="+merged)
	}
	return out
}

// mergePathPrepend prepends prefix path entries to base, skipping any that are
// already present so PATH does not grow unbounded across restarts.
func mergePathPrepend(prefix, base string) string {
	seen := map[string]bool{}
	var ordered []string
	add := func(list string) {
		for _, p := range strings.Split(list, string(os.PathListSeparator)) {
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			ordered = append(ordered, p)
		}
	}
	add(prefix)
	add(base)
	return strings.Join(ordered, string(os.PathListSeparator))
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
	return GasCityConfig{
		BaseURL:              baseURL,
		CityDir:              cityDir,
		TranscriptArchiveDir: resolveGasCityTranscriptArchiveDir(),
		GCExtraPath:          resolveGasCityGCExtraPath(),
	}
}

// gasCityServiceTmux is the tmux the minimal chrote.service PATH resolves to.
// It is tmux 3.4 here and cannot read the Gas City server's 3.6a sockets, so a
// candidate tmux dir is only useful if it holds a DIFFERENT tmux than this one.
const gasCityServiceTmux = "/usr/bin/tmux"

// gasCityTmuxCandidateDirs are bin dirs that commonly hold a newer tmux than the
// service PATH's /usr/bin/tmux (e.g. the Linuxbrew tmux 3.6a the Gas City
// supervisor uses). Ordered by preference.
var gasCityTmuxCandidateDirs = []string{
	"/home/linuxbrew/.linuxbrew/bin",
	"/usr/local/bin",
}

// resolveGasCityGCExtraPath returns PATH entries to prepend for gc invocations
// so gc resolves the same tmux build the Gas City supervisor used to create the
// session sockets.
//
// ROOT CAUSE this addresses: the chrote.service runs with a minimal PATH whose
// tmux is /usr/bin/tmux 3.4, but the supervisor created the `-L gascity` server
// with Linuxbrew tmux 3.6a. tmux 3.4 cannot read a 3.6a server ("server exited
// unexpectedly"), so a PATH-resolved `gc session peek` fails and the route 502s.
// Prepending the dir of a compatible tmux fixes it for gc subprocesses only,
// without touching CHROTE's own /usr/bin/tmux terminal-proxy sessions.
//
// CHROTE_GASCITY_GC_PATH overrides the result ("off" adds nothing). Otherwise we
// pick the first candidate dir that holds a tmux executable distinct from the
// service tmux. This is a deliberate version dependency: if the supervisor's
// tmux moves, set CHROTE_GASCITY_GC_PATH to its bin dir.
func resolveGasCityGCExtraPath() string {
	configured := strings.TrimSpace(os.Getenv("CHROTE_GASCITY_GC_PATH"))
	if strings.EqualFold(configured, "off") {
		return ""
	}
	if configured != "" {
		return configured
	}
	for _, dir := range gasCityTmuxCandidateDirs {
		tmuxPath := filepath.Join(dir, "tmux")
		if tmuxPath == gasCityServiceTmux {
			continue
		}
		if info, err := os.Stat(tmuxPath); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return dir
		}
	}
	return ""
}

// resolveGasCityTranscriptArchiveDir picks the CHROTE-owned transcript archive
// directory. CHROTE_GASCITY_TRANSCRIPT_DIR overrides it; the value "off" (any
// case) disables archiving. The default is under XDG_STATE_HOME (or
// ~/.local/state), keeping the archive out of the Gas City runtime tree so Gas
// City never becomes the durable owner of CHROTE recovery data.
func resolveGasCityTranscriptArchiveDir() string {
	configured := strings.TrimSpace(os.Getenv("CHROTE_GASCITY_TRANSCRIPT_DIR"))
	if strings.EqualFold(configured, "off") {
		return ""
	}
	if configured != "" {
		return configured
	}
	base := strings.TrimSpace(os.Getenv("XDG_STATE_HOME"))
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			return ""
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "chrote", "gascity-transcripts")
}

// NewGasCityHandler creates a Gas City handler with production defaults.
func NewGasCityHandler(config GasCityConfig) *GasCityHandler {
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = defaultGasCityBaseURL
	}
	if strings.TrimSpace(config.CityDir) == "" {
		config.CityDir = defaultGasCityCityDir
	}
	baseURL, configError := validateGasCityBaseURL(config.BaseURL)
	cityDir, cityDirError := validateGasCityCityDir(config.CityDir)
	extraPath := strings.TrimSpace(config.GCExtraPath)
	return &GasCityHandler{
		config: GasCityConfig{
			BaseURL:              baseURL,
			CityDir:              cityDir,
			TranscriptArchiveDir: strings.TrimSpace(config.TranscriptArchiveDir),
			GCExtraPath:          extraPath,
		},
		client:             newGasCityHTTPClient(),
		configError:        configError,
		controlConfigError: cityDirError,
		runner:             gasCityExecRunner{extraPath: extraPath},
		transcriptArchive:  newGasCityTranscriptArchive(strings.TrimSpace(config.TranscriptArchiveDir)),
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
	mux.HandleFunc("GET /api/gascity/sessions/{id}/transcript", h.Transcript)
}

// Observer handles GET /api/gascity/observer.
func (h *GasCityHandler) Observer(w http.ResponseWriter, r *http.Request) {
	core.WriteSuccess(w, h.snapshot(r.Context()))
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
		// The supervisor could not resolve the session (e.g. it was restarted,
		// is down, or pruned the session). Recover the last archived peek so an
		// operator still sees the most recent Gas City-owned output.
		if snapshot, ok := h.transcriptArchive.load(sessionID); ok {
			core.WriteSuccess(w, gasCityArchiveTranscriptResponse(snapshot, lines))
			return
		}
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
		if snapshot, ok := h.transcriptArchive.load(session.ID); ok {
			core.WriteSuccess(w, gasCityArchiveTranscriptResponse(snapshot, lines))
			return
		}
		core.WriteError(w, http.StatusBadGateway, "GASCITY_TRANSCRIPT_UNAVAILABLE", "Gas City transcript peek failed")
		return
	}

	transcript, truncated := sanitizeGasCityTranscriptOutput(output)
	// A live peek with no content (e.g. tmux pane recreated empty after a
	// supervisor restart) should still surface the last archived transcript
	// rather than a misleading empty pane.
	if strings.TrimSpace(transcript) == "" {
		if snapshot, ok := h.transcriptArchive.load(session.ID); ok && strings.TrimSpace(snapshot.Transcript) != "" {
			core.WriteSuccess(w, gasCityArchiveTranscriptResponse(snapshot, lines))
			return
		}
	}

	live := GasCityTranscriptResponse{
		Source:     "gc-session-peek",
		Stale:      false,
		SessionID:  session.ID,
		Alias:      session.Alias,
		Template:   session.Template,
		State:      session.State,
		City:       session.CityName,
		Lines:      lines,
		LineCount:  countGasCityTranscriptLines(transcript),
		Transcript: transcript,
		Truncated:  truncated,
	}

	// Archive the fresh, already-sanitized peek so it survives a restart.
	// Best-effort: never fail the live response on an archive write error.
	if strings.TrimSpace(transcript) != "" {
		if err := h.transcriptArchive.save(gasCityTranscriptSnapshot{
			SessionID:  live.SessionID,
			Alias:      live.Alias,
			Template:   live.Template,
			State:      live.State,
			City:       live.City,
			Lines:      live.Lines,
			LineCount:  live.LineCount,
			Transcript: live.Transcript,
			Truncated:  live.Truncated,
		}); err != nil {
			log.Printf("gascity transcript archive write failed session=%s: %v", live.SessionID, err)
		}
	}

	core.WriteSuccess(w, live)
}

// gasCityArchiveTranscriptResponse builds a stale, archive-sourced transcript
// response from a stored snapshot. lines is the request bound used for display.
func gasCityArchiveTranscriptResponse(snapshot gasCityTranscriptSnapshot, lines int) GasCityTranscriptResponse {
	return GasCityTranscriptResponse{
		Source:     "chrote-archive",
		Stale:      true,
		SessionID:  snapshot.SessionID,
		Alias:      snapshot.Alias,
		Template:   snapshot.Template,
		State:      snapshot.State,
		City:       snapshot.City,
		Lines:      lines,
		LineCount:  snapshot.LineCount,
		CapturedAt: snapshot.CapturedAt,
		Transcript: snapshot.Transcript,
		Truncated:  snapshot.Truncated,
	}
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
		if city.Name == "" || !city.Running {
			continue
		}
		h.addCitySnapshot(ctx, city.Name, &response)
	}

	if len(response.UpstreamErrors) > 0 && response.Status == "ok" {
		response.Status = "degraded"
		response.Error = "Some Gas City session metadata is unavailable"
	}
	return response
}

func (h *GasCityHandler) addCitySnapshot(ctx context.Context, cityName string, response *GasCityObserverResponse) {
	escapedCity := url.PathEscape(cityName)

	var sessions gasCitySessionsResponse
	sessionPath := "/v0/city/" + escapedCity + "/sessions?" + url.Values{
		"limit": []string{"100"},
		"peek":  []string{"false"},
		"state": []string{"all"},
	}.Encode()
	if err := h.getJSON(ctx, sessionPath, &sessions); err != nil {
		addGasCityUpstreamError(response, "/v0/city/{city}/sessions", err)
	} else {
		for _, session := range sessions.Items {
			response.Sessions = append(response.Sessions, session.summary(cityName))
		}
	}
}

type gasCityTranscriptSession struct {
	ID       string
	Alias    string
	Template string
	State    string
	CityName string
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

func newGasCityObserverResponse(status string) GasCityObserverResponse {
	return GasCityObserverResponse{
		Status:    status,
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
		Sessions:  []GasCitySessionSummary{},
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
	attachTarget := ""
	if strings.TrimSpace(s.ID) != "" {
		attachTarget = "gc:" + strings.TrimSpace(s.ID)
	}
	return GasCitySessionSummary{
		Source:       "gascity",
		City:         cityName,
		ID:           s.ID,
		Name:         firstNonEmpty(s.Alias, s.Title, s.ID),
		Title:        s.Title,
		Alias:        s.Alias,
		Template:     s.Template,
		Status:       s.State,
		State:        s.State,
		AttachTarget: attachTarget,
		CreatedAt:    s.CreatedAt,
		LastActive:   s.LastActive,
		Running:      s.Running,
		Attached:     s.Attached,
	}
}
