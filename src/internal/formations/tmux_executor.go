package formations

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	osuser "os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chrote/server/internal/core"
)

const (
	defaultTmuxOutputCapBytes = 8192
	defaultTmuxTimeoutSeconds = 30
	peerPlaneMaxBytes         = 1 << 20
	// ownedSessionNameAttempts bounds how many times the executor regenerates a
	// unique owned session name when the candidate collides with a session
	// already present on the shared socket (a pre-existing / foreign session).
	// Collision is astronomically unlikely with a random nonce, so a small bound
	// is plenty; exhausting it fails closed rather than reusing a foreign name.
	ownedSessionNameAttempts = 8
	// tmuxKeeperSuffix names the executor's own lazy-start "keeper" session,
	// appended to the configured SessionPrefix. The keeper's only job is to hold
	// a freshly lazy-started tmux server alive; it is infrastructure, never an
	// owned run session, so teardown must never reclaim it.
	tmuxKeeperSuffix = "keeper"
	// tmuxKeeperHoldCommand is the long-lived pane command for the keeper. It is
	// run through a real shell (see StartKeeper's SHELL=/bin/bash) so the keeper
	// pane survives even when the invoking service user's login shell is
	// /usr/sbin/nologin; without an explicit long-lived command the pane would
	// exit immediately and take the just-started server down with it.
	tmuxKeeperHoldCommand = "exec sleep 2147483647"
)

var (
	errTmuxTargetMissing = errors.New("tmux target missing")
	tmuxSubmitDelay      = 500 * time.Millisecond
	// tmuxPasteSettleDelay lets a large bracketed paste finish rendering in the
	// agent TUI before the first Enter. A single Enter sent too soon is absorbed
	// into the still-settling input instead of submitting the turn.
	tmuxPasteSettleDelay = 1200 * time.Millisecond
	// tmuxSubmitMaxEnters bounds the "send a few more Enters" submit: keep pressing
	// Enter until the pane shows the agent actually working, up to this many tries.
	tmuxSubmitMaxEnters = 8
	tmuxSleep           = time.Sleep
	// newSessionNonce returns a collision-proof suffix for an owned session name.
	// It is a package var so tests can make owned names deterministic.
	newSessionNonce = func() string {
		var buf [6]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return strconv.FormatInt(time.Now().UnixNano(), 36)
		}
		return hex.EncodeToString(buf[:])
	}
	// formationServiceUID returns the numeric uid the CHROTE service process runs
	// as. This is the default expected agent-user, so single-user installs work
	// with zero configuration. Package var for test injection; never hardcoded.
	formationServiceUID = func() (int, error) {
		current, err := osuser.Current()
		if err != nil {
			return 0, err
		}
		return strconv.Atoi(current.Uid)
	}
	// formationLookupUID maps a configured agent-user name to its numeric uid.
	// Package var for test injection; never hardcoded to a specific user.
	formationLookupUID = func(name string) (int, error) {
		account, err := osuser.Lookup(name)
		if err != nil {
			return 0, err
		}
		return strconv.Atoi(account.Uid)
	}
	// formationSocketOwnerUID returns the uid that owns the tmux socket file, i.e.
	// the user the backing tmux server runs as (the pane's Unix identity). It
	// stats the socket; tests inject a fake to avoid a real tmux server / uid.
	formationSocketOwnerUID = func(socket string) (int, error) {
		info, err := os.Stat(socket)
		if err != nil {
			return 0, err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return 0, fmt.Errorf("tmux socket %s owner is not stat-able", socket)
		}
		return int(stat.Uid), nil
	}
)

// expectedAgentUser is the resolved identity the executor requires the tmux
// server to run as. self is true when that identity is the service user itself,
// which is the only case where lazy-starting a server (as the service user) is
// safe.
type expectedAgentUser struct {
	uid  int
	self bool
	name string
}

// resolveExpectedAgentUser resolves the configured agent-user to a uid and
// decides whether it is the service user (self). An empty AgentUser defaults to
// the service user, so single-user installs need no configuration and always run
// self. The self/other decision is purely uid-based, so an explicitly configured
// name that resolves to the service uid behaves exactly like the default.
func (e *TmuxFormationExecutor) resolveExpectedAgentUser() (expectedAgentUser, error) {
	serviceUID, err := formationServiceUID()
	if err != nil {
		return expectedAgentUser{}, runExecutionError("agent_user_unresolved", "formations executor could not resolve the service user", "executor", err)
	}
	name := strings.TrimSpace(e.config.AgentUser)
	if name == "" {
		return expectedAgentUser{uid: serviceUID, self: true}, nil
	}
	agentUID, err := formationLookupUID(name)
	if err != nil {
		return expectedAgentUser{}, runExecutionError("agent_user_unresolved", fmt.Sprintf("configured formations agent-user %q could not be resolved", name), "executor", err)
	}
	return expectedAgentUser{uid: agentUID, self: agentUID == serviceUID, name: name}, nil
}

// verifyServerOwner fails loud unless the tmux socket's backing server is owned by
// the expected agent-user uid. This subsumes chrote-fjy: a server owned by anyone
// other than the configured agent-user is refused rather than silently running
// agents under the wrong identity — including a self-mode socket that already
// hosts a foreign server.
func (e *TmuxFormationExecutor) verifyServerOwner(expected expectedAgentUser) error {
	ownerUID, err := formationSocketOwnerUID(e.config.Socket)
	if err != nil {
		return runExecutionError("agent_user_owner_unverified", fmt.Sprintf("could not determine the owner of tmux socket %s", e.config.Socket), "executor", err)
	}
	if ownerUID != expected.uid {
		return runExecutionError("agent_user_owner_mismatch", fmt.Sprintf("tmux socket %s is owned by uid %d but formations agents must run as uid %d (configured agent-user)", e.config.Socket, ownerUID, expected.uid), "executor", nil)
	}
	return nil
}

type TmuxExecutorConfig struct {
	Harnesses []string
	Socket    string
	Cwd       string
	Roots     []string
	// AgentUser is the Unix user the executor expects to own the tmux server it
	// drives (and therefore the user agents run as). Empty defaults to the service
	// user the CHROTE process runs as, so single-user installs need zero config.
	// Nothing is hardcoded to a specific username; a split install sets this to
	// the operator/agent-user and provisions a correctly-owned server out of band.
	AgentUser      string
	SessionPrefix  string
	OutputCapBytes int
	TimeoutSeconds int
}

type TmuxFormationExecutor struct {
	store          *Store
	personas       *PersonaStore
	config         TmuxExecutorConfig
	client         tmuxHarnessClient
	socketIdentity os.FileInfo
}

// tmuxHarnessClient is the executor's entire surface onto tmux. It is
// deliberately narrow: it can probe whether a server is running (read-only),
// lazy-START a server via a keeper session, enumerate sessions (read-only),
// CREATE a session, KILL a named session, describe/capture a pane (read-only)
// and send a prompt. There is intentionally NO kill-server, rename, resize,
// respawn, or attach operation, so the shared-socket safety invariant — never
// disrupt a session the executor did not itself create — cannot be violated by
// construction. HasServer/StartKeeper only ever ADD infrastructure (a server and
// its keeper); they never touch or tear down anything, owned or foreign.
type tmuxHarnessClient interface {
	HasServer(ctx context.Context, socket string) (bool, error)
	StartKeeper(ctx context.Context, socket, keeper string) error
	ListSessions(ctx context.Context, socket string) ([]string, error)
	CreateSession(ctx context.Context, socket, name, cwd, launch string) error
	KillSession(ctx context.Context, socket, name string) error
	DescribeActivePane(ctx context.Context, socket, target string) (tmuxPaneState, error)
	SendPrompt(ctx context.Context, socket, target, dispatchID, prompt string) error
	CapturePane(ctx context.Context, socket, target string, maxBytes int) (string, error)
}

// ownedSessions tracks the non-persistent tmux sessions a single formation
// execution created on the (shared) socket. Only names recorded here may ever be
// torn down, guaranteeing teardown never touches a foreign session. It is keyed
// by slot so a slot dispatched more than once in one execution (e.g. a peer that
// also takes the facilitator turn) reuses its own session instead of spawning a
// second one.
type ownedSessions struct {
	bySlot  map[string]string
	startAt map[string]time.Time
	order   []string
}

func newOwnedSessions() *ownedSessions {
	return &ownedSessions{bySlot: map[string]string{}, startAt: map[string]time.Time{}}
}

func (o *ownedSessions) name(slotID string) (string, bool) {
	if o == nil {
		return "", false
	}
	name, ok := o.bySlot[slotID]
	return name, ok
}

func (o *ownedSessions) record(slotID, name string) {
	if o == nil {
		return
	}
	if _, ok := o.bySlot[slotID]; ok {
		return
	}
	o.bySlot[slotID] = name
	o.order = append(o.order, name)
	// Record the session's dispatch-start once, when it is first created. Reused
	// dispatches on the same session (e.g. a peer that also takes the facilitator
	// turn) keep this earliest start so the transcript-capture mtime guard still
	// includes the same, growing transcript file and previousSentinels can tell a
	// prior turn's sentinel from this turn's.
	if o.startAt != nil {
		if _, ok := o.startAt[slotID]; !ok {
			o.startAt[slotID] = time.Now()
		}
	}
}

// startedAt reports when the slot's owned session was first created. The zero
// value / false is returned for an unknown slot; callers fall back to now.
func (o *ownedSessions) startedAt(slotID string) (time.Time, bool) {
	if o == nil || o.startAt == nil {
		return time.Time{}, false
	}
	at, ok := o.startAt[slotID]
	return at, ok
}

func (o *ownedSessions) owns(name string) bool {
	if o == nil {
		return false
	}
	for _, owned := range o.order {
		if owned == name {
			return true
		}
	}
	return false
}

type tmuxPaneState struct {
	Dead        bool
	CurrentPath string
}

func TmuxExecutorConfigFromEnv() TmuxExecutorConfig {
	capBytes := defaultTmuxOutputCapBytes
	if raw := strings.TrimSpace(os.Getenv("CHROTE_FORMATIONS_TMUX_OUTPUT_CAP_BYTES")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			capBytes = parsed
		}
	}
	timeoutSeconds := defaultTmuxTimeoutSeconds
	if raw := strings.TrimSpace(os.Getenv("CHROTE_FORMATIONS_TMUX_TIMEOUT_SECONDS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			timeoutSeconds = parsed
		}
	}
	return TmuxExecutorConfig{
		Harnesses:      splitLabCSV(os.Getenv("CHROTE_FORMATIONS_TMUX_HARNESSES")),
		Socket:         strings.TrimSpace(os.Getenv("CHROTE_FORMATIONS_TMUX_SOCKET")),
		Cwd:            strings.TrimSpace(os.Getenv("CHROTE_FORMATIONS_TMUX_CWD")),
		Roots:          splitLabCSV(os.Getenv("CHROTE_FORMATIONS_TMUX_ROOTS")),
		AgentUser:      strings.TrimSpace(os.Getenv("CHROTE_FORMATIONS_AGENT_USER")),
		SessionPrefix:  strings.TrimSpace(os.Getenv("CHROTE_FORMATIONS_TMUX_SESSION_PREFIX")),
		OutputCapBytes: capBytes,
		TimeoutSeconds: timeoutSeconds,
	}
}

