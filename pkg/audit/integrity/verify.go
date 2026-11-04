package integrity

import (
	"bytes"
	"errors"
	"fmt"
	"time"
)

// Export is what `POST audit/exports` produces and `datatug access audit
// verify` consumes: a claimed-contiguous range of windows of one Space and
// project, their signed statements, and a subset of their events with
// inclusion proofs.
type Export struct {
	Space   string                  `json:"space"`
	Project string                  `json:"project"`
	From    time.Time               `json:"from"` // start of the first window
	To      time.Time               `json:"to"`   // start of the last window
	Windows []ExportWindow          `json:"windows"`
	Holds   []Signed[HoldStatement] `json:"holds,omitempty"`
}

// ExportWindow is one window of an export. It carries a signed root, a
// tombstone, or both (they must agree). Whole claims that every event of the
// window is included; the verifier fails the export if that is not so.
type ExportWindow struct {
	Root      *Signed[RootStatement]      `json:"root,omitempty"`
	Expiry    *Signed[ExpiryStatement]    `json:"expiry,omitempty"`
	Tombstone *Signed[TombstoneStatement] `json:"tombstone,omitempty"`
	Whole     bool                        `json:"whole,omitempty"`
	Events    []ExportEvent               `json:"events,omitempty"`
}

// ExportEvent is one included event at its insertion index, with its proof.
type ExportEvent struct {
	Index uint64         `json:"index"`
	Proof []Hash         `json:"proof"`
	Event DisclosedEvent `json:"event"`
}

// VerifyOptions configure [VerifyExport].
type VerifyOptions struct {
	// Verifier holds the operator's public key(s). Required.
	Verifier Verifier
	// Anchors are window roots from an independent record (the anchor bucket,
	// or the Space's own copy), by [WindowID.Key]. A root that differs is
	// [ErrAnchorMismatch]; an anchor for the window before the range pins the
	// first previous-root link.
	Anchors map[string]Hash
}

// ChainStart says what the first window's previous-root link was checked against.
type ChainStart string

// Chain starts.
const (
	// ChainGenesis: the first window is the first ever sealed (zero previous root).
	ChainGenesis ChainStart = "genesis"
	// ChainAnchored: the previous root matched an anchored root.
	ChainAnchored ChainStart = "anchored"
	// ChainUnverified: nothing to check the first link against.
	ChainUnverified ChainStart = "unverified"
)

// Report is what a successful verification proves.
type Report struct {
	Space      string         `json:"space"`
	Project    string         `json:"project"`
	From       time.Time      `json:"from"`
	To         time.Time      `json:"to"`
	ChainStart ChainStart     `json:"chainStart"`
	Windows    []WindowReport `json:"windows"`
}

// WindowReport is the verdict for one window.
type WindowReport struct {
	Window     WindowID  `json:"window"`
	Root       Hash      `json:"root"`
	LeafCount  uint64    `json:"leafCount"`
	Tombstoned bool      `json:"tombstoned,omitempty"`
	ExpiresAt  time.Time `json:"expiresAt,omitzero"`
	// Complete is true only when every event of the window is included and
	// none has a redacted field. Otherwise only the included events are
	// proven authentic; nothing is proven about omitted ones.
	Complete bool          `json:"complete"`
	Events   []EventReport `json:"events,omitempty"`
}

// EventReport is one authentic event.
type EventReport struct {
	Index    uint64   `json:"index"`
	Leaf     Hash     `json:"leaf"`
	Redacted []string `json:"redacted,omitempty"`
}

// Events is the number of events proven authentic.
func (r *Report) Events() int {
	n := 0
	for _, w := range r.Windows {
		n += len(w.Events)
	}
	return n
}

// Complete reports whether the export proves completeness of the whole
// range: every window is complete or tombstoned (its absence pre-explained).
func (r *Report) Complete() bool {
	for _, w := range r.Windows {
		if !w.Complete && !w.Tombstoned {
			return false
		}
	}
	return true
}

