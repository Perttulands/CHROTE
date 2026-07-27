package formations

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	errRunStopped               = errors.New("formations run stopped")
	ErrRunExecutorUnavailable   = errors.New("formations run executor unavailable")
	ErrGateEvaluatorUnavailable = errors.New("formations gate evaluator unavailable")
	ErrRunWallClockExceeded     = errors.New("formations run wall clock exceeded")
)

type FormationExecutor interface {
	ExecuteFormation(FormationExecution) (FormationExecutionResult, error)
}

type FormationReattachExecutor interface {
	ReattachFormationDispatch(FormationReattachRequest) (FormationExecutionResult, error)
}

type FormationReattachRequest struct {
	RunID      string
	DispatchID string
	NodeID     string
	SlotID     string
	Formation  FormationNode
}

type unavailableFormationExecutor struct {
	boundary string
}

type GateEvaluator interface {
	EvaluateGate(GateEvaluation) (GateEvaluationResult, error)
}

type RunEngine struct {
	store            *Store
	personas         *PersonaStore
	executor         FormationExecutor
	gateEvaluator    GateEvaluator
	needsYouNotifier NeedsYouNotifier
	needsYouBoardURL string
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
	RunID        string
	GateID       string
	Title        string
	Kinds        []string
	Criterion    string
	Check        string
	CheckVersion string
	CheckValue   string
	Input        RunInputRef
	Binding      *RunGateBinding
}

type GateEvaluationResult struct {
	Verdict         string
	Reason          string
	Evidence        []GateEvidenceRef
	PerKind         map[string]string
	KindResultSeqs  map[string]int
	CodeVerdict     string
	CodeReason      string
	ResultEncoding  string
	ResultSHA256    string
	CanonicalResult string
	GateBindingID   string
}

type GateEvidenceRef struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
}

