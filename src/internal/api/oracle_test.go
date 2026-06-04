package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestParseAgentStatus_Idle(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{"empty output", "", AgentStatusIdle},
		{"dollar prompt", "some output\n$ ", AgentStatusIdle},
		{"hash prompt", "root@host:~# ", AgentStatusIdle},
		{"chevron prompt", "claude ❯ ", AgentStatusIdle},
		{"angle bracket prompt", "prompt> ", AgentStatusIdle},
		{"only whitespace", "   \n  \n  ", AgentStatusIdle},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAgentStatus(tt.output)
			if got != tt.want {
				t.Errorf("ParseAgentStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseAgentStatus_Working(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"active output", "Processing file main.go...\nAnalyzing dependencies"},
		{"mid-line output", "Running tests"},
		{"progress indicator", "Building project [=====>    ] 50%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAgentStatus(tt.output)
			if got != AgentStatusWorking {
				t.Errorf("ParseAgentStatus() = %q, want %q", got, AgentStatusWorking)
			}
		})
	}
}

func TestParseAgentStatus_Complete(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"task complete", "All done.\nTask complete"},
		{"completed successfully", "Build completed successfully"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAgentStatus(tt.output)
			if got != AgentStatusComplete {
				t.Errorf("ParseAgentStatus() = %q, want %q", got, AgentStatusComplete)
			}
		})
	}
}

func TestParseAgentStatus_Error(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"error line", "error: compilation failed"},
		{"fatal error", "fatal: repository not found"},
		{"panic", "panic: runtime error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseAgentStatus(tt.output)
			if got != AgentStatusError {
				t.Errorf("ParseAgentStatus() = %q, want %q", got, AgentStatusError)
			}
		})
	}
}

func TestExtractContextPercent(t *testing.T) {
	re := regexp.MustCompile(`(\d+)%\s*(?:context|of context)`)

	tests := []struct {
		name   string
		output string
		want   int
	}{
		{"no context", "some random output", 0},
		{"simple percent", "Using 45% context remaining", 45},
		{"of context", "73% of context used", 73},
		{"multiple values picks last", "10% context\n85% context", 85},
		{"100 percent", "100% context exhausted", 100},
		{"over 100 clamped", "150% context somehow", 100},
		{"zero percent", "0% context used", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractContextPercent(tt.output, re)
			if got != tt.want {
				t.Errorf("ExtractContextPercent() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestExtractBeadID(t *testing.T) {
	re := regexp.MustCompile(`(?:bead|bd|mol)-([a-z0-9]{3,})`)

	tests := []struct {
		name   string
		output string
		want   string
	}{
		{"no bead", "random output here", ""},
		{"bead prefix", "Working on bead-abc123", "abc123"},
		{"bd prefix", "Assigned bd-xyz789", "xyz789"},
		{"mol prefix", "Processing mol-def456", "def456"},
		{"multiple picks last", "bead-first\nmol-last123", "last123"},
		{"too short id", "bead-ab", ""},
		{"embedded in text", "status: bead-quick99 is active", "quick99"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractBeadID(tt.output, re)
			if got != tt.want {
				t.Errorf("ExtractBeadID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractLastLines(t *testing.T) {
	tests := []struct {
		name   string
		output string
		n      int
		want   int // expected count
	}{
		{"empty", "", 5, 0},
		{"few lines", "line1\nline2\nline3", 5, 3},
		{"more than n", "a\nb\nc\nd\ne\nf\ng", 3, 3},
		{"with empty lines", "a\n\nb\n\nc\n\n", 5, 3},
		{"all empty", "\n\n\n", 5, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractLastLines(tt.output, tt.n)
			if len(got) != tt.want {
				t.Errorf("extractLastLines() returned %d lines, want %d", len(got), tt.want)
			}
		})
	}
}

func TestExtractLastLines_Order(t *testing.T) {
	output := "first\nsecond\nthird\nfourth\nfifth"
	got := extractLastLines(output, 3)
	if len(got) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(got))
	}
	if got[0] != "third" || got[1] != "fourth" || got[2] != "fifth" {
		t.Errorf("expected [third, fourth, fifth], got %v", got)
	}
}

func TestIsAgentSession(t *testing.T) {
	tests := []struct {
		name    string
		session string
		want    bool
	}{
		{"claude agent", "claude-opus-1", true},
		{"claude prefix", "claude-test", true},
		{"codex session", "codex", true},
		{"codex with suffix", "codex-run", true},
		{"regular shell", "shell-abc", false},
		{"main session", "main", false},
		{"chrote chat", "chrote-chat", false},
		{"hq session", "hq-monitor", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAgentSession(tt.session)
			if got != tt.want {
				t.Errorf("isAgentSession(%q) = %v, want %v", tt.session, got, tt.want)
			}
		})
	}
}

func TestOracleHandlerLiveAgentSessionsUsesUnfilteredSessionNames(t *testing.T) {
	argsPath := installLiveAgentSessionsTmux(t)
	handler := NewOracleHandler(NewTmuxHandler(), NewBeadsHandler())
	defer handler.Stop()

	live, err := handler.LiveAgentSessions()

	if err != nil {
		t.Fatalf("LiveAgentSessions error = %v", err)
	}
	wantNames := []string{"susie", "scratch", "codex-run"}
	gotNames := make([]string, 0, len(live))
	for _, session := range live {
		gotNames = append(gotNames, session.Name)
		if session.Status != "live" {
			t.Fatalf("session %q status = %q, want live", session.Name, session.Status)
		}
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("live session names = %#v, want %#v", gotNames, wantNames)
	}
	if !live[0].Attached || live[1].Attached || live[2].Attached {
		t.Fatalf("attached flags = %#v, want only susie attached", live)
	}
	if gotCalls := readLines(t, argsPath); !reflect.DeepEqual(gotCalls, []string{"list-sessions -F #{session_name}:#{session_attached}"}) {
		t.Fatalf("tmux calls = %#v, want only unfiltered list-sessions", gotCalls)
	}
}

func TestOracleHandler_New(t *testing.T) {
	tmux := NewTmuxHandler()
	beads := NewBeadsHandler()
	handler := NewOracleHandler(tmux, beads)
	defer handler.Stop()

	if handler == nil {
		t.Fatal("NewOracleHandler() returned nil")
	}
	if handler.tmuxHandler == nil {
		t.Error("tmuxHandler is nil")
	}
	if handler.beadsHandler == nil {
		t.Error("beadsHandler is nil")
	}
	if handler.contextRegex == nil {
		t.Error("contextRegex is nil")
	}
	if handler.beadRegex == nil {
		t.Error("beadRegex is nil")
	}
	if handler.clients == nil {
		t.Error("clients map is nil")
	}
}

func installLiveAgentSessionsTmux(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	argsPath := filepath.Join(dir, "tmux-args.txt")
	scriptPath := filepath.Join(dir, "tmux")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$TMUX_ARGS_FILE"
case "$*" in
  "list-sessions -F #{session_name}:#{session_attached}")
    printf 'susie:1\nscratch:0\ncodex-run:0\n'
    ;;
  *)
    printf 'unexpected tmux call: %s\n' "$*" >&2
    exit 9
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
	return argsPath
}

func readLines(t *testing.T, path string) []string {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read lines: %v", err)
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func TestOracleHandler_RegisterRoutes(t *testing.T) {
	tmux := NewTmuxHandler()
	beads := NewBeadsHandler()
	handler := NewOracleHandler(tmux, beads)
	defer handler.Stop()

	mux := http.NewServeMux()
	// Should not panic
	handler.RegisterRoutes(mux)
}

func TestOracleHandler_GetStatus_ReturnsValidJSON(t *testing.T) {
	tmux := NewTmuxHandler()
	beads := NewBeadsHandler()
	handler := NewOracleHandler(tmux, beads)
	defer handler.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/oracle/status", nil)
	recorder := httptest.NewRecorder()

	handler.GetStatus(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusOK)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("Response is not valid JSON: %v", err)
	}

	if response["success"] != true {
		t.Error("Response should indicate success")
	}
}

func TestOracleHandler_GetAgents_ReturnsValidJSON(t *testing.T) {
	tmux := NewTmuxHandler()
	beads := NewBeadsHandler()
	handler := NewOracleHandler(tmux, beads)
	defer handler.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/oracle/agents", nil)
	recorder := httptest.NewRecorder()

	handler.GetAgents(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusOK)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("Response is not valid JSON: %v", err)
	}

	if response["success"] != true {
		t.Error("Response should indicate success")
	}
}

func TestOracleHandler_GetRalph_ReturnsValidJSON(t *testing.T) {
	tmux := NewTmuxHandler()
	beads := NewBeadsHandler()
	handler := NewOracleHandler(tmux, beads)
	defer handler.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/oracle/ralph", nil)
	recorder := httptest.NewRecorder()

	handler.GetRalph(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusOK)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("Response is not valid JSON: %v", err)
	}

	if response["success"] != true {
		t.Error("Response should indicate success")
	}
}