// VerifyExport verifies exp and reports what it proves. Windows are checked in
// order; the first failure is returned as a [*VerifyError] naming the window.
func VerifyExport(exp *Export, opts VerifyOptions) (*Report, error) {
	if exp == nil || opts.Verifier == nil {
		return nil, ErrInvalidArgument
	}
	first := WindowID{Space: exp.Space, Project: exp.Project, Start: exp.From}
	last := WindowID{Space: exp.Space, Project: exp.Project, Start: exp.To}
	for _, w := range []WindowID{first, last} {
		if err := w.Validate(); err != nil {
			return nil, &VerifyError{Window: w, Event: -1, Err: ErrWindowSequence, Cause: err}
		}
	}
	if exp.To.Before(exp.From) {
		return nil, &VerifyError{Window: first, Event: -1, Err: ErrWindowSequence, Cause: errors.New("range ends before it starts")}
	}
	holds, err := verifyHolds(exp, opts.Verifier, first)
	if err != nil {
		return nil, err
	}
	v := &verifier{opts: opts, holds: holds}
	rep := &Report{Space: exp.Space, Project: exp.Project, From: exp.From, To: exp.To, ChainStart: ChainUnverified}
	want := first
	var prev Hash
	i := 0
	for ; !want.Start.After(exp.To); i++ {
		if i >= len(exp.Windows) {
			return nil, &VerifyError{Window: want, Event: -1, Err: ErrMissingWindow}
		}
		wr, err := v.window(&exp.Windows[i], want, i == 0, prev, rep)
		if err != nil {
			return nil, err
		}
		rep.Windows = append(rep.Windows, *wr)
		prev = wr.Root
		want = want.Next()
	}
	if i < len(exp.Windows) {
		return nil, &VerifyError{Window: want, Event: -1, Err: ErrWindowSequence,
			Cause: fmt.Errorf("%d window(s) beyond the range", len(exp.Windows)-i)}
	}
	return rep, nil
}

// signatureKind tells a bad signature from a statement that breaks its own rules.
func signatureKind(err error) error {
	if errors.Is(err, ErrBadSignature) {
		return ErrBadSignature
	}
	return ErrInvalidStatement
}

func verifyHolds(exp *Export, v Verifier, first WindowID) ([]HoldStatement, error) {
	holds := make([]HoldStatement, 0, len(exp.Holds))
	for i, h := range exp.Holds {
		if h.Statement.Space != exp.Space || h.Statement.Project != exp.Project {
			return nil, &VerifyError{Window: first, Event: -1, Err: ErrStatementMismatch,
				Cause: fmt.Errorf("hold %d is for another space or project", i)}
		}
		if err := h.Verify(v); err != nil {
			return nil, &VerifyError{Window: first, Event: -1, Err: signatureKind(err), Cause: fmt.Errorf("hold %d: %w", i, err)}
		}
		holds = append(holds, h.Statement)
	}
	return holds, nil
}

type verifier struct {
	opts  VerifyOptions
	holds []HoldStatement
}

// held reports whether a legal hold covered window w at time t.
func (v *verifier) held(w WindowID, t time.Time) bool {
	for _, p := range v.holds {
		if p.Action != HoldPlace || !p.covers(w) || p.At.After(t) {
			continue
		}
		released := false
		for _, r := range v.holds {
			if r.Action == HoldRelease && r.HoldID == p.HoldID && !r.At.After(t) {
				released = true
			}
		}
		if !released {
			return true
		}
	}
	return false
}

func (v *verifier) window(ew *ExportWindow, want WindowID, first bool, prev Hash, rep *Report) (*WindowReport, error) {
	fail := func(kind, cause error) error { return &VerifyError{Window: want, Event: -1, Err: kind, Cause: cause} }

	var seal RootStatement
	switch {
	case ew.Tombstone != nil:
		seal = ew.Tombstone.Statement.Seal
	case ew.Root != nil:
		seal = ew.Root.Statement
	default:
		return nil, fail(ErrMissingSeal, nil)
	}
	if !seal.Window.Equal(want) {
		got := seal.Window
		if got.Space == want.Space && got.Project == want.Project && got.Start.After(want.Start) {
			return nil, fail(ErrMissingWindow, fmt.Errorf("next window present is %s", got.Key()))
		}
		return nil, fail(ErrWindowSequence, fmt.Errorf("got window %s", got.Key()))
	}
	if ew.Root != nil {
		if err := ew.Root.Verify(v.opts.Verifier); err != nil {
			return nil, fail(ErrBadSignature, err)
		}
	}
	root := seal.Root()
	wr := &WindowReport{Window: want, Root: root, LeafCount: seal.LeafCount}
	if ew.Expiry != nil {
		x := ew.Expiry.Statement
		if !x.Window.Equal(want) || x.Root != root {
			return nil, fail(ErrStatementMismatch, errors.New("expiry statement names another window or root"))
		}
		if err := ew.Expiry.Verify(v.opts.Verifier); err != nil {
			return nil, fail(signatureKind(err), err)
		}
		wr.ExpiresAt = x.ExpiresAt
	}
	if ew.Tombstone != nil {
		if err := v.tombstone(ew, want, seal); err != nil {
			return nil, fail(ErrForgedTombstone, err)
		}
		wr.Tombstoned = true
		wr.ExpiresAt = ew.Tombstone.Statement.ExpiresAt
	}
	if first {
		if a, ok := v.opts.Anchors[want.Prev().Key()]; ok {
			if seal.PrevRoot != a {
				return nil, fail(ErrChainBroken, fmt.Errorf("previous root %s, anchored %s", seal.PrevRoot, a))
			}
			rep.ChainStart = ChainAnchored
		} else if seal.PrevRoot.IsZero() {
			rep.ChainStart = ChainGenesis
		}
	} else if seal.PrevRoot != prev {
		return nil, fail(ErrChainBroken, fmt.Errorf("previous root %s, previous window's root %s", seal.PrevRoot, prev))
	}
	if a, ok := v.opts.Anchors[want.Key()]; ok && a != root {
		return nil, fail(ErrAnchorMismatch, fmt.Errorf("root %s, anchored %s", root, a))
	}
	if wr.Tombstoned {
		return wr, nil
	}
	if err := v.events(ew, want, seal, wr); err != nil {
		return nil, err
	}
	return wr, nil
}

