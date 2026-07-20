package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chrote/server/internal/core"
	"github.com/chrote/server/internal/formations"
)

type FormationsHandler struct {
	store    *formations.Store
	personas *formations.PersonaStore
}

type formationsRunStartRequest struct {
	Board       string               `json:"board"`
	MissionID   string               `json:"missionId"`
	FormationID string               `json:"formationId"`
	Actor       string               `json:"actor"`
	Limits      formations.RunLimits `json:"limits"`
	ExpectedRev int                  `json:"expectedRev"`
}

type formationsRunAbortRequest struct {
	Reason      string `json:"reason"`
	RequestedBy string `json:"requestedBy"`
}

type formationsRunResumeRequest struct {
	Actor  string `json:"actor"`
	Mode   string `json:"mode"`
	Reason string `json:"reason"`
}

type formationsHumanGateVerdictRequest struct {
	Verdict string `json:"verdict"`
	Reason  string `json:"reason"`
	Actor   string `json:"actor"`
}

type formationsBoardPatchRequest struct {
	Title                         *string                                 `json:"title"`
	CreateTool                    *formationsToolCreateRequest            `json:"createTool"`
	UpdateTool                    *formationsToolUpdateRequest            `json:"updateTool"`
	DeleteTool                    *formationsToolDeleteRequest            `json:"deleteTool"`
	CreateFormation               *formationsCreateFormationRequest       `json:"createFormation"`
	DeleteFormation               *formationsDeleteFormationRequest       `json:"deleteFormation"`
	DeleteGate                    *formationsDeleteGateRequest            `json:"deleteGate"`
	DeleteMission                 *formationsDeleteMissionRequest         `json:"deleteMission"`
	AssignSlot                    *formationsAssignSlotRequest            `json:"assignSlot"`
	MakeController                *formationsMakeControllerRequest        `json:"makeController"`
	SetBrief                      *formationsSetBriefRequest              `json:"setBrief"`
	ClearBrief                    *formationsClearBriefRequest            `json:"clearBrief"`
	SetVerification               *formationsSetVerificationRequest       `json:"setVerification"`
	RemoveVerification            *formationsRemoveVerificationRequest    `json:"removeVerification"`
	AddPort                       *formationsAddPortRequest               `json:"addPort"`
	RemovePort                    *formationsRemovePortRequest            `json:"removePort"`
	WireConnection                *formationsWireConnectionRequest        `json:"wireConnection"`
	UnwireConnection              *formationsWireConnectionRequest        `json:"unwireConnection"`
	RewireConnection              *formationsRewireConnectionRequest      `json:"rewireConnection"`
	CreateGate                    *formationsCreateGateRequest            `json:"createGate"`
	SetGateJudge                  *formationsSetGateJudgeRequest          `json:"setGateJudge"`
	DetachGateJudge               *formationsDetachGateJudgeRequest       `json:"detachGateJudge"`
	CreateMission                 *formationsCreateMissionRequest         `json:"createMission"`
	LegacyCommandFieldsPresent    bool                                    `json:"-"`
	SetVerificationOccurrences    int                                     `json:"-"`
	RemoveVerificationOccurrences int                                     `json:"-"`
	MutationOccurrences           int                                     `json:"-"`
	ExpectedRev                   int                                     `json:"expectedRev"`
	LayoutExpectation             *formationsToolLayoutExpectationRequest `json:"layoutExpectation"`
	UpdatedBy                     string                                  `json:"updatedBy"`
	ToolOperationOccurrences      int                                     `json:"-"`
	ToolFrameInvalid              bool                                    `json:"-"`
	ExpectedRevOccurrences        int                                     `json:"-"`
	LayoutExpectationOccurrences  int                                     `json:"-"`
	UpdatedByOccurrences          int                                     `json:"-"`
	UpdatedByNull                 bool                                    `json:"-"`
}

func (request *formationsBoardPatchRequest) UnmarshalJSON(raw []byte) error {
	presence, err := inspectBoardPatchPresence(raw)
	if err != nil {
		return err
	}
	invalidToolSurrogate := false
	if presence.ToolOperationOccurrences > 0 {
		invalidToolSurrogate, err = inspectToolFrameUnicode(raw)
		if err != nil {
			return err
		}
	}

	type requestFields formationsBoardPatchRequest
	var decoded requestFields
	if presence.ToolOperationOccurrences == 0 {
		legacy := struct {
			*requestFields
			LayoutExpectation json.RawMessage `json:"layoutExpectation"`
		}{requestFields: &decoded}
		if err := json.Unmarshal(raw, &legacy); err != nil {
			return err
		}
	} else {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return err
		}
	}
	*request = formationsBoardPatchRequest(decoded)
	request.LegacyCommandFieldsPresent = presence.LegacyCommandFieldsPresent
	request.SetVerificationOccurrences = presence.SetVerificationOccurrences
	request.RemoveVerificationOccurrences = presence.RemoveVerificationOccurrences
	request.MutationOccurrences = presence.MutationOccurrences
	request.ToolOperationOccurrences = presence.ToolOperationOccurrences
	request.ToolFrameInvalid = presence.ToolFrameInvalid || invalidToolSurrogate
	request.ExpectedRevOccurrences = presence.ExpectedRevOccurrences
	request.LayoutExpectationOccurrences = presence.LayoutExpectationOccurrences
	request.UpdatedByOccurrences = presence.UpdatedByOccurrences
	request.UpdatedByNull = presence.UpdatedByNull
	return nil
}

