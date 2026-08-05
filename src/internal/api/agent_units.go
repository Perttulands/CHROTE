package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	osuser "os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	agentUnitTemplate      = "chrote-agent@"
	agentUnitSuffix        = ".service"
	agentUnitConfigMode    = 0o640
	agentUnitDirMode       = 0o700
	agentSystemctlBudget   = 15 * time.Second
	agentHealthBudget      = 2 * time.Second
	agentHealthCacheTTL    = 2 * time.Second
	agentHealthConcurrency = 4
	agentPreflightUnit     = "chrote-agent@chrote-preflight.service"

	// This is the entire privilege boundary. Both paths are root-owned deployment
	// artifacts; PATH must never choose what runs after sudo crosses accounts.
	agentUnitSudoBinary   = "/usr/bin/sudo"
	agentUnitHelperBinary = "/usr/local/libexec/chrote/chrote-agentctl"
	agentSystemctlBinary  = "/usr/bin/systemctl"

	agentHealthHealthy  = "healthy"
	agentHealthDegraded = "degraded"
	agentHealthFailed   = "failed"
	agentHealthInactive = "inactive"
	agentHealthUnlocked = "unlocked"

	agentDetailUnitUnreachable   = "unit-unreachable"
	agentDetailConfigUnreadable  = "config-unreadable"
	agentDetailConfigMissing     = "config-missing"
	agentDetailReceiptMissing    = "receipt-missing"
	agentDetailReceiptUnreadable = "receipt-unreadable"
	agentDetailReceiptMismatch   = "receipt-mismatch"
	agentDetailReceiptStale      = "receipt-stale"
	agentDetailProcessMissing    = "process-missing"
	agentDetailUnitFailed        = "unit-failed"
	agentDetailUnitInactive      = "unit-inactive"
	agentDetailLookupTimeout     = "lookup-timeout"
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

var agentAbsolutePathRegex = regexp.MustCompile(`^/[a-zA-Z0-9._/-]+$`)

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
	Session       string `json:"session"`
	UnixUser      string `json:"unixUser,omitempty"`
	Locked        bool   `json:"locked"`
	Unit          string `json:"unit,omitempty"`
	Health        string `json:"health"`
	ActiveState   string `json:"activeState,omitempty"`
	SubState      string `json:"subState,omitempty"`
	Detail        string `json:"detail,omitempty"`
	DetailCode    string `json:"detailCode,omitempty"`
	CorrelationID string `json:"correlationId,omitempty"`
}

type agentPublicError struct {
	code          string
	message       string
	correlationID string
	cause         error
}

func (e *agentPublicError) Error() string {
	return fmt.Sprintf("%s (%s; reference %s)", e.message, e.code, e.correlationID)
}

func (e *agentPublicError) Unwrap() error { return e.cause }

var agentStatusIncidentSequence atomic.Uint64

func newAgentPublicError(code, message string, cause error) *agentPublicError {
	var random [8]byte
	correlationID := ""
	if _, err := rand.Read(random[:]); err == nil {
		correlationID = "pa-" + hex.EncodeToString(random[:])
	} else {
		correlationID = fmt.Sprintf("pa-%016x", agentStatusIncidentSequence.Add(1))
	}
	log.Printf("persistent-agent incident %s code=%s: %v", correlationID, code, cause)
	return &agentPublicError{code: code, message: message, correlationID: correlationID, cause: cause}
}

func setAgentStatusIncident(status *AgentUnitStatus, code, message string, cause error) {
	publicErr := newAgentPublicError(code, message, cause)
	status.Health = agentHealthDegraded
	status.Detail = message
	status.DetailCode = code
	status.CorrelationID = publicErr.correlationID
}

func setAgentStatusDetail(status *AgentUnitStatus, health, code, detail string) {
	status.Health = health
	status.Detail = detail
	status.DetailCode = code
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
	root              string
	receiptRoot       string
	systemctl         systemctlRunner
	operationMu       sync.Mutex
	operations        map[string]*sync.Mutex
	healthMu          sync.Mutex
	healthCache       map[string]cachedAgentUnitStatus
	capabilityMu      sync.RWMutex
	capabilityChecked bool
	capabilities      map[string]agentUnitCapability
}

type cachedAgentUnitStatus struct {
	status    AgentUnitStatus
	err       error
	expiresAt time.Time
}

type agentUnitCapability struct {
	Available bool
	Detail    string
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
		root:         root,
		receiptRoot:  agentReceiptsPathForUnitsRoot(root),
		systemctl:    runner,
		operations:   make(map[string]*sync.Mutex),
		healthCache:  make(map[string]cachedAgentUnitStatus),
		capabilities: make(map[string]agentUnitCapability),
	}
}

