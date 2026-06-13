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
	body    string
	newline string
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
		if strings.HasPrefix(trimmed, "[") {
			doc.firstSection = i
			break
		}
		if key, ok := topLevelKey(line.body); ok {
			doc.fields[key] = i
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
	return lines
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
		if strings.HasPrefix(trimmed, "[") && d.firstSection == len(d.lines) {
			d.firstSection = i
		}
		if i < d.firstSection {
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
	eq := strings.Index(line, "=")
	if eq < 0 {
		return "", false
	}
	key := strings.TrimSpace(line[:eq])
	if key == "" {
		return "", false
	}
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return "", false
	}
	return key, true
}

func valuePart(line string) string {
	eq := strings.Index(line, "=")
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
	eq := strings.Index(line, "=")
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
