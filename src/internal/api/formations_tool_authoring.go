package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/chrote/server/internal/core"
	"github.com/chrote/server/internal/formations"
	"github.com/chrote/server/internal/jsonstrict"
)

type formationsToolParameters struct {
	Values  map[string]any
	Invalid bool
}

func inspectToolFrameUnicode(raw []byte) (bool, error) {
	err := jsonstrict.ValidateUnicode(raw)
	if errors.Is(err, jsonstrict.ErrInvalidSurrogate) {
		return true, nil
	}
	return false, err
}

func (parameters *formationsToolParameters) UnmarshalJSON(raw []byte) error {
	presence, err := inspectToolJSONObject(raw, nil, true)
	if err != nil {
		return err
	}
	values, err := formations.ParseToolParametersJSON(raw)
	if err != nil {
		parameters.Invalid = true
		return nil
	}
	parameters.Values = values
	parameters.Invalid = presence.Invalid
	return nil
}

type formationsToolPlacementRequest struct {
	X                 *int   `json:"x"`
	Y                 *int   `json:"y"`
	PredecessorNodeID string `json:"predecessorNodeId"`
	SuccessorNodeID   string `json:"successorNodeId"`
	presence          toolJSONObjectPresence
}

func (request *formationsToolPlacementRequest) UnmarshalJSON(raw []byte) error {
	type fields formationsToolPlacementRequest
	var decoded fields
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	presence, err := inspectToolJSONObject(raw, []string{"x", "y", "predecessorNodeId", "successorNodeId"}, false)
	if err != nil {
		return err
	}
	*request = formationsToolPlacementRequest(decoded)
	request.presence = presence
	return nil
}

func (request *formationsToolPlacementRequest) invalid() bool {
	if request == nil || request.presence.Invalid {
		return true
	}
	for _, name := range []string{"x", "y", "predecessorNodeId", "successorNodeId"} {
		if request.presence.Null[name] {
			return true
		}
	}
	if request.presence.Occurrences["predecessorNodeId"] == 1 && request.PredecessorNodeID == "" {
		return true
	}
	return request.presence.Occurrences["successorNodeId"] == 1 && request.SuccessorNodeID == ""
}

type formationsToolCreateRequest struct {
	ProfileID      string                          `json:"profileId"`
	ProfileVersion string                          `json:"profileVersion"`
	Title          string                          `json:"title"`
	Params         *formationsToolParameters       `json:"params"`
	Placement      *formationsToolPlacementRequest `json:"placement"`
	presence       toolJSONObjectPresence
}

func (request *formationsToolCreateRequest) UnmarshalJSON(raw []byte) error {
	type fields formationsToolCreateRequest
	var decoded fields
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	presence, err := inspectToolJSONObject(raw, []string{"profileId", "profileVersion", "title", "params", "placement"}, false)
	if err != nil {
		return err
	}
	*request = formationsToolCreateRequest(decoded)
	request.presence = presence
	return nil
}

func (request *formationsToolCreateRequest) invalid() bool {
	if request == nil || request.presence.Invalid {
		return true
	}
	for _, name := range []string{"profileId", "profileVersion", "title", "params", "placement"} {
		if request.presence.Occurrences[name] != 1 || request.presence.Null[name] {
			return true
		}
	}
	return request.Params == nil || request.Params.Invalid || request.Placement.invalid()
}

type formationsToolUpdateRequest struct {
	ID       string                    `json:"id"`
	Title    *string                   `json:"title"`
	Params   *formationsToolParameters `json:"params"`
	presence toolJSONObjectPresence
}

func (request *formationsToolUpdateRequest) UnmarshalJSON(raw []byte) error {
	type fields formationsToolUpdateRequest
	var decoded fields
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	presence, err := inspectToolJSONObject(raw, []string{"id", "title", "params"}, false)
	if err != nil {
		return err
	}
	*request = formationsToolUpdateRequest(decoded)
	request.presence = presence
	return nil
}

