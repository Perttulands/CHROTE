package formations

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	errRunStopped                       = errors.New("formations run stopped")
	ErrRunExecutorUnavailable           = errors.New("formations run executor unavailable")
	ErrGateEvaluatorUnavailable         = errors.New("formations gate evaluator unavailable")
	ErrVerificationEvaluatorUnavailable = errors.New("formations verification evaluator unavailable")
	ErrRunWallClockExceeded             = errors.New("formations run wall clock exceeded")
)

type FormationExecutor interface {
	ExecuteFormation(FormationExecution) (FormationExecutionResult, error)
}

type unavailableFormationExecutor struct {
	boundary string
}

type GateEvaluator interface {
	EvaluateGate(GateEvaluation) (GateEvaluationResult, error)
}

type VerificationEvaluator interface {
	EvaluateVerification(VerificationEvaluation) (VerificationEvaluationResult, error)
}

type RunEngine struct {
	store                 *Store
	personas              *PersonaStore
	executor              FormationExecutor
	gateEvaluator         GateEvaluator
	verificationEvaluator VerificationEvaluator
}

type FormationRunRequest struct {
	Actor    string
	Personas *PersonaStore
	Limits   RunLimits
}

type FormationExecution struct {
	RunID     string
	NodeID    string
	Title     string
	Formation FormationNode
	Brief     FormationBrief
	Inputs    []RunInputRef
	Attempt   int
}

type FormationExecutionResult struct {
	Status    string
	ReportRef string
	Text      string
	Outputs   map[string]FormationOutputPayload
}

type FormationOutputPayload struct {
	Ref         string `json:"ref,omitempty"`
	Text        string `json:"text,omitempty"`
	ReportRef   string `json:"reportRef,omitempty"`
	ArtifactRef string `json:"artifactRef,omitempty"`
}

type GateEvaluation struct {
	RunID     string
	GateID    string
	Title     string
	Kinds     []string
	Criterion string
	Input     RunInputRef
}

type GateEvaluationResult struct {
	Verdict  string
	Reason   string
	PerKind  map[string]string
	Outputs  map[string]FormationOutputPayload
	Evidence map[string]any
}

type HumanGateVerdictRequest struct {
	GateID  string
	Verdict string
	Reason  string
	Actor   string
}

type VerificationEvaluation struct {
	RunID          string
	NodeID         string
	VerificationID string
	Kinds          []string
	Criterion      string
	OnFail         string
	OutputText     string
	Attempt        int
}

type VerificationEvaluationResult struct {
	Verdict  string
	Feedback string
	PerKind  map[string]string
}

type RunInputRef struct {
	EdgeID      string `json:"edgeId,omitempty"`
	FromNodeID  string `json:"fromNodeId,omitempty"`
	FromPortID  string `json:"fromPortId,omitempty"`
	ToPortID    string `json:"toPortId,omitempty"`
	OutputSeq   int    `json:"outputSeq,omitempty"`
	Ref         string `json:"ref,omitempty"`
	Text        string `json:"text,omitempty"`
	ReportRef   string `json:"reportRef,omitempty"`
	ArtifactRef string `json:"artifactRef,omitempty"`
}

func NewRunEngine(store *Store, personas *PersonaStore, executor FormationExecutor) *RunEngine {
	return &RunEngine{
		store:    store,
		personas: personas,
		executor: executor,
	}
}

func NewUnavailableFormationExecutor(boundary string) FormationExecutor {
	return unavailableFormationExecutor{boundary: strings.TrimSpace(boundary)}
}

func (e unavailableFormationExecutor) ExecuteFormation(FormationExecution) (FormationExecutionResult, error) {
	if e.boundary == "" {
		return FormationExecutionResult{}, ErrRunExecutorUnavailable
	}
	return FormationExecutionResult{}, fmt.Errorf("%w: %s executor is not configured", ErrRunExecutorUnavailable, e.boundary)
}

func (e *RunEngine) SetGateEvaluator(evaluator GateEvaluator) {
	e.gateEvaluator = evaluator
}

func (e *RunEngine) SetVerificationEvaluator(evaluator VerificationEvaluator) {
	e.verificationEvaluator = evaluator
}

func (e *RunEngine) RunMission(slug string, req RunStartRequest) (*RunStatusProjection, error) {
	if e == nil || e.store == nil {
		return nil, fmt.Errorf("%w: run engine store required", ErrNotFound)
	}
	board, err := e.store.ReadBoard(slug)
	if err != nil {
		return nil, err
	}
	mission, ok := findMission(board, req.MissionID)
	if !ok {
		return nil, fmt.Errorf("%w: mission %q", ErrNotFound, req.MissionID)
	}
	if len(outgoingConnections(board.Connections, mission.ID)) == 0 {
		return nil, fmt.Errorf("%w: wire the mission to a step", ErrConflict)
	}
	if req.Personas == nil {
		req.Personas = e.personas
	}
	started, err := e.store.StartRun(slug, req)
	if err != nil {
		return nil, err
	}
	runBoard, err := e.readRunBoard(started.SnapshotPath)
	if err != nil {
		return nil, err
	}
	mission, ok = findMission(runBoard, req.MissionID)
	if !ok {
		return nil, fmt.Errorf("%w: mission %q", ErrNotFound, req.MissionID)
	}
	if err := e.executeSnapshot(started.RunID, runBoard, mission, req.Limits); err != nil {
		return nil, err
	}
	return e.store.ProjectRun(started.RunID)
}

func (e *RunEngine) RunFormation(slug, formationID string, req FormationRunRequest) (*RunStatusProjection, error) {
	if e == nil || e.store == nil {
		return nil, fmt.Errorf("%w: run engine store required", ErrNotFound)
	}
	board, err := e.store.ReadBoard(slug)
	if err != nil {
		return nil, err
	}
	formation, ok := findFormation(board.Formations, formationID)
	if !ok {
		return nil, fmt.Errorf("%w: formation %q", ErrNotFound, formationID)
	}
	personas := req.Personas
	if personas == nil {
		personas = e.personas
	}
	started, mission, seedInput, err := e.startFormationRun(slug, board, formation, req.Actor, personas, req.Limits)
	if err != nil {
		return nil, err
	}
	if err := e.store.AppendRunEvent(started.RunID, RunEvent{
		Type:    RunEventNodeStarted,
		NodeID:  formation.ID,
		Attempt: 1,
		Data: map[string]any{
			"nodeKind":  "formation",
			"inputRefs": []RunInputRef{seedInput},
			"reason":    "single-formation",
			"brief":     formationBriefEventData(formationBriefValue(formation)),
		},
	}); err != nil {
		return nil, err
	}
	result, err := e.executeFormation(FormationExecution{
		RunID:     started.RunID,
		NodeID:    formation.ID,
		Title:     formation.Title,
		Formation: formation,
		Brief:     formationBriefValue(formation),
		Inputs:    []RunInputRef{seedInput},
		Attempt:   1,
	}, req.Limits)
	if err != nil {
		if blockErr := e.appendExecutionFailureAndBlock(started.RunID, formation.ID, err); blockErr != nil {
			return nil, blockErr
		}
		return e.store.ProjectRun(started.RunID)
	}
	if result.Status == "" {
		result.Status = "done"
	}
	if err := e.ensureFormationOutputPayloads(started.RunID, formation, result); err != nil {
		if errors.Is(err, errRunStopped) {
			return e.store.ProjectRun(started.RunID)
		}
		return nil, err
	}
	if err := e.store.AppendRunEvent(started.RunID, RunEvent{
		Type:   RunEventNodeOutput,
		NodeID: formation.ID,
		Data:   formationOutputEventData(result),
	}); err != nil {
		return nil, err
	}
	if err := e.store.AppendRunEvent(started.RunID, RunEvent{
		Type: RunEventSucceeded,
		Data: map[string]any{
			"summaryRef":   "",
			"outputRefs":   []string{},
			"artifactRefs": []string{},
			"final":        true,
			"mode":         "formation",
			"formationId":  formation.ID,
			"missionId":    mission.ID,
		},
	}); err != nil {
		return nil, err
	}
	return e.store.ProjectRun(started.RunID)
}