type HumanGateVerdictRequest struct {
	GateID  string
	Verdict string
	Reason  string
	Actor   string
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

// SetNeedsYouNotifier wires the outbound needs-you channel. A nil notifier
// leaves the feature off (reconcileNeedsYou becomes a no-op). boardBaseURL is an
// optional origin used to build a pointer back to the board.
func (e *RunEngine) SetNeedsYouNotifier(notifier NeedsYouNotifier, boardBaseURL string) {
	e.needsYouNotifier = notifier
	e.needsYouBoardURL = strings.TrimSpace(boardBaseURL)
}

// projectAndNotify pushes any newly-opened needs-you asks and then returns the
// run projection. Notification is best-effort and never affects the run result.
func (e *RunEngine) projectAndNotify(runID string) (*RunStatusProjection, error) {
	e.reconcileNeedsYou(runID)
	return e.store.ProjectRun(runID)
}

// reconcileNeedsYou delivers one notification per open ask that has not been
// delivered yet, sourced from the durable run ledger. It is idempotent: dedup is
// persisted, resolved asks are never projected as open, and send failures leave
// the ask undelivered so the next genuine state transition retries it.
func (e *RunEngine) reconcileNeedsYou(runID string) {
	if e == nil || e.needsYouNotifier == nil || e.store == nil {
		return
	}
	events, err := e.store.ReadRunEvents(runID)
	if err != nil || len(events) == 0 {
		return
	}
	asks := projectOpenNeedsYouAsks(events)
	if len(asks) == 0 {
		return
	}
	notified, err := e.store.NeedsYouNotifiedSeqs(runID)
	if err != nil {
		return
	}
	boardSlug := stringFromEventData(events[0], "boardSlug")
	for _, ask := range asks {
		if notified[ask.Seq] {
			continue
		}
		notification := buildNeedsYouNotification(ask, boardSlug, e.needsYouBoardURL)
		if err := e.needsYouNotifier.NotifyNeedsYou(context.Background(), notification); err != nil {
			continue // best-effort; retry on the next state transition
		}
		_ = e.store.MarkNeedsYouNotified(runID, ask.Seq)
	}
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
	if err := preflightMissionMigrations(board, mission.ID, reachableNodeIDs(board, mission.ID)); err != nil {
		return nil, err
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
	runBoard, err := e.readRunBoard(started.RunID)
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
	return e.projectAndNotify(started.RunID)
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
	if err := preflightIsolatedFormationDefinition(board, formation.ID); err != nil {
		return nil, err
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
		return e.projectAndNotify(started.RunID)
	}
	if result.Status == "" {
		result.Status = "done"
	}
	if err := e.ensureFormationOutputPayloads(started.RunID, formation, result); err != nil {
		if errors.Is(err, errRunStopped) {
			return e.projectAndNotify(started.RunID)
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
	return e.projectAndNotify(started.RunID)
}

func (e *RunEngine) ResumeRun(runID string, req RunResumeRequest) (*RunStatusProjection, error) {
	if e == nil || e.store == nil {
		return nil, fmt.Errorf("%w: run engine store required", ErrNotFound)
	}
	if err := e.store.RequireRuntimeAuthority(); err != nil {
		return nil, err
	}
	beforeEvents, err := e.store.ReadRunEvents(runID)
	if err != nil {
		return nil, err
	}
	beforeBoard, err := e.readRunBoard(runID)
	if err != nil {
		return nil, err
	}
	if err := e.validateDurableCodeGateState(runID, beforeBoard, beforeEvents); err != nil {
		return nil, err
	}
	_, board, err := e.store.resumeRunWithSnapshot(runID, req)
	if err != nil {
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
		openDispatches = enrichOpenDispatchRefs(events, openDispatches)
		handled, err := e.reattachOpenDispatches(runID, board, openDispatches)
		if err != nil {
			return nil, err
		}
		if !handled {
			if err := e.appendOpenDispatchReattachFailure(runID, openDispatches); err != nil {
				return nil, err
			}
			return e.projectAndNotify(runID)
		}
		events, err = e.store.ReadRunEvents(runID)
		if err != nil {
			return nil, err
		}
	}
	started := events[0]
	mission, ok := findMission(board, started.MissionID)
	if !ok {
		return nil, fmt.Errorf("%w: mission %q", ErrNotFound, started.MissionID)
	}
	if err := e.resumeSnapshot(runID, board, mission, runLimitsFromEvent(started), events); err != nil {
		return nil, err
	}
	return e.projectAndNotify(runID)
}

func (e *RunEngine) validateDurableCodeGateState(runID string, board *BoardDocument, events []RunEvent) error {
	gates := map[string]GateNode{}
	for _, gate := range board.Gates {
		gates[gate.ID] = gate
	}
	for _, event := range events {
		gateID := event.GateID
		if gateID == "" {
			gateID = event.NodeID
		}
		gate, ok := gates[gateID]
		if !ok {
			continue
		}
		switch event.Type {
		case RunEventGateKindResult:
			if stringFromAny(event.Data["kind"]) != "code" {
				continue
			}
			input := runInputRefFromAny(event.Data["inputRef"])
			if _, err := e.gateKindResultFromEvent(runID, gate, input, event); err != nil {
				return err
			}
		case RunEventHumanInputRequested:
			if err := e.validateHumanRequestKindResults(runID, gate, event, events); err != nil {
				return err
			}
		case RunEventGateVerdict:
			if err := e.validateGateVerdictKindResults(runID, gate, event, events); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *RunEngine) validateGateVerdictKindResults(runID string, gate GateNode, verdictEvent RunEvent, events []RunEvent) error {
	rawSeqs, present := verdictEvent.Data["kindResultSeqs"]
	if !present {
		return nil
	}
	seqs := intMapFromRunEventData(rawSeqs)
	perKind := stringMapFromRunEventData(verdictEvent.Data["perKind"])
	input := runInputRefFromAny(verdictEvent.Data["inputRef"])
	for kind, seq := range seqs {
		if kind != "code" && kind != "formation" {
			return fmt.Errorf("%w: Gate %q aggregate result names unknown kind %q", ErrRunLedgerInvalid, gate.ID, kind)
		}
		if seq <= 0 || seq >= verdictEvent.Seq || seq > len(events) {
			return fmt.Errorf("%w: Gate %q aggregate %s result sequence is invalid", ErrRunLedgerInvalid, gate.ID, kind)
		}
		event := events[seq-1]
		eventGateID := event.GateID
		if eventGateID == "" {
			eventGateID = event.NodeID
		}
		if event.Seq != seq ||
			event.Type != RunEventGateKindResult ||
			eventGateID != gate.ID ||
			stringFromAny(event.Data["kind"]) != kind ||
			runInputRefFromAny(event.Data["inputRef"]) != input {
			return fmt.Errorf("%w: Gate %q aggregate %s result identity mismatch", ErrRunLedgerInvalid, gate.ID, kind)
		}
		result, err := e.gateKindResultFromEvent(runID, gate, input, event)
		if err != nil {
			return err
		}
		if perKind[kind] != result.Verdict {
			return fmt.Errorf("%w: Gate %q aggregate %s verdict mismatch", ErrRunLedgerInvalid, gate.ID, kind)
		}
		if kind == "code" &&
			(stringFromAny(verdictEvent.Data["codeVerdict"]) != result.Verdict ||
				stringFromAny(verdictEvent.Data["codeReason"]) != result.Reason ||
				stringFromAny(verdictEvent.Data["resultEncoding"]) != result.ResultEncoding ||
				stringFromAny(verdictEvent.Data["resultSha256"]) != result.ResultSHA256 ||
				stringFromAny(verdictEvent.Data["gateBindingId"]) != result.GateBindingID ||
				!gateEvidenceRefsEqual(gateEvidenceRefsFromRunEventData(verdictEvent.Data["evidence"]), result.Evidence)) {
			return fmt.Errorf("%w: Gate %q aggregate code result mismatch", ErrRunLedgerInvalid, gate.ID)
		}
	}
	for _, kind := range []string{"code", "formation"} {
		state := perKind[kind]
		if hasGateKind(gate.Kinds, kind) && state != "" && state != "not_run" && seqs[kind] == 0 {
			return fmt.Errorf("%w: Gate %q aggregate %s result sequence is missing", ErrRunLedgerInvalid, gate.ID, kind)
		}
	}
	return nil
}

func (e *RunEngine) RecordHumanGateVerdict(runID string, req HumanGateVerdictRequest) (*RunStatusProjection, error) {
	if e == nil || e.store == nil {
		return nil, fmt.Errorf("%w: run engine store required", ErrNotFound)
	}
	if err := e.store.RequireRuntimeAuthority(); err != nil {
		return nil, err
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
	board, err := e.readRunBoard(runID)
	if err != nil {
		return nil, err
	}
	gate, ok := findGate(board.Gates, req.GateID)
	if !ok {
		return nil, fmt.Errorf("%w: gate %q", ErrNotFound, req.GateID)
	}
	if err := rejectLegacyScriptGateForRun(board, events[0], nil); err != nil {
		return nil, err
	}
	if err := rejectLegacyInlineVerification(board); err != nil {
		return nil, err
	}
	if err := e.validateHumanRequestKindResults(runID, gate, requestEvent, events); err != nil {
		return nil, err
	}
	verdict := normalizeGateVerdict(req.Verdict)
	actor := defaultRunActor(req.Actor)
	validatedBoard, err := e.store.appendRunEventWithSnapshot(runID, RunEvent{
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
	})
	if err != nil {
		return nil, err
	}
	if validatedBoard == nil {
		return nil, ErrRunLedgerInvalid
	}
	input := runInputRefFromAny(requestEvent.Data["inputRef"])
	status, err := e.routeGateVerdict(runID, validatedBoard, gate, input, verdict, "human verdict", runLimitsFromEvent(events[0]), requestEvent)
	if err != nil {
		return nil, err
	}
	e.reconcileNeedsYou(runID)
	return status, nil
}

func (e *RunEngine) validateHumanRequestKindResults(runID string, gate GateNode, request RunEvent, events []RunEvent) error {
	seqs := intMapFromRunEventData(request.Data["kindResultSeqs"])
	perKind := stringMapFromRunEventData(request.Data["codePerKind"])
	input := runInputRefFromAny(request.Data["inputRef"])
	requiredKinds := make([]string, 0, 2)
	for _, kind := range []string{"code", "formation"} {
		if hasGateKind(gate.Kinds, kind) {
			requiredKinds = append(requiredKinds, kind)
		}
	}
	if len(seqs) != len(requiredKinds) {
		return fmt.Errorf("%w: Gate %q human request kind result set mismatch", ErrRunLedgerInvalid, gate.ID)
	}
	for _, kind := range requiredKinds {
		seq := seqs[kind]
		if seq <= 0 || seq >= request.Seq || seq > len(events) {
			return fmt.Errorf("%w: Gate %q human request %s result sequence is invalid", ErrRunLedgerInvalid, gate.ID, kind)
		}
		event := events[seq-1]
		eventGateID := event.GateID
		if eventGateID == "" {
			eventGateID = event.NodeID
		}
		if event.Seq != seq ||
			event.Type != RunEventGateKindResult ||
			eventGateID != gate.ID ||
			stringFromAny(event.Data["kind"]) != kind ||
			runInputRefFromAny(event.Data["inputRef"]) != input {
			return fmt.Errorf("%w: Gate %q human request %s result identity mismatch", ErrRunLedgerInvalid, gate.ID, kind)
		}
		result, err := e.gateKindResultFromEvent(runID, gate, input, event)
		if err != nil {
			return err
		}
		if perKind[kind] != result.Verdict {
			return fmt.Errorf("%w: Gate %q human request %s verdict mismatch", ErrRunLedgerInvalid, gate.ID, kind)
		}
		if kind == "code" &&
			(stringFromAny(request.Data["codeVerdict"]) != result.Verdict ||
				stringFromAny(request.Data["codeReason"]) != result.Reason ||
				stringFromAny(request.Data["resultEncoding"]) != result.ResultEncoding ||
				stringFromAny(request.Data["resultSha256"]) != result.ResultSHA256 ||
				stringFromAny(request.Data["gateBindingId"]) != result.GateBindingID ||
				!gateEvidenceRefsEqual(gateEvidenceRefsFromRunEventData(request.Data["evidence"]), result.Evidence)) {
			return fmt.Errorf("%w: Gate %q human request code result mismatch", ErrRunLedgerInvalid, gate.ID)
		}
	}
	return nil
}

func gateEvidenceRefsEqual(left, right []GateEvidenceRef) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
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

func enrichOpenDispatchRefs(events []RunEvent, refs []openDispatchRef) []openDispatchRef {
	if len(refs) == 0 {
		return refs
	}
	dispatches := map[string]RunEvent{}
	for _, event := range events {
		if event.Type != RunEventSlotDispatch || event.Data == nil {
			continue
		}
		dispatchID := stringFromEventData(event, "dispatchId")
		if dispatchID != "" {
			dispatches[dispatchID] = event
		}
	}
	enriched := append([]openDispatchRef(nil), refs...)
	for i, ref := range enriched {
		dispatch := dispatches[ref.DispatchID]
		if ref.NodeID == "" {
			enriched[i].NodeID = dispatch.NodeID
			if enriched[i].NodeID == "" {
				enriched[i].NodeID = stringFromEventData(dispatch, "nodeId")
			}
		}
		if ref.SlotID == "" {
			enriched[i].SlotID = dispatch.SlotID
			if enriched[i].SlotID == "" {
				enriched[i].SlotID = stringFromEventData(dispatch, "slotId")
			}
		}
	}
	return enriched
}

func (e *RunEngine) reattachOpenDispatches(runID string, board *BoardDocument, refs []openDispatchRef) (bool, error) {
	reattacher, ok := e.executor.(FormationReattachExecutor)
	if !ok || reattacher == nil || len(refs) == 0 {
		return false, nil
	}
	if len(refs) != 1 {
		return false, nil
	}
	ref := refs[0]
	formation, ok := findFormation(board.Formations, ref.NodeID)
	if !ok {
		return false, nil
	}
	result, err := reattacher.ReattachFormationDispatch(FormationReattachRequest{
		RunID:      runID,
		DispatchID: ref.DispatchID,
		NodeID:     ref.NodeID,
		SlotID:     ref.SlotID,
		Formation:  formation,
	})
	if err != nil {
		return true, err
	}
	if result.Status == "" {
		result.Status = "done"
	}
	if err := e.ensureFormationOutputPayloads(runID, formation, result); err != nil {
		return true, err
	}
	if err := e.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventNodeOutput,
		NodeID: ref.NodeID,
		Data:   formationOutputEventData(result),
	}); err != nil {
		return true, err
	}
	return true, nil
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

func (e *RunEngine) startFormationRun(slug string, board *BoardDocument, formation FormationNode, actor string, personas *PersonaStore, limits RunLimits) (*RunStartResult, MissionNode, RunInputRef, error) {
	if err := e.store.RequireRuntimeAuthority(); err != nil {
		return nil, MissionNode{}, RunInputRef{}, err
	}
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
	runID := newPrefixedID("run")
	ledgerPath := runArtifactPath(slug, runID, ".ndjson")
	snapshotPath := runArtifactPath(slug, runID, ".snapshot.toml")
	bindingsPath := runArtifactPath(slug, runID, ".bindings.toml")
	boardRaw := []byte(board.TOML)
	if int64(len(boardRaw)) > runtimeAuthorityMaxRecordBytes {
		return nil, MissionNode{}, RunInputRef{}, fmt.Errorf("%w: run snapshot exceeds byte limit", ErrRunLedgerInvalid)
	}
	runDirectory, err := e.store.openRunArtifactDirectory(slug, true)
	if err != nil {
		return nil, MissionNode{}, RunInputRef{}, err
	}
	defer runDirectory.close()
	if err := writeRunArtifactExclusiveAt(runDirectory, runID+".snapshot.toml", boardRaw); err != nil {
		return nil, MissionNode{}, RunInputRef{}, err
	}
	if err := writeRunArtifactExclusiveAt(runDirectory, runID+".bindings.toml", []byte(renderRunBindings(runID, board, mission, bindings, nil))); err != nil {
		return nil, MissionNode{}, RunInputRef{}, err
	}
	started := &RunStartResult{
		RunID:                runID,
		BoardSlug:            slug,
		LedgerPath:           ledgerPath,
		SnapshotPath:         snapshotPath,
		BindingsSnapshotPath: bindingsPath,
	}
	event := RunEvent{
		Timestamp: e.store.now().Format(time.RFC3339Nano),
		RunID:     runID,
		Seq:       1,
		Type:      RunEventStarted,
		Actor:     defaultRunActor(actor),
		BoardID:   board.ID,
		BoardRev:  board.Rev,
		MissionID: mission.ID,
		BeadID:    mission.BeadID,
		Epoch:     0,
		Attempt:   0,
		Data: map[string]any{
			"boardSlug":        slug,
			"boardPath":        filepath.ToSlash(e.store.BoardPath(slug)),
			"boardRev":         board.Rev,
			"snapshot":         snapshotPath,
			"bindingsSnapshot": bindingsPath,
			"missionId":        mission.ID,
			"beadId":           mission.BeadID,
			"objective":        mission.Goal,
			"limits":           limits,
			"mode":             "formation",
			"formationId":      formation.ID,
		},
	}
	if err := writeInitialRunEventAt(runDirectory, runID, event); err != nil {
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
	if terminalPassReachedOnBoard(board, events) {
		return e.appendResumeSucceeded(runID)
	}

	ready := map[string]map[string]RunInputRef{}
	queued := map[string]bool{}
	attempts := map[string]int{}
	completed := map[string]bool{}
	lastOutputIdx := map[string]int{}
	processedGateInputs := processedGateInputRefs(board, events)
	replayOutputOrdinals := map[string]int{}
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
		if err := e.replayNodeOutputToReady(runID, board, gateByID, event, limits, processedGateInputs, replayOutputOrdinals, ready, queued, &queue); err != nil {
			if errors.Is(err, errRunStopped) {
				return nil
			}
			return err
		}
	}
	if err := e.resumeIncompleteGateEvaluations(runID, board, gateByID, events, limits, ready, queued, &queue); err != nil {
		if errors.Is(err, errRunStopped) {
			return nil
		}
		return err
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
		completionEvents := events
		if refreshed, err := e.store.ReadRunEvents(runID); err == nil && len(refreshed) > len(events) {
			completionEvents = refreshed
			completed = completedFormationsFromEvents(refreshed)
		}
		if terminalPassReachedOnBoard(board, completionEvents) || (latestGateVerdictAllowsGraphCompletion(completionEvents) && runGraphComplete(board, mission.ID, completed)) {
			return e.appendResumeSucceeded(runID)
		}
		return e.appendErrorAndBlock(runID, "resume_no_work", "no resumable work found", "engine", "", "no resumable work found")
	}
	return e.appendResumeSucceeded(runID)
}

func (e *RunEngine) resumeIncompleteGateEvaluations(runID string, board *BoardDocument, gates map[string]GateNode, events []RunEvent, limits RunLimits, ready map[string]map[string]RunInputRef, queued map[string]bool, queue *[]string) error {
	pending := map[string]RunEvent{}
	for _, event := range events {
		gateID := event.GateID
		if gateID == "" {
			gateID = event.NodeID
		}
		if gateID == "" {
			continue
		}
		switch event.Type {
		case RunEventGateEvaluating:
			pending[gateID] = event
		case RunEventGateVerdict, RunEventHumanInputRequested, RunEventError:
			delete(pending, gateID)
		}
	}
	evaluations := make([]RunEvent, 0, len(pending))
	for _, event := range pending {
		evaluations = append(evaluations, event)
	}
	sort.Slice(evaluations, func(i, j int) bool { return evaluations[i].Seq < evaluations[j].Seq })
	for _, evaluation := range evaluations {
		gateID := evaluation.GateID
		if gateID == "" {
			gateID = evaluation.NodeID
		}
		gate, ok := gates[gateID]
		if !ok {
			return fmt.Errorf("%w: gate %q", ErrNotFound, gateID)
		}
		input := runInputRefFromAny(evaluation.Data["inputRef"])
		prior, err := e.durableGateKindResultsForEvaluation(runID, gate, input, events, evaluation.Seq)
		if err != nil {
			return err
		}
		if err := e.evaluateGateKinds(runID, board, gates, gate, input, limits, ready, queued, queue, prior); err != nil {
			return err
		}
	}
	return nil
}

func (e *RunEngine) durableGateKindResultsForEvaluation(runID string, gate GateNode, input RunInputRef, events []RunEvent, evaluatingSeq int) (map[string]durableGateKindResult, error) {
	results := map[string]durableGateKindResult{}
	lastKindIndex := -1
	canonicalOrder := map[string]int{"code": 0, "formation": 1}
	for _, event := range events {
		if event.Seq <= evaluatingSeq {
			continue
		}
		gateID := event.GateID
		if gateID == "" {
			gateID = event.NodeID
		}
		if gateID != gate.ID {
			continue
		}
		if event.Type == RunEventGateEvaluating || event.Type == RunEventGateVerdict || event.Type == RunEventHumanInputRequested {
			break
		}
		if event.Type != RunEventGateKindResult {
			continue
		}
		kind := stringFromAny(event.Data["kind"])
		kindIndex, ok := canonicalOrder[kind]
		if !ok || kindIndex <= lastKindIndex {
			return nil, fmt.Errorf("%w: Gate %q has duplicate or out-of-order kind result %q", ErrRunLedgerInvalid, gate.ID, kind)
		}
		if runInputRefFromAny(event.Data["inputRef"]) != input {
			return nil, fmt.Errorf("%w: Gate %q kind result input mismatch", ErrRunLedgerInvalid, gate.ID)
		}
		result, err := e.gateKindResultFromEvent(runID, gate, input, event)
		if err != nil {
			return nil, err
		}
		results[kind] = durableGateKindResult{Seq: event.Seq, Result: result}
		lastKindIndex = kindIndex
	}
	return results, nil
}

func (e *RunEngine) gateKindResultFromEvent(runID string, gate GateNode, input RunInputRef, event RunEvent) (GateEvaluationResult, error) {
	kind := stringFromAny(event.Data["kind"])
	verdict := stringFromAny(event.Data["verdict"])
	if verdict != "pass" && verdict != "fail" {
		return GateEvaluationResult{}, fmt.Errorf("%w: Gate %q kind %q has invalid verdict", ErrRunLedgerInvalid, gate.ID, kind)
	}
	result := GateEvaluationResult{
		Verdict:        verdict,
		Reason:         stringFromAny(event.Data["reason"]),
		Evidence:       gateEvidenceRefsFromRunEventData(event.Data["evidence"]),
		PerKind:        map[string]string{kind: verdict},
		KindResultSeqs: map[string]int{kind: event.Seq},
		ResultEncoding: stringFromAny(event.Data["resultEncoding"]),
		ResultSHA256:   stringFromAny(event.Data["resultSha256"]),
		GateBindingID:  stringFromAny(event.Data["gateBindingId"]),
	}
	if kind != "code" {
		return result, nil
	}
	binding, err := e.store.readRunGateBinding(runID, gate.ID)
	if err != nil {
		return GateEvaluationResult{}, err
	}
	if stringFromAny(event.Data["inputSha256"]) != codeGateSHA256(input.Text) ||
		stringFromAny(event.Data["profileId"]) != binding.ProfileID ||
		stringFromAny(event.Data["profileVersion"]) != binding.ProfileVersion ||
		stringFromAny(event.Data["profileSha256"]) != binding.ProfileSHA256 ||
		stringFromAny(event.Data["evaluatorBundleSha256"]) != binding.EvaluatorBundleSHA256 ||
		stringFromAny(event.Data["parametersSha256"]) != binding.ParametersSHA256 ||
		stringFromAny(event.Data["policySha256"]) != binding.PolicySHA256 ||
		stringFromAny(event.Data["determinismPolicySha256"]) != binding.DeterminismPolicySHA256 ||
		intFromRunEventData(event.Data["maxInputBytes"]) != binding.MaxInputBytes ||
		intFromRunEventData(event.Data["maxResultBytes"]) != binding.MaxResultBytes ||
		intFromRunEventData(event.Data["maxOperations"]) != binding.MaxOperations {
		return GateEvaluationResult{}, fmt.Errorf("%w: Gate %q code result frozen binding mismatch", ErrRunLedgerInvalid, gate.ID)
	}
	canonical, err := canonicalCodeGateResult(result.Verdict, result.Reason, result.Evidence)
	if err != nil {
		return GateEvaluationResult{}, fmt.Errorf("%w: Gate %q canonical code result: %v", ErrRunLedgerInvalid, gate.ID, err)
	}
	result.CanonicalResult = canonical
	result.CodeVerdict = result.Verdict
	result.CodeReason = result.Reason
	if err := validateCodeGateEvaluationResult(GateEvaluation{
		RunID:        runID,
		GateID:       gate.ID,
		Check:        gate.Check,
		CheckVersion: gate.CheckVersion,
		CheckValue:   gate.CheckValue,
		Input:        input,
		Binding:      binding,
	}, result); err != nil {
		return GateEvaluationResult{}, fmt.Errorf("%w: %v", ErrRunLedgerInvalid, err)
	}
	return result, nil
}

func (e *RunEngine) appendResumeSucceeded(runID string) error {
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

func completedFormationsFromEvents(events []RunEvent) map[string]bool {
	completed := map[string]bool{}
	for _, event := range events {
		if event.Type == RunEventNodeOutput && event.NodeID != "" {
			completed[event.NodeID] = true
		}
	}
	return completed
}

func terminalPassReached(events []RunEvent) bool {
	return terminalPassReachedOnBoard(nil, events)
}

func terminalPassReachedOnBoard(board *BoardDocument, events []RunEvent) bool {
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if event.Type != RunEventGateVerdict {
			continue
		}
		if stringFromEventData(event, "routePort") != "pass" {
			return false
		}
		gateID := event.GateID
		if gateID == "" {
			gateID = event.NodeID
		}
		if board != nil {
			return len(gateVerdictRoutes(board, event, gateID, "pass")) == 0
		}
		return len(stringSliceFromAny(event.Data["routedEdges"])) == 0
	}
	return false
}

func latestGateVerdictAllowsGraphCompletion(events []RunEvent) bool {
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if event.Type != RunEventGateVerdict {
			continue
		}
		return stringFromEventData(event, "routePort") == "pass"
	}
	return true
}

func runGraphComplete(board *BoardDocument, startNodeID string, completed map[string]bool) bool {
	formationIDs := map[string]bool{}
	for _, formation := range board.Formations {
		formationIDs[formation.ID] = true
	}
	visited := map[string]bool{}
	queue := []string{startNodeID}
	reachableFormations := map[string]bool{}
	for len(queue) > 0 {
		nodeID := queue[0]
		queue = queue[1:]
		if visited[nodeID] {
			continue
		}
		visited[nodeID] = true
		if formationIDs[nodeID] {
			reachableFormations[nodeID] = true
		}
		for _, connection := range outgoingConnections(board.Connections, nodeID) {
			toNode, _ := endpointParts(connection.To)
			if toNode != "" && !visited[toNode] {
				queue = append(queue, toNode)
			}
		}
	}
	if len(reachableFormations) == 0 {
		return false
	}
	for id := range reachableFormations {
		if !completed[id] {
			return false
		}
	}
	return true
}

func processedGateInputRefs(board *BoardDocument, events []RunEvent) map[string]bool {
	processed := map[string]bool{}
	outputOrdinals := map[string]int{}
	for _, event := range events {
		if event.Type == RunEventNodeOutput {
			advanceOutputOrdinals(board, outputOrdinals, event)
			continue
		}
		switch event.Type {
		case RunEventGateEvaluating, RunEventGateVerdict, RunEventHumanInputRequested:
		default:
			continue
		}
		if event.Data == nil {
			continue
		}
		input := runInputRefFromAny(event.Data["inputRef"])
		if input.EdgeID != "" {
			processed[gateInputReplayKey(input.EdgeID, gateInputOutputSeq(input, outputOrdinals))] = true
		}
	}
	return processed
}

func advanceOutputOrdinals(board *BoardDocument, outputOrdinals map[string]int, event RunEvent) {
	if board == nil || event.Type != RunEventNodeOutput {
		return
	}
	for _, connection := range outgoingConnections(board.Connections, event.NodeID) {
		_, fromPort := endpointParts(connection.From)
		if _, ok := outputPayloadForPortFromEvent(event, fromPort); ok {
			outputOrdinals[connection.ID]++
		}
	}
}

func gateInputOutputSeq(input RunInputRef, outputOrdinals map[string]int) int {
	if input.OutputSeq > 0 {
		return input.OutputSeq
	}
	return outputOrdinals[input.EdgeID]
}

func gateInputReplayKey(edgeID string, outputSeq int) string {
	return fmt.Sprintf("%s#%d", edgeID, outputSeq)
}

func (e *RunEngine) replayNodeOutputToReady(runID string, board *BoardDocument, gates map[string]GateNode, event RunEvent, limits RunLimits, processedGateInputs map[string]bool, outputOrdinals map[string]int, ready map[string]map[string]RunInputRef, queued map[string]bool, queue *[]string) error {
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
		outputOrdinals[connection.ID]++
		outputSeq := outputOrdinals[connection.ID]
		input := runInputRefForConnection(runID, connection, payload)
		input.OutputSeq = outputSeq
		if _, ok := gates[toNode]; ok {
			key := gateInputReplayKey(connection.ID, outputSeq)
			if processedGateInputs[key] {
				continue
			}
			processedGateInputs[key] = true
			if err := e.deliverConnection(runID, board, gates, connection, input, limits, ready, queued, queue); err != nil {
				return err
			}
			continue
		}
		formation, ok := findFormation(board.Formations, toNode)
		if !ok {
			continue
		}
		if ready[toNode] == nil {
			ready[toNode] = map[string]RunInputRef{}
		}
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
			// A gate verdict newer than the target node's last output is a pushback
			// route (for example gate:fail -> work). Clear the completed mark so a
			// resume re-dispatches the stale target, but do not rerun a pushback that
			// was already serviced by a later node_output.
			if toNode, _ := endpointParts(route.To); toNode != "" {
				if idx, ok := lastOutputIdx[toNode]; ok && i > idx {
					delete(completed, toNode)
				}
			}
			nextInput := input
			if routePort == "fail" {
				nextInput.EdgeID = route.ID
				nextInput.FromNodeID = gateID
				nextInput.FromPortID = routePort
				nextInput.Ref = fmt.Sprintf("ledger://%s/%s", runID, route.ID)
			}
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

func (e *RunEngine) readRunBoard(runID string) (*BoardDocument, error) {
	ledger, err := e.store.openRunLedger(runID, false)
	if err != nil {
		return nil, fmt.Errorf("%w: open run ledger: %v", ErrRunLedgerInvalid, err)
	}
	defer ledger.close()
	events, err := classifyAndReadRunEvents(ledger.file, runID)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, ErrRunLedgerInvalid
	}
	return e.store.readRunSnapshot(events[0], runID, ledger)
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
	return e.evaluateGateKinds(runID, board, gates, gate, input, limits, ready, queued, queue, nil)
}

type durableGateKindResult struct {
	Seq    int
	Result GateEvaluationResult
}

func (e *RunEngine) evaluateGateKinds(runID string, board *BoardDocument, gates map[string]GateNode, gate GateNode, input RunInputRef, limits RunLimits, ready map[string]map[string]RunInputRef, queued map[string]bool, queue *[]string, prior map[string]durableGateKindResult) error {
	result := GateEvaluationResult{
		Verdict:        "pass",
		PerKind:        map[string]string{},
		KindResultSeqs: map[string]int{},
	}
	if hasGateKind(gate.Kinds, "code") {
		codeResult := GateEvaluationResult{}
		codeResultSeq := 0
		if durable, ok := prior["code"]; ok {
			codeResult = durable.Result
			codeResultSeq = durable.Seq
		} else {
			var err error
			codeResult, err = e.evaluateCodeGateResult(GateEvaluation{
				RunID:        runID,
				GateID:       gate.ID,
				Title:        gate.Title,
				Kinds:        []string{"code"},
				Criterion:    gate.Criterion,
				Check:        gate.Check,
				CheckVersion: gate.CheckVersion,
				CheckValue:   gate.CheckValue,
				Input:        input,
			})
			if err != nil {
				return err
			}
			codeResult.Verdict = normalizeGateVerdict(codeResult.Verdict)
			codeResult.PerKind = map[string]string{"code": codeResult.Verdict}
			codeResultSeq, err = e.appendGateKindResult(runID, gate, "code", input, codeResult)
			if err != nil {
				return err
			}
		}
		codeVerdict := normalizeGateVerdict(codeResult.Verdict)
		codeResult.Verdict = codeVerdict
		codeResult.PerKind = map[string]string{"code": codeVerdict}
		codeResult.KindResultSeqs = map[string]int{"code": codeResultSeq}
		codeResult.CodeVerdict = codeVerdict
		codeResult.CodeReason = codeResult.Reason
		result = codeResult
		result.PerKind = map[string]string{"code": codeVerdict}
		result.KindResultSeqs = map[string]int{"code": codeResultSeq}
		if codeVerdict == "fail" {
			markLaterGateKindsNotRun(gate.Kinds, result.PerKind, "code")
			return e.routeGateEvaluation(runID, board, gates, gate, input, "fail", result, limits, ready, queued, queue)
		}
	}
	if hasGateKind(gate.Kinds, "formation") {
		formationResult := GateEvaluationResult{}
		formationResultSeq := 0
		if durable, ok := prior["formation"]; ok {
			formationResult = durable.Result
			formationResultSeq = durable.Seq
		} else {
			text, err := e.runJudgeChain(board, GateEvaluation{
				RunID:     runID,
				GateID:    gate.ID,
				Title:     gate.Title,
				Kinds:     []string{"formation"},
				Criterion: gate.Criterion,
				Input:     input,
			}, judgeChainForGate(board, gate.ID), limits)
			if err != nil {
				return err
			}
			formationResult = GateEvaluationResult{
				Verdict: normalizeGateVerdict(strings.TrimSpace(text)),
				Reason:  "judge chain",
				PerKind: map[string]string{},
			}
			formationResultSeq, err = e.appendGateKindResult(runID, gate, "formation", input, formationResult)
			if err != nil {
				return err
			}
		}
		formationResult.PerKind["formation"] = formationResult.Verdict
		result.Verdict = formationResult.Verdict
		result.Reason = formationResult.Reason
		result.PerKind["formation"] = formationResult.Verdict
		result.KindResultSeqs["formation"] = formationResultSeq
		if formationResult.Verdict == "fail" {
			markLaterGateKindsNotRun(gate.Kinds, result.PerKind, "formation")
			return e.routeGateEvaluation(runID, board, gates, gate, input, "fail", result, limits, ready, queued, queue)
		}
	}
	if hasGateKind(gate.Kinds, "human") {
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
				"codeVerdict":    result.CodeVerdict,
				"codeReason":     result.CodeReason,
				"codePerKind":    result.PerKind,
				"codeResultSeq":  result.KindResultSeqs["code"],
				"kindResultSeqs": result.KindResultSeqs,
				"evidence":       result.Evidence,
				"resultEncoding": result.ResultEncoding,
				"resultSha256":   result.ResultSHA256,
				"gateBindingId":  result.GateBindingID,
				"timeoutSeconds": 0,
			},
		}); err != nil {
			return err
		}
		return errRunStopped
	}
	return e.routeGateEvaluation(runID, board, gates, gate, input, "pass", result, limits, ready, queued, queue)
}

func (e *RunEngine) appendGateKindResult(runID string, gate GateNode, kind string, input RunInputRef, result GateEvaluationResult) (int, error) {
	data := map[string]any{
		"kind":           kind,
		"verdict":        normalizeGateVerdict(result.Verdict),
		"reason":         result.Reason,
		"evidence":       result.Evidence,
		"resultEncoding": result.ResultEncoding,
		"resultSha256":   result.ResultSHA256,
		"gateBindingId":  result.GateBindingID,
		"inputRef":       input,
	}
	if kind == "code" {
		binding, err := e.store.readRunGateBinding(runID, gate.ID)
		if err != nil {
			return 0, err
		}
		data["inputSha256"] = codeGateSHA256(input.Text)
		data["profileId"] = binding.ProfileID
		data["profileVersion"] = binding.ProfileVersion
		data["profileSha256"] = binding.ProfileSHA256
		data["evaluatorBundleSha256"] = binding.EvaluatorBundleSHA256
		data["parametersSha256"] = binding.ParametersSHA256
		data["policySha256"] = binding.PolicySHA256
		data["determinismPolicySha256"] = binding.DeterminismPolicySHA256
		data["maxInputBytes"] = binding.MaxInputBytes
		data["maxResultBytes"] = binding.MaxResultBytes
		data["maxOperations"] = binding.MaxOperations
	}
	if err := e.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventGateKindResult,
		GateID: gate.ID,
		NodeID: gate.ID,
		Data:   data,
	}); err != nil {
		return 0, err
	}
	events, err := e.store.ReadRunEvents(runID)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 || events[len(events)-1].Type != RunEventGateKindResult {
		return 0, ErrRunLedgerInvalid
	}
	return events[len(events)-1].Seq, nil
}

