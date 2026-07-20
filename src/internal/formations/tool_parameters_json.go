package formations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/chrote/server/internal/jsonstrict"
)

// ParseToolParametersJSON decodes the closed scalar-object grammar shared by
// every public Tool authoring boundary. Descriptor validation remains separate.
func ParseToolParametersJSON(raw []byte) (map[string]any, error) {
	if err := jsonstrict.ValidateUnicode(raw); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	opening, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delimiter, ok := opening.(json.Delim); !ok || delimiter != '{' {
		return nil, fmt.Errorf("Tool parameters must be one JSON object")
	}

	values := make(map[string]any)
	seen := make(map[string]bool)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("Tool parameter key must be a string")
		}
		if seen[key] {
			return nil, fmt.Errorf("duplicate Tool parameter %q", key)
		}
		seen[key] = true

		valueToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		switch value := valueToken.(type) {
		case string, bool:
			values[key] = value
		case json.Number:
			integer, err := strconv.ParseInt(value.String(), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("Tool parameter %q must be a signed 64-bit integer: %w", key, err)
			}
			values[key] = integer
		default:
			return nil, fmt.Errorf("Tool parameter %q must be a string, boolean, or integer", key)
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delimiter, ok := closing.(json.Delim); !ok || delimiter != '}' {
		return nil, fmt.Errorf("Tool parameters object is not closed")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("Tool parameters contain trailing JSON")
		}
		return nil, err
	}
	return values, nil
}