func (e *RunEngine) ResumeRun(runID string, req RunResumeRequest) (*RunStatusProjection, error) {
	if e == nil || e.store == nil {
		return nil, fmt.Errorf("%w: run engine store required", ErrNotFound)
	}
	if _, err := e.store.ResumeRun(runID, req); err != nil {
		return nil, err
	}
	events, err := e.store.ReadRunEvents(runID)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, ErrRunLedgerInvalid
	}
	resumeEvent := events[len(events)-1]
	if openDispatches := openDispatchRefsFromEvent(resumeEvent); len(openDispatches) > 0 {
		if err := e.appendOpenDispatchReattachFailure(runID, openDispatches); err != nil {
			return nil, err
		}
		return e.store.ProjectRun(runID)
	}
	started := events[0]
	board, err := e.readRunBoard(stringFromEventData(started, "snapshot"))
	if err != nil {
		return nil, err
	}
	mission, ok := findMission(board, started.MissionID)
	if !ok {
		return nil, fmt.Errorf("%w: mission %q", ErrNotFound, started.MissionID)
	}
	if err := e.resumeSnapshot(runID, board, mission, runLimitsFromEvent(started), events); err != nil {
		return nil, err
	}
	return e.store.ProjectRun(runID)
}

func (e *RunEngine) RecordHumanGateVerdict(runID string, req HumanGateVerdictRequest) (*RunStatusProjection, error) {
	if e == nil || e.store == nil {
		return nil, fmt.Errorf("%w: run engine store required", ErrNotFound)
	}
	events, err := e.store.ReadRunEvents(runID)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, ErrRunLedgerInvalid
	}
	requestEvent, ok := latestHumanRequest(events, req.GateID)
	if !ok {
		return nil, fmt.Errorf("%w: human gate request %q", ErrNotFound, req.GateID)
	}
	verdict, recognized := parseStrictVerdict(req.Verdict)
	if !recognized {
		// Human verdicts arrive pre-constrained to pass/fail from the CLI/API;
		// an unrecognized value here means a caller bypassed that contract.
		// Block loudly rather than silently routing pass.
		if err := e.appendAmbiguousGateVerdictBlock(runID, req.GateID, req.Verdict); err != nil {
			return nil, err
		}
		return e.store.ProjectRun(runID)
	}
	actor := defaultRunActor(req.Actor)
	if err := e.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventHumanVerdictRecorded,
		Actor:  actor,
		GateID: req.GateID,
		NodeID: req.GateID,
		Data: map[string]any{
			"gateId":       req.GateID,
			"nodeId":       req.GateID,
			"verdict":      verdict,
			"reason":       strings.TrimSpace(req.Reason),
			"requestedSeq": requestEvent.Seq,
			"decidedBy":    actor,
		},
	}); err != nil {
		return nil, err
	}
	board, err := e.readRunBoard(stringFromEventData(events[0], "snapshot"))
	if err != nil {
		return nil, err
	}
	gate, ok := findGate(board.Gates, req.GateID)
	if !ok {
		return nil, fmt.Errorf("%w: gate %q", ErrNotFound, req.GateID)
	}
	input := runInputRefFromAny(requestEvent.Data["inputRef"])
	return e.routeGateVerdict(runID, board, gate, input, verdict, "human verdict", runLimitsFromEvent(events[0]))
}

type openDispatchRef struct {
	DispatchID string
	NodeID     string
	SlotID     string
}

func openDispatchRefsFromEvent(event RunEvent) []openDispatchRef {
	if event.Data == nil {
		return nil
	}
	raw, ok := event.Data["openDispatches"].([]any)
	if !ok {
		return nil
	}
	refs := make([]openDispatchRef, 0, len(raw))
	for _, item := range raw {
		fields, ok := item.(map[string]any)
		if !ok {
			continue
		}
		ref := openDispatchRef{
			DispatchID: stringFromAny(fields["dispatchId"]),
			NodeID:     stringFromAny(fields["nodeId"]),
			SlotID:     stringFromAny(fields["slotId"]),
		}
		if ref.DispatchID != "" {
			refs = append(refs, ref)
		}
	}
	return refs
}

func (e *RunEngine) appendOpenDispatchReattachFailure(runID string, refs []openDispatchRef) error {
	openDispatches := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		openDispatches = append(openDispatches, map[string]any{
			"dispatchId": ref.DispatchID,
			"nodeId":     ref.NodeID,
			"slotId":     ref.SlotID,
		})
		if err := e.store.AppendRunEvent(runID, RunEvent{
			Type:   RunEventError,
			NodeID: ref.NodeID,
			SlotID: ref.SlotID,
			Data: map[string]any{
				"code":        "dispatch_reattach_failed",
				"message":     "could not reattach open dispatch without live capture",
				"reason":      "could not reattach open dispatch without live capture",
				"boundary":    "recovery",
				"nodeId":      ref.NodeID,
				"slotId":      ref.SlotID,
				"recoverable": true,
				"dispatchId":  ref.DispatchID,
			},
		}); err != nil {
			return err
		}
	}
	blockedNodeID := ""
	blockedSlotID := ""
	if len(refs) > 0 {
		blockedNodeID = refs[0].NodeID
		blockedSlotID = refs[0].SlotID
	}
	return e.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventBlocked,
		NodeID: blockedNodeID,
		SlotID: blockedSlotID,
		Data: map[string]any{
			"reason":         "dispatch reattach failed",
			"blockedNodeId":  blockedNodeID,
			"resumeAllowed":  true,
			"resumePolicy":   "explicit",
			"openDispatches": openDispatches,
			"nextEpoch":      1,
		},
	})
}

// startFormationRun bootstraps a single-formation run. It models the run as a
// synthetic mission (single_<formationID>) that is not on the board, then routes
// the snapshot + bindings + initial run_started write through the shared
// store.bootstrapRun path so this start cannot drift from store.StartRun
// (ADR-0002). The single-formation marker (mode + formationId) rides along as
// bootstrap ExtraData; everything else is the shared start contract.
func (e *RunEngine) startFormationRun(slug string, board *BoardDocument, formation FormationNode, actor string, personas *PersonaStore, limits RunLimits) (*RunStartResult, MissionNode, RunInputRef, error) {
	bindings, err := resolveRunBindings(board, personas)
	if err != nil {
		return nil, MissionNode{}, RunInputRef{}, err
	}
	goal := ""
	beadID := ""
	if formation.Brief != nil {
		goal = formation.Brief.Goal
		beadID = formation.Brief.BeadID
	}
	mission := MissionNode{
		ID:     "single_" + formation.ID,
		Title:  "Single formation: " + formation.Title,
		Goal:   goal,
		BeadID: beadID,
	}
	started, err := e.store.bootstrapRun(runBootstrap{
		Slug:     slug,
		Board:    board,
		BoardRaw: []byte(board.TOML),
		Mission:  mission,
		Bindings: bindings,
		Actor:    actor,
		Limits:   limits,
		ExtraData: map[string]any{
			"mode":        "formation",
			"formationId": formation.ID,
		},
	})
	if err != nil {
		return nil, MissionNode{}, RunInputRef{}, err
	}
	seedInput := RunInputRef{
		Ref:  "brief://" + formation.ID,
		Text: goal,
	}
	return started, mission, seedInput, nil
}

