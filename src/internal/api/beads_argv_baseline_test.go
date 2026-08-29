package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func beadsHandlerWithFakeCommand(t *testing.T, bdOutput string) (*BeadsHandler, string, string) {
	t.Helper()

	resetBeadsTestEnv(t)

	root := t.TempDir()
	projectPath := filepath.Join(root, "project")
	makeValidBeadsWorkspace(t, projectPath)
	t.Setenv("CHROTE_ROOTS", root)

	_, argsPath := makeFakeBdCommand(t, bdOutput)
	return NewBeadsHandler(), projectPath, argsPath
}

func TestBeadsHandler_IssueDetailPassesNormalIDAsBDPositionals(t *testing.T) {
	handler, projectPath, argsPath := beadsHandlerWithFakeCommand(t, `{"_type":"issue","id":"home-abc1","title":"baseline"}`)

	values := url.Values{}
	values.Set("path", projectPath)
	values.Set("id", "home-abc1")
	req := httptest.NewRequest(http.MethodGet, "/api/beads/issue?"+values.Encode(), nil)
	rec := httptest.NewRecorder()

	handler.IssueDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("IssueDetail status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	wantArgs := []string{"--json", "show", "home-abc1"}
	if gotArgs := readFakeBdArgs(t, argsPath); !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("bd args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestBeadsHandler_CurrentlyPassesLeadingDashIssueIDAsRawPositionals(t *testing.T) {
	handler, projectPath, argsPath := beadsHandlerWithFakeCommand(t, `{"_type":"issue","id":"--db=/tmp/evil","title":"baseline"}`)

	values := url.Values{}
	values.Set("path", projectPath)
	values.Set("id", "--db=/tmp/evil")
	req := httptest.NewRequest(http.MethodGet, "/api/beads/issue?"+values.Encode(), nil)
	rec := httptest.NewRecorder()

	handler.IssueDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("IssueDetail status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	wantArgs := []string{"--json", "show", "--db=/tmp/evil"}
	if gotArgs := readFakeBdArgs(t, argsPath); !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("bd args = %#v, want current raw positional args %#v", gotArgs, wantArgs)
	}
}

func TestBeadsHandler_AddCommentPassesNormalIDAndCommentAsBDPositionals(t *testing.T) {
	handler, projectPath, argsPath := beadsHandlerWithFakeCommand(t, `{"id":"comment-1","body":"baseline comment"}`)

	body, err := json.Marshal(map[string]string{
		"path":    projectPath,
		"id":      "home-abc1",
		"comment": "baseline comment",
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/beads/comments", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.AddComment(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("AddComment status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	wantArgs := []string{"--json", "comments", "add", "home-abc1", "baseline comment"}
	if gotArgs := readFakeBdArgs(t, argsPath); !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("bd args = %#v, want %#v", gotArgs, wantArgs)
	}
}

func TestBeadsHandler_CurrentlyPassesLeadingDashCommentAsRawPositional(t *testing.T) {
	handler, projectPath, argsPath := beadsHandlerWithFakeCommand(t, `{"id":"comment-1","body":"--file=/tmp/secret"}`)

	body, err := json.Marshal(map[string]string{
		"path":    projectPath,
		"id":      "home-abc1",
		"comment": "--file=/tmp/secret",
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/beads/comments", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.AddComment(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("AddComment status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	wantArgs := []string{"--json", "comments", "add", "home-abc1", "--file=/tmp/secret"}
	if gotArgs := readFakeBdArgs(t, argsPath); !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("bd args = %#v, want current raw positional args %#v", gotArgs, wantArgs)
	}
}

func TestBeadsHandler_BaselineDoesNotUseRealWorkspaceDatabase(t *testing.T) {
	handler, projectPath, _ := beadsHandlerWithFakeCommand(t, `[]`)

	if !filepath.IsAbs(projectPath) {
		t.Fatalf("projectPath = %q, want absolute temp path", projectPath)
	}
	if !strings.HasPrefix(projectPath, os.TempDir()) && !strings.HasPrefix(projectPath, "/tmp/") {
		t.Fatalf("projectPath = %q, want a temp fixture path", projectPath)
	}
	if handler.bdCommand == "bd" {
		t.Fatal("baseline argv tests must use a fake bd command")
	}
}
