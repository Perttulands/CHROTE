package formations

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultGateCommandTimeout        = 5 * time.Minute
	defaultGateCommandOutputCapBytes = 8192
)

type ScriptGateEvaluator struct {
	Workspace      string
	Timeout        time.Duration
	OutputCapBytes int
}

func NewConfiguredGateEvaluatorFromEnv(workspace string) GateEvaluator {
	if !tmuxProdSmokeAllowed(os.Getenv("CHROTE_FORMATIONS_SCRIPT_GATES")) {
		return nil
	}
	timeout := defaultGateCommandTimeout
	if raw := strings.TrimSpace(os.Getenv("CHROTE_FORMATIONS_GATE_TIMEOUT_SECONDS")); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			timeout = time.Duration(seconds) * time.Second
		}
	}
	capBytes := defaultGateCommandOutputCapBytes
	if raw := strings.TrimSpace(os.Getenv("CHROTE_FORMATIONS_GATE_OUTPUT_CAP_BYTES")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			capBytes = parsed
		}
	}
	return ScriptGateEvaluator{Workspace: workspace, Timeout: timeout, OutputCapBytes: capBytes}
}

func (e ScriptGateEvaluator) EvaluateGate(req GateEvaluation) (GateEvaluationResult, error) {
	if reason, ok := unsupportedGateKindReason(req.Kinds); ok {
		return e.fail(req.Kinds, reason), nil
	}
	cwd, err := e.commandCWD(req.CommandCWD)
	if err != nil {
		return e.fail(req.Kinds, err.Error()), nil
	}
	argv, shell, err := e.command(req)
	if err != nil {
		return e.fail(req.Kinds, err.Error()), nil
	}

	timeout := e.Timeout
	if timeout <= 0 {
		timeout = defaultGateCommandTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var cmd *exec.Cmd
	if shell != "" {
		cmd = exec.Command("bash", "-lc", shell)
	} else {
		cmd = exec.Command(argv[0], argv[1:]...)
	}
	cmd.Dir = cwd
	cmd.Env = scriptGateEnv(cwd)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	capBytes := e.outputCapBytes()
	stdout := newCappedGateOutput(capBytes)
	stderr := newCappedGateOutput(capBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err = runGateCommand(ctx, cmd)
	output := summarizeGateCommandOutput(stdout.String(), stderr.String(), capBytes, stdout.Truncated() || stderr.Truncated())
	if ctx.Err() == context.DeadlineExceeded {
		return e.fail(req.Kinds, fmt.Sprintf("command timed out after %s: %s", timeout, output)), nil
	}
	if err != nil {
		return e.fail(req.Kinds, fmt.Sprintf("command failed: %v\n%s", err, output)), nil
	}
	return e.pass(req.Kinds, "command passed\n"+output), nil
}

func runGateCommand(ctx context.Context, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Process.Kill()
		}
		<-done
		return ctx.Err()
	}
}

func (e ScriptGateEvaluator) command(req GateEvaluation) ([]string, string, error) {
	hasArgv := len(req.CommandArgv) > 0
	hasShell := strings.TrimSpace(req.CommandShell) != ""
	if hasArgv && hasShell {
		return nil, "", fmt.Errorf("gate command must use either command argv or explicit shell command, not both")
	}
	if hasShell {
		return nil, req.CommandShell, nil
	}
	if !hasArgv {
		reason := "missing command argv"
		if strings.TrimSpace(req.Command) != "" {
			reason += "; legacy command string is not executable by script gates; use commandArgv or commandShell"
		}
		return nil, "", fmt.Errorf("%s", reason)
	}
	argv := make([]string, 0, len(req.CommandArgv))
	for _, arg := range req.CommandArgv {
		argv = append(argv, arg)
	}
	argv, err := e.expandGateCommandArgv(argv, req)
	if err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(argv[0]) == "" {
		return nil, "", fmt.Errorf("command argv executable is missing")
	}
	if e.argvInvokesShell(argv) {
		return nil, "", fmt.Errorf("shell interpreter requires explicit shell command")
	}
	return argv, "", nil
}

