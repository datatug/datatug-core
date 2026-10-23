package meaninggraph

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/datatug/datatug-core/internal/jsonstrict"
)

func splitPath(path string) []string { return strings.Split(path, "/") }
func parseJSON(body []byte) (map[string]any, error) {
	if err := jsonstrict.ValidateUnicode(body); err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	type frame struct {
		object, key bool
		seen        map[string]bool
	}
	var stack []frame
	for {
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if v, ok := t.(json.Delim); ok && (v == '}' || v == ']') {
			stack = stack[:len(stack)-1]
			continue
		}
		if len(stack) > 0 && stack[len(stack)-1].object {
			f := &stack[len(stack)-1]
			if f.key {
				// Decoder.Token guarantees strings in object-key position.
				key := t.(string)
				if f.seen[key] {
					return nil, errors.New("duplicate retained JSON object key")
				}
				f.seen[key] = true
				f.key = false
				continue
			}
			f.key = true
		}
		if v, ok := t.(json.Delim); ok && (v == '{' || v == '[') {
			if len(stack) >= 128 {
				return nil, errors.New("retained JSON depth bound exceeded")
			}
			stack = append(stack, frame{object: v == '{', key: true, seen: map[string]bool{}})
		}
	}
	d = json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	var value map[string]any
	if err := d.Decode(&value); err != nil {
		return nil, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, errors.New("one retained JSON object required")
	}
	if value == nil {
		return nil, errors.New("retained JSON object required")
	}
	return value, nil
}
func obj(value any) (map[string]any, error) {
	v, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("retained object required")
	}
	return v, nil
}
func list(value any) ([]any, error) {
	v, ok := value.([]any)
	if !ok {
		return nil, errors.New("retained array required")
	}
	return v, nil
}
func str(value any) (string, error) {
	v, ok := value.(string)
	if !ok || v == "" {
		return "", errors.New("retained nonempty string required")
	}
	return v, nil
}
func text(m map[string]any, key string) (string, error) { return str(m[key]) }
func utf16Less(a, b string) bool {
	x, y := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(x) && i < len(y); i++ {
		if x[i] != y[i] {
			return x[i] < y[i]
		}
	}
	return len(x) < len(y)
}
func quoteJSON(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 32 {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}
func canonical(value any) ([]byte, error) {
	var b bytes.Buffer
	var write func(any) error
	write = func(value any) error {
		switch v := value.(type) {
		case nil:
			b.WriteString("null")
		case bool:
			if v {
				b.WriteString("true")
			} else {
				b.WriteString("false")
			}
		case string:
			quoteJSON(&b, v)
		case json.Number:
			b.WriteString(string(v))
		case []any:
			b.WriteByte('[')
			for i, x := range v {
				if i > 0 {
					b.WriteByte(',')
				}
				if err := write(x); err != nil {
					return err
				}
			}
			b.WriteByte(']')
		case map[string]any:
			keys := make([]string, 0, len(v))
			for k := range v {
				keys = append(keys, k)
			}
			sort.Slice(keys, func(i, j int) bool { return utf16Less(keys[i], keys[j]) })
			b.WriteByte('{')
			for i, k := range keys {
				if i > 0 {
					b.WriteByte(',')
				}
				quoteJSON(&b, k)
				b.WriteByte(':')
				if err := write(v[k]); err != nil {
					return err
				}
			}
			b.WriteByte('}')
		default:
			return errors.New("unsupported rounded retained JSON value")
		}
		return nil
	}
	err := write(value)
	return b.Bytes(), err
}
func decimal(s string) (string, error) {
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign = "-"
		s = s[1:]
	}
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == 'e' || r == 'E' })
	mantissa := parts[0]
	exponent := new(big.Int)
	if len(parts) > 1 {
		if len(parts[1]) > 128 {
			return "", errors.New("numeric comparison exponent bound exceeded")
		}
		if _, ok := exponent.SetString(parts[1], 10); !ok {
			return "", errors.New("invalid retained number")
		}
	}
	frac := 0
	if at := strings.IndexByte(mantissa, '.'); at >= 0 {
		frac = len(mantissa) - at - 1
		mantissa = mantissa[:at] + mantissa[at+1:]
	}
	digits := strings.TrimLeft(mantissa, "0")
	if digits == "" {
		return "0", nil
	}
	trimmed := strings.TrimRight(digits, "0")
	exponent.Add(exponent, big.NewInt(int64(len(digits)-len(trimmed)-frac)))
	return sign + trimmed + "e" + exponent.String(), nil
}
func equal(a, b any) (bool, error) {
	if x, ok := a.(json.Number); ok {
		y, ok := b.(json.Number)
		if !ok {
			return false, nil
		}
		nx, e := decimal(string(x))
		if e != nil {
			return false, e
		}
		ny, e := decimal(string(y))
		return nx == ny, e
	}
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false, nil
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok {
				return false, nil
			}
			same, e := equal(v, w)
			if e != nil || !same {
				return same, e
			}
		}
		return true, nil
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false, nil
		}
		for i, v := range x {
			same, e := equal(v, y[i])
			if e != nil || !same {
				return same, e
			}
		}
		return true, nil
	default:
		return fmt.Sprintf("%T", a) == fmt.Sprintf("%T", b) && a == b, nil
	}
}
