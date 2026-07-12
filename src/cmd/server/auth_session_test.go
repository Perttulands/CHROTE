package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuthSessionLoginSetsSecureBrowserCookie(t *testing.T) {
	mux := http.NewServeMux()
	registerAuthSessionRoutes(mux, "secret-token")

	req := httptest.NewRequest(http.MethodPost, "/auth/session", strings.NewReader(`{"token":"secret-token"}`))
	req.Host = "chrote.example"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://chrote.example")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != authSessionCookieName || cookie.Value == "" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("unsafe auth cookie: %#v", cookie)
	}
	if cookie.Value == "secret-token" {
		t.Fatal("browser cookie exposed the configured bearer token")
	}
}

func TestAuthSessionLoginRejectsWrongTokenAndCrossOrigin(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		origin string
		token  string
		want   int
	}{
		{name: "wrong token", token: "wrong", want: http.StatusForbidden},
		{name: "cross origin", token: "secret-token", origin: "https://evil.example", want: http.StatusForbidden},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			mux := http.NewServeMux()
			registerAuthSessionRoutes(mux, "secret-token")
			req := httptest.NewRequest(http.MethodPost, "/auth/session", strings.NewReader(`{"token":"`+testCase.token+`"}`))
			req.Host = "chrote.example"
			if testCase.origin != "" {
				req.Header.Set("Origin", testCase.origin)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != testCase.want {
				t.Fatalf("status = %d, want %d", rec.Code, testCase.want)
			}
			if len(rec.Result().Cookies()) != 0 {
				t.Fatal("rejected login set a cookie")
			}
		})
	}
}

func TestAuthSessionStatusReportsConfiguredAuthentication(t *testing.T) {
	mux := http.NewServeMux()
	registerAuthSessionRoutes(mux, "secret-token")

	unauthenticated := httptest.NewRecorder()
	mux.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/auth/session", nil))
	if unauthenticated.Code != http.StatusUnauthorized || !strings.Contains(unauthenticated.Body.String(), `"required":true`) {
		t.Fatalf("unexpected unauthenticated status: %d %s", unauthenticated.Code, unauthenticated.Body.String())
	}

	authenticatedRequest := httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	authenticatedRequest.AddCookie(&http.Cookie{Name: authSessionCookieName, Value: authSessionValue("secret-token")})
	authenticated := httptest.NewRecorder()
	mux.ServeHTTP(authenticated, authenticatedRequest)
	if authenticated.Code != http.StatusOK || !strings.Contains(authenticated.Body.String(), `"authenticated":true`) {
		t.Fatalf("unexpected authenticated status: %d %s", authenticated.Code, authenticated.Body.String())
	}
}

func TestAuthSessionStatusRejectsExpiredSignedCookie(t *testing.T) {
	mux := http.NewServeMux()
	registerAuthSessionRoutes(mux, "secret-token")

	expired := authSessionValueAt("secret-token", time.Now().Add(-authSessionMaxAge-time.Minute))
	req := httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	req.AddCookie(&http.Cookie{Name: authSessionCookieName, Value: expired})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired browser session status = %d, want 401", rec.Code)
	}
}
