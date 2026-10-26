package walkthrough

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

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

type jsonFrame struct {
	kind       jsonKind
	items      []*jsonValue
	members    []jsonMember
	keys       map[string]bool
	pendingKey string
}

// parseJSON parses exactly one JSON value with the strictness the declaration
// contract needs: valid UTF-8, no duplicate keys (after escape and surrogate
// normalization), no trailing data, no unescaped controls and no lenient
// number spellings.
//
// The parser is iterative and keeps its own container stack. A declaration may
// legitimately carry a deeply nested opaque body inside an unknown target
// kind, so nesting is bounded only by the 64 KiB input budget, never by an
// invented content depth limit. Paths are built as path segments and joined
// only when an issue is reported, so a deeply nested declaration does not
// allocate a full path string per level.
func parseJSON(data []byte) (*jsonValue, *Issue) {
	parser := &jsonParser{data: data}
	stack := make([]*jsonFrame, 0, 8)
	segments := make([]string, 0, 8)
	var root *jsonValue
	expectKey := false

outer:
	for {
		parser.skipSpace()
		if expectKey {
			frame := stack[len(stack)-1]
			if parser.peek() != '"' {
				return nil, &Issue{Code: CodeInvalidJSON, Path: pointerPath(segments), Message: "expected an object key"}
			}
			key, issue := parser.stringValue()
			if issue != nil {
				issue.Path = pointerPath(segments)
				return nil, issue
			}
			if frame.keys[key] {
				return nil, &Issue{
					Code:    CodeDuplicateKey,
					Path:    pointerChild(pointerPath(segments), key),
					Message: "duplicate JSON object key",
				}
			}
			frame.keys[key] = true
			parser.skipSpace()
			if parser.peek() != ':' {
				return nil, &Issue{Code: CodeInvalidJSON, Path: pointerChild(pointerPath(segments), key), Message: "expected ':' after an object key"}
			}
			parser.pos++
			parser.skipSpace()
			frame.pendingKey = key
			segments = append(segments, key)
			expectKey = false
		}

		if parser.pos >= len(parser.data) {
			return nil, &Issue{Code: CodeInvalidJSON, Path: pointerPath(segments), Message: "unexpected end of JSON"}
		}

		switch parser.peek() {
		case '{':
			frame := &jsonFrame{kind: jsonObject, keys: make(map[string]bool)}
			parser.pos++
			parser.skipSpace()
			if parser.peek() == '}' {
				parser.pos++
				attachJSON(&stack, &root, &jsonValue{kind: jsonObject})
			} else {
				stack = append(stack, frame)
				expectKey = true
				continue
			}
		case '[':
			frame := &jsonFrame{kind: jsonArray}
			parser.pos++
			parser.skipSpace()
			if parser.peek() == ']' {
				parser.pos++
				attachJSON(&stack, &root, &jsonValue{kind: jsonArray})
			} else {
				stack = append(stack, frame)
				segments = append(segments, "0")
				expectKey = false
				continue
			}
		case '"':
			text, issue := parser.stringValue()
			if issue != nil {
				issue.Path = pointerPath(segments)
				return nil, issue
			}
			attachJSON(&stack, &root, &jsonValue{kind: jsonString, text: text})
		case 't':
			literalValue, issue := parser.literal("true", jsonBoolean, true)
			if issue != nil {
				issue.Path = pointerPath(segments)
				return nil, issue
			}
			attachJSON(&stack, &root, literalValue)
		case 'f':
			literalValue, issue := parser.literal("false", jsonBoolean, false)
			if issue != nil {
				issue.Path = pointerPath(segments)
				return nil, issue
			}
			attachJSON(&stack, &root, literalValue)
		case 'n':
			literalValue, issue := parser.literal("null", jsonNull, false)
			if issue != nil {
				issue.Path = pointerPath(segments)
				return nil, issue
			}
			attachJSON(&stack, &root, literalValue)
		default:
			if parser.peek() == '-' || (parser.peek() >= '0' && parser.peek() <= '9') {
				numberValue, issue := parser.number()
				if issue != nil {
					issue.Path = pointerPath(segments)
					return nil, issue
				}
				attachJSON(&stack, &root, numberValue)
			} else {
				return nil, &Issue{Code: CodeInvalidJSON, Path: pointerPath(segments), Message: "unexpected character in JSON"}
			}
		}

		for {
			if len(stack) == 0 {
				parser.skipSpace()
				if parser.pos != len(parser.data) {
					return nil, &Issue{Code: CodeTrailingData, Path: "", Message: "trailing data after the JSON value"}
				}
				return root, nil
			}
			frame := stack[len(stack)-1]
			parser.skipSpace()
			if frame.kind == jsonArray {
				switch parser.peek() {
				case ',':
					parser.pos++
					if len(segments) > 0 {
						segments = segments[:len(segments)-1]
					}
					segments = append(segments, strconv.Itoa(len(frame.items)))
					expectKey = false
					continue outer
				case ']':
					parser.pos++
					stack = stack[:len(stack)-1]
					if len(segments) > 0 {
						segments = segments[:len(segments)-1]
					}
					attachJSON(&stack, &root, &jsonValue{kind: jsonArray, array: frame.items})
					continue
				default:
					return nil, &Issue{Code: CodeInvalidJSON, Path: pointerPath(segments), Message: "expected ',' or ']' in an array"}
				}
			}
			switch parser.peek() {
			case ',':
				parser.pos++
				if len(segments) > 0 {
					segments = segments[:len(segments)-1]
				}
				expectKey = true
				continue outer
			case '}':
				parser.pos++
				stack = stack[:len(stack)-1]
				if len(segments) > 0 {
					segments = segments[:len(segments)-1]
				}
				attachJSON(&stack, &root, &jsonValue{kind: jsonObject, object: frame.members})
				continue
			default:
				return nil, &Issue{Code: CodeInvalidJSON, Path: pointerPath(segments), Message: "expected ',' or '}' in an object"}
			}
		}
	}
}

