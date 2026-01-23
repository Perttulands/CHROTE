package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess mimics the 'gt' command for testing.
// It is invoked by the tests via execCommand mocking.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_TEST_PROCESS") != "1" {
		return
	}
	// Do not defer os.Exit(0) here, call it explicitly to bypass test runner output
	
	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}

	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "No command provided\n")
		os.Exit(1)
	}

	cmd := args[0]
	subCmd := args[1:]

	fmt.Fprintf(os.Stderr, "DEBUG: cmd=%s subCmd=%v\n", cmd, subCmd)

	if cmd == "tmux" {
		// Mock tmux success for has-session/new-session checking
		os.Exit(0)
	}

	if cmd != "gt" {
		fmt.Fprintf(os.Stderr, "Unexpected command: %s\n", cmd)
		os.Exit(1)
	}

	// Route based on subcommands
	// gt mail channel list --json
	if contains(subCmd, "channel") && contains(subCmd, "list") && contains(subCmd, "--json") {
		channels := []GtChannel{
			{Name: "alerts", Status: "active", RetentionCount: 50, CreatedBy: "mayor"},
			{Name: "random", Status: "active", RetentionCount: 0, CreatedBy: "witness"},
		}
		b, _ := json.Marshal(channels)
		os.Stdout.Write(b)
		os.Exit(0)
	}

	// gt mail channel create <name>
	if contains(subCmd, "channel") && contains(subCmd, "create") {
		// Verify name present
		if len(subCmd) < 4 { // gt mail channel create NAME
			os.Exit(1)
		}
		// Success (exit 0)
		os.Exit(0)
	}

	// gt mail send <target> -m <msg> (Invite/Message)
	if contains(subCmd, "send") || (contains(subCmd, "channel") && contains(subCmd, "invite")) {
		os.Exit(0)
	}
	
	// gt mail channel invite?
	if contains(subCmd, "channel") && contains(subCmd, "invite") {
		os.Exit(0)
	}

	// gt mail channel show <name> --json
	if contains(subCmd, "channel") && contains(subCmd, "show") && contains(subCmd, "--json") {
		ts, _ := time.Parse(time.RFC3339, "2023-01-01T12:00:00Z")
		messages := []MailMessage{
			{ID: "msg1", From: "mayor", Body: "Hello channel", Timestamp: ts},
		}
		b, _ := json.Marshal(messages)
		os.Stdout.Write(b)
		os.Exit(0)
	}

	// Default: failure
	fmt.Fprintf(os.Stderr, "Unknown test command: %v\n", subCmd)
	os.Exit(1)
}

func contains(arr []string, str string) bool {
	for _, v := range arr {
		if v == str {
			return true
		}
	}
	return false
}

// mockExecCommand sets up the execCommand to run the TestHelperProcess
func mockExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_TEST_PROCESS=1"}
	return cmd
}

func TestCreateChannel(t *testing.T) {
	// Swap execCommand
	oldExec := execCommand
	execCommand = mockExecCommand
	defer func() { execCommand = oldExec }()

	workspace := t.TempDir()
	// Create daemon dir to pass validation
	os.Mkdir(workspace+"/daemon", 0755)

	h := NewChatHandler()

	payload := fmt.Sprintf(`{"workspace": "%s", "name": "new-chan"}`, workspace)
	req := httptest.NewRequest("POST", "/api/chat/channel/create", strings.NewReader(payload))
	w := httptest.NewRecorder()

	h.CreateChannel(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("CreateChannel status %d, want 200", resp.StatusCode)
	}
}

func TestListChannels(t *testing.T) {
	oldExec := execCommand
	execCommand = mockExecCommand
	defer func() { execCommand = oldExec }()

	workspace := t.TempDir()

	h := NewChatHandler()

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/channel/list?workspace=%s", workspace), nil)
	w := httptest.NewRecorder()

	h.ListChannels(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ListChannels status %d, want 200", resp.StatusCode)
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Channels []GtChannel `json:"channels"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if len(result.Data.Channels) != 2 {
		t.Errorf("Expected 2 channels, got %d", len(result.Data.Channels))
	}
	if result.Data.Channels[0].Name != "alerts" {
		t.Errorf("Expected first channel 'alerts', got %q", result.Data.Channels[0].Name)
	}
}

func TestInviteChannel(t *testing.T) {
	oldExec := execCommand
	execCommand = mockExecCommand
	defer func() { execCommand = oldExec }()

	workspace := t.TempDir()

	h := NewChatHandler()

	payload := fmt.Sprintf(`{"workspace": "%s", "channel": "alerts", "targets": ["witness", "deacon"]}`, workspace)
	req := httptest.NewRequest("POST", "/api/chat/channel/invite", strings.NewReader(payload))
	w := httptest.NewRecorder()

	h.InviteChannel(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("InviteChannel status %d, want 200", resp.StatusCode)
	}
	
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Sent int `json:"sent"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	
	if result.Data.Sent != 2 {
		t.Errorf("Expected 2 sent notifications, got %d", result.Data.Sent)
	}
}

func TestGetChannelMessages(t *testing.T) {
	oldExec := execCommand
	execCommand = mockExecCommand
	defer func() { execCommand = oldExec }()

	workspace := t.TempDir()

	h := NewChatHandler()

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/chat/channel/messages?workspace=%s&channel=alerts", workspace), nil)
	w := httptest.NewRecorder()

	h.GetChannelMessages(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GetChannelMessages status %d, want 200", resp.StatusCode)
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Messages []ChatMessage `json:"messages"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if len(result.Data.Messages) != 1 {
		t.Errorf("Expected 1 message, got %d", len(result.Data.Messages))
	}
	if result.Data.Messages[0].Content != "Hello channel" {
		t.Errorf("Expected 'Hello channel', got %q", result.Data.Messages[0].Content)
	}
	// Verify transformation
	if result.Data.Messages[0].Role != "agent" {
		t.Errorf("Expected role 'agent' for channel msg, got %q", result.Data.Messages[0].Role)
	}
	if result.Data.Messages[0].To != "alerts" {
		t.Errorf("Expected to 'alerts', got %q", result.Data.Messages[0].To)
	}
}
