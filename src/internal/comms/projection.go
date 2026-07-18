package comms

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"github.com/chrote/server/internal/formations"
)

const ProjectionSchema = "mission-room.projection.v1"

var (
	ErrInvalidRoomRef = errors.New("invalid room ref")
	ErrRoomNotFound   = errors.New("mission room not found")
	roomKindRE        = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,40}$`)
	roomIDRE          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,120}$`)
)

type Store struct {
	Workspace       string
	formationsStore *formations.Store
}

type ProjectionOptions struct {
	IncludePrivateFor string
}

type MessageOptions struct {
	IncludePrivateFor string
	Since             int
	Limit             int
}

type RoomProjection struct {
	Schema              string                 `json:"schema"`
	RoomRef             string                 `json:"roomRef"`
	RoomKind            string                 `json:"roomKind"`
	RoomID              string                 `json:"roomId"`
	Source              RoomSource             `json:"source"`
	Summary             RoomSummary            `json:"summary"`
	LatestBoundary      *RoomBoundary          `json:"latestBoundary,omitempty"`
	Messages            []RoomMessage          `json:"messages"`
	Claims              []RoomClaim            `json:"claims"`
	Reservations        map[string]Reservation `json:"reservations"`
	Mentions            []RoomMention          `json:"mentions"`
	Artifacts           []RoomArtifact         `json:"artifacts"`
	Risks               []RoomRisk             `json:"risks"`
	ReservationWarnings []ReservationWarning   `json:"reservationWarnings"`
}

type RoomSource struct {
	Kind       string `json:"kind"`
	ReadOnly   bool   `json:"readOnly"`
	RunStatus  string `json:"runStatus,omitempty"`
	RunFinal   bool   `json:"runFinal,omitempty"`
	BoardSlug  string `json:"boardSlug,omitempty"`
	MissionID  string `json:"missionId,omitempty"`
	BeadID     string `json:"beadId,omitempty"`
	EventCount int    `json:"eventCount"`
}

type RoomSummary struct {
	EventCount              int `json:"eventCount"`
	ClaimCount              int `json:"claimCount"`
	OpenClaimCount          int `json:"openClaimCount"`
	DoneClaimCount          int `json:"doneClaimCount"`
	ArtifactCount           int `json:"artifactCount"`
	SalvagedArtifactCount   int `json:"salvagedArtifactCount"`
	MentionCount            int `json:"mentionCount"`
	UnaddressedMentionCount int `json:"unaddressedMentionCount"`
	RiskCount               int `json:"riskCount"`
}

type RoomBoundary struct {
	Seq       int    `json:"seq"`
	Actor     string `json:"actor"`
	Text      string `json:"text"`
	Timestamp string `json:"timestamp,omitempty"`
}