func (e *RunEngine) resumeSnapshot(runID string, board *BoardDocument, mission MissionNode, limits RunLimits, events []RunEvent) error {
	formationByID := map[string]FormationNode{}
	for _, formation := range board.Formations {
		formationByID[formation.ID] = formation
	}
	gateByID := map[string]GateNode{}
	for _, gate := range board.Gates {
		gateByID[gate.ID] = gate
	}

	ready := map[string]map[string]RunInputRef{}
	queued := map[string]bool{}
	attempts := map[string]int{}
	completed := map[string]bool{}
	lastOutputIdx := map[string]int{}
	var queue []string

	for i, event := range events {
		if event.Type == RunEventNodeStarted && event.Attempt > attempts[event.NodeID] {
			attempts[event.NodeID] = event.Attempt
		}
		if event.Type != RunEventNodeOutput {
			continue
		}
		completed[event.NodeID] = true
		lastOutputIdx[event.NodeID] = i
		if err := e.replayNodeOutputToReady(runID, board, event, ready, queued, &queue); err != nil {
			if errors.Is(err, errRunStopped) {
				return nil
			}
			return err
		}
	}
	if err := e.replayGateVerdictsToReady(runID, board, gateByID, events, limits, ready, queued, &queue, completed, lastOutputIdx); err != nil {
		if errors.Is(err, errRunStopped) {
			return nil
		}
		return err
	}

	dispatches := 0
	ranAny := false
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		queued[nodeID] = false
		if completed[nodeID] {
			continue
		}
		formation, ok := formationByID[nodeID]
		if !ok {
			continue
		}
		inputs := orderedInputs(formation, ready[nodeID])
		if !formationReady(formation, ready[nodeID]) {
			if err := e.appendWaiting(runID, formation, ready[nodeID]); err != nil {
				return err
			}
			continue
		}
		nextAttempt := attempts[nodeID] + 1
		if maxAttempts(limits) > 0 && nextAttempt > maxAttempts(limits) {
			return e.appendErrorAndBlock(runID, "resume_attempts_exhausted", "resume attempts exhausted", "engine", nodeID, "resume attempts exhausted")
		}
		if limits.MaxDispatch > 0 && dispatches >= limits.MaxDispatch {
			return e.appendErrorAndBlock(runID, "max_dispatch_exceeded", "max dispatch exceeded", "engine", nodeID, "max dispatch exceeded")
		}
		attempts[nodeID] = nextAttempt
		if err := e.store.AppendRunEvent(runID, RunEvent{
			Type:    RunEventNodeStarted,
			NodeID:  nodeID,
			Attempt: nextAttempt,
			Data: map[string]any{
				"nodeKind":  "formation",
				"inputRefs": inputs,
				"reason":    "resume",
				"brief":     formationBriefEventData(formationBriefValue(formation)),
			},
		}); err != nil {
			return err
		}
		result, err := e.executeFormation(FormationExecution{
			RunID:     runID,
			NodeID:    nodeID,
			Title:     formation.Title,
			Formation: formation,
			Brief:     formationBriefValue(formation),
			Inputs:    inputs,
			Attempt:   nextAttempt,
		}, limits)
		if err != nil {
			return e.appendExecutionFailureAndBlock(runID, nodeID, err)
		}
		ranAny = true
		dispatches++
		if result.Status == "" {
			result.Status = "done"
		}
		if err := e.ensureFormationOutputPayloads(runID, formation, result); err != nil {
			if errors.Is(err, errRunStopped) {
				return nil
			}
			return err
		}
		verificationAction, err := e.handleVerification(runID, formation, result, nextAttempt, limits, queued, &queue)
		if err != nil {
			return err
		}
		switch verificationAction {
		case verificationRetry:
			continue
		case verificationStopped:
			return nil
		}
		if err := e.store.AppendRunEvent(runID, RunEvent{
			Type:   RunEventNodeOutput,
			NodeID: nodeID,
			Data:   formationOutputEventData(result),
		}); err != nil {
			return err
		}
		completed[nodeID] = true
		if err := e.deliverFormationOutput(runID, board, gateByID, nodeID, result, limits, ready, queued, &queue); err != nil {
			if errors.Is(err, errRunStopped) {
				return nil
			}
			return err
		}
	}

	if starved := starvedFormations(formationByID, ready); len(starved) > 0 {
		return e.appendStarvedBlock(runID, starved)
	}
	if !ranAny {
		return e.appendErrorAndBlock(runID, "resume_no_work", "no resumable work found", "engine", "", "no resumable work found")
	}
	return e.store.AppendRunEvent(runID, RunEvent{
		Type: RunEventSucceeded,
		Data: map[string]any{
			"summaryRef":   "",
			"outputRefs":   []string{},
			"artifactRefs": []string{},
			"final":        true,
			"reason":       "resume",
		},
	})
}

func (e *RunEngine) replayNodeOutputToReady(runID string, board *BoardDocument, event RunEvent, ready map[string]map[string]RunInputRef, queued map[string]bool, queue *[]string) error {
	for _, connection := range outgoingConnections(board.Connections, event.NodeID) {
		_, fromPort := endpointParts(connection.From)
		payload, ok := outputPayloadForPortFromEvent(event, fromPort)
		if !ok {
			if err := e.appendErrorAndBlock(runID, "missing_output_payload", fmt.Sprintf("node %s did not produce output for port %s", event.NodeID, fromPort), "engine", event.NodeID, "missing output payload"); err != nil {
				return err
			}
			return errRunStopped
		}
		toNode, toPort := endpointParts(connection.To)
		if toNode == "" || toPort == "" {
			continue
		}
		formation, ok := findFormation(board.Formations, toNode)
		if !ok {
			continue
		}
		if ready[toNode] == nil {
			ready[toNode] = map[string]RunInputRef{}
		}
		input := runInputRefForConnection(runID, connection, payload)
		ready[toNode][toPort] = input
		if formationReady(formation, ready[toNode]) && !queued[toNode] {
			queued[toNode] = true
			*queue = append(*queue, toNode)
		}
	}
	return nil
}

func (e *RunEngine) replayGateVerdictsToReady(runID string, board *BoardDocument, gates map[string]GateNode, events []RunEvent, limits RunLimits, ready map[string]map[string]RunInputRef, queued map[string]bool, queue *[]string, completed map[string]bool, lastOutputIdx map[string]int) error {
	for i, event := range events {
		if event.Type != RunEventGateVerdict {
			continue
		}
		routePort := stringFromEventData(event, "routePort")
		if routePort == "" || routePort == "none" {
			continue
		}
		gateID := event.GateID
		if gateID == "" {
			gateID = event.NodeID
		}
		input := runInputRefFromAny(event.Data["inputRef"])
		for _, route := range gateVerdictRoutes(board, event, gateID, routePort) {
			// A gate verdict that routes back to a node which already produced
			// output BEFORE this verdict is a pushback (e.g. a human or code gate
			// fail wired back to the work formation). Clear its completed mark so
			// the dispatch loop re-runs it; without this the re-routed node is
			// skipped and the resume reports resume_no_work (home-28ww). Ordering
			// guard: only clear when this verdict is newer than the target's last
			// output, so a pushback already serviced by a later attempt is not
			// re-run on subsequent resumes.
			if toNode, _ := endpointParts(route.To); toNode != "" {
				if idx, ok := lastOutputIdx[toNode]; ok && i > idx {
					delete(completed, toNode)
				}
			}
			nextInput := input
			nextInput.EdgeID = route.ID
			nextInput.FromNodeID = gateID
			nextInput.FromPortID = routePort
			nextInput.Ref = fmt.Sprintf("ledger://%s/%s", runID, route.ID)
			if err := e.deliverConnection(runID, board, gates, route, nextInput, limits, ready, queued, queue); err != nil {
				return err
			}
		}
	}
	return nil
}

func gateVerdictRoutes(board *BoardDocument, event RunEvent, gateID, routePort string) []BoardConnection {
	routed := map[string]bool{}
	for _, id := range stringSliceFromAny(event.Data["routedEdges"]) {
		routed[id] = true
	}
	var routes []BoardConnection
	for _, connection := range outgoingConnectionsFromPort(board.Connections, gateID, routePort) {
		if len(routed) > 0 && !routed[connection.ID] {
			continue
		}
		routes = append(routes, connection)
	}
	return routes
}

func (e *RunEngine) readRunBoard(snapshotPath string) (*BoardDocument, error) {
	raw, err := os.ReadFile(filepath.Join(e.store.Workspace, snapshotPath))
	if err != nil {
		return nil, err
	}
	return parseBoard(raw)
}

