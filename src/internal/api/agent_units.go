package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	osuser "os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
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
	agentUnitConfigMode  = 0o640
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

// A hermes profile becomes an argv element after --profile, so it is held to the
// same shape as every other value that reaches a command line.
var agentHermesProfileRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)

var agentPaneIDRegex = regexp.MustCompile(`^%[0-9]+$`)

// systemctlRunner runs one systemctl --user invocation against a target user's
// manager. It is an injection point so tests never touch a real bus, and so the
// privileged mechanism (how CHROTE reaches another user's manager) stays in one
// place rather than being spread through handlers.
type systemctlRunner func(ctx context.Context, unixUser string, args ...string) (string, error)

type systemdUnitState struct {
	ActiveState      string
	SubState         string
	UnitFileState    string
	StartedMonotonic uint64
	InvocationID     string
}

// agentUnitConfig is CHROTE's desired state for one locked session. It carries
// TYPED fields only: the launcher constructs an argv from AgentKind plus
// AgentSessionID, so no rendered command is persisted or parsed as shell text
// (ADR-0001, ADR-0014 decision 2).
type agentUnitConfig struct {
	Session        string
	UnixUser       string
	AgentKind      string
	AgentSessionID string
	AgentBin       string
	// HermesProfile is required for, and only for, hermes agents: their canonical
	// argv is python -m hermes_cli.main --profile <profile> --resume <id>.
	HermesProfile string
	TmuxBin       string
	TmuxSocket    string
	Workdir       string
	KeeperUnit    string
	WatchInterval int
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
	PaneID         string `json:"paneId"`
	PanePID        int    `json:"panePid"`
	AgentPID       int    `json:"agentPid"`
	ProcessStart   uint64 `json:"processStartTicks"`
	InvocationID   string `json:"invocationId"`
	AttestedAt     uint64 `json:"attestedAtMonotonic"`
	StartedAt      string `json:"startedAt"`
}

type agentUnitController struct {
	root        string
	receiptRoot string
	systemctl   systemctlRunner
	mu          sync.Mutex
}

const (
	defaultAgentUnitsDir    = "/srv/data/chrote/agent-units"
	defaultAgentReceiptsDir = "/srv/data/chrote/agent-receipts"
)

func defaultAgentUnitsPath() string {
	if override := strings.TrimSpace(os.Getenv("CHROTE_AGENT_UNITS_DIR")); override != "" {
		return override
	}
	return defaultAgentUnitsDir
}

func agentReceiptsPathForUnitsRoot(unitsRoot string) string {
	if override := strings.TrimSpace(os.Getenv("CHROTE_AGENT_RECEIPTS_DIR")); override != "" {
		return override
	}
	if filepath.Clean(unitsRoot) == filepath.Clean(defaultAgentUnitsDir) {
		return defaultAgentReceiptsDir
	}
	return strings.TrimRight(unitsRoot, string(os.PathSeparator)) + "-receipts"
}

// agentSystemctlRun is the seam every controller falls back to. It exists as a
// package variable, matching tmuxCurrentUser above, so a test binary can never
// reach a real user manager by forgetting to inject a fake.
var agentSystemctlRun systemctlRunner = runSystemctlForUser