type formationsCreateFormationRequest struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	ExpectedRev int    `json:"expectedRev"`
	UpdatedBy   string `json:"updatedBy"`
}

type formationsDeleteFormationRequest struct {
	ID          string `json:"id"`
	ExpectedRev int    `json:"expectedRev"`
	UpdatedBy   string `json:"updatedBy"`
}

type formationsDeleteGateRequest struct {
	ID          string `json:"id"`
	ExpectedRev int    `json:"expectedRev"`
	UpdatedBy   string `json:"updatedBy"`
}

type formationsDeleteMissionRequest struct {
	ID          string `json:"id"`
	ExpectedRev int    `json:"expectedRev"`
	UpdatedBy   string `json:"updatedBy"`
}

type formationsAssignSlotRequest struct {
	FormationID string `json:"formationId"`
	SlotID      string `json:"slotId"`
	AgentID     string `json:"agentId"`
	Harness     string `json:"harness"`
	ExpectedRev int    `json:"expectedRev"`
	UpdatedBy   string `json:"updatedBy"`
}

type formationsMakeControllerRequest struct {
	FormationID string `json:"formationId"`
	SlotID      string `json:"slotId"`
	ExpectedRev int    `json:"expectedRev"`
	UpdatedBy   string `json:"updatedBy"`
}

type formationsSetBriefRequest struct {
	FormationID string   `json:"formationId"`
	Goal        string   `json:"goal"`
	BeadID      string   `json:"beadId"`
	Files       []string `json:"files"`
	Links       []string `json:"links"`
	ExpectedRev int      `json:"expectedRev"`
	UpdatedBy   string   `json:"updatedBy"`
}

type formationsClearBriefRequest struct {
	FormationID string `json:"formationId"`
	ExpectedRev int    `json:"expectedRev"`
	UpdatedBy   string `json:"updatedBy"`
}

type formationsSetVerificationRequest struct {
	FormationID string   `json:"formationId"`
	Kinds       []string `json:"kinds"`
	Criterion   string   `json:"criterion"`
	OnFail      string   `json:"onFail"`
	ExpectedRev int      `json:"expectedRev"`
	UpdatedBy   string   `json:"updatedBy"`
}

type formationsRemoveVerificationRequest struct {
	FormationID       string `json:"formationId"`
	ReplacementGateID string `json:"replacementGateId"`
	ExpectedRev       int    `json:"expectedRev"`
	UpdatedBy         string `json:"updatedBy"`
}

type formationsAddPortRequest struct {
	FormationID string `json:"formationId"`
	Direction   string `json:"direction"`
	Label       string `json:"label"`
	ExpectedRev int    `json:"expectedRev"`
	UpdatedBy   string `json:"updatedBy"`
}

type formationsRemovePortRequest struct {
	FormationID string `json:"formationId"`
	PortID      string `json:"portId"`
	ExpectedRev int    `json:"expectedRev"`
	UpdatedBy   string `json:"updatedBy"`
}

type formationsWireConnectionRequest struct {
	From        string `json:"from"`
	To          string `json:"to"`
	ExpectedRev int    `json:"expectedRev"`
	UpdatedBy   string `json:"updatedBy"`
}

type formationsCreateGateRequest struct {
	Title        string   `json:"title"`
	Kinds        []string `json:"kinds"`
	Criterion    string   `json:"criterion"`
	Command      string   `json:"command"`
	CommandArgv  []string `json:"commandArgv"`
	CommandCWD   string   `json:"commandCwd"`
	CommandShell string   `json:"commandShell"`
	X            int      `json:"x"`
	Y            int      `json:"y"`
	ExpectedRev  int      `json:"expectedRev"`
	UpdatedBy    string   `json:"updatedBy"`
}

type boardPatchPresence struct {
	LegacyCommandFieldsPresent    bool
	SetVerificationOccurrences    int
	RemoveVerificationOccurrences int
	MutationOccurrences           int
	ToolOperationOccurrences      int
	ToolFrameInvalid              bool
	ExpectedRevOccurrences        int
	LayoutExpectationOccurrences  int
	UpdatedByOccurrences          int
	UpdatedByNull                 bool
}

var boardPatchMutationKeys = []string{
	"title",
	"createTool",
	"updateTool",
	"deleteTool",
	"createFormation",
	"deleteFormation",
	"deleteGate",
	"deleteMission",
	"assignSlot",
	"makeController",
	"setBrief",
	"clearBrief",
	"setVerification",
	"removeVerification",
	"addPort",
	"removePort",
	"wireConnection",
	"unwireConnection",
	"rewireConnection",
	"createGate",
	"setGateJudge",
	"detachGateJudge",
	"createMission",
}

