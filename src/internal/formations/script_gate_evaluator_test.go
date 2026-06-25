package formations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScriptGateEvaluatorRequiresCommandArgvAndNeverRunsCriterion(t *testing.T) {
	dir := t.TempDir()
	pwned := filepath.Join(dir, "criterion-pwned")
	evaluator := ScriptGateEvaluator{Workspace: dir, Timeout: time.Second}
	result, err := evaluator.EvaluateGate(GateEvaluation{
		Kinds:     []string{"lint"},
		Criterion: "touch " + pwned,
		Command:   "touch " + filepath.Join(dir, "legacy-pwned"),
	})
	if err != nil {
		t.Fatalf("evaluate missing argv gate: %v", err)
	}
	if result.Verdict != "fail" || result.PerKind["lint"] != "fail" {
		t.Fatalf("result = %+v, want fail", result)
	}
	if !strings.Contains(result.Reason, "missing command argv") || !strings.Contains(result.Reason, "legacy command string is not executable") {
		t.Fatalf("reason = %q, want missing argv + legacy rejection", result.Reason)
	}
	for _, path := range []string{pwned, filepath.Join(dir, "legacy-pwned")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("unexpected command side effect at %s: %v", path, err)
		}
	}
}

func TestScriptGateEvaluatorExecsArgvWithoutShellAndConstrainedCWD(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	argsPath := filepath.Join(dir, "args.txt")
	cwdPath := filepath.Join(dir, "cwd.txt")
	helper := writeScriptGateHelper(t, dir, `#!/bin/sh
pwd > "$1"
shift
printf '%s\n' "$@" > "$1"
`)
	pwned := filepath.Join(dir, "pwned")
	evaluator := ScriptGateEvaluator{Workspace: dir, Timeout: time.Second}
	result, err := evaluator.EvaluateGate(GateEvaluation{
		Kinds:       []string{"lint"},
		CommandArgv: []string{helper, cwdPath, argsPath, "literal;touch " + pwned, "$HOME", "*.go"},
		CommandCWD:  "subdir",
	})
	if err != nil {
		t.Fatalf("evaluate argv gate: %v", err)
	}
	if result.Verdict != "pass" || result.PerKind["lint"] != "pass" {
		t.Fatalf("result = %+v, want pass", result)
	}
	if got := strings.TrimSpace(readTestFile(t, cwdPath)); got != subdir {
		t.Fatalf("cwd = %q, want %q", got, subdir)
	}
	args := readTestFile(t, argsPath)
	for _, want := range []string{"literal;touch " + pwned, "$HOME", "*.go"} {
		if !strings.Contains(args, want) {
			t.Fatalf("args = %q, missing literal %q", args, want)
		}
	}
	if _, err := os.Stat(pwned); !os.IsNotExist(err) {
		t.Fatalf("shell metacharacter side effect exists: %v", err)
	}

	outside := t.TempDir()
	result, err = evaluator.EvaluateGate(GateEvaluation{
		Kinds:       []string{"lint"},
		CommandArgv: []string{helper, cwdPath, argsPath},
		CommandCWD:  outside,
	})
	if err != nil {
		t.Fatalf("evaluate absolute cwd escape: %v", err)
	}
	if result.Verdict != "fail" || !strings.Contains(result.Reason, "outside workspace") {
		t.Fatalf("absolute cwd escape result = %+v, want fail outside workspace", result)
	}

	symlink := filepath.Join(dir, "escape")
	if err := os.Symlink(outside, symlink); err != nil {
		t.Fatalf("symlink escape: %v", err)
	}
	result, err = evaluator.EvaluateGate(GateEvaluation{
		Kinds:       []string{"lint"},
		CommandArgv: []string{helper, cwdPath, argsPath},
		CommandCWD:  "escape",
	})
	if err != nil {
		t.Fatalf("evaluate symlink cwd escape: %v", err)
	}
	if result.Verdict != "fail" || !strings.Contains(result.Reason, "outside workspace") {
		t.Fatalf("symlink cwd escape result = %+v, want fail outside workspace", result)
	}
}

