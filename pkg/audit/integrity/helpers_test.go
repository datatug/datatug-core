package integrity

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"fmt"
	mrand "math/rand/v2"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 3, 21, 5, 0, 0, time.UTC)

const (
	testSpace   = "s1"
	testProject = "shop"
	testKeyID   = "kms/audit-sealer/1"
)

func ptr[T any](v T) *T { return &v }

// testKey derives a deterministic Ed25519 key from a label.
func testKey(label string) ed25519.PrivateKey {
	seed := bytes.Repeat([]byte(label), ed25519.SeedSize/len(label)+1)[:ed25519.SeedSize]
	return ed25519.NewKeyFromSeed(seed)
}

type fixture struct {
	t        testing.TB
	signer   *Ed25519Signer
	verifier Ed25519Verifier
	rng      *mrand.Rand
}

func newFixture(t testing.TB, seed uint64) *fixture {
	t.Helper()
	key := testKey("operator")
	return &fixture{
		t:        t,
		signer:   NewEd25519Signer(testKeyID, key),
		verifier: Ed25519Verifier{testKeyID: key.Public().(ed25519.PublicKey)},
		rng:      mrand.New(mrand.NewPCG(seed, seed^0x9e3779b97f4a7c15)),
	}
}

func (f *fixture) opts() VerifyOptions { return VerifyOptions{Verifier: f.verifier} }

// randReader adapts the fixture's PRNG to io.Reader (salts, pseudonyms).
type randReader struct{ r *mrand.Rand }

func (r randReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(r.r.Uint32())
	}
	return len(p), nil
}

func (f *fixture) word(prefix string) string { return fmt.Sprintf("%s-%x", prefix, f.rng.Uint32()) }

func (f *fixture) maybe() bool { return f.rng.IntN(2) == 0 }

// event builds a plausible random event for window w.
func (f *fixture) event(w WindowID, i int) Event {
	at := w.Start.Add(time.Duration(i) * time.Second)
	e := Event{
		ID:     fmt.Sprintf("ev-%s-%d", w.Start.Format("1504"), i),
		At:     at,
		Kind:   "activity",
		Action: "operation.authorized",
		Tier:   "server-enforced",
		Actor: Actor{
			Ref: ActorRef(f.word("act")), Kind: "user", CredentialFamily: "firebase", ClientID: "datatug-cli/0.137.0",
		},
		Resource: Resource{
			Space: w.Space, Project: w.Project, Environment: "prod", Connection: f.word("conn"),
			Objects: []ObjectRef{{Namespace: "billing", Name: f.word("t")}},
			Fields:  []string{"id", f.word("col")},
		},
		Operation: Operation{Op: "read", CorrelationID: f.word("op")},
		Authority: Authority{Grants: []string{f.word("G")}, Roles: []string{"developer"}, PolicyFingerprint: "sha256:" + f.word("p")},
		Window:    w.Key(),
	}
	if f.maybe() {
		e.Kind, e.Action, e.Outcome, e.Reason = "access", "access.approved", "allow", f.word("because")
		e.Access = Parties{Requester: ActorRef(f.word("act")), Approver: ActorRef(f.word("act")), Beneficiary: ActorRef(f.word("act"))}
	}
	if f.maybe() {
		e.Operation.RecordCount, e.Operation.Pages = ptr(int64(f.rng.IntN(1000))), ptr(int64(0))
		e.Authority.ProtectionRevision = ptr(int64(7))
	}
	if f.maybe() {
		e.Resource.Bag = &BagRef{Session: f.word("s"), Bag: f.word("b"), Revision: int64(f.rng.IntN(9))}
		e.CaseRef = &CaseRef{Kind: "incident", ID: f.word("inc")}
		e.Execution = &ExecRef{StoreID: "local", Project: w.Project, ID: f.word("x")}
		e.Statement = &StmtHMAC{KeyVersion: 3, HMAC: f.word("h")}
	}
	if f.maybe() {
		e.Occurred = at.Add(-time.Hour)
		e.Operation.Denials = ptr(int64(-1))
		e.Actor.OnBehalfOf = ActorRef(f.word("act"))
		e.Authority.Decisions, e.Authority.Standing, e.Authority.TeamBasis = []string{"ev-1"}, []string{"posture"}, []string{""}
	}
	return e
}

