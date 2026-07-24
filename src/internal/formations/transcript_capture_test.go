package formations

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	fixtureRunID = "run_01TESTCAPTURE000000000000001"
	fixturePort  = "port_01TESTPORT0000000000000001"
	fixtureCwd   = "/workspace/demo"
)

func readTranscriptFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "transcripts", name))
	if err != nil {
		t.Fatalf("read transcript fixture %q: %v", name, err)
	}
	return data
}

// RED test 1: assistantTextFromTranscript flattens a real-shaped transcript
// (mixed record types; assistant content as BOTH an array-of-blocks and a bare
// string) and the chrote-outputs block + CHROTE-DONE sentinel survive intact,
// while non-text blocks (thinking) never leak.
func TestAssistantTextFromTranscriptExtractsCleanCompletion(t *testing.T) {
	text, err := assistantTextFromTranscript(readTranscriptFixture(t, "sample_run.jsonl"))
	if err != nil {
		t.Fatalf("assistantTextFromTranscript error = %v, want nil", err)
	}
	if !strings.Contains(text, "```chrote-outputs") {
		t.Fatalf("captured text missing chrote-outputs fence:\n%s", text)
	}
	wantSentinel := "<<<CHROTE-DONE run-id=" + fixtureRunID + " status=ok artifact=none>>>"
	if !strings.Contains(text, wantSentinel) {
		t.Fatalf("captured text missing sentinel %q:\n%s", wantSentinel, text)
	}
	if !strings.Contains(text, "Working through the brief now.") {
		t.Fatalf("captured text missing array-of-blocks assistant text:\n%s", text)
	}
	if strings.Contains(text, "internal reasoning that must not leak") {
		t.Fatalf("captured text leaked a thinking block:\n%s", text)
	}
}

// RED test 2: the transcript-derived text flows unchanged through the existing
// sentinel pipeline (ParseCompletionSentinel + countCompletionSentinels).
func TestTranscriptTextFeedsSentinelPipeline(t *testing.T) {
	text, err := assistantTextFromTranscript(readTranscriptFixture(t, "sample_run.jsonl"))
	if err != nil {
		t.Fatalf("assistantTextFromTranscript error = %v, want nil", err)
	}
	sentinel, ok := ParseCompletionSentinel(text, fixtureRunID)
	if !ok {
		t.Fatalf("ParseCompletionSentinel found no sentinel in transcript text")
	}
	if sentinel.Status != "ok" {
		t.Fatalf("sentinel status = %q, want ok", sentinel.Status)
	}
	if sentinel.Artifact != "none" {
		t.Fatalf("sentinel artifact = %q, want none", sentinel.Artifact)
	}
	if got := countCompletionSentinels(text, fixtureRunID); got != 1 {
		t.Fatalf("countCompletionSentinels = %d, want 1", got)
	}
}

// RED test 3: newestMatchingTranscript picks the newest file whose cwd matches
// and whose mtime is at/after dispatchStart; wrong cwd and too-old files are
// rejected, and "" is returned when none qualify.
func TestNewestMatchingTranscriptPicksNewestQualifying(t *testing.T) {
	dispatchStart := time.Unix(1_700_000_000, 0)
	candidates := []transcriptCandidate{
		{path: "/root/proj-a/wrong-cwd.jsonl", cwd: "/workspace/other", mtime: dispatchStart.Add(1 * time.Minute)},
		{path: "/root/proj-b/too-old.jsonl", cwd: fixtureCwd, mtime: dispatchStart.Add(-1 * time.Minute)},
		{path: "/root/proj-c/newest.jsonl", cwd: fixtureCwd, mtime: dispatchStart.Add(30 * time.Second)},
	}
	got := newestMatchingTranscript(candidates, fixtureCwd, dispatchStart)
	if got != "/root/proj-c/newest.jsonl" {
		t.Fatalf("newestMatchingTranscript = %q, want /root/proj-c/newest.jsonl", got)
	}

	none := newestMatchingTranscript(candidates, "/workspace/nomatch", dispatchStart)
	if none != "" {
		t.Fatalf("newestMatchingTranscript (no cwd match) = %q, want empty", none)
	}
}