func inspectBoardPatchPresence(raw []byte) (boardPatchPresence, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil {
		return boardPatchPresence{}, err
	}
	if delimiter, ok := opening.(json.Delim); !ok || delimiter != '{' {
		return boardPatchPresence{}, fmt.Errorf("board patch must be a JSON object")
	}
	presence := boardPatchPresence{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return boardPatchPresence{}, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return boardPatchPresence{}, fmt.Errorf("board patch key must be a string")
		}
		for _, mutationKey := range boardPatchMutationKeys {
			if strings.EqualFold(key, mutationKey) {
				presence.MutationOccurrences++
				break
			}
		}
		if isToolOperationKey(key) {
			presence.ToolOperationOccurrences++
		}
		if !isExactToolFrameKey(key) {
			presence.ToolFrameInvalid = true
		}
		if strings.EqualFold(key, "createGate") {
			legacyFieldsPresent, err := scanLegacyGateCommandFields(decoder)
			if err != nil {
				return boardPatchPresence{}, err
			}
			presence.LegacyCommandFieldsPresent = presence.LegacyCommandFieldsPresent || legacyFieldsPresent
			continue
		}
		if strings.EqualFold(key, "setVerification") {
			presence.SetVerificationOccurrences++
		}
		if strings.EqualFold(key, "removeVerification") {
			presence.RemoveVerificationOccurrences++
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return boardPatchPresence{}, err
		}
		switch key {
		case "expectedRev":
			presence.ExpectedRevOccurrences++
		case "layoutExpectation":
			presence.LayoutExpectationOccurrences++
		case "updatedBy":
			presence.UpdatedByOccurrences++
			presence.UpdatedByNull = presence.UpdatedByNull || bytes.Equal(bytes.TrimSpace(value), []byte("null"))
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return boardPatchPresence{}, err
	}
	if delimiter, ok := closing.(json.Delim); !ok || delimiter != '}' {
		return boardPatchPresence{}, fmt.Errorf("board patch object is not closed")
	}
	return presence, nil
}

func scanLegacyGateCommandFields(decoder *json.Decoder) (bool, error) {
	opening, err := decoder.Token()
	if err != nil {
		return false, err
	}
	delimiter, isContainer := opening.(json.Delim)
	if !isContainer {
		return false, nil
	}
	if delimiter != '{' {
		if err := skipJSONContainer(decoder, delimiter); err != nil {
			return false, err
		}
		return false, nil
	}
	found := false
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return false, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return false, fmt.Errorf("createGate key must be a string")
		}
		for _, legacyField := range []string{"command", "commandArgv", "commandCwd", "commandShell"} {
			if strings.EqualFold(key, legacyField) {
				found = true
				break
			}
		}
		if err := skipJSONValue(decoder); err != nil {
			return false, err
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return false, err
	}
	if closeDelimiter, ok := closing.(json.Delim); !ok || closeDelimiter != '}' {
		return false, fmt.Errorf("createGate object is not closed")
	}
	return found, nil
}

func skipJSONValue(decoder *json.Decoder) error {
	value, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := value.(json.Delim); ok {
		return skipJSONContainer(decoder, delimiter)
	}
	return nil
}