func (e ScriptGateEvaluator) expandGateCommandArgv(argv []string, req GateEvaluation) ([]string, error) {
	if len(argv) > 0 && (strings.Contains(argv[0], "{{") || strings.Contains(argv[0], "}}")) {
		return nil, fmt.Errorf("gate command executable must be fixed operator-authored argv")
	}
	artifactRef := ""
	if slicesContain(argv, "{{input.artifactRef}}") {
		if e.artifactPlaceholderSelectsInterpreterProgram(argv) {
			return nil, fmt.Errorf("input artifactRef cannot select an interpreter program or code argument")
		}
		var err error
		artifactRef, err = e.resolveGateArtifactRef(req.Input.ArtifactRef)
		if err != nil {
			return nil, err
		}
	}
	values := map[string]string{
		"{{run.id}}":            req.RunID,
		"{{gate.id}}":           req.GateID,
		"{{input.artifactRef}}": artifactRef,
	}
	expanded := make([]string, len(argv))
	for index, arg := range argv {
		if index == 0 && (strings.Contains(arg, "{{") || strings.Contains(arg, "}}")) {
			return nil, fmt.Errorf("gate command executable must be fixed operator-authored argv")
		}
		if arg == "{{input.ref}}" {
			return nil, fmt.Errorf("gate command placeholder {{input.ref}} is not allowed in argv")
		}
		if value, ok := values[arg]; ok {
			if strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("gate command placeholder %s has no value", arg)
			}
			expanded[index] = value
			continue
		}
		if strings.Contains(arg, "{{") || strings.Contains(arg, "}}") {
			return nil, fmt.Errorf("gate command placeholders must occupy a whole argument: %q", arg)
		}
		expanded[index] = arg
	}
	return expanded, nil
}

func (e ScriptGateEvaluator) artifactPlaceholderSelectsInterpreterProgram(argv []string) bool {
	placeholderIndex := -1
	for index, arg := range argv {
		if arg == "{{input.artifactRef}}" {
			placeholderIndex = index
			break
		}
	}
	if placeholderIndex < 1 {
		return false
	}
	executable := e.resolvedExecutableBase(argv[0])
	if commandWrapper(executable) || artifactIsCodeFileArgument(executable, argv, placeholderIndex) {
		return true
	}
	interpreter := executable == "node" || executable == "nodejs" || executable == "ruby" || executable == "perl" || executable == "php" || executable == "lua" || strings.HasPrefix(executable, "python")
	if !interpreter {
		return false
	}
	fixedProgram := false
	for _, arg := range argv[1:placeholderIndex] {
		switch arg {
		case "-c", "-e", "--eval", "--execute":
			return true
		}
		if !strings.HasPrefix(arg, "-") {
			fixedProgram = true
		}
	}
	return !fixedProgram
}

func commandWrapper(executable string) bool {
	switch executable {
	case "env", "busybox", "nice", "nohup", "setsid", "stdbuf", "timeout", "sudo", "doas", "xargs":
		return true
	default:
		return false
	}
}

func artifactIsCodeFileArgument(executable string, argv []string, placeholderIndex int) bool {
	if placeholderIndex <= 1 {
		return false
	}
	previous := argv[placeholderIndex-1]
	switch executable {
	case "awk", "gawk", "mawk", "nawk", "sed", "make", "gmake":
		return previous == "-f" || previous == "--file"
	case "cmake":
		return previous == "-P"
	case "find":
		for _, arg := range argv[1:placeholderIndex] {
			if arg == "-exec" || arg == "-execdir" {
				return true
			}
		}
	}
	return false
}

