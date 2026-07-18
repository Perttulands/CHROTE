package formations

import (
	"bytes"
	"strconv"
	"strings"
)

type tomlDocument struct {
	lines        []tomlLine
	fields       map[string]int
	firstSection int
}

type tomlLine struct {
	body              string
	newline           string
	valueContinuation bool
}

func parseTOMLDocument(raw []byte) *tomlDocument {
	lines := splitLines(raw)
	doc := &tomlDocument{
		lines:        lines,
		fields:       make(map[string]int),
		firstSection: len(lines),
	}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line.body)
		if !line.valueContinuation && strings.HasPrefix(trimmed, "[") {
			doc.firstSection = i
			break
		}
		if !line.valueContinuation {
			if key, ok := topLevelKey(line.body); ok {
				doc.fields[key] = i
			}
		}
	}
	return doc
}

func splitLines(raw []byte) []tomlLine {
	if len(raw) == 0 {
		return nil
	}
	parts := bytes.SplitAfter(raw, []byte("\n"))
	lines := make([]tomlLine, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		line := string(part)
		newline := ""
		if strings.HasSuffix(line, "\n") {
			newline = "\n"
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
			if strings.HasSuffix(string(part), "\r\n") {
				newline = "\r\n"
			}
		}
		lines = append(lines, tomlLine{body: line, newline: newline})
	}
	markTOMLValueContinuations(lines)
	return lines
}

type tomlValueContinuationState struct {
	arrayDepth       int
	inlineTableDepth int
	multilineQuote   byte
}

func (s tomlValueContinuationState) open() bool {
	return s.arrayDepth > 0 || s.inlineTableDepth > 0 || s.multilineQuote != 0
}

func markTOMLValueContinuations(lines []tomlLine) {
	var state tomlValueContinuationState
	for i := range lines {
		lines[i].valueContinuation = state.open()
		body := lines[i].body
		if !lines[i].valueContinuation {
			trimmed := strings.TrimSpace(body)
			if strings.HasPrefix(trimmed, "[") {
				continue
			}
			assignment := tomlAssignmentIndex(body)
			if assignment < 0 {
				continue
			}
			body = body[assignment+1:]
		}
		scanTOMLValueContinuation(&state, body)
	}
}

func scanTOMLValueContinuation(state *tomlValueContinuationState, line string) {
	for i := 0; i < len(line); {
		if state.multilineQuote != 0 {
			delimiter := "\"\"\""
			if state.multilineQuote == '\'' {
				delimiter = "'''"
			}
			if strings.HasPrefix(line[i:], delimiter) {
				state.multilineQuote = 0
				i += len(delimiter)
				continue
			}
			if state.multilineQuote == '"' && line[i] == '\\' {
				i += 2
				continue
			}
			i++
			continue
		}

		switch {
		case strings.HasPrefix(line[i:], "\"\"\""):
			state.multilineQuote = '"'
			i += 3
		case strings.HasPrefix(line[i:], "'''"):
			state.multilineQuote = '\''
			i += 3
		case line[i] == '"':
			i = tomlStringEnd(line, i, '"')
		case line[i] == '\'':
			i = tomlStringEnd(line, i, '\'')
		case line[i] == '#':
			return
		case line[i] == '[':
			state.arrayDepth++
			i++
		case line[i] == ']':
			if state.arrayDepth > 0 {
				state.arrayDepth--
			}
			i++
		case line[i] == '{':
			state.inlineTableDepth++
			i++
		case line[i] == '}':
			if state.inlineTableDepth > 0 {
				state.inlineTableDepth--
			}
			i++
		default:
			i++
		}
	}
}

func tomlStringEnd(line string, start int, quote byte) int {
	escaped := false
	for i := start + 1; i < len(line); i++ {
		switch {
		case escaped:
			escaped = false
		case quote == '"' && line[i] == '\\':
			escaped = true
		case line[i] == quote:
			return i + 1
		}
	}
	return len(line)
}

func (d *tomlDocument) bytes() []byte {
	var b strings.Builder
	for _, line := range d.lines {
		b.WriteString(line.body)
		b.WriteString(line.newline)
	}
	return []byte(b.String())
}

func (d *tomlDocument) stringValue(key string) string {
	lineIndex, ok := d.fields[key]
	if !ok {
		return ""
	}
	value := strings.TrimSpace(valuePart(d.lines[lineIndex].body))
	unquoted, err := strconv.Unquote(value)
	if err != nil {
		return value
	}
	return unquoted
}

func (d *tomlDocument) intValue(key string) int {
	lineIndex, ok := d.fields[key]
	if !ok {
		return 0
	}
	value := strings.TrimSpace(valuePart(d.lines[lineIndex].body))
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}

func (d *tomlDocument) setScalar(key, renderedValue string) {
	if lineIndex, ok := d.fields[key]; ok {
		d.lines[lineIndex].body = replaceScalarValue(d.lines[lineIndex].body, renderedValue)
		return
	}
	insertAt := d.firstSection
	newLine := tomlLine{body: key + " = " + renderedValue, newline: "\n"}
	d.lines = append(d.lines, tomlLine{})
	copy(d.lines[insertAt+1:], d.lines[insertAt:])
	d.lines[insertAt] = newLine
	d.fields = make(map[string]int)
	d.firstSection = len(d.lines)
	for i, line := range d.lines {
		trimmed := strings.TrimSpace(line.body)
		if !line.valueContinuation && strings.HasPrefix(trimmed, "[") && d.firstSection == len(d.lines) {
			d.firstSection = i
		}
		if i < d.firstSection && !line.valueContinuation {
			if fieldKey, ok := topLevelKey(line.body); ok {
				d.fields[fieldKey] = i
			}
		}
	}
}

func topLevelKey(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	eq := tomlAssignmentIndex(line)
	if eq < 0 {
		return "", false
	}
	path, ok := parseTOMLKeyPath(line[:eq])
	if !ok {
		return "", false
	}
	if len(path) != 1 {
		return "", false
	}
	return canonicalTOMLKeyPath(path), true
}

func valuePart(line string) string {
	eq := tomlAssignmentIndex(line)
	if eq < 0 {
		return ""
	}
	value := line[eq+1:]
	if comment := commentIndex(value); comment >= 0 {
		value = value[:comment]
	}
	return value
}

func replaceScalarValue(line, renderedValue string) string {
	eq := tomlAssignmentIndex(line)
	if eq < 0 {
		return line
	}
	valueStart := eq + 1
	for valueStart < len(line) && (line[valueStart] == ' ' || line[valueStart] == '\t') {
		valueStart++
	}
	comment := commentIndex(line[valueStart:])
	valueEnd := len(line)
	if comment >= 0 {
		valueEnd = valueStart + comment
	}
	for valueEnd > valueStart && (line[valueEnd-1] == ' ' || line[valueEnd-1] == '\t') {
		valueEnd--
	}
	return line[:valueStart] + renderedValue + line[valueEnd:]
}

func commentIndex(line string) int {
	inString := false
	escaped := false
	for i, r := range line {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case r == '#' && !inString:
			return i
		}
	}
	return -1
}