func newAgentUnitController(root string, runner systemctlRunner) *agentUnitController {
	if runner == nil {
		runner = func(ctx context.Context, unixUser string, args ...string) (string, error) {
			return agentSystemctlRun(ctx, unixUser, args...)
		}
	}
	root = strings.TrimSpace(root)
	return &agentUnitController{
		root:        root,
		receiptRoot: agentReceiptsPathForUnitsRoot(root),
		systemctl:   runner,
	}
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

// resolveAgentUnixUser returns the canonical account name to act on. An empty
// value is not hostile, it is the common case: a session on CHROTE's own account
// carries no unixUser at all, and treating that as invalid made such sessions
// impossible to lock AND impossible to unlock -- the unlock handler returned
// before reaching Forget, stranding the registry entry with no way to clear it.
// Callers must use the returned value, never their own copy (see M6).
func resolveAgentUnixUser(unixUser string) (string, error) {
	unixUser = strings.TrimSpace(unixUser)
	if unixUser == "" {
		current, err := osuser.Current()
		if err != nil {
			return "", fmt.Errorf("cannot determine the account to supervise: %w", err)
		}
		unixUser = strings.TrimSpace(current.Username)
	}
	if !agentUnixUserRegex.MatchString(unixUser) {
		return "", fmt.Errorf("unix user %q is not a valid account name", unixUser)
	}
	if strings.ContainsAny(unixUser, "\n\r\x00") {
		return "", fmt.Errorf("unix user contains control characters")
	}
	return unixUser, nil
}

func (c *agentUnitConfig) validate() error {
	if _, err := agentUnitName(c.Session); err != nil {
		return err
	}
	if _, err := resolveAgentUnixUser(c.UnixUser); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(c.AgentKind)) {
	case "codex", "claude":
	case "hermes":
		if !agentHermesProfileRegex.MatchString(strings.TrimSpace(c.HermesProfile)) {
			return fmt.Errorf("hermes agents require a profile name, got %q", c.HermesProfile)
		}
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
	// Canonical values only: validating a trimmed copy while joining the raw one
	// let "alice " and "alice" produce the same unit name but different config
	// paths, so a lock made with one spelling could not be unlocked with the
	// other -- and the unit stayed enabled while the API reported it gone.
	unixUser, err := resolveAgentUnixUser(unixUser)
	if err != nil {
		return "", err
	}
	session = strings.TrimSpace(session)
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
	unixUser, err := resolveAgentUnixUser(unixUser)
	if err != nil {
		return "", err
	}
	session = strings.TrimSpace(session)
	if _, err := agentUnitName(session); err != nil {
		return "", err
	}
	if strings.TrimSpace(c.receiptRoot) == "" {
		return "", fmt.Errorf("agent receipt state directory is not configured")
	}
	base := filepath.Join(c.receiptRoot, unixUser)
	path := filepath.Join(base, session+".receipt.json")
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(base)+string(os.PathSeparator)) {
		return "", fmt.Errorf("refusing a receipt path outside %s", base)
	}
	return path, nil
}

// Enable writes the typed config and hands supervision to systemd. The config is
// written first: a unit that starts before its config exists would fail loudly,
// but a unit enabled without config would be a lock CHROTE cannot explain.
func (c *agentUnitController) Enable(ctx context.Context, config agentUnitConfig) error {
	if err := config.validate(); err != nil {
		return err
	}
	config.Session = strings.TrimSpace(config.Session)
	resolvedUser, err := resolveAgentUnixUser(config.UnixUser)
	if err != nil {
		return err
	}
	config.UnixUser = resolvedUser
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
		// `enable --now` is two operations. If the wants-symlink landed and only
		// the start failed, removing our config here would make the unit
		// unreachable: OwnsUnit needs the config, Disable stats it and returns a
		// silent no-op, and AnnotateHealth never sees a session the handler
		// already rolled back. That is an orphan that restarts on every login and
		// that CHROTE can neither show nor stop. So undo the enable first, and if
		// THAT fails, say so instead of dropping it.
		if _, disableErr := c.systemctl(ctx, config.UnixUser, "disable", "--now", unit); disableErr != nil {
			return fmt.Errorf("enable %s for %s: %w; and it could not be disabled again (%v) -- the unit may still be enabled",
				unit, config.UnixUser, err, disableErr)
		}
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
		case receipt.Session != config.Session || receipt.AgentKind != config.AgentKind:
			status.Health = agentHealthDegraded
			status.Detail = "unit receipt does not identify the configured agent"
		case receipt.AgentSessionID != config.AgentSessionID:
			status.Health = agentHealthDegraded
			status.Detail = "unit is running a different transcript than the one this lock configured"
		case state.InvocationID == "" || receipt.InvocationID != state.InvocationID:
			status.Health = agentHealthDegraded
			status.Detail = "launcher receipt belongs to a previous unit invocation"
		case state.StartedMonotonic == 0 || receipt.AttestedAt < state.StartedMonotonic:
			status.Health = agentHealthDegraded
			status.Detail = "launcher receipt predates the current unit invocation"
		case receipt.PanePID <= 0 || receipt.AgentPID != receipt.PanePID || !agentPaneIDRegex.MatchString(receipt.PaneID):
			status.Health = agentHealthDegraded
			status.Detail = "launcher receipt has invalid pane or process identity"
		case !agentReceiptProcessMatches(receipt):
			status.Health = agentHealthDegraded
			status.Detail = "the agent process confirmed by the launcher is no longer running"
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
		"--property=UnitFileState", "--property=ExecMainStartTimestampMonotonic",
		"--property=InvocationID", unit)
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
			state.StartedMonotonic, _ = strconv.ParseUint(value, 10, 64)
		case "InvocationID":
			state.InvocationID = value
		}
	}
	return state, nil
}

