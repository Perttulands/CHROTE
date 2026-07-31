package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	osuser "os/user"
	"path/filepath"
	"strings"
	"testing"
)

func TestSendToSessionRealTmuxPinsExactPane(t *testing.T) {
	if os.Getenv("CHROTE_REAL_TMUX_TEST") != "1" {
		t.Skip("set CHROTE_REAL_TMUX_TEST=1 only with explicit approval to start and stop a disposable tmux server")
	}
	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}
	if _, err := exec.LookPath("setfacl"); err != nil {
		t.Skip("setfacl is not installed")
	}
	current, err := osuser.Current()
	if err != nil || current.Username == "" {
		t.Skip("current Unix user is unavailable")
	}

	root := t.TempDir()
	socket := filepath.Join(root, "tmux.sock")
	drops := filepath.Join(root, "drops")
	runTmux := func(args ...string) (string, error) {
		cmdArgs := append([]string{"-S", socket}, args...)
		output, commandErr := exec.Command(tmuxBin, cmdArgs...).CombinedOutput()
		return string(output), commandErr
	}
	t.Cleanup(func() {
		for _, sessionName := range []string{"alpha-long", "multi"} {
			_, _ = runTmux("kill-session", "-t", sessionName)
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
