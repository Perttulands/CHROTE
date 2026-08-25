package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	osuser "os/user"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/chrote/server/internal/core"
)

func snapshotProtectedTmuxSockets(handler *TmuxHandler, raw string) (map[string]string, error) {
	snapshots := map[string]string{}
	for _, socket := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' }) {
		socket = strings.TrimSpace(socket)
		if socket == "" {
			continue
		}
		if !filepath.IsAbs(socket) {
			return nil, fmt.Errorf("protected tmux socket is not absolute: %q", socket)
		}
		output, err := handler.runTmuxOnSocket(socket, "list-panes", "-a", "-F", "#{session_id}\t#{window_id}\t#{pane_id}\t#{session_name}")
		if err != nil {
			return nil, fmt.Errorf("snapshot protected tmux socket %q: %w", socket, err)
		}
		lines := strings.Split(strings.TrimSpace(output), "\n")
		sort.Strings(lines)
		snapshots[filepath.Clean(socket)] = strings.Join(lines, "\n")
	}
	return snapshots, nil
}

func TestTmuxHandler_ExactLaunchRealPrivateSocket(t *testing.T) {
	if os.Getenv("CHROTE_REAL_EXACT_LAUNCH_TEST") != "1" {
		t.Skip("set CHROTE_REAL_EXACT_LAUNCH_TEST=1 for the approved private-socket proof")
	}
	if os.Getenv("CHROTE_REAL_TMUX_OWNER_APPROVED") != "1" {
		t.Skip("set CHROTE_REAL_TMUX_OWNER_APPROVED=1 after approving exact private-session cleanup")
	}
	current, err := osuser.Current()
	if err != nil {
		t.Fatal(err)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required for the exact argv witness")
	}
	python, err = filepath.Abs(python)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp("/tmp", "chrote-tmux.exact-launch-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(root, "tmux.sock")
	outputPath := filepath.Join(root, "argv.json")
	sentinelPath := filepath.Join(root, "shell-interpolation-must-not-run")
	sessionName := "exact-launch-real"
	tmuxBin := core.TmuxBin()
	protectedSockets := os.Getenv("CHROTE_PROTECTED_TMUX_SOCKETS")
	protectedBefore, err := snapshotProtectedTmuxSockets(NewTmuxHandler(), protectedSockets)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		protectedAfter, snapshotErr := snapshotProtectedTmuxSockets(NewTmuxHandler(), protectedSockets)
		if snapshotErr != nil {
			t.Errorf("verify protected tmux sockets: %v", snapshotErr)
			return
		}
		if !reflect.DeepEqual(protectedBefore, protectedAfter) {
			t.Errorf("protected tmux inventory changed during private proof: before=%v after=%v", protectedBefore, protectedAfter)
		}
	})

	t.Cleanup(func() {
		deadline := time.Now().Add(5 * time.Second)
		for {
			probe := exec.Command(tmuxBin, "-S", socket, "list-sessions")
			probeOutput, probeErr := probe.CombinedOutput()
			if probeErr != nil {
				if !isTmuxNoServerError(string(probeOutput)) {
					t.Errorf("prove private tmux server exited: %v: %s; retained %s", probeErr, strings.TrimSpace(string(probeOutput)), root)
					return
				}
				break
			}
			if time.Now().After(deadline) {
				t.Errorf("private tmux server survived exact session cleanup; retained %s", root)
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		if err := os.RemoveAll(root); err != nil {
			t.Errorf("remove proven-dead private root: %v", err)
			return
		}
	})

	absent := exec.Command(tmuxBin, "-S", socket, "list-sessions")
	absentOutput, absentErr := absent.CombinedOutput()
	if absentErr == nil || !isTmuxNoServerError(string(absentOutput)) {
		t.Fatalf("private socket was not authoritatively absent: %v: %s", absentErr, strings.TrimSpace(string(absentOutput)))
	}

	t.Setenv("CHROTE_TERMINAL_USERS", current.Username)
	t.Setenv("CHROTE_TERMINAL_USER_SOCKETS", current.Username+"="+socket)
	t.Setenv("CHROTE_TERMINAL_USER_WORKDIRS", current.Username+"="+workspace)
	t.Setenv("CHROTE_TERMINAL_USER_HOMES", current.Username+"="+workspace)
	t.Setenv("CHROTE_ROOTS", workspace)
	t.Setenv("CHROTE_SESSION_BANK_PATH", filepath.Join(root, "session-bank.json"))
	t.Setenv("CHROTE_MANAGED_RECOVERY_STATUS_PATH", filepath.Join(root, "managed-status.json"))
	core.ResetConfigForTesting()
	t.Cleanup(core.ResetConfigForTesting)
	handler := NewTmuxHandler()
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	inspectRec := httptest.NewRecorder()
	mux.ServeHTTP(inspectRec, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	if inspectRec.Code != http.StatusOK {
		t.Fatalf("inspect status = %d: %s", inspectRec.Code, inspectRec.Body.String())
	}
	var before SessionsResponse
	if err := json.Unmarshal(inspectRec.Body.Bytes(), &before); err != nil {
		t.Fatal(err)
	}
	if len(before.Sources) != 1 || before.Sources[0].Status != tmuxSourceComplete || before.Sources[0].Generation == "" {
		t.Fatalf("inspect sources = %+v", before.Sources)
	}

	argv := []string{
		python,
		"-c",
		"import json,pathlib,sys,time; pathlib.Path(sys.argv[1]).write_text(json.dumps(sys.argv[2:])); time.sleep(2)",
		outputPath,
		"arg with spaces",
		"; touch " + sentinelPath,
	}
	body, err := json.Marshal(map[string]interface{}{
		"sourceId":         tmuxSourceID(current.Username),
		"sourceGeneration": before.Sources[0].Generation,
		"unixUser":         current.Username,
		"name":             sessionName,
		"cwd":              workspace,
		"argv":             argv,
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/tmux/recovery-launches/22222222-2222-4222-8222-222222222222", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("exact launch status = %d: %s", rec.Code, rec.Body.String())
	}
	var receipt ExactLaunchReceipt
	if err := json.Unmarshal(rec.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if !receipt.Success || receipt.State != "launched" || receipt.SessionName != sessionName || receipt.PanePID <= 0 || receipt.ProcessStart == "" || receipt.CWD != workspace {
		t.Fatalf("exact launch receipt = %+v", receipt)
	}
	verifyRec := httptest.NewRecorder()
	mux.ServeHTTP(verifyRec, httptest.NewRequest(http.MethodGet, "/api/tmux/sessions", nil))
	var after SessionsResponse
	if err := json.Unmarshal(verifyRec.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if len(after.Sources) != 1 || after.Sources[0].Generation == before.Sources[0].Generation {
		t.Fatalf("source generation did not advance: before=%+v after=%+v", before.Sources, after.Sources)
	}
	if len(after.RecoveryEvidence) != 1 || after.RecoveryEvidence[0].State != recoveryEvidenceLive || after.RecoveryEvidence[0].Name != sessionName || after.RecoveryEvidence[0].TmuxSessionID != receipt.SessionID || after.RecoveryEvidence[0].CWD != workspace {
		t.Fatalf("post-launch evidence = %+v", after.RecoveryEvidence)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		raw, err := os.ReadFile(outputPath)
		if err == nil {
			var observed []string
			if err := json.Unmarshal(raw, &observed); err != nil {
				t.Fatal(err)
			}
			want := []string{"arg with spaces", "; touch " + sentinelPath}
			if strings.Join(observed, "\x00") != strings.Join(want, "\x00") {
				t.Fatalf("observed argv = %#v, want %#v", observed, want)
			}
			break
		}
		if !os.IsNotExist(err) || time.Now().After(deadline) {
			t.Fatalf("read exact argv witness: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(sentinelPath); !os.IsNotExist(err) {
		t.Fatalf("shell interpolation sentinel exists: %v", err)
	}
}
