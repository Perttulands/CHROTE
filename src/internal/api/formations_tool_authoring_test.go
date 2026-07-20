package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/chrote/server/internal/formations"
)

type formationsAPIToolAuthoringHarness struct {
	store  *formations.Store
	mux    *http.ServeMux
	board  *formations.BoardDocument
	layout *formations.LayoutDocument
	slug   string
}

type formationsAPIToolMutationEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		Board  *formations.BoardDocument  `json:"board"`
		Layout *formations.LayoutDocument `json:"layout"`
		Tool   formations.ToolNode        `json:"tool"`
		ToolID string                     `json:"toolId"`
	} `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestFormationsHandlerToolCRUDPublishesCanonicalPairs(t *testing.T) {
	tests := []struct {
		name         string
		withLayout   bool
		operation    string
		wantLayout   bool
		assertResult func(*testing.T, *formationsAPIToolAuthoringHarness, formationsAPIToolMutationEnvelope)
	}{
		{
			name:       "present exact-coordinate create",
			withLayout: true,
			operation:  `"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Created exact","params":{"mode":"strict"},"placement":{"x":123,"y":-456}}`,
			wantLayout: true,
			assertResult: func(t *testing.T, harness *formationsAPIToolAuthoringHarness, response formationsAPIToolMutationEnvelope) {
				if response.Data.Tool.ID == "" || response.Data.Tool.Title != "Created exact" {
					t.Fatalf("created Tool = %#v, want generated id and exact title", response.Data.Tool)
				}
				position, found := formationsAPIToolLayoutNode(response.Data.Layout, response.Data.Tool.ID)
				if !found || position.X != 123 || position.Y != -456 {
					t.Fatalf("created Tool position = %#v found=%t, want 123,-456", position, found)
				}
				if len(response.Data.Layout.Nodes) < len(harness.layout.Nodes) {
					t.Fatalf("exact create layout nodes = %#v, want retained nodes plus created Tool", response.Data.Layout.Nodes)
				}
				if !reflect.DeepEqual(response.Data.Layout.Nodes[:len(harness.layout.Nodes)], harness.layout.Nodes) {
					t.Fatalf("exact create moved retained nodes:\n got  %#v\n want %#v", response.Data.Layout.Nodes, harness.layout.Nodes)
				}
				if !reflect.DeepEqual(response.Data.Board.Connections, harness.board.Connections) {
					t.Fatalf("create implicitly wired Tool: %#v", response.Data.Board.Connections)
				}
			},
		},
		{
			name:       "absent default-heuristic create",
			operation:  `"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Created heuristic","params":{"mode":"strict"},"placement":{}}`,
			wantLayout: true,
			assertResult: func(t *testing.T, harness *formationsAPIToolAuthoringHarness, response formationsAPIToolMutationEnvelope) {
				if response.Data.Tool.ID == "" || len(response.Data.Layout.Nodes) != 1 || response.Data.Layout.Nodes[0].ID != response.Data.Tool.ID {
					t.Fatalf("absent-layout create result = tool %#v layout %#v", response.Data.Tool, response.Data.Layout)
				}
				if !reflect.DeepEqual(response.Data.Board.Connections, harness.board.Connections) {
					t.Fatalf("heuristic create implicitly wired Tool: %#v", response.Data.Board.Connections)
				}
			},
		},
		{
			name:       "present title-only update",
			withLayout: true,
			operation:  `"updateTool":{"id":"tool_normalize","title":"Renamed through API"}`,
			wantLayout: true,
			assertResult: func(t *testing.T, harness *formationsAPIToolAuthoringHarness, response formationsAPIToolMutationEnvelope) {
				if response.Data.Tool.Title != "Renamed through API" || response.Data.Tool.Params["mode"] != "strict" {
					t.Fatalf("title-only update = %#v", response.Data.Tool)
				}
				if !reflect.DeepEqual(response.Data.Layout.Nodes, harness.layout.Nodes) {
					t.Fatalf("title-only update moved layout nodes:\n got  %#v\n want %#v", response.Data.Layout.Nodes, harness.layout.Nodes)
				}
			},
		},
		{
			name:      "absent params-only update",
			operation: `"updateTool":{"id":"tool_normalize","params":{"mode":"strict"}}`,
			assertResult: func(t *testing.T, _ *formationsAPIToolAuthoringHarness, response formationsAPIToolMutationEnvelope) {
				if response.Data.Tool.Title != "Normalize report" || response.Data.Tool.Params["mode"] != "strict" {
					t.Fatalf("params-only update = %#v", response.Data.Tool)
				}
			},
		},
		{
			name:       "present delete",
			withLayout: true,
			operation:  `"deleteTool":{"id":"tool_normalize"}`,
			wantLayout: true,
			assertResult: func(t *testing.T, _ *formationsAPIToolAuthoringHarness, response formationsAPIToolMutationEnvelope) {
				if response.Data.ToolID != "tool_normalize" || formationsAPIToolBoardHasTool(response.Data.Board, "tool_normalize") {
					t.Fatalf("delete result = toolId %q tools %#v", response.Data.ToolID, response.Data.Board.Tools)
				}
				if _, found := formationsAPIToolLayoutNode(response.Data.Layout, "tool_normalize"); found {
					t.Fatalf("present delete retained Tool layout node: %#v", response.Data.Layout.Nodes)
				}
			},
		},
		{
			name:      "absent delete",
			operation: `"deleteTool":{"id":"tool_normalize"}`,
			assertResult: func(t *testing.T, _ *formationsAPIToolAuthoringHarness, response formationsAPIToolMutationEnvelope) {
				if response.Data.ToolID != "tool_normalize" || formationsAPIToolBoardHasTool(response.Data.Board, "tool_normalize") {
					t.Fatalf("absent delete result = toolId %q tools %#v", response.Data.ToolID, response.Data.Board.Tools)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newFormationsAPIToolAuthoringHarness(t, test.withLayout)
			body := fmt.Sprintf(
				`{%s,"expectedRev":%d,"layoutExpectation":%s,"updatedBy":"agent:api-test"}`,
				test.operation,
				harness.board.Rev,
				harness.layoutExpectationJSON(),
			)
			recorder := harness.patch(body, harness.board.ETag)
			response := assertFormationsAPIToolMutationSuccess(t, harness, recorder, test.wantLayout)
			test.assertResult(t, harness, response)
		})
	}
}

func TestFormationsHandlerToolCRUDRejectsInvalidFramesBeforePairMutation(t *testing.T) {
	tests := []struct {
		name       string
		body       func(*formationsAPIToolAuthoringHarness) string
		wantStatus int
		wantCode   string
	}{
		{
			name: "duplicate Tool operation",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"deleteTool":{"id":"tool_normalize"},"deleteTool":{"id":"tool_sink"}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "case-variant Tool alias",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"CreateTool":{"profileId":"json.normalize","profileVersion":"1","title":"Alias","params":{"mode":"strict"},"placement":{}}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "Tool plus legacy mutation",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"deleteTool":{"id":"tool_normalize"},"title":"must not change"`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name:       "null Tool operation",
			body:       func(h *formationsAPIToolAuthoringHarness) string { return h.validFrame(`"deleteTool":null`) },
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "update omits both mutable fields",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"updateTool":{"id":"tool_normalize"}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "update title null despite params",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"updateTool":{"id":"tool_normalize","title":null,"params":{"mode":"strict"}}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "duplicate presence-sensitive params",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"updateTool":{"id":"tool_normalize","params":{"mode":"strict"},"params":{"mode":"lenient"}}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "create omits placement",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Missing placement","params":{"mode":"strict"}}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "create placement null",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Null placement","params":{"mode":"strict"},"placement":null}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "duplicate expected revision",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"expectedRev":%d,"layoutExpectation":%s}`, h.board.Rev, h.board.Rev, h.layoutExpectationJSON())
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "duplicate layout expectation",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":%s,"layoutExpectation":%s}`, h.board.Rev, h.layoutExpectationJSON(), h.layoutExpectationJSON())
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "missing expected revision",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"layoutExpectation":%s}`, h.layoutExpectationJSON())
			},
			wantStatus: http.StatusPreconditionRequired,
			wantCode:   "PRECONDITION_REQUIRED",
		},
		{
			name: "null layout expectation",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":null}`, h.board.Rev)
			},
			wantStatus: http.StatusPreconditionRequired,
			wantCode:   "PRECONDITION_REQUIRED",
		},
		{
			name:       "non-object Tool payload",
			body:       func(h *formationsAPIToolAuthoringHarness) string { return h.validFrame(`"createTool":[]`) },
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "trailing JSON",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"deleteTool":{"id":"tool_normalize"}`) + ` {}`
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newFormationsAPIToolAuthoringHarness(t, true)
			beforeBoard := readFormationsAPIFile(t, harness.store.BoardPath(harness.slug))
			beforeLayout := readFormationsAPIFile(t, harness.store.LayoutPath(harness.slug))
			recorder := harness.patch(test.body(harness), harness.board.ETag)
			assertFormationsAPIToolError(t, recorder, test.wantStatus, test.wantCode)
			assertFormationsAPIToolPairBytes(t, harness, beforeBoard, beforeLayout)
		})
	}
}

func TestFormationsHandlerToolCRUDPreservesStoreErrorIdentity(t *testing.T) {
	tests := []struct {
		name       string
		body       func(*formationsAPIToolAuthoringHarness) string
		ifMatch    func(*formationsAPIToolAuthoringHarness) string
		wantStatus int
		wantCode   string
	}{
		{
			name: "stale board ETag",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"updateTool":{"id":"tool_normalize","title":"Must not publish"}`)
			},
			ifMatch:    func(*formationsAPIToolAuthoringHarness) string { return strings.Repeat("0", 64) },
			wantStatus: http.StatusConflict,
			wantCode:   "CONFLICT",
		},
		{
			name: "stale layout ETag",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":{"state":"present","etag":"%s"}}`, h.board.Rev, strings.Repeat("f", 64))
			},
			wantStatus: http.StatusConflict,
			wantCode:   "CONFLICT",
		},
		{
			name: "inverse layout presence",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":{"state":"absent"}}`, h.board.Rev)
			},
			wantStatus: http.StatusConflict,
			wantCode:   "CONFLICT",
		},
		{
			name: "unknown Tool",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"deleteTool":{"id":"tool_missing"}`)
			},
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
		{
			name: "invalid Tool tuple",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"createTool":{"profileId":"unknown.profile","profileVersion":"1","title":"Invalid","params":{},"placement":{}}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "invalid Tool params",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Invalid params","params":{"mode":"lenient"},"placement":{}}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newFormationsAPIToolAuthoringHarness(t, true)
			beforeBoard := readFormationsAPIFile(t, harness.store.BoardPath(harness.slug))
			beforeLayout := readFormationsAPIFile(t, harness.store.LayoutPath(harness.slug))
			ifMatch := harness.board.ETag
			if test.ifMatch != nil {
				ifMatch = test.ifMatch(harness)
			}
			recorder := harness.patch(test.body(harness), ifMatch)
			assertFormationsAPIToolError(t, recorder, test.wantStatus, test.wantCode)
			assertFormationsAPIToolPairBytes(t, harness, beforeBoard, beforeLayout)
		})
	}
}

func TestFormationsHandlerMapsDefinitionPublicationUncertaintySafely(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeFormationsError(recorder, fmt.Errorf("private publication detail: %w", formations.ErrDefinitionPublicationUncertain))
	response := assertFormationsAPIToolError(t, recorder, http.StatusServiceUnavailable, "DEFINITION_PUBLICATION_UNCERTAIN")
	message := strings.ToLower(response.Error.Message)
	if !strings.Contains(message, "reload both board and layout") || !strings.Contains(message, "explicit retry") {
		t.Fatalf("uncertainty message = %q, want safe reload-before-retry guidance", response.Error.Message)
	}
	if strings.Contains(response.Error.Message, "private publication detail") || strings.Contains(response.Error.Message, formations.ErrDefinitionPublicationUncertain.Error()) {
		t.Fatalf("uncertainty response leaked raw error: %q", response.Error.Message)
	}
}

func TestFormationsHandlerMapsWrappedInvalidToolMutation(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeFormationsError(recorder, fmt.Errorf("candidate detail: %w", formations.ErrInvalidToolMutation))
	assertFormationsAPIToolError(t, recorder, http.StatusUnprocessableEntity, "INVALID_TOOL_MUTATION")
}

func newFormationsAPIToolAuthoringHarness(t *testing.T, withLayout bool) *formationsAPIToolAuthoringHarness {
	t.Helper()
	const slug = "tool-parity"
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath(slug), formationsAPIToolParityBoardFixture())
	if withLayout {
		writeFormationsAPIFixture(t, store.LayoutPath(slug), formationsAPIToolParityLayoutFixture())
	}
	board, err := store.ReadBoard(slug)
	if err != nil {
		t.Fatalf("read Tool authoring board: %v", err)
	}
	var layout *formations.LayoutDocument
	if withLayout {
		layout, err = store.ReadLayout(slug)
		if err != nil {
			t.Fatalf("read Tool authoring layout: %v", err)
		}
	}
	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return &formationsAPIToolAuthoringHarness{store: store, mux: mux, board: board, layout: layout, slug: slug}
}

func (h *formationsAPIToolAuthoringHarness) layoutExpectationJSON() string {
	if h.layout == nil {
		return `{"state":"absent"}`
	}
	return fmt.Sprintf(`{"state":"present","etag":%q}`, h.layout.ETag)
}

func (h *formationsAPIToolAuthoringHarness) validFrame(operation string) string {
	return fmt.Sprintf(`{%s,"expectedRev":%d,"layoutExpectation":%s,"updatedBy":"agent:api-test"}`, operation, h.board.Rev, h.layoutExpectationJSON())
}

func (h *formationsAPIToolAuthoringHarness) patch(body, ifMatch string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/"+h.slug, bytes.NewBufferString(body))
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	recorder := httptest.NewRecorder()
	h.mux.ServeHTTP(recorder, req)
	return recorder
}

func assertFormationsAPIToolMutationSuccess(
	t *testing.T,
	harness *formationsAPIToolAuthoringHarness,
	recorder *httptest.ResponseRecorder,
	wantLayout bool,
) formationsAPIToolMutationEnvelope {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("Tool mutation status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response formationsAPIToolMutationEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode Tool mutation response: %v\n%s", err, recorder.Body.String())
	}
	if !response.Success || response.Data.Board == nil {
		t.Fatalf("Tool mutation response = %#v body=%s", response, recorder.Body.String())
	}
	if got := recorder.Header().Get("ETag"); got == "" || got != response.Data.Board.ETag {
		t.Fatalf("Tool mutation header ETag = %q, board ETag = %q", got, response.Data.Board.ETag)
	}
	persistedBoard, err := harness.store.ReadBoard(harness.slug)
	if err != nil {
		t.Fatalf("read canonical Tool mutation board: %v", err)
	}
	assertFormationsAPIToolCanonicalJSON(t, "board", response.Data.Board, persistedBoard)
	if wantLayout {
		persistedLayout, err := harness.store.ReadLayout(harness.slug)
		if err != nil {
			t.Fatalf("read canonical Tool mutation layout: %v", err)
		}
		if response.Data.Layout == nil {
			t.Fatalf("Tool mutation response omitted present canonical layout")
		}
		assertFormationsAPIToolCanonicalJSON(t, "layout", response.Data.Layout, persistedLayout)
	} else {
		if response.Data.Layout != nil {
			t.Fatalf("absent-layout Tool mutation returned layout %#v", response.Data.Layout)
		}
		if _, err := os.Lstat(harness.store.LayoutPath(harness.slug)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("absent-layout Tool mutation materialized layout: %v", err)
		}
	}
	return response
}

func assertFormationsAPIToolCanonicalJSON(t *testing.T, kind string, got, want any) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal Tool mutation %s response: %v", kind, err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal persisted Tool mutation %s: %v", kind, err)
	}
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("Tool mutation %s response is not canonical:\n response %s\npersisted %s", kind, gotJSON, wantJSON)
	}
}

func assertFormationsAPIToolError(t *testing.T, recorder *httptest.ResponseRecorder, wantStatus int, wantCode string) formationsAPIToolMutationEnvelope {
	t.Helper()
	var response formationsAPIToolMutationEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode Tool mutation error: %v\n%s", err, recorder.Body.String())
	}
	if recorder.Code != wantStatus || response.Success || response.Error == nil || response.Error.Code != wantCode {
		t.Fatalf("Tool mutation error = status %d response %#v, want %d/%s", recorder.Code, response, wantStatus, wantCode)
	}
	return response
}

func assertFormationsAPIToolPairBytes(t *testing.T, harness *formationsAPIToolAuthoringHarness, wantBoard, wantLayout string) {
	t.Helper()
	if got := readFormationsAPIFile(t, harness.store.BoardPath(harness.slug)); got != wantBoard {
		t.Fatalf("rejected Tool mutation changed board bytes\n--- before ---\n%s\n--- after ---\n%s", wantBoard, got)
	}
	if got := readFormationsAPIFile(t, harness.store.LayoutPath(harness.slug)); got != wantLayout {
		t.Fatalf("rejected Tool mutation changed layout bytes\n--- before ---\n%s\n--- after ---\n%s", wantLayout, got)
	}
}

func formationsAPIToolBoardHasTool(board *formations.BoardDocument, id string) bool {
	if board == nil {
		return false
	}
	for _, tool := range board.Tools {
		if tool.ID == id {
			return true
		}
	}
	return false
}

func formationsAPIToolLayoutNode(layout *formations.LayoutDocument, id string) (formations.LayoutNode, bool) {
	if layout == nil {
		return formations.LayoutNode{}, false
	}
	for _, node := range layout.Nodes {
		if node.ID == id {
			return node, true
		}
	}
	return formations.LayoutNode{}, false
}
