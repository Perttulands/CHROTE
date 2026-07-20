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

const (
	formationsAPIToolBoardSourceSentinel  = `x_api_tool_source_sentinel = "keep"`
	formationsAPIToolLayoutSourceSentinel = `x_api_layout_source_sentinel = "keep"`
)

func TestFormationsHandlerToolCRUDPublishesCanonicalPairs(t *testing.T) {
	tests := []struct {
		name               string
		withLayout         bool
		operation          string
		wantLayout         bool
		wantTool           bool
		wantNullLayoutJSON bool
		assertResult       func(*testing.T, *formationsAPIToolAuthoringHarness, formationsAPIToolMutationEnvelope)
	}{
		{
			name:       "present exact-coordinate create",
			withLayout: true,
			operation:  `"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Created exact","params":{"mode":"strict"},"placement":{"x":123,"y":-456}}`,
			wantLayout: true,
			wantTool:   true,
			assertResult: func(t *testing.T, harness *formationsAPIToolAuthoringHarness, response formationsAPIToolMutationEnvelope) {
				if response.Data.Tool.ID == "" || response.Data.Tool.Title != "Created exact" {
					t.Fatalf("created Tool = %#v, want generated id and exact title", response.Data.Tool)
				}
				position, found := formationsAPIToolLayoutNode(response.Data.Layout, response.Data.Tool.ID)
				if !found || position.X != 123 || position.Y != -456 {
					t.Fatalf("created Tool position = %#v found=%t, want 123,-456", position, found)
				}
				if len(response.Data.Layout.Nodes) != len(harness.layout.Nodes)+1 {
					t.Fatalf("exact create layout nodes = %#v, want retained nodes plus created Tool", response.Data.Layout.Nodes)
				}
				for _, retained := range harness.layout.Nodes {
					got, retainedFound := formationsAPIToolLayoutNode(response.Data.Layout, retained.ID)
					if !retainedFound || !reflect.DeepEqual(got, retained) {
						t.Fatalf("exact create moved retained node %q: got %#v found=%t want %#v", retained.ID, got, retainedFound, retained)
					}
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
			wantTool:   true,
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
			name:         "present default-heuristic create",
			withLayout:   true,
			operation:    `"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Created default present","params":{"mode":"strict"},"placement":{}}`,
			wantLayout:   true,
			wantTool:     true,
			assertResult: assertFormationsAPIToolHintPlacement(112, 112),
		},
		{
			name:         "present predecessor-only heuristic create",
			withLayout:   true,
			operation:    `"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Created after hint","params":{"mode":"strict"},"placement":{"predecessorNodeId":"tool_sink"}}`,
			wantLayout:   true,
			wantTool:     true,
			assertResult: assertFormationsAPIToolHintPlacement(1064, 420),
		},
		{
			name:         "present successor-only heuristic create",
			withLayout:   true,
			operation:    `"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Created before hint","params":{"mode":"strict"},"placement":{"successorNodeId":"gate_review"}}`,
			wantLayout:   true,
			wantTool:     true,
			assertResult: assertFormationsAPIToolHintPlacement(1120, 420),
		},
		{
			name:         "present predecessor-successor heuristic create",
			withLayout:   true,
			operation:    `"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Created between hints","params":{"mode":"strict"},"placement":{"predecessorNodeId":"tool_sink","successorNodeId":"gate_review"}}`,
			wantLayout:   true,
			wantTool:     true,
			assertResult: assertFormationsAPIToolHintPlacement(1036, 420),
		},
		{
			name:       "present title-only update",
			withLayout: true,
			operation:  `"updateTool":{"id":"tool_normalize","title":"Renamed through API"}`,
			wantLayout: true,
			wantTool:   true,
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
			name:               "absent params-only update",
			operation:          `"updateTool":{"id":"tool_normalize","params":{"mode":"strict"}}`,
			wantTool:           true,
			wantNullLayoutJSON: true,
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
			name:               "absent delete",
			operation:          `"deleteTool":{"id":"tool_normalize"}`,
			wantNullLayoutJSON: true,
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
			assertFormationsAPIToolResultKeys(t, recorder.Body.Bytes(), test.wantTool)
			if test.wantNullLayoutJSON {
				assertFormationsAPIToolNullDataLayout(t, recorder.Body.Bytes())
			}
			response := assertFormationsAPIToolMutationSuccess(t, harness, recorder, test.wantLayout)
			if test.wantTool && response.Data.Tool.ID == "" {
				t.Fatalf("Tool create/update returned zero Tool: %s", recorder.Body.String())
			}
			test.assertResult(t, harness, response)
		})
	}
}

func assertFormationsAPIToolHintPlacement(wantX, wantY int) func(*testing.T, *formationsAPIToolAuthoringHarness, formationsAPIToolMutationEnvelope) {
	return func(t *testing.T, harness *formationsAPIToolAuthoringHarness, response formationsAPIToolMutationEnvelope) {
		t.Helper()
		if len(response.Data.Layout.Nodes) != len(harness.layout.Nodes)+1 {
			t.Fatalf("hint create layout node count = %d, want retained nodes plus one", len(response.Data.Layout.Nodes))
		}
		position, found := formationsAPIToolLayoutNode(response.Data.Layout, response.Data.Tool.ID)
		if !found || position.X != wantX || position.Y != wantY {
			t.Fatalf("hint create position = %#v found=%t, want %d,%d", position, found, wantX, wantY)
		}
		for _, retained := range harness.layout.Nodes {
			got, retainedFound := formationsAPIToolLayoutNode(response.Data.Layout, retained.ID)
			if !retainedFound || !reflect.DeepEqual(got, retained) {
				t.Fatalf("hint create moved retained node %q: got %#v found=%t want %#v", retained.ID, got, retainedFound, retained)
			}
		}
		if !reflect.DeepEqual(response.Data.Layout.Edges, harness.layout.Edges) {
			t.Fatalf("hint create changed retained layout edges: got %#v want %#v", response.Data.Layout.Edges, harness.layout.Edges)
		}
		if !reflect.DeepEqual(response.Data.Board.Connections, harness.board.Connections) {
			t.Fatalf("hint create implicitly wired Tool: %#v", response.Data.Board.Connections)
		}
	}
}

func TestFormationsHandlerToolCRUDRejectsInvalidFramesBeforePairMutation(t *testing.T) {
	tests := []struct {
		name        string
		body        func(*formationsAPIToolAuthoringHarness) string
		omitIfMatch bool
		wantStatus  int
		wantCode    string
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
			name: "cross-distinct Tool operations",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Must not create","params":{"mode":"strict"},"placement":{}},"deleteTool":{"id":"tool_normalize"}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "Tool operation rejects unknown top-level field",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"deleteTool":{"id":"tool_normalize"},"unexpected":true`)
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
			name: "Tool plus wire mutation",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"deleteTool":{"id":"tool_normalize"},"wireConnection":{"from":"tool_normalize:port_tool_out","to":"tool_sink:port_sink_in"}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "Tool fence precedes inline verification guard",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"deleteTool":{"id":"tool_normalize"},"setVerification":{"formationId":"fmn_work","criterion":"must not route"}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "Tool fence precedes command-bearing Gate guard",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"deleteTool":{"id":"tool_normalize"},"createGate":{"title":"must not route","command":"printf unsafe"}`)
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
			name: "update omits id",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"updateTool":{"title":"Missing id"}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "update id null",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"updateTool":{"id":null,"title":"Null id"}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "update id empty",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"updateTool":{"id":"","title":"Empty id"}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "update title wrong type",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"updateTool":{"id":"tool_normalize","title":7}`)
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "update params wrong type",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"updateTool":{"id":"tool_normalize","params":[]}`)
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "update rejects nested expected revision",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(fmt.Sprintf(`"updateTool":{"id":"tool_normalize","title":"Nested precondition","expectedRev":%d}`, h.board.Rev))
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "update rejects nested layout expectation",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"updateTool":{"id":"tool_normalize","title":"Nested precondition","layoutExpectation":{"state":"absent"}}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "update rejects unknown field",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"updateTool":{"id":"tool_normalize","title":"Unknown field","unexpected":true}`)
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
			name: "update params null despite title",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"updateTool":{"id":"tool_normalize","title":"Still invalid","params":null}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "duplicate presence-sensitive title",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"updateTool":{"id":"tool_normalize","title":"first","title":"second"}`)
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
			name: "create predecessor hint null",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Null hint","params":{"mode":"strict"},"placement":{"predecessorNodeId":null}}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "create coordinate wrong type",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Wrong coordinate","params":{"mode":"strict"},"placement":{"x":"1","y":2}}`)
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "create params wrong type",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Wrong params","params":[],"placement":{}}`)
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "create rejects strict extra field",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Extra field","params":{"mode":"strict"},"placement":{},"unexpected":true}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "create placement rejects unknown field",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Unknown placement field","params":{"mode":"strict"},"placement":{"unexpected":true}}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "create placement x null",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Null x","params":{"mode":"strict"},"placement":{"x":null,"y":2}}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "create placement y null",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Null y","params":{"mode":"strict"},"placement":{"x":1,"y":null}}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "create placement both coordinates null",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Null coordinates","params":{"mode":"strict"},"placement":{"x":null,"y":null}}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "duplicate exact coordinate",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Duplicate x","params":{"mode":"strict"},"placement":{"x":1,"x":2,"y":3}}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "case-variant exact coordinate",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Alias x","params":{"mode":"strict"},"placement":{"x":1,"X":2,"y":3}}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "delete rejects nested actor",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"deleteTool":{"id":"tool_normalize","updatedBy":"agent:nested"}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "delete omits id",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"deleteTool":{}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "delete id null",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"deleteTool":{"id":null}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "delete id empty",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"deleteTool":{"id":""}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "delete rejects unknown field",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"deleteTool":{"id":"tool_normalize","unexpected":true}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "delete rejects duplicate id",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"deleteTool":{"id":"tool_normalize","id":"tool_sink"}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "delete rejects case-variant id",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"deleteTool":{"id":"tool_normalize","ID":"tool_sink"}`)
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
			name: "absent layout expectation rejects null ETag",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":{"state":"absent","etag":null}}`, h.board.Rev)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "absent layout expectation rejects empty ETag",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":{"state":"absent","etag":""}}`, h.board.Rev)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "absent layout expectation rejects nonempty ETag",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":{"state":"absent","etag":"%s"}}`, h.board.Rev, h.layout.ETag)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "present layout expectation requires ETag",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":{"state":"present"}}`, h.board.Rev)
			},
			wantStatus: http.StatusPreconditionRequired,
			wantCode:   "PRECONDITION_REQUIRED",
		},
		{
			name: "present layout expectation rejects null ETag",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":{"state":"present","etag":null}}`, h.board.Rev)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "present layout expectation rejects empty ETag",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":{"state":"present","etag":""}}`, h.board.Rev)
			},
			wantStatus: http.StatusPreconditionRequired,
			wantCode:   "PRECONDITION_REQUIRED",
		},
		{
			name: "layout expectation requires state",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":{"etag":"%s"}}`, h.board.Rev, h.layout.ETag)
			},
			wantStatus: http.StatusPreconditionRequired,
			wantCode:   "PRECONDITION_REQUIRED",
		},
		{
			name: "layout expectation rejects null state",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":{"state":null}}`, h.board.Rev)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "layout expectation state wrong type",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":{"state":1,"etag":"%s"}}`, h.board.Rev, h.layout.ETag)
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "layout expectation ETag wrong type",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":{"state":"present","etag":1}}`, h.board.Rev)
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "layout expectation rejects duplicate state",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":{"state":"present","state":"absent","etag":"%s"}}`, h.board.Rev, h.layout.ETag)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "layout expectation rejects case-variant ETag",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":{"state":"present","etag":"%s","ETag":"%s"}}`, h.board.Rev, h.layout.ETag, h.layout.ETag)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "layout expectation rejects unknown field",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":{"state":"present","etag":"%s","unexpected":true}}`, h.board.Rev, h.layout.ETag)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "layout expectation rejects unknown state",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":{"state":"unknown","etag":"%s"}}`, h.board.Rev, h.layout.ETag)
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
			name: "zero expected revision",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":0,"layoutExpectation":%s}`, h.layoutExpectationJSON())
			},
			wantStatus: http.StatusPreconditionRequired,
			wantCode:   "PRECONDITION_REQUIRED",
		},
		{
			name: "null expected revision",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":null,"layoutExpectation":%s}`, h.layoutExpectationJSON())
			},
			wantStatus: http.StatusPreconditionRequired,
			wantCode:   "PRECONDITION_REQUIRED",
		},
		{
			name: "wrong-type expected revision",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":"%d","layoutExpectation":%s}`, h.board.Rev, h.layoutExpectationJSON())
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name: "missing layout expectation",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d}`, h.board.Rev)
			},
			wantStatus: http.StatusPreconditionRequired,
			wantCode:   "PRECONDITION_REQUIRED",
		},
		{
			name: "missing If-Match",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"deleteTool":{"id":"tool_normalize"}`)
			},
			omitIfMatch: true,
			wantStatus:  http.StatusPreconditionRequired,
			wantCode:    "PRECONDITION_REQUIRED",
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
			name: "wrong-type layout expectation",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":[]}`, h.board.Rev)
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
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
			ifMatch := harness.board.ETag
			if test.omitIfMatch {
				ifMatch = ""
			}
			recorder := harness.patch(test.body(harness), ifMatch)
			assertFormationsAPIToolError(t, recorder, test.wantStatus, test.wantCode)
			assertFormationsAPIToolPairBytes(t, harness, beforeBoard, beforeLayout)
		})
	}
}