func (e ScriptGateEvaluator) resolvedExecutableBase(executable string) string {
	path := executable
	if !strings.Contains(path, string(filepath.Separator)) {
		resolved, err := exec.LookPath(path)
		if err != nil {
			return normalizedExecutableBase(executable)
		}
		path = resolved
	} else if !filepath.IsAbs(path) {
		cwd, err := e.commandCWD("")
		if err != nil {
			return normalizedExecutableBase(executable)
		}
		path = filepath.Join(cwd, path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return normalizedExecutableBase(executable)
	}
	return normalizedExecutableBase(resolved)
}

func slicesContain(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (e ScriptGateEvaluator) resolveGateArtifactRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("gate command placeholder {{input.artifactRef}} has no value")
	}
	if filepath.IsAbs(ref) {
		return "", fmt.Errorf("input artifactRef must be workspace-relative")
	}
	cleaned := filepath.Clean(filepath.FromSlash(ref))
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("input artifactRef is outside workspace")
	}
	workspace, err := e.commandCWD("")
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(workspace, cleaned)
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("input artifactRef is unavailable: %w", err)
	}
	inside, err := pathInside(resolved, workspace)
	if err != nil {
		return "", err
	}
	if !inside {
		return "", fmt.Errorf("input artifactRef resolves outside workspace")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("input artifactRef is not a regular file")
	}
	return resolved, nil
}

func (e ScriptGateEvaluator) argvInvokesShell(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	base := e.resolvedExecutableBase(argv[0])
	if shellInterpreter(base) {
		return true
	}
	if base == "env" {
		nested, valid := envCommandArgv(argv[1:])
		return !valid || e.argvInvokesShell(nested)
	}
	if base == "busybox" && len(argv) > 1 {
		return e.argvInvokesShell(argv[1:])
	}
	return false
}

func envCommandArgv(argv []string) ([]string, bool) {
	for index := 0; index < len(argv); {
		arg := argv[index]
		switch {
		case arg == "--":
			index++
			if index >= len(argv) {
				return nil, false
			}
			return argv[index:], true
		case arg == "-i" || arg == "--ignore-environment" || arg == "-0" || arg == "--null":
			index++
		case arg == "-u" || arg == "--unset" || arg == "-C" || arg == "--chdir":
			if index+1 >= len(argv) {
				return nil, false
			}
			index += 2
		case strings.HasPrefix(arg, "--unset=") || strings.HasPrefix(arg, "--chdir="):
			index++
		case strings.Contains(arg, "=") && !strings.HasPrefix(arg, "-"):
			index++
		case strings.HasPrefix(arg, "-"):
			return nil, false
		default:
			return argv[index:], true
		}
	}
	return nil, false
}

func (e ScriptGateEvaluator) commandCWD(commandCWD string) (string, error) {
	workspace := strings.TrimSpace(e.Workspace)
	if workspace == "" {
		workspace = "."
	}
	workspace, err := filepath.Abs(workspace)
	if err != nil {
		return "", fmt.Errorf("invalid workspace: %w", err)
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", fmt.Errorf("workspace is unavailable: %w", err)
	}
	candidate := workspace
	if strings.TrimSpace(commandCWD) != "" {
		candidate = commandCWD
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(workspace, candidate)
		}
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("invalid command cwd: %w", err)
	}
	candidate, err = filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("command cwd is unavailable: %w", err)
	}
	inside, err := pathInside(candidate, workspace)
	if err != nil {
		return "", err
	}
	if !inside {
		return "", fmt.Errorf("command cwd %q is outside workspace %q", candidate, workspace)
	}
	return candidate, nil
}

func pathInside(path, root string) (bool, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false, err
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))), nil
}

func shellInterpreter(executable string) bool {
	base := normalizedExecutableBase(executable)
	switch base {
	case "sh", "bash", "dash", "zsh", "fish", "pwsh", "powershell", "cmd":
		return true
	default:
		return false
	}
}

func normalizedExecutableBase(executable string) string {
	base := strings.ToLower(filepath.Base(executable))
	return strings.TrimSuffix(base, ".exe")
}

func unsupportedGateKindReason(kinds []string) (string, bool) {
	for _, kind := range kinds {
		switch kind {
		case "code", "lint", "script":
			// Supported by configured script gate commands.
		default:
			return fmt.Sprintf("unsupported script gate kind %q", kind), true
		}
	}
	return "", false
}