func NewTmuxFormationExecutor(store *Store, personas *PersonaStore, config TmuxExecutorConfig) *TmuxFormationExecutor {
	return newTmuxFormationExecutorWithClient(store, personas, config, realTmuxHarnessClient{})
}

func newTmuxFormationExecutorWithClient(store *Store, personas *PersonaStore, config TmuxExecutorConfig, client tmuxHarnessClient) *TmuxFormationExecutor {
	if config.OutputCapBytes <= 0 {
		config.OutputCapBytes = defaultTmuxOutputCapBytes
	}
	if config.TimeoutSeconds <= 0 {
		config.TimeoutSeconds = defaultTmuxTimeoutSeconds
	}
	if client == nil {
		client = realTmuxHarnessClient{}
	}
	return &TmuxFormationExecutor{store: store, personas: personas, config: config, client: client}
}

type tmuxSlotOutput struct {
	SlotID     string
	AgentID    string
	Harness    string
	SessionRef string
	Artifact   string
	Phase      string
	Text       string
}

type tmuxSlotBinding struct {
	Slot        FormationSlot
	Card        *PersonaCard
	Variant     HarnessVariant
	SessionName string
}

type peerPlaneHandle struct {
	ledger       *runLedgerHandle
	name         string
	relativePath string
}

func (h *peerPlaneHandle) close() {
	if h != nil && h.ledger != nil {
		h.ledger.close()
	}
}

func (o tmuxSlotOutput) summary() string {
	return fmt.Sprintf("tmux harness completed for agent %s harness %s sessionRef %s artifact %s", o.AgentID, o.Harness, o.SessionRef, o.Artifact)
}

func (e *TmuxFormationExecutor) ExecuteFormation(req FormationExecution) (FormationExecutionResult, error) {
	if e == nil || e.store == nil {
		return FormationExecutionResult{}, runExecutionError("missing_executor", "tmux executor store is not configured", "executor", ErrRunExecutorUnavailable)
	}
	if err := e.store.RequireRuntimeAuthority(); err != nil {
		return FormationExecutionResult{}, err
	}
	if err := e.validateConfiguredBoundary(); err != nil {
		return FormationExecutionResult{}, err
	}
	if len(req.Formation.Slots) == 0 {
		return FormationExecutionResult{}, runExecutionError("missing_slot", fmt.Sprintf("formation %q has no slots to dispatch", req.NodeID), "executor", nil)
	}
	if req.Formation.Type == FormationTypeOrchestrated {
		return e.executeOrchestratedFormation(req)
	}
	if req.Formation.Type == FormationTypePeer {
		return e.executePeerFormation(req)
	}

	allowed := e.allowedHarnesses()
	dispatcher := NewSlotDispatcher(e.store, nil)
	owned := newOwnedSessions()
	defer e.teardownOwnedSessions(owned)
	outputs := make([]string, 0, len(req.Formation.Slots))
	for _, slot := range req.Formation.Slots {
		output, err := e.executeSlot(req, slot, allowed, dispatcher, "", outputContractExtraLines(req.Formation), owned)
		if err != nil {
			return FormationExecutionResult{}, err
		}
		text := strings.TrimSpace(output.Text)
		if text == "" {
			text = output.summary()
		}
		outputs = append(outputs, text)
	}
	text := strings.Join(outputs, "\n\n")
	return e.formationResultFromText(req, fmt.Sprintf("tmux://%s/%s/report", req.RunID, req.NodeID), text)
}

func (e *TmuxFormationExecutor) ReattachFormationDispatch(req FormationReattachRequest) (FormationExecutionResult, error) {
	if e == nil || e.store == nil {
		return FormationExecutionResult{}, runExecutionError("missing_executor", "tmux executor store is not configured", "executor", ErrRunExecutorUnavailable)
	}
	if err := e.store.RequireRuntimeAuthority(); err != nil {
		return FormationExecutionResult{}, err
	}
	if err := e.validateConfiguredBoundary(); err != nil {
		return FormationExecutionResult{}, err
	}
	if req.DispatchID == "" || req.NodeID == "" || req.SlotID == "" {
		return FormationExecutionResult{}, runExecutionError("invalid_reattach", "reattach requires dispatch, node, and slot identifiers", "recovery", nil)
	}
	dispatcher := NewSlotDispatcher(e.store, nil)
	dispatch := dispatcher.dispatchEvent(req.RunID, req.DispatchID)
	if dispatch.Type == "" {
		return FormationExecutionResult{}, runExecutionError("unknown_dispatch", fmt.Sprintf("dispatch %q was not found", req.DispatchID), "recovery", nil)
	}
	sessionRef := stringFromEventData(dispatch, "sessionRef")
	sessionName, ok := strings.CutPrefix(sessionRef, "tmux:")
	if !ok || !safeTmuxSessionName(sessionName) {
		return FormationExecutionResult{}, runExecutionError("invalid_session", fmt.Sprintf("dispatch %q has invalid sessionRef %q", req.DispatchID, sessionRef), "recovery", nil)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(e.config.TimeoutSeconds)*time.Second)
	defer cancel()
	if err := e.validatePinnedTmuxSocket(); err != nil {
		return FormationExecutionResult{}, withSlot(err, req.NodeID, req.SlotID, req.DispatchID)
	}
	captured, err := e.client.CapturePane(ctx, e.config.Socket, sessionName, e.config.OutputCapBytes+1)
	if err != nil {
		return FormationExecutionResult{}, runSlotExecutionError("reattach_capture_failed", redactLedgerText(err.Error()), "adapter", err, req.NodeID, req.SlotID, req.DispatchID)
	}
	capBytes := e.config.OutputCapBytes
	if capBytes <= 0 {
		capBytes = defaultTmuxOutputCapBytes
	}
	if len(captured) > capBytes {
		return FormationExecutionResult{}, runSlotExecutionError("oversized_output", "tmux captured output exceeds configured cap", "adapter", nil, req.NodeID, req.SlotID, req.DispatchID)
	}
	sentinel, _ := ParseCompletionSentinel(captured, req.RunID)
	artifact := redactLedgerText(sentinel.Artifact)
	if artifact == "" {
		artifact = "tmux://" + req.RunID + "/" + req.NodeID + "/" + req.SlotID
	}
	text := extractCapturedSlotText(captured, "", req.RunID)
	if strings.TrimSpace(text) == "" {
		text = fmt.Sprintf("tmux harness completed for sessionRef %s artifact %s", sessionRef, artifact)
	}
	result, err := e.formationResultFromText(FormationExecution{
		RunID:     req.RunID,
		NodeID:    req.NodeID,
		Formation: req.Formation,
	}, artifact, text)
	if err != nil {
		return FormationExecutionResult{}, err
	}
	if err := dispatcher.CompleteFromCapture(req.RunID, req.DispatchID, captured); err != nil {
		return FormationExecutionResult{}, err
	}
	return result, nil
}

func (e *TmuxFormationExecutor) executeOrchestratedFormation(req FormationExecution) (FormationExecutionResult, error) {
	controller, workers, err := splitOrchestratedSlots(req.Formation)
	if err != nil {
		return FormationExecutionResult{}, err
	}
	allowed := e.allowedHarnesses()
	dispatcher := NewSlotDispatcher(e.store, nil)
	owned := newOwnedSessions()
	defer e.teardownOwnedSessions(owned)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(e.config.TimeoutSeconds)*time.Second)
	defer cancel()

	controllerBinding, err := e.resolveSlotBinding(ctx, req, controller, allowed, owned)
	if err != nil {
		return FormationExecutionResult{}, withSlot(err, req.NodeID, controller.ID, "")
	}
	workerBindings := make([]tmuxSlotBinding, 0, len(workers))
	for _, worker := range workers {
		binding, err := e.resolveSlotBinding(ctx, req, worker, allowed, owned)
		if err != nil {
			return FormationExecutionResult{}, e.failWorkerBind(req, controllerBinding, worker, owned, err)
		}
		workerBindings = append(workerBindings, binding)
	}
	if err := e.appendOrchestrationTeamEvent(req, controllerBinding, workerBindings); err != nil {
		return FormationExecutionResult{}, err
	}
	baselines, err := e.captureWorkerBaselines(req, controllerBinding, workerBindings)
	if err != nil {
		return FormationExecutionResult{}, err
	}

	leaderExtra := append(e.leaderAgenticExtraLines(controllerBinding, workerBindings), outputContractExtraLines(req.Formation)...)
	leader, leaderErr := e.executeSlot(req, controller, allowed, dispatcher, "leader-agentic", leaderExtra, owned)
	// Observe the workers whether or not the leader finished: a leader that timed
	// out is exactly when a hung or never-touched worker most needs evidence. The
	// leader's own failure still wins as the returned error.
	observeErr := e.observeWorkerOutcomes(req, controllerBinding, baselines)
	if leaderErr != nil {
		return FormationExecutionResult{}, leaderErr
	}
	if observeErr != nil {
		return FormationExecutionResult{}, observeErr
	}
	text := leader.Text
	if strings.TrimSpace(text) == "" {
		text = leader.summary()
	}
	return e.formationResultFromText(req, leader.Artifact, text)
}

func (e *TmuxFormationExecutor) executePeerFormation(req FormationExecution) (FormationExecutionResult, error) {
	peers, err := peerFormationSlots(req.Formation)
	if err != nil {
		return FormationExecutionResult{}, err
	}
	allowed := e.allowedHarnesses()
	dispatcher := NewSlotDispatcher(e.store, nil)
	owned := newOwnedSessions()
	defer e.teardownOwnedSessions(owned)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(e.config.TimeoutSeconds)*time.Second)
	defer cancel()

	bindings := make([]tmuxSlotBinding, 0, len(peers))
	for _, peer := range peers {
		binding, err := e.resolveSlotBinding(ctx, req, peer, allowed, owned)
		if err != nil {
			return FormationExecutionResult{}, withSlot(err, req.NodeID, peer.ID, "")
		}
		bindings = append(bindings, binding)
	}
	plane, err := e.seedPeerPlane(req, bindings)
	if err != nil {
		return FormationExecutionResult{}, err
	}
	defer plane.close()
	if err := e.appendPeerPlaneEvent(req, plane.relativePath, bindings); err != nil {
		return FormationExecutionResult{}, err
	}

	for i, peer := range peers {
		output, err := e.executeSlot(req, peer, allowed, dispatcher, "peer-turn", e.peerTurnExtraLines(plane.relativePath, bindings, bindings[i], false), owned)
		if err != nil {
			return FormationExecutionResult{}, err
		}
		if err := e.appendPeerPlaneOutput(plane, output); err != nil {
			return FormationExecutionResult{}, err
		}
	}

	facilitator := peers[0]
	facilitatorBinding := bindings[0]
	facilitatorExtra := append(e.peerTurnExtraLines(plane.relativePath, bindings, facilitatorBinding, true), outputContractExtraLines(req.Formation)...)
	final, err := e.executeSlot(req, facilitator, allowed, dispatcher, "peer-facilitator", facilitatorExtra, owned)
	if err != nil {
		return FormationExecutionResult{}, err
	}
	if err := e.appendPeerPlaneOutput(plane, final); err != nil {
		return FormationExecutionResult{}, err
	}
	text := final.Text
	if strings.TrimSpace(text) == "" {
		text = final.summary()
	}
	return e.formationResultFromText(req, final.Artifact, text)
}

