package apicontract

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// DecodeStrict decodes data into v with no leniency: unknown JSON fields are
// rejected (encoding/json's DisallowUnknownFields), duplicate keys at any
// nesting level are rejected (encoding/json silently keeps only the last
// occurrence otherwise - the appendix requires duplicate JSON keys to be
// "rejected, never reconciled by precedence"), trailing data after the JSON
// value is rejected, and any field name listed in forbidden is rejected even
// when the target type has no field for it - the same "client-supplied
// principal or role are rejected" rule the appendix states, made explicit at
// every call site that carries security-relevant identity.
func DecodeStrict(data []byte, v any, forbidden ...string) error {
	if err := checkNoDuplicateKeys(data); err != nil {
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
// silently keeps only the last "a".
func checkNoDuplicateKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := walkNoDuplicateKeys(dec); err != nil {
		return err
	}
	return nil
}

func walkNoDuplicateKeys(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("apicontract: %w", err)
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil // scalar value: nothing to walk
	}
	switch delim {
	case '{':
		seen := make(map[string]bool)
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return fmt.Errorf("apicontract: %w", err)
			}
			key, _ := keyTok.(string)
			if seen[key] {
				return fmt.Errorf("apicontract: duplicate key %q", key)
			}
			seen[key] = true
			if err := walkNoDuplicateKeys(dec); err != nil {
				return err
			}
		}
		if _, err := dec.Token(); err != nil { // consume closing '}'
			return fmt.Errorf("apicontract: %w", err)
		}
	case '[':
		for dec.More() {
			if err := walkNoDuplicateKeys(dec); err != nil {
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
