package integrity

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestNewActorRef(t *testing.T) {
	a, err := NewActorRef(nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewActorRef(nil)
	if a == b || !strings.HasPrefix(string(a), "act-") || len(a) != 4+26 || strings.ToLower(string(a)) != string(a) {
		t.Fatalf("refs %q %q", a, b)
	}
	if _, err := NewActorRef(failingReader{}); err == nil {
		t.Fatal("want error")
	}
}

func TestIdentityMap(t *testing.T) {
	m := NewIdentityMap(nil)
	if _, err := m.Pseudonym("", "u"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal(err)
	}
	if _, err := m.Pseudonym("s", ""); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal(err)
	}
	a1, err := m.Pseudonym("s1", "alice")
	if err != nil {
		t.Fatal(err)
	}
	a2, _ := m.Pseudonym("s1", "alice")
	a3, _ := m.Pseudonym("s2", "alice")
	if a1 != a2 || a1 == a3 {
		t.Fatal("pseudonyms are stable per Space and differ across Spaces")
	}
	if strings.Contains(string(a1), "alice") {
		t.Fatal("pseudonym derived from the id")
	}
	if u, ok := m.Resolve("s1", a1); !ok || u != "alice" {
		t.Fatal("resolve")
	}
	if ref, ok := m.Erase("s1", "alice"); !ok || ref != a1 {
		t.Fatal("erase")
	}
	if _, ok := m.Resolve("s1", a1); ok {
		t.Fatal("erased pseudonym still resolves")
	}
	if _, ok := m.Erase("s1", "alice"); ok {
		t.Fatal("double erase")
	}
	if u, ok := m.Resolve("s2", a3); !ok || u != "alice" {
		t.Fatal("erasure in one Space must not touch another")
	}
	if a4, _ := m.Pseudonym("s1", "alice"); a4 == a1 {
		t.Fatal("a returning user gets a fresh pseudonym")
	}
}

func TestIdentityMapRandomFailures(t *testing.T) {
	if _, err := NewIdentityMap(failingReader{}).Pseudonym("s", "u"); err == nil {
		t.Fatal("want reader error")
	}
	m := NewIdentityMap(zeroReader{})
	if _, err := m.Pseudonym("s", "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Pseudonym("s", "u2"); !errors.Is(err, ErrPseudonymCollision) {
		t.Fatalf("want ErrPseudonymCollision, got %v", err)
	}
}

func TestIdentityMapConcurrent(t *testing.T) {
	m := NewIdentityMap(nil)
	var wg sync.WaitGroup
	refs := make([]ActorRef, 32)
	for i := range refs {
		wg.Go(func() {
			r, err := m.Pseudonym("s", "shared")
			if err != nil {
				t.Error(err)
			}
			refs[i] = r
			m.Resolve("s", r)
		})
	}
	wg.Wait()
	for _, r := range refs {
		if r != refs[0] {
			t.Fatal("concurrent callers got different pseudonyms")
		}
	}
}

// TestErasureKeepsVerification is AC:erasure-keeps-verification: after the
// identity-map entry is erased, the export still verifies, shows only the
// pseudonym, and contains no trace of the user id.
func TestErasureKeepsVerification(t *testing.T) {
	const userID = "firebase-uid-alice"
	f := newFixture(t, 60)
	ids := NewIdentityMap(randReader{f.rng})
	alice, err := ids.Pseudonym(testSpace, userID)
	if err != nil {
		t.Fatal(err)
	}
	w := WindowFor(testSpace, testProject, t0)
	e1, e2 := f.event(w, 0), f.event(w, 1)
	e1.Actor.Ref = alice
	e2.Access.Requester = alice
	e2.Reason = "requested on behalf of " + userID // free text that names the person
	s := sealed{id: w}
	for _, ev := range []Event{e1, e2} {
		salt, _ := NewSalt(randReader{f.rng})
		s.events = append(s.events, stored{salt: salt, event: ev})
		s.leaves = append(s.leaves, LeafOf(salt, &ev))
	}
	s.stmt, _ = Seal(w, Hash{}, s.leaves)
	s.root = mustSign(t, f.signer, s.stmt)
	s.expiry = mustSign(t, f.signer, ExpiryStatement{Window: w, Root: s.stmt.Root(), ExpiresAt: t0.AddDate(1, 0, 0)})
	exp := fullExport(t, []sealed{s})

	if _, ok := ids.Erase(testSpace, userID); !ok {
		t.Fatal("erase")
	}
	_, err = VerifyErasure(exp, f.opts(), userID)
	var ve *VerifyError
	if !errors.Is(err, ErrIdentityLeak) || !errors.As(err, &ve) || ve.Event != 1 || ve.Field != "reason" {
		t.Fatalf("want leak in event 1 reason, got %v", err)
	}
	// The erasure replaces the stored reason with its commitment.
	red, err := exp.Windows[0].Events[1].Event.Redact("reason")
	if err != nil {
		t.Fatal(err)
	}
	exp.Windows[0].Events[1].Event = red
	rep, err := VerifyErasure(exp, f.opts(), userID)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Events() != 2 || rep.Windows[0].Root != s.stmt.Root() {
		t.Fatal("roots and events untouched")
	}
	got, _ := exp.Windows[0].Events[0].Event.Event()
	if got.Actor.Ref != alice {
		t.Fatal("event shows the pseudonym")
	}
	if _, ok := ids.Resolve(testSpace, alice); ok {
		t.Fatal("pseudonym no longer resolves")
	}
	// A broken export still fails before the leak scan.
	exp.Windows[0].Root = nil
	if _, err := VerifyErasure(exp, f.opts(), userID); !errors.Is(err, ErrMissingSeal) {
		t.Fatal(err)
	}
}