func (e *TmuxFormationExecutor) formationResultFromText(req FormationExecution, reportRef, text string) (FormationExecutionResult, error) {
	cleanText, namedOutputs, err := parseChroteOutputs(text)
	if err != nil {
		return FormationExecutionResult{}, runExecutionError("invalid_output_payloads", err.Error(), "executor", err)
	}
	knownPorts := map[string]bool{}
	for _, output := range req.Formation.Outputs {
		knownPorts[output.ID] = true
		if _, ok := namedOutputs[output.ID]; !ok {
			return FormationExecutionResult{}, runExecutionError("missing_output_payload", fmt.Sprintf("formation %q did not emit required output port %q", req.NodeID, output.ID), "executor", nil)
		}
	}
	for portID := range namedOutputs {
		if !knownPorts[portID] {
			return FormationExecutionResult{}, runExecutionError("invalid_output_payloads", fmt.Sprintf("formation %q emitted unknown output port %q", req.NodeID, portID), "executor", nil)
		}
	}
	if err := e.materializeOutputRefs(namedOutputs); err != nil {
		return FormationExecutionResult{}, err
	}
	if len(cleanText) > e.config.OutputCapBytes {
		return FormationExecutionResult{}, runExecutionError("oversized_output", "tmux executor output exceeds configured cap", "adapter", nil)
	}
	return FormationExecutionResult{
		Status:    "done",
		ReportRef: reportRef,
		Text:      cleanText,
		Outputs:   namedOutputs,
	}, nil
}

func (e *TmuxFormationExecutor) materializeOutputRefs(outputs map[string]FormationOutputPayload) error {
	if len(outputs) == 0 {
		return nil
	}
	for portID, payload := range outputs {
		ref := strings.TrimSpace(payload.Ref)
		if ref == "" {
			continue
		}
		body, err := e.readOutputRefArtifact(ref)
		if err != nil {
			return err
		}
		payload.Text = redactLedgerText(body)
		outputs[portID] = payload
	}
	return nil
}

func (e *TmuxFormationExecutor) readOutputRefArtifact(ref string) (string, error) {
	path, err := e.resolveOutputRefPath(ref)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", runExecutionError("unavailable_output_ref", fmt.Sprintf("output ref %q is not stat-able", ref), "executor", err)
	}
	if !info.Mode().IsRegular() {
		return "", runExecutionError("invalid_output_ref", fmt.Sprintf("output ref %q is not a regular file", ref), "executor", nil)
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		code := "unavailable_output_ref"
		if errors.Is(err, syscall.ELOOP) {
			code = "output_ref_outside_root"
		}
		return "", runExecutionError(code, fmt.Sprintf("output ref %q is not readable", ref), "executor", err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		syscall.Close(fd)
		return "", runExecutionError("unavailable_output_ref", fmt.Sprintf("output ref %q could not be opened", ref), "executor", nil)
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return "", runExecutionError("unavailable_output_ref", fmt.Sprintf("output ref %q is not stat-able", ref), "executor", err)
	}
	if !info.Mode().IsRegular() {
		return "", runExecutionError("invalid_output_ref", fmt.Sprintf("output ref %q is not a regular file", ref), "executor", nil)
	}
	capBytes := e.config.OutputCapBytes
	if capBytes <= 0 {
		capBytes = defaultTmuxOutputCapBytes
	}
	raw, err := io.ReadAll(io.LimitReader(file, int64(capBytes)+1))
	if err != nil {
		return "", runExecutionError("unavailable_output_ref", fmt.Sprintf("output ref %q could not be read", ref), "executor", err)
	}
	if len(raw) > capBytes {
		return "", runExecutionError("oversized_output_ref", fmt.Sprintf("output ref %q exceeds configured output cap", ref), "executor", nil)
	}
	if bytes.Contains(raw, []byte{0}) {
		return "", runExecutionError("invalid_output_ref", fmt.Sprintf("output ref %q is not text", ref), "executor", nil)
	}
	return string(raw), nil
}

func (e *TmuxFormationExecutor) resolveOutputRefPath(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", runExecutionError("invalid_output_ref", "empty output ref", "executor", nil)
	}
	if strings.ContainsRune(ref, 0) || strings.Contains(ref, "://") {
		return "", runExecutionError("invalid_output_ref", fmt.Sprintf("unsupported output ref %q", ref), "executor", nil)
	}
	var candidate string
	if filepath.IsAbs(ref) {
		candidate = filepath.Clean(ref)
	} else {
		base := e.config.Cwd
		if e.store != nil && strings.TrimSpace(e.store.workspaceRoot()) != "" {
			base = e.store.workspaceRoot()
		}
		candidate = filepath.Join(base, filepath.FromSlash(ref))
	}
	candidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", runExecutionError("invalid_output_ref", fmt.Sprintf("output ref %q is invalid", ref), "executor", err)
	}
	if !e.pathWithinRoots(candidate) {
		return "", runExecutionError("output_ref_outside_root", fmt.Sprintf("output ref %q is outside configured roots", ref), "executor", nil)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		if os.IsNotExist(err) {
			return "", runExecutionError("unavailable_output_ref", fmt.Sprintf("output ref %q does not exist", ref), "executor", err)
		}
		return "", runExecutionError("unavailable_output_ref", fmt.Sprintf("output ref %q could not be resolved", ref), "executor", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", runExecutionError("invalid_output_ref", fmt.Sprintf("output ref %q is invalid", ref), "executor", err)
	}
	if !e.pathWithinRoots(resolved) {
		return "", runExecutionError("output_ref_outside_root", fmt.Sprintf("output ref %q resolves outside configured roots", ref), "executor", nil)
	}
	return resolved, nil
}

func outputContractExtraLines(formation FormationNode) []string {
	if len(formation.Outputs) == 0 {
		return nil
	}
	lines := []string{
		"formation output contract:",
		"Every formation output must be emitted through exactly one fenced JSON block before the sentinel. Keep JSON string values short; terminal wrapping corrupts long JSON strings.",
		"Required routing payload shape:",
		"```chrote-outputs",
		"{",
	}
	for i, output := range formation.Outputs {
		comma := ","
		if i == len(formation.Outputs)-1 {
			comma = ""
		}
		lines = append(lines, fmt.Sprintf("  %q: {\"text\": \"one-line summary\", \"ref\": \"artifact/path.md\"}%s", output.ID, comma))
	}
	lines = append(lines,
		"}",
		"```",
		"Use all and only the exact output port ids in that skeleton.",
	)
	lines = append(lines,
		"Do not rely on free-form answer text for routing; it is display-only. Missing or unknown output ids block the run.",
		"Use text for a short, non-secret routed payload or summary.",
		"For longer payloads, create a text artifact under the artifact directory shown below and put its path in ref; CHROTE reads that file for routing.",
		"Do not point ref at arbitrary host files or secrets. Invalid, unreadable, out-of-root, symlink-escaped, non-text, or oversized refs block the run.",
		"Before the CHROTE-DONE sentinel, you MUST emit a fresh ```chrote-outputs fenced JSON block for this run. Do not omit the fence.",
	)
	return lines
}

func parseChroteOutputs(text string) (string, map[string]FormationOutputPayload, error) {
	start := strings.LastIndex(text, "```chrote-outputs")
	if start == -1 {
		if clean, outputs, ok := parseBareChroteOutputs(text); ok {
			return clean, outputs, nil
		}
		return strings.TrimSpace(text), nil, nil
	}
	afterStart := text[start:]
	newline := strings.Index(afterStart, "\n")
	if newline == -1 {
		return "", nil, fmt.Errorf("chrote-outputs fence is missing JSON body")
	}
	jsonStart := start + newline + 1
	afterJSONStart := text[jsonStart:]
	endRel := strings.Index(afterJSONStart, "```")
	if endRel == -1 {
		return "", nil, fmt.Errorf("chrote-outputs fence is missing closing fence")
	}
	jsonText := strings.TrimSpace(afterJSONStart[:endRel])
	raw, err := decodeChroteOutputsJSON(jsonText)
	if err != nil {
		return "", nil, fmt.Errorf("chrote-outputs JSON is invalid: %w", err)
	}
	outputs := outputPayloadsFromAny(raw)
	if len(outputs) == 0 {
		return "", nil, fmt.Errorf("chrote-outputs JSON must contain at least one output payload")
	}
	end := jsonStart + endRel + len("```")
	clean := strings.TrimSpace(text[:start] + text[end:])
	return clean, outputs, nil
}

func parseBareChroteOutputs(text string) (string, map[string]FormationOutputPayload, bool) {
	trimmed := strings.TrimSpace(text)
	for start := 0; start < len(trimmed); start++ {
		if trimmed[start] != '{' {
			continue
		}
		raw, ok := decodeBareChroteOutputsCandidate(trimmed[start:])
		if !ok {
			continue
		}
		if !hasOutputPortKey(raw) {
			continue
		}
		outputs := outputPayloadsFromAny(raw)
		if len(outputs) == 0 {
			continue
		}
		return strings.TrimSpace(trimmed[:start]), outputs, true
	}
	return strings.TrimSpace(text), nil, false
}

func decodeBareChroteOutputsCandidate(candidate string) (map[string]any, bool) {
	decode := func(rawText string) (map[string]any, bool) {
		decoder := json.NewDecoder(strings.NewReader(rawText))
		var raw map[string]any
		if err := decoder.Decode(&raw); err != nil {
			return nil, false
		}
		if strings.TrimSpace(rawText[int(decoder.InputOffset()):]) != "" {
			return nil, false
		}
		return raw, true
	}
	if raw, ok := decode(candidate); ok {
		return raw, true
	}
	return decode(repairTerminalWrappedJSONStrings(candidate))
}

func decodeChroteOutputsJSON(jsonText string) (map[string]any, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(jsonText), &raw); err != nil {
		if repairedErr := json.Unmarshal([]byte(repairTerminalWrappedJSONStrings(jsonText)), &raw); repairedErr == nil {
			return raw, nil
		}
		return nil, err
	}
	return raw, nil
}

func repairTerminalWrappedJSONStrings(jsonText string) string {
	var b strings.Builder
	inString := false
	escaped := false
	for i := 0; i < len(jsonText); i++ {
		ch := jsonText[i]
		if inString && ch == '\n' {
			for i+1 < len(jsonText) && (jsonText[i+1] == ' ' || jsonText[i+1] == '	') {
				i++
			}
			continue
		}
		b.WriteByte(ch)
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
		}
	}
	return b.String()
}

func hasOutputPortKey(raw map[string]any) bool {
	for key := range raw {
		if strings.HasPrefix(key, "port_") {
			return true
		}
	}
	return false
}