func agentReceiptProcessMatches(receipt agentUnitReceipt) bool {
	if receipt.AgentPID <= 0 || receipt.ProcessStart == 0 {
		return false
	}
	start, err := processStartTicks(receipt.AgentPID)
	return err == nil && start == receipt.ProcessStart
}

func processStartTicks(pid int) (uint64, error) {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}
	line := string(raw)
	end := strings.LastIndex(line, ") ")
	if end < 0 {
		return 0, fmt.Errorf("process stat has no command boundary")
	}
	fields := strings.Fields(line[end+2:])
	if len(fields) <= 19 {
		return 0, fmt.Errorf("process stat has %d fields after command", len(fields))
	}
	return strconv.ParseUint(fields[19], 10, 64)
}

// writeAgentUnitConfigFile writes the typed KEY=value file the launcher reads.
// Atomic rename so the launcher never reads a half-written config. Mode 0640 is
// an ACL mask: provisioning grants only the target account named read access.
func writeAgentUnitConfigFile(path string, config agentUnitConfig, receiptPath string) error {
	watch := config.WatchInterval
	if watch <= 0 {
		watch = 10
	}
	var builder strings.Builder
	builder.WriteString("# Written by CHROTE. Typed fields only: the launcher constructs the resume\n")
	builder.WriteString("# argv from kind + native session id; this file cannot carry a command.\n")
	fmt.Fprintf(&builder, "CHROTE_AGENT_SESSION=%s\n", config.Session)
	fmt.Fprintf(&builder, "CHROTE_AGENT_KIND=%s\n", strings.ToLower(strings.TrimSpace(config.AgentKind)))
	fmt.Fprintf(&builder, "CHROTE_AGENT_SESSION_ID=%s\n", strings.TrimSpace(config.AgentSessionID))
	fmt.Fprintf(&builder, "CHROTE_AGENT_BIN=%s\n", config.AgentBin)
	fmt.Fprintf(&builder, "CHROTE_AGENT_TMUX_BIN=%s\n", config.TmuxBin)
	fmt.Fprintf(&builder, "CHROTE_AGENT_TMUX_SOCKET=%s\n", config.TmuxSocket)
	fmt.Fprintf(&builder, "CHROTE_AGENT_WORKDIR=%s\n", config.Workdir)
	fmt.Fprintf(&builder, "CHROTE_AGENT_RECEIPT_PATH=%s\n", receiptPath)
	fmt.Fprintf(&builder, "CHROTE_AGENT_WATCH_INTERVAL=%d\n", watch)
	if profile := strings.TrimSpace(config.HermesProfile); profile != "" {
		fmt.Fprintf(&builder, "CHROTE_AGENT_HERMES_PROFILE=%s\n", profile)
	}
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
	// The group-read bit is the POSIX ACL mask. Provisioning gives only the target
	// account a named read entry; 0640 makes that entry effective while keeping
	// group::--- and other::--- in the ACL itself.
	if err := tmp.Chmod(agentUnitConfigMode); err != nil {
		tmp.Close()
		return fmt.Errorf("set agent config mode: %w", err)
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
		case "CHROTE_AGENT_HERMES_PROFILE":
			config.HermesProfile = value
		}
	}
	return config, nil
}

func readAgentUnitReceipt(path string) (agentUnitReceipt, error) {
	parent := filepath.Dir(path)
	if err := rejectSymlinkPath(parent); err != nil {
		return agentUnitReceipt{}, err
	}
	parentFD, err := syscall.Open(parent, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return agentUnitReceipt{}, err
	}
	defer syscall.Close(parentFD)
	var parentStat syscall.Stat_t
	if err := syscall.Fstat(parentFD, &parentStat); err != nil {
		return agentUnitReceipt{}, err
	}
	fileFD, err := syscall.Openat(parentFD, filepath.Base(path), syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return agentUnitReceipt{}, err
	}
	file := os.NewFile(uintptr(fileFD), path)
	if file == nil {
		syscall.Close(fileFD)
		return agentUnitReceipt{}, fmt.Errorf("open agent receipt")
	}
	defer file.Close()
	var fileStat syscall.Stat_t
	if err := syscall.Fstat(fileFD, &fileStat); err != nil {
		return agentUnitReceipt{}, err
	}
	if fileStat.Mode&syscall.S_IFMT != syscall.S_IFREG {
		return agentUnitReceipt{}, fmt.Errorf("agent receipt is not a regular file")
	}
	if mode := os.FileMode(fileStat.Mode).Perm(); mode != 0o640 {
		return agentUnitReceipt{}, fmt.Errorf("agent receipt mode is %04o, want 0640", mode)
	}
	if fileStat.Uid != parentStat.Uid || fileStat.Gid != parentStat.Gid {
		return agentUnitReceipt{}, fmt.Errorf("agent receipt owner does not match its runtime directory")
	}
	const maxReceiptBytes = 64 * 1024
	raw, err := io.ReadAll(io.LimitReader(file, maxReceiptBytes+1))
	if err != nil {
		return agentUnitReceipt{}, err
	}
	if len(raw) > maxReceiptBytes {
		return agentUnitReceipt{}, fmt.Errorf("agent receipt exceeds %d bytes", maxReceiptBytes)
	}
	receipt := agentUnitReceipt{}
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return agentUnitReceipt{}, err
	}
	return receipt, nil
}

