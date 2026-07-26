package formations

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var (
	ErrRunFinal            = errors.New("formations run is final")
	ErrRunLedgerInvalid    = errors.New("formations run ledger invalid")
	ErrRunResumeNotAllowed = errors.New("formations run resume is not allowed")
	ErrRunEpochBlocked     = errors.New("formations run epoch is blocked")
)

const (
	RunEventStarted              = "run_started"
	RunEventResumed              = "run_resumed"
	RunEventNodeWaiting          = "node_waiting"
	RunEventNodeStarted          = "node_started"
	RunEventOrchestrationTeam    = "orchestration_team"
	RunEventWorkerObservation    = "worker_observation"
	RunEventPeerPlane            = "peer_plane"
	RunEventSlotDispatch         = "slot_dispatch"
	RunEventAdapterSend          = "adapter_send"
	RunEventSlotResult           = "slot_result"
	RunEventNodeOutput           = "node_output"
	RunEventGateEvaluating       = "gate_evaluating"
	RunEventGateVerdict          = "gate_verdict"
	RunEventVerificationVerdict  = "verification_verdict"
	RunEventEscalationRaised     = "escalation_raised"
	RunEventHumanInputRequested  = "human_input_requested"
	RunEventHumanVerdictRecorded = "human_verdict_recorded"
	RunEventError                = "error"
	RunEventBlocked              = "run_blocked"
	RunEventCanceled             = "run_canceled"
	RunEventFailed               = "run_failed"
	RunEventSucceeded            = "run_succeeded"

	RunStatusRunning   = "running"
	RunStatusBlocked   = "blocked"
	RunStatusCanceled  = "canceled"
	RunStatusFailed    = "failed"
	RunStatusSucceeded = "succeeded"
)

type RunStartRequest struct {
	MissionID         string
	Actor             string
	ExpectedBoardETag string
	ExpectedBoardRev  int
	Personas          *PersonaStore
	Limits            RunLimits
}

type RunResumeRequest struct {
	Actor  string
	Mode   string
	Reason string
}

type RunLimits struct {
	MaxDispatch      int  `json:"maxDispatch"`
	MaxAttempts      int  `json:"maxAttempts,omitempty"`
	WallClockSeconds int  `json:"wallClockSeconds"`
	Redact           bool `json:"redact"`
}

type RunStartResult struct {
	RunID                string `json:"runId"`
	BoardSlug            string `json:"boardSlug"`
	LedgerPath           string `json:"ledgerPath"`
	SnapshotPath         string `json:"snapshot"`
	BindingsSnapshotPath string `json:"bindingsSnapshot"`
}