func TestFormationsHandlerToolCRUDRejectsInvalidUnicodeBeforePairMutation(t *testing.T) {
	rawInvalidUTF8 := string([]byte{0xff})
	tests := []struct {
		name       string
		jsonValue  string
		target     string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "raw invalid UTF-8 title",
			jsonValue:  `"Raw ` + rawInvalidUTF8 + `"`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name:       "raw invalid UTF-8 actor",
			jsonValue:  `"agent:` + rawInvalidUTF8 + `"`,
			target:     "actor",
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name:       "raw invalid UTF-8 create title",
			jsonValue:  `"Raw create ` + rawInvalidUTF8 + `"`,
			target:     "create title",
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name:       "raw invalid UTF-8 delete id",
			jsonValue:  `"tool_` + rawInvalidUTF8 + `"`,
			target:     "delete id",
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name:       "lone high surrogate title",
			jsonValue:  `"Unicode \uD800"`,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name:       "lone high surrogate actor",
			jsonValue:  `"agent:\uD800"`,
			target:     "actor",
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name:       "lone high surrogate create title",
			jsonValue:  `"Unicode create \uD800"`,
			target:     "create title",
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name:       "lone high surrogate delete id",
			jsonValue:  `"tool_\uD800"`,
			target:     "delete id",
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name:       "lone low surrogate title",
			jsonValue:  `"Unicode \uDC00"`,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name:       "lone low surrogate actor",
			jsonValue:  `"agent:\uDC00"`,
			target:     "actor",
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name:       "mispaired surrogate title",
			jsonValue:  `"Unicode \uD800\u0041"`,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name:       "mispaired surrogate actor",
			jsonValue:  `"agent:\uD800\u0041"`,
			target:     "actor",
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name:       "high surrogate followed by high surrogate",
			jsonValue:  `"Unicode \uD800\uD801"`,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name:       "high surrogate followed by escaped literal low surrogate",
			jsonValue:  `"Unicode \uD800\\uDC00"`,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newFormationsAPIToolAuthoringHarness(t, true)
			beforeBoard := readFormationsAPIFile(t, harness.store.BoardPath(harness.slug))
			beforeLayout := readFormationsAPIFile(t, harness.store.LayoutPath(harness.slug))
			var body string
			switch test.target {
			case "actor":
				body = fmt.Sprintf(
					`{"updateTool":{"id":"tool_normalize","title":"Unicode actor must not persist"},"expectedRev":%d,"layoutExpectation":%s,"updatedBy":%s}`,
					harness.board.Rev,
					harness.layoutExpectationJSON(),
					test.jsonValue,
				)
			case "create title":
				body = harness.validFrame(fmt.Sprintf(
					`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":%s,"params":{"mode":"strict"},"placement":{}}`,
					test.jsonValue,
				))
			case "delete id":
				body = harness.validFrame(fmt.Sprintf(`"deleteTool":{"id":%s}`, test.jsonValue))
			case "":
				body = harness.validFrame(fmt.Sprintf(`"updateTool":{"id":"tool_normalize","title":%s}`, test.jsonValue))
			default:
				t.Fatalf("unknown Unicode test target %q", test.target)
			}
			recorder := harness.patch(body, harness.board.ETag)
			assertFormationsAPIToolError(t, recorder, test.wantStatus, test.wantCode)
			assertFormationsAPIToolPairBytes(t, harness, beforeBoard, beforeLayout)
		})
	}
}

