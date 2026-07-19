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
	if len(board.Tools) != 2 {
		t.Fatalf("Tool count = %d, want 2", len(board.Tools))
	}

	gotJSON, err := json.Marshal(board.Tools)
	if err != nil {
		t.Fatalf("marshal Tool projection: %v", err)
	}
	wantJSON := []byte(`[{
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
	},{
		"id":"tool_01J9_normalize_archive",
		"title":"Normalize archive",
		"profileId":"json.normalize",
		"profileVersion":"1",
		"params":{"mode":"strict"},
		"inputs":[{
			"id":"port_01J9_normalize_archive_in",
			"name":"input",
			"label":"Report",
			"direction":"input",
			"kind":"work",
			"acceptedMediaTypes":["application/json"],
			"required":true,
			"role":"data"
		}],
		"outputs":[{
			"id":"port_01J9_normalize_archive_out",
			"name":"output",
			"label":"Normalized report",
			"direction":"output",
			"kind":"work",
			"acceptedMediaTypes":["application/json"]
		}]
	}]`)

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

func TestToolParameterScalarParserAcceptsFrozenDomain(t *testing.T) {
	tests := []struct {
		name     string
		literal  string
		wantJSON string
	}{
		{name: "string", literal: `"strict"`, wantJSON: `"strict"`},
		{name: "literal string", literal: `'strict'`, wantJSON: `"strict"`},
		{name: "literal string with hash", literal: `'strict#mode'`, wantJSON: `"strict#mode"`},
		{name: "boolean true", literal: "true", wantJSON: "true"},
		{name: "boolean false", literal: "false", wantJSON: "false"},
		{name: "minimum safe integer", literal: "-9007199254740991", wantJSON: "-9007199254740991"},
		{name: "maximum safe integer", literal: "9007199254740991", wantJSON: "9007199254740991"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseToolParameterScalar(tt.literal)
			if err != nil {
				t.Fatalf("parse approved Tool parameter scalar %q: %v", tt.literal, err)
			}
			gotJSON, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal approved Tool parameter scalar %q: %v", tt.literal, err)
			}
			if string(gotJSON) != tt.wantJSON {
				t.Fatalf("Tool parameter scalar %q projected as %s, want %s", tt.literal, gotJSON, tt.wantJSON)
			}
		})
	}
}

func TestToolParameterLinePreservesHashInsideLiteralString(t *testing.T) {
	params := make(map[string]any)
	if err := parseToolParameterLine(params, "mode = 'strict#mode' # trailing comment"); err != nil {
		t.Fatalf("parse Tool literal-string parameter line: %v", err)
	}
	gotJSON, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal Tool literal-string parameter map: %v", err)
	}
	if got, want := string(gotJSON), `{"mode":"strict#mode"}`; got != want {
		t.Fatalf("Tool literal-string parameter map = %s, want %s", got, want)
	}
}

func TestToolParameterScalarParserRejectsBothUnsafeIntegerBounds(t *testing.T) {
	for _, literal := range []string{"-9007199254740992", "9007199254740992"} {
		t.Run(literal, func(t *testing.T) {
			if _, err := parseToolParameterScalar(literal); err == nil {
				t.Fatalf("Tool parameter scalar parser accepted unsafe integer %s", literal)
			}
		})
	}
}

func TestToolBoardParserRejectsNonTOMLRequiredBooleansWithoutMutation(t *testing.T) {
	for _, literal := range []string{"1", "TRUE", `"true"`} {
		t.Run(literal, func(t *testing.T) {
			store := NewStore(t.TempDir())
			raw := strings.Replace(frozenJSONNormalizeBoardFixture(), "required = true\n", "required = "+literal+"\n", 1)
			path := store.BoardPath("tool-model")
			writeFixture(t, path, raw)
			wantIdentity := operativeFileIdentityForTest(t, path)

			_, firstErr := store.ReadBoard("tool-model")
			if firstErr == nil {
				t.Fatalf("ReadBoard accepted non-TOML required boolean %s", literal)
			}
			_, secondErr := store.ReadBoard("tool-model")
			if secondErr == nil || secondErr.Error() != firstErr.Error() {
				t.Fatalf("required boolean %s error was not deterministic: first=%v second=%v", literal, firstErr, secondErr)
			}
			if got := readFile(t, path); got != raw {
				t.Fatalf("rejected required boolean %s changed canonical bytes:\n got %q\nwant %q", literal, got, raw)
			}
			if got := operativeFileIdentityForTest(t, path); got != wantIdentity {
				t.Fatalf("rejected required boolean %s replaced operative file identity = %v, want %v", literal, got, wantIdentity)
			}
		})
	}
}

func TestToolBoardParserRejectsInvalidParameterForms(t *testing.T) {
	tests := []struct {
		name          string
		params        string
		secondSibling bool
	}{
		{name: "float", params: "[tool.params]\nmode = 1.5\n"},
		{name: "datetime", params: "[tool.params]\nmode = 1979-05-27T07:32:00Z\n"},
		{name: "array", params: "[tool.params]\nmode = [\"strict\"]\n"},
		{name: "inline table", params: "[tool.params]\nmode = { value = \"strict\" }\n"},
		{name: "nested table", params: "[tool.params.mode]\nvalue = \"strict\"\n"},
		{name: "dotted nested value", params: "[tool.params]\nmode.value = \"strict\"\n"},
		{name: "duplicate key", params: "[tool.params]\nmode = \"strict\"\nmode = \"strict\"\n"},
		{name: "unsafe negative integer", params: "[tool.params]\nmode = -9007199254740992\n"},
		{name: "unsafe positive integer", params: "[tool.params]\nmode = 9007199254740992\n"},
		{name: "second sibling invalid scalar", params: "[tool.params]\nmode = 1.5\n", secondSibling: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewStore(t.TempDir())
			const frozenParams = "[tool.params]\nmode = \"strict\"\n"
			raw := frozenJSONNormalizeBoardFixture()
			if count := strings.Count(raw, frozenParams); count != 2 {
				t.Fatalf("frozen Tool fixture parameter section count = %d, want 2", count)
			}
			if tt.secondSibling {
				index := strings.LastIndex(raw, frozenParams)
				raw = raw[:index] + tt.params + raw[index+len(frozenParams):]
			} else {
				raw = strings.Replace(raw, frozenParams, tt.params, 1)
			}
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

[[tool]]
id = "tool_01J9_normalize_archive"
title = "Normalize archive"
profileId = "json.normalize"
profileVersion = "1"

[tool.params]
mode = "strict"

[[tool.input]]
id = "port_01J9_normalize_archive_in"
name = "input"
label = "Report"
direction = "input"
kind = "work"
acceptedMediaTypes = ["application/json"]
required = true
role = "data"

[[tool.output]]
id = "port_01J9_normalize_archive_out"
name = "output"
label = "Normalized report"
direction = "output"
kind = "work"
acceptedMediaTypes = ["application/json"]
`
}
