package formations

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	osuser "os/user"
	"path/filepath"
	"strings"
	"time"
)

// harnessClaudeCode is the harness id whose completion is read from the agent's
// native JSONL transcript instead of the live tmux pane. Every other harness
// keeps scraping the pane (see captureCompletionText), so the codex path is
// unchanged.
const harnessClaudeCode = "claude-code"

// transcriptRootEnv optionally overrides the directory that holds claude-code's
// per-project transcript subdirectories. When unset the root is derived from the
// configured agent-user's home (never hardcoded).
const transcriptRootEnv = "CHROTE_FORMATIONS_TRANSCRIPT_ROOT"

// transcriptMtimeSkew tolerates a small clock/scheduling gap between the recorded
// dispatchStart and the transcript file's mtime, so a session whose transcript
// was created a moment before dispatchStart still qualifies.
const transcriptMtimeSkew = 2 * time.Second

// transcriptCurrentUser / transcriptLookupUser resolve the agent-home directory
// used to locate transcripts. They are package vars so tests can inject fake
// identities; nothing is ever hardcoded to a specific user or home path.
var (
	transcriptCurrentUser = osuser.Current
	transcriptLookupUser  = osuser.Lookup
)

// transcriptCandidate is one enumerated transcript file plus the two facts the
// selection guard needs: the cwd its first cwd-bearing record reports, and its
// modification time.
type transcriptCandidate struct {
	path  string
	cwd   string
	mtime time.Time
}

// captureCompletionText returns the "captured" text for a dispatch from the
// source appropriate to the harness: claude-code reads the native transcript;
// every other harness scrapes the live pane exactly as before. Only the SOURCE
// changes — the returned string flows unchanged through the existing sentinel
// counting/parsing/extraction pipeline.
func (e *TmuxFormationExecutor) captureCompletionText(ctx context.Context, harnessID, sessionName, runID string, dispatchStart time.Time) (string, error) {
	if harnessID == harnessClaudeCode {
		return e.captureFromTranscript(runID, dispatchStart)
	}
	return e.client.CapturePane(ctx, e.config.Socket, sessionName, e.config.OutputCapBytes+1)
}

// captureFromTranscript reads the claude-code native transcript as a read-only
// side-channel — it never touches the live tmux session. It picks the newest
// transcript whose reported cwd matches the executor's configured cwd and whose
// mtime is at/after dispatchStart, then flattens its assistant turns. When no
// qualifying transcript exists yet it returns "" (no error) so the caller treats
// it as "no sentinel yet" and keeps polling; it NEVER falls back to the pane for
// claude-code (that would reintroduce the pane-scrape bug).
func (e *TmuxFormationExecutor) captureFromTranscript(runID string, dispatchStart time.Time) (string, error) {
	root, err := e.transcriptRoot()
	if err != nil {
		return "", err
	}
	candidates, err := listTranscriptCandidates(root)
	if err != nil {
		return "", err
	}
	path := newestMatchingTranscript(candidates, e.config.Cwd, dispatchStart)
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return assistantTextFromTranscript(data)
}

