package main

import (
	"bytes"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionServerHasNoAccessTokenAuthentication(t *testing.T) {
	var productionSource strings.Builder
	err := filepath.WalkDir("../..", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		productionSource.Write(contents)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, forbidden := range []string{
		"APIAuthToken",
		"auth-token",
		"authMiddleware(",
		"registerAuthSessionRoutes(",
		"authSessionCookieName",
	} {
		if strings.Contains(productionSource.String(), forbidden) {
			t.Errorf("production server still contains access-token authentication marker %q", forbidden)
		}
	}
}

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