func (v *verifier) tombstone(ew *ExportWindow, want WindowID, seal RootStatement) error {
	t := ew.Tombstone
	if err := t.Verify(v.opts.Verifier); err != nil {
		return err
	}
	if ew.Root != nil && ew.Root.Statement.Root() != seal.Root() {
		return fmt.Errorf("%w: tombstone seal differs from the signed root", ErrStatementMismatch)
	}
	if ew.Expiry != nil && !ew.Expiry.Statement.ExpiresAt.Equal(t.Statement.ExpiresAt) {
		return fmt.Errorf("%w: tombstone expiry differs from the expiry statement", ErrStatementMismatch)
	}
	if len(ew.Events) > 0 {
		return ErrTombstoneHasEvents
	}
	if v.held(want, t.Statement.DeletedAt) {
		return ErrDeletedUnderHold
	}
	return nil
}

func eventKind(err error) error {
	for _, s := range []error{ErrFieldTampered, ErrUnsupportedVersion} {
		if errors.Is(err, s) {
			return s
		}
	}
	return ErrMalformedEvent
}

func (v *verifier) events(ew *ExportWindow, want WindowID, seal RootStatement, wr *WindowReport) error {
	fail := func(j int, field string, kind, cause error) error {
		return &VerifyError{Window: want, Event: j, Field: field, Err: kind, Cause: cause}
	}
	leaves := make([]Hash, len(ew.Events))
	for j, ev := range ew.Events {
		if ev.Index >= seal.LeafCount {
			return fail(j, "", ErrLeafIndex, fmt.Errorf("index %d of %d", ev.Index, seal.LeafCount))
		}
		if j > 0 && ev.Index <= ew.Events[j-1].Index {
			return fail(j, "", ErrLeafOrder, fmt.Errorf("index %d after %d", ev.Index, ew.Events[j-1].Index))
		}
		leaf, err := ev.Event.Leaf()
		if err != nil {
			var fe *FieldError
			_ = errors.As(err, &fe)
			return fail(j, fe.Field, eventKind(err), err)
		}
		leaves[j] = leaf
	}
	for j, ev := range ew.Events {
		if VerifyInclusion(leaves[j], ev.Index, seal.LeafCount, ev.Proof, seal.MerkleRoot) == nil {
			continue
		}
		for k, other := range ew.Events {
			if k != j && VerifyInclusion(leaves[j], other.Index, seal.LeafCount, other.Proof, seal.MerkleRoot) == nil {
				return fail(j, "", ErrLeafReordered, fmt.Errorf("event claims index %d but proves at %d", ev.Index, other.Index))
			}
		}
		return fail(j, "", ErrInclusionProof, nil)
	}
	all := uint64(len(ew.Events)) == seal.LeafCount
	if ew.Whole && !all {
		return fail(-1, "", ErrIncompleteWindow, fmt.Errorf("%d of %d events", len(ew.Events), seal.LeafCount))
	}
	wr.Complete = all
	for j, ev := range ew.Events {
		redacted := ev.Event.Redacted()
		if len(redacted) > 0 {
			wr.Complete = false
		}
		wr.Events = append(wr.Events, EventReport{Index: ev.Index, Leaf: leaves[j], Redacted: redacted})
	}
	return nil
}

// VerifyErasure verifies exp and then proves that the erased user id appears
// nowhere in it: events name people only by pseudonymous [ActorRef], so
// deleting the identity-map entry leaves every event and root verifiable.
// A revealed field containing the id is [ErrIdentityLeak].
func VerifyErasure(exp *Export, opts VerifyOptions, erasedUserID string) (*Report, error) {
	if erasedUserID == "" {
		return nil, ErrInvalidArgument
	}
	rep, err := VerifyExport(exp, opts)
	if err != nil {
		return nil, err
	}
	needle := []byte(erasedUserID)
	for i, w := range exp.Windows {
		for j, ev := range w.Events {
			for _, f := range ev.Event.Fields {
				if f.Revealed() && bytes.Contains(f.Value, needle) {
					return nil, &VerifyError{Window: rep.Windows[i].Window, Event: j, Field: f.Name, Err: ErrIdentityLeak}
				}
			}
		}
	}
	return rep, nil
}
