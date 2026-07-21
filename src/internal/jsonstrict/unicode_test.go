package jsonstrict

import "testing"

func TestValidateUnicodeKeepsJSONBoundariesFromNormalizingInvalidText(t *testing.T) {
	valid := [][]byte{
		[]byte(`{"plain":"text"}`),
		[]byte(`{"emoji":"\uD83D\uDE00","replacement":"\uFFFD"}`),
		[]byte(`{"literal":"\\uD800"}`),
	}
	for _, raw := range valid {
		if err := ValidateUnicode(raw); err != nil {
			t.Fatalf("ValidateUnicode(%q): %v", raw, err)
		}
	}

	invalidUTF8 := []byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}
	invalid := [][]byte{
		invalidUTF8,
		[]byte(`{"value":"\uD800"}`),
		[]byte(`{"value":"\uDC00"}`),
		[]byte(`{"value":"\uD800\uD800"}`),
		[]byte(`{"\uD800":"value"}`),
	}
	for _, raw := range invalid {
		if err := ValidateUnicode(raw); err == nil {
			t.Fatalf("ValidateUnicode(%q) succeeded", raw)
		}
	}
}