func scriptGateEnv(cwd string) []string {
	allow := map[string]bool{
		"HOME": true, "LANG": true, "LC_ALL": true, "PATH": true,
		"TERM": true, "TMPDIR": true, "USER": true,
	}
	for _, name := range splitLabCSV(os.Getenv("CHROTE_FORMATIONS_GATE_ENV_ALLOWLIST")) {
		allow[name] = true
	}
	env := make([]string, 0, len(allow)+1)
	seen := map[string]bool{}
	for _, item := range os.Environ() {
		name, _, ok := strings.Cut(item, "=")
		if !ok || !allow[name] || blockedGateEnvName(name) {
			continue
		}
		env = append(env, item)
		seen[name] = true
	}
	if !seen["PATH"] {
		env = append(env, "PATH=/usr/local/bin:/usr/bin:/bin")
	}
	env = append(env, "PWD="+cwd)
	return env
}

func blockedGateEnvName(name string) bool {
	upper := strings.ToUpper(name)
	if strings.HasPrefix(upper, "CHROTE_") && !explicitlyAllowedGateEnvName(upper) {
		return true
	}
	for _, needle := range []string{"TOKEN", "SECRET", "PASSWORD", "PASSWD", "API_KEY", "ACCESS_KEY", "PRIVATE_KEY"} {
		if strings.Contains(upper, needle) {
			return true
		}
	}
	return false
}

func explicitlyAllowedGateEnvName(upper string) bool {
	for _, raw := range splitLabCSV(os.Getenv("CHROTE_FORMATIONS_GATE_ENV_ALLOWLIST")) {
		if strings.ToUpper(raw) == upper {
			return true
		}
	}
	return false
}

func (e ScriptGateEvaluator) outputCapBytes() int {
	if e.OutputCapBytes > 0 {
		return e.OutputCapBytes
	}
	return defaultGateCommandOutputCapBytes
}

func (e ScriptGateEvaluator) fail(kinds []string, reason string) GateEvaluationResult {
	perKind := make(map[string]string, len(kinds))
	for _, kind := range kinds {
		perKind[kind] = "fail"
	}
	return GateEvaluationResult{Verdict: "fail", PerKind: perKind, Reason: truncateGateReason(redactLedgerText(reason))}
}

func (e ScriptGateEvaluator) pass(kinds []string, reason string) GateEvaluationResult {
	perKind := make(map[string]string, len(kinds))
	for _, kind := range kinds {
		perKind[kind] = "pass"
	}
	return GateEvaluationResult{Verdict: "pass", PerKind: perKind, Reason: truncateGateReason(redactLedgerText(reason))}
}

type cappedGateOutput struct {
	limit     int
	data      []byte
	truncated bool
}

func newCappedGateOutput(limit int) *cappedGateOutput {
	if limit <= 0 {
		limit = defaultGateCommandOutputCapBytes
	}
	// Keep a small redaction boundary margin so tokens split at the visible cap
	// still have a chance to match the redaction regex before final truncation.
	return &cappedGateOutput{limit: limit + 256}
}

func (w *cappedGateOutput) Write(p []byte) (int, error) {
	remaining := w.limit - len(w.data)
	if remaining > 0 {
		if len(p) <= remaining {
			w.data = append(w.data, p...)
		} else {
			w.data = append(w.data, p[:remaining]...)
			w.truncated = true
		}
	} else if len(p) > 0 {
		w.truncated = true
	}
	return len(p), nil
}

func (w *cappedGateOutput) String() string { return string(w.data) }

func (w *cappedGateOutput) Truncated() bool { return w.truncated }

func summarizeGateCommandOutput(stdout, stderr string, capBytes int, alreadyTruncated bool) string {
	combined := strings.TrimSpace(strings.Join([]string{stdout, stderr}, "\n"))
	if combined == "" {
		return "no output"
	}
	combined = redactLedgerText(combined)
	truncated := alreadyTruncated
	if capBytes <= 0 {
		capBytes = defaultGateCommandOutputCapBytes
	}
	if len(combined) > capBytes {
		combined = combined[:capBytes]
		truncated = true
	}
	lines := strings.Split(combined, "\n")
	const maxLines = 40
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		truncated = true
	}
	out := strings.TrimSpace(strings.Join(lines, "\n"))
	if truncated {
		out = strings.TrimSpace(out + "\n... truncated output")
	}
	return out
}

func truncateGateReason(reason string) string {
	const maxReasonBytes = 512
	if len(reason) <= maxReasonBytes {
		return reason
	}
	return strings.TrimSpace(reason[:maxReasonBytes]) + "... truncated reason"
}
