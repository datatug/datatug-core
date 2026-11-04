package integrity

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// signRaw signs a statement without validating it (a forger with the key).
func signRaw[S statement](t testing.TB, signer Signer, s S) Signed[S] {
	t.Helper()
	sig, err := signer.Sign(context.Background(), s.message())
	if err != nil {
		t.Fatal(err)
	}
	return Signed[S]{Statement: s, KeyID: signer.KeyID(), Signature: sig}
}

func attacker() *Ed25519Signer { return NewEd25519Signer(testKeyID, testKey("attacker")) }

func TestVerifyFullExport(t *testing.T) {
	f := newFixture(t, 20)
	ws := f.sealChain(t0, Hash{}, 3, 0, 5)
	rep, err := VerifyExport(fullExport(t, ws), f.opts())
	if err != nil {
		t.Fatal(err)
	}
	if rep.ChainStart != ChainGenesis || len(rep.Windows) != 3 || rep.Events() != 8 || !rep.Complete() {
		t.Fatalf("report %+v", rep)
	}
	for i, w := range rep.Windows {
		if !w.Complete || w.Root != ws[i].stmt.Root() || !w.Window.Equal(ws[i].id) || w.ExpiresAt.IsZero() || w.Tombstoned {
			t.Fatalf("window %d: %+v", i, w)
		}
	}
	if rep.Windows[2].Events[4].Leaf != ws[2].leaves[4] {
		t.Fatal("event leaf")
	}
}

func TestVerifyFilteredAndRedactedExportProvesAuthenticityOnly(t *testing.T) {
	f := newFixture(t, 21)
	ws := f.sealChain(t0, Hash{}, 4, 3)
	exp := fullExport(t, ws)
	// Window 0: a filtered subset. Window 1: every event, one redacted field.
	exp.Windows[0].Whole = false
	exp.Windows[0].Events = []ExportEvent{exp.Windows[0].Events[1], exp.Windows[0].Events[3]}
	red, err := exp.Windows[1].Events[2].Event.Redact("reason", "actor.ref")
	if err != nil {
		t.Fatal(err)
	}
	exp.Windows[1].Events[2].Event = red
	rep, err := VerifyExport(exp, f.opts())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Windows[0].Complete || rep.Windows[1].Complete || rep.Complete() {
		t.Fatal("completeness must not be claimed for filtered or redacted windows")
	}
	if rep.Events() != 5 || len(rep.Windows[1].Events[2].Redacted) != 2 || rep.Holds != HoldsFromExportOnly {
		t.Fatalf("report %+v", rep)
	}
	// An empty window is complete even when not marked whole.
	ws = f.sealChain(t0, Hash{}, 0)
	exp = fullExport(t, ws)
	exp.Windows[0].Whole = false
	rep, err = VerifyExport(exp, f.opts())
	if err != nil || !rep.Complete() {
		t.Fatalf("empty window: %v %+v", err, rep)
	}
}

func TestVerifyExportJSONRoundTrip(t *testing.T) {
	f := newFixture(t, 22)
	ws := f.sealChain(t0, Hash{}, 2, 1)
	exp := fullExport(t, ws)
	exp.Windows[1].Tombstone = f.tombstoneFor(ws[1], ws[1].expires())
	exp.Windows[1].Root = nil // a tombstone replaces the root
	exp.Windows[1].Events = nil
	exp.Windows[1].Whole = false
	hold := mustSign(t, f.signer, HoldStatement{HoldID: "h", Space: testSpace, Project: testProject, From: t0, To: t0,
		Action: HoldPlace, At: t0, Actors: []ActorRef{"act-a"}})
	exp.Holds = []Signed[HoldStatement]{hold}
	b, err := json.Marshal(exp)
	if err != nil {
		t.Fatal(err)
	}
	var back Export
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	rep, err := VerifyExport(&back, f.opts())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(rep); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyArguments(t *testing.T) {
	f := newFixture(t, 23)
	exp := fullExport(t, f.sealChain(t0, Hash{}, 1))
	if _, err := VerifyExport(nil, f.opts()); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal(err)
	}
	if _, err := VerifyExport(exp, VerifyOptions{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal(err)
	}
	if _, err := VerifyErasure(exp, f.opts(), ""); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal(err)
	}
	if _, err := VerifyErasure(nil, f.opts(), "u"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatal(err)
	}
}

