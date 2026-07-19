package formations

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxToolParameterInteger int64 = 9007199254740991

type ToolNode struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	ProfileID      string         `json:"profileId"`
	ProfileVersion string         `json:"profileVersion"`
	Params         map[string]any `json:"params"`
	Inputs         []ToolPort     `json:"inputs"`
	Outputs        []ToolPort     `json:"outputs"`
}

type ToolPort struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Label              string   `json:"label"`
	Direction          string   `json:"direction"`
	Kind               string   `json:"kind"`
	AcceptedMediaTypes []string `json:"acceptedMediaTypes"`
	Required           *bool    `json:"required,omitempty"`
	Role               *string  `json:"role,omitempty"`
}

type toolParseSection uint8

const (
	toolSectionNone toolParseSection = iota
	toolSectionNode
	toolSectionParams
	toolSectionInput
	toolSectionOutput
)

func parseToolNodes(raw []byte) ([]ToolNode, error) {
	var tools []ToolNode
	var current *ToolNode
	active := toolSectionNone
	paramsSeen := false
	var fieldsSeen map[string]bool

	for _, line := range splitLines(raw) {
		trimmed := strings.TrimSpace(line.body)
		section, isSection := tomlLineSectionName(line)
		isArraySection := strings.HasPrefix(trimmed, "[[")
		if isSection {
			switch section {
			case "tool":
				if !isArraySection {
					return nil, fmt.Errorf("invalid Tool section")
				}
				tools = append(tools, ToolNode{Params: make(map[string]any)})
				current = &tools[len(tools)-1]
				active = toolSectionNode
				paramsSeen = false
				fieldsSeen = make(map[string]bool)
			case "tool.params":
				if isArraySection || current == nil || paramsSeen {
					return nil, fmt.Errorf("invalid Tool parameter section")
				}
				active = toolSectionParams
				paramsSeen = true
				fieldsSeen = nil
			case "tool.input":
				if !isArraySection || current == nil {
					return nil, fmt.Errorf("invalid Tool input section")
				}
				current.Inputs = append(current.Inputs, ToolPort{})
				active = toolSectionInput
				fieldsSeen = make(map[string]bool)
			case "tool.output":
				if !isArraySection || current == nil {
					return nil, fmt.Errorf("invalid Tool output section")
				}
				current.Outputs = append(current.Outputs, ToolPort{})
				active = toolSectionOutput
				fieldsSeen = make(map[string]bool)
			default:
				if tomlSectionIsOrDescendsFrom(section, "tool") {
					return nil, fmt.Errorf("unknown Tool section")
				}
				active = toolSectionNone
				fieldsSeen = nil
			}
			continue
		}
		if isTOMLHeader(line) {
			active = toolSectionNone
			fieldsSeen = nil
			continue
		}
		if line.valueContinuation || current == nil {
			continue
		}

		switch active {
		case toolSectionParams:
			if err := parseToolParameterLine(current.Params, line.body); err != nil {
				return nil, err
			}
		case toolSectionNode:
			if err := applyToolNodeLine(current, fieldsSeen, line.body); err != nil {
				return nil, err
			}
		case toolSectionInput:
			if err := applyToolPortLine(&current.Inputs[len(current.Inputs)-1], fieldsSeen, line.body); err != nil {
				return nil, err
			}
		case toolSectionOutput:
			if err := applyToolPortLine(&current.Outputs[len(current.Outputs)-1], fieldsSeen, line.body); err != nil {
				return nil, err
			}
		}
	}

	return tools, nil
}

func parseToolParameterLine(params map[string]any, line string) error {
	key, literal, present, err := parseToolAssignment(line)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	if _, exists := params[key]; exists {
		return fmt.Errorf("duplicate Tool parameter %q", key)
	}
	value, err := parseToolParameterScalar(literal)
	if err != nil {
		return err
	}
	params[key] = value
	return nil
}

func parseToolAssignment(line string) (string, string, bool, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false, nil
	}
	eq := tomlAssignmentIndex(line)
	if eq < 0 {
		return "", "", false, fmt.Errorf("invalid Tool assignment")
	}
	path, ok := parseTOMLKeyPath(line[:eq])
	if !ok || len(path) != 1 {
		return "", "", false, fmt.Errorf("nested Tool fields are not supported")
	}
	literal := strings.TrimSpace(toolValuePart(line, eq))
	if literal == "" {
		return "", "", false, fmt.Errorf("invalid Tool assignment")
	}
	return path[0], literal, true, nil
}

