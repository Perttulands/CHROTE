package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCORSMiddlewareDefaultDoesNotSetAllowOrigin(t *testing.T) {
	handler := corsMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/services", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want no CORS header by default", got)
	}
}

func TestCORSMiddlewareAllowsOnlyExactConfiguredOrigins(t *testing.T) {
	handler := corsMiddleware([]string{"https://app.example", "https://admin.example"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	allowedReq := httptest.NewRequest(http.MethodGet, "/api/services", nil)
	allowedReq.Header.Set("Origin", "https://app.example")
	allowedRec := httptest.NewRecorder()
	handler.ServeHTTP(allowedRec, allowedReq)

	if got := allowedRec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("allowed Access-Control-Allow-Origin = %q, want exact requesting origin", got)
	}

	disallowedReq := httptest.NewRequest(http.MethodOptions, "/api/services", nil)
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