type tamperCase struct {
	name   string
	tamper func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions
	want   []error
	window int // index of the window the error must name
	event  int
	field  string
}

// TestVerifyDetectsTampering covers every tamper class. The chain is three
// windows of 3, 2 and 4 events, exported whole.
func TestVerifyDetectsTampering(t *testing.T) {
	cases := []tamperCase{
		{
			name: "altered field, commitment kept",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Windows[1].Events[0].Event.Fields[0].Value = encString("ev-forged")
				return f.opts()
			},
			want: []error{ErrFieldTampered}, window: 1, event: 0, field: "id",
		},
		{
			name: "altered event, commitments recomputed",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				ev := ws[1].events[1].Event
				ev.Operation.Op = "delete"
				exp.Windows[1].Events[1].Event = Disclose(ws[1].events[1].Salt, &ev)
				return f.opts()
			},
			want: []error{ErrInclusionProof}, window: 1, event: 1,
		},
		{
			name: "middle window removed",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Windows = append(exp.Windows[:1], exp.Windows[2:]...)
				return f.opts()
			},
			want: []error{ErrMissingWindow}, window: 1, event: -1,
		},
		{
			name: "last window removed",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Windows = exp.Windows[:2]
				return f.opts()
			},
			want: []error{ErrMissingWindow}, window: 2, event: -1,
		},
		{
			name: "last window truncated",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Windows[2].Events = exp.Windows[2].Events[:3]
				return f.opts()
			},
			want: []error{ErrIncompleteWindow}, window: 2, event: -1,
		},
		{
			name: "last window truncated and re-sealed with another key",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				s, _ := Seal(ws[2].id, ws[1].stmt.Root(), ws[2].leaves[:3], ws[2].expires())
				forged := mustSign(t, attacker(), s)
				exp.Windows[2].Root = &forged
				exp.Windows[2].Events = exp.Windows[2].Events[:3]
				return f.opts()
			},
			want: []error{ErrBadSignature}, window: 2, event: -1,
		},
		{
			name: "root re-signed with an untrusted key id",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				forged := mustSign(t, NewEd25519Signer("someone-else", testKey("attacker")), ws[0].stmt)
				exp.Windows[0].Root = &forged
				return f.opts()
			},
			want: []error{ErrBadSignature, ErrUnknownKey}, window: 0, event: -1,
		},
		{
			name: "middle root re-signed with the real key after dropping an event",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				s, _ := Seal(ws[1].id, ws[0].stmt.Root(), ws[1].leaves[:1], ws[1].expires())
				forged := mustSign(t, f.signer, s)
				exp.Windows[1].Root = &forged
				exp.Windows[1].Events = exp.Windows[1].Events[:1]
				exp.Windows[1].Events[0].Proof = nil // the honest proof in a one-leaf tree
				return f.opts()
			},
			want: []error{ErrChainBroken}, window: 2, event: -1,
		},
		{
			name: "last root re-signed with the real key, caught by the anchor",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				s, _ := Seal(ws[2].id, ws[1].stmt.Root(), ws[2].leaves[:3], ws[2].expires())
				forged := mustSign(t, f.signer, s)
				exp.Windows[2].Root = &forged
				exp.Windows[2].Events = exp.Windows[2].Events[:3]
				o := f.opts()
				o.Anchors = map[string]Hash{ws[2].id.Key(): ws[2].stmt.Root()}
				return o
			},
			want: []error{ErrAnchorMismatch}, window: 2, event: -1,
		},
		{
			name: "first link differs from the anchored previous root",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				o := f.opts()
				o.Anchors = map[string]Hash{ws[0].id.Prev().Key(): {9}}
				return o
			},
			want: []error{ErrChainBroken}, window: 0, event: -1,
		},
		{
			name: "leaf reordered (contents swapped, positions kept)",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				e := exp.Windows[2].Events
				e[1].Event, e[2].Event = e[2].Event, e[1].Event
				return f.opts()
			},
			want: []error{ErrLeafReordered}, window: 2, event: 1,
		},
		{
			name: "events listed out of insertion order",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				e := exp.Windows[0].Events
				e[0], e[1] = e[1], e[0]
				return f.opts()
			},
			want: []error{ErrLeafOrder}, window: 0, event: 1,
		},
		{
			name: "event duplicated",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Windows[0].Whole = false
				exp.Windows[0].Events = append(exp.Windows[0].Events[:2], exp.Windows[0].Events[1])
				return f.opts()
			},
			want: []error{ErrLeafOrder}, window: 0, event: 2,
		},
		{
			name: "index past the window",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Windows[1].Events[1].Index = 7
				return f.opts()
			},
			want: []error{ErrLeafIndex}, window: 1, event: 1,
		},
		{
			name: "event moved to another window",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Windows[1].Whole = false
				exp.Windows[1].Events = []ExportEvent{exp.Windows[0].Events[0]}
				return f.opts()
			},
			want: []error{ErrInclusionProof}, window: 1, event: 0,
		},
		{
			name: "unsupported event version",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Windows[0].Events[2].Event.Version = 9
				return f.opts()
			},
			want: []error{ErrUnsupportedVersion}, window: 0, event: 2,
		},
		{
			name: "malformed event",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Windows[0].Events[2].Event.Fields[1].Value = []byte{tagTime}
				return f.opts()
			},
			want: []error{ErrMalformedEvent}, window: 0, event: 2, field: "at",
		},
		{
			name: "window without seal",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Windows[1].Root = nil
				return f.opts()
			},
			want: []error{ErrMissingSeal}, window: 1, event: -1,
		},
		{
			name: "windows out of order",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Windows[1], exp.Windows[0] = exp.Windows[0], exp.Windows[1]
				return f.opts()
			},
			want: []error{ErrMissingWindow}, window: 0, event: -1,
		},
		{
			name: "window duplicated",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Windows[1] = exp.Windows[0]
				return f.opts()
			},
			want: []error{ErrWindowSequence}, window: 1, event: -1,
		},
		{
			name: "window of another project",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Project = "other"
				return f.opts()
			},
			want: []error{ErrWindowSequence}, window: -2, event: -1,
		},
		{
			name: "windows beyond the claimed range",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.To = exp.From.Add(WindowDuration)
				return f.opts()
			},
			want: []error{ErrWindowSequence}, window: 2, event: -1,
		},
		{
			name: "range misaligned",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.To = exp.To.Add(time.Second)
				return f.opts()
			},
			want: []error{ErrWindowSequence, ErrInvalidWindow}, window: -2, event: -1,
		},
		{
			name: "range reversed",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.To = exp.From.Add(-WindowDuration)
				return f.opts()
			},
			want: []error{ErrWindowSequence}, window: 0, event: -1,
		},
		{
			name: "root expiry altered (pre-explained absence moved)",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				r := *exp.Windows[1].Root
				r.Statement.ExpiresAt = r.Statement.ExpiresAt.Add(-24 * time.Hour)
				exp.Windows[1].Root = &r
				return f.opts()
			},
			want: []error{ErrBadSignature}, window: 1, event: -1,
		},
		{
			name: "root that breaks its own rules",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				s := ws[1].stmt
				s.ExpiresAt = time.Time{}
				r := signRaw(t, f.signer, s)
				exp.Windows[1].Root = &r
				return f.opts()
			},
			want: []error{ErrInvalidStatement}, window: 1, event: -1,
		},
		{
			name: "event names another window",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				return f.resealWith(t, ws, exp, 1, func(e *Event) { e.Window = ws[0].id.Key() })
			},
			want: []error{ErrEventMisplaced}, window: 1, event: 0, field: "window",
		},
		{
			name: "event names another space",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				return f.resealWith(t, ws, exp, 1, func(e *Event) { e.Resource.Space = "s2" })
			},
			want: []error{ErrEventMisplaced}, window: 1, event: 0, field: "resource.space",
		},
		{
			name: "event names another project",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				return f.resealWith(t, ws, exp, 1, func(e *Event) { e.Resource.Project = "other" })
			},
			want: []error{ErrEventMisplaced}, window: 1, event: 0, field: "resource.project",
		},
		{
			name: "window sealed out of canonical order",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				return f.resealWith(t, ws, exp, 1, nil)
			},
			want: []error{ErrLeafOrder}, window: 1, event: 1,
		},
		{
			name: "tombstone forged with another key",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				x := mustSign(t, attacker(), TombstoneStatement{Seal: ws[1].stmt, DeletedAt: ws[1].expires()})
				exp.Windows[1].Tombstone, exp.Windows[1].Events = &x, nil
				exp.Windows[1].Root = nil // a tombstone replaces the root
				return f.opts()
			},
			want: []error{ErrForgedTombstone, ErrBadSignature}, window: 1, event: -1,
		},
		{
			name: "tombstone dated before expiry",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				x := signRaw(t, f.signer, TombstoneStatement{Seal: ws[1].stmt, DeletedAt: ws[1].id.Start})
				exp.Windows[1].Tombstone, exp.Windows[1].Events = &x, nil
				exp.Windows[1].Root = nil // a tombstone replaces the root
				return f.opts()
			},
			want: []error{ErrForgedTombstone, ErrTombstoneBeforeExpiry}, window: 1, event: -1,
		},
		{
			name: "root and tombstone both present",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Windows[1].Tombstone = f.tombstoneFor(ws[1], ws[1].expires())
				exp.Windows[1].Events = nil
				return f.opts()
			},
			want: []error{ErrStatementMismatch}, window: 1, event: -1,
		},
		{
			name: "tombstone with an invented seal and no root",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				s := ws[1].stmt
				s.LeafCount = 0
				s.MerkleRoot = MerkleRoot(nil)
				exp.Windows[1].Tombstone = mustSignPtr(t, f.signer, TombstoneStatement{Seal: s, DeletedAt: ws[1].expires()})
				exp.Windows[1].Root, exp.Windows[1].Events = nil, nil
				return f.opts()
			},
			want: []error{ErrChainBroken}, window: 2, event: -1,
		},
		{
			name: "tombstoned window still has events",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Windows[1].Tombstone = f.tombstoneFor(ws[1], ws[1].expires())
				exp.Windows[1].Root = nil // a tombstone replaces the root
				return f.opts()
			},
			want: []error{ErrForgedTombstone, ErrTombstoneHasEvents}, window: 1, event: -1,
		},
		{
			name: "tombstone dated while a legal hold was in force",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Windows[1].Tombstone = f.tombstoneFor(ws[1], ws[1].expires().Add(time.Hour))
				exp.Windows[1].Root = nil // a tombstone replaces the root
				exp.Windows[1].Events = nil
				exp.Holds = []Signed[HoldStatement]{
					mustSign(t, f.signer, HoldStatement{HoldID: "h1", Space: testSpace, Project: testProject, From: t0, To: ws[2].id.Start,
						Action: HoldPlace, At: t0.Add(time.Hour), Actors: []ActorRef{"act-a"}}),
				}
				return f.opts()
			},
			want: []error{ErrForgedTombstone, ErrDeletedUnderHold}, window: 1, event: -1,
		},
		{
			name: "hold for another space",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Holds = []Signed[HoldStatement]{mustSign(t, f.signer, HoldStatement{HoldID: "h1", Space: "s2", From: t0, To: t0,
					Action: HoldPlace, At: t0, Actors: []ActorRef{"act-a"}})}
				return f.opts()
			},
			want: []error{ErrStatementMismatch}, window: 0, event: -1,
		},
		{
			name: "hold forged",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Holds = []Signed[HoldStatement]{mustSign(t, attacker(), HoldStatement{HoldID: "h1", Space: testSpace,
					Project: testProject, From: t0, To: t0, Action: HoldPlace, At: t0, Actors: []ActorRef{"act-a"}})}
				return f.opts()
			},
			want: []error{ErrBadSignature}, window: 0, event: -1,
		},
		{
			name: "hold released by one person",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Holds = []Signed[HoldStatement]{signRaw(t, f.signer, HoldStatement{HoldID: "h1", Space: testSpace,
					Project: testProject, From: t0, To: t0, Action: HoldRelease, At: t0, Actors: []ActorRef{"act-a"}})}
				return f.opts()
			},
			want: []error{ErrInvalidStatement, ErrHoldReleaseNeedsTwo}, window: 0, event: -1,
		},
		{
			name: "deletion under a hold the export omits, found in the anchor",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Windows[1].Tombstone = f.tombstoneFor(ws[1], ws[1].expires().Add(time.Hour))
				exp.Windows[1].Root = nil // a tombstone replaces the root
				exp.Windows[1].Events = nil
				o := f.opts()
				o.AnchoredHolds = []Signed[HoldStatement]{mustSign(t, f.signer, hold("h1", ws[1].id.Start, ws[1].id.Start, HoldPlace, t0, "act-a"))}
				o.HoldsAnchored = true
				return o
			},
			want: []error{ErrForgedTombstone, ErrDeletedUnderHold}, window: 1, event: -1,
		},
		{
			name: "anchored hold forged",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				o := f.opts()
				o.AnchoredHolds = []Signed[HoldStatement]{mustSign(t, attacker(), hold("h1", t0, t0, HoldPlace, t0, "act-a"))}
				return o
			},
			want: []error{ErrBadSignature}, window: 0, event: -1,
		},
		{
			name: "release dated before its placement",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Holds = []Signed[HoldStatement]{
					mustSign(t, f.signer, hold("h1", t0, t0, HoldPlace, t0.Add(time.Hour), "act-a")),
					mustSign(t, f.signer, hold("h1", t0, t0, HoldRelease, t0, "act-a", "act-b")),
				}
				return f.opts()
			},
			want: []error{ErrStatementMismatch}, window: 0, event: -1,
		},
		{
			name: "release of another range",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Holds = []Signed[HoldStatement]{
					mustSign(t, f.signer, hold("h1", t0, ws[2].id.Start, HoldPlace, t0, "act-a")),
					mustSign(t, f.signer, hold("h1", t0, t0, HoldRelease, t0.Add(time.Hour), "act-a", "act-b")),
				}
				return f.opts()
			},
			want: []error{ErrStatementMismatch}, window: 0, event: -1,
		},
		{
			name: "export-only release lifting an anchored hold",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				del := ws[1].expires().Add(time.Hour)
				exp.Windows[1].Tombstone = f.tombstoneFor(ws[1], del)
				exp.Windows[1].Root, exp.Windows[1].Events = nil, nil
				place := hold("h1", ws[1].id.Start, ws[1].id.Start, HoldPlace, t0, "act-a")
				o := f.opts()
				o.AnchoredHolds, o.HoldsAnchored = []Signed[HoldStatement]{mustSign(t, f.signer, place)}, true
				exp.Holds = []Signed[HoldStatement]{mustSign(t, f.signer,
					hold("h1", ws[1].id.Start, ws[1].id.Start, HoldRelease, del.Add(-time.Minute), "act-a", "act-b"))}
				return o
			},
			want: []error{ErrStatementMismatch}, window: 0, event: -1,
		},
		{
			name: "hold re-placed after an earlier release still binds",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Windows[1].Tombstone = f.tombstoneFor(ws[1], ws[1].expires().Add(time.Hour))
				exp.Windows[1].Root, exp.Windows[1].Events = nil, nil
				from := ws[1].id.Start
				exp.Holds = []Signed[HoldStatement]{
					mustSign(t, f.signer, hold("h1", from, from, HoldPlace, t0, "act-a")),
					mustSign(t, f.signer, hold("h1", from, from, HoldRelease, t0.Add(time.Hour), "act-a", "act-b")),
					mustSign(t, f.signer, hold("h1", from, from, HoldPlace, t0.Add(2*time.Hour), "act-a")),
				}
				return f.opts()
			},
			want: []error{ErrForgedTombstone, ErrDeletedUnderHold}, window: 1, event: -1,
		},
		{
			name: "retention below the policy floor",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				o := f.opts()
				o.MinRetention = retention + time.Hour
				return o
			},
			want: []error{ErrRetentionTooShort}, window: 0, event: -1,
		},
		{
			name: "event time before its window",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				return f.resealWith(t, ws, exp, 1, func(e *Event) { e.At = ws[1].id.Start.Add(-time.Nanosecond) })
			},
			want: []error{ErrEventMisplaced}, window: 1, event: 0, field: "at",
		},
		{
			name: "event time past its window and the grace",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				return f.resealWith(t, ws, exp, 1, func(e *Event) { e.At = ws[1].id.End().Add(SealGrace) })
			},
			want: []error{ErrEventMisplaced}, window: 1, event: 1, field: "at",
		},
		{
			name: "erased label on a revealed field",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				exp.Windows[0].Events[0].Event.Fields[6].Erased = true
				return f.opts()
			},
			want: []error{ErrMalformedEvent}, window: 0, event: 0, field: "reason",
		},
		{
			name: "release without a placement",
			tamper: func(t *testing.T, f *fixture, ws []sealed, exp *Export) VerifyOptions {
				o := f.opts()
				o.AnchoredHolds = []Signed[HoldStatement]{mustSign(t, f.signer, hold("h9", t0, t0, HoldRelease, t0, "act-a", "act-b"))}
				return o
			},
			want: []error{ErrStatementMismatch}, window: 0, event: -1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, 30)
			ws := f.sealChain(t0, Hash{}, 3, 2, 4)
			exp := fullExport(t, ws)
			opts := tc.tamper(t, f, ws, exp)
			rep, err := VerifyExport(exp, opts)
			if err == nil || rep != nil {
				t.Fatal("tampering not detected")
			}
			for _, w := range tc.want {
				if !errors.Is(err, w) {
					t.Fatalf("want %v, got %v", w, err)
				}
			}
			var ve *VerifyError
			if !errors.As(err, &ve) {
				t.Fatalf("want *VerifyError, got %T", err)
			}
			if tc.window >= 0 && !ve.Window.Equal(ws[tc.window].id) {
				t.Fatalf("names window %s, want %s", ve.Window.Key(), ws[tc.window].id.Key())
			}
			if ve.Event != tc.event || ve.Field != tc.field {
				t.Fatalf("event %d field %q, want %d %q (%v)", ve.Event, ve.Field, tc.event, tc.field, err)
			}
			if !strings.Contains(err.Error(), "window space=") {
				t.Fatalf("error text %q", err)
			}
		})
	}
}