func (request *formationsToolUpdateRequest) invalid() bool {
	if request == nil || request.presence.Invalid || request.presence.Occurrences["id"] != 1 || request.presence.Null["id"] || request.ID == "" {
		return true
	}
	titleCount := request.presence.Occurrences["title"]
	paramsCount := request.presence.Occurrences["params"]
	if titleCount+paramsCount == 0 || request.presence.Null["title"] || request.presence.Null["params"] {
		return true
	}
	return request.Params != nil && request.Params.Invalid
}

type formationsToolDeleteRequest struct {
	ID       string `json:"id"`
	presence toolJSONObjectPresence
}

func (request *formationsToolDeleteRequest) UnmarshalJSON(raw []byte) error {
	type fields formationsToolDeleteRequest
	var decoded fields
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	presence, err := inspectToolJSONObject(raw, []string{"id"}, false)
	if err != nil {
		return err
	}
	*request = formationsToolDeleteRequest(decoded)
	request.presence = presence
	return nil
}

func (request *formationsToolDeleteRequest) invalid() bool {
	return request == nil || request.presence.Invalid || request.presence.Occurrences["id"] != 1 || request.presence.Null["id"] || request.ID == ""
}

type formationsToolLayoutExpectationRequest struct {
	State    string `json:"state"`
	ETag     string `json:"etag"`
	presence toolJSONObjectPresence
}

func (request *formationsToolLayoutExpectationRequest) UnmarshalJSON(raw []byte) error {
	type fields formationsToolLayoutExpectationRequest
	var decoded fields
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	presence, err := inspectToolJSONObject(raw, []string{"state", "etag"}, false)
	if err != nil {
		return err
	}
	*request = formationsToolLayoutExpectationRequest(decoded)
	request.presence = presence
	return nil
}

func (request *formationsToolLayoutExpectationRequest) invalid() bool {
	if request == nil || request.presence.Invalid || request.presence.Null["state"] || request.presence.Null["etag"] {
		return true
	}
	switch request.State {
	case "":
		return false
	case formations.LayoutWriteAbsent:
		return request.presence.Occurrences["etag"] != 0
	case formations.LayoutWritePresent:
		return false
	default:
		return true
	}
}

func (request *formationsToolLayoutExpectationRequest) complete() bool {
	if request == nil || request.presence.Occurrences["state"] != 1 || request.State == "" {
		return false
	}
	switch request.State {
	case formations.LayoutWriteAbsent:
		return true
	case formations.LayoutWritePresent:
		return request.presence.Occurrences["etag"] == 1 && request.ETag != ""
	default:
		return false
	}
}

type toolJSONObjectPresence struct {
	Occurrences map[string]int
	Null        map[string]bool
	Invalid     bool
}

func inspectToolJSONObject(raw []byte, allowed []string, allowUnknown bool) (toolJSONObjectPresence, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil {
		return toolJSONObjectPresence{}, err
	}
	if delimiter, ok := opening.(json.Delim); !ok || delimiter != '{' {
		return toolJSONObjectPresence{}, fmt.Errorf("Tool field must be a JSON object")
	}
	allowedFields := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedFields[name] = struct{}{}
	}
	presence := toolJSONObjectPresence{
		Occurrences: make(map[string]int),
		Null:        make(map[string]bool),
	}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return toolJSONObjectPresence{}, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return toolJSONObjectPresence{}, fmt.Errorf("Tool object key must be a string")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return toolJSONObjectPresence{}, err
		}
		presence.Occurrences[key]++
		if presence.Occurrences[key] > 1 {
			presence.Invalid = true
		}
		if !allowUnknown {
			if _, ok := allowedFields[key]; !ok {
				presence.Invalid = true
			}
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			presence.Null[key] = true
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return toolJSONObjectPresence{}, err
	}
	if delimiter, ok := closing.(json.Delim); !ok || delimiter != '}' {
		return toolJSONObjectPresence{}, fmt.Errorf("Tool object is not closed")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return toolJSONObjectPresence{}, fmt.Errorf("Tool object has trailing JSON")
		}
		return toolJSONObjectPresence{}, err
	}
	return presence, nil
}

