package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/chrote/server/internal/formations"
)

func TestFormationsHandlerUsesExistingBoardWireAndArrangeSurfacesForTools(t *testing.T) {
	store := formations.NewStore(t.TempDir())
	store.Now = fixedFormationsAPIClock()
	writeFormationsAPIFixture(t, store.BoardPath("tool-parity"), formationsAPIToolParityBoardFixture())
	writeFormationsAPIFixture(t, store.LayoutPath("tool-parity"), formationsAPIToolParityLayoutFixture())
	boardBytesBeforeRead := readFormationsAPIFile(t, store.BoardPath("tool-parity"))
	layoutBytesBeforeRead := readFormationsAPIFile(t, store.LayoutPath("tool-parity"))

	handler := NewFormationsHandlerWithStore(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/api/formations/boards/tool-parity", nil))
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET Tool board status = %d, want %d: %s", getRec.Code, http.StatusOK, getRec.Body.String())
	}
	var getResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Board formations.BoardDocument `json:"board"`
		} `json:"data"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResponse); err != nil {
		t.Fatalf("decode Tool board response: %v\n%s", err, getRec.Body.String())
	}
	if !getResponse.Success {
		t.Fatalf("GET Tool board success=false: %s", getRec.Body.String())
	}
	assertFormationsAPIToolParityProjection(t, getResponse.Data.Board)
	if got := readFormationsAPIFile(t, store.BoardPath("tool-parity")); got != boardBytesBeforeRead {
		t.Fatalf("GET Tool board changed board bytes:\n%s", got)
	}
	if got := readFormationsAPIFile(t, store.LayoutPath("tool-parity")); got != layoutBytesBeforeRead {
		t.Fatalf("GET Tool board changed layout bytes:\n%s", got)
	}

	wireReq := httptest.NewRequest(
		http.MethodPatch,
		"/api/formations/boards/tool-parity",
		bytes.NewBufferString(`{"wireConnection":{"from":"tool_normalize:port_tool_out","to":"tool_sink:port_sink_in"},"expectedRev":4,"updatedBy":"agent:test"}`),
	)
	wireReq.Header.Set("If-Match", getResponse.Data.Board.ETag)
	wireRec := httptest.NewRecorder()
	mux.ServeHTTP(wireRec, wireReq)
	if wireRec.Code != http.StatusOK {
		t.Fatalf("wire Tool output status = %d, want %d: %s", wireRec.Code, http.StatusOK, wireRec.Body.String())
	}
	var wireResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Board formations.BoardDocument `json:"board"`
		} `json:"data"`
	}
	if err := json.Unmarshal(wireRec.Body.Bytes(), &wireResponse); err != nil {
		t.Fatalf("decode Tool wire response: %v\n%s", err, wireRec.Body.String())
	}
	wired := wireResponse.Data.Board
	assertFormationsAPIToolParityProjection(t, wired)
	if !wireResponse.Success || wireRec.Header().Get("ETag") == "" || wireRec.Header().Get("ETag") != wired.ETag {
		t.Fatalf("Tool wire response success=%v header ETag=%q board ETag=%q", wireResponse.Success, wireRec.Header().Get("ETag"), wired.ETag)
	}
	if len(wired.Connections) != 1 || wired.Connections[0].ID == "" ||
		wired.Connections[0].From != "tool_normalize:port_tool_out" || wired.Connections[0].To != "tool_sink:port_sink_in" {
		t.Fatalf("Tool connection = %+v, want exact Tool-output to Tool-input endpoints", wired.Connections)
	}
	if got := readFormationsAPIFile(t, store.LayoutPath("tool-parity")); got != layoutBytesBeforeRead {
		t.Fatalf("Tool wire changed layout bytes:\n%s", got)
	}
	persistedWired, err := store.ReadBoard("tool-parity")
	if err != nil {
		t.Fatalf("read wired Tool board: %v", err)
	}
	if persistedWired.ETag != wired.ETag || len(persistedWired.Connections) != 1 || persistedWired.Connections[0] != wired.Connections[0] {
		t.Fatalf("Tool wire response does not match persisted board: response=%+v persisted=%+v", wired.Connections, persistedWired.Connections)
	}

	unwireReq := httptest.NewRequest(
		http.MethodPatch,
		"/api/formations/boards/tool-parity",
		bytes.NewBufferString(fmt.Sprintf(
			`{"unwireConnection":{"from":"tool_normalize:port_tool_out","to":"tool_sink:port_sink_in"},"expectedRev":%d,"updatedBy":"agent:test"}`,
			wired.Rev,
		)),
	)
	unwireReq.Header.Set("If-Match", wired.ETag)
	unwireRec := httptest.NewRecorder()
	mux.ServeHTTP(unwireRec, unwireReq)
	if unwireRec.Code != http.StatusOK {
		t.Fatalf("unwire Tool input status = %d, want %d: %s", unwireRec.Code, http.StatusOK, unwireRec.Body.String())
	}
	var unwireResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Board formations.BoardDocument `json:"board"`
		} `json:"data"`
	}
	if err := json.Unmarshal(unwireRec.Body.Bytes(), &unwireResponse); err != nil {
		t.Fatalf("decode Tool unwire response: %v\n%s", err, unwireRec.Body.String())
	}
	unwired := unwireResponse.Data.Board
	assertFormationsAPIToolParityProjection(t, unwired)
	if !unwireResponse.Success || len(unwired.Connections) != 0 ||
		unwireRec.Header().Get("ETag") == "" || unwireRec.Header().Get("ETag") != unwired.ETag {
		t.Fatalf("Tool unwire response = success %v connections %+v header ETag %q board ETag %q", unwireResponse.Success, unwired.Connections, unwireRec.Header().Get("ETag"), unwired.ETag)
	}
	if got := readFormationsAPIFile(t, store.LayoutPath("tool-parity")); got != layoutBytesBeforeRead {
		t.Fatalf("Tool unwire changed layout bytes:\n%s", got)
	}

	gateWireReq := httptest.NewRequest(
		http.MethodPatch,
		"/api/formations/boards/tool-parity",
		bytes.NewBufferString(fmt.Sprintf(
			`{"wireConnection":{"from":"tool_normalize:port_tool_out","to":"gate_review:in"},"expectedRev":%d,"updatedBy":"agent:test"}`,
			unwired.Rev,
		)),
	)
	gateWireReq.Header.Set("If-Match", unwired.ETag)
	gateWireRec := httptest.NewRecorder()
	mux.ServeHTTP(gateWireRec, gateWireReq)
	if gateWireRec.Code != http.StatusOK {
		t.Fatalf("wire Tool output to Gate status = %d, want %d: %s", gateWireRec.Code, http.StatusOK, gateWireRec.Body.String())
	}
	var gateWireResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Board formations.BoardDocument `json:"board"`
		} `json:"data"`
	}
	if err := json.Unmarshal(gateWireRec.Body.Bytes(), &gateWireResponse); err != nil {
		t.Fatalf("decode Tool-to-Gate wire response: %v\n%s", err, gateWireRec.Body.String())
	}
	if !gateWireResponse.Success || len(gateWireResponse.Data.Board.Connections) != 1 ||
		gateWireResponse.Data.Board.Connections[0].From != "tool_normalize:port_tool_out" ||
		gateWireResponse.Data.Board.Connections[0].To != "gate_review:in" {
		t.Fatalf("Tool-to-Gate wire response = %+v", gateWireResponse.Data.Board.Connections)
	}

	layout, err := store.ReadLayout("tool-parity")
	if err != nil {
		t.Fatalf("read Tool layout before arrange: %v", err)
	}
	boardBytesBeforeArrange := readFormationsAPIFile(t, store.BoardPath("tool-parity"))
	arrangeReq := httptest.NewRequest(http.MethodPatch, "/api/formations/boards/tool-parity/layout", bytes.NewBufferString(`{"arrange":true}`))
	arrangeReq.Header.Set("If-Match", layout.ETag)
	arrangeRec := httptest.NewRecorder()
	mux.ServeHTTP(arrangeRec, arrangeReq)
	if arrangeRec.Code != http.StatusOK {
		t.Fatalf("arrange Tool board status = %d, want %d: %s", arrangeRec.Code, http.StatusOK, arrangeRec.Body.String())
	}
	var arrangeResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Layout formations.LayoutDocument `json:"layout"`
		} `json:"data"`
	}
	if err := json.Unmarshal(arrangeRec.Body.Bytes(), &arrangeResponse); err != nil {
		t.Fatalf("decode Tool arrange response: %v\n%s", err, arrangeRec.Body.String())
	}
	if !arrangeResponse.Success || arrangeRec.Header().Get("ETag") == "" || arrangeRec.Header().Get("ETag") != arrangeResponse.Data.Layout.ETag {
		t.Fatalf("Tool arrange response success=%v header ETag=%q layout ETag=%q", arrangeResponse.Success, arrangeRec.Header().Get("ETag"), arrangeResponse.Data.Layout.ETag)
	}
	if got := readFormationsAPIFile(t, store.BoardPath("tool-parity")); got != boardBytesBeforeArrange {
		t.Fatalf("Tool arrange changed board bytes:\n%s", got)
	}
	arranged, err := store.ReadLayout("tool-parity")
	if err != nil {
		t.Fatalf("read arranged Tool layout: %v", err)
	}
	if arranged.ETag != arrangeResponse.Data.Layout.ETag {
		t.Fatalf("Tool arrange response ETag %q does not match persisted ETag %q", arrangeResponse.Data.Layout.ETag, arranged.ETag)
	}
	if arrangeResponse.Data.Layout.BoardID != arranged.BoardID ||
		arrangeResponse.Data.Layout.BoardRev != arranged.BoardRev ||
		!reflect.DeepEqual(arrangeResponse.Data.Layout.Nodes, arranged.Nodes) ||
		!reflect.DeepEqual(arrangeResponse.Data.Layout.Edges, arranged.Edges) {
		t.Fatalf("Tool arrange response does not match persisted layout: response=%+v persisted=%+v", arrangeResponse.Data.Layout, arranged)
	}
	positions := make(map[string]formations.LayoutNode, len(arranged.Nodes))
	counts := make(map[string]int, len(arranged.Nodes))
	for _, node := range arranged.Nodes {
		positions[node.ID] = node
		counts[node.ID]++
	}
	if len(arranged.Nodes) != 4 || len(positions) != 4 ||
		counts["mis_main"] != 1 || counts["tool_normalize"] != 1 || counts["tool_sink"] != 1 || counts["gate_review"] != 1 ||
		positions["tool_normalize"].X >= positions["gate_review"].X {
		t.Fatalf("arranged Tool topology nodes=%+v counts=%+v, want four exact board nodes with Tool left of connected Gate", arranged.Nodes, counts)
	}
}

func assertFormationsAPIToolParityProjection(t *testing.T, board formations.BoardDocument) {
	t.Helper()
	if board.Schema != formations.CurrentBoardSchema || len(board.Tools) != 2 {
		t.Fatalf("Tool board projection = schema %d tools %+v", board.Schema, board.Tools)
	}
	tool := board.Tools[0]
	if tool.ID != "tool_normalize" || tool.Title != "Normalize report" ||
		tool.ProfileID != "json.normalize" || tool.ProfileVersion != "1" ||
		len(tool.Params) != 1 || tool.Params["mode"] != "strict" ||
		len(tool.Inputs) != 1 || len(tool.Outputs) != 1 {
		t.Fatalf("Tool projection = %+v, want exact json.normalize@1 definition", tool)
	}
	input := tool.Inputs[0]
	if input.ID != "port_tool_in" || input.Name != "input" || input.Label != "Report" ||
		input.Direction != "input" || input.Kind != "work" ||
		len(input.AcceptedMediaTypes) != 1 || input.AcceptedMediaTypes[0] != "application/json" ||
		input.Required == nil || !*input.Required || input.Role == nil || *input.Role != "data" {
		t.Fatalf("Tool input projection = %+v", input)
	}
	output := tool.Outputs[0]
	if output.ID != "port_tool_out" || output.Name != "output" || output.Label != "Normalized report" ||
		output.Direction != "output" || output.Kind != "work" ||
		len(output.AcceptedMediaTypes) != 1 || output.AcceptedMediaTypes[0] != "application/json" ||
		output.Required != nil || output.Role != nil {
		t.Fatalf("Tool output projection = %+v", output)
	}
	sink := board.Tools[1]
	if sink.ID != "tool_sink" || sink.Title != "Receive normalized report" ||
		sink.ProfileID != "json.normalize" || sink.ProfileVersion != "1" ||
		len(sink.Params) != 1 || sink.Params["mode"] != "strict" ||
		len(sink.Inputs) != 1 || sink.Inputs[0].ID != "port_sink_in" || sink.Inputs[0].Name != "input" ||
		len(sink.Outputs) != 1 || sink.Outputs[0].ID != "port_sink_out" || sink.Outputs[0].Name != "output" {
		t.Fatalf("second Tool projection = %+v, want exact json.normalize@1 sink definition", sink)
	}
}

func formationsAPIToolParityBoardFixture() string {
	return `schema = 2
