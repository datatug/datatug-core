package walkthrough

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// maxJSONDepth bounds parser recursion against a pathological 64 KiB input.
// It is a parser-safety guard, not a declaration-content limit.
const maxJSONDepth = 256

type jsonKind uint8

const (
	jsonNull jsonKind = iota
	jsonBoolean
	jsonNumber
	jsonString
	jsonArray
	jsonObject
)

// jsonValue is the raw syntax tree. It deliberately preserves object member
// order and the exact number lexeme, and it has already applied the shared
// escaped-surrogate normalization to strings and keys.
type jsonValue struct {
	kind   jsonKind
	path   string
	bool   bool
	number string
	text   string
	array  []*jsonValue
	object []jsonMember
}

type jsonMember struct {
	key   string
	value *jsonValue
}

func (v *jsonValue) member(name string) (*jsonValue, bool) {
	for index := range v.object {
		if v.object[index].key == name {
			return v.object[index].value, true
		}
	}
	return nil, false
}

type jsonParser struct {
	data []byte
	pos  int
}

// parseJSON parses exactly one JSON value with the strictness the declaration
// contract needs: valid UTF-8, no duplicate keys (after escape and surrogate
// normalization), no trailing data, no unescaped controls and no lenient
// number spellings.
func parseJSON(data []byte) (*jsonValue, *Issue) {
	parser := &jsonParser{data: data}
	value, issue := parser.value("", 0)
	if issue != nil {
		return nil, issue
	}
	parser.skipSpace()
	if parser.pos != len(parser.data) {
		return nil, &Issue{
			Code:    CodeTrailingData,
			Path:    "",
			Message: "trailing data after the JSON value",
		}
	}
	return value, nil
}

func (p *jsonParser) value(path string, depth int) (*jsonValue, *Issue) {
	if depth > maxJSONDepth {
		return nil, &Issue{Code: CodeInvalidJSON, Path: path, Message: "JSON nesting is too deep"}
	}
	p.skipSpace()
	if p.pos >= len(p.data) {
		return nil, &Issue{Code: CodeInvalidJSON, Path: path, Message: "unexpected end of JSON"}
	}
	switch char := p.data[p.pos]; {
	case char == '{':
		return p.object(path, depth)
	case char == '[':
		return p.array(path, depth)
	case char == '"':
		text, issue := p.stringValue(path)
		if issue != nil {
			return nil, issue
		}
		return &jsonValue{kind: jsonString, path: path, text: text}, nil
	case char == 't':
		return p.literal(path, "true", jsonBoolean, true)
	case char == 'f':
		return p.literal(path, "false", jsonBoolean, false)
	case char == 'n':
		return p.literal(path, "null", jsonNull, false)
	case char == '-' || (char >= '0' && char <= '9'):
		return p.number(path)
	default:
		return nil, &Issue{
			Code:    CodeInvalidJSON,
			Path:    path,
			Message: "unexpected character in JSON",
		}
	}
}

func (p *jsonParser) object(path string, depth int) (*jsonValue, *Issue) {
	p.pos++ // {
	value := &jsonValue{kind: jsonObject, path: path}
	seen := make(map[string]bool)
	p.skipSpace()
	if p.peek() == '}' {
		p.pos++
		return value, nil
	}
	for {
		p.skipSpace()
		if p.peek() != '"' {
			return nil, &Issue{Code: CodeInvalidJSON, Path: path, Message: "expected an object key"}
		}
		key, issue := p.stringValue(path)
		if issue != nil {
			return nil, issue
		}
		keyPath := pointerChild(path, key)
		if seen[key] {
			return nil, &Issue{
				Code:    CodeDuplicateKey,
				Path:    keyPath,
				Message: "duplicate JSON object key",
			}
		}
		seen[key] = true
		p.skipSpace()
		if p.peek() != ':' {
			return nil, &Issue{Code: CodeInvalidJSON, Path: keyPath, Message: "expected ':' after an object key"}
		}
		p.pos++
		memberValue, issue := p.value(keyPath, depth+1)
		if issue != nil {
			return nil, issue
		}
		value.object = append(value.object, jsonMember{key: key, value: memberValue})
		p.skipSpace()
		switch p.peek() {
		case ',':
			p.pos++
		case '}':
			p.pos++
			return value, nil
		default:
			return nil, &Issue{Code: CodeInvalidJSON, Path: path, Message: "expected ',' or '}' in an object"}
		}
	}
}

func (p *jsonParser) array(path string, depth int) (*jsonValue, *Issue) {
	p.pos++ // [
	value := &jsonValue{kind: jsonArray, path: path}
	p.skipSpace()
	if p.peek() == ']' {
		p.pos++
		return value, nil
	}
	for index := 0; ; index++ {
		item, issue := p.value(pointerIndex(path, index), depth+1)
		if issue != nil {
			return nil, issue
		}
		value.array = append(value.array, item)
		p.skipSpace()
		switch p.peek() {
		case ',':
			p.pos++
		case ']':
			p.pos++
			return value, nil
		default:
			return nil, &Issue{Code: CodeInvalidJSON, Path: path, Message: "expected ',' or ']' in an array"}
		}
	}
}

func (p *jsonParser) literal(path, spelling string, kind jsonKind, boolean bool) (*jsonValue, *Issue) {
	if !bytes.HasPrefix(p.data[p.pos:], []byte(spelling)) {
		return nil, &Issue{Code: CodeInvalidJSON, Path: path, Message: "invalid JSON literal"}
	}
	p.pos += len(spelling)
	return &jsonValue{kind: kind, path: path, bool: boolean}, nil
}