// transcriptRoot resolves the directory that holds claude-code's per-project
// transcript subdirectories. An explicit CHROTE_FORMATIONS_TRANSCRIPT_ROOT
// override wins; otherwise it is <agent-home>/.claude/projects.
func (e *TmuxFormationExecutor) transcriptRoot() (string, error) {
	if override := strings.TrimSpace(os.Getenv(transcriptRootEnv)); override != "" {
		return override, nil
	}
	home, err := e.agentHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// agentHomeDir derives the agent's home directory from the configured agent-user
// (os/user.Lookup); an empty agent-user falls back to the service user's own home
// (os/user.Current). Nothing is hardcoded to a specific username or home path.
func (e *TmuxFormationExecutor) agentHomeDir() (string, error) {
	name := strings.TrimSpace(e.config.AgentUser)
	if name == "" {
		current, err := transcriptCurrentUser()
		if err != nil {
			return "", err
		}
		return current.HomeDir, nil
	}
	account, err := transcriptLookupUser(name)
	if err != nil {
		return "", err
	}
	return account.HomeDir, nil
}

// listTranscriptCandidates enumerates *.jsonl files under the project
// subdirectories of root, tagging each with its reported cwd and mtime. A missing
// root (the agent has not written any transcript yet) is not an error — it yields
// no candidates so the caller keeps polling.
func listTranscriptCandidates(root string) ([]transcriptCandidate, error) {
	projects, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var candidates []transcriptCandidate
	for _, project := range projects {
		if !project.IsDir() {
			continue
		}
		projectDir := filepath.Join(root, project.Name())
		files, err := os.ReadDir(projectDir)
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".jsonl") {
				continue
			}
			path := filepath.Join(projectDir, file.Name())
			info, err := file.Info()
			if err != nil {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			cwd := firstTranscriptCwd(data)
			if cwd == "" {
				continue
			}
			candidates = append(candidates, transcriptCandidate{path: path, cwd: cwd, mtime: info.ModTime()})
		}
	}
	return candidates, nil
}

// newestMatchingTranscript picks the newest candidate whose cwd == wantCwd and
// whose mtime is at/after since (minus the skew tolerance). Returns "" when none
// qualify. Pure over the injected candidate listing so it is unit-testable.
func newestMatchingTranscript(candidates []transcriptCandidate, wantCwd string, since time.Time) string {
	threshold := since.Add(-transcriptMtimeSkew)
	want := filepath.Clean(wantCwd)
	best := ""
	var bestMtime time.Time
	for _, candidate := range candidates {
		if filepath.Clean(candidate.cwd) != want {
			continue
		}
		if candidate.mtime.Before(threshold) {
			continue
		}
		if best == "" || candidate.mtime.After(bestMtime) {
			best = candidate.path
			bestMtime = candidate.mtime
		}
	}
	return best
}

// firstTranscriptCwd returns the cwd from the first record that carries a
// non-empty cwd field. Real transcripts open with a few cwd-less bookkeeping
// records (last-prompt/mode/permission-mode), so this scans forward to the first
// record that actually reports the agent's working directory.
func firstTranscriptCwd(data []byte) string {
	for _, line := range splitJSONLines(data) {
		var record struct {
			Cwd string `json:"cwd"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}
		if strings.TrimSpace(record.Cwd) != "" {
			return record.Cwd
		}
	}
	return ""
}

// assistantTextFromTranscript decodes claude-code JSONL bytes and returns the
// concatenated assistant-message text, in order. Malformed or half-written lines
// (agents append incrementally) are skipped rather than treated as fatal, so an
// in-progress transcript still parses. Pure; unit-tested with fixtures.
func assistantTextFromTranscript(jsonl []byte) (string, error) {
	var parts []string
	for _, line := range splitJSONLines(jsonl) {
		var record struct {
			Type    string `json:"type"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}
		if record.Type != "assistant" {
			continue
		}
		parts = append(parts, assistantContentText(record.Message.Content)...)
	}
	return strings.Join(parts, "\n"), nil
}

// assistantContentText flattens a claude-code assistant message.content — which
// is either a bare string or an array of typed blocks — to its plain text. Only
// text blocks contribute; thinking and tool blocks are ignored so internal
// reasoning never leaks into the captured string.
func assistantContentText(content json.RawMessage) []string {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return nil
	}
	if trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return nil
		}
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return []string{text}
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(trimmed, &blocks); err != nil {
		return nil
	}
	var parts []string
	for _, block := range blocks {
		if block.Type == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return parts
}

// splitJSONLines splits JSONL bytes into non-empty trimmed lines.
func splitJSONLines(data []byte) [][]byte {
	raw := bytes.Split(data, []byte("\n"))
	lines := make([][]byte, 0, len(raw))
	for _, line := range raw {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}