func (e *TmuxFormationExecutor) executeSlot(req FormationExecution, slot FormationSlot, allowed map[string]bool, dispatcher *SlotDispatcher, phase string, extraLines []string, owned *ownedSessions) (tmuxSlotOutput, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(e.config.TimeoutSeconds)*time.Second)
	defer cancel()
	binding, err := e.resolveSlotBinding(ctx, req, slot, allowed, owned)
	if err != nil {
		return tmuxSlotOutput{}, withSlot(err, req.NodeID, slot.ID, "")
	}
	card := binding.Card
	variant := binding.Variant
	sessionName := binding.SessionName

	// dispatchStart bounds the transcript-capture mtime guard (claude-code). It is
	// the session's creation time so a reused session (facilitator turn) keeps its
	// growing transcript in scope; it falls back to now for a session not tracked
	// as owned. Non-transcript harnesses ignore it.
	dispatchStart, ok := owned.startedAt(slot.ID)
	if !ok {
		dispatchStart = time.Now()
	}
	turnMarker := "turn marker: " + newPrefixedID("turn")
	promptExtra := append(append([]string{}, extraLines...), turnMarker)
	prompt := e.renderPromptWithContext(req, slot, *card, variant, phase, promptExtra)
	existingSentinels, err := e.countExistingCompletionSentinels(ctx, sessionName, req.RunID, variant.ID, dispatchStart)
	if err != nil {
		return tmuxSlotOutput{}, withSlot(err, req.NodeID, slot.ID, "")
	}
	lease, err := dispatcher.DispatchSlot(req.RunID, SlotDispatchRequest{
		NodeID:      req.NodeID,
		SlotID:      slot.ID,
		AgentID:     card.ID,
		Harness:     variant.ID,
		SessionStem: variant.SessionStem,
		SessionRef:  "tmux:" + sessionName,
		Prompt:      prompt,
		Phase:       phase,
		Attempt:     req.Attempt,
	})
	if err != nil {
		return tmuxSlotOutput{}, err
	}
	if err := e.validatePinnedTmuxSocket(); err != nil {
		return tmuxSlotOutput{}, withSlot(err, req.NodeID, slot.ID, lease.DispatchID)
	}
	if err := e.client.SendPrompt(ctx, e.config.Socket, sessionName, lease.DispatchID, prompt); err != nil {
		return tmuxSlotOutput{}, runSlotExecutionError("dispatch_failed", redactPromptFromLedgerText(err.Error(), prompt), "adapter", err, req.NodeID, slot.ID, lease.DispatchID)
	}
	if err := e.appendAdapterSend(req.RunID, req.NodeID, slot.ID, lease.DispatchID, sessionName, prompt, phase); err != nil {
		return tmuxSlotOutput{}, err
	}

	captured, err := e.waitForCompletion(ctx, sessionName, req.RunID, prompt, existingSentinels, variant.ID, dispatchStart)
	if err != nil {
		return tmuxSlotOutput{}, withSlot(err, req.NodeID, slot.ID, lease.DispatchID)
	}
	if err := dispatcher.CompleteFromCapture(req.RunID, lease.DispatchID, captured); err != nil {
		return tmuxSlotOutput{}, err
	}
	sentinel, _ := ParseCompletionSentinel(captured, req.RunID)
	artifact := redactLedgerText(sentinel.Artifact)
	if artifact == "" {
		artifact = "tmux://" + req.RunID + "/" + req.NodeID + "/" + slot.ID
	}
	return tmuxSlotOutput{
		SlotID:     slot.ID,
		AgentID:    card.ID,
		Harness:    variant.ID,
		SessionRef: "tmux:" + sessionName,
		Artifact:   artifact,
		Phase:      phase,
		Text:       extractCapturedSlotText(captured, prompt, req.RunID),
	}, nil
}

func (e *TmuxFormationExecutor) resolveSlotBinding(ctx context.Context, req FormationExecution, slot FormationSlot, allowed map[string]bool, owned *ownedSessions) (tmuxSlotBinding, error) {
	if slot.AgentID == "" {
		return tmuxSlotBinding{}, runExecutionError("missing_agent", fmt.Sprintf("slot %q is not staffed", slot.ID), "executor", nil)
	}
	if e.personas == nil {
		return tmuxSlotBinding{}, runExecutionError("missing_persona_store", "persona store is not configured", "executor", nil)
	}
	card, err := e.personas.ReadPersona(slot.AgentID)
	if err != nil {
		return tmuxSlotBinding{}, runExecutionError("missing_persona", fmt.Sprintf("persona %q could not be resolved", slot.AgentID), "executor", err)
	}
	variant, err := card.SelectHarnessVariant(slot.Harness)
	if err != nil {
		code := "missing_harness"
		if errors.Is(err, ErrAmbiguousAgentBinding) {
			code = "ambiguous_harness"
		}
		return tmuxSlotBinding{}, runExecutionError(code, redactLedgerText(err.Error()), "executor", err)
	}
	if !allowed[variant.ID] {
		return tmuxSlotBinding{}, runExecutionError("unconfigured_harness", fmt.Sprintf("tmux executor is not configured for harness %q", variant.ID), "executor", nil)
	}
	if variant.SessionStem == "" {
		return tmuxSlotBinding{}, runExecutionError("missing_session", fmt.Sprintf("agent %q harness %q has no session stem", card.ID, variant.ID), "executor", nil)
	}
	sessionName, err := e.provisionOwnedSession(ctx, owned, req.RunID, slot, variant)
	if err != nil {
		return tmuxSlotBinding{}, err
	}
	return tmuxSlotBinding{
		Slot:        slot,
		Card:        card,
		Variant:     variant,
		SessionName: sessionName,
	}, nil
}

// provisionOwnedSession returns a tmux session this execution owns for the slot,
// spawning a fresh non-persistent one on demand when none exists yet. The
// spawned session runs the harness launch command in the configured cwd; the
// executor owns its whole lifecycle and tears it down at the end of the run.
// A slot already provisioned in this execution reuses its own session.
func (e *TmuxFormationExecutor) provisionOwnedSession(ctx context.Context, owned *ownedSessions, runID string, slot FormationSlot, variant HarnessVariant) (string, error) {
	if name, ok := owned.name(slot.ID); ok {
		if err := e.ensureOwnedSessionReady(ctx, name); err != nil {
			return "", err
		}
		return name, nil
	}
	launch := strings.TrimSpace(variant.Launch)
	if launch == "" {
		return "", runExecutionError("missing_launch", fmt.Sprintf("agent %q harness %q has no launch command to spawn an on-demand session", slot.AgentID, variant.ID), "executor", nil)
	}
	name, err := e.pickOwnedSessionName(ctx, runID, slot.ID)
	if err != nil {
		return "", err
	}
	if err := e.validatePinnedTmuxSocket(); err != nil {
		return "", err
	}
	if err := e.client.CreateSession(ctx, e.config.Socket, name, e.config.Cwd, launch); err != nil {
		return "", runExecutionError("session_spawn_failed", redactLedgerText(err.Error()), "adapter", err)
	}
	// Record ownership immediately after a successful create so teardown reclaims
	// the session even if the readiness check below fails.
	owned.record(slot.ID, name)
	if err := e.ensureOwnedSessionReady(ctx, name); err != nil {
		return "", err
	}
	return name, nil
}

// pickOwnedSessionName derives a collision-proof, executor-owned tmux session
// name (run + slot scoped, random-nonce suffixed). It refuses any candidate that
// already exists on the socket, so it can never reuse or alias a pre-existing /
// foreign session; if it cannot find a free name it fails closed.
func (e *TmuxFormationExecutor) pickOwnedSessionName(ctx context.Context, runID, slotID string) (string, error) {
	if err := e.validatePinnedTmuxSocket(); err != nil {
		return "", err
	}
	existing, err := e.client.ListSessions(ctx, e.config.Socket)
	if err != nil {
		return "", runExecutionError("tmux_unavailable", redactLedgerText(err.Error()), "adapter", err)
	}
	taken := make(map[string]bool, len(existing))
	for _, name := range existing {
		taken[name] = true
	}
	for attempt := 0; attempt < ownedSessionNameAttempts; attempt++ {
		candidate := e.config.SessionPrefix + sanitizeSessionComponent(runID) + "-" + sanitizeSessionComponent(slotID) + "-" + newSessionNonce()
		if !safeTmuxSessionName(candidate) {
			continue
		}
		if taken[candidate] {
			// Fail-closed on collision: never reuse or touch a session we did
			// not create; regenerate with fresh entropy instead.
			continue
		}
		return candidate, nil
	}
	return "", runExecutionError("session_name_collision", "could not derive a collision-free owned tmux session name", "executor", nil)
}

// ensureOwnedSessionReady confirms an owned session is live and rooted in the
// workspace before dispatch. It targets only the exact owned name, so it never
// enumerates or inspects foreign sessions.
func (e *TmuxFormationExecutor) ensureOwnedSessionReady(ctx context.Context, sessionName string) error {
	if err := e.validatePinnedTmuxSocket(); err != nil {
		return err
	}
	pane, err := e.client.DescribeActivePane(ctx, e.config.Socket, sessionName)
	if err != nil {
		code := "pane_unavailable"
		if errors.Is(err, errTmuxTargetMissing) {
			code = "missing_session"
		}
		return runExecutionError(code, redactLedgerText(err.Error()), "adapter", err)
	}
	if pane.Dead {
		return runExecutionError("dead_pane", fmt.Sprintf("tmux session %q active pane is dead", sessionName), "adapter", ErrDispatchDeadPane)
	}
	if pane.CurrentPath != "" {
		paneCwd, err := filepath.Abs(pane.CurrentPath)
		if err != nil {
			return runExecutionError("invalid_cwd", "tmux pane cwd is invalid", "adapter", err)
		}
		if !e.pathWithinRoots(paneCwd) {
			return runExecutionError("cwd_outside_root", "tmux pane cwd is outside configured roots", "adapter", nil)
		}
	}
	return nil
}

// teardownOwnedSessions kills every session this execution created, and nothing
// else. It only proceeds while the pinned socket identity is intact, so a socket
// that was swapped mid-run cannot cause a kill on a different tmux server; it
// never issues kill-server and never targets a name it did not create.
func (e *TmuxFormationExecutor) teardownOwnedSessions(owned *ownedSessions) {
	if owned == nil || len(owned.order) == 0 {
		return
	}
	if err := e.validatePinnedTmuxSocket(); err != nil {
		return
	}
	for _, name := range owned.order {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(e.config.TimeoutSeconds)*time.Second)
		_ = e.client.KillSession(ctx, e.config.Socket, name)
		cancel()
	}
}

// sanitizeSessionComponent maps an identifier into the tmux-safe alphabet so a
// derived owned session name always passes safeTmuxSessionName.
func sanitizeSessionComponent(value string) string {
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('-')
	}
	if b.Len() == 0 {
		return "x"
	}
	return b.String()
}

func splitOrchestratedSlots(formation FormationNode) (FormationSlot, []FormationSlot, error) {
	var controller FormationSlot
	controllers := 0
	workers := make([]FormationSlot, 0, len(formation.Slots))
	for _, slot := range formation.Slots {
		if slot.Controller {
			controller = slot
			controllers++
			continue
		}
		workers = append(workers, slot)
	}
	switch {
	case controllers == 0:
		return FormationSlot{}, nil, runExecutionError("missing_controller", fmt.Sprintf("orchestrated formation %q has no controller slot", formation.ID), "executor", nil)
	case controllers > 1:
		return FormationSlot{}, nil, runExecutionError("ambiguous_controller", fmt.Sprintf("orchestrated formation %q has %d controller slots", formation.ID, controllers), "executor", nil)
	case len(workers) == 0:
		return FormationSlot{}, nil, runExecutionError("missing_worker", fmt.Sprintf("orchestrated formation %q has no worker slots", formation.ID), "executor", nil)
	}
	return controller, workers, nil
}