func hold(id string, from, to time.Time, action HoldAction, at time.Time, actors ...ActorRef) HoldStatement {
	return HoldStatement{HoldID: id, Space: testSpace, Project: testProject, From: from, To: to, Action: action, At: at, Actors: actors}
}

// resealWith re-seals window wi with the real key: mutate changes its first
// event; a nil mutate seals the events in reverse canonical order instead.
// The window is re-exported whole.
func (f *fixture) resealWith(t *testing.T, ws []sealed, exp *Export, wi int, mutate func(*Event)) VerifyOptions {
	t.Helper()
	events := slices.Clone(ws[wi].events)
	var leaves []Hash
	if mutate != nil {
		mutate(&events[0].Event)
		leaves, _ = OrderLeaves(events)
	} else {
		slices.Reverse(events)
		for i := range events {
			l, _ := events[i].Leaf()
			leaves = append(leaves, l)
		}
	}
	stmt, err := Seal(ws[wi].id, ws[wi].stmt.PrevRoot, leaves, ws[wi].expires())
	if err != nil {
		t.Fatal(err)
	}
	s := sealed{id: ws[wi].id, events: events, leaves: leaves, tree: NewTree(leaves), stmt: stmt, root: mustSign(t, f.signer, stmt)}
	exp.Windows[wi] = fullExport(t, []sealed{s}).Windows[0]
	return f.opts()
}

