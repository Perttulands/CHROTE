package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServicesProxyForwardsOnlyKnownActions(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "services.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	upstream := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"method": r.Method, "path": r.URL.Path})
	})}
	go func() { _ = upstream.Serve(listener) }()
	t.Cleanup(func() { _ = upstream.Close() })
	mux := http.NewServeMux()
	NewServicesHandler(socket).RegisterRoutes(mux)
	for _, tc := range []struct {
		method, path, want string
		code               int
	}{
		{"GET", "/api/services", "/services", 200},
		{"POST", "/api/services/example/start", "/services/example/start", 200},
		{"POST", "/api/services/example/stop", "/services/example/stop", 200},
		{"POST", "/api/services/example/restart", "", 400},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("X-Requested-With", "chrote")
		mux.ServeHTTP(rec, req)
		if rec.Code != tc.code || (tc.want != "" && !strings.Contains(rec.Body.String(), tc.want)) {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestServicesActionRejectsUnmarkedRequest(t *testing.T) {
	mux := http.NewServeMux()
	NewServicesHandler("").RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/services/example/start", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestServicesNotConfigured(t *testing.T) {
	mux := http.NewServeMux()
	NewServicesHandler("").RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/services", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestServicesConfigIsHostOwned(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CHROTE_SERVICES_CONFIG", "")
	t.Setenv("CHROTE_LAUNCH_CONFIG", filepath.Join(dir, "launch.json"))
	if err := os.WriteFile(filepath.Join(dir, "services.json"), []byte(`{"socket":"/tmp/example.sock","resident":{"session":"example","folder":"/example"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadServicesConfig()
	if err != nil || config.Socket != "/tmp/example.sock" || config.Resident.Session != "example" {
		t.Fatalf("config = %+v, error = %v", config, err)
	}
}
