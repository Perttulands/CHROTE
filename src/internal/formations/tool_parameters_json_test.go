package formations

import (
	"reflect"
	"testing"
)

func TestParseToolParametersJSONOwnsTheSharedBoundaryGrammar(t *testing.T) {
	valid := []struct {
		raw  string
		want map[string]any
	}{
		{raw: `{}`, want: map[string]any{}},
		{
			raw:  `{"text":"literal","flag":false,"minimum":-9223372036854775808,"maximum":9223372036854775807}`,
			want: map[string]any{"text": "literal", "flag": false, "minimum": int64(-9223372036854775808), "maximum": int64(9223372036854775807)},
		},
		{
			raw:  `{"emoji":"\uD83D\uDE00","replacement":"\uFFFD"}`,
			want: map[string]any{"emoji": "😀", "replacement": "�"},
		},
	}
	for _, test := range valid {
		got, err := ParseToolParametersJSON([]byte(test.raw))
		if err != nil {
			t.Fatalf("ParseToolParametersJSON(%q): %v", test.raw, err)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Fatalf("ParseToolParametersJSON(%q) = %#v, want %#v", test.raw, got, test.want)
		}
	}

	invalidUTF8 := string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'})
	invalid := []string{
		``,
		`[]`,
		`null`,
		`{} {}`,
		`{"mode":"strict","\u006dode":"strict"}`,
		`{"mode":{"value":"strict"}}`,
		`{"mode":["strict"]}`,
		`{"mode":null}`,
		`{"count":1.0}`,
		`{"count":1e0}`,
		`{"count":9223372036854775808}`,
		invalidUTF8,
		`{"mode":"\uD800"}`,
		`{"\uDC00":"strict"}`,
	}
	for _, raw := range invalid {
		if got, err := ParseToolParametersJSON([]byte(raw)); err == nil {
			t.Fatalf("ParseToolParametersJSON(%q) succeeded with %#v", raw, got)
		}
	}
}
