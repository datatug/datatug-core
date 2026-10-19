package datatug

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// environmentJSON avoids recursive JSON method dispatch while retaining the
// ordinary encoding/json behavior of every typed field, including embedded ones.
type environmentJSON Environment

// Keep this list aligned with Environment and its embedded JSON fields. The
// schema-alignment test makes adding a typed field without updating it fail.
var environmentJSONFields = [...]string{
	"id", "title", "folder", "tags", "userIds", "access", "dbServers", "editionConnections",
}

func environmentJSONField(key string) string {
	for _, field := range environmentJSONFields {
		// Match encoding/json's Unicode-aware case folding for typed fields.
		if strings.EqualFold(key, field) {
			return field
		}
	}
	return ""
}

// UnmarshalJSON retains unknown JSON metadata without assigning it meaning.
// Number tokens remain json.Number, so opaque integers never pass through float64.
// Distinct case aliases are refused rather than producing contradictory fields
// on the next save; identical duplicate keys keep encoding/json's last-value rule.
func (v *Environment) UnmarshalJSON(data []byte) error {
	var fields map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&fields); err != nil {
		return err
	}
	if fields == nil {
		return nil // root null has always been a no-op for an existing struct
	}
	seen := make(map[string]bool, len(fields))
	var metadata map[string]any
	for key, value := range fields {
		field := environmentJSONField(key)
		folded := strings.ToLower(key)
		if field != "" {
			folded = strings.ToLower(field)
		}
		if seen[folded] {
			return fmt.Errorf("environment JSON contains case-aliased field %q", key)
		}
		seen[folded] = true
		if field == "" {
			if metadata == nil {
				metadata = make(map[string]any)
			}
			metadata[key] = value
		}
	}
	// Refuse aliases before typed decoding can mutate existing pointers or slices.
	// Preserve encoding/json's partial-update semantics for ordinary typed errors.
	decoded := environmentJSON(*v)
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	// Each object replaces retained metadata, including when the new object has none.
	decoded.jsonMetadata = metadata
	*v = Environment(decoded)
	return nil
}

// MarshalJSON merges opaque metadata with the current typed JSON. Known fields
// were excluded from metadata at decode time, so clearing an omitempty field
// cannot resurrect its old value or an alternative spelling.
func (v Environment) MarshalJSON() ([]byte, error) {
	typed, err := json.Marshal(environmentJSON(v))
	if err != nil || len(v.jsonMetadata) == 0 {
		return typed, err
	}
	var known map[string]json.RawMessage
	// typed was just produced by encoding/json from a struct: it is a valid object.
	_ = json.Unmarshal(typed, &known)
	fields := make(map[string]any, len(known)+len(v.jsonMetadata))
	for key, value := range v.jsonMetadata {
		fields[key] = value
	}
	for key, value := range known {
		fields[key] = value
	}
	return json.Marshal(fields)
}
