package integrity

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

const aliceUID = "firebase-uid-alice"

// erasureFixture seals one window whose two events name Alice: by pseudonym,
// and (event 1) in a free-text reason.
func erasureFixture(t *testing.T) (*fixture, *IdentityMap, ActorRef, sealed) {
	f := newFixture(t, 60)
	ids := NewIdentityMap(randReader{f.rng})
	alice, err := ids.Pseudonym(testSpace, aliceUID)
	if err != nil {
		t.Fatal(err)
	}
	w := WindowFor(testSpace, testProject, t0)
	events := f.storedEvents(w, 2)
	slices.SortFunc(events, func(a, b StoredEvent) int { return CompareEvents(&a.Event, &b.Event) })
	events[0].Event.Actor.Ref = alice
	events[1].Event.Access.Requester = alice
	events[1].Event.Reason = "requested on behalf of " + aliceUID
	return f, ids, alice, f.sealWindow(w, Hash{}, events)
}

// TestErasureKeepsVerification is AC:erasure-keeps-verification through the
// store, as §19.4 describes it: the identity-map entry is deleted and the
// stored reason is replaced by its commitment; exports made afterwards from
// the store still verify against the root sealed before the erasure, and
// the erased value is gone.
func TestErasureKeepsVerification(t *testing.T) {
	f, ids, alice, s := erasureFixture(t)
	root := s.stmt.Root()

	before := fullExport(t, []sealed{s})
	_, err := VerifyErasure(before, f.opts(), aliceUID)
	var ve *VerifyError
	if !errors.Is(err, ErrIdentityLeak) || !errors.As(err, &ve) || ve.Event != 1 || ve.Field != "reason" {
		t.Fatalf("want leak in event 1 reason, got %v", err)
	}

	// The erasure request: delete the identity-map entry, erase the stored reason.
	if _, ok := ids.Erase(testSpace, aliceUID); !ok {
		t.Fatal("erase identity")
	}
	stored := &s.events[1]
	if err := stored.Erase("reason"); err != nil {
		t.Fatal(err)
	}
	commitment := stored.Erased["reason"]
	if err := stored.Erase("reason"); err != nil || stored.Erased["reason"] != commitment {
		t.Fatal("Erase must be idempotent")
	}
	if stored.Event.Reason != "" {
		t.Fatal("the store still holds the value")
	}
	record, _ := json.Marshal(stored)
	if strings.Contains(string(record), "alice") {
		t.Fatal("the stored record still names the person")
	}

	// A later export, built from the store only, verifies against the old root.
	after := fullExport(t, []sealed{s})
	rep, err := VerifyErasure(after, f.opts(), aliceUID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Windows[0].Root != root || !slices.Equal(rep.Windows[0].Events[1].Redacted, []string{"reason"}) || rep.Windows[0].Complete {
		t.Fatalf("report %+v", rep.Windows[0])
	}
	b, _ := json.Marshal(after)
	if strings.Contains(string(b), "alice") {
		t.Fatal("the export still names the person")
	}
	got, _ := after.Windows[0].Events[0].Event.Event()
	if got.Actor.Ref != alice {
		t.Fatal("events keep the pseudonym")
	}
	if _, ok := ids.Resolve(testSpace, alice); ok {
		t.Fatal("pseudonym no longer resolves")
	}

	// Without the stored commitment the leaf would change: the failure the
	// stored path exists to prevent.
	naive := fullExport(t, []sealed{s})
	naive.Windows[0].Events[1].Event = Disclose(stored.Salt, &stored.Event)
	if _, err := VerifyExport(naive, f.opts()); !errors.Is(err, ErrInclusionProof) {
		t.Fatalf("want ErrInclusionProof, got %v", err)
	}

	// A broken export still fails before the leak scan.
	after.Windows[0].Root = nil
	if _, err := VerifyErasure(after, f.opts(), aliceUID); !errors.Is(err, ErrMissingSeal) {
		t.Fatal(err)
	}
}

func TestEraseRejects(t *testing.T) {
	_, _, _, s := erasureFixture(t)
	x := s.events[1]
	want := x.Event.Reason
	for name, wantErr := range map[string]error{"nope": ErrUnknownField, "id": ErrInvalidArgument, "at": ErrInvalidArgument} {
		if err := x.Erase("reason", name); !errors.Is(err, wantErr) {
			t.Fatalf("%s: want %v, got %v", name, wantErr, err)
		}
	}
	if x.Event.Reason != want || x.Erased != nil {
		t.Fatal("a rejected Erase must change nothing")
	}
	x.Erased = map[string]Hash{"nope": {}}
	if _, err := x.Leaf(); !errors.Is(err, ErrUnknownField) {
		t.Fatal(err)
	}
	if _, err := x.Disclose(); !errors.Is(err, ErrUnknownField) {
		t.Fatal(err)
	}
	if _, err := OrderLeaves([]StoredEvent{x}); !errors.Is(err, ErrUnknownField) {
		t.Fatal(err)
	}
}

func TestEraseEveryFieldKind(t *testing.T) {
	f := newFixture(t, 61)
	w := WindowFor(testSpace, testProject, t0)
	for range 32 {
		x := f.storedEvents(w, 1)[0]
		leaf, _ := x.Leaf()
		var names []string
		for _, n := range FieldNames() {
			if n != "id" && n != "at" {
				names = append(names, n)
			}
		}
		if err := x.Erase(names...); err != nil {
			t.Fatal(err)
		}
		after, err := x.Leaf()
		if err != nil || after != leaf {
			t.Fatal("erasing every erasable field must keep the leaf")
		}
		e := x.Event
		if e.Reason != "" || e.Resource.Bag != nil || e.CaseRef != nil || e.Execution != nil || e.Statement != nil ||
			e.Resource.Objects != nil || e.Operation.RecordCount != nil || !e.Occurred.IsZero() || e.Actor.Ref != "" {
			t.Fatalf("values left after erasure: %+v", e)
		}
		d, _ := x.Disclose()
		if r := d.Redacted(); len(r) != len(names) {
			t.Fatalf("redacted %d of %d", len(r), len(names))
		}
	}
}

func TestOrderLeavesIsCanonical(t *testing.T) {
	f := newFixture(t, 62)
	w := WindowFor(testSpace, testProject, t0)
	events := f.storedEvents(w, 12)
	// Same instant for several events: the id decides; a full tie (same at and id) falls back to the leaf.
	for i := range events {
		events[i].Event.At = t0.Add(time.Duration(i%3) * time.Second)
	}
	twin := events[0]
	twin.Salt[0] ^= 1
	events = append(events, twin)
	a := slices.Clone(events)
	la, err := OrderLeaves(a)
	if err != nil {
		t.Fatal(err)
	}
	b := slices.Clone(events)
	slices.Reverse(b)
	lb, _ := OrderLeaves(b)
	if !slices.Equal(la, lb) {
		t.Fatal("order depends on the input order")
	}
	for i := 1; i < len(a); i++ {
		c := CompareEvents(&a[i-1].Event, &a[i].Event)
		if c > 0 || c == 0 && string(la[i-1][:]) > string(la[i][:]) {
			t.Fatalf("not canonical at %d", i)
		}
		if l, _ := a[i].Leaf(); l != la[i] {
			t.Fatal("leaves do not follow the events")
		}
	}
}

func TestVerifyErasureMatching(t *testing.T) {
	f, _, _, s := erasureFixture(t)
	s.events[1].Event.Reason = ""
	s.events[0].Event.Authority.Grants = []string{"G1", "Granted to FIREBASE-UID-ALICE"}
	s = f.sealWindow(s.id, Hash{}, s.events)
	exp := fullExport(t, []sealed{s})
	cases := []struct {
		id    string
		leak  bool
		field string
	}{
		{aliceUID, true, "authority.grants"},
		{"Firebase UID Alice", true, "authority.grants"},
		{"al", false, ""},         // a short id is not a substring match ("approval")
		{"uid-alicex", false, ""}, // nor a prefix of a token
	}
	for _, tc := range cases {
		_, err := VerifyErasure(exp, f.opts(), tc.id)
		var ve *VerifyError
		if tc.leak != errors.As(err, &ve) || tc.leak && (ve.Field != tc.field || !errors.Is(err, ErrIdentityLeak)) {
			t.Fatalf("%q: %v", tc.id, err)
		}
	}
	if _, err := VerifyErasure(exp, f.opts(), "-- --"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal(err)
	}
	if got := valueStrings(encInt(ptr(int64(7)))); got != nil {
		t.Fatal("numbers hold no text")
	}
}
