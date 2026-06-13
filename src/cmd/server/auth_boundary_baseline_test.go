package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthMiddlewareBoundaryTruthTable(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		method      string
		path        string
		authHeader  string
		origin      string
		preflight   string
		wantStatus  int
		wantReached bool
	}{
		{
			name:        "no configured token bypasses api route",
			path:        "/api/services",
			wantStatus:  http.StatusNoContent,
			wantReached: true,
		},
		{
			name:       "missing bearer on api route",
			token:      "secret-token",
			path:       "/api/services",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "malformed authorization on api route",
			token:      "secret-token",
			path:       "/api/services",
			authHeader: "Basic secret-token",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong bearer on api route",
			token:      "secret-token",
			path:       "/api/services",
			authHeader: "Bearer wrong-token",
			wantStatus: http.StatusForbidden,
		},
		{
			name:        "correct bearer on api route",
			token:       "secret-token",
			path:        "/api/services",
			authHeader:  "Bearer secret-token",
			wantStatus:  http.StatusNoContent,
			wantReached: true,
		},
		{
			name:        "health bypasses configured token",
			token:       "secret-token",
			path:        "/api/health",
			wantStatus:  http.StatusNoContent,
			wantReached: true,
		},
		{
			name:        "terminal route bypasses configured token",
			token:       "secret-token",
			path:        "/terminal/ws",
			wantStatus:  http.StatusNoContent,
			wantReached: true,
		},
		{
			name:        "dashboard route bypasses configured token",
			token:       "secret-token",
			path:        "/",
			wantStatus:  http.StatusNoContent,
			wantReached: true,
		},
		{
			name:        "browser preflight bypasses configured token",
			token:       "secret-token",
			method:      http.MethodOptions,
			path:        "/api/services/context/docs",
			origin:      "https://app.example",
			preflight:   http.MethodGet,
			wantStatus:  http.StatusNoContent,
			wantReached: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reached = true
				w.WriteHeader(http.StatusNoContent)
			})
			handler := authMiddleware(tt.token)(next)
			method := tt.method
			if method == "" {
				method = http.MethodGet
			}
			req := httptest.NewRequest(method, tt.path, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.preflight != "" {
				req.Header.Set("Access-Control-Request-Method", tt.preflight)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if reached != tt.wantReached {
				t.Fatalf("protected handler reached = %v, want %v", reached, tt.wantReached)
			}
		})
	}
}

func TestAuthMiddlewareWrongBearerValuesRemainForbidden(t *testing.T) {
	tests := []string{
		"Bearer secret-tokem",
		"Bearer x",
		"Bearer secret-token-extra",
	}

	for _, authHeader := range tests {
		t.Run(authHeader, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Fatalf("wrong bearer value reached protected handler")
			})
			handler := authMiddleware("secret-token")(next)
			req := httptest.NewRequest(http.MethodGet, "/api/services", nil)
			req.Header.Set("Authorization", authHeader)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusForbidden, rec.Body.String())
			}
		})
	}
}

func TestPanicRecoveryMiddlewareFutureSpec(t *testing.T) {
	t.Skip("Known gap: enable in home-idhj.2 after server handlers are wrapped with panic recovery that returns a clean 500")
}