func (e *RunEngine) executeSnapshot(runID string, board *BoardDocument, mission MissionNode, limits RunLimits) error {
	formationByID := map[string]FormationNode{}
	for _, formation := range board.Formations {
		formationByID[formation.ID] = formation
	}
	gateByID := map[string]GateNode{}
	for _, gate := range board.Gates {
		gateByID[gate.ID] = gate
	}

	ready := map[string]map[string]RunInputRef{}
	queued := map[string]bool{}
	attempts := map[string]int{}
	dispatches := 0
	var queue []string

	if err := e.store.AppendRunEvent(runID, RunEvent{
		Type:      RunEventNodeStarted,
		NodeID:    mission.ID,
		MissionID: mission.ID,
		Data: map[string]any{
			"nodeKind":  "mission",
			"inputRefs": []RunInputRef{},
			"reason":    "initial",
		},
	}); err != nil {
		return err
	}
	missionOutputs := map[string]FormationOutputPayload{
		"out": {Text: mission.Goal},
	}
	if err := e.store.AppendRunEvent(runID, RunEvent{
		Type:      RunEventNodeOutput,
		NodeID:    mission.ID,
		MissionID: mission.ID,
		Data: formationOutputEventData(FormationExecutionResult{
			Status:  "done",
			Text:    mission.Goal,
			Outputs: missionOutputs,
		}),
	}); err != nil {
		return err
	}
	if err := e.deliverOutputPayloads(runID, board, gateByID, mission.ID, missionOutputs, limits, ready, queued, &queue); err != nil {
		if errors.Is(err, errRunStopped) {
			return nil
		}
		return err
	}

	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		queued[nodeID] = false
		formation, ok := formationByID[nodeID]
		if !ok {
			continue
		}
		inputs := orderedInputs(formation, ready[nodeID])
		if !formationReady(formation, ready[nodeID]) {
			if err := e.appendWaiting(runID, formation, ready[nodeID]); err != nil {
				return err
			}
			continue
		}
		nextAttempt := attempts[nodeID] + 1
		if maxAttempts(limits) > 0 && nextAttempt > maxAttempts(limits) {
			if err := e.appendErrorAndBlock(runID, "revise_loop_exhausted", "revise loop exhausted", "engine", nodeID, "revise loop exhausted"); err != nil {
				return err
			}
			return nil
		}
		if limits.MaxDispatch > 0 && dispatches >= limits.MaxDispatch {
			if err := e.appendErrorAndBlock(runID, "max_dispatch_exceeded", "max dispatch exceeded", "engine", nodeID, "max dispatch exceeded"); err != nil {
				return err
			}
			return nil
		}
		attempts[nodeID] = nextAttempt
		if err := e.store.AppendRunEvent(runID, RunEvent{
			Type:    RunEventNodeStarted,
			NodeID:  nodeID,
			Attempt: nextAttempt,
			Data: map[string]any{
				"nodeKind":  "formation",
				"inputRefs": inputs,
				"reason":    "initial",
				"brief":     formationBriefEventData(formationBriefValue(formation)),
			},
		}); err != nil {
			return err
		}
		result, err := e.executeFormation(FormationExecution{
			RunID:     runID,
			NodeID:    nodeID,
			Title:     formation.Title,
			Formation: formation,
			Brief:     formationBriefValue(formation),
			Inputs:    inputs,
			Attempt:   nextAttempt,
		}, limits)
		if err != nil {
			if blockErr := e.appendExecutionFailureAndBlock(runID, nodeID, err); blockErr != nil {
				return blockErr
			}
			return nil
		}
		dispatches++
		if result.Status == "" {
			result.Status = "done"
		}
		if err := e.ensureFormationOutputPayloads(runID, formation, result); err != nil {
			if errors.Is(err, errRunStopped) {
				return nil
			}
			return err
		}
		verificationAction, err := e.handleVerification(runID, formation, result, nextAttempt, limits, queued, &queue)
		if err != nil {
			return err
		}
		switch verificationAction {
		case verificationRetry:
			continue
		case verificationStopped:
			return nil
		}
		if err := e.store.AppendRunEvent(runID, RunEvent{
			Type:   RunEventNodeOutput,
			NodeID: nodeID,
			Data:   formationOutputEventData(result),
		}); err != nil {
			return err
		}
		if err := e.deliverFormationOutput(runID, board, gateByID, nodeID, result, limits, ready, queued, &queue); err != nil {
			if errors.Is(err, errRunStopped) {
				return nil
			}
			return err
		}
	}

	if starved := starvedFormations(formationByID, ready); len(starved) > 0 {
		return e.appendStarvedBlock(runID, starved)
	}
	return e.store.AppendRunEvent(runID, RunEvent{
		Type: RunEventSucceeded,
		Data: map[string]any{
			"summaryRef":   "",
			"outputRefs":   []string{},
			"artifactRefs": []string{},
			"final":        true,
		},
	})
}

func (e *RunEngine) executeFormation(req FormationExecution, limits RunLimits) (FormationExecutionResult, error) {
	if e.executor == nil {
		return FormationExecutionResult{}, ErrRunExecutorUnavailable
	}
	if limits.WallClockSeconds <= 0 {
		return e.executor.ExecuteFormation(req)
	}
	type executionResult struct {
		result FormationExecutionResult
		err    error
	}
	done := make(chan executionResult, 1)
	go func() {
		result, err := e.executor.ExecuteFormation(req)
		done <- executionResult{result: result, err: err}
	}()
	select {
	case result := <-done:
		return result.result, result.err
	case <-time.After(time.Duration(limits.WallClockSeconds) * time.Second):
		return FormationExecutionResult{}, ErrRunWallClockExceeded
	}
}

type verificationAction string

const (
	verificationContinue verificationAction = "continue"
	verificationRetry    verificationAction = "retry"
	verificationStopped  verificationAction = "stopped"
)

func (e *RunEngine) handleVerification(runID string, formation FormationNode, result FormationExecutionResult, attempt int, limits RunLimits, queued map[string]bool, queue *[]string) (verificationAction, error) {
	if formation.Verification == nil {
		return verificationContinue, nil
	}
	evaluation, err := e.evaluateVerification(VerificationEvaluation{
		RunID:          runID,
		NodeID:         formation.ID,
		VerificationID: formation.Verification.ID,
		Kinds:          formation.Verification.Kinds,
		Criterion:      formation.Verification.Criterion,
		OnFail:         formation.Verification.OnFail,
		OutputText:     result.Text,
		Attempt:        attempt,
	})
	if err != nil {
		if errors.Is(err, errRunStopped) {
			return verificationStopped, nil
		}
		return verificationStopped, err
	}
	verdict, ok := parseStrictVerdict(evaluation.Verdict)
	if !ok {
		if err := e.appendAmbiguousVerificationVerdictBlock(runID, formation.ID, formation.Verification.ID, evaluation.Verdict); err != nil {
			return verificationStopped, err
		}
		return verificationStopped, nil
	}
	if err := e.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventVerificationVerdict,
		NodeID: formation.ID,
		Data: map[string]any{
			"verificationId": formation.Verification.ID,
			"verdict":        verdict,
			"kinds":          formation.Verification.Kinds,
			"criterion":      formation.Verification.Criterion,
			"onFail":         formation.Verification.OnFail,
			"feedback":       evaluation.Feedback,
			"perKind":        evaluation.PerKind,
		},
	}); err != nil {
		return verificationStopped, err
	}
	if verdict == "pass" {
		return verificationContinue, nil
	}
	switch formation.Verification.OnFail {
	case "pushback":
		if attempt >= maxAttempts(limits) {
			if err := e.appendErrorAndBlock(runID, "verification_pushback_exhausted", "verification pushback exhausted", "engine", formation.ID, "verification pushback exhausted"); err != nil {
				return verificationStopped, err
			}
			return verificationStopped, nil
		}
		if !queued[formation.ID] {
			queued[formation.ID] = true
			*queue = append(*queue, formation.ID)
		}
		return verificationRetry, nil
	default:
		if err := e.appendRunBlocked(runID, "verification failed", formation.ID, ""); err != nil {
			return verificationStopped, err
		}
		return verificationStopped, nil
	}
}

func (e *RunEngine) evaluateVerification(req VerificationEvaluation) (VerificationEvaluationResult, error) {
	if e.verificationEvaluator == nil {
		if err := e.appendErrorAndBlock(req.RunID, "missing_verification_evaluator", "verification evaluator unavailable", "verification", req.NodeID, "verification evaluator unavailable"); err != nil {
			return VerificationEvaluationResult{}, err
		}
		return VerificationEvaluationResult{}, errRunStopped
	}
	return e.verificationEvaluator.EvaluateVerification(req)
}

