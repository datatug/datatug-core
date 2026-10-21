package dto

import (
	"fmt"
	"strconv"
	"unicode/utf8"
)

// ValidateEnvironmentJSONUnicode refuses malformed UTF-8 and escaped unpaired surrogates.
// encoding/json otherwise silently substitutes the Unicode replacement character.
func ValidateEnvironmentJSONUnicode(body []byte) error {
	if !utf8.Valid(body) {
		return fmt.Errorf("invalid UTF-8")
	}
	inside := false
	for i := 0; i < len(body); i++ {
		if body[i] == '"' {
			inside = !inside
			continue
		}
		if !inside || body[i] != '\\' {
			continue
		}
		i++
		if i >= len(body) {
			return fmt.Errorf("incomplete JSON escape")
		}
		if body[i] != 'u' {
			continue
		}
		if i+4 >= len(body) {
			return fmt.Errorf("incomplete Unicode escape")
		}
		unit, e := strconv.ParseUint(string(body[i+1:i+5]), 16, 16)
		if e != nil {
			return e
		}
		i += 4
		if unit >= 0xd800 && unit <= 0xdbff {
			if i+6 >= len(body) || body[i+1] != '\\' || body[i+2] != 'u' {
				return fmt.Errorf("unpaired Unicode surrogate")
			}
			low, e := strconv.ParseUint(string(body[i+3:i+7]), 16, 16)
			if e != nil || low < 0xdc00 || low > 0xdfff {
				return fmt.Errorf("unpaired Unicode surrogate")
			}
			i += 6
		} else if unit >= 0xdc00 && unit <= 0xdfff {
			return fmt.Errorf("unpaired Unicode surrogate")
		}
	}
	return nil
}