type RoomMessage struct {
	Seq       int            `json:"seq"`
	Timestamp string         `json:"timestamp,omitempty"`
	Actor     string         `json:"actor"`
	Type      string         `json:"type"`
	Text      string         `json:"text"`
	To        []string       `json:"to,omitempty"`
	VisibleTo []string       `json:"visibleTo,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type RoomClaim struct {
	Seq                  int                  `json:"seq"`
	Actor                string               `json:"actor"`
	Task                 string               `json:"task"`
	Status               string               `json:"status"`
	Category             string               `json:"category,omitempty"`
	Dependencies         []string             `json:"dependencies"`
	Reservations         []string             `json:"reservations"`
	ReservationWarnings  []ReservationWarning `json:"reservationWarnings"`
	ExpectedArtifacts    []string             `json:"expectedArtifacts"`
	VerificationCommand  string               `json:"verificationCommand,omitempty"`
	VerificationEvidence string               `json:"verificationEvidence,omitempty"`
	ArtifactSeqs         []int                `json:"artifactSeqs"`
	SalvagedArtifactSeqs []int                `json:"salvagedArtifactSeqs"`
	DoneSeq              int                  `json:"doneSeq,omitempty"`
	DoneText             string               `json:"doneText,omitempty"`
	ResolvedBy           string               `json:"resolvedBy,omitempty"`
	ResolvedSeq          int                  `json:"resolvedSeq,omitempty"`
	StatusHistory        []ClaimStatus        `json:"statusHistory"`
}

type ClaimStatus struct {
	Seq    int    `json:"seq"`
	Actor  string `json:"actor"`
	Status string `json:"status"`
	Text   string `json:"text"`
}

type Reservation struct {
	ClaimSeq int    `json:"claimSeq"`
	Actor    string `json:"actor"`
	Status   string `json:"status"`
}

type ReservationWarning struct {
	Type              string   `json:"type"`
	Severity          string   `json:"severity"`
	Path              string   `json:"path"`
	ExpectedArtifacts []string `json:"expectedArtifacts,omitempty"`
	Detail            string   `json:"detail,omitempty"`
}

type RoomMention struct {
	Seq           int    `json:"seq"`
	SourceSeq     int    `json:"sourceSeq,omitempty"`
	Target        string `json:"target"`
	Status        string `json:"status"`
	Passive       bool   `json:"passive"`
	TmuxInjection bool   `json:"tmuxInjection"`
	AddressedBy   string `json:"addressedBy,omitempty"`
	AddressedSeq  int    `json:"addressedSeq,omitempty"`
}

type RoomArtifact struct {
	Seq          int    `json:"seq"`
	Actor        string `json:"actor"`
	Type         string `json:"type"`
	Text         string `json:"text"`
	Path         string `json:"path,omitempty"`
	ClaimSeq     int    `json:"claimSeq,omitempty"`
	ClaimOwner   string `json:"claimOwner,omitempty"`
	Salvaged     bool   `json:"salvaged,omitempty"`
	SalvagedBy   string `json:"salvagedBy,omitempty"`
	VerifiedBy   string `json:"verifiedBy,omitempty"`
	Verification string `json:"verification,omitempty"`
}

type RoomRisk struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	ClaimSeq    int    `json:"claimSeq,omitempty"`
	ClaimSeqs   []int  `json:"claimSeqs,omitempty"`
	MentionSeqs []int  `json:"mentionSeqs,omitempty"`
	Path        string `json:"path,omitempty"`
	Detail      string `json:"detail"`
}

type RoomMessages struct {
	RoomRef   string        `json:"roomRef"`
	Messages  []RoomMessage `json:"messages"`
	NextSince int           `json:"nextSince"`
}

type RoomExport struct {
	RoomRef  string        `json:"roomRef"`
	Format   string        `json:"format"`
	Events   []RoomMessage `json:"events,omitempty"`
	Markdown string        `json:"markdown,omitempty"`
}

type rawEvent struct {
	Seq        int            `json:"seq"`
	Kind       string         `json:"kind"`
	Type       string         `json:"type"`
	Actor      string         `json:"actor"`
	Text       string         `json:"text"`
	Timestamp  string         `json:"timestamp"`
	TS         string         `json:"ts"`
	VisibleTo  []string       `json:"visible_to"`
	VisibleTo2 []string       `json:"visibleTo"`
	To         []string       `json:"to"`
	Metadata   map[string]any `json:"metadata"`
	Data       map[string]any `json:"data"`
}

// NewStore constructs the schema-1 compatibility projection store. Production
// server wiring must use NewStoreWithFormations with its shared runtime Store.
func NewStore(workspace string) *Store {
	return NewStoreWithFormations(workspace, formations.NewStore(workspace))
}

func NewStoreWithFormations(workspace string, formationsStore *formations.Store) *Store {
	return &Store{Workspace: workspace, formationsStore: formationsStore}
}

func (s *Store) ProjectRoom(roomRef string, options ProjectionOptions) (RoomProjection, error) {
	kind, id, err := parseRoomRef(roomRef)
	if err != nil {
		return RoomProjection{}, err
	}
	if kind == "run" {
		return s.projectRunRoom(roomRef, id, options)
	}
	events, err := s.readEvents(kind, id)
	if err != nil {
		return RoomProjection{}, err
	}
	visible := filterVisible(events, options.IncludePrivateFor)
	projection := RoomProjection{
		Schema:       ProjectionSchema,
		RoomRef:      roomRef,
		RoomKind:     kind,
		RoomID:       id,
		Source:       RoomSource{Kind: "comms-ledger", ReadOnly: false, EventCount: len(events)},
		Messages:     make([]RoomMessage, 0, len(visible)),
		Claims:       []RoomClaim{},
		Reservations: map[string]Reservation{},
		Mentions:     []RoomMention{},
		Artifacts:    []RoomArtifact{},
		Risks:        []RoomRisk{},
	}
	claims := map[int]*RoomClaim{}
	mentions := map[int]*RoomMention{}
	for _, event := range visible {
		projection.Messages = append(projection.Messages, roomMessageFromEvent(event))
		switch event.eventType() {
		case "boundary_pinned", "decision_recorded":
			projection.LatestBoundary = &RoomBoundary{Seq: event.Seq, Actor: event.Actor, Text: event.Text, Timestamp: event.timestamp()}
		case "task_claimed":
			claim := newClaim(event)
			claims[claim.Seq] = &claim
			if isOpenClaim(claim.Status) {
				setReservations(projection.Reservations, claim)
			}
		case "task_claim_updated", "task_claim_resolved":
			claimSeq := intValue(event.meta("claim_seq"))
			claim := claims[claimSeq]
			if claim == nil {
				continue
			}
			applyClaimUpdate(projection.Reservations, claim, event)
		case "task_done":
			claimSeq := intValue(event.meta("claim_seq"))
			claim := claims[claimSeq]
			if claim == nil {
				continue
			}
			claim.Status = "done"
			claim.DoneSeq = event.Seq
			claim.DoneText = event.Text
			claim.VerificationEvidence = stringValue(event.meta("verification"))
			claim.StatusHistory = append(claim.StatusHistory, statusFromEvent(event, "done"))
			releaseReservations(projection.Reservations, claim.Seq)
		case "artifact_recorded", "artifact_attached", "artifact_salvaged":
			artifact := artifactFromEvent(event)
			projection.Artifacts = append(projection.Artifacts, artifact)
			if artifact.ClaimSeq != 0 {
				if claim := claims[artifact.ClaimSeq]; claim != nil {
					if artifact.Salvaged {
						claim.SalvagedArtifactSeqs = append(claim.SalvagedArtifactSeqs, artifact.Seq)
					} else {
						claim.ArtifactSeqs = append(claim.ArtifactSeqs, artifact.Seq)
					}
				}
			}
		case "passive_mention", "needs_input_signal":
			mention := mentionFromEvent(event)
			mentions[mention.Seq] = &mention
		case "passive_mention_addressed":
			mentionSeq := intValue(event.meta("mention_seq"))
			if mention := mentions[mentionSeq]; mention != nil {
				mention.Status = "addressed"
				mention.AddressedBy = event.Actor
				mention.AddressedSeq = event.Seq
			}
		}
	}
	projection.Claims = sortedClaims(claims)
	projection.Mentions = sortedMentions(mentions)
	projection.ReservationWarnings = collectReservationWarnings(projection.Claims)
	projection.Risks = projectionRisks(projection.Claims, projection.Mentions)
	projection.Summary = summarizeProjection(projection)
	return projection, nil
}

func (s *Store) projectRunRoom(roomRef, runID string, options ProjectionOptions) (RoomProjection, error) {
	if s == nil || s.formationsStore == nil {
		return RoomProjection{}, ErrRoomNotFound
	}
	events, err := s.formationsStore.ReadRunEvents(runID)
	if err != nil {
		if errors.Is(err, formations.ErrNotFound) {
			return RoomProjection{}, ErrRoomNotFound
		}
		if errors.Is(err, formations.ErrInvalidSlug) {
			return RoomProjection{}, ErrInvalidRoomRef
		}
		return RoomProjection{}, err
	}
	status, err := s.formationsStore.ProjectRun(runID)
	if err != nil {
		if errors.Is(err, formations.ErrNotFound) {
			return RoomProjection{}, ErrRoomNotFound
		}
		return RoomProjection{}, err
	}
	messages := make([]RoomMessage, 0, len(events))
	for _, event := range events {
		messages = append(messages, messageFromRunEvent(event))
	}
	projection := RoomProjection{
		Schema:       ProjectionSchema,
		RoomRef:      roomRef,
		RoomKind:     "run",
		RoomID:       runID,
		Source:       RoomSource{Kind: "formations-run-ledger", ReadOnly: true, RunStatus: status.Status, RunFinal: status.Final, BoardSlug: status.BoardSlug, MissionID: status.MissionID, BeadID: status.BeadID, EventCount: len(events)},
		Messages:     messages,
		Claims:       []RoomClaim{},
		Reservations: map[string]Reservation{},
		Mentions:     []RoomMention{},
		Artifacts:    []RoomArtifact{},
		Risks:        []RoomRisk{},
	}
	projection.Summary = summarizeProjection(projection)
	return projection, nil
}

func (s *Store) Messages(roomRef string, options MessageOptions) (RoomMessages, error) {
	projection, err := s.ProjectRoom(roomRef, ProjectionOptions{IncludePrivateFor: options.IncludePrivateFor})
	if err != nil {
		return RoomMessages{}, err
	}
	limit := options.Limit
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	messages := make([]RoomMessage, 0, limit)
	for _, message := range projection.Messages {
		if message.Seq <= options.Since {
			continue
		}
		messages = append(messages, message)
		if len(messages) == limit {
			break
		}
	}
	nextSince := options.Since
	if len(messages) > 0 {
		nextSince = messages[len(messages)-1].Seq
	}
	return RoomMessages{RoomRef: roomRef, Messages: messages, NextSince: nextSince}, nil
}

func (s *Store) Export(roomRef, format, includePrivateFor string) (RoomExport, error) {
	if format == "" {
		format = "ndjson"
	}
	messages, err := s.Messages(roomRef, MessageOptions{IncludePrivateFor: includePrivateFor, Limit: 1000})
	if err != nil {
		return RoomExport{}, err
	}
	switch format {
	case "ndjson":
		return RoomExport{RoomRef: roomRef, Format: format, Events: messages.Messages}, nil
	case "md", "markdown":
		var builder strings.Builder
		for _, message := range messages.Messages {
			fmt.Fprintf(&builder, "- #%03d `%s` **%s**: %s\n", message.Seq, message.Type, message.Actor, message.Text)
		}
		return RoomExport{RoomRef: roomRef, Format: "markdown", Markdown: builder.String()}, nil
	default:
		return RoomExport{}, fmt.Errorf("unsupported export format: %s", format)
	}
}

func (s *Store) readEvents(kind, id string) ([]rawEvent, error) {
	file, err := s.openRoomLedger(kind, id)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrRoomNotFound
		}
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	events := []rawEvent{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event rawEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("invalid room ledger event: %w", err)
		}
		if event.Metadata == nil && event.Data != nil {
			event.Metadata = event.Data
		}
		if event.Metadata == nil {
			event.Metadata = map[string]any{}
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Seq < events[j].Seq })
	return events, nil
}

func (s *Store) openRoomLedger(kind, id string) (*os.File, error) {
	if s == nil || !roomKindRE.MatchString(kind) || !roomIDRE.MatchString(id) || strings.Contains(id, "..") {
		return nil, ErrInvalidRoomRef
	}
	workspace, err := filepath.Abs(s.Workspace)
	if err != nil {
		return nil, err
	}
	workspace = filepath.Clean(workspace)
	// The configured workspace itself may be a compatibility symlink. Pin the
	// opened root once, then refuse every descendant symlink relative to it.
	fd, err := syscall.Open(workspace, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NONBLOCK|syscall.O_DIRECTORY, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: workspace, Err: err}
	}
	current := os.NewFile(uintptr(fd), workspace)
	if current == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("could not open mission room workspace")
	}

	components := []string{".formations", "comms", kind, id + ".ndjson"}
	for index, component := range components {
		directory := index < len(components)-1
		next, openErr := openRoomLedgerComponentAt(current, component, directory)
		_ = current.Close()
		if openErr != nil {
			return nil, openErr
		}
		current = next
	}
	return current, nil
}

func openRoomLedgerComponentAt(parent *os.File, name string, directory bool) (*os.File, error) {
	if parent == nil || name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsRune(name, 0) {
		return nil, errors.New("invalid mission room ledger component")
	}
	flags := syscall.O_RDONLY | syscall.O_CLOEXEC | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	if directory {
		flags |= syscall.O_DIRECTORY
	}
	fd, err := syscall.Openat(int(parent.Fd()), name, flags, 0)
	if err != nil {
		return nil, &os.PathError{Op: "openat", Path: name, Err: err}
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, errors.New("could not open mission room ledger component")
	}
	if directory {
		return file, nil
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	// A private ledger hard-linked into the workspace has no path evidence of
	// its origin, so only single-link regular ledgers are readable here.
	if !info.Mode().IsRegular() || !ok || stat.Nlink != 1 {
		_ = file.Close()
		return nil, errors.New("mission room ledger must be a single-link regular file")
	}
	return file, nil
}

func parseRoomRef(roomRef string) (string, string, error) {
	kind, id, ok := strings.Cut(roomRef, ":")
	if !ok || !roomKindRE.MatchString(kind) || !roomIDRE.MatchString(id) || strings.Contains(id, "..") {
		return "", "", ErrInvalidRoomRef
	}
	return kind, id, nil
}

func filterVisible(events []rawEvent, includePrivateFor string) []rawEvent {
	visible := make([]rawEvent, 0, len(events))
	for _, event := range events {
		if event.isVisible(includePrivateFor) {
			visible = append(visible, event)
		}
	}
	return visible
}

func (e rawEvent) eventType() string {
	if e.Kind != "" {
		return e.Kind
	}
	return e.Type
}

func (e rawEvent) timestamp() string {
	if e.Timestamp != "" {
		return e.Timestamp
	}
	return e.TS
}

func (e rawEvent) visibleTo() []string {
	if len(e.VisibleTo) > 0 {
		return e.VisibleTo
	}
	return e.VisibleTo2
}

func (e rawEvent) meta(key string) any {
	if e.Metadata == nil {
		return nil
	}
	return e.Metadata[key]
}

func (e rawEvent) isVisible(includePrivateFor string) bool {
	visibleTo := e.visibleTo()
	if len(visibleTo) == 0 {
		return true
	}
	if includePrivateFor != "" && containsString(visibleTo, includePrivateFor) {
		return true
	}
	return len(visibleTo) > 1
}

func roomMessageFromEvent(event rawEvent) RoomMessage {
	return RoomMessage{Seq: event.Seq, Timestamp: event.timestamp(), Actor: event.Actor, Type: event.eventType(), Text: event.Text, To: event.To, VisibleTo: event.visibleTo(), Metadata: event.Metadata}
}

func messageFromRunEvent(event formations.RunEvent) RoomMessage {
	metadata := map[string]any{}
	for key, value := range event.Data {
		metadata[key] = value
	}
	if event.RunID != "" {
		metadata["runId"] = event.RunID
	}
	if event.BoardID != "" {
		metadata["boardId"] = event.BoardID
	}
	if event.BoardRev != 0 {
		metadata["boardRev"] = event.BoardRev
	}
	if event.MissionID != "" {
		metadata["missionId"] = event.MissionID
	}
	if event.BeadID != "" {
		metadata["beadId"] = event.BeadID
	}
	if event.NodeID != "" {
		metadata["nodeId"] = event.NodeID
	}
	if event.SlotID != "" {
		metadata["slotId"] = event.SlotID
	}
	if event.GateID != "" {
		metadata["gateId"] = event.GateID
	}
	if event.EdgeID != "" {
		metadata["edgeId"] = event.EdgeID
	}
	if event.Epoch != 0 {
		metadata["epoch"] = event.Epoch
	}
	if event.Attempt != 0 {
		metadata["attempt"] = event.Attempt
	}
	return RoomMessage{Seq: event.Seq, Timestamp: event.Timestamp, Actor: event.Actor, Type: event.Type, Text: runEventText(event), Metadata: metadata}
}

func runEventText(event formations.RunEvent) string {
	for _, key := range []string{"text", "message", "summary", "objective", "reason"} {
		if value, ok := event.Data[key].(string); ok && value != "" {
			return value
		}
	}
	return event.Type
}

func newClaim(event rawEvent) RoomClaim {
	status := stringValue(event.meta("claim_status"))
	if status == "" {
		status = "claimed"
	}
	return RoomClaim{
		Seq:                  event.Seq,
		Actor:                event.Actor,
		Task:                 event.Text,
		Status:               status,
		Category:             stringValue(event.meta("category")),
		Dependencies:         stringSlice(event.meta("dependencies")),
		Reservations:         stringSlice(event.meta("reservations")),
		ReservationWarnings:  reservationWarnings(event.meta("reservation_warnings")),
		ExpectedArtifacts:    stringSlice(event.meta("expected_artifacts")),
		VerificationCommand:  stringValue(event.meta("verification_command")),
		ArtifactSeqs:         []int{},
		SalvagedArtifactSeqs: []int{},
		StatusHistory:        []ClaimStatus{statusFromEvent(event, status)},
	}
}

func applyClaimUpdate(reservations map[string]Reservation, claim *RoomClaim, event rawEvent) {
	status := stringValue(event.meta("claim_status"))
	if status == "" {
		return
	}
	claim.Status = status
	if task := stringValue(event.meta("task")); task != "" {
		claim.Task = task
	}
	if category := stringValue(event.meta("category")); category != "" {
		claim.Category = category
	}
	if hasMeta(event, "dependencies") {
		claim.Dependencies = stringSlice(event.meta("dependencies"))
	}
	if hasMeta(event, "reservations") {
		claim.Reservations = stringSlice(event.meta("reservations"))
	}
	if hasMeta(event, "reservation_warnings") {
		claim.ReservationWarnings = reservationWarnings(event.meta("reservation_warnings"))
	}
	if hasMeta(event, "expected_artifacts") {
		claim.ExpectedArtifacts = stringSlice(event.meta("expected_artifacts"))
	}
	if command := stringValue(event.meta("verification_command")); command != "" {
		claim.VerificationCommand = command
	}
	if resolvedBy := stringValue(event.meta("resolved_by")); resolvedBy != "" {
		claim.ResolvedBy = resolvedBy
		claim.ResolvedSeq = event.Seq
	}
	claim.StatusHistory = append(claim.StatusHistory, statusFromEvent(event, status))
	releaseReservations(reservations, claim.Seq)
	if isOpenClaim(status) {
		setReservations(reservations, *claim)
	}
}

func artifactFromEvent(event rawEvent) RoomArtifact {
	return RoomArtifact{
		Seq:          event.Seq,
		Actor:        event.Actor,
		Type:         event.eventType(),
		Text:         event.Text,
		Path:         stringValue(event.meta("path")),
		ClaimSeq:     intValue(event.meta("claim_seq")),
		ClaimOwner:   stringValue(event.meta("claim_owner")),
		Salvaged:     boolValue(event.meta("salvaged")),
		SalvagedBy:   stringValue(event.meta("salvaged_by")),
		VerifiedBy:   stringValue(event.meta("verified_by")),
		Verification: stringValue(event.meta("verification")),
	}
}

func mentionFromEvent(event rawEvent) RoomMention {
	target := stringValue(event.meta("mentioned"))
	if target == "" && len(event.To) > 0 {
		target = event.To[0]
	}
	return RoomMention{Seq: event.Seq, SourceSeq: intValue(event.meta("source_seq")), Target: target, Status: "unaddressed", Passive: true, TmuxInjection: boolValue(event.meta("tmux_injection"))}
}

func statusFromEvent(event rawEvent, status string) ClaimStatus {
	return ClaimStatus{Seq: event.Seq, Actor: event.Actor, Status: status, Text: event.Text}
}

func setReservations(reservations map[string]Reservation, claim RoomClaim) {
	for _, path := range claim.Reservations {
		reservations[path] = Reservation{ClaimSeq: claim.Seq, Actor: claim.Actor, Status: "active"}
	}
}

func releaseReservations(reservations map[string]Reservation, claimSeq int) {
	for path, reservation := range reservations {
		if reservation.ClaimSeq == claimSeq {
			delete(reservations, path)
		}
	}
}

func isOpenClaim(status string) bool {
	switch status {
	case "claimed", "narrowed", "blocked":
		return true
	default:
		return false
	}
}

func sortedClaims(claims map[int]*RoomClaim) []RoomClaim {
	seqs := make([]int, 0, len(claims))
	for seq := range claims {
		seqs = append(seqs, seq)
	}
	sort.Ints(seqs)
	result := make([]RoomClaim, 0, len(seqs))
	for _, seq := range seqs {
		result = append(result, *claims[seq])
	}
	return result
}

func sortedMentions(mentions map[int]*RoomMention) []RoomMention {
	seqs := make([]int, 0, len(mentions))
	for seq := range mentions {
		seqs = append(seqs, seq)
	}
	sort.Ints(seqs)
	result := make([]RoomMention, 0, len(seqs))
	for _, seq := range seqs {
		result = append(result, *mentions[seq])
	}
	return result
}

func collectReservationWarnings(claims []RoomClaim) []ReservationWarning {
	warnings := []ReservationWarning{}
	for _, claim := range claims {
		warnings = append(warnings, claim.ReservationWarnings...)
	}
	return warnings
}

func projectionRisks(claims []RoomClaim, mentions []RoomMention) []RoomRisk {
	risks := []RoomRisk{}
	openClaims := []RoomClaim{}
	for _, claim := range claims {
		if isOpenClaim(claim.Status) {
			openClaims = append(openClaims, claim)
			for _, warning := range claim.ReservationWarnings {
				risks = append(risks, RoomRisk{Type: "broad-active-reservation", Severity: defaultString(warning.Severity, "medium"), ClaimSeq: claim.Seq, Path: warning.Path, Detail: defaultString(warning.Detail, "active reservation may be too broad")})
			}
		}
		if claim.Status == "done" && (len(claim.ArtifactSeqs) == 0 || claim.VerificationEvidence == "") {
			risks = append(risks, RoomRisk{Type: "done-claim-missing-handoff-evidence", Severity: "medium", ClaimSeq: claim.Seq, Detail: "done claim lacks linked artifact and/or verification evidence"})
		}
	}
	for i, left := range openClaims {
		for _, right := range openClaims[i+1:] {
			if score := taskOverlapScore(left.Task, right.Task); score >= 5 {
				risks = append(risks, RoomRisk{Type: "overlapping-active-claims", Severity: "high", ClaimSeqs: []int{left.Seq, right.Seq}, Detail: fmt.Sprintf("active claims share %d task tokens", score)})
			}
		}
	}
	unaddressed := []int{}
	for _, mention := range mentions {
		if mention.Status != "addressed" {
			unaddressed = append(unaddressed, mention.Seq)
		}
	}
	if len(unaddressed) > 0 {
		risks = append(risks, RoomRisk{Type: "unaddressed-passive-mentions", Severity: "medium", MentionSeqs: unaddressed, Detail: "mentions are passive but still need triage"})
	}
	return risks
}

func summarizeProjection(projection RoomProjection) RoomSummary {
	openClaims := 0
	doneClaims := 0
	for _, claim := range projection.Claims {
		if isOpenClaim(claim.Status) {
			openClaims++
		}
		if claim.Status == "done" {
			doneClaims++
		}
	}
	salvaged := 0
	for _, artifact := range projection.Artifacts {
		if artifact.Salvaged {
			salvaged++
		}
	}
	unaddressed := 0
	for _, mention := range projection.Mentions {
		if mention.Status != "addressed" {
			unaddressed++
		}
	}
	return RoomSummary{EventCount: len(projection.Messages), ClaimCount: len(projection.Claims), OpenClaimCount: openClaims, DoneClaimCount: doneClaims, ArtifactCount: len(projection.Artifacts), SalvagedArtifactCount: salvaged, MentionCount: len(projection.Mentions), UnaddressedMentionCount: unaddressed, RiskCount: len(projection.Risks)}
}

func reservationWarnings(value any) []ReservationWarning {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	warnings := make([]ReservationWarning, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		warnings = append(warnings, ReservationWarning{Type: stringValue(m["type"]), Severity: stringValue(m["severity"]), Path: stringValue(m["path"]), ExpectedArtifacts: stringSlice(m["expected_artifacts"]), Detail: stringValue(m["detail"])})
	}
	return warnings
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string{}, typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return []string{}
	}
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		n, _ := typed.Int64()
		return int(n)
	default:
		return 0
	}
}

func stringValue(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

func boolValue(value any) bool {
	if b, ok := value.(bool); ok {
		return b
	}
	return false
}

func hasMeta(event rawEvent, key string) bool {
	_, ok := event.Metadata[key]
	return ok
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func taskOverlapScore(left, right string) int {
	stop := map[string]bool{"the": true, "and": true, "for": true, "with": true, "room": true, "mission": true, "prototype": true, "build": true, "create": true}
	leftTokens := tokenSet(left, stop)
	rightTokens := tokenSet(right, stop)
	score := 0
	for token := range leftTokens {
		if rightTokens[token] {
			score++
		}
	}
	return score
}

func tokenSet(text string, stop map[string]bool) map[string]bool {
	fields := regexp.MustCompile(`[a-z0-9]+`).FindAllString(strings.ToLower(text), -1)
	out := map[string]bool{}
	for _, field := range fields {
		if len(field) > 2 && !stop[field] {
			out[field] = true
		}
	}
	return out
}
