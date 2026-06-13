package formations

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	errScriptGateMissingConfig = errors.New("missing script config")
	errScriptGateInvalidConfig = errors.New("invalid script config")
)

func (e *RunEngine) evaluateScriptGate(req GateEvaluation, config *GateScriptConfig) (GateEvaluationResult, error) {
	if config == nil {
		return GateEvaluationResult{}, errScriptGateMissingConfig
	}
	if e == nil || e.store == nil {
		return GateEvaluationResult{}, fmt.Errorf("%w: store required", errScriptGateInvalidConfig)
	}
	resolved, err := resolveScriptGateConfig(e.store.Workspace, config)
	if err != nil {
		return GateEvaluationResult{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.TimeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, config.Command[0], config.Command[1:]...)
	cmd.Dir = resolved.cwd
	cmd.Env = scriptGateEnv(resolved.root, resolved.cwd, req)
	var stdout, stderr cappedBuffer
	stdout.limit = config.OutputLimitBytes
	stderr.limit = config.OutputLimitBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	timedOut := ctx.Err() == context.DeadlineExceeded
	status := "pass"
	verdict := "pass"
	exitCode := 0
	reason := "script exit 0"
	if timedOut {
		status = "timeout"
		verdict = "fail"
		exitCode = -1
		reason = "script timed out"
	} else if runErr != nil {
		verdict = "fail"
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			status = "exit"
			exitCode = exitErr.ExitCode()
			reason = fmt.Sprintf("script exit %d", exitCode)
		} else {
			status = "exec_error"
			exitCode = -1
			reason = redactLedgerText(runErr.Error())
		}
	}

	stdoutText := redactLedgerText(stdout.String())
	stderrText := redactLedgerText(stderr.String())
	outputText := strings.TrimSpace(stdoutText)
	if outputText == "" {
		outputText = strings.TrimSpace(stderrText)
	}
	if outputText == "" {
		outputText = reason
	}
	outputPort := verdict
	return GateEvaluationResult{
		Verdict: verdict,
		Reason:  reason,
		PerKind: map[string]string{
			"script": verdict,
		},
		Outputs: map[string]FormationOutputPayload{
			outputPort: {
				Ref:  fmt.Sprintf("ledger://%s/%s/%s", req.RunID, req.GateID, outputPort),
				Text: outputText,
			},
		},
		Evidence: map[string]any{
			"status":           status,
			"exitCode":         exitCode,
			"timedOut":         timedOut,
			"timeoutSeconds":   config.TimeoutSeconds,
			"outputLimitBytes": config.OutputLimitBytes,
			"stdout":           strings.TrimSpace(stdoutText),
			"stderr":           strings.TrimSpace(stderrText),
			"stdoutTruncated":  stdout.truncated,
			"stderrTruncated":  stderr.truncated,
			"argv0":            filepath.Base(config.Command[0]),
			"argvSha256":       etag([]byte(strings.Join(config.Command, "\x00"))),
			"root":             resolved.rootForLedger,
			"cwd":              resolved.cwdForLedger,
		},
	}, nil
}

func scriptGateErrorCode(err error) string {
	switch {
	case errors.Is(err, errScriptGateMissingConfig):
		return "missing_script_config"
	case errors.Is(err, errScriptGateInvalidConfig):
		return "invalid_script_config"
	default:
		return "script_gate_error"
	}
}

type resolvedScriptGateConfig struct {
	root          string
	cwd           string
	rootForLedger string
	cwdForLedger  string
}

