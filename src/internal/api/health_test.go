package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The dashboard reads /api/health flat, with no data envelope, and it reads
// every key on every response: an unstamped build has to serve an empty commit
// rather than omit the field. The request travels through the mux because a
// health endpoint nothing routes to is a health endpoint nobody can reach.
func TestHealthHandler_Health(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		version string
		commit  string
	}{
		{name: "an unstamped build still serves the commit key", version: "2.0.0-alpha.2-dev", commit: ""},
		{name: "a stamped build reports the commit it was built from", version: "test-version", commit: "abc123def456"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			mux := http.NewServeMux()
			NewHealthHandlerWithBuildInfo(testCase.version, testCase.commit).RegisterRoutes(mux)

			req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, req)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
			}
			response := decodeJSONMap(t, recorder)
			assertTopLevelKeys(t, response, []string{"commit", "status", "timestamp", "version"})
			assertNoTopLevelKey(t, response, "data")
			if response["status"] != "ok" {
				t.Errorf("status = %v, want ok", response["status"])
			}
			if response["commit"] != testCase.commit {
				t.Errorf("commit = %v, want %q", response["commit"], testCase.commit)
			}
			if response["version"] != testCase.version {
				t.Errorf("version = %v, want %q", response["version"], testCase.version)
			}
		})
	}
}