const retention = 365 * 24 * time.Hour

type sealed struct {
	id     WindowID
	events []StoredEvent // canonical leaf order
	leaves []Hash
	tree   *Tree
	stmt   RootStatement
	root   Signed[RootStatement]
}

// sealWindow simulates the sealer for one window: canonical order, seal, sign.
func (f *fixture) sealWindow(w WindowID, prev Hash, events []StoredEvent) sealed {
	f.t.Helper()
	leaves, err := OrderLeaves(events)
	if err != nil {
		f.t.Fatal(err)
	}
	stmt, err := Seal(w, prev, leaves, w.Start.Add(retention))
	if err != nil {
		f.t.Fatal(err)
	}
	return sealed{id: w, events: events, leaves: leaves, tree: NewTree(leaves), stmt: stmt, root: mustSign(f.t, f.signer, stmt)}
}

// storedEvents writes n random events into window w, in a shuffled
// (Firestore-like) order.
func (f *fixture) storedEvents(w WindowID, n int) []StoredEvent {
	f.t.Helper()
	events := make([]StoredEvent, 0, n)
	for i := range n {
		salt, err := NewSalt(randReader{f.rng})
		if err != nil {
			f.t.Fatal(err)
		}
		events = append(events, StoredEvent{Salt: salt, Event: f.event(w, i)})
	}
	f.rng.Shuffle(len(events), func(i, j int) { events[i], events[j] = events[j], events[i] })
	return events
}

// sealChain simulates the writer and the sealer: counts[i] events in the
// i-th window from start, sealed in order and chained.
func (f *fixture) sealChain(start time.Time, prev Hash, counts ...int) []sealed {
	f.t.Helper()
	w := WindowFor(testSpace, testProject, start)
	var out []sealed
	for _, n := range counts {
		s := f.sealWindow(w, prev, f.storedEvents(w, n))
		out = append(out, s)
		prev = s.stmt.Root()
		w = w.Next()
	}
	return out
}

func mustSign[S statement](t testing.TB, signer Signer, s S) Signed[S] {
	t.Helper()
	x, err := Sign(context.Background(), signer, s)
	if err != nil {
		t.Fatal(err)
	}
	return x
}

// exportEvent discloses stored event i of window s with its proof.
func exportEvent(t testing.TB, s sealed, i int) ExportEvent {
	t.Helper()
	proof, err := s.tree.Proof(uint64(i))
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.events[i].Disclose()
	if err != nil {
		t.Fatal(err)
	}
	return ExportEvent{Index: uint64(i), Proof: proof, Event: d}
}

// fullExport exports every window whole, every event revealed.
func fullExport(t testing.TB, ws []sealed) *Export {
	t.Helper()
	exp := &Export{Space: testSpace, Project: testProject, From: ws[0].id.Start, To: ws[len(ws)-1].id.Start}
	for _, s := range ws {
		root := s.root
		ew := ExportWindow{Root: &root, Whole: true}
		for i := range s.events {
			ew.Events = append(ew.Events, exportEvent(t, s, i))
		}
		exp.Windows = append(exp.Windows, ew)
	}
	return exp
}

// tombstoneFor signs a retention tombstone for s.
func (f *fixture) tombstoneFor(s sealed, deletedAt time.Time) *Signed[TombstoneStatement] {
	x := mustSign(f.t, f.signer, TombstoneStatement{Seal: s.stmt, DeletedAt: deletedAt})
	return &x
}

func (s sealed) expires() time.Time { return s.stmt.ExpiresAt }