func parseToolParameterScalar(literal string) (any, error) {
	literal = strings.TrimSpace(literal)
	if strings.HasPrefix(literal, "\"") {
		return parseToolBasicString(literal)
	}
	if strings.HasPrefix(literal, "'") {
		return parseToolLiteralString(literal)
	}
	if literal == "true" {
		return true, nil
	}
	if literal == "false" {
		return false, nil
	}
	return parseToolInteger(literal)
}

func parseToolBasicString(literal string) (string, error) {
	if len(literal) < 2 || literal[len(literal)-1] != '"' {
		return "", fmt.Errorf("invalid Tool string")
	}
	body := literal[1 : len(literal)-1]
	for i := 0; i < len(body); i++ {
		if body[i] != '\\' {
			continue
		}
		i++
		if i >= len(body) {
			return "", fmt.Errorf("invalid Tool string")
		}
		switch body[i] {
		case 'b', 't', 'n', 'f', 'r', '"', '\\':
		case 'u':
			if i+4 >= len(body) || !toolHexDigits(body[i+1:i+5]) {
				return "", fmt.Errorf("invalid Tool string")
			}
			i += 4
		case 'U':
			if i+8 >= len(body) || !toolHexDigits(body[i+1:i+9]) {
				return "", fmt.Errorf("invalid Tool string")
			}
			i += 8
		default:
			return "", fmt.Errorf("invalid Tool string")
		}
	}
	value, err := strconv.Unquote(literal)
	if err != nil || !validToolString(value) {
		return "", fmt.Errorf("invalid Tool string")
	}
	return value, nil
}

func parseToolLiteralString(literal string) (string, error) {
	if len(literal) < 2 || literal[len(literal)-1] != '\'' {
		return "", fmt.Errorf("invalid Tool string")
	}
	value := literal[1 : len(literal)-1]
	if strings.ContainsRune(value, '\'') || strings.ContainsAny(value, "\r\n") || !validToolString(value) {
		return "", fmt.Errorf("invalid Tool string")
	}
	return value, nil
}

func validToolString(value string) bool {
	return utf8.ValidString(value) && strings.IndexByte(value, 0) < 0
}

func toolHexDigits(value string) bool {
	for i := 0; i < len(value); i++ {
		if !toolDigitForBase(value[i], 16) {
			return false
		}
	}
	return true
}

func parseToolInteger(literal string) (int64, error) {
	if literal == "" {
		return 0, fmt.Errorf("invalid Tool integer")
	}
	if strings.HasPrefix(literal, "0x") {
		return parseToolPrefixedInteger(literal[2:], 16)
	}
	if strings.HasPrefix(literal, "0o") {
		return parseToolPrefixedInteger(literal[2:], 8)
	}
	if strings.HasPrefix(literal, "0b") {
		return parseToolPrefixedInteger(literal[2:], 2)
	}

	sign := ""
	digits := literal
	if literal[0] == '+' || literal[0] == '-' {
		sign = literal[:1]
		digits = literal[1:]
	}
	normalized, err := normalizeToolDigits(digits, 10)
	if err != nil || len(normalized) > 1 && normalized[0] == '0' {
		return 0, fmt.Errorf("invalid Tool integer")
	}
	value, err := strconv.ParseInt(sign+normalized, 10, 64)
	if err != nil || value < -maxToolParameterInteger || value > maxToolParameterInteger {
		return 0, fmt.Errorf("invalid Tool integer")
	}
	return value, nil
}

func parseToolPrefixedInteger(digits string, base int) (int64, error) {
	normalized, err := normalizeToolDigits(digits, base)
	if err != nil {
		return 0, fmt.Errorf("invalid Tool integer")
	}
	value, err := strconv.ParseUint(normalized, base, 64)
	if err != nil || value > uint64(maxToolParameterInteger) {
		return 0, fmt.Errorf("invalid Tool integer")
	}
	return int64(value), nil
}

func normalizeToolDigits(digits string, base int) (string, error) {
	if digits == "" {
		return "", fmt.Errorf("invalid Tool integer")
	}
	var normalized strings.Builder
	for i := 0; i < len(digits); i++ {
		if digits[i] == '_' {
			if i == 0 || i+1 == len(digits) || !toolDigitForBase(digits[i-1], base) || !toolDigitForBase(digits[i+1], base) {
				return "", fmt.Errorf("invalid Tool integer")
			}
			continue
		}
		if !toolDigitForBase(digits[i], base) {
			return "", fmt.Errorf("invalid Tool integer")
		}
		normalized.WriteByte(digits[i])
	}
	return normalized.String(), nil
}

