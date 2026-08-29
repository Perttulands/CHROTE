//go:build live

package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	osuser "os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func privateTmuxAuthoritativelyAbsent(output []byte, socket string) bool {
	lower := strings.ToLower(string(output))
	if strings.Contains(lower, "no server running") {
		return true
	}
	if !strings.Contains(lower, "error connecting") || !strings.Contains(lower, "no such file or directory") {
		return false
	}
	_, err := os.Lstat(socket)
	return errors.Is(err, os.ErrNotExist)
}

func cleanupPrivateTmuxSessions(tmuxBin, socket, root string, sessions ...string) error {
	for _, session := range sessions {
		command := exec.Command(tmuxBin, "-S", socket, "kill-session", "-t", session)
		output, err := command.CombinedOutput()
		if err == nil {
			continue
		}
		if privateTmuxAuthoritativelyAbsent(output, socket) {
			break
		}
		return fmt.Errorf("kill exact private tmux session %q: %w: %s", session, err, strings.TrimSpace(string(output)))
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		probe := exec.Command(tmuxBin, "-S", socket, "list-sessions", "-F", "#{session_name}")
		output, err := probe.CombinedOutput()
		if err != nil {
			if !privateTmuxAuthoritativelyAbsent(output, socket) {
				return fmt.Errorf("prove private tmux server exited: %w: %s", err, strings.TrimSpace(string(output)))
			}
			if removeErr := os.RemoveAll(root); removeErr != nil {
				return fmt.Errorf("remove exited private tmux root: %w", removeErr)
			}
			return nil
		}
		if sessions := strings.TrimSpace(string(output)); sessions != "" {
			return fmt.Errorf("private tmux server retained unexpected sessions %q; retained root %s", sessions, root)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("private tmux server remained reachable without sessions; retained root %s", root)
}

func TestCleanupPrivateTmuxSessionsRetainsRootOnAmbiguousClientFailure(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "TestCleanupPrivateTmuxSessions")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatalf("create private root: %v", err)
	}
	socket := filepath.Join(root, "tmux.sock")
	if err := os.WriteFile(socket, []byte("fixture"), 0o600); err != nil {
		t.Fatalf("create socket sentinel: %v", err)
	}
	tmuxBin := filepath.Join(parent, "tmux")
	if err := os.WriteFile(tmuxBin, []byte("#!/bin/sh\nprintf 'tmux-guard: blocked owner-required-command\\n' >&2\nexit 73\n"), 0o700); err != nil {
		t.Fatalf("write fake tmux: %v", err)
	}

	if err := cleanupPrivateTmuxSessions(tmuxBin, socket, root, "owned"); err == nil {
		t.Fatal("cleanup succeeded after an ambiguous client failure")
	}
	if _, err := os.Stat(socket); err != nil {
		t.Fatalf("ambiguous cleanup removed the private socket/root: %v", err)
	}
}

