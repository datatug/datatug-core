package integrity

import (
	"bytes"
	"encoding/json"
	"testing"
)

// FuzzDisclosedFieldValue feeds arbitrary bytes as the value of one field,
// with a matching commitment, so only the value decoder judges it. Accepted
// values must be canonical: re-disclosing the decoded event reproduces them.
func FuzzDisclosedFieldValue(f *testing.F) {
	fx := newFixture(f, 70)
	w := WindowFor(testSpace, testProject, t0)
	ev := fx.event(w, 0)
	salt, _ := NewSalt(randReader{fx.rng})
	base := Disclose(salt, &ev)
	for i, fl := range base.Fields {
		f.Add(uint8(i), fl.Value)
	}
	f.Add(uint8(1), []byte{tagTime, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff})
	f.Add(uint8(21), []byte{tagStrings, 0xff, 0xff, 0xff, 0xff})
	f.Fuzz(func(t *testing.T, field uint8, value []byte) {
		i := int(field) % len(base.Fields)
		d := DisclosedEvent{Version: base.Version, Fields: append([]DisclosedField(nil), base.Fields...)}
		fs := *d.Fields[i].Salt
		d.Fields[i].Value = value
		d.Fields[i].Commitment = commit(fs, d.Fields[i].Name, value)
		got, err := d.Event()
		if _, lerr := d.Leaf(); (err == nil) != (lerr == nil) {
			t.Fatalf("Event and Leaf disagree: %v / %v", err, lerr)
		}
		if err != nil {
			return
		}
		again := Disclose(salt, &got)
		if !bytes.Equal(again.Fields[i].Value, value) {
			t.Fatalf("accepted non-canonical value %x, re-encodes as %x", value, again.Fields[i].Value)
		}
	})
}

// FuzzParseHash checks that only the canonical form parses.
func FuzzParseHash(f *testing.F) {
	f.Add(Hash{1, 2, 3}.String())
	f.Add("")
	f.Add("ZZ")
	f.Fuzz(func(t *testing.T, s string) {
		h, err := ParseHash(s)
		if err == nil && h.String() != s {
			t.Fatalf("parsed %q as %s", s, h)
		}
	})
}

// FuzzVerifyExportJSON decodes arbitrary JSON as an export and verifies it:
// it must never panic, and anything that verifies must have valid signatures
// from the trusted key, so random input essentially never verifies.
func FuzzVerifyExportJSON(f *testing.F) {
	fx := newFixture(f, 71)
	ws := fx.sealChain(t0, Hash{}, 2, 0, 1)
	exp := fullExport(f, ws)
	exp.Windows[1].Tombstone = fx.tombstoneFor(ws[1], ws[1].expires())
	seed, err := json.Marshal(exp)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	f.Add([]byte(`{"space":"s1","from":"2026-11-03T21:05:00Z","to":"2026-11-03T21:05:00Z","windows":[{}]}`))
	f.Add([]byte(`{"space":"s1","from":"2026-11-03T21:05:00Z","to":"9999-11-03T21:05:00Z"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var e Export
		if json.Unmarshal(data, &e) != nil {
			return
		}
		rep, err := VerifyExport(&e, fx.opts())
		if err == nil && rep == nil {
			t.Fatal("nil report without error")
		}
	})
}
