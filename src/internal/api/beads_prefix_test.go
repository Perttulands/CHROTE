package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBeadsProjectsDistinguishesFailedPrefixFromEmptyAndRecovers(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CHROTE_ROOTS", root)
	t.Setenv("CHROTE_BEADS_WORKSPACES", "")
	params := url.Values{}
	for _, name := range []string{"healthy", "empty", "failed", "malformed"} {
		path := filepath.Join(root, name)
		makeValidBeadsWorkspace(t, path)
		params.Add("path", path)
	}
	command := filepath.Join(root, "bd-fixture")
	if err := os.WriteFile(command, []byte(`#!/bin/sh
case "${PWD##*/}" in
  healthy) printf '[{"id":"good-abc","status":"open"}]' ;;
  empty) printf '[]' ;;
  failed)
    if [ -f ../recovered ]; then
      printf '[{"id":"recovered-abc","status":"open"}]'
    else
      printf 'identity lookup refused\n' >&2
      exit 7
    fi ;;
  malformed) printf '{broken' ;;
esac
`), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHROTE_BD_COMMAND", command)
	h := NewBeadsHandler()
	list := func() map[string]map[string]interface{} {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ListProjects(rec, httptest.NewRequest(http.MethodGet, "/api/beads/projects?"+params.Encode(), nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("partial prefix lookup lost the projects response: %d %s", rec.Code, rec.Body.String())
		}
		data := decodeBeadsData(t, rec)
		projects := make(map[string]map[string]interface{})
		for _, raw := range data["projects"].([]interface{}) {
			project := raw.(map[string]interface{})
			projects[filepath.Base(project["path"].(string))] = project
		}
		if len(projects) != 4 {
			t.Fatalf("partial lookup removed peers: %#v", projects)
		}
		return projects
	}
	projects := list()
	if projects["healthy"]["prefix"] != "good" || projects["healthy"]["prefixError"] != nil {
		t.Errorf("healthy identity = %#v", projects["healthy"])
	}
	if projects["empty"]["prefix"] != nil || projects["empty"]["prefixError"] != nil {
		t.Errorf("successful empty store should remain quiet: %#v", projects["empty"])
	}
	message, _ := projects["failed"]["prefixError"].(string)
	if message != "bd list --status all --limit 1 failed: identity lookup refused\n" {
		t.Errorf("failed identity lost its actual command cause: %#v", projects["failed"])
	}
	if message, _ := projects["malformed"]["prefixError"].(string); !strings.Contains(message, "invalid JSON") {
		t.Errorf("malformed identity was treated as empty: %#v", projects["malformed"])
	}
	if err := os.WriteFile(filepath.Join(root, "recovered"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	projects = list()
	if projects["failed"]["prefix"] != "recovered" || projects["failed"]["prefixError"] != nil {
		t.Errorf("same-handler recovery did not replace failure with a healthy identity: %#v", projects["failed"])
	}
}

func TestBeadsProjectsReportsCanceledAndTimedOutPrefixAdmission(t *testing.T) {
	for _, name := range []string{"canceled", "timed out"} {
		t.Run(name, func(t *testing.T) {
			h, paths := readerProjects(t, 1)
			h.execTimeout = 20 * time.Millisecond
			h.bdCommand = "must-not-start-a-prefix-command"
			for i := 0; i < cap(h.prefixSlots); i++ {
				h.prefixSlots <- struct{}{}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := context.DeadlineExceeded.Error()
			if name == "canceled" {
				cancel()
				want = context.Canceled.Error()
			}
			params := url.Values{"path": {paths[0]}}
			rec := httptest.NewRecorder()
			h.ListProjects(rec, httptest.NewRequest(http.MethodGet, "/api/beads/projects?"+params.Encode(), nil).WithContext(ctx))
			projects := decodeBeadsData(t, rec)["projects"].([]interface{})
			project := projects[0].(map[string]interface{})
			if project["prefixError"] != want || project["prefix"] != nil {
				t.Fatalf("%s prefix admission = %#v, want exact %q without starting the command", name, project, want)
			}
		})
	}
}
