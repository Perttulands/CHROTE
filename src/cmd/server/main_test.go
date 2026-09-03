package main

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chrote/server/internal/api"
)

func TestRuntimeRoutesCanDisableSystemHistorySampler(t *testing.T) {
	called := false
	original := startDefaultSystemHistorySampler
	startDefaultSystemHistorySampler = func(*api.SystemHandler, context.Context) context.CancelFunc {
		called = true
		return func() {}
	}
	t.Cleanup(func() { startDefaultSystemHistorySampler = original })
	tmuxLog := filepath.Join(t.TempDir(), "tmux-calls.log")
	fakeTmux := filepath.Join(t.TempDir(), "tmux")
	if err := os.WriteFile(tmuxLog, nil, 0o600); err != nil {
		t.Fatalf("create tmux call log: %v", err)
	}
	if err := os.WriteFile(fakeTmux, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$TMUX_CALL_LOG\"\n"), 0o700); err != nil {
		t.Fatalf("create fake tmux: %v", err)
	}
	t.Setenv("CHROTE_TMUX_BIN", fakeTmux)
	t.Setenv("TMUX_CALL_LOG", tmuxLog)

	mux := http.NewServeMux()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, stopRuntimeMaintenance := registerRuntimeRoutes(mux, Config{
		StartSystemHistory: false,
	}, ctx)
	<-ctx.Done()
	stopRuntimeMaintenance()

	if called {
		t.Fatal("disabled system history started the host sampler")
	}
	if raw, err := os.ReadFile(tmuxLog); err != nil {
		t.Fatalf("read tmux call log: %v", err)
	} else if len(raw) != 0 {
		t.Fatalf("runtime routes dispatched background tmux calls; nothing should sweep tmux on its own: %q", raw)
	}
}

func TestCORSMiddlewareAllowsOnlyExactConfiguredOrigins(t *testing.T) {
	handler := corsMiddleware([]string{"https://app.example", "https://admin.example"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	allowedReq := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	allowedReq.Header.Set("Origin", "https://app.example")
	allowedRec := httptest.NewRecorder()
	handler.ServeHTTP(allowedRec, allowedReq)

	if got := allowedRec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("allowed Access-Control-Allow-Origin = %q, want exact requesting origin", got)
	}

	disallowedReq := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
	disallowedReq.Header.Set("Origin", "https://evil.example")
	disallowedReq.Header.Set("Access-Control-Request-Method", http.MethodGet)
	disallowedRec := httptest.NewRecorder()
	handler.ServeHTTP(disallowedRec, disallowedReq)

	if got := disallowedRec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("disallowed preflight Access-Control-Allow-Origin = %q, want no CORS grant", got)
	}
	if got := disallowedRec.Header().Get("Access-Control-Allow-Methods"); got != "" {
		t.Fatalf("disallowed preflight Access-Control-Allow-Methods = %q, want no CORS grant", got)
	}

	t.Run("configuring no origins grants none", func(t *testing.T) {
		defaultHandler := corsMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodGet, "/api/services", nil)
		req.Header.Set("Origin", "https://evil.example")
		rec := httptest.NewRecorder()
		defaultHandler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("Access-Control-Allow-Origin = %q, want no CORS header by default", got)
		}
	})
}

// The removed access-token setting is still read, so an operator who kept it in
// a unit file learns it does nothing rather than believing CHROTE is protected.
func TestRemovedAPIAuthTokenEmitsMigrationWarningWithoutLeakingValue(t *testing.T) {
	const staleValue = "stale-owner-value"
	t.Setenv("API_AUTH_TOKEN", staleValue)

	var output bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previousOutput) })

	warnRemovedAccessTokenSetting()

	message := output.String()
	if !strings.Contains(message, "API_AUTH_TOKEN is no longer supported") || !strings.Contains(message, "does not protect CHROTE") {
		t.Fatalf("migration warning was not explicit: %q", message)
	}
	if strings.Contains(message, staleValue) {
		t.Fatal("migration warning leaked the removed token value")
	}
}

func TestResponseWriterAllowsResponseControllerWriteDeadline(t *testing.T) {
	underlying := &deadlineAwareResponseWriter{header: make(http.Header)}
	wrapped := &responseWriter{ResponseWriter: underlying, status: http.StatusOK}

	if err := http.NewResponseController(wrapped).SetWriteDeadline(time.Time{}); err != nil {
		t.Fatalf("SetWriteDeadline through responseWriter returned error: %v", err)
	}
	if !underlying.writeDeadlineSet {
		t.Fatal("underlying response writer did not receive write deadline reset")
	}
	if !underlying.writeDeadline.IsZero() {
		t.Fatalf("write deadline = %v, want zero time", underlying.writeDeadline)
	}
}

type deadlineAwareResponseWriter struct {
	header           http.Header
	writeDeadline    time.Time
	writeDeadlineSet bool
}

func (w *deadlineAwareResponseWriter) Header() http.Header {
	return w.header
}

func (w *deadlineAwareResponseWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (w *deadlineAwareResponseWriter) WriteHeader(statusCode int) {}

func (w *deadlineAwareResponseWriter) SetWriteDeadline(deadline time.Time) error {
	w.writeDeadline = deadline
	w.writeDeadlineSet = true
	return nil
}