func (p *jsonParser) number(path string) (*jsonValue, *Issue) {
	start := p.pos
	if p.peek() == '-' {
		p.pos++
	}
	if p.pos >= len(p.data) {
		return nil, &Issue{Code: CodeInvalidJSON, Path: path, Message: "invalid JSON number"}
	}
	switch char := p.data[p.pos]; {
	case char == '0':
		p.pos++
	case char >= '1' && char <= '9':
		p.consumeDigits()
	default:
		return nil, &Issue{Code: CodeInvalidJSON, Path: path, Message: "invalid JSON number"}
	}
	if p.peek() == '.' {
		p.pos++
		if !p.consumeDigits() {
			return nil, &Issue{Code: CodeInvalidJSON, Path: path, Message: "invalid JSON number fraction"}
		}
	}
	if char := p.peek(); char == 'e' || char == 'E' {
		p.pos++
		if char := p.peek(); char == '+' || char == '-' {
			p.pos++
		}
		if !p.consumeDigits() {
			return nil, &Issue{Code: CodeInvalidJSON, Path: path, Message: "invalid JSON number exponent"}
		}
	}
	return &jsonValue{kind: jsonNumber, path: path, number: string(p.data[start:p.pos])}, nil
}

func (p *jsonParser) consumeDigits() bool {
	start := p.pos
	for p.pos < len(p.data) && p.data[p.pos] >= '0' && p.data[p.pos] <= '9' {
		p.pos++
	}
	return p.pos > start
}

// stringValue decodes one JSON string. Valid UTF-16 pairs become one code
// point; an unpaired surrogate becomes U+FFFD, matching the repository's
// established Go-compatible treatment and the TypeScript mirror.
func (p *jsonParser) stringValue(path string) (string, *Issue) {
	p.pos++ // opening quote
	buffer := make([]byte, 0, 32)
	for p.pos < len(p.data) {
		char := p.data[p.pos]
		switch {
		case char == '"':
			p.pos++
			return string(buffer), nil
		case char == '\\':
			p.pos++
			if p.pos >= len(p.data) {
				return "", &Issue{Code: CodeInvalidJSON, Path: path, Message: "unterminated JSON escape"}
			}
			switch escaped := p.data[p.pos]; escaped {
			case '"', '\\', '/':
				buffer = append(buffer, escaped)
				p.pos++
			case 'b':
				buffer = append(buffer, '\b')
				p.pos++
			case 'f':
				buffer = append(buffer, '\f')
				p.pos++
			case 'n':
				buffer = append(buffer, '\n')
				p.pos++
			case 'r':
				buffer = append(buffer, '\r')
				p.pos++
			case 't':
				buffer = append(buffer, '\t')
				p.pos++
			case 'u':
				p.pos++
				unit, ok := p.readHex4()
				if !ok {
					return "", &Issue{Code: CodeInvalidJSON, Path: path, Message: "invalid \\u escape"}
				}
				buffer = p.appendCodeUnit(buffer, unit)
			default:
				return "", &Issue{Code: CodeInvalidJSON, Path: path, Message: "invalid JSON escape"}
			}
		case char < 0x20:
			return "", &Issue{Code: CodeInvalidJSON, Path: path, Message: "unescaped control character in JSON string"}
		default:
			buffer = append(buffer, char)
			p.pos++
		}
	}
	return "", &Issue{Code: CodeInvalidJSON, Path: path, Message: "unterminated JSON string"}
}

func (p *jsonParser) appendCodeUnit(buffer []byte, unit uint16) []byte {
	if unit >= 0xD800 && unit <= 0xDBFF {
		// A high surrogate consumes an immediately following low surrogate.
		if p.pos+6 <= len(p.data) && p.data[p.pos] == '\\' && p.data[p.pos+1] == 'u' {
			if low, ok := peekHex4(p.data[p.pos+2 : p.pos+6]); ok && low >= 0xDC00 && low <= 0xDFFF {
				p.pos += 6
				return utf8.AppendRune(buffer, utf16.DecodeRune(rune(unit), rune(low)))
			}
		}
		return utf8.AppendRune(buffer, utf8.RuneError)
	}
	if unit >= 0xDC00 && unit <= 0xDFFF {
		return utf8.AppendRune(buffer, utf8.RuneError)
	}
	return utf8.AppendRune(buffer, rune(unit))
}

func (p *jsonParser) readHex4() (uint16, bool) {
	if p.pos+4 > len(p.data) {
		return 0, false
	}
	value, ok := peekHex4(p.data[p.pos : p.pos+4])
	if !ok {
		return 0, false
	}
	p.pos += 4
	return value, true
}

func peekHex4(data []byte) (uint16, bool) {
	if len(data) < 4 {
		return 0, false
	}
	var value uint16
	for _, char := range data[:4] {
		value <<= 4
		switch {
		case char >= '0' && char <= '9':
			value |= uint16(char - '0')
		case char >= 'a' && char <= 'f':
			value |= uint16(char-'a') + 10
		case char >= 'A' && char <= 'F':
			value |= uint16(char-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

func (p *jsonParser) skipSpace() {
	for p.pos < len(p.data) {
		switch p.data[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *jsonParser) peek() byte {
	if p.pos >= len(p.data) {
		return 0
	}
	return p.data[p.pos]
}

func pointerChild(path, token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	token = strings.ReplaceAll(token, "/", "~1")
	return path + "/" + token
}

func pointerIndex(path string, index int) string {
	return path + "/" + strconv.Itoa(index)
}