func toolDigitForBase(ch byte, base int) bool {
	switch {
	case ch >= '0' && ch <= '9':
		return int(ch-'0') < base
	case ch >= 'a' && ch <= 'f':
		return base == 16
	case ch >= 'A' && ch <= 'F':
		return base == 16
	default:
		return false
	}
}

func toolValuePart(line string, eq int) string {
	value := line[eq+1:]
	inBasicString := false
	inLiteralString := false
	escaped := false
	for i := 0; i < len(value); i++ {
		switch {
		case escaped:
			escaped = false
		case inBasicString && value[i] == '\\':
			escaped = true
		case !inLiteralString && value[i] == '"':
			inBasicString = !inBasicString
		case !inBasicString && value[i] == '\'':
			inLiteralString = !inLiteralString
		case !inBasicString && !inLiteralString && value[i] == '#':
			return value[:i]
		}
	}
	return value
}

func applyToolNodeLine(node *ToolNode, fieldsSeen map[string]bool, line string) error {
	key, literal, present, err := parseToolAssignment(line)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	if fieldsSeen[key] {
		return fmt.Errorf("duplicate Tool field %q", key)
	}
	fieldsSeen[key] = true
	if key != "id" && key != "title" && key != "profileId" && key != "profileVersion" {
		return fmt.Errorf("unknown Tool field %q", key)
	}
	value, err := parseToolString(literal)
	if err != nil {
		return err
	}
	switch key {
	case "id":
		node.ID = value
	case "title":
		node.Title = value
	case "profileId":
		node.ProfileID = value
	case "profileVersion":
		node.ProfileVersion = value
	}
	return nil
}

func applyToolPortLine(port *ToolPort, fieldsSeen map[string]bool, line string) error {
	key, literal, present, err := parseToolAssignment(line)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	if fieldsSeen[key] {
		return fmt.Errorf("duplicate Tool port field %q", key)
	}
	fieldsSeen[key] = true
	switch key {
	case "id", "name", "label", "direction", "kind", "role":
		value, err := parseToolString(literal)
		if err != nil {
			return err
		}
		switch key {
		case "id":
			port.ID = value
		case "name":
			port.Name = value
		case "label":
			port.Label = value
		case "direction":
			port.Direction = value
		case "kind":
			port.Kind = value
		case "role":
			port.Role = &value
		}
	case "acceptedMediaTypes":
		values, err := parseToolStringArray(literal)
		if err != nil {
			return err
		}
		port.AcceptedMediaTypes = values
	case "required":
		required, err := parseToolBoolean(literal)
		if err != nil {
			return err
		}
		port.Required = &required
	default:
		return fmt.Errorf("unknown Tool port field %q", key)
	}
	return nil
}

func parseToolString(literal string) (string, error) {
	value, err := parseToolParameterScalar(literal)
	if err != nil {
		return "", err
	}
	stringValue, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("Tool field must be a string")
	}
	return stringValue, nil
}

func parseToolBoolean(literal string) (bool, error) {
	value, err := parseToolParameterScalar(literal)
	if err != nil {
		return false, err
	}
	boolValue, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("Tool field must be a boolean")
	}
	return boolValue, nil
}

func parseToolStringArray(literal string) ([]string, error) {
	literal = strings.TrimSpace(literal)
	if len(literal) < 2 || literal[0] != '[' || literal[len(literal)-1] != ']' {
		return nil, fmt.Errorf("Tool field must be a string array")
	}
	body := strings.TrimSpace(literal[1 : len(literal)-1])
	if body == "" {
		return []string{}, nil
	}
	items, err := splitToolArrayItems(body)
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		value, err := parseToolString(strings.TrimSpace(item))
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func splitToolArrayItems(body string) ([]string, error) {
	var items []string
	start := 0
	inBasicString := false
	inLiteralString := false
	escaped := false
	for i := 0; i < len(body); i++ {
		switch {
		case escaped:
			escaped = false
		case inBasicString && body[i] == '\\':
			escaped = true
		case !inLiteralString && body[i] == '"':
			inBasicString = !inBasicString
		case !inBasicString && body[i] == '\'':
			inLiteralString = !inLiteralString
		case !inBasicString && !inLiteralString && body[i] == ',':
			item := strings.TrimSpace(body[start:i])
			if item == "" {
				return nil, fmt.Errorf("invalid Tool string array")
			}
			items = append(items, item)
			start = i + 1
		}
	}
	if escaped || inBasicString || inLiteralString {
		return nil, fmt.Errorf("invalid Tool string array")
	}
	last := strings.TrimSpace(body[start:])
	if last != "" {
		items = append(items, last)
	} else if len(items) == 0 {
		return nil, fmt.Errorf("invalid Tool string array")
	}
	return items, nil
}