// RED test 4: a reused claude session keeps one growing transcript with two
// run-id sentinels, so the count is 2; hasNewCompletionSentinel is true against
// previousSentinels=1 and false against previousSentinels=2.
func TestTranscriptReusedSessionCountsBothSentinels(t *testing.T) {
	text, err := assistantTextFromTranscript(readTranscriptFixture(t, "reused_session.jsonl"))
	if err != nil {
		t.Fatalf("assistantTextFromTranscript error = %v, want nil", err)
	}
	if got := countCompletionSentinels(text, fixtureRunID); got != 2 {
		t.Fatalf("countCompletionSentinels = %d, want 2", got)
	}
	if !hasNewCompletionSentinel(text, "", fixtureRunID, 1) {
		t.Fatalf("hasNewCompletionSentinel(previousSentinels=1) = false, want true")
	}
	if hasNewCompletionSentinel(text, "", fixtureRunID, 2) {
		t.Fatalf("hasNewCompletionSentinel(previousSentinels=2) = true, want false")
	}
}

// RED test 5: capture routing. claude-code reads the transcript (pane never
// consulted); any other harness id scrapes the pane via the tmux client.
func TestCaptureRoutingClaudeReadsTranscriptOthersScrapePane(t *testing.T) {
	root := t.TempDir()
	cwd := t.TempDir()
	writeRoutingTranscript(t, root, cwd)
	t.Setenv(transcriptRootEnv, root)

	client := &fakeTmuxHarnessClient{paneText: "PANE-SCRAPE-MARKER"}
	cfg := TmuxExecutorConfig{
		Socket:         filepath.Join(root, "ignored.sock"),
		Cwd:            cwd,
		OutputCapBytes: defaultTmuxOutputCapBytes,
		TimeoutSeconds: 1,
	}
	exec := newTmuxFormationExecutorWithClient(nil, nil, cfg, client)
	dispatchStart := time.Now().Add(-time.Hour)

	claudeText, err := exec.captureCompletionText(context.Background(), harnessClaudeCode, "sess-claude", fixtureRunID, dispatchStart)
	if err != nil {
		t.Fatalf("captureCompletionText(claude-code) error = %v, want nil", err)
	}
	if !strings.Contains(claudeText, "<<<CHROTE-DONE run-id="+fixtureRunID) {
		t.Fatalf("claude-code capture did not read the transcript sentinel:\n%s", claudeText)
	}
	if client.captureCalls != 0 {
		t.Fatalf("claude-code capture consulted the pane %d times, want 0", client.captureCalls)
	}

	paneText, err := exec.captureCompletionText(context.Background(), "openai-codex", "sess-codex", fixtureRunID, dispatchStart)
	if err != nil {
		t.Fatalf("captureCompletionText(openai-codex) error = %v, want nil", err)
	}
	if !strings.Contains(paneText, "PANE-SCRAPE-MARKER") {
		t.Fatalf("non-claude capture = %q, want pane scrape marker", paneText)
	}
	if client.captureCalls != 1 {
		t.Fatalf("non-claude capture consulted the pane %d times, want 1", client.captureCalls)
	}
}

// RED test 6: malformed / half-written JSONL lines are skipped, not fatal, so an
// incremental poll over a partially-written transcript still returns the valid
// assistant text without error.
func TestAssistantTextFromTranscriptSkipsMalformedLines(t *testing.T) {
	text, err := assistantTextFromTranscript(readTranscriptFixture(t, "partial_trailing.jsonl"))
	if err != nil {
		t.Fatalf("assistantTextFromTranscript error = %v, want nil (partial lines must be skipped)", err)
	}
	if !strings.Contains(text, "<<<CHROTE-DONE run-id="+fixtureRunID) {
		t.Fatalf("valid assistant turn was dropped:\n%s", text)
	}
	if strings.Contains(text, "half written and truncat") {
		t.Fatalf("captured text included a half-written trailing line:\n%s", text)
	}
}

// writeRoutingTranscript materializes one qualifying claude-code transcript under
// root/<project>/<uuid>.jsonl whose first cwd-bearing record reports wantCwd.
func writeRoutingTranscript(t *testing.T, root, wantCwd string) {
	t.Helper()
	projectDir := filepath.Join(root, "-workspace-demo")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	final := "Routing check complete.\n\n" +
		"```chrote-outputs\n" +
		"{\n  \"" + fixturePort + "\": {\"text\": \"routing-pass\"}\n}\n" +
		"```\n\n" +
		"<<<CHROTE-DONE run-id=" + fixtureRunID + " status=ok artifact=none>>>"
	records := []map[string]any{
		{"type": "attachment", "cwd": wantCwd, "sessionId": "sess-claude"},
		{"type": "assistant", "cwd": wantCwd, "sessionId": "sess-claude",
			"message": map[string]any{"role": "assistant", "content": final}},
	}
	var b strings.Builder
	for _, rec := range records {
		raw, err := json.Marshal(rec)
		if err != nil {
			t.Fatalf("marshal routing record: %v", err)
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(projectDir, "sess-claude.jsonl"), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write routing transcript: %v", err)
	}
}
