package formations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTmuxOutputCapBytes = 8192
	defaultTmuxTimeoutSeconds = 30
)

var (
	errTmuxTargetMissing = errors.New("tmux target missing")
	tmuxSubmitDelay      = 500 * time.Millisecond
	tmuxSleep            = time.Sleep
)

type TmuxExecutorConfig struct {
	Harnesses      []string
	Socket         string
	Cwd            string
	Roots          []string
	SessionPrefix  string
	OutputCapBytes int
	TimeoutSeconds int
	ProdSmoke      bool
}

type TmuxFormationExecutor struct {
	store    *Store
	personas *PersonaStore
	config   TmuxExecutorConfig
	client   tmuxHarnessClient
}

type tmuxHarnessClient interface {
	ListSessions(ctx context.Context, socket string) ([]string, error)
	DescribeActivePane(ctx context.Context, socket, target string) (tmuxPaneState, error)
	SendPrompt(ctx context.Context, socket, target, dispatchID, prompt string) error
	CapturePane(ctx context.Context, socket, target string, maxBytes int) (string, error)
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
		SessionPrefix:  strings.TrimSpace(os.Getenv("CHROTE_FORMATIONS_TMUX_SESSION_PREFIX")),
		OutputCapBytes: capBytes,
		TimeoutSeconds: timeoutSeconds,
		ProdSmoke:      tmuxProdSmokeAllowed(os.Getenv("CHROTE_FORMATIONS_TMUX_PROD_SMOKE")),
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

func (o tmuxSlotOutput) summary() string {
	return fmt.Sprintf("tmux harness completed for agent %s harness %s sessionRef %s artifact %s", o.AgentID, o.Harness, o.SessionRef, o.Artifact)
}

func (e *TmuxFormationExecutor) ExecuteFormation(req FormationExecution) (FormationExecutionResult, error) {
	if e == nil || e.store == nil {
		return FormationExecutionResult{}, runExecutionError("missing_executor", "tmux executor store is not configured", "executor", ErrRunExecutorUnavailable)
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
	outputs := make([]string, 0, len(req.Formation.Slots))
	for _, slot := range req.Formation.Slots {
		output, err := e.executeSlot(req, slot, allowed, dispatcher, "", outputContractExtraLines(req.Formation))
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

func (e *TmuxFormationExecutor) executeOrchestratedFormation(req FormationExecution) (FormationExecutionResult, error) {
	controller, workers, err := splitOrchestratedSlots(req.Formation)
	if err != nil {
		return FormationExecutionResult{}, err
	}
	allowed := e.allowedHarnesses()
	dispatcher := NewSlotDispatcher(e.store, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(e.config.TimeoutSeconds)*time.Second)
	defer cancel()

	controllerBinding, err := e.resolveSlotBinding(ctx, req, controller, allowed)
	if err != nil {
		return FormationExecutionResult{}, withSlot(err, req.NodeID, controller.ID, "")
	}
	workerBindings := make([]tmuxSlotBinding, 0, len(workers))
	for _, worker := range workers {
		binding, err := e.resolveSlotBinding(ctx, req, worker, allowed)
		if err != nil {
			return FormationExecutionResult{}, withSlot(err, req.NodeID, worker.ID, "")
		}
		workerBindings = append(workerBindings, binding)
	}
	if err := e.appendOrchestrationTeamEvent(req, controllerBinding, workerBindings); err != nil {
		return FormationExecutionResult{}, err
	}

	leaderExtra := append(e.leaderAgenticExtraLines(controllerBinding, workerBindings), outputContractExtraLines(req.Formation)...)
	leader, err := e.executeSlot(req, controller, allowed, dispatcher, "leader-agentic", leaderExtra)
	if err != nil {
		return FormationExecutionResult{}, err
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(e.config.TimeoutSeconds)*time.Second)
	defer cancel()

	bindings := make([]tmuxSlotBinding, 0, len(peers))
	for _, peer := range peers {
		binding, err := e.resolveSlotBinding(ctx, req, peer, allowed)
		if err != nil {
			return FormationExecutionResult{}, withSlot(err, req.NodeID, peer.ID, "")
		}
		bindings = append(bindings, binding)
	}
	planeRel, planeAbs, err := e.seedPeerPlane(req, bindings)
	if err != nil {
		return FormationExecutionResult{}, err
	}
	if err := e.appendPeerPlaneEvent(req, planeRel, bindings); err != nil {
		return FormationExecutionResult{}, err
	}

	for i, peer := range peers {
		output, err := e.executeSlot(req, peer, allowed, dispatcher, "peer-turn", e.peerTurnExtraLines(planeRel, bindings, bindings[i], false))
		if err != nil {
			return FormationExecutionResult{}, err
		}
		if err := e.appendPeerPlaneOutput(planeAbs, output); err != nil {
			return FormationExecutionResult{}, err
		}
	}

	facilitator := peers[0]
	facilitatorBinding := bindings[0]
	facilitatorExtra := append(e.peerTurnExtraLines(planeRel, bindings, facilitatorBinding, true), outputContractExtraLines(req.Formation)...)
	final, err := e.executeSlot(req, facilitator, allowed, dispatcher, "peer-facilitator", facilitatorExtra)
	if err != nil {
		return FormationExecutionResult{}, err
	}
	if err := e.appendPeerPlaneOutput(planeAbs, final); err != nil {
		return FormationExecutionResult{}, err
	}
	text := final.Text
	if strings.TrimSpace(text) == "" {
		text = final.summary()
	}
	return e.formationResultFromText(req, final.Artifact, text)
}

func (e *TmuxFormationExecutor) formationResultFromText(req FormationExecution, reportRef, text string) (FormationExecutionResult, error) {
	knownPorts := map[string]bool{}
	for _, output := range req.Formation.Outputs {
		knownPorts[output.ID] = true
	}
	cleanText, namedOutputs, err := parseChroteOutputs(text)
	if err != nil {
		return FormationExecutionResult{}, runExecutionError("invalid_output_payloads", err.Error(), "executor", err)
	}
	// Real coding-agent TUIs (Claude Code, Codex) emit the chrote-outputs payload
	// as bare JSON without a literal ```chrote-outputs fence — the markdown fence
	// is rendered away or omitted — so the fence-only parse recovers nothing (or
	// the echoed placeholder example). When no declared port was recovered, fall
	// back to the declared-port-keyed JSON the agent actually emitted. Output port
	// ids are high-entropy, so this cannot collide with echoed prose, and the
	// missing/unknown checks below keep the routing contract fail-loud (home-3b7m).
	if len(knownPorts) > 0 && !outputsContainKnownPort(namedOutputs, knownPorts) {
		if recoveredText, recovered, ok := extractDeclaredPortOutputs(text, knownPorts); ok {
			cleanText, namedOutputs = recoveredText, recovered
		}
	}
	for _, output := range req.Formation.Outputs {
		if _, ok := namedOutputs[output.ID]; !ok {
			return FormationExecutionResult{}, runExecutionError("missing_output_payload", fmt.Sprintf("formation %q did not emit required output port %q", req.NodeID, output.ID), "executor", nil)
		}
	}
	for portID := range namedOutputs {
		if !knownPorts[portID] {
			return FormationExecutionResult{}, runExecutionError("invalid_output_payloads", fmt.Sprintf("formation %q emitted unknown output port %q", req.NodeID, portID), "executor", nil)
		}
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

func outputsContainKnownPort(outputs map[string]FormationOutputPayload, knownPorts map[string]bool) bool {
	for portID := range outputs {
		if knownPorts[portID] {
			return true
		}
	}
	return false
}

// extractDeclaredPortOutputs recovers the routing payload a real coding-agent
// TUI emits as bare JSON when the literal ```chrote-outputs fence does not
// survive capture (rendered away/omitted, with the echoed prompt holding the
// contract's placeholder example). It scans for the last single-line JSON
// object whose keys are all declared (high-entropy) output port ids, so it
// cannot collide with echoed prose or the "port_id" placeholder. The brief
// instructs agents to keep the chrote-outputs JSON on a single line.
func extractDeclaredPortOutputs(text string, knownPorts map[string]bool) (string, map[string]FormationOutputPayload, bool) {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		open := strings.Index(lines[i], "{")
		closeIdx := strings.LastIndex(lines[i], "}")
		if open < 0 || closeIdx <= open {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(lines[i][open:closeIdx+1]), &raw); err != nil || len(raw) == 0 {
			continue
		}
		allKnown := true
		for portID := range raw {
			if !knownPorts[portID] {
				allKnown = false
				break
			}
		}
		if !allKnown {
			continue
		}
		outputs := outputPayloadsFromAny(raw)
		if len(outputs) == 0 {
			continue
		}
		remaining := append(append([]string{}, lines[:i]...), lines[i+1:]...)
		return strings.TrimSpace(strings.Join(remaining, "\n")), outputs, true
	}
	return "", nil, false
}

func outputContractExtraLines(formation FormationNode) []string {
	if len(formation.Outputs) == 0 {
		return nil
	}
	lines := []string{
		"formation output contract:",
		"Every formation output must be emitted through exactly one fenced JSON block before the sentinel:",
		"```chrote-outputs",
		"{\"port_id\":{\"text\":\"payload for that output\"}}",
		"```",
		"Use all and only these output port ids:",
	}
	for _, output := range formation.Outputs {
		lines = append(lines, fmt.Sprintf("- %s label=%q", output.ID, output.Label))
	}
	lines = append(lines, "Do not rely on free-form answer text for routing; it is display-only. Missing or unknown output ids block the run.")
	return lines
}

func parseChroteOutputs(text string) (string, map[string]FormationOutputPayload, error) {
	start := strings.LastIndex(text, "```chrote-outputs")
	if start == -1 {
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
	var raw map[string]any
	if err := json.Unmarshal([]byte(jsonText), &raw); err != nil {
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

func (e *TmuxFormationExecutor) executeSlot(req FormationExecution, slot FormationSlot, allowed map[string]bool, dispatcher *SlotDispatcher, phase string, extraLines []string) (tmuxSlotOutput, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(e.config.TimeoutSeconds)*time.Second)
	defer cancel()
	binding, err := e.resolveSlotBinding(ctx, req, slot, allowed)
	if err != nil {
		return tmuxSlotOutput{}, withSlot(err, req.NodeID, slot.ID, "")
	}
	card := binding.Card
	variant := binding.Variant
	sessionName := binding.SessionName

	turnMarker := "turn marker: " + newPrefixedID("turn")
	promptExtra := append(append([]string{}, extraLines...), turnMarker)
	prompt := e.renderPromptWithContext(req, slot, *card, variant, phase, promptExtra)
	existingSentinels, err := e.countExistingCompletionSentinels(ctx, sessionName, req.RunID)
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
	if err := e.client.SendPrompt(ctx, e.config.Socket, sessionName, lease.DispatchID, prompt); err != nil {
		return tmuxSlotOutput{}, runSlotExecutionError("dispatch_failed", redactPromptFromLedgerText(err.Error(), prompt), "adapter", err, req.NodeID, slot.ID, lease.DispatchID)
	}
	if err := e.appendAdapterSend(req.RunID, req.NodeID, slot.ID, lease.DispatchID, sessionName, prompt, phase); err != nil {
		return tmuxSlotOutput{}, err
	}

	captured, err := e.waitForCompletion(ctx, sessionName, req.RunID, prompt, existingSentinels)
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

func (e *TmuxFormationExecutor) resolveSlotBinding(ctx context.Context, req FormationExecution, slot FormationSlot, allowed map[string]bool) (tmuxSlotBinding, error) {
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
	sessionName := e.config.SessionPrefix + variant.SessionStem
	if !safeTmuxSessionName(sessionName) {
		return tmuxSlotBinding{}, runExecutionError("invalid_session", fmt.Sprintf("resolved tmux session %q is not a safe isolated target", sessionName), "executor", nil)
	}
	if err := e.ensureSessionReady(ctx, sessionName); err != nil {
		return tmuxSlotBinding{}, err
	}
	return tmuxSlotBinding{
		Slot:        slot,
		Card:        card,
		Variant:     variant,
		SessionName: sessionName,
	}, nil
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

func (e *TmuxFormationExecutor) seedPeerPlane(req FormationExecution, peers []tmuxSlotBinding) (string, string, error) {
	ledgerPath, err := e.store.findRunLedger(req.RunID)
	if err != nil {
		return "", "", err
	}
	planeAbs := strings.TrimSuffix(ledgerPath, ".ndjson") + ".peer.md"
	planeRel, err := filepath.Rel(e.store.Workspace, planeAbs)
	if err != nil {
		return "", "", err
	}
	planeRel = filepath.ToSlash(planeRel)
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
	if err := writeAtomic(planeAbs, []byte(b.String())); err != nil {
		return "", "", err
	}
	return planeRel, planeAbs, nil
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

func (e *TmuxFormationExecutor) appendPeerPlaneOutput(planeAbs string, output tmuxSlotOutput) error {
	raw, err := os.ReadFile(planeAbs)
	if err != nil {
		return err
	}
	var b strings.Builder
	b.Write(raw)
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
	return writeAtomic(planeAbs, []byte(b.String()))
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
	if strings.TrimSpace(e.config.Socket) == "" {
		return runExecutionError("missing_socket", "tmux executor socket is not configured", "executor", nil)
	}
	socket, err := filepath.Abs(e.config.Socket)
	if err != nil {
		return runExecutionError("invalid_socket", "tmux executor socket is invalid", "executor", err)
	}
	e.config.Socket = socket
	if strings.TrimSpace(e.config.SessionPrefix) == "" {
		return runExecutionError("missing_session_prefix", "tmux executor session prefix is not configured", "executor", nil)
	}
	if !safeTmuxSessionName(e.config.SessionPrefix + "probe") {
		return runExecutionError("invalid_session_prefix", "tmux executor session prefix is not a safe tmux target prefix", "executor", nil)
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
	if !e.config.ProdSmoke {
		if isDefaultTmuxSocket(socket) || !pathWithinRoot(socket, os.TempDir()) {
			return runExecutionError("unsafe_socket", "tmux executor socket must be an isolated temp socket unless prod-smoke is explicitly enabled", "executor", nil)
		}
		for _, root := range append([]string{cwd}, roots...) {
			if !pathWithinRoot(root, os.TempDir()) {
				return runExecutionError("unsafe_root", "tmux executor roots must be isolated temp paths unless prod-smoke is explicitly enabled", "executor", nil)
			}
		}
	}
	return nil
}

func (e *TmuxFormationExecutor) ensureSessionReady(ctx context.Context, sessionName string) error {
	sessions, err := e.client.ListSessions(ctx, e.config.Socket)
	if err != nil {
		return runExecutionError("tmux_unavailable", redactLedgerText(err.Error()), "adapter", err)
	}
	matches := 0
	for _, name := range sessions {
		if name == sessionName {
			matches++
		}
	}
	switch matches {
	case 0:
		return runExecutionError("missing_session", fmt.Sprintf("tmux session %q was not found on isolated socket", sessionName), "adapter", nil)
	case 1:
	default:
		return runExecutionError("ambiguous_session", fmt.Sprintf("tmux session %q matched %d sessions on isolated socket", sessionName, matches), "adapter", nil)
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

func (e *TmuxFormationExecutor) countExistingCompletionSentinels(ctx context.Context, sessionName, runID string) (int, error) {
	captured, err := e.client.CapturePane(ctx, e.config.Socket, sessionName, e.config.OutputCapBytes+1)
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

func (e *TmuxFormationExecutor) waitForCompletion(ctx context.Context, sessionName, runID, prompt string, previousSentinels int) (string, error) {
	deadline := time.Now().Add(time.Duration(e.config.TimeoutSeconds) * time.Second)
	for {
		captured, err := e.client.CapturePane(ctx, e.config.Socket, sessionName, e.config.OutputCapBytes+1)
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

func tmuxProdSmokeAllowed(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "allow", "allow-live", "prod-smoke":
		return true
	default:
		return false
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

func isDefaultTmuxSocket(socket string) bool {
	candidates := []string{}
	if tmpdir := strings.TrimSpace(os.Getenv("TMUX_TMPDIR")); tmpdir != "" {
		candidates = append(candidates, filepath.Join(tmpdir, "default"))
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); xdg != "" {
		candidates = append(candidates, filepath.Join(xdg, "tmux", "default"))
	}
	candidates = append(candidates, filepath.Join(fmt.Sprintf("/tmp/tmux-%d", os.Getuid()), "default"))
	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err == nil && abs == socket {
			return true
		}
	}
	return false
}

type realTmuxHarnessClient struct{}

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
	tmuxSleep(tmuxSubmitDelay)
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := runTmuxCommand(ctx, socket, nil, "send-keys", "-t", target, "ENTER"); err != nil {
			return err
		}
		tmuxSleep(tmuxSubmitDelay)
		if _, err := runTmuxCommand(ctx, socket, nil, "send-keys", "-t", target, "C-m"); err != nil {
			return err
		}
		tmuxSleep(tmuxSubmitDelay)
		captured, err := runTmuxCommand(ctx, socket, nil, "capture-pane", "-p", "-J", "-t", target, "-S", "-40")
		if err != nil {
			return err
		}
		if !tmuxPaneLooksLikePendingPastedInput(captured) {
			return nil
		}
	}
	return nil
}

func tmuxPaneLooksLikePendingPastedInput(captured string) bool {
	lines := strings.Split(captured, "\n")
	lastPasted := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "› [Pasted Content") || strings.HasPrefix(trimmed, "> [Pasted Content") {
			lastPasted = i
		}
	}
	if lastPasted == -1 {
		return false
	}
	tail := strings.Join(lines[lastPasted:], "\n")
	if strings.Contains(tail, "Working (") || strings.Contains(tail, "─ Worked") || strings.Contains(tail, "<<<CHROTE-DONE run-id=run_") {
		return false
	}
	return true
}

func (realTmuxHarnessClient) CapturePane(ctx context.Context, socket, target string, _ int) (string, error) {
	// -J joins tmux's soft-wrapped lines so a long single-line chrote-outputs
	// JSON payload survives capture intact at any pane width (home-o25k).
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
	cmd := exec.CommandContext(ctx, "tmux", allArgs...)
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