id = "brd_tool_parity"
slug = "tool-parity"
title = "Tool parity"
rev = 4
updatedBy = "agent:test"
updatedAt = "2026-07-20T00:00:00Z"

[[mission]]
id = "mis_main"
title = "Main"
goal = "Inspect the Tool"
beadId = "ctx-ug7.8.1"

[[tool]]
id = "tool_normalize"
title = "Normalize report"
profileId = "json.normalize"
profileVersion = "1"

[tool.params]
mode = "strict"

[[tool.input]]
id = "port_tool_in"
name = "input"
label = "Report"
direction = "input"
kind = "work"
acceptedMediaTypes = ["application/json"]
required = true
role = "data"

[[tool.output]]
id = "port_tool_out"
name = "output"
label = "Normalized report"
direction = "output"
kind = "work"
acceptedMediaTypes = ["application/json"]

[[tool]]
id = "tool_sink"
title = "Receive normalized report"
profileId = "json.normalize"
profileVersion = "1"

[tool.params]
mode = "strict"

[[tool.input]]
id = "port_sink_in"
name = "input"
label = "Report"
direction = "input"
kind = "work"
acceptedMediaTypes = ["application/json"]
required = true
role = "data"

[[tool.output]]
id = "port_sink_out"
name = "output"
label = "Normalized report"
direction = "output"
kind = "work"
acceptedMediaTypes = ["application/json"]

[[gate]]
id = "gate_review"
title = "Review"
kinds = ["human"]
criterion = "Normalized report is acceptable"
`
}

func formationsAPIToolParityLayoutFixture() string {
	return `schema = 1
boardId = "brd_tool_parity"
boardRev = 4
updatedAt = "2026-07-20T00:00:00Z"

[[node]]
id = "mis_main"
x = 700
y = 420

[[node]]
id = "tool_normalize"
x = 500
y = 420

[[node]]
id = "tool_sink"
x = 400
y = 420

[[node]]
id = "gate_review"
x = 300
y = 420
`
}