func isToolOperationKey(key string) bool {
	for _, name := range []string{"createTool", "updateTool", "deleteTool"} {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

func isExactToolFrameKey(key string) bool {
	switch key {
	case "createTool", "updateTool", "deleteTool", "expectedRev", "layoutExpectation", "updatedBy":
		return true
	default:
		return false
	}
}

func (h *FormationsHandler) patchToolBoard(w http.ResponseWriter, r *http.Request, request *formationsBoardPatchRequest) bool {
	if request.ToolOperationOccurrences == 0 {
		return false
	}
	operationCount := 0
	if request.CreateTool != nil {
		operationCount++
	}
	if request.UpdateTool != nil {
		operationCount++
	}
	if request.DeleteTool != nil {
		operationCount++
	}
	invalid := request.ToolFrameInvalid ||
		request.ToolOperationOccurrences != 1 ||
		request.MutationOccurrences != 1 ||
		request.ExpectedRevOccurrences > 1 ||
		request.LayoutExpectationOccurrences > 1 ||
		request.UpdatedByOccurrences > 1 ||
		request.UpdatedByNull ||
		operationCount != 1 ||
		(request.CreateTool != nil && request.CreateTool.invalid()) ||
		(request.UpdateTool != nil && request.UpdateTool.invalid()) ||
		(request.DeleteTool != nil && request.DeleteTool.invalid()) ||
		(request.LayoutExpectation != nil && request.LayoutExpectation.invalid())
	if invalid {
		writeFormationsError(w, formations.ErrInvalidToolMutation)
		return true
	}
	if r.Header.Get("If-Match") == "" ||
		request.ExpectedRevOccurrences != 1 ||
		request.ExpectedRev == 0 ||
		request.LayoutExpectationOccurrences != 1 ||
		request.LayoutExpectation == nil ||
		!request.LayoutExpectation.complete() {
		writeFormationsError(w, formations.ErrPreconditionRequired)
		return true
	}
	slug, err := h.store.ResolveBoardSelector(r.PathValue("board"))
	if err != nil {
		writeFormationsError(w, err)
		return true
	}
	opts := formations.ToolWriteOptions{
		Board: formations.WriteOptions{
			ExpectedETag: r.Header.Get("If-Match"),
			ExpectedRev:  request.ExpectedRev,
		},
		Layout: &formations.LayoutWriteExpectation{
			State: request.LayoutExpectation.State,
			ETag:  request.LayoutExpectation.ETag,
		},
	}
	switch {
	case request.CreateTool != nil:
		create := request.CreateTool
		result, err := h.store.CreateTool(slug, formations.ToolCreateRequest{
			ProfileID:      create.ProfileID,
			ProfileVersion: create.ProfileVersion,
			Title:          create.Title,
			Params:         create.Params.Values,
			Placement: formations.ToolPlacement{
				X:                 create.Placement.X,
				Y:                 create.Placement.Y,
				PredecessorNodeID: create.Placement.PredecessorNodeID,
				SuccessorNodeID:   create.Placement.SuccessorNodeID,
			},
			UpdatedBy: request.UpdatedBy,
		}, opts)
		if err != nil {
			writeFormationsError(w, err)
			return true
		}
		w.Header().Set("ETag", result.Board.ETag)
		core.WriteSuccess(w, result)
	case request.UpdateTool != nil:
		update := request.UpdateTool
		var params *map[string]any
		if update.Params != nil {
			values := update.Params.Values
			params = &values
		}
		result, err := h.store.UpdateTool(slug, formations.ToolUpdateRequest{
			ToolID:    update.ID,
			Title:     update.Title,
			Params:    params,
			UpdatedBy: request.UpdatedBy,
		}, opts)
		if err != nil {
			writeFormationsError(w, err)
			return true
		}
		w.Header().Set("ETag", result.Board.ETag)
		core.WriteSuccess(w, result)
	case request.DeleteTool != nil:
		result, err := h.store.DeleteTool(slug, formations.ToolDeleteRequest{
			ID:        request.DeleteTool.ID,
			UpdatedBy: request.UpdatedBy,
		}, opts)
		if err != nil {
			writeFormationsError(w, err)
			return true
		}
		w.Header().Set("ETag", result.Board.ETag)
		core.WriteSuccess(w, result)
	}
	return true
}