func TestSendToSessionRealTmuxPinsExactPane(t *testing.T) {
	if os.Getenv("CHROTE_REAL_TMUX_TEST") != "1" {
		t.Fatal("live tmux test requires CHROTE_REAL_TMUX_TEST=1 and explicit approval for a disposable tmux server")
	}
	if os.Getenv("CHROTE_REAL_TMUX_OWNER_APPROVED") != "1" {
		t.Fatal("live tmux test requires CHROTE_REAL_TMUX_OWNER_APPROVED=1 after owner authorization is configured")
	}
	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatalf("live tmux test requires tmux: %v", err)
	}
	if _, err := exec.LookPath("setfacl"); err != nil {
		t.Fatalf("live tmux test requires setfacl: %v", err)
	}
	current, err := osuser.Current()
	if err != nil || current.Username == "" {
		t.Fatalf("live tmux test requires the current Unix user: %v", err)
	}

	root, err := os.MkdirTemp("", "TestSendToSessionRealTmuxPinsExactPane-*")
	if err != nil {
		t.Fatalf("create private tmux root: %v", err)
	}
	socket := filepath.Join(root, "tmux.sock")
	drops := filepath.Join(root, "drops")
	runTmux := func(args ...string) (string, error) {
		cmdArgs := append([]string{"-S", socket}, args...)
		output, commandErr := exec.Command(tmuxBin, cmdArgs...).CombinedOutput()
		return string(output), commandErr
	}
	t.Cleanup(func() {
		if cleanupErr := cleanupPrivateTmuxSessions(tmuxBin, socket, root, "alpha-long", "multi"); cleanupErr != nil {
			t.Errorf("cleanup private tmux fixture: %v", cleanupErr)
		}
	})
	if output, err := runTmux("new-session", "-d", "-s", "alpha-long", "-x", "100", "-y", "20", "bash", "--noprofile", "--norc"); err != nil {
		t.Fatalf("create prefix fixture: %v: %s", err, output)
	}
	if output, err := runTmux("new-session", "-d", "-s", "multi", "-x", "100", "-y", "20", "bash", "--noprofile", "--norc"); err != nil {
		t.Fatalf("create multi fixture: %v: %s", err, output)
	}
	if output, err := runTmux("split-window", "-d", "-t", "multi", "bash", "--noprofile", "--norc"); err != nil {
		t.Fatalf("split multi fixture: %v: %s", err, output)
	}

	paneOutput, err := runTmux("list-panes", "-t", "multi", "-F", "#{session_id}	#{pane_id}	#{pane_pid}	#{pid}")
	if err != nil {
		t.Fatalf("list fixture panes: %v: %s", err, paneOutput)
	}
	paneTargets := []sendPaneTarget{}
	for _, line := range strings.Split(strings.TrimSpace(paneOutput), "\n") {
		parts := strings.Split(line, "	")
		if len(parts) != 4 {
			t.Fatalf("unexpected fixture pane identity %q", line)
		}
		paneTargets = append(paneTargets, sendPaneTarget{SessionID: parts[0], Session: "multi", PaneID: parts[1], PanePID: parts[2], ServerPID: parts[3]})
	}
	if len(paneTargets) != 2 {
		t.Fatalf("fixture panes = %v, want 2", paneTargets)
	}

	t.Setenv("CHROTE_SESSION_DROPS_DIR", drops)
	t.Setenv("CHROTE_TERMINAL_USERS", current.Username)
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", current.Username+"="+socket)
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", current.Username+"="+root)
	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	send := func(session string, fields map[string]string) *httptest.ResponseRecorder {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		for key, value := range fields {
			if err := writer.WriteField(key, value); err != nil {
				t.Fatalf("write %s: %v", key, err)
			}
		}
		if err := writer.WriteField("unixUser", current.Username); err != nil {
			t.Fatalf("write unixUser: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("close form: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/"+session+"/send", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, req)
		return recorder
	}

	prefix := send("alpha", map[string]string{"text": "PREFIX_MUST_NOT_ROUTE"})
	if prefix.Code != http.StatusNotFound {
		t.Fatalf("prefix status = %d, want %d; body=%s", prefix.Code, http.StatusNotFound, prefix.Body.String())
	}
	ambiguous := send("multi", map[string]string{"text": "AMBIGUOUS_MUST_NOT_ROUTE"})
	if ambiguous.Code != http.StatusConflict {
		t.Fatalf("ambiguous status = %d, want %d; body=%s", ambiguous.Code, http.StatusConflict, ambiguous.Body.String())
	}

	marker := ": # CHROTE_REAL_SEND_EXACT_PANE"
	selected := paneTargets[0]
	exact := send("multi", map[string]string{
		"text":      marker,
		"pane":      selected.PaneID,
		"sessionId": selected.SessionID,
		"panePid":   selected.PanePID,
		"serverPid": selected.ServerPID,
		"submit":    "true",
	})
	if exact.Code != http.StatusOK {
		t.Fatalf("exact status = %d, want %d; body=%s", exact.Code, http.StatusOK, exact.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(exact.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["pane"] != selected.PaneID || response["submitKeyDispatched"] != true || response["targetVerified"] != true {
		t.Fatalf("exact response = %#v", response)
	}

	for index, paneTarget := range paneTargets {
		pane := paneTarget.PaneID
		capture, err := runTmux("capture-pane", "-p", "-t", pane, "-S", "-20")
		if err != nil {
			t.Fatalf("capture %s: %v: %s", pane, err, capture)
		}
		contains := strings.Contains(capture, marker)
		if index == 0 && !contains {
			t.Fatalf("selected pane %s did not receive marker: %q", pane, capture)
		}
		if index == 1 && contains {
			t.Fatalf("unselected pane %s received marker: %q", pane, capture)
		}
	}
	buffers, bufferErr := runTmux("list-buffers", "-F", "#{buffer_name}")
	if bufferErr == nil && strings.Contains(buffers, "chrote-send-") {
		t.Fatalf("send buffer leaked after success: %q", buffers)
	}
}

func TestSendToSessionRealCodexLongPrompt(t *testing.T) {
	if os.Getenv("CHROTE_REAL_CODEX_TEST") != "1" {
		t.Fatal("live Codex smoke requires CHROTE_REAL_CODEX_TEST=1 and explicit private-socket approval")
	}
	if os.Getenv("CHROTE_REAL_TMUX_OWNER_APPROVED") != "1" {
		t.Fatal("live Codex smoke requires CHROTE_REAL_TMUX_OWNER_APPROVED=1 after owner authorization is configured")
	}
	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatalf("live Codex smoke requires tmux: %v", err)
	}
	codexBin, err := exec.LookPath("codex")
	if err != nil {
		t.Fatalf("live Codex smoke requires codex: %v", err)
	}
	if _, err := exec.LookPath("setfacl"); err != nil {
		t.Fatalf("live Codex smoke requires setfacl: %v", err)
	}
	current, err := osuser.Current()
	if err != nil || current.Username == "" {
		t.Fatalf("live Codex smoke requires the current Unix user: %v", err)
	}

	root, err := os.MkdirTemp("", "TestSendToSessionRealCodexLongPrompt-*")
	if err != nil {
		t.Fatalf("create private smoke root: %v", err)
	}
	socket := filepath.Join(root, "tmux.sock")
	drops := filepath.Join(root, "drops")
	const session = "chrote-ylb-codex-smoke"
	runTmux := func(args ...string) (string, error) {
		cmdArgs := append([]string{"-S", socket}, args...)
		output, commandErr := exec.Command(tmuxBin, cmdArgs...).CombinedOutput()
		return string(output), commandErr
	}
	t.Cleanup(func() {
		if cleanupErr := cleanupPrivateTmuxSessions(tmuxBin, socket, root, session); cleanupErr != nil {
			t.Errorf("cleanup private Codex fixture: %v", cleanupErr)
		}
	})

	repositoryRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve repository workdir: %v", err)
	}
	if filepath.Base(repositoryRoot) == "src" {
		repositoryRoot = filepath.Dir(repositoryRoot)
	}
	if output, err := runTmux("new-session", "-d", "-s", session, "-x", "120", "-y", "35", "-c", repositoryRoot, codexBin, "--no-alt-screen"); err != nil {
		t.Fatalf("create private Codex session: %v: %s", err, output)
	}

	if output, err := runTmux("set-option", "-g", "exit-empty", "on"); err != nil {
		t.Fatalf("pin private server exit-empty policy: %v: %s", err, output)
	}

	readyDeadline := time.Now().Add(20 * time.Second)
	var capture string
	for time.Now().Before(readyDeadline) {
		capture, err = runTmux("capture-pane", "-p", "-J", "-t", session, "-S", "-50")
		if err == nil && strings.Contains(capture, "OpenAI Codex") && strings.Contains(capture, "›") {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil || !strings.Contains(capture, "OpenAI Codex") || !strings.Contains(capture, "›") {
		t.Fatalf("Codex did not reach a recognized idle composer: err=%v capture=%q", err, capture)
	}

	paneOutput, err := runTmux("list-panes", "-t", session, "-F", "#{session_id}	#{session_name}	#{pane_id}	#{pane_pid}	#{pid}	#{pane_current_command}")
	if err != nil {
		t.Fatalf("resolve private Codex pane: %v: %s", err, paneOutput)
	}
	parts := strings.Split(strings.TrimSpace(paneOutput), "	")
	if len(parts) != 6 || parts[1] != session || parts[5] != "codex" {
		t.Fatalf("unexpected private Codex pane identity: %q", paneOutput)
	}
	pane := sendPaneTarget{SessionID: parts[0], Session: parts[1], PaneID: parts[2], PanePID: parts[3], ServerPID: parts[4], CurrentCommand: parts[5]}

	t.Setenv("CHROTE_SESSION_DROPS_DIR", drops)
	t.Setenv("CHROTE_TERMINAL_USERS", current.Username)
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", current.Username+"="+socket)
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", current.Username+"="+repositoryRoot)
	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	const expectedResponse = "CHROTE_YLB_SMOKE_OK_8f31"
	instruction := "\nIgnore all padding above. Reply with only the two fragments joined without spaces: CHROTE_YLB_SMOKE_ and OK_8f31. Do not inspect or modify files."
	paddingUnit := "Context padding for bracketed paste transport only; no action is requested here.\n"
	paddingLength := 9477 - len(instruction)
	prompt := strings.Repeat(paddingUnit, paddingLength/len(paddingUnit)+1)[:paddingLength] + instruction
	if len(prompt) != 9477 || strings.Contains(prompt, expectedResponse) {
		t.Fatalf("long-prompt fixture invalid: bytes=%d containsResponse=%t", len(prompt), strings.Contains(prompt, expectedResponse))
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range map[string]string{
		"text":      prompt,
		"unixUser":  current.Username,
		"pane":      pane.PaneID,
		"sessionId": pane.SessionID,
		"panePid":   pane.PanePID,
		"serverPid": pane.ServerPID,
		"submit":    "true",
	} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write %s: %v", key, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close long-prompt form: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/tmux/sessions/"+session+"/send", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("long-prompt send status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode long-prompt receipt: %v", err)
	}
	if response["transport"] != "pasted" || response["submitKeyDispatched"] != true || response["targetVerified"] != true || response["bufferCleaned"] != true {
		t.Fatalf("long-prompt transport receipt = %#v", response)
	}
	if _, legacy := response["submitted"]; legacy {
		t.Fatalf("long-prompt receipt claimed application acceptance: %#v", response)
	}

	launchDeadline := time.Now().Add(60 * time.Second)
	launchEvidence := ""
	for time.Now().Before(launchDeadline) {
		capture, err = runTmux("capture-pane", "-p", "-J", "-t", pane.PaneID, "-S", "-300")
		if err != nil {
			break
		}
		if strings.Contains(strings.ToLower(capture), "esc to interrupt") {
			launchEvidence = "active-turn affordance"
			break
		}
		if strings.Contains(capture, expectedResponse) {
			launchEvidence = "completed response marker"
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if launchEvidence == "" {
		t.Fatalf("long Codex prompt did not show working or completed evidence: err=%v capture=%q", err, capture)
	}
	t.Logf("private Codex long-prompt smoke: payloadBytes=%d pane=%s evidence=%s receipt=transport:pasted submitKeyDispatched:true targetVerified:true bufferCleaned:true", len(prompt), pane.PaneID, launchEvidence)

	if buffers, bufferErr := runTmux("list-buffers", "-F", "#{buffer_name}"); bufferErr == nil && strings.Contains(buffers, "chrote-send-") {
		t.Fatalf("send buffer leaked after long-prompt smoke: %q", buffers)
	}
}
