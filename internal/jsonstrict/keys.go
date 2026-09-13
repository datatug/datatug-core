// Package jsonstrict provides target-aware JSON key validation shared by
// transport and persisted event decoders.
package jsonstrict

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

// CheckNoDuplicateKeysFor rejects repeated object keys. When an object is
// decoded into a struct, keys are compared with encoding/json's case-folding
// rules so two spellings cannot silently target the same field.
func CheckNoDuplicateKeysFor(data []byte, target reflect.Type) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	return walkNoDuplicateKeys(decoder, target)
}

func walkNoDuplicateKeys(decoder *json.Decoder, target reflect.Type) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	target = derefType(target)
	switch delim {
	case '{':
		fields, fold := ObjectFields(target)
		seen := make(map[string]string)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, _ := keyToken.(string)
			name := key
			if fold {
				name = FoldName(key)
			}
			if first, duplicate := seen[name]; duplicate {
				if first == key {
					return fmt.Errorf("duplicate key %q", key)
				}
				return fmt.Errorf("duplicate key %q: it names the same field as %q", key, first)
			}
			seen[name] = key
			var child reflect.Type
			switch {
			case fold:
				child = fields[name]
			case target != nil && target.Kind() == reflect.Map:
				child = target.Elem()
			}
			if err := walkNoDuplicateKeys(decoder, child); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
	case '[':
		var element reflect.Type
		if target != nil && (target.Kind() == reflect.Slice || target.Kind() == reflect.Array) {
			element = target.Elem()
		}
		for decoder.More() {
			if err := walkNoDuplicateKeys(decoder, element); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
	}
	return err
}

var jsonUnmarshalerType = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()

func derefType(target reflect.Type) reflect.Type {
	for target != nil && target.Kind() == reflect.Pointer {
		target = target.Elem()
	}
	return target
}

// ObjectFields returns the fields encoding/json decodes an object into,
// keyed by their folded JSON name.
func ObjectFields(target reflect.Type) (map[string]reflect.Type, bool) {
	if target == nil || target.Kind() != reflect.Struct || target.Implements(jsonUnmarshalerType) || reflect.PointerTo(target).Implements(jsonUnmarshalerType) {
		return nil, false
	}
	fields := make(map[string]reflect.Type)
	addJSONFields(fields, target)
	return fields, true
}

func addJSONFields(fields map[string]reflect.Type, target reflect.Type) {
	for index := 0; index < target.NumField(); index++ {
		field := target.Field(index)
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if field.Anonymous && name == "" {
			if embedded := derefType(field.Type); embedded.Kind() == reflect.Struct {
				addJSONFields(fields, embedded)
				continue
			}
		}
		if !field.IsExported() {
			continue
		}
		if name == "" {
			name = field.Name
		}
		if folded := FoldName(name); fields[folded] == nil {
			fields[folded] = field.Type
		}
	}
}

// FoldName folds a JSON name the same way encoding/json matches struct fields.
func FoldName(name string) string {
	var folded strings.Builder
	for _, r := range name {
		if r < utf8.RuneSelf {
			if 'a' <= r && r <= 'z' {
				r -= 'a' - 'A'
			}
			folded.WriteRune(r)
			continue
		}
		folded.WriteRune(FoldRune(r))
	}
	return folded.String()
}

// FoldRune returns the smallest rune in r's simple case-folding orbit.
func FoldRune(r rune) rune {
	for {
		next := unicode.SimpleFold(r)
		if next <= r {
			return next
		}
		r = next
	}
}
