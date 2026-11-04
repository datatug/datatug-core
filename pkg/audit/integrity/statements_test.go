package integrity

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"
)

type failingSigner struct{}

func (failingSigner) KeyID() string { return "kms/down" }
func (failingSigner) Sign(context.Context, []byte) ([]byte, error) {
	return nil, errors.New("kms unavailable")
}

func validStatements() (RootStatement, TombstoneStatement, HoldStatement) {
	w := WindowFor(testSpace, testProject, t0)
	r := RootStatement{Window: w, MerkleRoot: MerkleRoot(nil), ExpiresAt: t0.Add(time.Hour)}
	ts := TombstoneStatement{Seal: r, DeletedAt: r.ExpiresAt.Add(time.Minute)}
	h := HoldStatement{HoldID: "hold-1", Space: testSpace, Project: testProject, From: t0, To: t0.Add(time.Hour),
		Action: HoldRelease, At: t0.Add(2 * time.Hour), Actors: []ActorRef{"act-a", "act-b"}}
	return r, ts, h
}

func TestSignVerifyEveryKind(t *testing.T) {
	f := newFixture(t, 10)
	r, ts, h := validStatements()
	other := Ed25519Verifier{testKeyID: testKey("attacker").Public().(ed25519.PublicKey)}
	check := func(name string, verify func(Verifier) error, tamper func() error) {
		t.Run(name, func(t *testing.T) {
			if err := verify(f.verifier); err != nil {
				t.Fatal(err)
			}
			if err := verify(other); !errors.Is(err, ErrBadSignature) {
				t.Fatalf("other key: want ErrBadSignature, got %v", err)
			}
			if err := verify(Ed25519Verifier{}); !errors.Is(err, ErrUnknownKey) {
				t.Fatalf("no key: want ErrUnknownKey, got %v", err)
			}
			if err := verify(Ed25519Verifier{testKeyID: ed25519.PublicKey{1, 2}}); !errors.Is(err, ErrUnknownKey) {
				t.Fatalf("malformed key: want ErrUnknownKey, got %v", err)
			}
			if err := tamper(); !errors.Is(err, ErrBadSignature) {
				t.Fatalf("tampered: want ErrBadSignature, got %v", err)
			}
		})
	}
	sr := mustSign(t, f.signer, r)
	check("root", sr.Verify, func() error { y := sr; y.Statement.LeafCount++; return y.Verify(f.verifier) })
	check("root expiry", sr.Verify, func() error {
		y := sr
		y.Statement.ExpiresAt = y.Statement.ExpiresAt.Add(time.Hour)
		return y.Verify(f.verifier)
	})
	st := mustSign(t, f.signer, ts)
	check("tombstone", st.Verify, func() error {
		y := st
		y.Statement.DeletedAt = y.Statement.DeletedAt.Add(time.Second)
		return y.Verify(f.verifier)
	})
	sh := mustSign(t, f.signer, h)
	check("hold", sh.Verify, func() error {
		y := sh
		y.Statement.Actors = []ActorRef{"act-a", "act-c"}
		return y.Verify(f.verifier)
	})
	if sr.KeyID != testKeyID || f.signer.KeyID() != testKeyID {
		t.Fatal("key id")
	}
}

func TestSignRejectsInvalidStatements(t *testing.T) {
	f := newFixture(t, 11)
	r, ts, h := validStatements()
	sign := func(s any) error {
		var err error
		switch v := s.(type) {
		case RootStatement:
			_, err = Sign(context.Background(), f.signer, v)
		case TombstoneStatement:
			_, err = Sign(context.Background(), f.signer, v)
		case HoldStatement:
			_, err = Sign(context.Background(), f.signer, v)
		}
		return err
	}
	type tc struct {
		name string
		s    any
		want error
	}
	var cases []tc
	add := func(name string, s any, want error) { cases = append(cases, tc{name, s, want}) }

	bad := r
	bad.Window.Space = ""
	add("root window", bad, ErrInvalidWindow)

	bad = r
	bad.ExpiresAt = time.Time{}
	add("root without expiry", bad, ErrInvalidStatement)
	bad = r
	bad.ExpiresAt = r.Window.End()
	add("root expiring before the window ends", bad, ErrInvalidStatement)

	bt := ts
	bt.Seal.Window.Space = ""
	add("tombstone seal", bt, ErrInvalidWindow)
	bt = ts
	bt.DeletedAt = time.Time{}
	add("tombstone without deletion time", bt, ErrTombstoneBeforeExpiry)
	bt = ts
	bt.DeletedAt = ts.Seal.ExpiresAt.Add(-time.Second)
	add("tombstone before expiry", bt, ErrTombstoneBeforeExpiry)

	hm := func(mut func(*HoldStatement)) HoldStatement { c := h; mut(&c); return c }
	add("hold from", hm(func(c *HoldStatement) { c.From = t0.Add(time.Second) }), ErrInvalidWindow)
	add("hold to", hm(func(c *HoldStatement) { c.To = t0.Add(time.Second) }), ErrInvalidWindow)
	add("hold id", hm(func(c *HoldStatement) { c.HoldID = "" }), ErrInvalidStatement)
	add("hold range", hm(func(c *HoldStatement) { c.To = t0.Add(-time.Hour) }), ErrInvalidStatement)
	add("hold at", hm(func(c *HoldStatement) { c.At = time.Time{} }), ErrInvalidStatement)
	add("hold action", hm(func(c *HoldStatement) { c.Action = "extend" }), ErrInvalidStatement)
	add("hold actors", hm(func(c *HoldStatement) { c.Actors = nil }), ErrInvalidStatement)
	add("release by one", hm(func(c *HoldStatement) { c.Actors = []ActorRef{"act-a"} }), ErrHoldReleaseNeedsTwo)
	add("release by one twice", hm(func(c *HoldStatement) { c.Actors = []ActorRef{"act-a", "act-a", ""} }), ErrHoldReleaseNeedsTwo)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := sign(c.s); !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
		})
	}
	place := hm(func(c *HoldStatement) { c.Action = HoldPlace; c.Actors = []ActorRef{"act-a"} })
	if err := sign(place); err != nil {
		t.Fatalf("one person may place a hold: %v", err)
	}
	// Verify re-validates: a statement that breaks its rules never verifies.
	sh := mustSign(t, f.signer, place)
	sh.Statement.Action = HoldRelease
	if err := sh.Verify(f.verifier); !errors.Is(err, ErrHoldReleaseNeedsTwo) {
		t.Fatalf("want ErrHoldReleaseNeedsTwo, got %v", err)
	}
}

func TestSignerFailures(t *testing.T) {
	r, _, _ := validStatements()
	if _, err := Sign(context.Background(), failingSigner{}, r); err == nil {
		t.Fatal("want signer error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Sign(ctx, newFixture(t, 12).signer, r); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestStatementMessagesAreDomainSeparated(t *testing.T) {
	r, ts, h := validStatements()
	msgs := map[string]bool{}
	for _, m := range [][]byte{r.message(), ts.message(), h.message()} {
		if msgs[string(m)] {
			t.Fatal("two statement kinds share a message")
		}
		msgs[string(m)] = true
	}
}