func TestFormationsHandlerToolUnicodeGrammarPrecedesMissingPreconditions(t *testing.T) {
	rawInvalidUTF8 := string([]byte{0xff})
	for _, test := range []struct {
		name       string
		jsonValue  string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "raw invalid UTF-8",
			jsonValue:  `"Raw ` + rawInvalidUTF8 + `"`,
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
		},
		{
			name:       "lone surrogate",
			jsonValue:  `"Unicode \uD800"`,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newFormationsAPIToolAuthoringHarness(t, true)
			beforeBoard := readFormationsAPIFile(t, harness.store.BoardPath(harness.slug))
			beforeLayout := readFormationsAPIFile(t, harness.store.LayoutPath(harness.slug))
			body := harness.validFrame(fmt.Sprintf(`"updateTool":{"id":"tool_normalize","title":%s}`, test.jsonValue))
			recorder := harness.patch(body, "")
			assertFormationsAPIToolError(t, recorder, test.wantStatus, test.wantCode)
			assertFormationsAPIToolPairBytes(t, harness, beforeBoard, beforeLayout)
		})
	}
}

func TestFormationsHandlerToolCRUDAcceptsIntentionalUnicode(t *testing.T) {
	tests := []struct {
		name          string
		jsonValue     string
		actor         bool
		wantTitle     string
		wantUpdatedBy string
	}{
		{
			name:          "surrogate pair title",
			jsonValue:     `"Unicode \uD83D\uDE80"`,
			wantTitle:     "Unicode 🚀",
			wantUpdatedBy: "agent:api-test",
		},
		{
			name:          "literal replacement rune title",
			jsonValue:     `"Unicode �"`,
			wantTitle:     "Unicode �",
			wantUpdatedBy: "agent:api-test",
		},
		{
			name:          "escaped backslash surrogate text title",
			jsonValue:     `"Unicode \\uD800"`,
			wantTitle:     `Unicode \uD800`,
			wantUpdatedBy: "agent:api-test",
		},
		{
			name:          "escaped active BMP title",
			jsonValue:     `"Unicode \u03A9"`,
			wantTitle:     "Unicode Ω",
			wantUpdatedBy: "agent:api-test",
		},
		{
			name:          "escaped replacement rune title",
			jsonValue:     `"Unicode \uFFFD"`,
			wantTitle:     "Unicode �",
			wantUpdatedBy: "agent:api-test",
		},
		{
			name:          "surrogate pair actor",
			jsonValue:     `"agent:\uD83D\uDE80"`,
			actor:         true,
			wantTitle:     "Unicode actor update",
			wantUpdatedBy: "agent:🚀",
		},
		{
			name:          "literal replacement rune actor",
			jsonValue:     `"agent:�"`,
			actor:         true,
			wantTitle:     "Unicode actor update",
			wantUpdatedBy: "agent:�",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newFormationsAPIToolAuthoringHarness(t, false)
			var body string
			if test.actor {
				body = fmt.Sprintf(
					`{"updateTool":{"id":"tool_normalize","title":"Unicode actor update"},"expectedRev":%d,"layoutExpectation":%s,"updatedBy":%s}`,
					harness.board.Rev,
					harness.layoutExpectationJSON(),
					test.jsonValue,
				)
			} else {
				body = harness.validFrame(fmt.Sprintf(`"updateTool":{"id":"tool_normalize","title":%s}`, test.jsonValue))
			}
			recorder := harness.patch(body, harness.board.ETag)
			response := assertFormationsAPIToolCanonicalAbsentLayoutSuccess(t, harness, recorder, test.wantUpdatedBy)
			responseTool, found := formationsAPIToolBoardTool(response.Data.Board, "tool_normalize")
			if !found || responseTool.Title != test.wantTitle {
				t.Fatalf("Unicode Tool title = %q found=%t, want %q", responseTool.Title, found, test.wantTitle)
			}
			persisted, err := harness.store.ReadBoard(harness.slug)
			if err != nil {
				t.Fatalf("read Unicode Tool board: %v", err)
			}
			persistedTool, found := formationsAPIToolBoardTool(persisted, "tool_normalize")
			if !found || persistedTool.Title != test.wantTitle {
				t.Fatalf("persisted Unicode Tool title = %q found=%t, want %q", persistedTool.Title, found, test.wantTitle)
			}
		})
	}
}

