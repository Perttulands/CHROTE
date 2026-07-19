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
			case "tool.params":
				if isArraySection || current == nil || paramsSeen {
					return nil, fmt.Errorf("invalid Tool parameter section")
				}
				active = toolSectionParams
				paramsSeen = true
			case "tool.input":
				if !isArraySection || current == nil {
					return nil, fmt.Errorf("invalid Tool input section")
				}
				current.Inputs = append(current.Inputs, ToolPort{})
				active = toolSectionInput
			case "tool.output":
				if !isArraySection || current == nil {
					return nil, fmt.Errorf("invalid Tool output section")
				}
				current.Outputs = append(current.Outputs, ToolPort{})
				active = toolSectionOutput
			default:
				if tomlSectionIsOrDescendsFrom(section, "tool.params") {
					return nil, fmt.Errorf("nested Tool parameters are not supported")
				}
				active = toolSectionNone
			}
			continue
		}
		if isTOMLHeader(line) {
			active = toolSectionNone
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
			key, value, ok := tomlKeyValue(line.body)
			if !ok {
				continue
			}
			switch key {
			case "id":
				current.ID = value
			case "title":
				current.Title = value
			case "profileId":
				current.ProfileID = value
			case "profileVersion":
				current.ProfileVersion = value
			}
		case toolSectionInput:
			if err := applyToolPortLine(&current.Inputs[len(current.Inputs)-1], line.body); err != nil {
				return nil, err
			}
		case toolSectionOutput:
			if err := applyToolPortLine(&current.Outputs[len(current.Outputs)-1], line.body); err != nil {
				return nil, err
			}
		}
	}

	return tools, nil
}

func parseToolParameterLine(params map[string]any, line string) error {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return nil
	}
	eq := tomlAssignmentIndex(line)
	if eq < 0 {
		return fmt.Errorf("invalid Tool parameter assignment")
	}
	path, ok := parseTOMLKeyPath(line[:eq])
	if !ok || len(path) != 1 {
		return fmt.Errorf("nested Tool parameters are not supported")
	}
	key := path[0]
	if _, exists := params[key]; exists {
		return fmt.Errorf("duplicate Tool parameter %q", key)
	}
	value, err := parseToolParameterScalar(toolParameterValuePart(line, eq))
	if err != nil {
		return err
	}
	params[key] = value
	return nil
}

func parseToolParameterScalar(literal string) (any, error) {
	literal = strings.TrimSpace(literal)
	if len(literal) >= 2 && literal[0] == '"' && literal[len(literal)-1] == '"' {
		value, err := strconv.Unquote(literal)
		if err != nil {
			return nil, fmt.Errorf("invalid Tool parameter string")
		}
		if !utf8.ValidString(value) || strings.IndexByte(value, 0) >= 0 {
			return nil, fmt.Errorf("invalid Tool parameter string")
		}
		return value, nil
	}
	if len(literal) >= 2 && literal[0] == '\'' && literal[len(literal)-1] == '\'' {
		value := literal[1 : len(literal)-1]
		if strings.ContainsRune(value, '\'') || strings.ContainsAny(value, "\r\n") || !utf8.ValidString(value) || strings.IndexByte(value, 0) >= 0 {
			return nil, fmt.Errorf("invalid Tool parameter string")
		}
		return value, nil
	}
	if literal == "true" {
		return true, nil
	}
	if literal == "false" {
		return false, nil
	}
	value, err := strconv.ParseInt(literal, 10, 64)
	if err != nil || value < -maxToolParameterInteger || value > maxToolParameterInteger {
		return nil, fmt.Errorf("invalid Tool parameter scalar")
	}
	return value, nil
}

func toolParameterValuePart(line string, eq int) string {
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

func applyToolPortLine(port *ToolPort, line string) error {
	key, value, ok := tomlKeyValue(line)
	if !ok {
		return nil
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
	case "acceptedMediaTypes":
		port.AcceptedMediaTypes = parseStringArray(value)
	case "required":
		var required bool
		switch strings.TrimSpace(valuePart(line)) {
		case "true":
			required = true
		case "false":
			required = false
		default:
			return fmt.Errorf("invalid Tool port required field")
		}
		port.Required = &required
	case "role":
		port.Role = &value
	}
	return nil
}