func resolveScriptGateConfig(workspace string, config *GateScriptConfig) (resolvedScriptGateConfig, error) {
	if len(config.Command) == 0 {
		return resolvedScriptGateConfig{}, errScriptGateMissingConfig
	}
	for _, part := range config.Command {
		if strings.TrimSpace(part) == "" {
			return resolvedScriptGateConfig{}, fmt.Errorf("%w: command contains an empty argv part", errScriptGateInvalidConfig)
		}
	}
	if disallowedInlineShell(config.Command) {
		return resolvedScriptGateConfig{}, fmt.Errorf("%w: inline shell commands are not allowed", errScriptGateInvalidConfig)
	}
	if strings.TrimSpace(config.Root) == "" {
		return resolvedScriptGateConfig{}, fmt.Errorf("%w: scriptRoot is required", errScriptGateInvalidConfig)
	}
	if config.TimeoutSeconds <= 0 {
		return resolvedScriptGateConfig{}, fmt.Errorf("%w: scriptTimeoutSeconds must be positive", errScriptGateInvalidConfig)
	}
	if config.OutputLimitBytes <= 0 {
		return resolvedScriptGateConfig{}, fmt.Errorf("%w: scriptOutputLimitBytes must be positive", errScriptGateInvalidConfig)
	}

	workspaceAbs, err := cleanExistingDir(workspace)
	if err != nil {
		return resolvedScriptGateConfig{}, fmt.Errorf("%w: workspace: %v", errScriptGateInvalidConfig, err)
	}
	root := config.Root
	if !filepath.IsAbs(root) {
		root = filepath.Join(workspaceAbs, root)
	}
	rootAbs, err := cleanExistingDir(root)
	if err != nil {
		return resolvedScriptGateConfig{}, fmt.Errorf("%w: scriptRoot: %v", errScriptGateInvalidConfig, err)
	}
	if !pathWithin(workspaceAbs, rootAbs) {
		return resolvedScriptGateConfig{}, fmt.Errorf("%w: scriptRoot escapes workspace", errScriptGateInvalidConfig)
	}

	cwd := config.Cwd
	if strings.TrimSpace(cwd) == "" {
		cwd = "."
	}
	if !filepath.IsAbs(cwd) {
		cwd = filepath.Join(rootAbs, cwd)
	}
	cwdAbs, err := cleanExistingDir(cwd)
	if err != nil {
		return resolvedScriptGateConfig{}, fmt.Errorf("%w: scriptCwd: %v", errScriptGateInvalidConfig, err)
	}
	if !pathWithin(rootAbs, cwdAbs) {
		return resolvedScriptGateConfig{}, fmt.Errorf("%w: scriptCwd escapes scriptRoot", errScriptGateInvalidConfig)
	}

	rootRel, err := filepath.Rel(workspaceAbs, rootAbs)
	if err != nil {
		rootRel = rootAbs
	}
	cwdRel, err := filepath.Rel(workspaceAbs, cwdAbs)
	if err != nil {
		cwdRel = cwdAbs
	}
	return resolvedScriptGateConfig{
		root:          rootAbs,
		cwd:           cwdAbs,
		rootForLedger: filepath.ToSlash(rootRel),
		cwdForLedger:  filepath.ToSlash(cwdRel),
	}, nil
}

func cleanExistingDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		abs = resolved
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}
	return filepath.Clean(abs), nil
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func disallowedInlineShell(command []string) bool {
	if len(command) < 2 {
		return false
	}
	base := strings.ToLower(filepath.Base(command[0]))
	switch base {
	case "sh", "bash", "dash", "zsh", "ksh", "fish":
		for _, arg := range command[1:] {
			if !strings.HasPrefix(arg, "-") {
				return false
			}
			if strings.Contains(arg, "c") {
				return true
			}
		}
		return false
	case "cmd", "cmd.exe":
		return strings.EqualFold(command[1], "/c")
	case "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return strings.EqualFold(command[1], "-command")
	default:
		return false
	}
}

func scriptGateEnv(root, cwd string, req GateEvaluation) []string {
	env := []string{
		"HOME=" + root,
		"PWD=" + cwd,
		"TMPDIR=" + root,
		"CHROTE_FORMATIONS_RUN_ID=" + req.RunID,
		"CHROTE_FORMATIONS_GATE_ID=" + req.GateID,
	}
	if path := os.Getenv("PATH"); path != "" {
		env = append(env, "PATH="+path)
	}
	return env
}

type cappedBuffer struct {
	limit     int
	data      []byte
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		b.truncated = true
		return len(p), nil
	}
	remaining := b.limit - len(b.data)
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		b.data = append(b.data, p[:remaining]...)
		b.truncated = true
		return len(p), nil
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	return string(b.data)
}