func peerFormationSlots(formation FormationNode) ([]FormationSlot, error) {
	peers := make([]FormationSlot, 0, len(formation.Slots))
	for _, slot := range formation.Slots {
		if slot.Controller {
			return nil, runExecutionError("peer_controller", fmt.Sprintf("peer formation %q must not mark slot %q as controller", formation.ID, slot.ID), "executor", nil)
		}
		peers = append(peers, slot)
	}
	if len(peers) < 2 {
		return nil, runExecutionError("missing_peer", fmt.Sprintf("peer formation %q needs at least two peer slots", formation.ID), "executor", nil)
	}
	return peers, nil
}

func (e *TmuxFormationExecutor) seedPeerPlane(req FormationExecution, peers []tmuxSlotBinding) (*peerPlaneHandle, error) {
	ledger, err := e.store.openRunLedger(req.RunID, false)
	if err != nil {
		return nil, err
	}
	plane := &peerPlaneHandle{
		ledger:       ledger,
		name:         req.RunID + ".peer.md",
		relativePath: runArtifactPath(ledger.directory.slug, req.RunID, ".peer.md"),
	}
	complete := false
	defer func() {
		if !complete {
			plane.close()
		}
	}()
	var b strings.Builder
	b.WriteString("# Peer Plane\n\n")
	b.WriteString("run: " + req.RunID + "\n")
	b.WriteString("formation: " + req.NodeID + "\n")
	b.WriteString("mode: no-hierarchy peer collaboration\n")
	b.WriteString("brief: " + redactLedgerText(req.Brief.Goal) + "\n")
	for _, input := range req.Inputs {
		if strings.TrimSpace(input.Text) != "" {
			b.WriteString("input: " + redactLedgerText(input.Text) + "\n")
		}
	}
	b.WriteString("\n## Working agreement\n")
	b.WriteString("- Read this plane before acting.\n")
	b.WriteString("- Append useful evidence, proposal, critique, question, or synthesis.\n")
	b.WriteString("- Peers may inspect scoped sibling tmux sessions when useful; no peer is the boss.\n")
	b.WriteString("- The temporary facilitator turn only nudges/synthesizes; it does not replace peer judgment.\n")
	b.WriteString("\n## Roster\n")
	for _, peer := range peers {
		b.WriteString(fmt.Sprintf("- slot %s label=%q agent=%q harness=%q session=%q\n", peer.Slot.ID, peer.Slot.Label, peer.Card.ID, peer.Variant.ID, peer.SessionName))
	}
	b.WriteString("\n## Seed\n")
	b.WriteString("Start by adding one concrete contribution, then read and respond to what the other peer adds.\n")
	if err := writeRunArtifactAtomicAt(ledger.directory, plane.name, []byte(b.String()), peerPlaneMaxBytes); err != nil {
		return nil, err
	}
	complete = true
	return plane, nil
}

func (e *TmuxFormationExecutor) appendPeerPlaneEvent(req FormationExecution, planeRel string, peers []tmuxSlotBinding) error {
	peerData := make([]map[string]any, 0, len(peers))
	for _, peer := range peers {
		peerData = append(peerData, map[string]any{
			"slotId":      peer.Slot.ID,
			"label":       peer.Slot.Label,
			"agentId":     peer.Card.ID,
			"harness":     peer.Variant.ID,
			"sessionStem": peer.Variant.SessionStem,
			"sessionRef":  "tmux:" + peer.SessionName,
		})
	}
	return e.store.AppendRunEvent(req.RunID, RunEvent{
		Type:    RunEventPeerPlane,
		NodeID:  req.NodeID,
		Attempt: req.Attempt,
		Data: map[string]any{
			"mode":   "shared-peer-plane",
			"path":   planeRel,
			"socket": e.config.Socket,
			"cwd":    e.config.Cwd,
			"peers":  peerData,
		},
	})
}

func (e *TmuxFormationExecutor) appendPeerPlaneOutput(plane *peerPlaneHandle, output tmuxSlotOutput) error {
	if plane == nil || plane.ledger == nil || plane.ledger.directory == nil {
		return ErrRunLedgerInvalid
	}
	var b strings.Builder
	b.WriteString("\n## ")
	if output.Phase == "peer-facilitator" {
		b.WriteString("Temporary facilitator synthesis")
	} else {
		b.WriteString("Peer contribution")
	}
	b.WriteString(fmt.Sprintf(" — slot %s agent %s\n", output.SlotID, output.AgentID))
	b.WriteString("phase: " + output.Phase + "\n")
	b.WriteString("artifact: " + output.Artifact + "\n\n")
	b.WriteString(redactLedgerText(strings.TrimSpace(output.Text)))
	b.WriteString("\n")
	return appendRunArtifactAt(plane.ledger.directory, plane.name, []byte(b.String()), peerPlaneMaxBytes)
}

func (e *TmuxFormationExecutor) peerTurnExtraLines(planeRel string, peers []tmuxSlotBinding, self tmuxSlotBinding, facilitator bool) []string {
	phase := "peer-turn"
	if facilitator {
		phase = "facilitator"
	}
	lines := []string{
		"peer phase: " + phase,
		"shared peer plane: " + planeRel,
		"Read the shared peer plane before deciding your next move.",
		"Use it like an append-only team chat/blackboard: add useful evidence, critique, proposal, question, or synthesis.",
		"You are not in a hierarchy. Coordinate with peers, but keep moving when the next useful action is obvious.",
		fmt.Sprintf("your peer identity: slot %s agent=%q session=%q", self.Slot.ID, self.Card.ID, self.SessionName),
		"peer roster:",
	}
	for _, peer := range peers {
		lines = append(lines, fmt.Sprintf("- slot %s label=%q agent=%q harness=%q session=%q", peer.Slot.ID, peer.Slot.Label, peer.Card.ID, peer.Variant.ID, peer.SessionName))
	}
	lines = append(lines, "native tmux examples:")
	for _, peer := range peers {
		lines = append(lines, fmt.Sprintf("tmux -S %s capture-pane -t %s -p -S -120", e.config.Socket, peer.SessionName))
	}
	if facilitator {
		lines = append(lines,
			"temporary facilitator task: read the peer plane, detect obvious stalls/contradictions, and synthesize a final answer grounded in at least two peer contributions.",
			"Do not become a hidden boss; cite what the peers wrote and finish only when the shared plane has enough evidence.",
		)
	} else {
		lines = append(lines,
			"append or state one useful contribution for the shared plane, then optionally inspect a sibling session or respond to another peer if that is the best next move.",
			"if you append via shell, append to "+planeRel+"; Archon will also capture your final turn into the plane.",
		)
	}
	return lines
}

func (e *TmuxFormationExecutor) appendOrchestrationTeamEvent(req FormationExecution, controller tmuxSlotBinding, workers []tmuxSlotBinding) error {
	workerData := make([]map[string]any, 0, len(workers))
	for _, worker := range workers {
		workerData = append(workerData, map[string]any{
			"slotId":      worker.Slot.ID,
			"label":       worker.Slot.Label,
			"agentId":     worker.Card.ID,
			"harness":     worker.Variant.ID,
			"sessionStem": worker.Variant.SessionStem,
			"sessionRef":  "tmux:" + worker.SessionName,
		})
	}
	return e.store.AppendRunEvent(req.RunID, RunEvent{
		Type:    RunEventOrchestrationTeam,
		NodeID:  req.NodeID,
		SlotID:  controller.Slot.ID,
		Attempt: req.Attempt,
		Data: map[string]any{
			"mode":           "agentic-leader",
			"socket":         e.config.Socket,
			"cwd":            e.config.Cwd,
			"controllerSlot": controller.Slot.ID,
			"controller": map[string]any{
				"slotId":      controller.Slot.ID,
				"label":       controller.Slot.Label,
				"agentId":     controller.Card.ID,
				"harness":     controller.Variant.ID,
				"sessionStem": controller.Variant.SessionStem,
				"sessionRef":  "tmux:" + controller.SessionName,
			},
			"workers": workerData,
		},
	})
}

// Worker observation outcomes. ADR-0011 makes the leader agentic — Archon never
// mediates its commands — so the ledger's only independent account of what each
// worker did is Archon's own pane observation. These are the outcomes that
// account can reach; every one but workerOutcomeOutputCaptured is an anomaly a
// reviewer should see without reading the leader's self-report.
const (
	// workerOutcomeMissingSession: the worker's tmux session was not there when
	// Archon looked (at bind time, or gone again by completion).
	workerOutcomeMissingSession = "missing_session"
	// workerOutcomeBindFailed: the worker slot never bound for some other reason
	// (unstaffed, unknown persona, unconfigured harness, dead pane).
	workerOutcomeBindFailed = "bind_failed"
	// workerOutcomeCaptureFailed: the session exists but Archon could not read it.
	workerOutcomeCaptureFailed = "capture_failed"
	// workerOutcomeNeverTouched: the pane was byte-identical to its bind-time
	// baseline at completion, so the leader never used this worker.
	workerOutcomeNeverTouched = "never_touched"
	// workerOutcomeHangTimeout: the pane changed but the harness was still
	// mid-turn when the run finished — the worker hung or ran past the deadline.
	workerOutcomeHangTimeout = "hang_timeout"
	// workerOutcomeOutputCaptured: the pane changed and settled; the recorded
	// excerpt is what the worker produced.
	workerOutcomeOutputCaptured = "output_captured"
)

const (
	workerObservationPhaseBind       = "bind"
	workerObservationPhaseCompletion = "completion"
	// workerObservationExcerptBytes caps the pane excerpt carried by one
	// observation event. Observation is evidence, not formation output, so an
	// oversized worker pane is truncated (and marked) rather than failing the run
	// the way an oversized dispatch capture does.
	workerObservationExcerptBytes = 4096
)

// workerBaseline is Archon's bind-time observation of one bound worker session:
// the pane as it stood before the leader could touch it. Diffing the completion
// capture against it is what separates a worker the leader never used from one
// that actually did work.
type workerBaseline struct {
	binding tmuxSlotBinding
	capture string
}

// captureWorkerBaselines records the pre-leader pane of every bound worker. It
// fails closed: a worker Archon cannot observe now can never be covered by the
// per-worker evidence contract, so the run stops loudly here instead of
// dispatching a leader whose worker would be invisible.
func (e *TmuxFormationExecutor) captureWorkerBaselines(req FormationExecution, controller tmuxSlotBinding, workers []tmuxSlotBinding) ([]workerBaseline, error) {
	baselines := make([]workerBaseline, 0, len(workers))
	for _, worker := range workers {
		captured, err := e.captureWorkerPane(worker.SessionName)
		if err != nil {
			data := e.workerObservationIdentity(worker.Slot, &worker, controller, worker.SessionName)
			data["phase"] = workerObservationPhaseBind
			data["outcome"] = workerCaptureOutcome(err)
			data["anomaly"] = true
			data["code"] = executionFailureEvent(err).Code
			data["message"] = redactLedgerText(err.Error())
			if appendErr := e.appendWorkerObservation(req, worker.Slot.ID, data); appendErr != nil {
				return nil, appendErr
			}
			return nil, withSlot(err, req.NodeID, worker.Slot.ID, "")
		}
		baselines = append(baselines, workerBaseline{binding: worker, capture: captured})
	}
	return baselines, nil
}