type RunEvent struct {
	Timestamp string         `json:"ts,omitempty"`
	RunID     string         `json:"runId,omitempty"`
	Seq       int            `json:"seq,omitempty"`
	Type      string         `json:"type"`
	Actor     string         `json:"actor,omitempty"`
	BoardID   string         `json:"boardId,omitempty"`
	BoardRev  int            `json:"boardRev,omitempty"`
	MissionID string         `json:"missionId,omitempty"`
	BeadID    string         `json:"beadId,omitempty"`
	NodeID    string         `json:"nodeId,omitempty"`
	SlotID    string         `json:"slotId,omitempty"`
	GateID    string         `json:"gateId,omitempty"`
	EdgeID    string         `json:"edgeId,omitempty"`
	Epoch     int            `json:"epoch,omitempty"`
	Attempt   int            `json:"attempt,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

type RunStatusProjection struct {
	RunID         string `json:"runId"`
	Status        string `json:"status"`
	Final         bool   `json:"final"`
	BoardSlug     string `json:"boardSlug"`
	BoardID       string `json:"boardId"`
	BoardRev      int    `json:"boardRev"`
	MissionID     string `json:"missionId"`
	BeadID        string `json:"beadId"`
	Epoch         int    `json:"epoch"`
	EventCount    int    `json:"eventCount"`
	ResumeAllowed bool   `json:"resumeAllowed"`
}

type RunListFilter struct {
	BoardSlug string
}

type RunNodeReport struct {
	RunID      string                            `json:"runId"`
	NodeID     string                            `json:"nodeId"`
	Status     string                            `json:"status"`
	ReportRef  string                            `json:"reportRef,omitempty"`
	Text       string                            `json:"text,omitempty"`
	Outputs    map[string]FormationOutputPayload `json:"outputs,omitempty"`
	OutputSeq  int                               `json:"outputSeq,omitempty"`
	EventCount int                               `json:"eventCount"`
	Brief      FormationBrief                    `json:"brief,omitempty"`
}

type runBinding struct {
	NodeID      string
	SlotID      string
	AgentID     string
	Harness     string
	SessionStem string
	CardPath    string
	CardHash    string
	Launch      string
	Source      string
}

func (s *Store) StartRun(slug string, req RunStartRequest) (*RunStartResult, error) {
	if err := validateSlug(slug); err != nil {
		return nil, err
	}
	boardPath := s.BoardPath(slug)
	definition, err := s.openBoardDefinition(slug, false)
	if err != nil {
		return nil, err
	}
	defer definition.close()
	boardRaw, err := definition.readBytes()
	if err != nil {
		return nil, err
	}
	board, err := parseBoard(boardRaw)
	if err != nil {
		return nil, err
	}
	if req.ExpectedBoardETag != "" && req.ExpectedBoardETag != board.ETag {
		return nil, ErrConflict
	}
	if req.ExpectedBoardRev != 0 && req.ExpectedBoardRev != board.Rev {
		return nil, ErrConflict
	}
	mission, ok := findMission(board, req.MissionID)
	if !ok {
		return nil, fmt.Errorf("%w: mission %q", ErrNotFound, req.MissionID)
	}
	if err := preflightMissionDefinition(board, mission.ID); err != nil {
		return nil, err
	}
	if err := s.RequireRuntimeAuthority(); err != nil {
		return nil, err
	}
	bindings, err := resolveRunBindings(board, req.Personas)
	if err != nil {
		return nil, err
	}

	runID := newPrefixedID("run")
	ledgerPath := runArtifactPath(slug, runID, ".ndjson")
	snapshotPath := runArtifactPath(slug, runID, ".snapshot.toml")
	bindingsPath := runArtifactPath(slug, runID, ".bindings.toml")

	if int64(len(boardRaw)) > runtimeAuthorityMaxRecordBytes {
		return nil, fmt.Errorf("%w: run snapshot exceeds byte limit", ErrRunLedgerInvalid)
	}
	runDirectory, err := s.openRunArtifactDirectory(slug, true)
	if err != nil {
		return nil, err
	}
	defer runDirectory.close()
	if err := writeRunArtifactExclusiveAt(runDirectory, runID+".snapshot.toml", boardRaw); err != nil {
		return nil, err
	}
	if err := writeRunArtifactExclusiveAt(runDirectory, runID+".bindings.toml", []byte(renderRunBindings(runID, board, mission, bindings))); err != nil {
		return nil, err
	}

	result := &RunStartResult{
		RunID:                runID,
		BoardSlug:            slug,
		LedgerPath:           ledgerPath,
		SnapshotPath:         snapshotPath,
		BindingsSnapshotPath: bindingsPath,
	}
	event := RunEvent{
		Timestamp: s.now().Format(time.RFC3339Nano),
		RunID:     runID,
		Seq:       1,
		Type:      RunEventStarted,
		Actor:     defaultRunActor(req.Actor),
		BoardID:   board.ID,
		BoardRev:  board.Rev,
		MissionID: mission.ID,
		BeadID:    mission.BeadID,
		Epoch:     0,
		Attempt:   0,
		Data: map[string]any{
			"boardSlug":        slug,
			"boardPath":        filepath.ToSlash(boardPath),
			"boardRev":         board.Rev,
			"snapshot":         snapshotPath,
			"bindingsSnapshot": bindingsPath,
			"missionId":        mission.ID,
			"beadId":           mission.BeadID,
			"objective":        mission.Goal,
			"limits":           req.Limits,
		},
	}
	if err := writeInitialRunEventAt(runDirectory, runID, event); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) AppendRunEvent(runID string, event RunEvent) error {
	if err := s.RequireRuntimeAuthority(); err != nil {
		return err
	}
	_, err := s.appendRunEventWithSnapshot(runID, event)
	return err
}

func (s *Store) appendRunEventWithSnapshot(runID string, event RunEvent) (*BoardDocument, error) {
	if err := s.RequireRuntimeAuthority(); err != nil {
		return nil, err
	}
	if event.Type == RunEventVerificationVerdict {
		return nil, fmt.Errorf("%w: new verification_verdict events are retired; use an explicit Gate", ErrLegacyInlineVerificationRequiresMigration)
	}
	ledger, err := s.openRunLedger(runID, true)
	if err != nil {
		return nil, err
	}
	defer ledger.close()
	var snapshot *BoardDocument
	err = ledger.withLock(func() error {
		events, err := classifyAndReadRunEvents(ledger.file, runID)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return ErrRunLedgerInvalid
		}
		if isFinalRunEvent(events[len(events)-1].Type) {
			return ErrRunFinal
		}
		first := events[0]
		last := events[len(events)-1]
		if err := s.validateRunSnapshotIdentity(first, runID, ledger); err != nil {
			return err
		}
		var validatedSnapshot *BoardDocument
		if event.Type != RunEventCanceled && event.Type != RunEventFailed {
			validatedSnapshot, err = s.readRunSnapshot(first, runID, ledger)
			if err != nil {
				return err
			}
			if err := rejectLegacyScriptGateForRun(validatedSnapshot, first, &event); err != nil {
				return err
			}
			if err := rejectLegacyInlineVerification(validatedSnapshot); err != nil {
				return err
			}
		}
		if last.Type == RunEventBlocked {
			if event.Type != RunEventResumed && event.Type != RunEventCanceled && event.Type != RunEventFailed {
				return ErrRunEpochBlocked
			}
			if event.Type == RunEventResumed {
				if !boolFromEventData(last, "resumeAllowed") {
					return ErrRunResumeNotAllowed
				}
				if event.Epoch == 0 {
					event.Epoch = last.Epoch + 1
				}
			}
		} else if event.Type == RunEventResumed {
			return ErrRunResumeNotAllowed
		}
		event.RunID = runID
		event.Seq = last.Seq + 1
		if event.Timestamp == "" {
			event.Timestamp = s.now().Format(time.RFC3339Nano)
		}
		if event.Actor == "" {
			event.Actor = first.Actor
		}
		if event.BoardID == "" {
			event.BoardID = first.BoardID
		}
		if event.BoardRev == 0 {
			event.BoardRev = first.BoardRev
		}
		if event.MissionID == "" {
			event.MissionID = first.MissionID
		}
		if event.BeadID == "" {
			event.BeadID = first.BeadID
		}
		if event.Epoch == 0 && last.Epoch != 0 {
			event.Epoch = last.Epoch
		}
		if err := appendRunEventToFile(ledger.file, ledger.directory.file, event); err != nil {
			return err
		}
		snapshot = validatedSnapshot
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (s *Store) ResumeRun(runID string, req RunResumeRequest) (*RunStatusProjection, error) {
	if err := s.RequireRuntimeAuthority(); err != nil {
		return nil, err
	}
	status, _, err := s.resumeRunWithSnapshot(runID, req)
	return status, err
}

func (s *Store) resumeRunWithSnapshot(runID string, req RunResumeRequest) (*RunStatusProjection, *BoardDocument, error) {
	if err := s.RequireRuntimeAuthority(); err != nil {
		return nil, nil, err
	}
	ledger, err := s.openRunLedger(runID, true)
	if err != nil {
		return nil, nil, err
	}
	defer ledger.close()
	var snapshot *BoardDocument
	if err := ledger.withLock(func() error {
		events, err := classifyAndReadRunEvents(ledger.file, runID)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return ErrRunLedgerInvalid
		}
		if isFinalRunEvent(events[len(events)-1].Type) {
			return ErrRunFinal
		}
		first := events[0]
		last := events[len(events)-1]
		runSnapshot, err := s.readRunSnapshot(first, runID, ledger)
		if err != nil {
			return err
		}
		if err := rejectLegacyScriptGateForRun(runSnapshot, first, nil); err != nil {
			return err
		}
		if err := rejectLegacyInlineVerification(runSnapshot); err != nil {
			return err
		}
		if last.Type != RunEventBlocked || !boolFromEventData(last, "resumeAllowed") {
			return ErrRunResumeNotAllowed
		}
		actor := defaultRunActor(req.Actor)
		mode := strings.TrimSpace(req.Mode)
		if mode == "" {
			mode = "reattach"
		}
		data := map[string]any{
			"resumedFromSeq": last.Seq,
			"resumedBy":      actor,
			"resumeMode":     mode,
		}
		if reason := strings.TrimSpace(req.Reason); reason != "" {
			data["reason"] = reason
		}
		if openDispatches, ok := last.Data["openDispatches"]; ok {
			data["openDispatches"] = openDispatches
		}
		event := RunEvent{
			Timestamp: s.now().Format(time.RFC3339Nano),
			RunID:     runID,
			Seq:       last.Seq + 1,
			Type:      RunEventResumed,
			Actor:     actor,
			BoardID:   first.BoardID,
			BoardRev:  first.BoardRev,
			MissionID: first.MissionID,
			BeadID:    first.BeadID,
			Epoch:     last.Epoch + 1,
			Data:      data,
		}
		if err := appendRunEventToFile(ledger.file, ledger.directory.file, event); err != nil {
			return err
		}
		snapshot = runSnapshot
		return nil
	}); err != nil {
		return nil, nil, err
	}
	status, err := s.ProjectRun(runID)
	if err != nil {
		return nil, nil, err
	}
	return status, snapshot, nil
}

func (s *Store) validateRunSnapshotIdentity(started RunEvent, expectedRunID string, ledger *runLedgerHandle) error {
	snapshotPath := stringFromEventData(started, "snapshot")
	bindingsSnapshotPath := stringFromEventData(started, "bindingsSnapshot")
	boardSlug := stringFromEventData(started, "boardSlug")
	if ledger == nil || ledger.directory == nil || started.Type != RunEventStarted || validateSlug(boardSlug) != nil || started.RunID != expectedRunID || ledger.runID != expectedRunID {
		return ErrRunLedgerInvalid
	}
	expectedSnapshotPath := runArtifactPath(boardSlug, expectedRunID, ".snapshot.toml")
	expectedBindingsSnapshotPath := runArtifactPath(boardSlug, expectedRunID, ".bindings.toml")
	if ledger.directory.slug != boardSlug || snapshotPath != expectedSnapshotPath || bindingsSnapshotPath != expectedBindingsSnapshotPath {
		return ErrRunLedgerInvalid
	}
	return nil
}

func (s *Store) readRunSnapshot(started RunEvent, expectedRunID string, ledger *runLedgerHandle) (*BoardDocument, error) {
	if err := s.validateRunSnapshotIdentity(started, expectedRunID, ledger); err != nil {
		return nil, err
	}
	snapshotRaw, err := readRunArtifactAt(ledger.directory, expectedRunID+".snapshot.toml", runtimeAuthorityMaxRecordBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: snapshot read failed: %v", ErrRunLedgerInvalid, err)
	}
	board, err := parseBoard(snapshotRaw)
	if err != nil {
		return nil, fmt.Errorf("%w: snapshot parse failed: %v", ErrRunLedgerInvalid, err)
	}
	boardSlug := stringFromEventData(started, "boardSlug")
	if board.ID != started.BoardID || board.Slug != boardSlug || board.Rev != started.BoardRev {
		return nil, ErrRunLedgerInvalid
	}
	return board, nil
}

func (s *Store) ProjectRun(runID string) (*RunStatusProjection, error) {
	ledger, err := s.openRunLedger(runID, false)
	if err != nil {
		return nil, err
	}
	defer ledger.close()
	events, err := classifyAndReadRunEvents(ledger.file, runID)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 || events[0].Seq != 1 || events[0].Type != RunEventStarted {
		return nil, ErrRunLedgerInvalid
	}
	status := &RunStatusProjection{
		RunID:     runID,
		Status:    RunStatusRunning,
		BoardSlug: stringFromEventData(events[0], "boardSlug"),
		BoardID:   events[0].BoardID,
		BoardRev:  events[0].BoardRev,
		MissionID: events[0].MissionID,
		BeadID:    events[0].BeadID,
	}
	for i, event := range events {
		if event.Seq != i+1 {
			return nil, ErrRunLedgerInvalid
		}
		status.EventCount++
		status.Epoch = event.Epoch
		switch event.Type {
		case RunEventStarted, RunEventResumed:
			status.Status = RunStatusRunning
			status.Final = false
			status.ResumeAllowed = false
		case RunEventBlocked:
			status.Status = RunStatusBlocked
			status.Final = false
			status.ResumeAllowed = boolFromEventData(event, "resumeAllowed")
		case RunEventCanceled:
			status.Status = RunStatusCanceled
			status.Final = true
			status.ResumeAllowed = false
		case RunEventFailed:
			status.Status = RunStatusFailed
			status.Final = true
			status.ResumeAllowed = false
		case RunEventSucceeded:
			status.Status = RunStatusSucceeded
			status.Final = true
			status.ResumeAllowed = false
		}
	}
	// Honesty safety net: a run can only project succeeded when every reachable
	// required node reached a terminal state. If the ledger still shows a node
	// whose last lifecycle event is node_waiting, the success is a lie (e.g. a
	// stray/legacy run_succeeded over a starved join); project blocked instead so
	// CLI, API, and UI never report finished work that never ran.
	if status.Status == RunStatusSucceeded {
		if waiting := unresolvedWaitingNodes(events); len(waiting) > 0 {
			status.Status = RunStatusBlocked
			status.Final = false
			status.ResumeAllowed = false
		}
	}
	return status, nil
}

// unresolvedWaitingNodes returns node IDs whose last lifecycle event in the
// ledger is node_waiting — reached but never run to output. Order follows first
// appearance for deterministic reporting.
func unresolvedWaitingNodes(events []RunEvent) []string {
	last := map[string]string{}
	order := make([]string, 0)
	for _, event := range events {
		switch event.Type {
		case RunEventNodeWaiting, RunEventNodeStarted, RunEventNodeOutput:
			if _, seen := last[event.NodeID]; !seen {
				order = append(order, event.NodeID)
			}
			last[event.NodeID] = event.Type
		}
	}
	var waiting []string
	for _, id := range order {
		if last[id] == RunEventNodeWaiting {
			waiting = append(waiting, id)
		}
	}
	return waiting
}

func (s *Store) ReadRunEvents(runID string) ([]RunEvent, error) {
	ledger, err := s.openRunLedger(runID, false)
	if err != nil {
		return nil, err
	}
	defer ledger.close()
	return classifyAndReadRunEvents(ledger.file, runID)
}

func (s *Store) ListRuns(filter RunListFilter) ([]RunStatusProjection, error) {
	runIDs, err := s.listRunIDs()
	if err != nil {
		return nil, err
	}
	runs := make([]RunStatusProjection, 0, len(runIDs))
	for _, runID := range runIDs {
		status, err := s.ProjectRun(runID)
		if err != nil {
			return nil, err
		}
		if filter.BoardSlug != "" && status.BoardSlug != filter.BoardSlug {
			continue
		}
		runs = append(runs, *status)
	}
	return runs, nil
}

func (s *Store) ProjectRunNodeReport(runID, nodeID string) (*RunNodeReport, error) {
	events, err := s.ReadRunEvents(runID)
	if err != nil {
		return nil, err
	}
	report := &RunNodeReport{
		RunID:  runID,
		NodeID: nodeID,
		Status: RunStatusRunning,
	}
	for _, event := range events {
		if event.NodeID != nodeID {
			continue
		}
		report.EventCount++
		switch event.Type {
		case RunEventNodeStarted:
			report.Brief = formationBriefFromEventData(event.Data["brief"])
		case RunEventNodeOutput:
			report.OutputSeq = event.Seq
			report.Status = stringFromEventData(event, "status")
			report.ReportRef = stringFromEventData(event, "reportRef")
			report.Text = stringFromEventData(event, "text")
			report.Outputs = outputPayloadsFromAny(event.Data["outputs"])
			if report.Status == "" {
				report.Status = "done"
			}
		}
	}
	if report.EventCount == 0 {
		return nil, fmt.Errorf("%w: node %q", ErrNotFound, nodeID)
	}
	return report, nil
}

func writeInitialRunEvent(path string, event RunEvent) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	absolute = filepath.Clean(absolute)
	directory, err := openOrCreateAbsoluteDirectory(filepath.Dir(absolute))
	if err != nil {
		return fmt.Errorf("%w: open run directory: %v", ErrRunLedgerInvalid, err)
	}
	defer directory.close()
	runID := strings.TrimSuffix(filepath.Base(absolute), ".ndjson")
	return writeInitialRunEventAt(directory, runID, event)
}

func appendRunEventLine(path string, event RunEvent) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	absolute = filepath.Clean(absolute)
	directory, err := openOrCreateAbsoluteDirectory(filepath.Dir(absolute))
	if err != nil {
		return fmt.Errorf("%w: open run directory: %v", ErrRunLedgerInvalid, err)
	}
	defer directory.close()
	file, err := openRunArtifactFileAt(directory.file, filepath.Base(absolute), syscall.O_RDWR|syscall.O_APPEND, true)
	if err != nil {
		return fmt.Errorf("%w: open run ledger: %v", ErrRunLedgerInvalid, err)
	}
	defer file.Close()
	return appendRunEventToFile(file, directory.file, event)
}

func readRunEventsFile(path string) ([]RunEvent, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	absolute = filepath.Clean(absolute)
	directoryFile, err := openRuntimeAuthorityRoot(filepath.Dir(absolute))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: open run directory: %v", ErrRunLedgerInvalid, err)
	}
	defer directoryFile.Close()
	file, err := openRunArtifactFileAt(directoryFile, filepath.Base(absolute), syscall.O_RDONLY, false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: open run ledger: %v", ErrRunLedgerInvalid, err)
	}
	defer file.Close()
	runID := strings.TrimSuffix(filepath.Base(absolute), ".ndjson")
	return classifyAndReadRunEvents(file, runID)
}

func resolveRunBindings(board *BoardDocument, personas *PersonaStore) ([]runBinding, error) {
	var bindings []runBinding
	for _, formation := range board.Formations {
		for _, slot := range formation.Slots {
			if slot.AgentID == "" {
				continue
			}
			if personas == nil {
				return nil, fmt.Errorf("%w: persona store required for slot %q", ErrNotFound, slot.ID)
			}
			card, err := personas.ReadPersona(slot.AgentID)
			if err != nil {
				return nil, err
			}
			variant, err := card.SelectHarnessVariant(slot.Harness)
			if err != nil {
				return nil, err
			}
			bindings = append(bindings, runBinding{
				NodeID:      formation.ID,
				SlotID:      slot.ID,
				AgentID:     card.ID,
				Harness:     variant.ID,
				SessionStem: variant.SessionStem,
				CardPath:    filepath.ToSlash(personas.PersonaPath(card.ID)),
				CardHash:    etag([]byte(card.TOML)),
				Launch:      variant.Launch,
				Source:      variant.Source,
			})
		}
	}
	return bindings, nil
}

func renderRunBindings(runID string, board *BoardDocument, mission MissionNode, bindings []runBinding) string {
	var b strings.Builder
	b.WriteString("schema = 1\n")
	b.WriteString("runId = " + renderString(runID) + "\n")
	b.WriteString("boardId = " + renderString(board.ID) + "\n")
	b.WriteString("boardSlug = " + renderString(board.Slug) + "\n")
	b.WriteString("boardRev = " + renderInt(board.Rev) + "\n")
	b.WriteString("missionId = " + renderString(mission.ID) + "\n")
	b.WriteString("\n")
	for _, binding := range bindings {
		b.WriteString("[[binding]]\n")
		b.WriteString("nodeId = " + renderString(binding.NodeID) + "\n")
		b.WriteString("slotId = " + renderString(binding.SlotID) + "\n")
		b.WriteString("agentId = " + renderString(binding.AgentID) + "\n")
		b.WriteString("harness = " + renderString(binding.Harness) + "\n")
		b.WriteString("sessionStem = " + renderString(binding.SessionStem) + "\n")
		b.WriteString("cardPath = " + renderString(binding.CardPath) + "\n")
		b.WriteString("cardSha256 = " + renderString(binding.CardHash) + "\n")
		if binding.Launch != "" {
			b.WriteString("launch = " + renderString(binding.Launch) + "\n")
		}
		if binding.Source != "" {
			b.WriteString("source = " + renderString(binding.Source) + "\n")
		}
		b.WriteString("\n")
	}
	return b.String()
}

func findMission(board *BoardDocument, missionID string) (MissionNode, bool) {
	for _, mission := range board.Missions {
		if mission.ID == missionID {
			return mission, true
		}
	}
	return MissionNode{}, false
}

func runArtifactPath(slug, runID, suffix string) string {
	return filepath.ToSlash(filepath.Join(".formations", "runs", slug, runID+suffix))
}

func defaultRunActor(actor string) string {
	if actor == "" {
		return "agent:archon"
	}
	return actor
}

func isFinalRunEvent(eventType string) bool {
	return eventType == RunEventSucceeded || eventType == RunEventFailed || eventType == RunEventCanceled
}

func stringFromEventData(event RunEvent, key string) string {
	if event.Data == nil {
		return ""
	}
	value, _ := event.Data[key].(string)
	return value
}

func boolFromEventData(event RunEvent, key string) bool {
	if event.Data == nil {
		return false
	}
	value, _ := event.Data[key].(bool)
	return value
}

func formationBriefFromEventData(value any) FormationBrief {
	raw, ok := value.(map[string]any)
	if !ok {
		return FormationBrief{}
	}
	return FormationBrief{
		Goal:   stringFromAny(raw["goal"]),
		BeadID: stringFromAny(raw["beadId"]),
		Files:  stringSliceFromAny(raw["files"]),
		Links:  stringSliceFromAny(raw["links"]),
	}
}

func stringFromAny(value any) string {
	text, _ := value.(string)
	return text
}

func stringSliceFromAny(value any) []string {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		values := make([]string, 0, len(v))
		for _, item := range v {
			if text := stringFromAny(item); text != "" {
				values = append(values, text)
			}
		}
		return values
	default:
		return nil
	}
}