func TestFormationsHandlerToolCRUDPreservesIntentionalReplacementRuneOperationSemantics(t *testing.T) {
	t.Run("create title publishes canonical replacement rune", func(t *testing.T) {
		harness := newFormationsAPIToolAuthoringHarness(t, false)
		body := harness.validFrame(
			`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Created \uFFFD","params":{"mode":"strict"},"placement":{}}`,
		)
		recorder := harness.patch(body, harness.board.ETag)
		response := assertFormationsAPIToolMutationSuccess(t, harness, recorder, true)
		if response.Data.Tool.ID == "" || response.Data.Tool.Title != "Created �" {
			t.Fatalf("created replacement-rune Tool = %#v, want generated id and exact decoded title", response.Data.Tool)
		}
		persisted, err := harness.store.ReadBoard(harness.slug)
		if err != nil {
			t.Fatalf("read replacement-rune create board: %v", err)
		}
		persistedTool, found := formationsAPIToolBoardTool(persisted, response.Data.Tool.ID)
		if !found || persistedTool.Title != "Created �" {
			t.Fatalf("persisted replacement-rune Tool title = %q found=%t, want exact decoded title", persistedTool.Title, found)
		}
	})

	t.Run("delete nonempty replacement-rune id remains not found", func(t *testing.T) {
		harness := newFormationsAPIToolAuthoringHarness(t, true)
		beforeBoard := readFormationsAPIFile(t, harness.store.BoardPath(harness.slug))
		beforeLayout := readFormationsAPIFile(t, harness.store.LayoutPath(harness.slug))
		body := harness.validFrame(`"deleteTool":{"id":"tool_\uFFFD"}`)
		recorder := harness.patch(body, harness.board.ETag)
		assertFormationsAPIToolError(t, recorder, http.StatusNotFound, "NOT_FOUND")
		assertFormationsAPIToolPairBytes(t, harness, beforeBoard, beforeLayout)
	})
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
			name: "stale board revision",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return fmt.Sprintf(`{"updateTool":{"id":"tool_normalize","title":"Must not publish"},"expectedRev":%d,"layoutExpectation":%s}`, h.board.Rev+1, h.layoutExpectationJSON())
			},
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
		{
			name: "invalid Tool title",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"updateTool":{"id":"tool_normalize","title":" \t"}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "invalid Tool placement union",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Invalid placement","params":{"mode":"strict"},"placement":{"x":1,"y":2,"predecessorNodeId":"mis_main"}}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "unknown Tool placement predecessor",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Unknown predecessor","params":{"mode":"strict"},"placement":{"predecessorNodeId":"node_missing"}}`)
			},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "INVALID_TOOL_MUTATION",
		},
		{
			name: "unknown Tool placement successor",
			body: func(h *formationsAPIToolAuthoringHarness) string {
				return h.validFrame(`"createTool":{"profileId":"json.normalize","profileVersion":"1","title":"Unknown successor","params":{"mode":"strict"},"placement":{"successorNodeId":"node_missing"}}`)
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

func TestFormationsHandlerToolCRUDRejectsPresentExpectationWhenLayoutIsAbsent(t *testing.T) {
	harness := newFormationsAPIToolAuthoringHarness(t, false)
	beforeBoard := readFormationsAPIFile(t, harness.store.BoardPath(harness.slug))
	body := fmt.Sprintf(
		`{"deleteTool":{"id":"tool_normalize"},"expectedRev":%d,"layoutExpectation":{"state":"present","etag":"%s"},"updatedBy":"agent:api-test"}`,
		harness.board.Rev,
		strings.Repeat("e", 64),
	)
	recorder := harness.patch(body, harness.board.ETag)
	assertFormationsAPIToolError(t, recorder, http.StatusConflict, "CONFLICT")
	if got := readFormationsAPIFile(t, harness.store.BoardPath(harness.slug)); got != beforeBoard {
		t.Fatalf("inverse absent-layout expectation changed board bytes\n--- before ---\n%s\n--- after ---\n%s", beforeBoard, got)
	}
	if _, err := os.Lstat(harness.store.LayoutPath(harness.slug)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inverse absent-layout expectation materialized layout: %v", err)
	}
}

func TestFormationsHandlerToolGrammarDoesNotChangeLegacyPatchSemantics(t *testing.T) {
	t.Run("metadata success remains compatible", func(t *testing.T) {
		harness := newFormationsAPIToolAuthoringHarness(t, false)
		body := fmt.Sprintf(`{"title":"Legacy still works","expectedRev":%d,"updatedBy":"agent:api-test","legacyExtension":{"preserved":"ignored"}}`, harness.board.Rev)
		recorder := harness.patch(body, harness.board.ETag)
		response := assertFormationsAPIToolMutationSuccess(t, harness, recorder, false)
		if response.Data.Board.Title != "Legacy still works" {
			t.Fatalf("legacy metadata title = %q, want compatibility update", response.Data.Board.Title)
		}
	})

	t.Run("unknown layout expectation shapes remain compatible", func(t *testing.T) {
		for _, test := range []struct {
			name  string
			key   string
			value string
		}{
			{name: "array", key: "layoutExpectation", value: `[]`},
			{name: "string", key: "layoutExpectation", value: `"legacy opaque"`},
			{name: "number", key: "layoutExpectation", value: `7`},
			{name: "boolean", key: "layoutExpectation", value: `true`},
			{name: "object with wrong nested types", key: "layoutExpectation", value: `{"state":[],"etag":7}`},
			{name: "case alias", key: "LayoutExpectation", value: `[]`},
		} {
			t.Run(test.name, func(t *testing.T) {
				harness := newFormationsAPIToolAuthoringHarness(t, false)
				body := fmt.Sprintf(
					`{"title":"Legacy shape %s","expectedRev":%d,"updatedBy":"agent:api-test",%q:%s}`,
					test.name,
					harness.board.Rev,
					test.key,
					test.value,
				)
				recorder := harness.patch(body, harness.board.ETag)
				response := assertFormationsAPIToolMutationSuccess(t, harness, recorder, false)
				if response.Data.Board.Title != "Legacy shape "+test.name {
					t.Fatalf("legacy metadata title = %q, want compatibility update", response.Data.Board.Title)
				}
			})
		}
	})

	t.Run("invalid Unicode decoding remains compatible without Tool operation", func(t *testing.T) {
		rawInvalidUTF8 := string([]byte{0xff})
		for _, test := range []struct {
			name          string
			jsonValue     string
			wantUpdatedBy string
		}{
			{
				name:          "raw invalid UTF-8 actor",
				jsonValue:     `"agent:` + rawInvalidUTF8 + `"`,
				wantUpdatedBy: "agent:�",
			},
			{
				name:          "lone surrogate actor",
				jsonValue:     `"agent:\uD800"`,
				wantUpdatedBy: "agent:�",
			},
		} {
			t.Run(test.name, func(t *testing.T) {
				harness := newFormationsAPIToolAuthoringHarness(t, false)
				body := fmt.Sprintf(
					`{"title":"Legacy Unicode still works","expectedRev":%d,"updatedBy":%s}`,
					harness.board.Rev,
					test.jsonValue,
				)
				recorder := harness.patch(body, harness.board.ETag)
				response := assertFormationsAPIToolCanonicalAbsentLayoutSuccess(t, harness, recorder, test.wantUpdatedBy)
				if response.Data.Board.Title != "Legacy Unicode still works" {
					t.Fatalf("legacy Unicode title = %q, want compatibility update", response.Data.Board.Title)
				}
			})
		}
	})

	t.Run("command Gate keeps legacy migration rejection", func(t *testing.T) {
		harness := newFormationsAPIToolAuthoringHarness(t, true)
		beforeBoard := readFormationsAPIFile(t, harness.store.BoardPath(harness.slug))
		beforeLayout := readFormationsAPIFile(t, harness.store.LayoutPath(harness.slug))
		body := fmt.Sprintf(
			`{"createGate":{"title":"Legacy lint","kinds":["code"],"criterion":"Lint passes","command":"printf unsafe"},"expectedRev":%d}`,
			harness.board.Rev,
		)
		recorder := harness.patch(body, harness.board.ETag)
		assertFormationsAPIToolError(t, recorder, http.StatusUnprocessableEntity, formations.LegacyScriptGateMigrationCode)
		assertFormationsAPIToolPairBytes(t, harness, beforeBoard, beforeLayout)
	})
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

func TestFormationsHandlerPublicationUncertaintyPrecedesInvalidToolMutation(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeFormationsError(recorder, errors.Join(
		fmt.Errorf("candidate detail: %w", formations.ErrInvalidToolMutation),
		fmt.Errorf("publication detail: %w", formations.ErrDefinitionPublicationUncertain),
	))
	assertFormationsAPIToolError(t, recorder, http.StatusServiceUnavailable, "DEFINITION_PUBLICATION_UNCERTAIN")
}

func TestFormationsHandlerDoesNotClassifyGenericToolErrorAsValidation(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeFormationsError(recorder, errors.New("generic persistence failure"))
	assertFormationsAPIToolError(t, recorder, http.StatusInternalServerError, "INTERNAL")
}

func newFormationsAPIToolAuthoringHarness(t *testing.T, withLayout bool) *formationsAPIToolAuthoringHarness {
	t.Helper()
	const slug = "tool-parity"
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath(slug), formationsAPIToolAuthoringBoardFixture())
	if withLayout {
		writeFormationsAPIFixture(t, store.LayoutPath(slug), formationsAPIToolAuthoringLayoutFixture())
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

func formationsAPIToolAuthoringBoardFixture() string {
	return strings.Replace(
		formationsAPIToolParityBoardFixture(),
		"\n\n[[mission]]",
		"\n"+formationsAPIToolBoardSourceSentinel+"\n\n[[mission]]",
		1,
	)
}

func formationsAPIToolAuthoringLayoutFixture() string {
	return strings.Replace(
		formationsAPIToolParityLayoutFixture(),
		"\n\n[[node]]",
		"\n"+formationsAPIToolLayoutSourceSentinel+"\n\n[[node]]",
		1,
	)
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
	for name := range recorder.Header() {
		if strings.Contains(strings.ToLower(name), "etag") && !strings.EqualFold(name, "ETag") {
			t.Fatalf("Tool mutation returned second ETag-like identity header %q", name)
		}
	}
	if response.Data.Board.Rev != harness.board.Rev+1 {
		t.Fatalf("Tool mutation board rev = %d, want one bump from %d", response.Data.Board.Rev, harness.board.Rev)
	}
	persistedBoard, err := harness.store.ReadBoard(harness.slug)
	if err != nil {
		t.Fatalf("read canonical Tool mutation board: %v", err)
	}
	assertFormationsAPIToolCanonicalJSON(t, "board", response.Data.Board, persistedBoard)
	if response.Data.Board.UpdatedBy != "agent:api-test" {
		t.Fatalf("Tool mutation updatedBy = %q, want sole top-level actor", response.Data.Board.UpdatedBy)
	}
	if response.Data.Tool.ID != "" {
		var canonical *formations.ToolNode
		for index := range persistedBoard.Tools {
			if persistedBoard.Tools[index].ID == response.Data.Tool.ID {
				canonical = &persistedBoard.Tools[index]
				break
			}
		}
		if canonical == nil || !reflect.DeepEqual(response.Data.Tool, *canonical) {
			t.Fatalf("returned Tool is not exact canonical board Tool:\n response %#v\ncanonical %#v", response.Data.Tool, canonical)
		}
	}
	if wantLayout {
		persistedLayout, err := harness.store.ReadLayout(harness.slug)
		if err != nil {
			t.Fatalf("read canonical Tool mutation layout: %v", err)
		}
		if response.Data.Layout == nil {
			t.Fatalf("Tool mutation response omitted present canonical layout")
		}
		if response.Data.Layout.BoardRev != response.Data.Board.Rev {
			t.Fatalf("Tool mutation layout boardRev = %d, board rev = %d", response.Data.Layout.BoardRev, response.Data.Board.Rev)
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
	assertFormationsAPIToolRetainedSentinels(t, harness, response)
	return response
}

func assertFormationsAPIToolCanonicalAbsentLayoutSuccess(
	t *testing.T,
	harness *formationsAPIToolAuthoringHarness,
	recorder *httptest.ResponseRecorder,
	wantUpdatedBy string,
) formationsAPIToolMutationEnvelope {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("mutation status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response formationsAPIToolMutationEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode mutation response: %v\n%s", err, recorder.Body.String())
	}
	if !response.Success || response.Data.Board == nil {
		t.Fatalf("mutation response = %#v body=%s", response, recorder.Body.String())
	}
	persisted, err := harness.store.ReadBoard(harness.slug)
	if err != nil {
		t.Fatalf("read canonical mutation board: %v", err)
	}
	assertFormationsAPIToolCanonicalJSON(t, "board", response.Data.Board, persisted)
	if response.Data.Board.UpdatedBy != wantUpdatedBy || persisted.UpdatedBy != wantUpdatedBy {
		t.Fatalf(
			"mutation updatedBy = response %q persisted %q, want %q",
			response.Data.Board.UpdatedBy,
			persisted.UpdatedBy,
			wantUpdatedBy,
		)
	}
	if response.Data.Layout != nil {
		t.Fatalf("absent-layout mutation returned layout %#v", response.Data.Layout)
	}
	if _, err := os.Lstat(harness.store.LayoutPath(harness.slug)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absent-layout mutation materialized layout: %v", err)
	}
	return response
}

func assertFormationsAPIToolNullDataLayout(t *testing.T, body []byte) {
	t.Helper()
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode Tool mutation raw response: %v\n%s", err, body)
	}
	raw, found := envelope.Data["layout"]
	if !found || !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		t.Fatalf("Tool mutation data.layout = %s found=%t, want explicit null in data object: %s", raw, found, body)
	}
}

func assertFormationsAPIToolResultKeys(t *testing.T, body []byte, wantTool bool) {
	t.Helper()
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode Tool mutation result shape: %v\n%s", err, body)
	}
	want := map[string]bool{"board": true, "layout": true}
	if wantTool {
		want["tool"] = true
	} else {
		want["toolId"] = true
	}
	got := make(map[string]bool, len(envelope.Data))
	for key := range envelope.Data {
		got[key] = true
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Tool mutation data keys = %#v, want exact %#v", got, want)
	}
}

func assertFormationsAPIToolRetainedSentinels(
	t *testing.T,
	harness *formationsAPIToolAuthoringHarness,
	response formationsAPIToolMutationEnvelope,
) {
	t.Helper()
	if !strings.Contains(response.Data.Board.TOML, formationsAPIToolBoardSourceSentinel) {
		t.Fatalf("Tool mutation dropped board source sentinel:\n%s", response.Data.Board.TOML)
	}
	missionFound := false
	for _, mission := range response.Data.Board.Missions {
		missionFound = missionFound || mission.ID == "mis_main"
	}
	gateFound := false
	for _, gate := range response.Data.Board.Gates {
		gateFound = gateFound || gate.ID == "gate_review"
	}
	if !missionFound || !gateFound || !formationsAPIToolBoardHasTool(response.Data.Board, "tool_sink") {
		t.Fatalf("Tool mutation dropped retained inventory: missions=%#v gates=%#v tools=%#v", response.Data.Board.Missions, response.Data.Board.Gates, response.Data.Board.Tools)
	}
	if harness.layout == nil {
		return
	}
	if response.Data.Layout == nil || !strings.Contains(response.Data.Layout.TOML, formationsAPIToolLayoutSourceSentinel) {
		t.Fatalf("Tool mutation dropped retained layout source sentinel: %#v", response.Data.Layout)
	}
	for _, id := range []string{"mis_main", "tool_sink", "gate_review"} {
		if _, found := formationsAPIToolLayoutNode(response.Data.Layout, id); !found {
			t.Fatalf("Tool mutation dropped retained layout inventory %q: %#v", id, response.Data.Layout.Nodes)
		}
	}
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

func formationsAPIToolBoardTool(board *formations.BoardDocument, id string) (formations.ToolNode, bool) {
	if board == nil {
		return formations.ToolNode{}, false
	}
	for _, tool := range board.Tools {
		if tool.ID == id {
			return tool, true
		}
	}
	return formations.ToolNode{}, false
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