func (e *RunEngine) ensureFormationOutputPayloads(runID string, formation FormationNode, result FormationExecutionResult) error {
	expected := make(map[string]bool, len(formation.Outputs))
	for _, output := range formation.Outputs {
		expected[output.ID] = true
		if _, ok := result.Outputs[output.ID]; !ok {
			if err := e.appendErrorAndBlock(runID, "missing_output_payload", fmt.Sprintf("formation %s did not produce output for port %s", formation.ID, output.ID), "engine", formation.ID, "missing output payload"); err != nil {
				return err
			}
			return errRunStopped
		}
	}
	for portID := range result.Outputs {
		if !expected[portID] {
			if err := e.appendErrorAndBlock(runID, "invalid_output_payload", fmt.Sprintf("formation %s produced unknown output port %s", formation.ID, portID), "engine", formation.ID, "invalid output payload"); err != nil {
				return err
			}
			return errRunStopped
		}
	}
	return nil
}

func (e *RunEngine) deliverOutputPayloads(runID string, board *BoardDocument, gates map[string]GateNode, fromNodeID string, outputs map[string]FormationOutputPayload, limits RunLimits, ready map[string]map[string]RunInputRef, queued map[string]bool, queue *[]string) error {
	for _, connection := range outgoingConnections(board.Connections, fromNodeID) {
		_, fromPort := endpointParts(connection.From)
		payload, ok := outputs[fromPort]
		if !ok {
			if err := e.appendErrorAndBlock(runID, "missing_output_payload", fmt.Sprintf("node %s did not produce output for port %s", fromNodeID, fromPort), "engine", fromNodeID, "missing output payload"); err != nil {
				return err
			}
			return errRunStopped
		}
		input := runInputRefForConnection(runID, connection, payload)
		if err := e.deliverConnection(runID, board, gates, connection, input, limits, ready, queued, queue); err != nil {
			return err
		}
	}
	return nil
}

func (e *RunEngine) deliverFormationOutput(runID string, board *BoardDocument, gates map[string]GateNode, fromNodeID string, result FormationExecutionResult, limits RunLimits, ready map[string]map[string]RunInputRef, queued map[string]bool, queue *[]string) error {
	return e.deliverOutputPayloads(runID, board, gates, fromNodeID, result.Outputs, limits, ready, queued, queue)
}

func runInputRefForConnection(runID string, connection BoardConnection, payload FormationOutputPayload) RunInputRef {
	fromNode, fromPort := endpointParts(connection.From)
	_, toPort := endpointParts(connection.To)
	ref := payload.Ref
	if ref == "" {
		ref = fmt.Sprintf("ledger://%s/%s", runID, connection.ID)
	}
	return RunInputRef{
		EdgeID:      connection.ID,
		FromNodeID:  fromNode,
		FromPortID:  fromPort,
		ToPortID:    toPort,
		Ref:         ref,
		Text:        payload.Text,
		ReportRef:   payload.ReportRef,
		ArtifactRef: payload.ArtifactRef,
	}
}

func formationOutputEventData(result FormationExecutionResult) map[string]any {
	outputs := make(map[string]FormationOutputPayload, len(result.Outputs))
	for portID, payload := range result.Outputs {
		outputs[portID] = payload
	}
	return map[string]any{
		"status":    result.Status,
		"reportRef": result.ReportRef,
		"text":      result.Text,
		"outputs":   outputs,
	}
}

func outputPayloadForPortFromEvent(event RunEvent, portID string) (FormationOutputPayload, bool) {
	outputs := outputPayloadsFromAny(event.Data["outputs"])
	payload, ok := outputs[portID]
	return payload, ok
}

func outputPayloadsFromAny(value any) map[string]FormationOutputPayload {
	switch raw := value.(type) {
	case map[string]FormationOutputPayload:
		outputs := make(map[string]FormationOutputPayload, len(raw))
		for portID, payload := range raw {
			outputs[portID] = payload
		}
		return outputs
	case map[string]any:
		outputs := make(map[string]FormationOutputPayload, len(raw))
		for portID, payload := range raw {
			outputs[portID] = outputPayloadFromAny(payload)
		}
		return outputs
	default:
		return nil
	}
}

func outputPayloadFromAny(value any) FormationOutputPayload {
	switch raw := value.(type) {
	case FormationOutputPayload:
		return raw
	case map[string]any:
		return FormationOutputPayload{
			Ref:         stringFromAny(raw["ref"]),
			Text:        stringFromAny(raw["text"]),
			ReportRef:   stringFromAny(raw["reportRef"]),
			ArtifactRef: stringFromAny(raw["artifactRef"]),
		}
	default:
		return FormationOutputPayload{}
	}
}

func (e *RunEngine) deliverConnection(runID string, board *BoardDocument, gates map[string]GateNode, connection BoardConnection, input RunInputRef, limits RunLimits, ready map[string]map[string]RunInputRef, queued map[string]bool, queue *[]string) error {
	toNode, toPort := endpointParts(connection.To)
	if toNode == "" || toPort == "" {
		return nil
	}
	input.ToPortID = toPort
	if gate, ok := gates[toNode]; ok {
		return e.evaluateGate(runID, board, gates, gate, input, limits, ready, queued, queue)
	}
	if ready[toNode] == nil {
		ready[toNode] = map[string]RunInputRef{}
	}
	ready[toNode][toPort] = input
	formation, ok := findFormation(board.Formations, toNode)
	if !ok {
		return nil
	}
	if formationReady(formation, ready[toNode]) {
		if !queued[toNode] {
			queued[toNode] = true
			*queue = append(*queue, toNode)
		}
		return nil
	}
	return e.appendWaiting(runID, formation, ready[toNode])
}

func (e *RunEngine) evaluateGate(runID string, board *BoardDocument, gates map[string]GateNode, gate GateNode, input RunInputRef, limits RunLimits, ready map[string]map[string]RunInputRef, queued map[string]bool, queue *[]string) error {
	judgeChain := judgeChainForGate(board, gate.ID)
	if err := e.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventGateEvaluating,
		GateID: gate.ID,
		NodeID: gate.ID,
		Data: map[string]any{
			"kinds":      gate.Kinds,
			"criterion":  gate.Criterion,
			"inputRef":   input,
			"judgeChain": formationIDs(judgeChain),
		},
	}); err != nil {
		return err
	}
	humanGate := hasGateKind(gate.Kinds, "human")
	evaluationGate := gate
	evaluationGate.Kinds = withoutGateKind(gate.Kinds, "human")
	if len(evaluationGate.Kinds) == 0 && humanGate {
		if err := e.store.AppendRunEvent(runID, RunEvent{
			Type:   RunEventHumanInputRequested,
			GateID: gate.ID,
			NodeID: gate.ID,
			Data: map[string]any{
				"gateId":         gate.ID,
				"nodeId":         gate.ID,
				"prompt":         gate.Criterion,
				"choices":        []string{"pass", "fail"},
				"requestedBy":    gate.ID,
				"inputRef":       input,
				"codeVerdict":    "",
				"codeReason":     "",
				"codePerKind":    map[string]string{},
				"timeoutSeconds": 0,
			},
		}); err != nil {
			return err
		}
		return errRunStopped
	}
	result, err := e.evaluateGateResult(board, evaluationGate, GateEvaluation{
		RunID:     runID,
		GateID:    gate.ID,
		Title:     gate.Title,
		Kinds:     evaluationGate.Kinds,
		Criterion: gate.Criterion,
		Input:     input,
	}, limits)
	if err != nil {
		return err
	}
	verdict, ok := parseStrictVerdict(result.Verdict)
	if !ok {
		if err := e.appendAmbiguousGateVerdictBlock(runID, gate.ID, result.Verdict); err != nil {
			return err
		}
		return errRunStopped
	}
	if humanGate && verdict == "pass" {
		if err := e.store.AppendRunEvent(runID, RunEvent{
			Type:   RunEventHumanInputRequested,
			GateID: gate.ID,
			NodeID: gate.ID,
			Data: map[string]any{
				"gateId":         gate.ID,
				"nodeId":         gate.ID,
				"prompt":         gate.Criterion,
				"choices":        []string{"pass", "fail"},
				"requestedBy":    gate.ID,
				"inputRef":       input,
				"codeVerdict":    verdict,
				"codeReason":     result.Reason,
				"codePerKind":    result.PerKind,
				"timeoutSeconds": 0,
			},
		}); err != nil {
			return err
		}
		return errRunStopped
	}
	return e.routeGateEvaluation(runID, board, gates, gate, input, verdict, result, limits, ready, queued, queue)
}

