package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	osuser "os/user"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chrote/server/internal/core"
)

// Supervision of a locked session belongs to systemd, not to this process
// (ADR-0014). This file is the entire control path: write a typed per-agent
// config, enable or disable one unit instantiated from the CHROTE-owned
// template, and read health back. There is no loop here and no retry state --
// those are `Restart=` and the journal.

const (
	agentUnitTemplate    = "chrote-agent@"
	agentUnitSuffix      = ".service"
	agentUnitConfigMode  = 0o600
	agentUnitDirMode     = 0o700
	agentSystemctlBudget = 15 * time.Second

	// The operator-installed setuid-free helper that reaches another account's
	// user manager. Resolved from PATH so the deployment, not the binary,
	// decides where it lives.
	agentUnitHelperBinary = "chrote-agentctl"

	agentHealthHealthy  = "healthy"
	agentHealthDegraded = "degraded"
	agentHealthFailed   = "failed"
	agentHealthInactive = "inactive"
	agentHealthUnlocked = "unlocked"
)

// A unix account name, deliberately stricter than POSIX allows: this value
// becomes an argument to a privileged command, so the useful question is not
// "could an account be named this" but "can this reach a shell". It cannot.
var agentUnixUserRegex = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// The native transcript id, as the agent CLIs accept it. Anything starting with
// a dash would be read as a flag by the agent, so the leading character is
// constrained separately.
var agentNativeSessionIDRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

// systemctlRunner runs one systemctl --user invocation against a target user's
// manager. It is an injection point so tests never touch a real bus, and so the
// privileged mechanism (how CHROTE reaches another user's manager) stays in one
// place rather than being spread through handlers.
type systemctlRunner func(ctx context.Context, unixUser string, args ...string) (string, error)

type systemdUnitState struct {
	ActiveState   string
	SubState      string
	UnitFileState string
	Started       bool
}

// agentUnitConfig is CHROTE's desired state for one locked session. It carries
// TYPED fields only: the launcher renders the resume argv from AgentKind plus
// AgentSessionID, so no rendered command is ever persisted and a config file
// cannot smuggle a command into a pane (ADR-0001, ADR-0014 decision 2).
type agentUnitConfig struct {
	Session        string
	UnixUser       string
	AgentKind      string
	AgentSessionID string
	AgentBin       string
	TmuxBin        string
	TmuxSocket     string
	Workdir        string
	KeeperUnit     string
	WatchInterval  int
}