// Preflight exercises the actual systemd user manager for every account the UI
// may target. It is read-only: LoadState on a reserved, never-started instance
// proves the bus, privilege crossing and installed template in one call.
func (c *agentUnitController) Preflight(parent context.Context, unixUsers []string) []error {
	if c == nil {
		return []error{fmt.Errorf("persistent-agent controller is unavailable")}
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, agentSystemctlBudget)
	defer cancel()
	if len(unixUsers) == 0 {
		unixUsers = []string{""}
	}

	capabilities := make(map[string]agentUnitCapability, len(unixUsers))
	errs := []error{}
	for _, requestedUser := range unixUsers {
		unixUser, err := resolveAgentUnixUser(requestedUser)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if _, duplicate := capabilities[unixUser]; duplicate {
			continue
		}
		out, runErr := c.systemctl(ctx, unixUser, "show", "--property=LoadState", agentPreflightUnit)
		loadState := systemdProperty(out, "LoadState")
		capability := agentUnitCapability{Available: runErr == nil && loadState == "loaded"}
		switch {
		case runErr != nil:
			capability.Detail = newAgentPublicError(agentDetailUnitUnreachable, "persistent-agent control is unavailable", runErr).Error()
		case loadState != "loaded":
			capability.Detail = fmt.Sprintf("persistent-agent unit template is not loaded for %s", unixUser)
		}
		capabilities[unixUser] = capability
		if !capability.Available {
			errs = append(errs, errors.New(capability.Detail))
		}
	}

	c.capabilityMu.Lock()
	c.capabilities = capabilities
	c.capabilityChecked = true
	c.capabilityMu.Unlock()
	return errs
}