func (e *RunEngine) routeGateEvaluation(runID string, board *BoardDocument, gates map[string]GateNode, gate GateNode, input RunInputRef, verdict string, result GateEvaluationResult, limits RunLimits, ready map[string]map[string]RunInputRef, queued map[string]bool, queue *[]string) error {
	routePort := verdict
	routes := outgoingConnectionsFromPort(board.Connections, gate.ID, routePort)
	if verdict == "fail" && len(routes) == 0 {
		routePort = "none"
	}
	data := map[string]any{
		"verdict":     verdict,
		"perKind":     result.PerKind,
		"routePort":   routePort,
		"routedEdges": connectionIDs(routes),
		"reason":      result.Reason,
		"inputRef":    input,
	}
	if len(result.Outputs) > 0 {
		data["outputs"] = result.Outputs
	}
	if len(result.Evidence) > 0 {
		data["script"] = result.Evidence
	}
	if err := e.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventGateVerdict,
		GateID: gate.ID,
		NodeID: gate.ID,
		Data:   data,
	}); err != nil {
		return err
	}
	if routePort == "none" {
		if err := e.appendRunBlocked(runID, "gate fail is unwired", "", gate.ID); err != nil {
			return err
		}
		return errRunStopped
	}
	for _, route := range routes {
		nextInput := input
		nextInput.EdgeID = route.ID
		nextInput.FromNodeID = gate.ID
		nextInput.FromPortID = routePort
		nextInput.Ref = fmt.Sprintf("ledger://%s/%s", runID, route.ID)
		if payload, ok := result.Outputs[routePort]; ok {
			nextInput.Text = payload.Text
			nextInput.ReportRef = payload.ReportRef
			nextInput.ArtifactRef = payload.ArtifactRef
			if payload.Ref != "" {
				nextInput.Ref = payload.Ref
			}
		}
		if err := e.deliverConnection(runID, board, gates, route, nextInput, limits, ready, queued, queue); err != nil {
			return err
		}
	}
	return nil
}

func (e *RunEngine) routeGateVerdict(runID string, board *BoardDocument, gate GateNode, input RunInputRef, verdict, reason string, limits RunLimits) (*RunStatusProjection, error) {
	routes := outgoingConnectionsFromPort(board.Connections, gate.ID, verdict)
	routePort := verdict
	if verdict == "fail" && len(routes) == 0 {
		routePort = "none"
	}
	if err := e.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventGateVerdict,
		GateID: gate.ID,
		NodeID: gate.ID,
		Data: map[string]any{
			"verdict":     verdict,
			"perKind":     map[string]string{"human": verdict},
			"routePort":   routePort,
			"routedEdges": connectionIDs(routes),
			"reason":      reason,
			"inputRef":    input,
		},
	}); err != nil {
		return nil, err
	}
	if routePort == "none" {
		if err := e.appendRunBlocked(runID, "gate fail is unwired", "", gate.ID); err != nil {
			return nil, err
		}
		return e.store.ProjectRun(runID)
	}

	if err := e.appendRunBlocked(runID, "human gate verdict recorded; resume required", "", gate.ID); err != nil {
		return nil, err
	}
	return e.store.ProjectRun(runID)
}

func (e *RunEngine) evaluateGateResult(board *BoardDocument, gate GateNode, req GateEvaluation, limits RunLimits) (GateEvaluationResult, error) {
	judgeChain := judgeChainForGate(board, gate.ID)
	if len(judgeChain) > 0 {
		text, err := e.runJudgeChain(board, req, judgeChain, limits)
		if err != nil {
			return GateEvaluationResult{}, err
		}
		// The judge formation's raw output is verified strictly by the caller
		// (evaluateGate) so judge, script, and evaluator gates share one parser
		// and one ambiguous-verdict block path.
		return GateEvaluationResult{
			Verdict: strings.TrimSpace(text),
			Reason:  "judge chain",
		}, nil
	}
	if hasGateKind(gate.Kinds, "script") {
		result, err := e.evaluateScriptGate(req, gate.Script)
		if err != nil {
			if blockErr := e.appendGateErrorAndBlock(req.RunID, gate.ID, scriptGateErrorCode(err), err.Error(), "script", "script gate unavailable"); blockErr != nil {
				return GateEvaluationResult{}, blockErr
			}
			return GateEvaluationResult{}, errRunStopped
		}
		return result, nil
	}
	if e.gateEvaluator == nil {
		if err := e.appendGateErrorAndBlock(req.RunID, gate.ID, "missing_gate_evaluator", "gate evaluator unavailable", "gate", "gate evaluator unavailable"); err != nil {
			return GateEvaluationResult{}, err
		}
		return GateEvaluationResult{}, errRunStopped
	}
	return e.gateEvaluator.EvaluateGate(req)
}

func (e *RunEngine) runJudgeChain(board *BoardDocument, req GateEvaluation, chain []FormationNode, limits RunLimits) (string, error) {
	input := req.Input
	var finalText string
	for _, formation := range chain {
		if err := e.store.AppendRunEvent(req.RunID, RunEvent{
			Type:    RunEventNodeStarted,
			NodeID:  formation.ID,
			Attempt: 1,
			Data: map[string]any{
				"nodeKind":  "formation",
				"inputRefs": []RunInputRef{input},
				"reason":    "judge",
				"brief":     formationBriefEventData(formationBriefValue(formation)),
			},
		}); err != nil {
			return "", err
		}
		result, err := e.executeFormation(FormationExecution{
			RunID:     req.RunID,
			NodeID:    formation.ID,
			Title:     formation.Title,
			Formation: formation,
			Brief:     formationBriefValue(formation),
			Inputs:    []RunInputRef{input},
			Attempt:   1,
		}, limits)
		if err != nil {
			if blockErr := e.appendExecutionFailureAndBlock(req.RunID, formation.ID, err); blockErr != nil {
				return "", blockErr
			}
			return "", errRunStopped
		}
		if result.Status == "" {
			result.Status = "done"
		}
		if err := e.ensureFormationOutputPayloads(req.RunID, formation, result); err != nil {
			return "", err
		}
		data := formationOutputEventData(result)
		data["reason"] = "judge"
		if err := e.store.AppendRunEvent(req.RunID, RunEvent{
			Type:   RunEventNodeOutput,
			NodeID: formation.ID,
			Data:   data,
		}); err != nil {
			return "", err
		}
		finalText = result.Text
		if len(formation.Outputs) == 0 {
			return "", fmt.Errorf("%w: judge formation %q has no output port", ErrConflict, formation.ID)
		}
		fromPortID := formation.Outputs[0].ID
		payload := result.Outputs[fromPortID]
		input = RunInputRef{
			FromNodeID:  formation.ID,
			FromPortID:  fromPortID,
			Ref:         payload.Ref,
			Text:        payload.Text,
			ReportRef:   payload.ReportRef,
			ArtifactRef: payload.ArtifactRef,
		}
		if input.Ref == "" {
			input.Ref = fmt.Sprintf("ledger://%s/%s", req.RunID, formation.ID)
		}
	}
	return finalText, nil
}

func (e *RunEngine) appendErrorAndBlock(runID, code, message, boundary, nodeID, blockReason string) error {
	return e.appendErrorAndBlockWithDetails(runID, code, message, boundary, nodeID, blockReason, "", "")
}

func (e *RunEngine) appendErrorAndBlockWithDetails(runID, code, message, boundary, nodeID, blockReason, slotID, dispatchID string) error {
	message = redactLedgerText(message)
	blockReason = redactLedgerText(blockReason)
	data := map[string]any{
		"code":        code,
		"message":     message,
		"reason":      blockReason,
		"boundary":    boundary,
		"nodeId":      nodeID,
		"recoverable": true,
	}
	if slotID != "" {
		data["slotId"] = slotID
	}
	if dispatchID != "" {
		data["dispatchId"] = dispatchID
	}
	if err := e.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventError,
		NodeID: nodeID,
		SlotID: slotID,
		Data:   data,
	}); err != nil {
		return err
	}
	return e.appendRunBlockedWithDispatches(runID, blockReason, nodeID, "", slotID, openDispatchesForBlock(nodeID, slotID, dispatchID))
}

