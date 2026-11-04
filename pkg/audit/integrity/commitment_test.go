package integrity

import (
	"bytes"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("entropy exhausted") }

func TestNewSalt(t *testing.T) {
	a, err := NewSalt(nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewSalt(nil)
	if err != nil {
		t.Fatal(err)
	}
	if a == b || a == (Salt{}) {
		t.Fatal("crypto/rand salts must differ and be non-zero")
	}
	if _, err := NewSalt(failingReader{}); err == nil {
		t.Fatal("want error from failing reader")
	}
}

func TestFieldNames(t *testing.T) {
	names := FieldNames()
	if len(names) != len(schemaV1) || names[0] != "id" || names[len(names)-1] != "occurredAt" {
		t.Fatalf("unexpected field names %v", names)
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			t.Fatalf("duplicate field %q", n)
		}
		seen[n] = true
	}
	if slices.Contains(names, "expireAt") {
		t.Fatal("expireAt must not be committed")
	}
}

func TestDiscloseRoundTrip(t *testing.T) {
	f := newFixture(t, 1)
	w := WindowFor(testSpace, testProject, t0)
	for i := range 64 {
		ev := f.event(w, i)
		salt, _ := NewSalt(randReader{f.rng})
		d := Disclose(salt, &ev)
		leaf, err := d.Leaf()
		if err != nil {
			t.Fatal(err)
		}
		if leaf != LeafOf(salt, &ev) {
			t.Fatal("disclosed leaf differs from LeafOf")
		}
		got, err := d.Event()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, ev) {
			t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, ev)
		}
		if d.Redacted() != nil {
			t.Fatal("nothing redacted yet")
		}
	}
}

func TestZeroEventRoundTrip(t *testing.T) {
	var ev Event
	d := Disclose(Salt{}, &ev)
	got, err := d.Event()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, ev) {
		t.Fatalf("zero event round trip: %+v", got)
	}
}

func TestLeafBindsSaltAndEveryField(t *testing.T) {
	f := newFixture(t, 2)
	w := WindowFor(testSpace, testProject, t0)
	ev := f.event(w, 0)
	salt, _ := NewSalt(randReader{f.rng})
	base := LeafOf(salt, &ev)
	other := salt
	other[0] ^= 1
	if LeafOf(other, &ev) == base {
		t.Fatal("leaf must depend on the salt")
	}
	mutations := map[string]func(e *Event){
		"id":         func(e *Event) { e.ID += "x" },
		"at":         func(e *Event) { e.At = e.At.Add(time.Nanosecond) },
		"actor.ref":  func(e *Event) { e.Actor.Ref += "x" },
		"objects":    func(e *Event) { e.Resource.Objects = append(e.Resource.Objects, ObjectRef{}) },
		"fields":     func(e *Event) { e.Resource.Fields = nil },
		"count zero": func(e *Event) { e.Operation.RecordCount = ptr(int64(0)) },
		"count nil":  func(e *Event) { e.Operation.RecordCount = nil; e.Operation.Pages = ptr(int64(1)) },
		"bag":        func(e *Event) { e.Resource.Bag = &BagRef{} },
		"occurredAt": func(e *Event) { e.Occurred = time.Unix(0, 0) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			e2 := ev
			e2.Resource.Objects = slices.Clone(ev.Resource.Objects)
			mutate(&e2)
			if LeafOf(salt, &e2) == base {
				t.Fatal("mutation not reflected in leaf")
			}
		})
	}
}

func TestRedactKeepsLeafAndHidesValue(t *testing.T) {
	f := newFixture(t, 3)
	ev := f.event(WindowFor(testSpace, testProject, t0), 0)
	ev.Reason = "approved by Alice Smith"
	salt, _ := NewSalt(randReader{f.rng})
	d := Disclose(salt, &ev)
	r, err := d.Redact("reason", "actor.ref")
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Redacted(); !slices.Equal(got, []string{"reason", "actor.ref"}) {
		t.Fatalf("redacted %v", got)
	}
	l1, _ := d.Leaf()
	l2, err := r.Leaf()
	if err != nil || l1 != l2 {
		t.Fatalf("redaction changed the leaf: %v", err)
	}
	for _, fld := range r.Fields {
		if fld.Name == "reason" && (fld.Salt != nil || len(fld.Value) != 0) {
			t.Fatal("redacted field still carries salt or value")
		}
		if fld.Revealed() && bytes.Contains(fld.Value, []byte("Alice")) {
			t.Fatal("name leaked")
		}
	}
	got, err := r.Event()
	if err != nil || got.Reason != "" || got.Actor.Ref != "" || got.ID != ev.ID {
		t.Fatalf("decoded redacted event wrong: %+v %v", got, err)
	}
	if d.Fields[6].Salt == nil {
		t.Fatal("Redact must not modify the original")
	}
	if _, err := d.Redact("nope"); !errors.Is(err, ErrUnknownField) {
		t.Fatalf("want ErrUnknownField, got %v", err)
	}
	// A revealed field's salt reveals nothing about another field's salt.
	if *d.Fields[0].Salt == *d.Fields[1].Salt {
		t.Fatal("field salts must differ")
	}
}