func mustSignPtr[S statement](t testing.TB, signer Signer, s S) *Signed[S] {
	t.Helper()
	x := mustSign(t, signer, s)
	return &x
}

func TestVerifyTombstones(t *testing.T) {
	f := newFixture(t, 40)
	ws := f.sealChain(t0, Hash{}, 2, 3, 1)
	exp := fullExport(t, ws)
	expires := ws[0].expires()
	// Window 0: tombstone only (root aged out of the export too).
	exp.Windows[0] = ExportWindow{Tombstone: f.tombstoneFor(ws[0], expires.Add(time.Minute))}
	// Window 1: tombstone with root, deleted after a hold was released by two.
	exp.Windows[1].Tombstone = f.tombstoneFor(ws[1], ws[1].expires().Add(48*time.Hour))
	exp.Windows[1].Root = nil // a tombstone replaces the root
	exp.Windows[1].Events, exp.Windows[1].Whole = nil, false
	place := HoldStatement{HoldID: "h1", Space: testSpace, Project: testProject, From: ws[1].id.Start, To: ws[1].id.Start,
		Action: HoldPlace, At: t0.Add(time.Hour), Actors: []ActorRef{"act-a"}}
	release := place
	release.Action, release.At, release.Actors = HoldRelease, ws[1].expires().Add(time.Hour), []ActorRef{"act-a", "act-b"}
	later := place
	later.HoldID, later.At = "h2", ws[1].expires().Add(72*time.Hour) // placed after the deletion
	other := place
	other.HoldID, other.From, other.To = "h3", ws[2].id.Start, ws[2].id.Start // covers another window
	exp.Holds = []Signed[HoldStatement]{
		mustSign(t, f.signer, place), mustSign(t, f.signer, release), mustSign(t, f.signer, later), mustSign(t, f.signer, other),
	}
	rep, err := VerifyExport(exp, f.opts())
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Windows[0].Tombstoned || !rep.Windows[1].Tombstoned || rep.Windows[2].Tombstoned || !rep.Complete() {
		t.Fatalf("report %+v", rep)
	}
	if rep.Windows[0].Root != ws[0].stmt.Root() || !rep.Windows[0].ExpiresAt.Equal(expires) || rep.Holds != HoldsFromExportOnly {
		t.Fatal("tombstone root, expiry or hold coverage")
	}
	// The same holds read from the anchor bucket instead; a hold of another
	// project in the bucket is ignored.
	o := f.opts()
	o.AnchoredHolds, o.HoldsAnchored = exp.Holds, true
	foreign := place
	foreign.Project = "other"
	o.AnchoredHolds = append(o.AnchoredHolds, mustSign(t, f.signer, foreign))
	exported := exp.Holds
	exp.Holds = nil
	rep, err = VerifyExport(exp, o)
	if err != nil || rep.Holds != HoldsFromAnchor {
		t.Fatalf("anchored holds: %v %+v", err, rep)
	}
	// The export may repeat anchored holds.
	exp.Holds = exported
	if rep, err = VerifyExport(exp, o); err != nil || rep.Holds != HoldsFromAnchor {
		t.Fatalf("export repeating the anchor: %v", err)
	}
	// A policy floor the seals meet passes.
	o.MinRetention = retention
	if _, err := VerifyExport(exp, o); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyChainStart(t *testing.T) {
	f := newFixture(t, 50)
	prev := Hash{7}
	ws := f.sealChain(t0, prev, 1, 1)
	exp := fullExport(t, ws)
	rep, err := VerifyExport(exp, f.opts())
	if err != nil || rep.ChainStart != ChainUnverified {
		t.Fatalf("%v %v", err, rep)
	}
	o := f.opts()
	o.Anchors = map[string]Hash{ws[0].id.Prev().Key(): prev, ws[0].id.Key(): ws[0].stmt.Root(), ws[1].id.Key(): ws[1].stmt.Root()}
	rep, err = VerifyExport(exp, o)
	if err != nil || rep.ChainStart != ChainAnchored {
		t.Fatalf("%v %v", err, rep)
	}
	// A sub-range of a longer chain verifies on its own.
	exp.From = exp.To
	exp.Windows = exp.Windows[1:]
	rep, err = VerifyExport(exp, f.opts())
	if err != nil || rep.ChainStart != ChainUnverified || len(rep.Windows) != 1 {
		t.Fatalf("%v %v", err, rep)
	}
}

func TestVerifyErrorText(t *testing.T) {
	e := &VerifyError{Window: WindowFor("s", "p", t0), Event: 2, Field: "reason", Err: ErrFieldTampered, Cause: errors.New("why")}
	got := e.Error()
	for _, want := range []string{"space=s|project=p|", "event 2", `field "reason"`, "why"} {
		if !strings.Contains(got, want) {
			t.Fatalf("%q lacks %q", got, want)
		}
	}
	short := (&VerifyError{Window: WindowFor("s", "p", t0), Event: -1, Err: ErrMissingWindow}).Error()
	if strings.Contains(short, "event") || strings.Contains(short, "field") {
		t.Fatalf("%q", short)
	}
}