func attachJSON(stack *[]*jsonFrame, root **jsonValue, value *jsonValue) {
	if len(*stack) == 0 {
		*root = value
		return
	}
	frame := (*stack)[len(*stack)-1]
	if frame.kind == jsonArray {
		frame.items = append(frame.items, value)
		return
	}
	frame.members = append(frame.members, jsonMember{key: frame.pendingKey, value: value})
}

func (p *jsonParser) literal(spelling string, kind jsonKind, boolean bool) (*jsonValue, *Issue) {
	if !bytes.HasPrefix(p.data[p.pos:], []byte(spelling)) {
		return nil, &Issue{Code: CodeInvalidJSON, Message: "invalid JSON literal"}
	}
	p.pos += len(spelling)
	return &jsonValue{kind: kind, bool: boolean}, nil
}

func (p *jsonParser) number() (*jsonValue, *Issue) {
	start := p.pos
	if p.peek() == '-' {
		p.pos++
	}
	if p.pos >= len(p.data) {
		return nil, &Issue{Code: CodeInvalidJSON, Message: "invalid JSON number"}
	}
	switch char := p.data[p.pos]; {
	case char == '0':
		p.pos++
	case char >= '1' && char <= '9':
		p.consumeDigits()
	default:
		return nil, &Issue{Code: CodeInvalidJSON, Message: "invalid JSON number"}
	}
	if p.peek() == '.' {
		p.pos++
		if !p.consumeDigits() {
			return nil, &Issue{Code: CodeInvalidJSON, Message: "invalid JSON number fraction"}
		}
	}
	if char := p.peek(); char == 'e' || char == 'E' {
		p.pos++
		if char := p.peek(); char == '+' || char == '-' {
			p.pos++
		}
		if !p.consumeDigits() {
			return nil, &Issue{Code: CodeInvalidJSON, Message: "invalid JSON number exponent"}
		}
	}
	return &jsonValue{kind: jsonNumber, number: string(p.data[start:p.pos])}, nil
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
func (p *jsonParser) stringValue() (string, *Issue) {
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
				return "", &Issue{Code: CodeInvalidJSON, Message: "unterminated JSON escape"}
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
					return "", &Issue{Code: CodeInvalidJSON, Message: "invalid \\u escape"}
				}
				buffer = p.appendCodeUnit(buffer, unit)
			default:
				return "", &Issue{Code: CodeInvalidJSON, Message: "invalid JSON escape"}
			}
		case char < 0x20:
			return "", &Issue{Code: CodeInvalidJSON, Message: "unescaped control character in JSON string"}
		default:
			buffer = append(buffer, char)
			p.pos++
		}
	}
	return "", &Issue{Code: CodeInvalidJSON, Message: "unterminated JSON string"}
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

// pointerPath joins already-normalized path segments only when an issue is
// reported. The parser never stores one path string per nesting level.
func pointerPath(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, segment := range segments {
		builder.WriteString(pointerChild("", segment))
	}
	return builder.String()
}
