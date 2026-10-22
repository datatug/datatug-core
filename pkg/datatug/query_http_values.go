package datatug

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Value coercion for HTTP query parameters. The web app mirrors every rule
// here (datatug-apps http-template.ts); the shared test vectors pin them.

// maxSafeHTTPInteger is the largest integer both Go and JavaScript represent
// exactly (2^53 - 1).
const maxSafeHTTPInteger = 9007199254740991

var (
	jsonNumberFullRe = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)
	httpIntegerRe    = regexp.MustCompile(`^-?[0-9]+$`)
	// httpTimeRe is RFC 3339: a calendar date, optionally with a time of day
	// that must then carry Z or a numeric offset.
	httpTimeRe = regexp.MustCompile(`^([0-9]{4})-([0-9]{2})-([0-9]{2})(?:[Tt ]([0-9]{2}):([0-9]{2}):([0-9]{2})(\.[0-9]{1,9})?([Zz]|[+-][0-9]{2}:[0-9]{2}))?$`)
)

// httpNumber reads a finite number from a decoded JSON or Go numeric value.
func httpNumber(v any) (float64, bool) {
	var f float64
	switch x := v.(type) {
	case float64:
		f = x
	case int:
		f = float64(x)
	case int64:
		f = float64(x)
	case json.Number:
		parsed, err := strconv.ParseFloat(string(x), 64)
		if err != nil {
			return 0, false
		}
		f = parsed
	default:
		return 0, false
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

// formatHTTPNumber is the shortest decimal that round-trips, without an
// exponent; negative zero is 0.
func formatHTTPNumber(f float64) string {
	if f == 0 {
		return "0"
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func parseHTTPInteger(s string) (string, bool) {
	if !httpIntegerRe.MatchString(s) {
		return "", false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n > maxSafeHTTPInteger || n < -maxSafeHTTPInteger {
		return "", false
	}
	return strconv.FormatInt(n, 10), true
}

// httpBool accepts a boolean, the numbers 0 and 1, and the strings true,
// false, 1 and 0.
func httpBool(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		switch x {
		case "true", "1":
			return true, true
		case "false", "0":
			return false, true
		}
		return false, false
	}
	f, ok := httpNumber(v)
	switch {
	case ok && f == 1:
		return true, true
	case ok && f == 0:
		return false, true
	}
	return false, false
}

// renderHTTPTime renders s as a date (YYYY-MM-DD) or an RFC 3339 UTC
// datetime. A datetime is converted to UTC first, so a date parameter takes
// its UTC calendar date. A local time without an offset is refused: it
// names no instant.
func renderHTTPTime(paramType, s string) (string, bool) {
	m := httpTimeRe.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	num := func(i int) int {
		n, _ := strconv.Atoi(m[i])
		return n
	}
	year, month, day := num(1), num(2), num(3)
	if month < 1 || month > 12 || day < 1 || day > daysInMonth(year, month) {
		return "", false
	}
	hour, minute, second, fraction := 0, 0, 0, ""
	offsetMinutes := 0
	if m[4] != "" {
		hour, minute, second = num(4), num(5), num(6)
		if hour > 23 || minute > 59 || second > 59 {
			return "", false
		}
		fraction = strings.TrimRight(strings.TrimPrefix(m[7], "."), "0")
		if zone := m[8]; zone != "Z" && zone != "z" {
			oh, _ := strconv.Atoi(zone[1:3])
			om, _ := strconv.Atoi(zone[4:6])
			if oh > 23 || om > 59 {
				return "", false
			}
			offsetMinutes = oh*60 + om
			if zone[0] == '-' {
				offsetMinutes = -offsetMinutes
			}
		}
	}
	t := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC).Add(-time.Duration(offsetMinutes) * time.Minute)
	if t.Year() < 0 || t.Year() > 9999 {
		return "", false
	}
	if paramType == "date" {
		return t.Format("2006-01-02"), true
	}
	out := t.Format("2006-01-02T15:04:05")
	if fraction != "" {
		out += "." + fraction
	}
	return out + "Z", true
}

func daysInMonth(year, month int) int {
	switch month {
	case 2:
		if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}