// observeWorkerOutcomes appends one capture-derived outcome event per bound
// worker after the leader's turn. It never prompts a worker and never trusts the
// leader's account: the outcome comes from comparing Archon's own completion
// capture with the bind-time baseline. An anomaly is recorded, not raised — the
// leader owns delegation strategy under ADR-0011, so an unused or still-running
// worker is evidence for the reviewer rather than a run failure. Only a ledger
// append failure is returned.
func (e *TmuxFormationExecutor) observeWorkerOutcomes(req FormationExecution, controller tmuxSlotBinding, baselines []workerBaseline) error {
	for _, baseline := range baselines {
		worker := baseline.binding
		data := e.workerObservationIdentity(worker.Slot, &worker, controller, worker.SessionName)
		data["phase"] = workerObservationPhaseCompletion
		data["baselineSha256"] = etag([]byte(baseline.capture))
		data["baselineBytes"] = len(baseline.capture)
		captured, err := e.captureWorkerPane(worker.SessionName)
		if err != nil {
			data["outcome"] = workerCaptureOutcome(err)
			data["anomaly"] = true
			data["code"] = executionFailureEvent(err).Code
			data["message"] = redactLedgerText(err.Error())
			if appendErr := e.appendWorkerObservation(req, worker.Slot.ID, data); appendErr != nil {
				return appendErr
			}
			continue
		}
		changed := captured != baseline.capture
		working := tmuxPaneShowsAgentWorking(captured)
		outcome := workerOutcomeOutputCaptured
		message := ""
		switch {
		case !changed:
			outcome = workerOutcomeNeverTouched
			message = "worker pane is unchanged from its bind-time baseline; the leader never touched this worker"
		case working:
			outcome = workerOutcomeHangTimeout
			message = "worker harness was still processing a turn when the run completed"
		}
		excerpt, truncated := workerObservationExcerpt(baseline.capture, captured, e.workerExcerptCap())
		data["captureSha256"] = etag([]byte(captured))
		data["captureBytes"] = len(captured)
		data["changed"] = changed
		data["agentWorking"] = working
		data["outcome"] = outcome
		data["anomaly"] = outcome != workerOutcomeOutputCaptured
		if message != "" {
			data["message"] = message
		}
		if excerpt != "" {
			data["text"] = excerpt
		}
		if truncated {
			data["truncated"] = true
		}
		if err := e.appendWorkerObservation(req, worker.Slot.ID, data); err != nil {
			return err
		}
	}
	return nil
}

// failWorkerBind records per-worker evidence for a bind-time failure before the
// error propagates, so a worker whose session is missing is a named ledger event
// instead of an unattributed run error. It returns the original (slot-annotated)
// error unless recording the evidence itself failed, which is the worse silence.
func (e *TmuxFormationExecutor) failWorkerBind(req FormationExecution, controller tmuxSlotBinding, worker FormationSlot, owned *ownedSessions, cause error) error {
	sessionName, _ := owned.name(worker.ID)
	data := e.workerObservationIdentity(worker, nil, controller, sessionName)
	data["phase"] = workerObservationPhaseBind
	data["outcome"] = workerBindOutcome(cause)
	data["anomaly"] = true
	data["code"] = executionFailureEvent(cause).Code
	data["message"] = redactLedgerText(cause.Error())
	if err := e.appendWorkerObservation(req, worker.ID, data); err != nil {
		return err
	}
	return withSlot(cause, req.NodeID, worker.ID, "")
}

// workerObservationIdentity builds the identity half of a worker_observation
// event. binding is nil when the worker never bound, so the identity falls back
// to the slot's authored agent/harness and to the owned session name if one was
// created before the failure.
func (e *TmuxFormationExecutor) workerObservationIdentity(slot FormationSlot, binding *tmuxSlotBinding, controller tmuxSlotBinding, sessionName string) map[string]any {
	agentID := slot.AgentID
	harness := slot.Harness
	sessionStem := ""
	if binding != nil {
		if binding.Card != nil {
			agentID = binding.Card.ID
		}
		harness = binding.Variant.ID
		sessionStem = binding.Variant.SessionStem
		if binding.SessionName != "" {
			sessionName = binding.SessionName
		}
	}
	data := map[string]any{
		"mode": "agentic-leader",
		// observer names the provenance this whole event exists for: Archon's own
		// read-only capture, never the leader's self-report.
		"observer":       "archon-capture",
		"slotId":         slot.ID,
		"label":          slot.Label,
		"agentId":        agentID,
		"harness":        harness,
		"controllerSlot": controller.Slot.ID,
	}
	if sessionStem != "" {
		data["sessionStem"] = sessionStem
	}
	if sessionName != "" {
		data["sessionRef"] = "tmux:" + sessionName
	}
	return data
}

func (e *TmuxFormationExecutor) appendWorkerObservation(req FormationExecution, slotID string, data map[string]any) error {
	return e.store.AppendRunEvent(req.RunID, RunEvent{
		Type:    RunEventWorkerObservation,
		NodeID:  req.NodeID,
		SlotID:  slotID,
		Attempt: req.Attempt,
		Data:    data,
	})
}

// captureWorkerPane is Archon's read-only look at a worker session. It runs on
// its own deadline because completion-time observation happens after the
// leader's dispatch has already consumed the execution context.
func (e *TmuxFormationExecutor) captureWorkerPane(sessionName string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(e.config.TimeoutSeconds)*time.Second)
	defer cancel()
	if err := e.validatePinnedTmuxSocket(); err != nil {
		return "", err
	}
	captured, err := e.client.CapturePane(ctx, e.config.Socket, sessionName, e.config.OutputCapBytes+1)
	if err != nil {
		code := "capture_failed"
		if errors.Is(err, errTmuxTargetMissing) {
			code = "missing_session"
		}
		return "", runExecutionError(code, redactLedgerText(err.Error()), "adapter", err)
	}
	return captured, nil
}

func workerCaptureOutcome(err error) string {
	if errors.Is(err, errTmuxTargetMissing) {
		return workerOutcomeMissingSession
	}
	return workerOutcomeCaptureFailed
}

func workerBindOutcome(err error) string {
	if errors.Is(err, errTmuxTargetMissing) {
		return workerOutcomeMissingSession
	}
	return workerOutcomeBindFailed
}

func (e *TmuxFormationExecutor) workerExcerptCap() int {
	capBytes := e.config.OutputCapBytes
	if capBytes <= 0 {
		capBytes = defaultTmuxOutputCapBytes
	}
	return min(capBytes, workerObservationExcerptBytes)
}

// workerObservationExcerpt returns the redacted, capped pane text a reviewer
// needs to see what a worker produced: the content that appeared after the
// bind-time baseline when the pane only grew, otherwise the tail of the whole
// capture, since a pane that scrolled no longer contains its baseline. Captured
// worker text goes through the same ledger redaction as every other capture.
func workerObservationExcerpt(baseline, captured string, capBytes int) (string, bool) {
	delta := captured
	if baseline != "" && strings.HasPrefix(captured, baseline) {
		delta = captured[len(baseline):]
	}
	truncated := false
	if capBytes > 0 && len(delta) > capBytes {
		delta = strings.ToValidUTF8(delta[len(delta)-capBytes:], "")
		truncated = true
	}
	return redactLedgerText(delta), truncated
}

func (e *TmuxFormationExecutor) leaderAgenticExtraLines(controller tmuxSlotBinding, workers []tmuxSlotBinding) []string {
	lines := []string{
		"leader task: act as the formation orchestrator. Do not wait for Archon to pre-dispatch workers; you own team coordination.",
		"Use native tmux/shell tools to steer the worker sessions yourself, and use judgment about delegation, inspection, revisions, checks, and finish timing.",
		"formation team packet:",
		"tmux socket: " + e.config.Socket,
		"workspace cwd: " + e.config.Cwd,
		fmt.Sprintf("leader session: slot %s agent=%q session=%q", controller.Slot.ID, controller.Card.ID, controller.SessionName),
		"worker sessions:",
	}
	for _, worker := range workers {
		lines = append(lines, fmt.Sprintf("- slot %s label=%q agent=%q harness=%q session=%q", worker.Slot.ID, worker.Slot.Label, worker.Card.ID, worker.Variant.ID, worker.SessionName))
	}
	if len(workers) > 0 {
		lines = append(lines, "native tmux examples:")
		for _, worker := range workers {
			lines = append(lines,
				fmt.Sprintf("tmux -S %s capture-pane -t %s -p -S -120", e.config.Socket, worker.SessionName),
				fmt.Sprintf("tmux -S %s send-keys -t %s C-u '<worker prompt>' ENTER", e.config.Socket, worker.SessionName),
			)
		}
	}
	lines = append(lines,
		"evidence expectation: in your final answer, state which worker sessions you prompted or inspected, what each contributed, what changed your decision, and where the final artifact/evidence lives.",
		"finish only after the team has produced enough evidence for the requested output contract.",
	)
	return lines
}

func controllerPlanExtraLines(workers []FormationSlot) []string {
	lines := []string{
		"controller task: write a concise delegation plan for the worker slots. Do not synthesize the final answer yet.",
		"worker roster:",
	}
	for _, worker := range workers {
		lines = append(lines, fmt.Sprintf("- slot %s label=%q agent=%q harness=%q", worker.ID, worker.Label, worker.AgentID, worker.Harness))
	}
	return lines
}

func workerExtraLines(plan tmuxSlotOutput, worker FormationSlot) []string {
	return []string{
		fmt.Sprintf("worker task: execute only the portion of the controller plan relevant to slot %s (%s).", worker.ID, worker.Label),
		"controller plan:",
		plan.Text,
	}
}

func synthesisExtraLines(plan tmuxSlotOutput, workerOutputs []tmuxSlotOutput) []string {
	lines := []string{
		"controller task: synthesize the final formation output from the worker outputs. Produce the defined proposal or artifact answer, not another plan.",
		"controller plan:",
		plan.Text,
		"worker outputs:",
	}
	for _, output := range workerOutputs {
		lines = append(lines, fmt.Sprintf("- slot %s agent=%s artifact=%s\n%s", output.SlotID, output.AgentID, output.Artifact, output.Text))
	}
	return lines
}

func extractCapturedSlotText(captured, prompt, runID string) string {
	text := strings.ReplaceAll(captured, prompt, "")
	if sentinelStart, _ := lastMatchingCompletionSentinelBounds(text, runID); sentinelStart >= 0 {
		beforeLatest := text[:sentinelStart]
		if _, previousEnd := lastMatchingCompletionSentinelBounds(beforeLatest, runID); previousEnd >= 0 {
			text = text[previousEnd:sentinelStart]
		} else {
			text = beforeLatest
		}
	} else if sentinelAt := strings.LastIndex(text, "<<<CHROTE-DONE "); sentinelAt >= 0 {
		text = text[:sentinelAt]
	}
	if markerAt := strings.LastIndex(text, "When complete, emit exactly one sentinel line"); markerAt >= 0 {
		afterMarker := text[markerAt:]
		if end := strings.Index(afterMarker, ">>>"); end >= 0 {
			text = afterMarker[end+len(">>>"):]
		}
	}
	text = textAfterLastTranscriptSeparator(text)
	lines := make([]string, 0, len(strings.Split(text, "\n")))
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "<<<CHROTE-DONE ") || strings.Contains(line, "run-id=<the-run-value-above>") {
			continue
		}
		lines = append(lines, line)
	}
	return redactLedgerText(strings.TrimSpace(strings.Join(lines, "\n")))
}