func markLaterGateKindsNotRun(kinds []string, perKind map[string]string, failedKind string) {
	afterFailure := false
	for _, kind := range []string{"code", "formation", "human"} {
		if kind == failedKind {
			afterFailure = true
			continue
		}
		if afterFailure && hasGateKind(kinds, kind) {
			perKind[kind] = "not_run"
		}
	}
}

func (e *RunEngine) routeGateEvaluation(runID string, board *BoardDocument, gates map[string]GateNode, gate GateNode, input RunInputRef, verdict string, result GateEvaluationResult, limits RunLimits, ready map[string]map[string]RunInputRef, queued map[string]bool, queue *[]string) error {
	routePort := verdict
	routes := outgoingConnectionsFromPort(board.Connections, gate.ID, routePort)
	if verdict == "fail" && len(routes) == 0 {
		routePort = "none"
	}
	if err := e.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventGateVerdict,
		GateID: gate.ID,
		NodeID: gate.ID,
		Data: map[string]any{
			"verdict":        verdict,
			"perKind":        result.PerKind,
			"kindResultSeqs": result.KindResultSeqs,
			"codeResultSeq":  result.KindResultSeqs["code"],
			"codeVerdict":    result.CodeVerdict,
			"codeReason":     result.CodeReason,
			"routePort":      routePort,
			"routedEdges":    connectionIDs(routes),
			"reason":         result.Reason,
			"evidence":       result.Evidence,
			"resultEncoding": result.ResultEncoding,
			"resultSha256":   result.ResultSHA256,
			"gateBindingId":  result.GateBindingID,
			"inputRef":       input,
		},
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
		if verdict == "fail" {
			nextInput.EdgeID = route.ID
			nextInput.FromNodeID = gate.ID
			nextInput.FromPortID = routePort
			nextInput.Ref = fmt.Sprintf("ledger://%s/%s", runID, route.ID)
		}
		if err := e.deliverConnection(runID, board, gates, route, nextInput, limits, ready, queued, queue); err != nil {
			return err
		}
	}
	return nil
}