func rejectSymlinkPath(path string) error {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) {
		return fmt.Errorf("agent receipt path is not absolute")
	}
	current := string(os.PathSeparator)
	for _, component := range strings.Split(strings.TrimPrefix(clean, string(os.PathSeparator)), string(os.PathSeparator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("agent receipt path contains a symlink: %s", current)
		}
	}
	return nil
}

// agentBinaryForKind resolves the CLI a locked agent resumes with. The unit
// needs an absolute path because it runs with the systemd user manager's PATH,
// not a login shell's. An operator override wins; otherwise the owner's usual
// per-user install location is used, and the launcher fails loud if it is wrong
// rather than silently running some other binary of the same name.
// absoluteToolPath resolves a possibly bare binary name against PATH. A unit
// runs with the user manager's PATH, not a login shell's, so a bare name in the
// config would resolve differently there -- or not at all.
func absoluteToolPath(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" || filepath.IsAbs(candidate) {
		return candidate
	}
	if resolved, err := exec.LookPath(candidate); err == nil {
		return resolved
	}
	return candidate
}

func agentBinaryForKind(kind, ownerHome string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	envKey := "CHROTE_AGENT_BIN_" + strings.ToUpper(kind)
	if override := strings.TrimSpace(os.Getenv(envKey)); override != "" {
		return override
	}
	if strings.TrimSpace(ownerHome) == "" {
		return ""
	}
	if kind == "hermes" {
		// Hermes runs from its own venv interpreter, not a launcher shim. Same
		// path canonicalRecoveryAgentArgv derives, kept in one shape.
		return filepath.Join(ownerHome, ".hermes", "hermes-agent-current", "venv", "bin", "python")
	}
	return filepath.Join(ownerHome, ".local", "bin", kind)
}

// runSystemctlForUser is the production mechanism. Argv-array with a timeout,
// never a shell. Reaching another user's manager is a privileged step the
// operator grants narrowly (ADR-0014 decision 4); when the target is this
// process's own account no elevation is used at all.
func runSystemctlForUser(ctx context.Context, unixUser string, args ...string) (string, error) {
	resolved, err := resolveAgentUnixUser(unixUser)
	if err != nil {
		return "", err
	}
	unixUser = resolved
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

// AnnotateHealth fills in each locked session's live unit health. It is separate
// from the desired-state annotation on purpose: the store knows what was asked
// for, systemd knows what is actually true, and conflating the two is how the
// old supervisor reported healthy right through an outage.
func (c *agentUnitController) AnnotateHealth(ctx context.Context, sessions []core.Session) []core.Session {
	if c == nil {
		return sessions
	}
	for i := range sessions {
		if !sessions[i].Persistent {
			continue
		}
		status, err := c.Status(ctx, sessions[i].Name, sessions[i].UnixUser)
		if err != nil {
			sessions[i].PersistentHealth = agentHealthDegraded
			sessions[i].PersistentDetail = err.Error()
			continue
		}
		if !status.Locked {
			// The desired-state store says this session is locked but no unit
			// config backs it: a failed enable, or a record left by the
			// pre-ADR-0014 supervisor. Saying "unlocked" here would contradict
			// the same payload's persistent:true, so say what is actually wrong.
			sessions[i].PersistentHealth = agentHealthDegraded
			sessions[i].PersistentDetail = "no supervising unit is installed for this session"
			continue
		}
		sessions[i].PersistentUnit = status.Unit
		sessions[i].PersistentHealth = status.Health
		sessions[i].PersistentActiveState = status.ActiveState
		sessions[i].PersistentDetail = status.Detail
	}
	return sessions
}
