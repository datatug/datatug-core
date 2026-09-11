package apicontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

// DecodeStrict decodes data into v with no leniency: unknown JSON fields are
// rejected (encoding/json's DisallowUnknownFields), duplicate keys at any
// nesting level are rejected (encoding/json silently keeps only the last
// occurrence otherwise - the appendix requires duplicate JSON keys to be
// "rejected, never reconciled by precedence") - including two spellings of
// one struct field that differ only in case ("project" and "Project"),
// which encoding/json matches to the same field - trailing data after the JSON
// value is rejected, and any field name listed in forbidden is rejected even
// when the target type has no field for it - the same "client-supplied
// principal or role are rejected" rule the appendix states, made explicit at
// every call site that carries security-relevant identity.
func DecodeStrict(data []byte, v any, forbidden ...string) error {
	if err := checkNoDuplicateKeysFor(data, reflect.TypeOf(v)); err != nil {
		return err
	}
	if err := checkNoForbiddenFields(data, forbidden); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("apicontract: decode: %w", err)
	}
	if dec.More() {
		return fmt.Errorf("apicontract: trailing data after JSON value")
	}
	return nil
}

// checkNoDuplicateKeys walks data's full JSON structure, at every nesting
// level, and fails on the first JSON object that repeats a key -
// encoding/json's own decoder does not do this: given {"a":1,"a":2} it
// silently keeps only the last "a". Keys are compared exactly; DecodeStrict
// uses checkNoDuplicateKeysFor, which also knows the target type.
func checkNoDuplicateKeys(data []byte) error {
	return checkNoDuplicateKeysFor(data, nil)
}

// checkNoDuplicateKeysFor is checkNoDuplicateKeys for data that will be
// decoded into a value of type target (nil when unknown). Wherever the
// value decoded is a struct, keys are compared the way encoding/json
// matches them to fields - case-insensitively - so "project" and "Project"
// are duplicates: encoding/json would decode both into one field and keep
// the last. Map keys, and keys of a value whose type is unknown or decodes
// itself (json.Unmarshaler), are compared exactly, since encoding/json keeps
// them apart.
func checkNoDuplicateKeysFor(data []byte, target reflect.Type) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	return walkNoDuplicateKeys(dec, target)
}

func walkNoDuplicateKeys(dec *json.Decoder, target reflect.Type) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("apicontract: %w", err)
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil // scalar value: nothing to walk
	}
	target = derefType(target)
	switch delim {
	case '{':
		fields, fold := jsonObjectFields(target)
		seen := make(map[string]string)
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return fmt.Errorf("apicontract: %w", err)
			}
			key, _ := keyTok.(string)
			name := key
			if fold {
				name = foldJSONName(key)
			}
			if first, dup := seen[name]; dup {
				if first == key {
					return fmt.Errorf("apicontract: duplicate key %q", key)
				}
				return fmt.Errorf("apicontract: duplicate key %q: it names the same field as %q", key, first)
			}
			seen[name] = key
			var child reflect.Type
			switch {
			case fold:
				child = fields[name]
			case target != nil && target.Kind() == reflect.Map:
				child = target.Elem()
			}
			if err := walkNoDuplicateKeys(dec, child); err != nil {
				return err
			}
		}
		if _, err := dec.Token(); err != nil { // consume closing '}'
			return fmt.Errorf("apicontract: %w", err)
		}
	case '[':
		var elem reflect.Type
		if target != nil && (target.Kind() == reflect.Slice || target.Kind() == reflect.Array) {
			elem = target.Elem()
		}
		for dec.More() {
			if err := walkNoDuplicateKeys(dec, elem); err != nil {
				return err
			}
		}
		if _, err := dec.Token(); err != nil { // consume closing ']'
			return fmt.Errorf("apicontract: %w", err)
		}
	}
	return nil
}

// checkNoForbiddenFields fails if any object anywhere in data has a key
// named in forbidden. Used for security-relevant field names (principal,
// role) that never appear in any of this package's legitimate types, so a
// caller can name them explicitly rather than relying only on the target
// struct happening to omit them.
func checkNoForbiddenFields(data []byte, forbidden []string) error {
	if len(forbidden) == 0 {
		return nil
	}
	blocked := make(map[string]bool, len(forbidden))
	for _, f := range forbidden {
		blocked[f] = true
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	return walkNoForbiddenFields(dec, blocked)
}

func walkNoForbiddenFields(dec *json.Decoder, blocked map[string]bool) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("apicontract: %w", err)
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return fmt.Errorf("apicontract: %w", err)
			}
			key, _ := keyTok.(string)
			if blocked[key] {
				return fmt.Errorf("apicontract: field %q is not accepted from a client", key)
			}
			if err := walkNoForbiddenFields(dec, blocked); err != nil {
				return err
			}
		}
		if _, err := dec.Token(); err != nil {
			return fmt.Errorf("apicontract: %w", err)
		}
	case '[':
		for dec.More() {
			if err := walkNoForbiddenFields(dec, blocked); err != nil {
				return err
			}
		}
		if _, err := dec.Token(); err != nil {
			return fmt.Errorf("apicontract: %w", err)
		}
	}
	return nil
}

// jsonUnmarshalerType is json.Unmarshaler's reflect.Type.
var jsonUnmarshalerType = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()

// derefType strips pointers from t.
func derefType(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// jsonObjectFields returns the fields encoding/json decodes a JSON object
// into when t is a struct that does not decode itself, keyed by
// foldJSONName of their JSON names, and whether it is one.
func jsonObjectFields(t reflect.Type) (map[string]reflect.Type, bool) {
	if t == nil || t.Kind() != reflect.Struct || t.Implements(jsonUnmarshalerType) || reflect.PointerTo(t).Implements(jsonUnmarshalerType) {
		return nil, false
	}
	fields := make(map[string]reflect.Type)
	addJSONFields(fields, t)
	return fields, true
}

// addJSONFields adds t's JSON fields to fields, flattening embedded
// structs the way encoding/json does.
func addJSONFields(fields map[string]reflect.Type, t reflect.Type) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if f.Anonymous && name == "" {
			if embedded := derefType(f.Type); embedded.Kind() == reflect.Struct {
				addJSONFields(fields, embedded)
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		if folded := foldJSONName(name); fields[folded] == nil {
			fields[folded] = f.Type
		}
	}
}

// foldJSONName folds name the way encoding/json does when it matches a key
// to a struct field: two names fold equal exactly when bytes.EqualFold
// holds for them (ASCII letters upper-cased, every other rune mapped to the
// smallest rune of its simple case-folding orbit, so the Kelvin sign
// folds with "k").
func foldJSONName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r < utf8.RuneSelf {
			if 'a' <= r && r <= 'z' {
				r -= 'a' - 'A'
			}
			b.WriteRune(r)
			continue
		}
		b.WriteRune(foldRune(r))
	}
	return b.String()
}

// foldRune returns the smallest rune of r's simple case-folding orbit.
func foldRune(r rune) rune {
	for {
		next := unicode.SimpleFold(r)
		if next <= r {
			return next
		}
		r = next
	}
}
