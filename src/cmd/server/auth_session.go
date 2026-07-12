package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	authSessionCookieName = "chrote_session"
	authSessionMaxAge     = 12 * time.Hour
)

type authSessionStatus struct {
	Required      bool `json:"required"`
	Authenticated bool `json:"authenticated"`
}

type authLoginRequest struct {
	Token string `json:"token"`
}

func authSessionValue(token string) string {
	return authSessionValueAt(token, time.Now())
}

func authSessionValueAt(token string, issuedAt time.Time) string {
	timestamp := strconv.FormatInt(issuedAt.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte("chrote-browser-session-v1\n" + timestamp))
	return timestamp + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func validAuthSession(value, token string, now time.Time) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return false
	}
	issuedUnix, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}
	issuedAt := time.Unix(issuedUnix, 0)
	if issuedAt.After(now.Add(time.Minute)) || now.Sub(issuedAt) > authSessionMaxAge {
		return false
	}
	expected := authSessionValueAt(token, issuedAt)
	return tokenMatches(value, expected)
}

func requestHasValidAuth(r *http.Request, token string) bool {
	if token == "" {
		return true
	}
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		return tokenMatches(strings.TrimPrefix(authHeader, "Bearer "), token)
	}
	cookie, err := r.Cookie(authSessionCookieName)
	return err == nil && validAuthSession(cookie.Value, token, time.Now())
}

func sameOriginRequest(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && strings.EqualFold(parsed.Host, r.Host)
}

func writeAuthStatus(w http.ResponseWriter, status int, body authSessionStatus) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func registerAuthSessionRoutes(mux *http.ServeMux, token string) {
	mux.HandleFunc("GET /auth/session", func(w http.ResponseWriter, r *http.Request) {
		required := token != ""
		authenticated := requestHasValidAuth(r, token)
		status := http.StatusOK
		if required && !authenticated {
			status = http.StatusUnauthorized
		}
		writeAuthStatus(w, status, authSessionStatus{Required: required, Authenticated: authenticated})
	})

	mux.HandleFunc("POST /auth/session", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !sameOriginRequest(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if token == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		var login authLoginRequest
		if err := json.NewDecoder(r.Body).Decode(&login); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if !tokenMatches(login.Token, token) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     authSessionCookieName,
			Value:    authSessionValue(token),
			Path:     "/",
			MaxAge:   int(authSessionMaxAge.Seconds()),
			Expires:  time.Now().Add(authSessionMaxAge),
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		})
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("DELETE /auth/session", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !sameOriginRequest(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     authSessionCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			Expires:  time.Unix(1, 0),
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		})
		w.WriteHeader(http.StatusNoContent)
	})
}
