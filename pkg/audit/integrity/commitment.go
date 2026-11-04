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

// LeafOf returns the Merkle leaf of event e under salt s, with no erased
// field. Use [StoredEvent.Leaf] once a field has been erased in the store.
func LeafOf(s Salt, e *Event) Hash { return leafOf(EncodingVersion, commitments(s, e, nil)) }

// commitments returns every field's commitment; erased fields take their
// stored commitment instead of one computed from e.
func commitments(s Salt, e *Event, erased map[string]Hash) []Hash {
	cs := make([]Hash, len(schemaV1))
	for i, f := range schemaV1 {
		if c, ok := erased[f.name]; ok {
			cs[i] = c
			continue
		}
		cs[i] = commit(s.fieldSalt(f.name), f.name, f.get(e))
	}
	return cs
}

// DisclosedEvent is an event as exported: every field carries its
// commitment; revealed fields also carry the field salt and canonical value.
type DisclosedEvent struct {
	Version uint16           `json:"v"`
	Fields  []DisclosedField `json:"fields"`
}

// DisclosedField is one committed field. Salt is nil when the field is not
// revealed; Value is then empty. Erased marks a field whose value the store
// dropped ([StoredEvent.Erase]) as opposed to one hidden from this reader;
// it is the exporter's label and only ever set on an unrevealed field.
type DisclosedField struct {
	Name       string `json:"name"`
	Commitment Hash   `json:"commitment"`
	Salt       *Hash  `json:"salt,omitempty"`
	Value      []byte `json:"value,omitempty"`
	Erased     bool   `json:"erased,omitempty"`
}

// Revealed reports whether the field's value is disclosed.
func (f DisclosedField) Revealed() bool { return f.Salt != nil }

// Disclose returns e with every field revealed (no erased field). Use
// [StoredEvent.Disclose] once a field has been erased in the store.
func Disclose(s Salt, e *Event) DisclosedEvent { return disclose(s, e, nil) }

func disclose(s Salt, e *Event, erased map[string]Hash) DisclosedEvent {
	d := DisclosedEvent{Version: EncodingVersion, Fields: make([]DisclosedField, len(schemaV1))}
	for i, f := range schemaV1 {
		if c, ok := erased[f.name]; ok {
			d.Fields[i] = DisclosedField{Name: f.name, Commitment: c, Erased: true}
			continue
		}
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
		if out.Fields[i].Revealed() {
			out.Fields[i] = DisclosedField{Name: n, Commitment: out.Fields[i].Commitment}
		}
	}
	return out, nil
}

// Redacted lists the fields hidden from this reader (not revealed and not
// erased), in schema order.
func (d DisclosedEvent) Redacted() []string {
	return d.hidden(func(f DisclosedField) bool { return !f.Revealed() && !f.Erased })
}

// Erased lists the fields whose value the store dropped, in schema order.
func (d DisclosedEvent) Erased() []string {
	return d.hidden(func(f DisclosedField) bool { return f.Erased })
}

func (d DisclosedEvent) hidden(match func(DisclosedField) bool) []string {
	var names []string
	for _, f := range d.Fields {
		if match(f) {
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
	_, leaf, err := d.verified()
	return leaf, err
}

// verified decodes and checks d and returns the decoded event and its leaf.
func (d DisclosedEvent) verified() (Event, Hash, error) {
	e, err := d.decode()
	if err != nil {
		return Event{}, Hash{}, err
	}
	cs := make([]Hash, len(d.Fields))
	for i, f := range d.Fields {
		cs[i] = f.Commitment
	}
	return e, leafOf(d.Version, cs), nil
}

// revealed reports whether the named field is disclosed.
func (d DisclosedEvent) revealed(name string) bool {
	i := slices.IndexFunc(d.Fields, func(f DisclosedField) bool { return f.Name == name })
	return i >= 0 && d.Fields[i].Revealed()
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
		if f.Erased {
			return Event{}, &FieldError{Field: f.Name, Err: fmt.Errorf("%w: erased field is revealed", ErrMalformedEvent)}
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