func skipJSONContainer(decoder *json.Decoder, opening json.Delim) error {
	switch opening {
	case '{':
		for decoder.More() {
			if _, err := decoder.Token(); err != nil {
				return err
			}
			if err := skipJSONValue(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := skipJSONValue(decoder); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported JSON delimiter %q", opening)
	}
	_, err := decoder.Token()
	return err
}

type formationsSetGateJudgeRequest struct {
	GateID      string   `json:"gateId"`
	Chain       []string `json:"chain"`
	ExpectedRev int      `json:"expectedRev"`
	UpdatedBy   string   `json:"updatedBy"`
}

type formationsDetachGateJudgeRequest struct {
	GateID      string `json:"gateId"`
	ExpectedRev int    `json:"expectedRev"`
	UpdatedBy   string `json:"updatedBy"`
}

type formationsRewireConnectionRequest struct {
	From        string `json:"from"`
	PreviousTo  string `json:"previousTo"`
	To          string `json:"to"`
	ExpectedRev int    `json:"expectedRev"`
	UpdatedBy   string `json:"updatedBy"`
}

type formationsCreateMissionRequest struct {
	Title       string `json:"title"`
	Goal        string `json:"goal"`
	BeadID      string `json:"beadId"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	ExpectedRev int    `json:"expectedRev"`
	UpdatedBy   string `json:"updatedBy"`
}

type formationsLayoutPatchRequest struct {
	UpdatedAt string                  `json:"updatedAt"`
	Nodes     []formations.LayoutNode `json:"nodes"`
	Edges     []formations.LayoutEdge `json:"edges"`
	Arrange   bool                    `json:"arrange"`
}

// NewFormationsHandler constructs a schema-1 compatibility handler. Production
// server wiring must inject a runtime-authority Store with NewFormationsHandlerWithStore.
func NewFormationsHandler(workspace string) *FormationsHandler {
	return NewFormationsHandlerWithStores(formations.NewStore(workspace), formations.NewPersonaStore(formations.DefaultAgentsDir()))
}

func NewFormationsHandlerWithStore(store *formations.Store) *FormationsHandler {
	return NewFormationsHandlerWithStores(store, formations.NewPersonaStore(formations.DefaultAgentsDir()))
}

func NewFormationsHandlerWithStores(store *formations.Store, personas *formations.PersonaStore) *FormationsHandler {
	return &FormationsHandler{store: store, personas: personas}
}

func (h *FormationsHandler) newRunEngine(boundary string) *formations.RunEngine {
	return formations.NewRunEngine(h.store, h.personas, formations.NewConfiguredFormationExecutorFromEnv(h.store, h.personas, boundary))
}

func (h *FormationsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/formations/boards", h.ListBoards)
	mux.HandleFunc("POST /api/formations/runs", h.StartRun)
	mux.HandleFunc("GET /api/formations/runs/{runId}", h.GetRun)
	mux.HandleFunc("GET /api/formations/runs/{runId}/events", h.GetRunEvents)
	mux.HandleFunc("GET /api/formations/runs/{runId}/stream", h.StreamRunEvents)
	mux.HandleFunc("POST /api/formations/runs/{runId}/resume", h.ResumeRun)
	mux.HandleFunc("POST /api/formations/runs/{runId}/abort", h.AbortRun)
	mux.HandleFunc("POST /api/formations/runs/{runId}/gates/{gateId}/verdict", h.RecordHumanGateVerdict)
	mux.HandleFunc("GET /api/formations/runs/{runId}/escalations", h.GetRunEscalations)
	mux.HandleFunc("GET /api/formations/boards/{board}/changes", h.GetBoardChanges)
	mux.HandleFunc("GET /api/formations/boards/{board}", h.GetBoard)
	mux.HandleFunc("PATCH /api/formations/boards/{board}", h.PatchBoard)
	mux.HandleFunc("GET /api/formations/boards/{board}/layout", h.GetLayout)
	mux.HandleFunc("PATCH /api/formations/boards/{board}/layout", h.PatchLayout)
}

func (h *FormationsHandler) StartRun(w http.ResponseWriter, r *http.Request) {
	var request formationsRunStartRequest
	if !decodeJSONBody(w, r, &request) {
		return
	}
	if request.Board == "" {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "board is required")
		return
	}
	if (request.MissionID == "") == (request.FormationID == "") {
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "exactly one of missionId or formationId is required")
		return
	}
	slug, err := h.store.ResolveBoardSelector(request.Board)
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	board, err := h.store.ReadBoard(slug)
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	expectedRev := request.ExpectedRev
	if expectedRev == 0 {
		expectedRev = board.Rev
	}
	engine := h.newRunEngine("api")
	if request.FormationID != "" {
		if match := r.Header.Get("If-Match"); match != "" && match != board.ETag {
			writeFormationsError(w, formations.ErrConflict)
			return
		}
		if expectedRev != 0 && expectedRev != board.Rev {
			writeFormationsError(w, formations.ErrConflict)
			return
		}
		status, err := engine.RunFormation(slug, request.FormationID, formations.FormationRunRequest{
			Actor:    request.Actor,
			Personas: h.personas,
			Limits:   request.Limits,
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		core.WriteSuccess(w, map[string]interface{}{"runId": status.RunID, "status": status})
		return
	}
	status, err := engine.RunMission(slug, formations.RunStartRequest{
		MissionID:         request.MissionID,
		Actor:             request.Actor,
		ExpectedBoardETag: r.Header.Get("If-Match"),
		ExpectedBoardRev:  expectedRev,
		Personas:          h.personas,
		Limits:            request.Limits,
	})
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	core.WriteSuccess(w, map[string]interface{}{"runId": status.RunID, "status": status})
}

func (h *FormationsHandler) GetRun(w http.ResponseWriter, r *http.Request) {
	status, err := h.store.ProjectRun(r.PathValue("runId"))
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	core.WriteSuccess(w, map[string]interface{}{"status": status})
}

func (h *FormationsHandler) GetRunEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.store.ReadRunEvents(r.PathValue("runId"))
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	core.WriteSuccess(w, map[string]interface{}{"events": events})
}

func (h *FormationsHandler) StreamRunEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.store.ReadRunEvents(r.PathValue("runId"))
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	since := 0
	if raw := r.URL.Query().Get("since"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "since must be an integer sequence")
			return
		}
		since = parsed
	}
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Last-Event-ID must be an integer sequence")
			return
		}
		since = parsed
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	for _, event := range events {
		if event.Seq <= since {
			continue
		}
		raw, err := json.Marshal(event)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
			return
		}
		fmt.Fprintf(w, "id: %d\n", event.Seq)
		fmt.Fprintf(w, "event: %s\n", event.Type)
		fmt.Fprintf(w, "data: %s\n\n", raw)
	}
}

func (h *FormationsHandler) AbortRun(w http.ResponseWriter, r *http.Request) {
	var request formationsRunAbortRequest
	if !decodeJSONBody(w, r, &request) {
		return
	}
	if request.Reason == "" {
		request.Reason = "operator stop"
	}
	if request.RequestedBy == "" {
		request.RequestedBy = "agent:archon"
	}
	runID := r.PathValue("runId")
	if err := h.store.AppendRunEvent(runID, formations.RunEvent{
		Type:  formations.RunEventCanceled,
		Actor: request.RequestedBy,
		Data: map[string]any{
			"reason":               request.Reason,
			"requestedBy":          request.RequestedBy,
			"softInterruptedSlots": []string{},
			"final":                true,
		},
	}); err != nil {
		writeFormationsError(w, err)
		return
	}
	status, err := h.store.ProjectRun(runID)
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	core.WriteSuccess(w, map[string]interface{}{"status": status})
}

func (h *FormationsHandler) ResumeRun(w http.ResponseWriter, r *http.Request) {
	var request formationsRunResumeRequest
	if !decodeJSONBody(w, r, &request) {
		return
	}
	engine := h.newRunEngine("api")
	status, err := engine.ResumeRun(r.PathValue("runId"), formations.RunResumeRequest{
		Actor:  request.Actor,
		Mode:   request.Mode,
		Reason: request.Reason,
	})
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	core.WriteSuccess(w, map[string]interface{}{"status": status})
}

func (h *FormationsHandler) RecordHumanGateVerdict(w http.ResponseWriter, r *http.Request) {
	var request formationsHumanGateVerdictRequest
	if !decodeJSONBody(w, r, &request) {
		return
	}
	engine := h.newRunEngine("api")
	status, err := engine.RecordHumanGateVerdict(r.PathValue("runId"), formations.HumanGateVerdictRequest{
		GateID:  r.PathValue("gateId"),
		Verdict: request.Verdict,
		Reason:  request.Reason,
		Actor:   request.Actor,
	})
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	core.WriteSuccess(w, map[string]interface{}{"status": status})
}

func (h *FormationsHandler) GetRunEscalations(w http.ResponseWriter, r *http.Request) {
	escalations, err := h.store.ProjectOpenEscalations(r.PathValue("runId"))
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	core.WriteSuccess(w, map[string]interface{}{"escalations": escalations})
}

func (h *FormationsHandler) ListBoards(w http.ResponseWriter, r *http.Request) {
	boards, err := h.store.ListBoards()
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	core.WriteSuccess(w, map[string]interface{}{"boards": boards})
}

func (h *FormationsHandler) GetBoard(w http.ResponseWriter, r *http.Request) {
	slug, err := h.store.ResolveBoardSelector(r.PathValue("board"))
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	board, err := h.store.ReadBoard(slug)
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	w.Header().Set("ETag", board.ETag)
	core.WriteSuccess(w, map[string]interface{}{"board": board})
}

func (h *FormationsHandler) GetBoardChanges(w http.ResponseWriter, r *http.Request) {
	slug, err := h.store.ResolveBoardSelector(r.PathValue("board"))
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	signal, err := h.store.BoardChangeSince(slug, r.URL.Query().Get("etag"))
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	core.WriteSuccess(w, map[string]interface{}{"signal": signal})
}

func (h *FormationsHandler) PatchBoard(w http.ResponseWriter, r *http.Request) {
	var request formationsBoardPatchRequest
	if !decodeJSONBody(w, r, &request) {
		return
	}
	if h.patchToolBoard(w, r, &request) {
		return
	}
	if request.SetVerificationOccurrences > 0 {
		writeFormationsError(w, formations.ErrLegacyInlineVerificationRequiresMigration)
		return
	}
	if request.RemoveVerificationOccurrences > 0 && (request.RemoveVerificationOccurrences != 1 || request.MutationOccurrences != 1 || request.RemoveVerification == nil) {
		writeFormationsError(w, formations.ErrLegacyInlineVerificationRequiresMigration)
		return
	}
	if request.LegacyCommandFieldsPresent {
		writeFormationsError(w, formations.ErrLegacyScriptGateRequiresFencedMigration)
		return
	}
	slug, err := h.store.ResolveBoardSelector(r.PathValue("board"))
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	if request.CreateFormation != nil {
		create := request.CreateFormation
		updatedBy := request.UpdatedBy
		if updatedBy == "" {
			updatedBy = create.UpdatedBy
		}
		expectedRev := request.ExpectedRev
		if expectedRev == 0 {
			expectedRev = create.ExpectedRev
		}
		result, err := h.store.CreateFormation(slug, formations.FormationCreateRequest{
			Type:      create.Type,
			Title:     create.Title,
			X:         create.X,
			Y:         create.Y,
			UpdatedBy: updatedBy,
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  expectedRev,
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", result.Board.ETag)
		core.WriteSuccess(w, result)
		return
	}
	if request.DeleteFormation != nil {
		deleteRequest := request.DeleteFormation
		updatedBy := request.UpdatedBy
		if updatedBy == "" {
			updatedBy = deleteRequest.UpdatedBy
		}
		expectedRev := request.ExpectedRev
		if expectedRev == 0 {
			expectedRev = deleteRequest.ExpectedRev
		}
		result, err := h.store.DeleteFormation(slug, formations.FormationDeleteRequest{
			ID:        deleteRequest.ID,
			UpdatedBy: updatedBy,
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  expectedRev,
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", result.Board.ETag)
		core.WriteSuccess(w, result)
		return
	}
	if request.DeleteGate != nil {
		deleteRequest := request.DeleteGate
		result, err := h.store.DeleteGate(slug, formations.GateDeleteRequest{
			ID:        deleteRequest.ID,
			UpdatedBy: patchUpdatedBy(request.UpdatedBy, deleteRequest.UpdatedBy),
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  patchExpectedRev(request.ExpectedRev, deleteRequest.ExpectedRev),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", result.Board.ETag)
		core.WriteSuccess(w, result)
		return
	}
	if request.DeleteMission != nil {
		deleteRequest := request.DeleteMission
		result, err := h.store.DeleteMission(slug, formations.MissionDeleteRequest{
			ID:        deleteRequest.ID,
			UpdatedBy: patchUpdatedBy(request.UpdatedBy, deleteRequest.UpdatedBy),
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  patchExpectedRev(request.ExpectedRev, deleteRequest.ExpectedRev),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", result.Board.ETag)
		core.WriteSuccess(w, result)
		return
	}
	if request.AssignSlot != nil {
		assign := request.AssignSlot
		board, err := h.store.AssignFormationSlot(slug, formations.FormationSlotAssignmentRequest{
			FormationID: assign.FormationID,
			SlotID:      assign.SlotID,
			AgentID:     assign.AgentID,
			Harness:     assign.Harness,
			UpdatedBy:   patchUpdatedBy(request.UpdatedBy, assign.UpdatedBy),
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  patchExpectedRev(request.ExpectedRev, assign.ExpectedRev),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", board.ETag)
		core.WriteSuccess(w, map[string]interface{}{"board": board})
		return
	}
	if request.MakeController != nil {
		controller := request.MakeController
		board, err := h.store.SetFormationController(slug, formations.FormationControllerRequest{
			FormationID: controller.FormationID,
			SlotID:      controller.SlotID,
			UpdatedBy:   patchUpdatedBy(request.UpdatedBy, controller.UpdatedBy),
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  patchExpectedRev(request.ExpectedRev, controller.ExpectedRev),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", board.ETag)
		core.WriteSuccess(w, map[string]interface{}{"board": board})
		return
	}
	if request.SetBrief != nil {
		brief := request.SetBrief
		board, err := h.store.SetFormationBrief(slug, formations.FormationBriefRequest{
			FormationID: brief.FormationID,
			Goal:        brief.Goal,
			BeadID:      brief.BeadID,
			Files:       brief.Files,
			Links:       brief.Links,
			UpdatedBy:   patchUpdatedBy(request.UpdatedBy, brief.UpdatedBy),
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  patchExpectedRev(request.ExpectedRev, brief.ExpectedRev),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", board.ETag)
		core.WriteSuccess(w, map[string]interface{}{"board": board})
		return
	}
	if request.ClearBrief != nil {
		brief := request.ClearBrief
		board, err := h.store.ClearFormationBrief(slug, formations.FormationBriefClearRequest{
			FormationID: brief.FormationID,
			UpdatedBy:   patchUpdatedBy(request.UpdatedBy, brief.UpdatedBy),
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  patchExpectedRev(request.ExpectedRev, brief.ExpectedRev),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", board.ETag)
		core.WriteSuccess(w, map[string]interface{}{"board": board})
		return
	}
	if request.SetVerification != nil {
		verification := request.SetVerification
		board, err := h.store.SetFormationVerification(slug, formations.FormationVerificationRequest{
			FormationID: verification.FormationID,
			Kinds:       verification.Kinds,
			Criterion:   verification.Criterion,
			OnFail:      verification.OnFail,
			UpdatedBy:   patchUpdatedBy(request.UpdatedBy, verification.UpdatedBy),
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  patchExpectedRev(request.ExpectedRev, verification.ExpectedRev),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", board.ETag)
		core.WriteSuccess(w, map[string]interface{}{"board": board})
		return
	}
	if request.RemoveVerification != nil {
		verification := request.RemoveVerification
		board, err := h.store.RemoveFormationVerification(slug, formations.FormationVerificationRemovalRequest{
			FormationID:       verification.FormationID,
			ReplacementGateID: verification.ReplacementGateID,
			UpdatedBy:         patchUpdatedBy(request.UpdatedBy, verification.UpdatedBy),
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  patchExpectedRev(request.ExpectedRev, verification.ExpectedRev),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", board.ETag)
		core.WriteSuccess(w, map[string]interface{}{"board": board})
		return
	}
	if request.AddPort != nil {
		addPort := request.AddPort
		board, err := h.store.AddFormationPort(slug, formations.FormationPortRequest{
			FormationID: addPort.FormationID,
			Direction:   addPort.Direction,
			Label:       addPort.Label,
			UpdatedBy:   patchUpdatedBy(request.UpdatedBy, addPort.UpdatedBy),
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  patchExpectedRev(request.ExpectedRev, addPort.ExpectedRev),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", board.ETag)
		core.WriteSuccess(w, map[string]interface{}{"board": board})
		return
	}
	if request.RemovePort != nil {
		removePort := request.RemovePort
		board, err := h.store.RemoveFormationPort(slug, formations.FormationPortRemovalRequest{
			FormationID: removePort.FormationID,
			PortID:      removePort.PortID,
			UpdatedBy:   patchUpdatedBy(request.UpdatedBy, removePort.UpdatedBy),
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  patchExpectedRev(request.ExpectedRev, removePort.ExpectedRev),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", board.ETag)
		core.WriteSuccess(w, map[string]interface{}{"board": board})
		return
	}
	if request.WireConnection != nil {
		wire := request.WireConnection
		board, err := h.store.WireFormationPorts(slug, formations.FormationWireRequest{
			From:      wire.From,
			To:        wire.To,
			UpdatedBy: patchUpdatedBy(request.UpdatedBy, wire.UpdatedBy),
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  patchExpectedRev(request.ExpectedRev, wire.ExpectedRev),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", board.ETag)
		core.WriteSuccess(w, map[string]interface{}{"board": board})
		return
	}
	if request.RewireConnection != nil {
		wire := request.RewireConnection
		board, err := h.store.RewireFormationTarget(slug, formations.FormationRewireRequest{
			From:       wire.From,
			PreviousTo: wire.PreviousTo,
			To:         wire.To,
			UpdatedBy:  patchUpdatedBy(request.UpdatedBy, wire.UpdatedBy),
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  patchExpectedRev(request.ExpectedRev, wire.ExpectedRev),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", board.ETag)
		core.WriteSuccess(w, map[string]interface{}{"board": board})
		return
	}
	if request.UnwireConnection != nil {
		wire := request.UnwireConnection
		board, err := h.store.UnwireFormationPorts(slug, formations.FormationWireRequest{
			From:      wire.From,
			To:        wire.To,
			UpdatedBy: patchUpdatedBy(request.UpdatedBy, wire.UpdatedBy),
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  patchExpectedRev(request.ExpectedRev, wire.ExpectedRev),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", board.ETag)
		core.WriteSuccess(w, map[string]interface{}{"board": board})
		return
	}
	if request.CreateGate != nil {
		gate := request.CreateGate
		result, err := h.store.CreateGate(slug, formations.GateCreateRequest{
			Title:                      gate.Title,
			Kinds:                      gate.Kinds,
			Criterion:                  gate.Criterion,
			Command:                    gate.Command,
			CommandArgv:                gate.CommandArgv,
			CommandCWD:                 gate.CommandCWD,
			CommandShell:               gate.CommandShell,
			LegacyCommandFieldsPresent: request.LegacyCommandFieldsPresent,
			X:                          gate.X,
			Y:                          gate.Y,
			UpdatedBy:                  patchUpdatedBy(request.UpdatedBy, gate.UpdatedBy),
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  patchExpectedRev(request.ExpectedRev, gate.ExpectedRev),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", result.Board.ETag)
		core.WriteSuccess(w, result)
		return
	}
	if request.SetGateJudge != nil {
		judge := request.SetGateJudge
		board, err := h.store.SetGateJudgeChain(slug, formations.GateJudgeRequest{
			GateID:    judge.GateID,
			Chain:     judge.Chain,
			UpdatedBy: patchUpdatedBy(request.UpdatedBy, judge.UpdatedBy),
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  patchExpectedRev(request.ExpectedRev, judge.ExpectedRev),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", board.ETag)
		core.WriteSuccess(w, map[string]interface{}{"board": board})
		return
	}
	if request.DetachGateJudge != nil {
		judge := request.DetachGateJudge
		board, err := h.store.DetachGateJudge(slug, formations.GateJudgeRequest{
			GateID:    judge.GateID,
			UpdatedBy: patchUpdatedBy(request.UpdatedBy, judge.UpdatedBy),
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  patchExpectedRev(request.ExpectedRev, judge.ExpectedRev),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", board.ETag)
		core.WriteSuccess(w, map[string]interface{}{"board": board})
		return
	}
	if request.CreateMission != nil {
		mission := request.CreateMission
		result, err := h.store.CreateMission(slug, formations.MissionCreateRequest{
			Title:     mission.Title,
			Goal:      mission.Goal,
			BeadID:    mission.BeadID,
			X:         mission.X,
			Y:         mission.Y,
			UpdatedBy: patchUpdatedBy(request.UpdatedBy, mission.UpdatedBy),
		}, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  patchExpectedRev(request.ExpectedRev, mission.ExpectedRev),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", result.Board.ETag)
		core.WriteSuccess(w, result)
		return
	}
	board, err := h.store.UpdateBoardMetadata(slug, formations.BoardMetadataPatch{
		Title:     request.Title,
		UpdatedBy: request.UpdatedBy,
	}, formations.WriteOptions{
		ExpectedETag: r.Header.Get("If-Match"),
		ExpectedRev:  request.ExpectedRev,
	})
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	w.Header().Set("ETag", board.ETag)
	core.WriteSuccess(w, map[string]interface{}{"board": board})
}

func (h *FormationsHandler) GetLayout(w http.ResponseWriter, r *http.Request) {
	slug, err := h.store.ResolveBoardSelector(r.PathValue("board"))
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	layout, err := h.store.ReadLayout(slug)
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	w.Header().Set("ETag", layout.ETag)
	core.WriteSuccess(w, map[string]interface{}{"layout": layout})
}

func (h *FormationsHandler) PatchLayout(w http.ResponseWriter, r *http.Request) {
	var request formationsLayoutPatchRequest
	if !decodeJSONBody(w, r, &request) {
		return
	}
	slug, err := h.store.ResolveBoardSelector(r.PathValue("board"))
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	var layout *formations.LayoutDocument
	if request.Arrange {
		if len(request.Nodes) > 0 || len(request.Edges) > 0 || request.UpdatedAt != "" {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "arrange cannot be combined with other layout mutations")
			return
		}
		layout, err = h.store.ArrangeLayout(slug, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", layout.ETag)
		core.WriteSuccess(w, map[string]interface{}{"layout": layout})
		return
	}
	if len(request.Nodes) > 0 {
		layout, err = h.store.UpdateLayoutNodes(slug, request.Nodes, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", layout.ETag)
		core.WriteSuccess(w, map[string]interface{}{"layout": layout})
		return
	}
	if len(request.Edges) > 0 {
		layout, err = h.store.UpdateLayoutEdges(slug, request.Edges, formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
		})
		if err != nil {
			writeFormationsError(w, err)
			return
		}
		w.Header().Set("ETag", layout.ETag)
		core.WriteSuccess(w, map[string]interface{}{"layout": layout})
		return
	}
	patch := formations.LayoutMetadataPatch{}
	if request.UpdatedAt != "" {
		updatedAt, err := time.Parse(time.RFC3339, request.UpdatedAt)
		if err != nil {
			core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "updatedAt must be RFC3339")
			return
		}
		patch.UpdatedAt = updatedAt
	}
	layout, err = h.store.UpdateLayoutMetadata(slug, patch, formations.WriteOptions{
		ExpectedETag: r.Header.Get("If-Match"),
	})
	if err != nil {
		writeFormationsError(w, err)
		return
	}
	w.Header().Set("ETag", layout.ETag)
	core.WriteSuccess(w, map[string]interface{}{"layout": layout})
}

func patchExpectedRev(parent, child int) int {
	if parent != 0 {
		return parent
	}
	return child
}

func patchUpdatedBy(parent, child string) string {
	if parent != "" {
		return parent
	}
	return child
}

func writeFormationsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, formations.ErrDefinitionPublicationUncertain):
		core.WriteError(w, http.StatusServiceUnavailable, "DEFINITION_PUBLICATION_UNCERTAIN", "Reload both board and layout before any explicit retry")
	case errors.Is(err, formations.ErrInvalidToolMutation):
		core.WriteError(w, http.StatusUnprocessableEntity, "INVALID_TOOL_MUTATION", "Tool mutation is invalid")
	case errors.Is(err, formations.ErrRuntimeAuthorityNonAuthorizing):
		core.WriteError(w, http.StatusServiceUnavailable, "RUNTIME_AUTHORITY_NON_AUTHORIZING", "Formations runtime authority is unavailable")
	case errors.Is(err, formations.ErrConflict):
		core.WriteError(w, http.StatusConflict, "CONFLICT", "Formation definition changed; reload and retry")
	case errors.Is(err, formations.ErrAmbiguousSelector):
		core.WriteError(w, http.StatusBadRequest, "AMBIGUOUS_SELECTOR", err.Error())
	case errors.Is(err, formations.ErrNotFound):
		core.WriteError(w, http.StatusNotFound, "NOT_FOUND", "Formation resource not found")
	case errors.Is(err, formations.ErrPreconditionRequired):
		core.WriteError(w, http.StatusPreconditionRequired, "PRECONDITION_REQUIRED", "If-Match and revision preconditions are required")
	case errors.Is(err, formations.ErrInvalidSlug):
		core.WriteError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid formation slug")
	case errors.Is(err, formations.ErrUnsupportedSchema):
		core.WriteError(w, http.StatusUnprocessableEntity, "UNSUPPORTED_SCHEMA", err.Error())
	case errors.Is(err, formations.ErrLegacyScriptGateRequiresFencedMigration):
		core.WriteError(w, http.StatusUnprocessableEntity, formations.LegacyScriptGateMigrationCode, err.Error())
	case errors.Is(err, formations.ErrLegacyInlineVerificationRequiresMigration):
		core.WriteError(w, http.StatusUnprocessableEntity, formations.LegacyInlineVerificationMigrationCode, err.Error())
	default:
		core.WriteError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
	}
}
