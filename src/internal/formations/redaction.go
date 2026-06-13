package formations

import (
	"regexp"
	"strings"
)

var (
	secretTokenPattern          = regexp.MustCompile(`(?i)\b(?:sk|xox[baprs])-[A-Za-z0-9_-]{8,}\b`)
	credentialAssignmentPattern = regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password)\s*[:=]\s*["']?[^\s,"']+`)
)

// redactLedgerText removes common secret-looking tokens before text reaches the
// durable run ledger. It is intentionally small and deterministic: executor
// code should still avoid logging raw prompts/captures at all, and this helper
// catches accidental credential-shaped values in adapter errors or sentinel
// metadata.
func redactLedgerText(text string) string {
	if text == "" {
		return ""
	}
	redacted := secretTokenPattern.ReplaceAllString(text, "[REDACTED_SECRET]")
	redacted = credentialAssignmentPattern.ReplaceAllString(redacted, "$1=[REDACTED]")
	return strings.TrimSpace(redacted)
}

func redactPromptFromLedgerText(text, prompt string) string {
	redacted := redactLedgerText(text)
	if redacted == "" || prompt == "" {
		return redacted
	}
	redacted = strings.ReplaceAll(redacted, prompt, "[REDACTED_PROMPT]")
	redacted = strings.ReplaceAll(redacted, redactLedgerText(prompt), "[REDACTED_PROMPT]")
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, candidate := range []string{line, redactLedgerText(line)} {
			if after, ok := strings.CutPrefix(candidate, "brief: "); ok {
				redacted = redactPromptFragment(redacted, candidate, after)
			}
			if after, ok := strings.CutPrefix(candidate, "input: "); ok {
				redacted = redactPromptFragment(redacted, candidate, after)
			}
		}
	}
	return strings.TrimSpace(redacted)
}

func redactPromptFragment(text, line, value string) string {
	text = strings.ReplaceAll(text, line, "[REDACTED_PROMPT]")
	value = strings.TrimSpace(value)
	if len(value) >= 8 {
		text = strings.ReplaceAll(text, value, "[REDACTED_PROMPT]")
	}
	return text
}