func textAfterLastTranscriptSeparator(text string) string {
	lines := strings.Split(text, "\n")
	lastSeparator := -1
	for i, line := range lines {
		if strings.Count(line, "─") >= 20 {
			lastSeparator = i
		}
	}
	if lastSeparator == -1 || lastSeparator+1 >= len(lines) {
		return text
	}
	return strings.Join(lines[lastSeparator+1:], "\n")
}

func lastMatchingCompletionSentinelStart(captured, runID string) int {
	start, _ := lastMatchingCompletionSentinelBounds(captured, runID)
	return start
}

func lastMatchingCompletionSentinelBounds(captured, runID string) (int, int) {
	remaining := captured
	offset := 0
	lastStart := -1
	lastEnd := -1
	for {
		startRel := strings.Index(remaining, "<<<CHROTE-DONE ")
		if startRel == -1 {
			return lastStart, lastEnd
		}
		start := offset + startRel
		afterStart := remaining[startRel+len("<<<CHROTE-DONE "):]
		endRel := strings.Index(afterStart, ">>>")
		if endRel == -1 {
			return lastStart, lastEnd
		}
		fields := parseSentinelFields(afterStart[:endRel])
		end := start + len("<<<CHROTE-DONE ") + endRel + len(">>>")
		if fields["run-id"] == runID {
			lastStart = start
			lastEnd = end
		}
		consumed := startRel + len("<<<CHROTE-DONE ") + endRel + len(">>>")
		offset += consumed
		remaining = remaining[consumed:]
	}
}

func (e *TmuxFormationExecutor) validateConfiguredBoundary() error {
	e.socketIdentity = nil
	if strings.TrimSpace(e.config.Socket) == "" {
		return runExecutionError("missing_socket", "tmux executor socket is not configured", "executor", nil)
	}
	socket, err := filepath.Abs(e.config.Socket)
	if err != nil {
		return runExecutionError("invalid_socket", "tmux executor socket is invalid", "executor", err)
	}
	e.config.Socket = socket
	// The session prefix is validated here, before pinning, because a lazy-start
	// keeper's name is derived from it (SessionPrefix + keeper) and ensureServer
	// runs next.
	if strings.TrimSpace(e.config.SessionPrefix) == "" {
		return runExecutionError("missing_session_prefix", "tmux executor session prefix is not configured", "executor", nil)
	}
	if !safeTmuxSessionName(e.config.SessionPrefix + "probe") {
		return runExecutionError("invalid_session_prefix", "tmux executor session prefix is not a safe tmux target prefix", "executor", nil)
	}
	// The executor shares the cockpit tmux socket by owner ruling, and by owner
	// ruling also supports ANY configured socket. ensureServer resolves the
	// configured agent-user (default: the service user), verifies the socket's
	// backing server is owned by that user, and — only in the self case — lazy
	// -starts a server (a keeper session) when none is running so the socket can
	// be pinned and used. An existing server is left completely untouched; a
	// wrong-owner or absent other-user server fails loud rather than silently
	// running agents under the wrong identity. Safety otherwise lives in
	// session-scoping (create + tear down only uniquely-named owned sessions,
	// never touch a foreign session). The socket must still be a real, stable,
	// non-symlink path, which pinTmuxSocketIdentity enforces below once a server
	// (and thus the socket file) exists.
	expected, err := e.resolveExpectedAgentUser()
	if err != nil {
		return err
	}
	if err := e.ensureServer(expected); err != nil {
		return err
	}
	if err := e.pinTmuxSocketIdentity(); err != nil {
		return err
	}
	// Ownership is verified only after the socket identity is pinned, so the stat
	// reads the confirmed real, non-symlink, stable socket path rather than a
	// symlink target. A server owned by anyone other than the configured
	// agent-user is refused loud here (subsumes chrote-fjy).
	if err := e.verifyServerOwner(expected); err != nil {
		return err
	}
	if strings.TrimSpace(e.config.Cwd) == "" {
		return runExecutionError("missing_cwd", "tmux executor cwd is not configured", "executor", nil)
	}
	if len(e.config.Roots) == 0 {
		return runExecutionError("missing_root", "tmux executor root is not configured", "executor", nil)
	}
	cwd, err := filepath.Abs(e.config.Cwd)
	if err != nil {
		return runExecutionError("invalid_cwd", "tmux executor cwd is invalid", "executor", err)
	}
	if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
		return runExecutionError("unavailable_cwd", "tmux executor cwd is unavailable", "executor", err)
	}
	e.config.Cwd = cwd
	roots := make([]string, 0, len(e.config.Roots))
	cwdInsideRoot := false
	for _, root := range e.config.Roots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return runExecutionError("invalid_root", "tmux executor root is invalid", "executor", err)
		}
		if info, err := os.Stat(absRoot); err != nil || !info.IsDir() {
			return runExecutionError("unavailable_root", "tmux executor root is unavailable", "executor", err)
		}
		if pathWithinRoot(cwd, absRoot) {
			cwdInsideRoot = true
		}
		roots = append(roots, absRoot)
	}
	if !cwdInsideRoot {
		return runExecutionError("cwd_outside_root", "tmux executor cwd is outside configured roots", "executor", nil)
	}
	e.config.Roots = roots
	return nil
}

// ensureServer guarantees a live tmux server on the socket before the executor
// pins its identity and dispatches. It probes for a server (read-only); if one is
// already running it leaves it untouched.
//
// Lazy-start is gated on identity: it fires ONLY in the self case, where the
// server the executor would start (as the service user) is owned by exactly the
// expected agent-user. When the configured agent-user is NOT the service user,
// ensureServer refuses to lazy-start — doing so would create a wrong-owner server
// and silently run agents under the service user's identity — and instead fails
// loud, requiring a correctly-owned server to have been provisioned out of band.
//
// Ownership of the resulting server is verified separately (verifyServerOwner)
// after the socket identity is pinned. Lazy-start uses the least possible
// footprint: a single detached "keeper" session (SessionPrefix + keeper) that
// only holds the new server alive; it is NEVER recorded as an owned run session,
// so teardownOwnedSessions never reclaims it. ensureServer only ever ADDS a
// server + keeper — it never issues kill-server, kill-session, attach, rename, or
// resize — so the only-own-sessions safety invariant holds by construction.
func (e *TmuxFormationExecutor) ensureServer(expected expectedAgentUser) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(e.config.TimeoutSeconds)*time.Second)
	defer cancel()
	running, err := e.client.HasServer(ctx, e.config.Socket)
	if err != nil {
		return runExecutionError("tmux_server_probe_failed", redactLedgerText(err.Error()), "adapter", err)
	}
	if running {
		return nil
	}
	if !expected.self {
		return runExecutionError("agent_user_server_absent", fmt.Sprintf("formations tmux server is not running on %s; a server owned by the configured agent-user %q must be provisioned before dispatch (refusing to lazy-start a wrong-owner server)", e.config.Socket, expected.name), "executor", nil)
	}
	keeper := e.config.SessionPrefix + tmuxKeeperSuffix
	if !safeTmuxSessionName(keeper) {
		return runExecutionError("invalid_session_prefix", "tmux executor session prefix cannot form a safe keeper session name", "executor", nil)
	}
	if err := e.client.StartKeeper(ctx, e.config.Socket, keeper); err != nil {
		return runExecutionError("tmux_server_start_failed", redactLedgerText(err.Error()), "adapter", err)
	}
	return nil
}

func (e *TmuxFormationExecutor) pinTmuxSocketIdentity() error {
	info, err := os.Lstat(e.config.Socket)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return runExecutionError("session_target_attachment_audit_unavailable", "disposable tmux socket identity is unavailable", "executor", err)
	}
	resolved, err := filepath.EvalSymlinks(e.config.Socket)
	if err != nil || filepath.Clean(resolved) != filepath.Clean(e.config.Socket) {
		return runExecutionError("session_target_attachment_audit_unavailable", "disposable tmux socket path is not stable", "executor", err)
	}
	e.socketIdentity = info
	return nil
}

func (e *TmuxFormationExecutor) validatePinnedTmuxSocket() error {
	if e.socketIdentity == nil {
		return runExecutionError("session_target_attachment_audit_unavailable", "disposable tmux socket identity is not pinned", "executor", nil)
	}
	info, err := os.Lstat(e.config.Socket)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !os.SameFile(e.socketIdentity, info) {
		return runExecutionError("session_target_attachment_audit_unavailable", "disposable tmux socket identity changed", "executor", err)
	}
	resolved, err := filepath.EvalSymlinks(e.config.Socket)
	if err != nil || filepath.Clean(resolved) != filepath.Clean(e.config.Socket) {
		return runExecutionError("session_target_attachment_audit_unavailable", "disposable tmux socket path changed", "executor", err)
	}
	return nil
}

func (e *TmuxFormationExecutor) countExistingCompletionSentinels(ctx context.Context, sessionName, runID, harnessID string, dispatchStart time.Time) (int, error) {
	if err := e.validatePinnedTmuxSocket(); err != nil {
		return 0, err
	}
	captured, err := e.captureCompletionText(ctx, harnessID, sessionName, runID, dispatchStart)
	if err != nil {
		code := "capture_failed"
		if errors.Is(err, errTmuxTargetMissing) {
			code = "missing_session"
		}
		return 0, runExecutionError(code, redactLedgerText(err.Error()), "adapter", err)
	}
	if len(captured) > e.config.OutputCapBytes {
		return 0, runExecutionError("oversized_output", "tmux captured output exceeds configured cap", "adapter", nil)
	}
	return countCompletionSentinels(captured, runID), nil
}

func (e *TmuxFormationExecutor) waitForCompletion(ctx context.Context, sessionName, runID, prompt string, previousSentinels int, harnessID string, dispatchStart time.Time) (string, error) {
	deadline := time.Now().Add(time.Duration(e.config.TimeoutSeconds) * time.Second)
	for {
		if err := e.validatePinnedTmuxSocket(); err != nil {
			return "", err
		}
		captured, err := e.captureCompletionText(ctx, harnessID, sessionName, runID, dispatchStart)
		if err != nil {
			code := "capture_failed"
			if errors.Is(err, errTmuxTargetMissing) {
				code = "missing_session"
			}
			return "", runExecutionError(code, redactPromptFromLedgerText(err.Error(), prompt), "adapter", err)
		}
		if len(captured) > e.config.OutputCapBytes {
			return "", runExecutionError("oversized_output", "tmux captured output exceeds configured cap", "adapter", nil)
		}
		if hasNewCompletionSentinel(captured, prompt, runID, previousSentinels) {
			return captured, nil
		}
		if time.Now().After(deadline) {
			return "", runExecutionError("completion_sentinel_timeout", "completion sentinel timeout", "adapter", ErrDispatchTimeout)
		}
		select {
		case <-ctx.Done():
			return "", runExecutionError("completion_sentinel_timeout", "completion sentinel timeout", "adapter", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func hasNewCompletionSentinel(captured, prompt, runID string, previousSentinels int) bool {
	if countCompletionSentinels(captured, runID) > previousSentinels {
		return true
	}
	marker := promptTurnMarker(prompt)
	if marker == "" {
		return false
	}
	markerAt := strings.LastIndex(captured, marker)
	if markerAt < 0 {
		return false
	}
	sentinelAt := lastMatchingCompletionSentinelStart(captured, runID)
	return sentinelAt > markerAt
}

func promptTurnMarker(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "turn marker: ") {
			return line
		}
	}
	return ""
}

func (e *TmuxFormationExecutor) appendAdapterSend(runID, nodeID, slotID, dispatchID, sessionName, prompt, phase string) error {
	return e.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventAdapterSend,
		NodeID: nodeID,
		SlotID: slotID,
		Data: map[string]any{
			"adapter":      "tmux",
			"dispatchId":   dispatchID,
			"nodeId":       nodeID,
			"slotId":       slotID,
			"sessionRef":   "tmux:" + sessionName,
			"phase":        phase,
			"socketSha256": etag([]byte(e.config.Socket)),
			"promptSha256": etag([]byte(prompt)),
			"sent":         true,
		},
	})
}

