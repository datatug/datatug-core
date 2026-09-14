package apicontract

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"
)

// CompareTypedKeys implements the portable natural order for typed composite
// keys. Components must have matching types. Numbers, integers and decimals
// sort numerically; booleans false before true; dates and datetimes
// chronologically; and strings by UTF-8 bytes (binary collation).
//
// Decimal and datetime representations that denote the same value retain a
// deterministic text tie-break, because TypedValue equality preserves their
// exact canonical transport spelling.
func CompareTypedKeys(left, right []TypedValue) (int, error) {
	if len(left) != len(right) {
		return 0, fmt.Errorf("apicontract: key lengths differ: %d and %d", len(left), len(right))
	}
	for i := range left {
		if left[i].Type != right[i].Type {
			return 0, fmt.Errorf("apicontract: key component %d types differ: %q and %q", i, left[i].Type, right[i].Type)
		}
		leftPart, err := sortableTypedValue(left[i])
		if err != nil {
			return 0, fmt.Errorf("apicontract: left key component %d: %w", i, err)
		}
		rightPart, err := sortableTypedValue(right[i])
		if err != nil {
			return 0, fmt.Errorf("apicontract: right key component %d: %w", i, err)
		}
		if order := strings.Compare(leftPart, rightPart); order != 0 {
			return order, nil
		}
	}
	return 0, nil
}

// TypedKeySortKey returns a stable string whose binary lexical order is the
// same as CompareTypedKeys. It is suitable for ordered-stream record IDs and
// opaque local-cache pagination keys. Callers must use binary rather than
// locale collation when ordering it.
func TypedKeySortKey(values []TypedValue) (string, error) {
	if len(values) == 0 {
		return "", fmt.Errorf("apicontract: key is empty")
	}
	var b strings.Builder
	for i, value := range values {
		part, err := sortableTypedValue(value)
		if err != nil {
			return "", fmt.Errorf("apicontract: key component %d: %w", i, err)
		}
		// Hex retains byte ordering. '/' sorts before every hex digit, so it
		// terminates a component prefix without disturbing component order.
		b.WriteString(hex.EncodeToString([]byte(part)))
		b.WriteByte('/')
	}
	return b.String(), nil
}

// TypedValueSortKey returns the binary-sortable encoding for a value that is
// not necessarily a key. Null sorts before every non-null value. This is used
// by typed distributions, whose column may contain null cells.
func TypedValueSortKey(value TypedValue) (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	if value.Type == ValueTypeNull {
		return "0/", nil
	}
	part, err := sortableTypedValue(value)
	if err != nil {
		return "", err
	}
	return "1" + hex.EncodeToString([]byte(part)) + "/", nil
}

func sortableTypedValue(value TypedValue) (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	switch value.Type {
	case ValueTypeString:
		return "s" + value.Str, nil
	case ValueTypeNumber:
		return "n" + sortableFloat(value.Num), nil
	case ValueTypeInteger:
		return "i" + sortableInteger(value.Str), nil
	case ValueTypeDecimal:
		return "d" + sortableDecimal(value.Str), nil
	case ValueTypeBoolean:
		if value.Bool {
			return "b1", nil
		}
		return "b0", nil
	case ValueTypeDate:
		return "a" + value.Str, nil
	case ValueTypeDatetime:
		parsed, err := time.Parse(time.RFC3339, value.Str)
		if err != nil {
			return "", err
		}
		return "t" + parsed.UTC().Format("20060102T150405.000000000") + value.Str, nil
	case ValueTypeNull:
		return "", fmt.Errorf("null key component is not allowed")
	default:
		return "", fmt.Errorf("unsupported key type %q", value.Type)
	}
}

func sortableFloat(value float64) string {
	if value == 0 {
		value = 0 // normalize negative zero to TypedValue equality semantics
	}
	bits := math.Float64bits(value)
	if bits&(uint64(1)<<63) != 0 {
		bits = ^bits
	} else {
		bits ^= uint64(1) << 63
	}
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], bits)
	return string(encoded[:])
}

func sortableInteger(value string) string {
	negative := strings.HasPrefix(value, "-")
	digits := strings.TrimPrefix(value, "-")
	if digits == "0" {
		return "1"
	}
	body := sortableLength(len(digits)) + digits
	if negative {
		return "0" + complement(body)
	}
	return "2" + body
}

func sortableDecimal(value string) string {
	negative := strings.HasPrefix(value, "-")
	abs := strings.TrimPrefix(value, "-")
	integer, fraction, _ := strings.Cut(abs, ".")
	digits := integer + fraction
	first := strings.IndexFunc(digits, func(r rune) bool { return r != '0' })
	if first < 0 {
		// Preserve exact spellings such as 0 and 0.0 as distinct keys.
		return "1" + value
	}
	exponent := len(integer) - first - 1
	significand := strings.TrimRight(digits[first:], "0")
	body := sortableSignedInt(int64(exponent)) + significand + "/"
	if negative {
		return "0" + complement(body) + value
	}
	return "2" + body + value
}

func sortableLength(length int) string {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], uint64(length))
	return string(encoded[:])
}

func sortableSignedInt(value int64) string {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], uint64(value)^(uint64(1)<<63))
	return string(encoded[:])
}

func complement(value string) string {
	result := []byte(value)
	for i := range result {
		result[i] = ^result[i]
	}
	return string(result)
}
