package jsonstrict

import (
	"errors"
	"strconv"
	"unicode/utf8"
)

var (
	ErrInvalidUTF8      = errors.New("JSON is not valid UTF-8")
	ErrInvalidSurrogate = errors.New("JSON contains an unpaired Unicode surrogate")
)

// ValidateUnicode rejects text that encoding/json would otherwise normalize.
// JSON syntax remains the caller's responsibility.
func ValidateUnicode(raw []byte) error {
	if !utf8.Valid(raw) {
		return ErrInvalidUTF8
	}
	for index := 0; index < len(raw); index++ {
		if raw[index] != '"' {
			continue
		}
		for index++; index < len(raw); index++ {
			switch raw[index] {
			case '"':
				goto nextString
			case '\\':
				index++
				if index >= len(raw) || raw[index] != 'u' {
					continue
				}
				first, ok := unicodeCodeUnit(raw, index+1)
				if !ok {
					continue
				}
				index += 4
				switch {
				case first >= 0xd800 && first <= 0xdbff:
					if index+6 >= len(raw) || raw[index+1] != '\\' || raw[index+2] != 'u' {
						return ErrInvalidSurrogate
					}
					second, ok := unicodeCodeUnit(raw, index+3)
					if !ok || second < 0xdc00 || second > 0xdfff {
						return ErrInvalidSurrogate
					}
					index += 6
				case first >= 0xdc00 && first <= 0xdfff:
					return ErrInvalidSurrogate
				}
			}
		}
	nextString:
	}
	return nil
}

func unicodeCodeUnit(raw []byte, start int) (uint16, bool) {
	if start+4 > len(raw) {
		return 0, false
	}
	value, err := strconv.ParseUint(string(raw[start:start+4]), 16, 16)
	return uint16(value), err == nil
}