// AgentUnitStatus is what the sessions API projects onto a session and the
// dashboard renders as the lock badge.
type AgentUnitStatus struct {
	Session     string `json:"session"`
	UnixUser    string `json:"unixUser,omitempty"`
	Locked      bool   `json:"locked"`
	Unit        string `json:"unit,omitempty"`
	Health      string `json:"health"`
	ActiveState string `json:"activeState,omitempty"`
	SubState    string `json:"subState,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

type agentUnitReceipt struct {
	Session        string `json:"session"`
	AgentKind      string `json:"agentKind"`
	AgentSessionID string `json:"agentSessionId"`
	PanePID        int    `json:"panePid"`
	StartedAt      string `json:"startedAt"`
}

type agentUnitController struct {
	root      string
	systemctl systemctlRunner
	mu        sync.Mutex
}

func newAgentUnitController(root string, runner systemctlRunner) *agentUnitController {
	if runner == nil {
		runner = runSystemctlForUser
	}
	return &agentUnitController{root: strings.TrimSpace(root), systemctl: runner}
}

// agentUnitName converts a session name into the one unit name it is allowed to
// produce. It re-validates rather than trusting its caller: a single accepted
// metacharacter here becomes a second systemctl argument, and this is the last
// place that can tell.
func agentUnitName(session string) (string, error) {
	session = strings.TrimSpace(session)
	if valid, msg := core.ValidateSessionName(session, "session name"); !valid {
		return "", fmt.Errorf("%s", msg)
	}
	// ValidateSessionName already pins ^[a-zA-Z0-9_-]{1,50}$; assert it here so a
	// future relaxation of that helper cannot silently widen what may become a
	// unit name.
	for _, r := range session {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return "", fmt.Errorf("session name %q cannot be part of a unit name", session)
		}
	}
	// A leading dash is legal in a session name and harmless in the unit name we
	// build here, because it ends up in the middle of a longer string that is
	// passed as one argument. It is refused anyway: the day some other code path
	// passes a bare session name to a command, "--user" stops being a name and
	// starts being a flag. The cost of forbidding it is zero.
	if strings.HasPrefix(session, "-") {
		return "", fmt.Errorf("session name %q may not start with a dash", session)
	}
	return agentUnitTemplate + session + agentUnitSuffix, nil
}

func validateAgentUnixUser(unixUser string) error {
	unixUser = strings.TrimSpace(unixUser)
	if !agentUnixUserRegex.MatchString(unixUser) {
		return fmt.Errorf("unix user %q is not a valid account name", unixUser)
	}
	return nil
}

func (c *agentUnitConfig) validate() error {
	if _, err := agentUnitName(c.Session); err != nil {
		return err
	}
	if err := validateAgentUnixUser(c.UnixUser); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(c.AgentKind)) {
	case "codex", "claude":
	default:
		return fmt.Errorf("unsupported agent kind %q", c.AgentKind)
	}
	if !agentNativeSessionIDRegex.MatchString(strings.TrimSpace(c.AgentSessionID)) {
		return fmt.Errorf("agent session id %q is not a canonical native session id", c.AgentSessionID)
	}
	for label, value := range map[string]string{
		"agent binary": c.AgentBin,
		"tmux binary":  c.TmuxBin,
		"tmux socket":  c.TmuxSocket,
		"workdir":      c.Workdir,
	} {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || !filepath.IsAbs(trimmed) || strings.ContainsAny(trimmed, "\n\r\x00") {
			return fmt.Errorf("%s must be an absolute path without control characters", label)
		}
	}
	if c.KeeperUnit != "" && strings.ContainsAny(c.KeeperUnit, "\n\r\x00") {
		return fmt.Errorf("keeper unit name contains control characters")
	}
	return nil
}

// configPath is where this agent's typed config lives. The result is proven to
// stay inside the controller's own directory: a session name is not a path
// component we are willing to trust twice.
func (c *agentUnitController) configPath(session, unixUser string) (string, error) {
	if err := validateAgentUnixUser(unixUser); err != nil {
		return "", err
	}
	if _, err := agentUnitName(session); err != nil {
		return "", err
	}
	if strings.TrimSpace(c.root) == "" {
		return "", fmt.Errorf("agent unit state directory is not configured")
	}
	base := filepath.Join(c.root, unixUser)
	path := filepath.Join(base, session+".conf")
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(base)+string(os.PathSeparator)) {
		return "", fmt.Errorf("refusing a config path outside %s", base)
	}
	return path, nil
}

func (c *agentUnitController) receiptPath(session, unixUser string) (string, error) {
	configPath, err := c.configPath(session, unixUser)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(configPath, ".conf") + ".receipt.json", nil
}

// Enable writes the typed config and hands supervision to systemd. The config is
// written first: a unit that starts before its config exists would fail loudly,
// but a unit enabled without config would be a lock CHROTE cannot explain.
func (c *agentUnitController) Enable(ctx context.Context, config agentUnitConfig) error {
	if err := config.validate(); err != nil {
		return err
	}
	unit, err := agentUnitName(config.Session)
	if err != nil {
		return err
	}
	configPath, err := c.configPath(config.Session, config.UnixUser)
	if err != nil {
		return err
	}
	receipt, err := c.receiptPath(config.Session, config.UnixUser)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(configPath), agentUnitDirMode); err != nil {
		return fmt.Errorf("agent config directory: %w", err)
	}
	if err := writeAgentUnitConfigFile(configPath, config, receipt); err != nil {
		return err
	}
	if _, err := c.systemctl(ctx, config.UnixUser, "enable", "--now", unit); err != nil {
		// Leave no half-lock behind: a config with no unit would make an
		// unlocked session look locked to OwnsUnit.
		_ = os.Remove(configPath)
		return fmt.Errorf("enable %s for %s: %w", unit, config.UnixUser, err)
	}
	return nil
}

// Disable stops supervising. It deliberately does NOT kill the tmux session or
// the agent: unlocking withdraws the promise, not the work (ADR-0014 decision 8).
func (c *agentUnitController) Disable(ctx context.Context, session, unixUser string) error {
	unit, err := agentUnitName(session)
	if err != nil {
		return err
	}
	configPath, err := c.configPath(session, unixUser)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
		// Not locked by us. Disabling a unit we never installed is not our call.
		return nil
	}
	if _, err := c.systemctl(ctx, unixUser, "disable", "--now", unit); err != nil {
		return fmt.Errorf("disable %s for %s: %w", unit, unixUser, err)
	}
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove agent config: %w", err)
	}
	if receipt, err := c.receiptPath(session, unixUser); err == nil {
		_ = os.Remove(receipt)
	}
	return nil
}

// OwnsUnit reports whether a unit is one CHROTE installed for this session --
// the precondition for treating an externally-managed session as restart-capable
// (ADR-0014 decision 3). Both halves are required: the name must match the
// template we own, and our config must be present. A hand-written unit that
// borrows our naming scheme still fails the second test.
func (c *agentUnitController) OwnsUnit(unit, session, unixUser string) bool {
	expected, err := agentUnitName(session)
	if err != nil || strings.TrimSpace(unit) != expected {
		return false
	}
	configPath, err := c.configPath(session, unixUser)
	if err != nil {
		return false
	}
	info, err := os.Lstat(configPath)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return true
}

// Status is read live from systemd, because a written registry is only as fresh
// as its writer. An active unit alone is not health: it proves a process
// started, not that the right transcript resumed, so the receipt is required
// (ADR-0014 decision 5).
func (c *agentUnitController) Status(ctx context.Context, session, unixUser string) (AgentUnitStatus, error) {
	status := AgentUnitStatus{Session: session, UnixUser: unixUser, Health: agentHealthUnlocked}
	unit, err := agentUnitName(session)
	if err != nil {
		return status, err
	}
	configPath, err := c.configPath(session, unixUser)
	if err != nil {
		return status, err
	}
	config, err := readAgentUnitConfigFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return status, nil
		}
		return status, err
	}
	status.Locked = true
	status.Unit = unit

	state, err := c.unitState(ctx, unixUser, unit)
	if err != nil {
		status.Health = agentHealthDegraded
		status.Detail = "unit status unavailable: " + err.Error()
		return status, nil
	}
	status.ActiveState = state.ActiveState
	status.SubState = state.SubState

	switch state.ActiveState {
	case "active", "activating", "reloading":
		receiptPath, receiptErr := c.receiptPath(session, unixUser)
		if receiptErr != nil {
			status.Health = agentHealthDegraded
			status.Detail = receiptErr.Error()
			return status, nil
		}
		receipt, receiptErr := readAgentUnitReceipt(receiptPath)
		switch {
		case receiptErr != nil:
			status.Health = agentHealthDegraded
			status.Detail = "unit is running but has not confirmed which transcript it resumed"
		case receipt.AgentSessionID != config.AgentSessionID:
			status.Health = agentHealthDegraded
			status.Detail = "unit is running a different transcript than the one this lock configured"
		default:
			status.Health = agentHealthHealthy
		}
	case "failed":
		status.Health = agentHealthFailed
		status.Detail = "unit failed; see the agent unit journal"
	default:
		status.Health = agentHealthInactive
		status.Detail = "unit is not running"
	}
	return status, nil
}

func (c *agentUnitController) unitState(ctx context.Context, unixUser, unit string) (systemdUnitState, error) {
	out, err := c.systemctl(ctx, unixUser, "show",
		"--property=ActiveState", "--property=SubState",
		"--property=UnitFileState", "--property=ExecMainStartTimestampMonotonic", unit)
	if err != nil {
		return systemdUnitState{}, err
	}
	state := systemdUnitState{}
	for _, line := range strings.Split(out, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}
		switch key {
		case "ActiveState":
			state.ActiveState = value
		case "SubState":
			state.SubState = value
		case "UnitFileState":
			state.UnitFileState = value
		case "ExecMainStartTimestampMonotonic":
			state.Started = value != "" && value != "0"
		}
	}
	return state, nil
}

// writeAgentUnitConfigFile writes the typed KEY=value file the launcher reads.
// Atomic rename so the launcher never reads a half-written config, and 0600
// because it names a workspace and a transcript.
func writeAgentUnitConfigFile(path string, config agentUnitConfig, receiptPath string) error {
	watch := config.WatchInterval
	if watch <= 0 {
		watch = 10
	}
	var builder strings.Builder
	builder.WriteString("# Written by CHROTE. Typed fields only: the launcher renders the resume\n")
	builder.WriteString("# argv from kind + native session id, so this file cannot carry a command.\n")
	fmt.Fprintf(&builder, "CHROTE_AGENT_SESSION=%s\n", config.Session)
	fmt.Fprintf(&builder, "CHROTE_AGENT_KIND=%s\n", strings.ToLower(strings.TrimSpace(config.AgentKind)))
	fmt.Fprintf(&builder, "CHROTE_AGENT_SESSION_ID=%s\n", strings.TrimSpace(config.AgentSessionID))
	fmt.Fprintf(&builder, "CHROTE_AGENT_BIN=%s\n", config.AgentBin)
	fmt.Fprintf(&builder, "CHROTE_AGENT_TMUX_BIN=%s\n", config.TmuxBin)
	fmt.Fprintf(&builder, "CHROTE_AGENT_TMUX_SOCKET=%s\n", config.TmuxSocket)
	fmt.Fprintf(&builder, "CHROTE_AGENT_WORKDIR=%s\n", config.Workdir)
	fmt.Fprintf(&builder, "CHROTE_AGENT_RECEIPT_PATH=%s\n", receiptPath)
	fmt.Fprintf(&builder, "CHROTE_AGENT_WATCH_INTERVAL=%d\n", watch)
	if strings.TrimSpace(config.KeeperUnit) != "" {
		fmt.Fprintf(&builder, "CHROTE_AGENT_TMUX_KEEPER_UNIT=%s\n", strings.TrimSpace(config.KeeperUnit))
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".agent-config-*")
	if err != nil {
		return fmt.Errorf("stage agent config: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(builder.String()); err != nil {
		tmp.Close()
		return fmt.Errorf("write agent config: %w", err)
	}
	if err := tmp.Chmod(agentUnitConfigMode); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod agent config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync agent config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close agent config: %w", err)
	}
	return os.Rename(tmpName, path)
}

func readAgentUnitConfigFile(path string) (agentUnitConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return agentUnitConfig{}, err
	}
	config := agentUnitConfig{}
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found || strings.HasPrefix(key, "#") {
			continue
		}
		switch key {
		case "CHROTE_AGENT_SESSION":
			config.Session = value
		case "CHROTE_AGENT_KIND":
			config.AgentKind = value
		case "CHROTE_AGENT_SESSION_ID":
			config.AgentSessionID = value
		case "CHROTE_AGENT_BIN":
			config.AgentBin = value
		case "CHROTE_AGENT_TMUX_BIN":
			config.TmuxBin = value
		case "CHROTE_AGENT_TMUX_SOCKET":
			config.TmuxSocket = value
		case "CHROTE_AGENT_WORKDIR":
			config.Workdir = value
		}
	}
	return config, nil
}

func readAgentUnitReceipt(path string) (agentUnitReceipt, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return agentUnitReceipt{}, err
	}
	receipt := agentUnitReceipt{}
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return agentUnitReceipt{}, err
	}
	return receipt, nil
}

// runSystemctlForUser is the production mechanism. Argv-array with a timeout,
// never a shell. Reaching another user's manager is a privileged step the
// operator grants narrowly (ADR-0014 decision 4); when the target is this
// process's own account no elevation is used at all.
func runSystemctlForUser(ctx context.Context, unixUser string, args ...string) (string, error) {
	if err := validateAgentUnixUser(unixUser); err != nil {
		return "", err
	}
	for _, arg := range args {
		if strings.ContainsAny(arg, "\n\r\x00") {
			return "", fmt.Errorf("refusing a systemctl argument containing control characters")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, agentSystemctlBudget)
	defer cancel()

	argv := append([]string{"--user"}, args...)
	var cmd *exec.Cmd
	if current, err := osuser.Current(); err == nil && current.Username == unixUser {
		cmd = exec.CommandContext(ctx, "systemctl", argv...)
	} else {
		// Reaching another account's manager is the privileged step. It goes
		// through a helper the operator installs and scopes to these verbs and
		// to the chrote-agent@ template; this call site never constructs a unit
		// name itself, it only forwards one that agentUnitName produced.
		cmd = exec.CommandContext(ctx, agentUnitHelperBinary, append([]string{unixUser}, argv...)...)
	}
	out, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
				detail = stderr
			}
		}
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("%s", detail)
	}
	return string(out), nil
}