func (e *RunEngine) appendGateErrorAndBlock(runID, gateID, code, message, boundary, blockReason string) error {
	message = redactLedgerText(message)
	blockReason = redactLedgerText(blockReason)
	if err := e.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventError,
		GateID: gateID,
		NodeID: gateID,
		Data: map[string]any{
			"code":        code,
			"message":     message,
			"reason":      blockReason,
			"boundary":    boundary,
			"gateId":      gateID,
			"recoverable": true,
		},
	}); err != nil {
		return err
	}
	return e.appendRunBlocked(runID, blockReason, "", gateID)
}

type executionFailureDetails struct {
	Code       string
	Message    string
	Boundary   string
	NodeID     string
	SlotID     string
	DispatchID string
}

func (e *RunEngine) appendExecutionFailureAndBlock(runID, nodeID string, err error) error {
	failure := executionFailureEvent(err)
	if failure.NodeID != "" {
		nodeID = failure.NodeID
	}
	return e.appendErrorAndBlockWithDetails(runID, failure.Code, failure.Message, failure.Boundary, nodeID, failure.Message, failure.SlotID, failure.DispatchID)
}

func executionFailureEvent(err error) executionFailureDetails {
	var executionErr *RunExecutionError
	if errors.As(err, &executionErr) {
		boundary := executionErr.Boundary
		if boundary == "" {
			boundary = "executor"
		}
		return executionFailureDetails{
			Code:       executionErr.Code,
			Message:    executionErr.Message,
			Boundary:   boundary,
			NodeID:     executionErr.NodeID,
			SlotID:     executionErr.SlotID,
			DispatchID: executionErr.DispatchID,
		}
	}
	switch {
	case errors.Is(err, ErrRunWallClockExceeded):
		return executionFailureDetails{Code: "wall_clock_exceeded", Message: "wall clock limit exceeded", Boundary: "limits"}
	case errors.Is(err, ErrRunExecutorUnavailable):
		return executionFailureDetails{Code: "missing_executor", Message: "formation executor unavailable", Boundary: "executor"}
	default:
		if err == nil {
			return executionFailureDetails{Code: "executor_failed", Message: "formation executor failed", Boundary: "executor"}
		}
		return executionFailureDetails{Code: "executor_failed", Message: redactLedgerText(err.Error()), Boundary: "executor"}
	}
}

func (e *RunEngine) appendRunBlocked(runID, reason, nodeID, gateID string) error {
	return e.appendRunBlockedWithDispatches(runID, reason, nodeID, gateID, "", nil)
}

func (e *RunEngine) appendRunBlockedWithDispatches(runID, reason, nodeID, gateID, slotID string, openDispatches []map[string]any) error {
	if openDispatches == nil {
		openDispatches = []map[string]any{}
	}
	return e.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventBlocked,
		NodeID: nodeID,
		SlotID: slotID,
		GateID: gateID,
		Data: map[string]any{
			"reason":         redactLedgerText(reason),
			"blockedNodeId":  nodeID,
			"blockedGateId":  gateID,
			"resumeAllowed":  true,
			"resumePolicy":   "explicit",
			"openDispatches": openDispatches,
			"nextEpoch":      1,
		},
	})
}

func openDispatchesForBlock(nodeID, slotID, dispatchID string) []map[string]any {
	if dispatchID == "" {
		return nil
	}
	return []map[string]any{{
		"dispatchId": dispatchID,
		"nodeId":     nodeID,
		"slotId":     slotID,
	}}
}

type starvedFormation struct {
	ID      string
	Title   string
	Missing []string
}

// starvedFormations returns the reachable formations that received at least one
// input but can never become runnable because a required input was never
// produced. A formation that completed has all its inputs filled (it only runs
// when formationReady), so it is excluded; one that was never reached has no
// entry in ready and is excluded too. The result is sorted by ID so the run
// ledger is deterministic.
func starvedFormations(formationByID map[string]FormationNode, ready map[string]map[string]RunInputRef) []starvedFormation {
	var starved []starvedFormation
	for id, formation := range formationByID {
		fed := ready[id]
		if len(fed) == 0 || formationReady(formation, fed) {
			continue
		}
		missing := make([]string, 0, len(formation.Inputs))
		for _, input := range formation.Inputs {
			if _, ok := fed[input.ID]; !ok {
				missing = append(missing, input.ID)
			}
		}
		starved = append(starved, starvedFormation{ID: id, Title: formation.Title, Missing: missing})
	}
	sort.Slice(starved, func(i, j int) bool { return starved[i].ID < starved[j].ID })
	return starved
}

// appendStarvedBlock records a fail-loud run_blocked instead of run_succeeded
// when reachable required formations can never run. Resume cannot conjure a
// missing producer, so the run is blocked non-resumably with recovery guidance:
// wire a producer to the starved ports and start a new run.
func (e *RunEngine) appendStarvedBlock(runID string, starved []starvedFormation) error {
	waitingNodes := make([]map[string]any, 0, len(starved))
	for _, s := range starved {
		waitingNodes = append(waitingNodes, map[string]any{
			"nodeId":        s.ID,
			"title":         s.Title,
			"missingInputs": s.Missing,
		})
	}
	primary := starved[0]
	reason := fmt.Sprintf("formation %q is waiting on inputs %v that no upstream node produces; wire a producer to those ports and start a new run", primary.ID, primary.Missing)
	return e.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventBlocked,
		NodeID: primary.ID,
		Data: map[string]any{
			"reason":         reason,
			"code":           "reachable_node_starved",
			"blockedNodeId":  primary.ID,
			"blockedGateId":  "",
			"boundary":       "wiring",
			"waitingNodes":   waitingNodes,
			"recoverable":    false,
			"resumeAllowed":  false,
			"resumePolicy":   "authoring",
			"openDispatches": []map[string]any{},
			"nextEpoch":      1,
		},
	})
}

// appendAmbiguousGateVerdictBlock records a fail-loud run_blocked when a gate
// (judge formation, gate evaluator, or human decision) returns a verdict that is
// not exactly "pass" or "fail". Routing such a verdict would silently pick a
// branch the verdict never authorized, so the run blocks non-resumably: resume
// cannot reinterpret the offending text, the judge/evaluator must be fixed and a
// new run started.
func (e *RunEngine) appendAmbiguousGateVerdictBlock(runID, gateID, rawVerdict string) error {
	reason := fmt.Sprintf("gate %s returned an unrecognized verdict %q; expected exactly \"pass\" or \"fail\"", gateID, verdictSnippet(rawVerdict))
	return e.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventBlocked,
		NodeID: gateID,
		GateID: gateID,
		Data: map[string]any{
			"reason":         reason,
			"code":           "ambiguous_gate_verdict",
			"blockedNodeId":  gateID,
			"blockedGateId":  gateID,
			"boundary":       "verdict",
			"recoverable":    false,
			"resumeAllowed":  false,
			"resumePolicy":   "authoring",
			"openDispatches": []map[string]any{},
			"nextEpoch":      1,
		},
	})
}

// appendAmbiguousVerificationVerdictBlock is the verification-path twin of
// appendAmbiguousGateVerdictBlock: an inline verification verdict that is not
// exactly "pass" or "fail" blocks the run non-resumably instead of failing open
// to pass and continuing downstream.
func (e *RunEngine) appendAmbiguousVerificationVerdictBlock(runID, nodeID, verificationID, rawVerdict string) error {
	reason := fmt.Sprintf("verification %s returned an unrecognized verdict %q; expected exactly \"pass\" or \"fail\"", verificationID, verdictSnippet(rawVerdict))
	return e.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventBlocked,
		NodeID: nodeID,
		Data: map[string]any{
			"reason":         reason,
			"code":           "ambiguous_verification_verdict",
			"blockedNodeId":  nodeID,
			"blockedGateId":  "",
			"boundary":       "verdict",
			"recoverable":    false,
			"resumeAllowed":  false,
			"resumePolicy":   "authoring",
			"openDispatches": []map[string]any{},
			"nextEpoch":      1,
		},
	})
}