func systemdProperty(output, name string) string {
	for _, line := range strings.Split(output, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found && key == name {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// RequireCapability is a no-op only for explicitly constructed handlers whose
// startup preflight has not run. Production main always preflights before it
// listens and then fails this check closed for each unavailable account.
func (c *agentUnitController) RequireCapability(unixUser string) error {
	if c == nil {
		return fmt.Errorf("persistent-agent capability is unavailable")
	}
	resolved, err := resolveAgentUnixUser(unixUser)
	if err != nil {
		return err
	}
	c.capabilityMu.RLock()
	defer c.capabilityMu.RUnlock()
	if !c.capabilityChecked {
		return nil
	}
	capability, found := c.capabilities[resolved]
	if !found {
		return fmt.Errorf("persistent-agent control for %s was not preflighted", resolved)
	}
	if !capability.Available {
		return errors.New(capability.Detail)
	}
	return nil
}

func (c *agentUnitController) AnnotateCapability(sessions []core.Session) []core.Session {
	if c == nil {
		return sessions
	}
	for i := range sessions {
		resolved, err := resolveAgentUnixUser(sessions[i].UnixUser)
		if err != nil {
			continue
		}
		c.capabilityMu.RLock()
		checked := c.capabilityChecked
		capability, found := c.capabilities[resolved]
		c.capabilityMu.RUnlock()
		if !checked {
			continue
		}
		available := found && capability.Available
		sessions[i].PersistentAvailable = &available
		if !available {
			if found {
				sessions[i].PersistentCapabilityDetail = capability.Detail
			} else {
				sessions[i].PersistentCapabilityDetail = fmt.Sprintf("persistent-agent control for %s was not preflighted", resolved)
			}
		}
	}
	return sessions
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
		if !validAgentAbsolutePath(value) {
			return fmt.Errorf("%s must be a canonical absolute path containing only letters, digits, dot, underscore, slash, and dash", label)
		}
	}
	if c.KeeperUnit != "" && strings.ContainsAny(c.KeeperUnit, "\n\r\x00") {
		return fmt.Errorf("keeper unit name contains control characters")
	}
	return nil
}

func validAgentAbsolutePath(value string) bool {
	return filepath.IsAbs(value) && filepath.Clean(value) == value && agentAbsolutePathRegex.MatchString(value)
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
	if !validAgentAbsolutePath(c.root) {
		return "", fmt.Errorf("agent unit state directory must be a canonical safe absolute path")
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
	if !validAgentAbsolutePath(c.receiptRoot) {
		return "", fmt.Errorf("agent receipt state directory must be a canonical safe absolute path")
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
	if !validAgentAbsolutePath(receipt) {
		return fmt.Errorf("agent receipt must be a canonical absolute path containing only letters, digits, dot, underscore, slash, and dash")
	}

	unlock := c.lockOperation(config.Session, config.UnixUser)
	defer unlock()
	defer c.invalidateHealth(config.Session, config.UnixUser)

	if err := os.MkdirAll(filepath.Dir(configPath), agentUnitDirMode); err != nil {
		return newAgentPublicError(agentDetailConfigUnreadable, "supervision config could not be written", err)
	}
	if err := writeAgentUnitConfigFile(configPath, config, receipt); err != nil {
		return newAgentPublicError(agentDetailConfigUnreadable, "supervision config could not be written", err)
	}
	if _, err := c.systemctl(ctx, config.UnixUser, "enable", "--now", unit); err != nil {
		enableErr := newAgentPublicError(agentDetailUnitUnreachable, "the supervising unit could not be enabled", err)
		// `enable --now` is two operations. If the wants-symlink landed and only
		// the start failed, removing our config here would make the unit
		// unreachable: OwnsUnit needs the config, Disable stats it and returns a
		// silent no-op, and AnnotateHealth never sees a session the handler
		// already rolled back. That is an orphan that restarts on every login and
		// that CHROTE can neither show nor stop. So undo the enable first, and if
		// THAT fails, say so instead of dropping it.
		if _, disableErr := c.systemctl(ctx, config.UnixUser, "disable", "--now", unit); disableErr != nil {
			rollbackErr := newAgentPublicError(agentDetailUnitUnreachable, "the partially enabled unit could not be disabled", disableErr)
			return fmt.Errorf("%w; %v -- the unit may still be enabled", enableErr, rollbackErr)
		}
		_ = os.Remove(configPath)
		return enableErr
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

	unlock := c.lockOperation(session, unixUser)
	defer unlock()
	defer c.invalidateHealth(session, unixUser)

	if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
		// Not locked by us. Disabling a unit we never installed is not our call.
		return nil
	}
	if _, err := c.systemctl(ctx, unixUser, "disable", "--now", unit); err != nil {
		return newAgentPublicError(agentDetailUnitUnreachable, "the supervising unit could not be disabled", err)
	}
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return newAgentPublicError(agentDetailConfigUnreadable, "supervision config could not be removed", err)
	}
	if receipt, err := c.receiptPath(session, unixUser); err == nil {
		_ = os.Remove(receipt)
	}
	return nil
}

func agentUnitOperationKey(session, unixUser string) string {
	if resolved, err := resolveAgentUnixUser(unixUser); err == nil {
		unixUser = resolved
	}
	return strings.TrimSpace(unixUser) + "\x00" + strings.TrimSpace(session)
}

func (c *agentUnitController) lockOperation(session, unixUser string) func() {
	key := agentUnitOperationKey(session, unixUser)
	c.operationMu.Lock()
	lock := c.operations[key]
	if lock == nil {
		lock = &sync.Mutex{}
		c.operations[key] = lock
	}
	c.operationMu.Unlock()
	lock.Lock()
	return lock.Unlock
}

func (c *agentUnitController) invalidateHealth(session, unixUser string) {
	c.healthMu.Lock()
	delete(c.healthCache, agentUnitOperationKey(session, unixUser))
	c.healthMu.Unlock()
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
		status.Locked = true
		status.Unit = unit
		setAgentStatusIncident(&status, agentDetailConfigUnreadable, "supervision config is unreadable", err)
		return status, nil
	}
	status.Locked = true
	status.Unit = unit

	state, err := c.unitState(ctx, unixUser, unit)
	if err != nil {
		if ctx.Err() != nil {
			setAgentStatusDetail(&status, agentHealthDegraded, agentDetailLookupTimeout, "unit status lookup timed out")
		} else {
			setAgentStatusIncident(&status, agentDetailUnitUnreachable, "unit status is unavailable", err)
		}
		return status, nil
	}
	status.ActiveState = state.ActiveState
	status.SubState = state.SubState

	switch state.ActiveState {
	case "active", "activating", "reloading":
		receiptPath, receiptErr := c.receiptPath(session, unixUser)
		if receiptErr != nil {
			setAgentStatusIncident(&status, agentDetailReceiptUnreadable, "launcher receipt is unreadable", receiptErr)
			return status, nil
		}
		receipt, receiptErr := readAgentUnitReceipt(receiptPath)
		switch {
		case os.IsNotExist(receiptErr):
			setAgentStatusDetail(&status, agentHealthDegraded, agentDetailReceiptMissing, "unit is running but has not confirmed which transcript it resumed")
		case receiptErr != nil:
			setAgentStatusIncident(&status, agentDetailReceiptUnreadable, "launcher receipt is unreadable", receiptErr)
		case receipt.Session != config.Session || receipt.AgentKind != config.AgentKind:
			setAgentStatusDetail(&status, agentHealthDegraded, agentDetailReceiptMismatch, "unit receipt does not identify the configured agent")
		case receipt.AgentSessionID != config.AgentSessionID:
			setAgentStatusDetail(&status, agentHealthDegraded, agentDetailReceiptMismatch, "unit is running a different transcript than the one this lock configured")
		case state.InvocationID == "" || receipt.InvocationID != state.InvocationID:
			setAgentStatusDetail(&status, agentHealthDegraded, agentDetailReceiptStale, "launcher receipt belongs to a previous unit invocation")
		case state.StartedMonotonic == 0 || receipt.AttestedAt < state.StartedMonotonic:
			setAgentStatusDetail(&status, agentHealthDegraded, agentDetailReceiptStale, "launcher receipt predates the current unit invocation")
		case receipt.PanePID <= 0 || receipt.AgentPID != receipt.PanePID || !agentPaneIDRegex.MatchString(receipt.PaneID):
			setAgentStatusDetail(&status, agentHealthDegraded, agentDetailReceiptMismatch, "launcher receipt has invalid pane or process identity")
		case !agentReceiptProcessMatches(receipt):
			setAgentStatusDetail(&status, agentHealthDegraded, agentDetailProcessMissing, "the agent process confirmed by the launcher is no longer running")
		default:
			status.Health = agentHealthHealthy
		}
	case "failed":
		setAgentStatusDetail(&status, agentHealthFailed, agentDetailUnitFailed, "unit failed; see the agent unit journal")
	default:
		setAgentStatusDetail(&status, agentHealthInactive, agentDetailUnitInactive, "unit is not running")
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
	seen := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found || strings.HasPrefix(key, "#") {
			continue
		}
		known := true
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
		default:
			known = false
		}
		if !known {
			continue
		}
		if seen[key] {
			return agentUnitConfig{}, fmt.Errorf("duplicate %s in agent config", key)
		}
		seen[key] = true
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
		cmd = exec.CommandContext(ctx, agentSystemctlBinary, argv...)
	} else {
		// Reaching another account's manager is the privileged step. It goes
		// through a helper the operator installs and scopes to these verbs and
		// to the chrote-agent@ template; this call site never constructs a unit
		// name itself, it only forwards one that agentUnitName produced.
		helperArgv := append([]string{"-n", "--", agentUnitHelperBinary, unixUser}, argv...)
		cmd = exec.CommandContext(ctx, agentUnitSudoBinary, helperArgv...)
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
	if ctx == nil {
		ctx = context.Background()
	}
	healthCtx, cancel := context.WithTimeout(ctx, agentHealthBudget)
	defer cancel()
	semaphore := make(chan struct{}, agentHealthConcurrency)
	var wait sync.WaitGroup
	for i := range sessions {
		if !sessions[i].Persistent {
			continue
		}
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-healthCtx.Done():
				sessions[index].PersistentHealth = agentHealthDegraded
				sessions[index].PersistentDetail = "unit status lookup timed out"
				sessions[index].PersistentDetailCode = agentDetailLookupTimeout
				return
			}
			status, err := c.cachedStatus(healthCtx, sessions[index].Name, sessions[index].UnixUser)
			if err != nil {
				publicErr := newAgentPublicError(agentDetailUnitUnreachable, "unit status is unavailable", err)
				sessions[index].PersistentHealth = agentHealthDegraded
				sessions[index].PersistentDetail = publicErr.message
				sessions[index].PersistentDetailCode = publicErr.code
				sessions[index].PersistentCorrelationID = publicErr.correlationID
				return
			}
			if !status.Locked {
				// The desired-state store says this session is locked but no unit
				// config backs it: a failed enable, or a record left by the
				// pre-ADR-0014 supervisor. Saying "unlocked" here would contradict
				// the same payload's persistent:true, so say what is actually wrong.
				sessions[index].PersistentHealth = agentHealthDegraded
				sessions[index].PersistentDetail = "no supervising unit is installed for this session"
				sessions[index].PersistentDetailCode = agentDetailConfigMissing
				return
			}
			sessions[index].PersistentUnit = status.Unit
			sessions[index].PersistentHealth = status.Health
			sessions[index].PersistentActiveState = status.ActiveState
			sessions[index].PersistentDetail = status.Detail
			sessions[index].PersistentDetailCode = status.DetailCode
			sessions[index].PersistentCorrelationID = status.CorrelationID
		}(i)
	}
	wait.Wait()
	return sessions
}

func (c *agentUnitController) cachedStatus(ctx context.Context, session, unixUser string) (AgentUnitStatus, error) {
	unlock := c.lockOperation(session, unixUser)
	defer unlock()
	key := agentUnitOperationKey(session, unixUser)
	now := time.Now()
	c.healthMu.Lock()
	cached, found := c.healthCache[key]
	if found && now.Before(cached.expiresAt) {
		c.healthMu.Unlock()
		return cached.status, cached.err
	}
	c.healthMu.Unlock()

	status, err := c.Status(ctx, session, unixUser)
	if ctx.Err() == nil {
		c.healthMu.Lock()
		c.healthCache[key] = cachedAgentUnitStatus{status: status, err: err, expiresAt: now.Add(agentHealthCacheTTL)}
		c.healthMu.Unlock()
	}
	return status, err
}
