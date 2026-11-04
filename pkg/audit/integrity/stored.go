package integrity

import (
	"bytes"
	"cmp"
	"slices"
)

// StoredEvent is an event as the audit store keeps it: the event, its salt
// and, for every field erased after insertion, the commitment that replaces
// the dropped value (permission model §19.4: "an erasure can replace the
// stored reason with its commitment without breaking verification").
//
// The store persists Erased next to the event. Leaf and Disclose use the
// stored commitment for an erased field, so the leaf, the window root and
// every later export still verify.
//
// The dropped value cannot be recovered from the store or from any export.
// Residual risk: whoever holds the store's salt can still test guesses
// against the commitment (a dictionary attack on short or predictable text).
// Exports never carry the event salt, so readers of an export cannot.
type StoredEvent struct {
	Salt   Salt
	Event  Event
	Erased map[string]Hash
}

func schemaField(name string) (fieldSpec, bool) {
	i := slices.IndexFunc(schemaV1, func(f fieldSpec) bool { return f.name == name })
	if i < 0 {
		return fieldSpec{}, false
	}
	return schemaV1[i], true
}

// orderKey reports whether a field fixes the leaf order and so can never be erased.
func orderKey(name string) bool { return name == "id" || name == "at" }

func (x *StoredEvent) check() error {
	for name := range x.Erased {
		if _, ok := schemaField(name); !ok {
			return &FieldError{Field: name, Err: ErrUnknownField}
		}
		if orderKey(name) {
			return &FieldError{Field: name, Err: ErrInvalidArgument}
		}
	}
	return nil
}

// Leaf returns the event's Merkle leaf, using stored commitments for erased fields.
func (x *StoredEvent) Leaf() (Hash, error) {
	if err := x.check(); err != nil {
		return Hash{}, err
	}
	return leafOf(EncodingVersion, commitments(x.Salt, &x.Event, x.Erased)), nil
}

// Disclose returns the event with every field revealed except erased ones,
// which carry only their stored commitment.
func (x *StoredEvent) Disclose() (DisclosedEvent, error) {
	if err := x.check(); err != nil {
		return DisclosedEvent{}, err
	}
	return disclose(x.Salt, &x.Event, x.Erased), nil
}

// Erase drops the named fields' values from the stored event, keeping their
// commitments in Erased. Call it on the stored record (an erasure request);
// it is idempotent. "id" and "at" fix the leaf order and cannot be erased.
// An unknown or protected name is an error and changes nothing.
func (x *StoredEvent) Erase(fields ...string) error {
	specs := make([]fieldSpec, 0, len(fields))
	for _, name := range fields {
		f, ok := schemaField(name)
		if !ok {
			return &FieldError{Field: name, Err: ErrUnknownField}
		}
		if orderKey(name) {
			return &FieldError{Field: name, Err: ErrInvalidArgument}
		}
		specs = append(specs, f)
	}
	if x.Erased == nil {
		x.Erased = map[string]Hash{}
	}
	for _, f := range specs {
		if _, done := x.Erased[f.name]; done {
			continue
		}
		x.Erased[f.name] = commit(x.Salt.fieldSalt(f.name), f.name, f.get(&x.Event))
		_ = f.set(&x.Event, nil) // decoding the absent value only zeroes the field
	}
	return nil
}

// CompareEvents is the canonical leaf order: by At, then by ID.
func CompareEvents(a, b *Event) int {
	return cmp.Or(a.At.Compare(b.At), cmp.Compare(a.ID, b.ID))
}

// OrderLeaves puts a window's events into canonical leaf order, in place,
// and returns their leaves in that order. Firestore has no insertion order,
// so the sealer and the exporter both call it on the window's events to agree
// on leaf positions: events sort by [CompareEvents] (At, then the
// server-assigned ID); a full tie, which unique IDs rule out, falls back to
// the leaf hash so the order is total.
func OrderLeaves(events []StoredEvent) ([]Hash, error) {
	type item struct {
		ev   StoredEvent
		leaf Hash
	}
	items := make([]item, len(events))
	for i := range events {
		leaf, err := events[i].Leaf()
		if err != nil {
			return nil, err
		}
		items[i] = item{events[i], leaf}
	}
	slices.SortFunc(items, func(a, b item) int {
		return cmp.Or(CompareEvents(&a.ev.Event, &b.ev.Event), bytes.Compare(a.leaf[:], b.leaf[:]))
	})
	leaves := make([]Hash, len(items))
	for i, it := range items {
		events[i] = it.ev
		leaves[i] = it.leaf
	}
	return leaves, nil
}