func TestScriptGateEvaluatorAllowsShellOnlyWithExplicitShellCommand(t *testing.T) {
	dir := t.TempDir()
	evaluator := ScriptGateEvaluator{Workspace: dir, Timeout: time.Second}
	result, err := evaluator.EvaluateGate(GateEvaluation{
		Kinds:        []string{"script"},
		CommandShell: "printf shell-ok > shell.out",
	})
	if err != nil {
		t.Fatalf("evaluate shell gate: %v", err)
	}
	if result.Verdict != "pass" || result.PerKind["script"] != "pass" {
		t.Fatalf("result = %+v, want pass", result)
	}
	if got := strings.TrimSpace(readTestFile(t, filepath.Join(dir, "shell.out"))); got != "shell-ok" {
		t.Fatalf("shell side effect = %q, want shell-ok", got)
	}
}

func TestScriptGateEvaluatorRejectsShellInterpreterInArgvMode(t *testing.T) {
	dir := t.TempDir()
	pwned := filepath.Join(dir, "pwned")
	shellLink := filepath.Join(dir, "not-a-shell-name")
	if err := os.Symlink("/bin/sh", shellLink); err != nil {
		t.Fatalf("shell symlink: %v", err)
	}
	tests := []struct {
		name string
		argv []string
	}{
		{name: "bash", argv: []string{"bash", "-lc", "touch " + pwned}},
		{name: "bin-sh", argv: []string{"/bin/sh", "-c", "touch " + pwned}},
		{name: "env-bash", argv: []string{"env", "bash", "-lc", "touch " + pwned}},
		{name: "busybox-sh", argv: []string{"busybox", "sh", "-c", "touch " + pwned}},
		{name: "symlink", argv: []string{shellLink, "-c", "touch " + pwned}},
		{name: "powershell", argv: []string{"powershell.exe", "-Command", "New-Item " + pwned}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator := ScriptGateEvaluator{Workspace: dir, Timeout: time.Second}
			result, err := evaluator.EvaluateGate(GateEvaluation{
				Kinds:       []string{"script"},
				CommandArgv: tt.argv,
			})
			if err != nil {
				t.Fatalf("evaluate rejected shell argv: %v", err)
			}
			if result.Verdict != "fail" || !strings.Contains(result.Reason, "shell interpreter requires explicit shell command") {
				t.Fatalf("result = %+v, want shell interpreter rejection", result)
			}
			if _, err := os.Stat(pwned); !os.IsNotExist(err) {
				t.Fatalf("shell argv side effect exists: %v", err)
			}
		})
	}
}

func TestScriptGateEvaluatorCapsAndRedactsOutputEvidence(t *testing.T) {
	dir := t.TempDir()
	helper := writeScriptGateHelper(t, dir, `#!/bin/sh
printf 'api_key=sk-scriptgate1234567890\n'
python3 - <<'PY'
print('X' * 4096)
PY
`)
	evaluator := ScriptGateEvaluator{Workspace: dir, Timeout: time.Second, OutputCapBytes: 128}
	result, err := evaluator.EvaluateGate(GateEvaluation{
		Kinds:       []string{"lint"},
		CommandArgv: []string{helper},
	})
	if err != nil {
		t.Fatalf("evaluate noisy gate: %v", err)
	}
	if result.Verdict != "pass" || result.PerKind["lint"] != "pass" {
		t.Fatalf("result = %+v, want pass", result)
	}
	if !strings.Contains(result.Reason, "truncated") {
		t.Fatalf("reason = %q, want truncation marker", result.Reason)
	}
	if strings.Contains(result.Reason, "sk-scriptgate1234567890") || !strings.Contains(result.Reason, "[REDACTED]") {
		t.Fatalf("reason did not redact secret-shaped output: %q", result.Reason)
	}
	if len(result.Reason) > 540 {
		t.Fatalf("reason length = %d, want bounded output evidence: %q", len(result.Reason), result.Reason)
	}
}

func TestScriptGateEvaluatorTimeoutKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	childSurvived := filepath.Join(dir, "child-survived")
	evaluator := ScriptGateEvaluator{Workspace: dir, Timeout: 50 * time.Millisecond, OutputCapBytes: 256}
	result, err := evaluator.EvaluateGate(GateEvaluation{
		Kinds:        []string{"script"},
		CommandShell: "(sleep 0.25; touch " + childSurvived + ") & sleep 5",
	})
	if err != nil {
		t.Fatalf("evaluate timed out gate: %v", err)
	}
	if result.Verdict != "fail" || result.PerKind["script"] != "fail" || !strings.Contains(result.Reason, "timed out") {
		t.Fatalf("result = %+v, want timeout fail", result)
	}
	time.Sleep(500 * time.Millisecond)
	if _, err := os.Stat(childSurvived); !os.IsNotExist(err) {
		t.Fatalf("timeout left shell child running; marker stat err=%v", err)
	}
}

func TestScriptGateEvaluatorSanitizesCommandEnvironment(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "env.txt")
	helper := writeScriptGateHelper(t, dir, `#!/bin/sh
printf 'token=%s\napi=%s\nsafe=%s\npath=%s\n' "$CHROTE_CONTEXT_API_TOKEN" "$API_KEY" "$SAFE_GATE_ENV" "$PATH" > "$1"
`)
	t.Setenv("CHROTE_CONTEXT_API_TOKEN", "secret-context-token")
	t.Setenv("API_KEY", "secret-api-key")
	t.Setenv("SAFE_GATE_ENV", "visible")
	t.Setenv("CHROTE_FORMATIONS_GATE_ENV_ALLOWLIST", "SAFE_GATE_ENV")
	evaluator := ScriptGateEvaluator{Workspace: dir, Timeout: time.Second}
	result, err := evaluator.EvaluateGate(GateEvaluation{
		Kinds:       []string{"lint"},
		CommandArgv: []string{helper, out},
	})
	if err != nil {
		t.Fatalf("evaluate env gate: %v", err)
	}
	if result.Verdict != "pass" {
		t.Fatalf("result = %+v, want pass", result)
	}
	env := readTestFile(t, out)
	if strings.Contains(env, "secret-context-token") || strings.Contains(env, "secret-api-key") {
		t.Fatalf("gate env leaked secret-shaped variables:\n%s", env)
	}
	if !strings.Contains(env, "safe=visible") || !strings.Contains(env, "path=") {
		t.Fatalf("gate env missing allowlisted safe var or PATH:\n%s", env)
	}
}

func TestScriptGateEvaluatorFailsOnNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	helper := writeScriptGateHelper(t, dir, `#!/bin/sh
printf nope >&2
exit 3
`)
	evaluator := ScriptGateEvaluator{Workspace: dir, Timeout: time.Second}
	result, err := evaluator.EvaluateGate(GateEvaluation{
		Kinds:       []string{"lint"},
		CommandArgv: []string{helper},
	})
	if err != nil {
		t.Fatalf("evaluate lint gate: %v", err)
	}
	if result.Verdict != "fail" || result.PerKind["lint"] != "fail" || !strings.Contains(result.Reason, "exit status 3") {
		t.Fatalf("result = %+v, want fail with exit status", result)
	}
}

func TestConfiguredScriptGateEvaluatorFromEnv(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("CHROTE_FORMATIONS_SCRIPT_GATES", "")
	if got := NewConfiguredGateEvaluatorFromEnv(workspace); got != nil {
		t.Fatalf("gate evaluator = %#v, want nil without opt-in", got)
	}
	t.Setenv("CHROTE_FORMATIONS_SCRIPT_GATES", "allow")
	t.Setenv("CHROTE_FORMATIONS_GATE_TIMEOUT_SECONDS", "7")
	t.Setenv("CHROTE_FORMATIONS_GATE_OUTPUT_CAP_BYTES", "64")
	evaluator, ok := NewConfiguredGateEvaluatorFromEnv(workspace).(ScriptGateEvaluator)
	if !ok {
		t.Fatalf("configured evaluator type = %T, want ScriptGateEvaluator", NewConfiguredGateEvaluatorFromEnv(workspace))
	}
	if evaluator.Workspace != workspace || evaluator.Timeout != 7*time.Second || evaluator.OutputCapBytes != 64 {
		t.Fatalf("configured evaluator = %+v, want workspace/timeout/cap from env", evaluator)
	}
}

func writeScriptGateHelper(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "helper.sh")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	return path
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(got)
}