func (e *RunEngine) routeGateVerdict(runID string, board *BoardDocument, gate GateNode, input RunInputRef, verdict, reason string, limits RunLimits, requestEvent RunEvent) (*RunStatusProjection, error) {
	routes := outgoingConnectionsFromPort(board.Connections, gate.ID, verdict)
	routePort := verdict
	if verdict == "fail" && len(routes) == 0 {
		routePort = "none"
	}
	result := gateResultFromHumanRequest(requestEvent, verdict, reason)
	if err := e.store.AppendRunEvent(runID, RunEvent{
		Type:   RunEventGateVerdict,
		GateID: gate.ID,
		NodeID: gate.ID,
		Data: map[string]any{
			"verdict":        verdict,
			"perKind":        result.PerKind,
			"kindResultSeqs": result.KindResultSeqs,
			"codeResultSeq":  result.KindResultSeqs["code"],
			"codeVerdict":    result.CodeVerdict,
			"codeReason":     result.CodeReason,
			"routePort":      routePort,
			"routedEdges":    connectionIDs(routes),
			"reason":         reason,
			"evidence":       result.Evidence,
			"resultEncoding": result.ResultEncoding,
			"resultSha256":   result.ResultSHA256,
			"gateBindingId":  result.GateBindingID,
			"inputRef":       input,
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

func (e *RunEngine) evaluateCodeGateResult(req GateEvaluation) (GateEvaluationResult, error) {
	if e.gateEvaluator == nil {
		if err := e.appendGateErrorAndBlock(req.RunID, req.GateID, "missing_gate_evaluator", "gate evaluator unavailable", "gate", "gate evaluator unavailable"); err != nil {
			return GateEvaluationResult{}, err
		}
		return GateEvaluationResult{}, errRunStopped
	}
	if strings.TrimSpace(req.Check) != "" || strings.TrimSpace(req.CheckVersion) != "" {
		binding, err := e.store.readRunGateBinding(req.RunID, req.GateID)
		if err != nil {
			if blockErr := e.appendGateErrorAndBlock(req.RunID, req.GateID, "gate_evaluator_error", err.Error(), "gate", "gate evaluator error"); blockErr != nil {
				return GateEvaluationResult{}, blockErr
			}
			return GateEvaluationResult{}, errRunStopped
		}
		req.Binding = binding
	}
	result, err := callGateEvaluator(e.gateEvaluator, req)
	if err != nil {
		if blockErr := e.appendGateErrorAndBlock(req.RunID, req.GateID, "gate_evaluator_error", err.Error(), "gate", "gate evaluator error"); blockErr != nil {
			return GateEvaluationResult{}, blockErr
		}
		return GateEvaluationResult{}, errRunStopped
	}
	if err := validateCodeGateEvaluationResult(req, result); err != nil {
		if blockErr := e.appendGateErrorAndBlock(req.RunID, req.GateID, "gate_evaluator_error", err.Error(), "gate", "gate evaluator error"); blockErr != nil {
			return GateEvaluationResult{}, blockErr
		}
		return GateEvaluationResult{}, errRunStopped
	}
	return result, nil
}

func validateCodeGateEvaluationResult(req GateEvaluation, result GateEvaluationResult) error {
	if result.Verdict != "pass" && result.Verdict != "fail" {
		return fmt.Errorf("gate %q evaluator returned invalid verdict %q", req.GateID, result.Verdict)
	}
	if req.Binding == nil || result.GateBindingID != req.Binding.GateBindingID {
		return fmt.Errorf("gate %q evaluator result binding mismatch", req.GateID)
	}
	if result.ResultEncoding != CodeGateResultEncoding {
		return fmt.Errorf("gate %q evaluator result encoding mismatch", req.GateID)
	}
	canonical, err := canonicalCodeGateResult(result.Verdict, result.Reason, result.Evidence)
	if err != nil {
		return fmt.Errorf("gate %q canonical result: %w", req.GateID, err)
	}
	if result.CanonicalResult != canonical {
		return fmt.Errorf("gate %q evaluator canonical result mismatch", req.GateID)
	}
	if result.ResultSHA256 != codeGateSHA256(canonical) {
		return fmt.Errorf("gate %q evaluator result hash mismatch", req.GateID)
	}
	return nil
}

func gateResultFromHumanRequest(request RunEvent, verdict, reason string) GateEvaluationResult {
	perKind := stringMapFromRunEventData(request.Data["codePerKind"])
	perKind["human"] = verdict
	return GateEvaluationResult{
		Verdict:        verdict,
		Reason:         reason,
		Evidence:       gateEvidenceRefsFromRunEventData(request.Data["evidence"]),
		PerKind:        perKind,
		KindResultSeqs: intMapFromRunEventData(request.Data["kindResultSeqs"]),
		CodeVerdict:    stringFromAny(request.Data["codeVerdict"]),
		CodeReason:     stringFromAny(request.Data["codeReason"]),
		ResultEncoding: stringFromAny(request.Data["resultEncoding"]),
		ResultSHA256:   stringFromAny(request.Data["resultSha256"]),
		GateBindingID:  stringFromAny(request.Data["gateBindingId"]),
	}
}

func intMapFromRunEventData(value any) map[string]int {
	result := map[string]int{}
	switch raw := value.(type) {
	case map[string]int:
		for key, item := range raw {
			result[key] = item
		}
	case map[string]any:
		for key, item := range raw {
			result[key] = intFromRunEventData(item)
		}
	}
	return result
}

func stringMapFromRunEventData(value any) map[string]string {
	result := map[string]string{}
	switch raw := value.(type) {
	case map[string]string:
		for key, item := range raw {
			result[key] = item
		}
	case map[string]any:
		for key, item := range raw {
			if text, ok := item.(string); ok {
				result[key] = text
			}
		}
	}
	return result
}

func gateEvidenceRefsFromRunEventData(value any) []GateEvidenceRef {
	switch raw := value.(type) {
	case []GateEvidenceRef:
		return append([]GateEvidenceRef(nil), raw...)
	case []any:
		result := make([]GateEvidenceRef, 0, len(raw))
		for _, item := range raw {
			fields, ok := item.(map[string]any)
			if !ok {
				continue
			}
			result = append(result, GateEvidenceRef{
				Kind: stringFromAny(fields["kind"]),
				Text: stringFromAny(fields["text"]),
			})
		}
		return result
	default:
		return nil
	}
}

func callGateEvaluator(evaluator GateEvaluator, req GateEvaluation) (result GateEvaluationResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = GateEvaluationResult{}
			err = fmt.Errorf("gate evaluator panic: %v", recovered)
		}
	}()
	return evaluator.EvaluateGate(req)
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

func normalizeGateVerdict(verdict string) string {
	if verdict == "fail" {
		return "fail"
	}
	return "pass"
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