func TestDisclosedEventRejects(t *testing.T) {
	f := newFixture(t, 4)
	ev := f.event(WindowFor(testSpace, testProject, t0), 0)
	salt, _ := NewSalt(randReader{f.rng})
	good := func() DisclosedEvent {
		d := Disclose(salt, &ev)
		d.Fields = slices.Clone(d.Fields)
		return d
	}
	idx := func(name string) int { return slices.Index(FieldNames(), name) }
	cases := []struct {
		name   string
		mutate func(d *DisclosedEvent)
		want   error
		field  string
	}{
		{"version", func(d *DisclosedEvent) { d.Version = 2 }, ErrUnsupportedVersion, ""},
		{"missing field", func(d *DisclosedEvent) { d.Fields = d.Fields[1:] }, ErrMalformedEvent, ""},
		{"swapped fields", func(d *DisclosedEvent) { d.Fields[0], d.Fields[1] = d.Fields[1], d.Fields[0] }, ErrMalformedEvent, "at"},
		{"redacted with value", func(d *DisclosedEvent) { d.Fields[0].Salt = nil }, ErrMalformedEvent, "id"},
		{"tampered value", func(d *DisclosedEvent) { d.Fields[0].Value = encString("ev-forged") }, ErrFieldTampered, "id"},
		{"tampered salt", func(d *DisclosedEvent) { s := *d.Fields[2].Salt; s[0] ^= 1; d.Fields[2].Salt = &s }, ErrFieldTampered, "kind"},
		{"non-canonical string", func(d *DisclosedEvent) { d.Fields[0].Value = []byte{tagString} }, ErrMalformedEvent, "id"},
		{"wrong tag", func(d *DisclosedEvent) { d.Fields[0].Value = []byte{tagInt, 'a'} }, ErrMalformedEvent, "id"},
		{"bad int", func(d *DisclosedEvent) { d.Fields[idx("operation.pages")].Value = []byte{tagInt, 1} }, ErrMalformedEvent, "operation.pages"},
		{"bad time", func(d *DisclosedEvent) { d.Fields[1].Value = []byte{tagTime, 1} }, ErrMalformedEvent, "at"},
		{"time nanos overflow", func(d *DisclosedEvent) {
			v := encTime(t0)
			v[9], v[10], v[11], v[12] = 0xff, 0xff, 0xff, 0xff
			d.Fields[1].Value = v
		}, ErrMalformedEvent, "at"},
		{"odd objects", func(d *DisclosedEvent) { d.Fields[idx("resource.objects")].Value = encStrings([]string{"a"}) }, ErrMalformedEvent, "resource.objects"},
		{"bag arity", func(d *DisclosedEvent) { d.Fields[idx("resource.bag")].Value = encStrings([]string{"a"}) }, ErrMalformedEvent, "resource.bag"},
		{"bag revision", func(d *DisclosedEvent) {
			d.Fields[idx("resource.bag")].Value = encStrings([]string{"a", "b", "x"})
		}, ErrMalformedEvent, "resource.bag"},
		{"bag revision non-canonical", func(d *DisclosedEvent) {
			d.Fields[idx("resource.bag")].Value = encStrings([]string{"a", "b", "+1"})
		}, ErrMalformedEvent, "resource.bag"},
		{"case arity", func(d *DisclosedEvent) { d.Fields[idx("caseRef")].Value = encStrings([]string{"a"}) }, ErrMalformedEvent, "caseRef"},
		{"exec arity", func(d *DisclosedEvent) { d.Fields[idx("executionRef")].Value = encStrings([]string{"a"}) }, ErrMalformedEvent, "executionRef"},
		{"digest arity", func(d *DisclosedEvent) { d.Fields[idx("statementDigest")].Value = encStrings([]string{"a"}) }, ErrMalformedEvent, "statementDigest"},
		{"digest key version", func(d *DisclosedEvent) {
			d.Fields[idx("statementDigest")].Value = encStrings([]string{"v", "h"})
		}, ErrMalformedEvent, "statementDigest"},
		{"empty list", func(d *DisclosedEvent) { d.Fields[idx("resource.fields")].Value = []byte{tagStrings, 0, 0, 0, 0} }, ErrMalformedEvent, "resource.fields"},
		{"list trailing bytes", func(d *DisclosedEvent) {
			d.Fields[idx("resource.fields")].Value = append(encStrings([]string{"a"}), 0)
		}, ErrMalformedEvent, "resource.fields"},
		{"list short length", func(d *DisclosedEvent) {
			d.Fields[idx("resource.fields")].Value = []byte{tagStrings, 0, 0, 0, 1, 0, 0}
		}, ErrMalformedEvent, "resource.fields"},
		{"list overlong element", func(d *DisclosedEvent) {
			d.Fields[idx("resource.fields")].Value = []byte{tagStrings, 0, 0, 0, 1, 0, 0, 0, 9, 'a'}
		}, ErrMalformedEvent, "resource.fields"},
		{"list too short", func(d *DisclosedEvent) { d.Fields[idx("resource.fields")].Value = []byte{tagStrings} }, ErrMalformedEvent, "resource.fields"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := good()
			tc.mutate(&d)
			_, err := d.Leaf()
			if !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
			var fe *FieldError
			if !errors.As(err, &fe) || fe.Field != tc.field {
				t.Fatalf("want field %q, got %v", tc.field, err)
			}
			if fe.Error() == "" {
				t.Fatal("empty error text")
			}
			if _, err := d.Event(); !errors.Is(err, tc.want) {
				t.Fatalf("Event(): want %v, got %v", tc.want, err)
			}
		})
	}
}
