package formations

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestToolBoardParserProjectsFrozenJSONNormalizeDefinition(t *testing.T) {
	store := NewStore(t.TempDir())
	raw := frozenJSONNormalizeBoardFixture()
	writeFixture(t, store.BoardPath("tool-model"), raw)

	board, err := store.ReadBoard("tool-model")
	if err != nil {
		t.Fatalf("read schema-2 Tool board: %v", err)
	}
	if len(board.Tools) != 1 {
		t.Fatalf("Tool count = %d, want 1", len(board.Tools))
	}

	gotJSON, err := json.Marshal(board.Tools[0])
	if err != nil {
		t.Fatalf("marshal Tool projection: %v", err)
	}
	wantJSON := []byte(`{
		"id":"tool_01J9_normalize",
		"title":"Normalize report",
		"profileId":"json.normalize",
		"profileVersion":"1",
		"params":{"mode":"strict"},
		"inputs":[{
			"id":"port_01J9_normalize_in",
			"name":"input",
			"label":"Report",
			"direction":"input",
			"kind":"work",
			"acceptedMediaTypes":["application/json"],
			"required":true,
			"role":"data"
		}],
		"outputs":[{
			"id":"port_01J9_normalize_out",
			"name":"output",
			"label":"Normalized report",
			"direction":"output",
			"kind":"work",
			"acceptedMediaTypes":["application/json"]
		}]
	}`)

	var got, want any
	if err := json.Unmarshal(gotJSON, &got); err != nil {
		t.Fatalf("decode Tool projection: %v", err)
	}
	if err := json.Unmarshal(wantJSON, &want); err != nil {
		t.Fatalf("decode expected Tool projection: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Tool projection = %s\nwant exactly = %s", gotJSON, wantJSON)
	}
}

func TestToolBoardParserPreservesSourceIdentity(t *testing.T) {
	store := NewStore(t.TempDir())
	raw := frozenJSONNormalizeBoardFixture()
	path := store.BoardPath("tool-model")
	writeFixture(t, path, raw)
	wantETag := etag([]byte(raw))
	wantIdentity := operativeFileIdentityForTest(t, path)

	board, err := store.ReadBoard("tool-model")
	if err != nil {
		t.Fatalf("read schema-2 Tool board: %v", err)
	}
	if board.TOML != raw || board.ETag != wantETag {
		t.Fatalf("Tool inspection changed source identity: TOML=%q ETag=%q, want TOML=%q ETag=%q", board.TOML, board.ETag, raw, wantETag)
	}
	if got := readFile(t, path); got != raw {
		t.Fatalf("Tool inspection changed canonical bytes:\n got %q\nwant %q", got, raw)
	}
	if got := operativeFileIdentityForTest(t, path); got != wantIdentity {
		t.Fatalf("Tool inspection replaced operative file identity = %v, want %v", got, wantIdentity)
	}
}

func TestToolBoardParserRejectsInvalidParameterForms(t *testing.T) {
	tests := []struct {
		name   string
		params string
	}{
		{name: "float", params: "[tool.params]\nmode = 1.5\n"},
		{name: "datetime", params: "[tool.params]\nmode = 1979-05-27T07:32:00Z\n"},
		{name: "array", params: "[tool.params]\nmode = [\"strict\"]\n"},
		{name: "inline table", params: "[tool.params]\nmode = { value = \"strict\" }\n"},
		{name: "nested table", params: "[tool.params.mode]\nvalue = \"strict\"\n"},
		{name: "duplicate key", params: "[tool.params]\nmode = \"strict\"\nmode = \"strict\"\n"},
		{name: "unsafe integer", params: "[tool.params]\nmode = 9007199254740992\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewStore(t.TempDir())
			raw := strings.Replace(
				frozenJSONNormalizeBoardFixture(),
				"[tool.params]\nmode = \"strict\"\n",
				tt.params,
				1,
			)
			path := store.BoardPath("tool-model")
			writeFixture(t, path, raw)
			wantIdentity := operativeFileIdentityForTest(t, path)

			_, firstErr := store.ReadBoard("tool-model")
			if firstErr == nil {
				t.Fatalf("ReadBoard accepted %s Tool parameter form", tt.name)
			}
			_, secondErr := store.ReadBoard("tool-model")
			if secondErr == nil || secondErr.Error() != firstErr.Error() {
				t.Fatalf("%s Tool parameter error was not deterministic: first=%v second=%v", tt.name, firstErr, secondErr)
			}
			if got := readFile(t, path); got != raw {
				t.Fatalf("rejected %s Tool parameter changed canonical bytes:\n got %q\nwant %q", tt.name, got, raw)
			}
			if got := operativeFileIdentityForTest(t, path); got != wantIdentity {
				t.Fatalf("rejected %s Tool parameter replaced operative file identity = %v, want %v", tt.name, got, wantIdentity)
			}
		})
	}
}

func frozenJSONNormalizeBoardFixture() string {
	return `schema = 2
id = "brd_01J9_tool_model"
slug = "tool-model"
title = "Tool model"
rev = 4
updatedBy = "agent:test"
updatedAt = "2026-07-19T10:00:00Z"

[[tool]]
id = "tool_01J9_normalize"
title = "Normalize report"
profileId = "json.normalize"
profileVersion = "1"

[tool.params]
mode = "strict"

[[tool.input]]
id = "port_01J9_normalize_in"
name = "input"
label = "Report"
direction = "input"
kind = "work"
acceptedMediaTypes = ["application/json"]
required = true
role = "data"

[[tool.output]]
id = "port_01J9_normalize_out"
name = "output"
label = "Normalized report"
direction = "output"
kind = "work"
acceptedMediaTypes = ["application/json"]
`
}
