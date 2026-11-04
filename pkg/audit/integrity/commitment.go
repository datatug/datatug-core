package integrity

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"slices"
)

const (
	labelFieldSalt = "datatug/audit/v1/field-salt"
	labelField     = "datatug/audit/v1/field"
	labelLeaf      = "datatug/audit/v1/leaf"
)

// Salt is an event's random secret. It is stored with the event and never
// exported; exports carry per-field salts derived from it.
type Salt [32]byte

// NewSalt draws a salt from r, or from crypto/rand when r is nil.
func NewSalt(r io.Reader) (Salt, error) {
	if r == nil {
		r = rand.Reader
	}
	var s Salt
	if _, err := io.ReadFull(r, s[:]); err != nil {
		return Salt{}, fmt.Errorf("audit integrity: read salt: %w", err)
	}
	return s, nil
}

// fieldSalt derives the salt of one field. Revealing it reveals nothing about
// other fields' salts.
func (s Salt) fieldSalt(name string) Hash {
	m := hmac.New(sha256.New, s[:])
	m.Write([]byte(labelFieldSalt + "\x00" + name))
	return Hash(m.Sum(nil))
}

// commit is the salted commitment to one field value.
func commit(fieldSalt Hash, name string, value []byte) Hash {
	w := builder{}
	w.label(labelField)
	w.str(name)
	w.b = append(w.b, value...)
	m := hmac.New(sha256.New, fieldSalt[:])
	m.Write(w.b)
	return Hash(m.Sum(nil))
}

// leafOf hashes the commitments of one event. The leading 0x00 is the RFC 9162
// leaf prefix; interior nodes use 0x01.
func leafOf(version uint16, commitments []Hash) Hash {
	w := builder{}
	w.byte1(0x00)
	w.label(labelLeaf)
	w.u32(uint32(version))
	w.u32(uint32(len(commitments)))
	for _, c := range commitments {
		w.hash(c)
	}
	return w.sum()
}

// LeafOf returns the Merkle leaf of event e under salt s. The sealer calls it
// for each stored event.
func LeafOf(s Salt, e *Event) Hash {
	cs := make([]Hash, len(schemaV1))
	for i, f := range schemaV1 {
		cs[i] = commit(s.fieldSalt(f.name), f.name, f.get(e))
	}
	return leafOf(EncodingVersion, cs)
}

// DisclosedEvent is an event as exported: every field carries its
// commitment; revealed fields also carry the field salt and canonical value.
type DisclosedEvent struct {
	Version uint16           `json:"v"`
	Fields  []DisclosedField `json:"fields"`
}

// DisclosedField is one committed field. Salt is nil when the field is
// redacted; Value is then empty.
type DisclosedField struct {
	Name       string `json:"name"`
	Commitment Hash   `json:"commitment"`
	Salt       *Hash  `json:"salt,omitempty"`
	Value      []byte `json:"value,omitempty"`
}

// Revealed reports whether the field's value is disclosed.
func (f DisclosedField) Revealed() bool { return f.Salt != nil }

// Disclose returns e with every field revealed.
func Disclose(s Salt, e *Event) DisclosedEvent {
	d := DisclosedEvent{Version: EncodingVersion, Fields: make([]DisclosedField, len(schemaV1))}
	for i, f := range schemaV1 {
		fs := s.fieldSalt(f.name)
		v := f.get(e)
		d.Fields[i] = DisclosedField{Name: f.name, Commitment: commit(fs, f.name, v), Salt: &fs, Value: v}
	}
	return d
}

// Redact returns a copy of d with the named fields replaced by their
// commitments. The leaf is unchanged. An unknown name is an error.
func (d DisclosedEvent) Redact(names ...string) (DisclosedEvent, error) {
	out := DisclosedEvent{Version: d.Version, Fields: slices.Clone(d.Fields)}
	for _, n := range names {
		i := slices.IndexFunc(out.Fields, func(f DisclosedField) bool { return f.Name == n })
		if i < 0 {
			return DisclosedEvent{}, &FieldError{Field: n, Err: ErrUnknownField}
		}
		out.Fields[i] = DisclosedField{Name: n, Commitment: out.Fields[i].Commitment}
	}
	return out, nil
}

// Redacted lists the names of redacted fields, in schema order.
func (d DisclosedEvent) Redacted() []string {
	var names []string
	for _, f := range d.Fields {
		if !f.Revealed() {
			names = append(names, f.Name)
		}
	}
	return names
}

// Leaf checks d (version, field set and order, canonical values, every
// revealed value against its commitment) and returns its Merkle leaf.
// A failure is a [*FieldError] wrapping [ErrUnsupportedVersion],
// [ErrMalformedEvent] or [ErrFieldTampered].
func (d DisclosedEvent) Leaf() (Hash, error) {
	if _, err := d.decode(); err != nil {
		return Hash{}, err
	}
	cs := make([]Hash, len(d.Fields))
	for i, f := range d.Fields {
		cs[i] = f.Commitment
	}
	return leafOf(d.Version, cs), nil
}

// Event decodes the revealed fields of d after the same checks as
// [DisclosedEvent.Leaf]. Redacted fields are left at their zero value; use
// [DisclosedEvent.Redacted] to tell them from absent ones.
func (d DisclosedEvent) Event() (Event, error) { return d.decode() }

func (d DisclosedEvent) decode() (Event, error) {
	var e Event
	if d.Version != EncodingVersion {
		return Event{}, &FieldError{Err: fmt.Errorf("%w: %d", ErrUnsupportedVersion, d.Version)}
	}
	if len(d.Fields) != len(schemaV1) {
		return Event{}, &FieldError{Err: fmt.Errorf("%w: %d fields, want %d", ErrMalformedEvent, len(d.Fields), len(schemaV1))}
	}
	for i, spec := range schemaV1 {
		f := d.Fields[i]
		if f.Name != spec.name {
			return Event{}, &FieldError{Field: f.Name, Err: fmt.Errorf("%w: position %d must be %q", ErrMalformedEvent, i, spec.name)}
		}
		if !f.Revealed() {
			if len(f.Value) != 0 {
				return Event{}, &FieldError{Field: f.Name, Err: fmt.Errorf("%w: redacted field carries a value", ErrMalformedEvent)}
			}
			continue
		}
		if err := spec.checkCanonical(&e, f.Value); err != nil {
			return Event{}, &FieldError{Field: f.Name, Err: fmt.Errorf("%w: %v", ErrMalformedEvent, err)}
		}
		if commit(*f.Salt, f.Name, f.Value) != f.Commitment {
			return Event{}, &FieldError{Field: f.Name, Err: ErrFieldTampered}
		}
	}
	return e, nil
}