func (e *TmuxFormationExecutor) allowedHarnesses() map[string]bool {
	allowed := make(map[string]bool, len(e.config.Harnesses))
	for _, harness := range e.config.Harnesses {
		if harness != "" {
			allowed[harness] = true
		}
	}
	return allowed
}

func (e *TmuxFormationExecutor) renderPrompt(req FormationExecution, slot FormationSlot, card PersonaCard, variant HarnessVariant) string {
	return e.renderPromptWithContext(req, slot, card, variant, "", nil)
}

func (e *TmuxFormationExecutor) renderPromptWithContext(req FormationExecution, slot FormationSlot, card PersonaCard, variant HarnessVariant, phase string, extraLines []string) string {
	var b strings.Builder
	b.WriteString("run: " + req.RunID + "\n")
	b.WriteString("node: " + req.NodeID + "\n")
	b.WriteString("slot: " + slot.ID + "\n")
	b.WriteString("agent: " + card.ID + "\n")
	b.WriteString("harness: " + variant.ID + "\n")
	b.WriteString("cwd: " + e.config.Cwd + "\n")
	if phase != "" {
		b.WriteString("orchestration phase: " + phase + "\n")
	}
	b.WriteString("brief: " + req.Brief.Goal + "\n")
	for _, input := range req.Inputs {
		if input.Text != "" {
			b.WriteString("input: " + input.Text + "\n")
		}
	}
	for _, line := range extraLines {
		if strings.TrimSpace(line) != "" {
			b.WriteString(line + "\n")
		}
	}
	if len(req.Formation.Outputs) > 0 && e.store != nil {
		artifactDir := filepath.Join(e.store.workspaceRoot(), ".formations", "artifacts", req.RunID)
		b.WriteString("artifact directory for long routed outputs: " + artifactDir + "\n")
		b.WriteString("If you use ref in chrote-outputs, create the file first under that artifact directory or another configured root path. Do not point ref at arbitrary host files or secrets.\n")
	}
	b.WriteString("When complete, emit exactly one sentinel line using the run value above: <<<CHROTE-DONE run-id=<the-run-value-above> status=ok artifact=<path-or-ref>>>\n")
	return b.String()
}

func (e *TmuxFormationExecutor) pathWithinRoots(path string) bool {
	for _, root := range e.config.Roots {
		if pathWithinRoot(path, root) {
			return true
		}
	}
	return false
}

func withSlot(err error, nodeID, slotID, dispatchID string) error {
	var executionErr *RunExecutionError
	if errors.As(err, &executionErr) {
		if executionErr.NodeID == "" {
			executionErr.NodeID = nodeID
		}
		if executionErr.SlotID == "" {
			executionErr.SlotID = slotID
		}
		if executionErr.DispatchID == "" {
			executionErr.DispatchID = dispatchID
		}
	}
	return err
}

func runSlotExecutionError(code, message, boundary string, cause error, nodeID, slotID, dispatchID string) error {
	return &RunExecutionError{
		Code:       code,
		Message:    redactLedgerText(message),
		Boundary:   boundary,
		Cause:      cause,
		NodeID:     nodeID,
		SlotID:     slotID,
		DispatchID: dispatchID,
	}
}

func safeTmuxSessionName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

type realTmuxHarnessClient struct{}

// HasServer reports whether a tmux server is live on the socket. It is a
// read-only probe: it lists sessions and treats any connection failure (no
// server running, absent or stale socket file) as "no server". It never starts,
// attaches to, or mutates anything.
func (realTmuxHarnessClient) HasServer(ctx context.Context, socket string) (bool, error) {
	if _, err := runTmuxCommand(ctx, socket, nil, "list-sessions", "-F", "#{session_name}"); err != nil {
		return false, nil
	}
	return true, nil
}

// StartKeeper lazy-starts a tmux server on the socket by creating a single
// detached keeper session that holds the server alive. Its pane runs an explicit
// long-lived command (tmuxKeeperHoldCommand) through a real shell — the process
// env forces SHELL=/bin/bash so tmux's default-shell is a real interactive shell
// rather than the service user's /usr/sbin/nologin, which would otherwise make
// the keeper pane exit immediately and take the just-started server with it.
// It uses new-session -d WITHOUT -A, so a name collision fails closed instead of
// attaching to or reusing a pre-existing session; the executor only ever passes
// its own reserved keeper name here. This is the only server-starting operation;
// it never issues kill-server, kill-session, attach, rename, or resize.
func (realTmuxHarnessClient) StartKeeper(ctx context.Context, socket, keeper string) error {
	cmd := exec.CommandContext(ctx, core.TmuxBin(), "-S", socket, "new-session", "-d", "-s", keeper, tmuxKeeperHoldCommand)
	cmd.Env = append(os.Environ(), "SHELL=/bin/bash")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return errors.New(redactLedgerText(message))
	}
	return nil
}

func (realTmuxHarnessClient) ListSessions(ctx context.Context, socket string) ([]string, error) {
	output, err := runTmuxCommand(ctx, socket, nil, "list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil, err
	}
	var sessions []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			sessions = append(sessions, name)
		}
	}
	return sessions, nil
}

// CreateSession spawns a detached, non-persistent session running launch in cwd.
// It uses new-session -d WITHOUT -A, so tmux fails closed on a name collision
// (it never attaches to or reuses a pre-existing session); the executor only
// ever passes a collision-checked, uniquely-owned name here.
func (realTmuxHarnessClient) CreateSession(ctx context.Context, socket, name, cwd, launch string) error {
	args := []string{"new-session", "-d", "-s", name}
	if strings.TrimSpace(cwd) != "" {
		args = append(args, "-c", cwd)
	}
	if strings.TrimSpace(launch) != "" {
		args = append(args, launch)
	}
	_, err := runTmuxCommand(ctx, socket, nil, args...)
	return err
}

// KillSession kills exactly one named session. It is only ever called by the
// executor with a name it created; it never runs kill-server.
func (realTmuxHarnessClient) KillSession(ctx context.Context, socket, name string) error {
	_, err := runTmuxCommand(ctx, socket, nil, "kill-session", "-t", name)
	return err
}

func (realTmuxHarnessClient) DescribeActivePane(ctx context.Context, socket, target string) (tmuxPaneState, error) {
	output, err := runTmuxCommand(ctx, socket, nil, "display-message", "-p", "-t", target, "#{pane_dead}	#{pane_current_path}")
	if err != nil {
		if strings.Contains(err.Error(), "can't find") || strings.Contains(err.Error(), "not found") {
			return tmuxPaneState{}, fmt.Errorf("%w: %s", errTmuxTargetMissing, err.Error())
		}
		return tmuxPaneState{}, err
	}
	parts := strings.SplitN(strings.TrimSpace(output), "\t", 2)
	state := tmuxPaneState{}
	if len(parts) > 0 {
		state.Dead = parts[0] == "1"
	}
	if len(parts) == 2 {
		state.CurrentPath = parts[1]
	}
	return state, nil
}

func (realTmuxHarnessClient) SendPrompt(ctx context.Context, socket, target, dispatchID, prompt string) error {
	bufferName := safeTmuxBufferName(dispatchID)
	if _, err := runTmuxCommand(ctx, socket, strings.NewReader(prompt), "load-buffer", "-b", bufferName, "-"); err != nil {
		return err
	}
	defer func() {
		if _, err := runTmuxCommand(context.Background(), socket, nil, "delete-buffer", "-b", bufferName); err != nil {
			return
		}
	}()
	if _, err := runTmuxCommand(ctx, socket, nil, "send-keys", "-t", target, "C-u"); err != nil {
		return err
	}
	if _, err := runTmuxCommand(ctx, socket, nil, "paste-buffer", "-t", target, "-b", bufferName); err != nil {
		return err
	}
	// Let the (possibly large, multi-line) bracketed paste finish rendering before
	// the first Enter — a too-early Enter is swallowed by the settling input.
	tmuxSleep(tmuxPasteSettleDelay)
	// Submit with the interactive "send to session" pattern: press Enter, then keep
	// pressing a few more, until the pane shows the agent is actually working. A
	// single Enter after a paste is unreliable; extra Enters on an already-working
	// (or empty) input are harmless no-ops in the agent TUI.
	for attempt := 0; attempt < tmuxSubmitMaxEnters; attempt++ {
		if _, err := runTmuxCommand(ctx, socket, nil, "send-keys", "-t", target, "Enter"); err != nil {
			return err
		}
		tmuxSleep(tmuxSubmitDelay)
		captured, err := runTmuxCommand(ctx, socket, nil, "capture-pane", "-p", "-J", "-t", target, "-S", "-6")
		if err != nil {
			return err
		}
		if tmuxPaneShowsAgentWorking(captured) {
			return nil
		}
	}
	return nil
}

// tmuxPaneShowsAgentWorking reports whether the captured pane shows the agent
// actively processing a turn. The coding-agent TUIs surface an "esc to interrupt"
// affordance on their status line only while a turn is in flight, which is a
// reliable positive signal that the pasted prompt was submitted (as opposed to
// still sitting unsent in the input box).
func tmuxPaneShowsAgentWorking(captured string) bool {
	return strings.Contains(captured, "esc to interrupt")
}

func (realTmuxHarnessClient) CapturePane(ctx context.Context, socket, target string, _ int) (string, error) {
	// -J joins tmux's soft-wrapped lines so long single-line chrote-outputs JSON
	// payloads survive capture intact at normal terminal widths.
	output, err := runTmuxCommand(ctx, socket, nil, "capture-pane", "-p", "-J", "-t", target, "-S", "-2000")
	if err != nil {
		if strings.Contains(err.Error(), "can't find") || strings.Contains(err.Error(), "not found") {
			return "", fmt.Errorf("%w: %s", errTmuxTargetMissing, err.Error())
		}
		return "", err
	}
	return output, nil
}

func runTmuxCommand(ctx context.Context, socket string, stdin *strings.Reader, args ...string) (string, error) {
	allArgs := append([]string{"-S", socket}, args...)
	cmd := exec.CommandContext(ctx, core.TmuxBin(), allArgs...)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", errors.New(redactLedgerText(message))
	}
	return string(output), nil
}

func safeTmuxBufferName(dispatchID string) string {
	var b strings.Builder
	b.WriteString("chrote-")
	for _, r := range dispatchID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == len("chrote-") {
		return "chrote-dispatch"
	}
	return b.String()
}