func TestSSEBroadcaster_SubscribeUnsubscribe(t *testing.T) {
	tmux := NewTmuxHandler()
	beads := NewBeadsHandler()
	handler := NewOracleHandler(tmux, beads)
	defer handler.Stop()

	ch := make(chan OracleEvent, 10)

	handler.subscribe(ch)

	handler.mu.RLock()
	if len(handler.clients) != 1 {
		t.Errorf("Expected 1 client, got %d", len(handler.clients))
	}
	handler.mu.RUnlock()

	handler.unsubscribe(ch)

	handler.mu.RLock()
	if len(handler.clients) != 0 {
		t.Errorf("Expected 0 clients, got %d", len(handler.clients))
	}
	handler.mu.RUnlock()
}

func TestSSEBroadcaster_Broadcast(t *testing.T) {
	tmux := NewTmuxHandler()
	beads := NewBeadsHandler()
	handler := NewOracleHandler(tmux, beads)
	defer handler.Stop()

	ch := make(chan OracleEvent, 10)
	handler.subscribe(ch)
	defer handler.unsubscribe(ch)

	event := OracleEvent{
		Type:      "test_event",
		Data:      map[string]string{"key": "value"},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	handler.broadcast(event)

	select {
	case received := <-ch:
		if received.Type != "test_event" {
			t.Errorf("Event type = %q, want %q", received.Type, "test_event")
		}
	case <-time.After(time.Second):
		t.Error("Timed out waiting for broadcast event")
	}
}

func TestSSEBroadcaster_SlowClientDropped(t *testing.T) {
	tmux := NewTmuxHandler()
	beads := NewBeadsHandler()
	handler := NewOracleHandler(tmux, beads)
	defer handler.Stop()

	// Channel with buffer of 1
	ch := make(chan OracleEvent, 1)
	handler.subscribe(ch)
	defer handler.unsubscribe(ch)

	// Fill the buffer
	handler.broadcast(OracleEvent{Type: "first", Timestamp: time.Now().UTC().Format(time.RFC3339)})

	// This should not block even though buffer is full
	handler.broadcast(OracleEvent{Type: "dropped", Timestamp: time.Now().UTC().Format(time.RFC3339)})

	// Should only get the first event
	received := <-ch
	if received.Type != "first" {
		t.Errorf("Expected first event, got %q", received.Type)
	}
}