func maxAttempts(limits RunLimits) int {
	if limits.MaxAttempts > 0 {
		return limits.MaxAttempts
	}
	return 1
}

func runLimitsFromEvent(event RunEvent) RunLimits {
	if event.Data == nil {
		return RunLimits{}
	}
	switch limits := event.Data["limits"].(type) {
	case RunLimits:
		return limits
	case map[string]any:
		return RunLimits{
			MaxDispatch:      intFromRunEventData(limits["maxDispatch"]),
			MaxAttempts:      intFromRunEventData(limits["maxAttempts"]),
			WallClockSeconds: intFromRunEventData(limits["wallClockSeconds"]),
			Redact:           boolFromAny(limits["redact"]),
		}
	default:
		return RunLimits{}
	}
}

func intFromRunEventData(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func boolFromAny(value any) bool {
	v, _ := value.(bool)
	return v
}

func latestHumanRequest(events []RunEvent, gateID string) (RunEvent, bool) {
	var request RunEvent
	var found bool
	for _, event := range events {
		if event.GateID != gateID {
			continue
		}
		switch event.Type {
		case RunEventHumanInputRequested:
			request = event
			found = true
		case RunEventHumanVerdictRecorded:
			found = false
		}
	}
	return request, found
}

func runInputRefFromAny(value any) RunInputRef {
	raw, ok := value.(map[string]any)
	if !ok {
		return RunInputRef{}
	}
	return RunInputRef{
		EdgeID:      stringFromAny(raw["edgeId"]),
		FromNodeID:  stringFromAny(raw["fromNodeId"]),
		FromPortID:  stringFromAny(raw["fromPortId"]),
		ToPortID:    stringFromAny(raw["toPortId"]),
		OutputSeq:   intFromRunEventData(raw["outputSeq"]),
		Ref:         stringFromAny(raw["ref"]),
		Text:        stringFromAny(raw["text"]),
		ReportRef:   stringFromAny(raw["reportRef"]),
		ArtifactRef: stringFromAny(raw["artifactRef"]),
	}
}

func hasGateKind(kinds []string, want string) bool {
	for _, kind := range kinds {
		if kind == want {
			return true
		}
	}
	return false
}

func withoutGateKind(kinds []string, without string) []string {
	filtered := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		if kind != without {
			filtered = append(filtered, kind)
		}
	}
	return filtered
}

func findGate(gates []GateNode, id string) (GateNode, bool) {
	for _, gate := range gates {
		if gate.ID == id {
			return gate, true
		}
	}
	return GateNode{}, false
}

func formationBriefValue(formation FormationNode) FormationBrief {
	if formation.Brief == nil {
		return FormationBrief{}
	}
	return FormationBrief{
		Goal:   formation.Brief.Goal,
		BeadID: formation.Brief.BeadID,
		Files:  append([]string(nil), formation.Brief.Files...),
		Links:  append([]string(nil), formation.Brief.Links...),
	}
}

func formationBriefEventData(brief FormationBrief) map[string]any {
	return map[string]any{
		"goal":   brief.Goal,
		"beadId": brief.BeadID,
		"files":  append([]string(nil), brief.Files...),
		"links":  append([]string(nil), brief.Links...),
	}
}

// recognizedVerdicts is the single source of truth for the verdict tokens a
// judge formation, gate evaluator, verification evaluator, or human decision may
// emit. Anything outside this set is ambiguous and must block the run loudly
// rather than fail open to pass (see parseStrictVerdict callers).
var recognizedVerdicts = map[string]string{
	"pass": "pass",
	"fail": "fail",
}

// parseStrictVerdict recognizes a verdict only when its trimmed, lower-cased
// text is exactly one of recognizedVerdicts. It returns (canonical, true) for a
// recognized verdict and ("", false) otherwise. Callers must block loudly on a
// false result instead of routing pass.
func parseStrictVerdict(verdict string) (string, bool) {
	canonical, ok := recognizedVerdicts[strings.ToLower(strings.TrimSpace(verdict))]
	return canonical, ok
}

// verdictSnippet returns a redacted, length-bounded view of an offending verdict
// for inclusion in a block reason. Empty input renders as an explicit marker so
// the message never reads as if nothing was checked.
func verdictSnippet(verdict string) string {
	snippet := redactLedgerText(verdict)
	if snippet == "" {
		return "(empty)"
	}
	const maxVerdictSnippet = 120
	if len(snippet) > maxVerdictSnippet {
		snippet = snippet[:maxVerdictSnippet] + "…"
	}
	return snippet
}

func connectionIDs(connections []BoardConnection) []string {
	ids := make([]string, 0, len(connections))
	for _, connection := range connections {
		ids = append(ids, connection.ID)
	}
	return ids
}

func formationIDs(formations []FormationNode) []string {
	ids := make([]string, 0, len(formations))
	for _, formation := range formations {
		ids = append(ids, formation.ID)
	}
	return ids
}

func judgeChainForGate(board *BoardDocument, gateID string) []FormationNode {
	entries := outgoingConnectionsFromPort(board.Connections, gateID, "judge")
	if len(entries) == 0 {
		return nil
	}
	formationByID := map[string]FormationNode{}
	for _, formation := range board.Formations {
		formationByID[formation.ID] = formation
	}
	currentNode, _ := endpointParts(entries[0].To)
	visited := map[string]bool{}
	var chain []FormationNode
	for currentNode != "" && !visited[currentNode] {
		visited[currentNode] = true
		formation, ok := formationByID[currentNode]
		if !ok {
			return chain
		}
		chain = append(chain, formation)
		var nextNode string
		for _, connection := range outgoingConnections(board.Connections, currentNode) {
			toNode, _ := endpointParts(connection.To)
			if toNode == gateID && strings.HasSuffix(connection.To, ":judge") {
				return chain
			}
			if _, ok := formationByID[toNode]; ok && nextNode == "" {
				nextNode = toNode
			}
		}
		currentNode = nextNode
	}
	return chain
}

func outgoingConnectionsFromPort(connections []BoardConnection, nodeID, portID string) []BoardConnection {
	var out []BoardConnection
	for _, connection := range connections {
		fromNode, fromPort := endpointParts(connection.From)
		if fromNode == nodeID && fromPort == portID {
			out = append(out, connection)
		}
	}
	return out
}

func (e *RunEngine) appendWaiting(runID string, formation FormationNode, ready map[string]RunInputRef) error {
	waitingFor := make([]string, 0, len(formation.Inputs))
	for _, input := range formation.Inputs {
		if _, ok := ready[input.ID]; !ok {
			waitingFor = append(waitingFor, input.ID)
		}
	}
	return e.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventNodeWaiting,
		NodeID: formation.ID,
		Data: map[string]any{
			"neededInputs": len(waitingFor),
			"readyInputs":  len(ready),
			"totalInputs":  len(formation.Inputs),
			"waitingFor":   waitingFor,
		},
	})
}

func outgoingConnections(connections []BoardConnection, nodeID string) []BoardConnection {
	var out []BoardConnection
	for _, connection := range connections {
		fromNode, _ := endpointParts(connection.From)
		if fromNode == nodeID {
			out = append(out, connection)
		}
	}
	return out
}

func endpointParts(endpoint string) (string, string) {
	parts := strings.SplitN(endpoint, ":", 2)
	if len(parts) != 2 {
		return endpoint, ""
	}
	return parts[0], parts[1]
}

func findFormation(formations []FormationNode, id string) (FormationNode, bool) {
	for _, formation := range formations {
		if formation.ID == id {
			return formation, true
		}
	}
	return FormationNode{}, false
}

func formationReady(formation FormationNode, ready map[string]RunInputRef) bool {
	if len(formation.Inputs) == 0 {
		return true
	}
	if len(ready) < len(formation.Inputs) {
		return false
	}
	for _, input := range formation.Inputs {
		if _, ok := ready[input.ID]; !ok {
			return false
		}
	}
	return true
}

func orderedInputs(formation FormationNode, ready map[string]RunInputRef) []RunInputRef {
	inputs := make([]RunInputRef, 0, len(formation.Inputs))
	for _, input := range formation.Inputs {
		if ref, ok := ready[input.ID]; ok {
			inputs = append(inputs, ref)
		}
	}
	return inputs
}
