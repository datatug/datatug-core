package integrity

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
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

// ExportWindow is one window of an export. It carries exactly one of a signed
// root or a tombstone (which embeds the signed seal). Whole claims that every event of the
// window is included; the verifier fails the export if that is not so.
type ExportWindow struct {
	Root      *Signed[RootStatement]      `json:"root,omitempty"`
	Tombstone *Signed[TombstoneStatement] `json:"tombstone,omitempty"`
	Whole     bool                        `json:"whole,omitempty"`
	Events    []ExportEvent               `json:"events,omitempty"`
}

// ExportEvent is one included event at its leaf index, with its proof.
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
	// AnchoredHolds are hold statements read from the anchor bucket, not from
	// the export. They are merged with the export's own; statements for
	// another Space or project are ignored.
	AnchoredHolds []Signed[HoldStatement]
	// HoldsAnchored declares that AnchoredHolds is the complete set of holds
	// for this Space and project in the anchor bucket. Tombstones are then
	// judged on the anchored holds only, every hold in the export must also
	// be in the anchor, and the report says hold coverage was independent.
	HoldsAnchored bool
	// MinRetention is the shortest retention policy allows (normally
	// [RetentionFloor]). A seal expiring earlier than its window's end plus MinRetention
	// is [ErrRetentionTooShort]. Zero skips the check.
	MinRetention time.Duration
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

// HoldCoverage says where the legal holds used to judge tombstones came from.
type HoldCoverage string

// Hold coverages.
const (
	// HoldsFromAnchor: the complete hold set came from the independent anchor.
	HoldsFromAnchor HoldCoverage = "anchored"
	// HoldsFromExportOnly: only holds the exporter chose to include were
	// seen; a deletion under an omitted hold would not be detected.
	HoldsFromExportOnly HoldCoverage = "export-only"
)

// Report is what a successful verification proves.
type Report struct {
	Space      string         `json:"space"`
	Project    string         `json:"project"`
	From       time.Time      `json:"from"`
	To         time.Time      `json:"to"`
	ChainStart ChainStart     `json:"chainStart"`
	Holds      HoldCoverage   `json:"holds"`
	Windows    []WindowReport `json:"windows"`
}

// WindowReport is the verdict for one window.
type WindowReport struct {
	Window     WindowID  `json:"window"`
	Root       Hash      `json:"root"`
	LeafCount  uint64    `json:"leafCount"`
	ExpiresAt  time.Time `json:"expiresAt"`
	Tombstoned bool      `json:"tombstoned,omitempty"`
	// Complete is true when every event (leaf) of the window is included,
	// whatever is hidden inside them. Otherwise only the included events are
	// proven authentic; nothing is proven about omitted ones.
	Complete bool `json:"complete"`
	// Disclosed is true when no included event hides a field, whether the
	// exporter labels it redacted or erased.
	Disclosed bool          `json:"disclosed"`
	Events    []EventReport `json:"events,omitempty"`
}

// EventReport is one authentic event.
type EventReport struct {
	Index uint64 `json:"index"`
	Leaf  Hash   `json:"leaf"`
	// Disclosed is true when every field of the event is revealed.
	Disclosed bool `json:"disclosed"`
	// Redacted and Erased are the hidden fields as the exporter labels them:
	// hidden from this reader, or dropped by the store (§19.4 erasure). The
	// labels are not authenticated; either way the field is proven only by
	// its commitment.
	Redacted []string `json:"redacted,omitempty"`
	Erased   []string `json:"erased,omitempty"`
}

// Events is the number of events proven authentic.
func (r *Report) Events() int {
	n := 0
	for _, w := range r.Windows {
		n += len(w.Events)
	}
	return n
}

// Disclosed reports whether every included event of the range reveals every field.
func (r *Report) Disclosed() bool {
	for _, w := range r.Windows {
		if !w.Disclosed && !w.Tombstoned {
			return false
		}
	}
	return true
}

// Complete reports whether the export proves completeness of the whole
// range: every window is complete or tombstoned (its absence pre-explained).
// It says nothing about hidden fields; see [Report.Disclosed].
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
	holds, err := verifyHolds(exp, opts, first)
	if err != nil {
		return nil, err
	}
	v := &verifier{opts: opts, holds: holds}
	rep := &Report{Space: exp.Space, Project: exp.Project, From: exp.From, To: exp.To,
		ChainStart: ChainUnverified, Holds: HoldsFromExportOnly}
	if opts.HoldsAnchored {
		rep.Holds = HoldsFromAnchor
	}
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

// verifyHolds checks every hold statement from the export and the anchor,
// and requires each release to follow a placement of the same hold and range.
func verifyHolds(exp *Export, opts VerifyOptions, first WindowID) ([]HoldStatement, error) {
	fail := func(kind, cause error) error { return &VerifyError{Window: first, Event: -1, Err: kind, Cause: cause} }
	ours := func(h HoldStatement) bool { return h.Space == exp.Space && h.Project == exp.Project }
	var exported, anchored []HoldStatement
	for i, h := range exp.Holds {
		if !ours(h.Statement) {
			return nil, fail(ErrStatementMismatch, fmt.Errorf("export hold %d is for another space or project", i))
		}
		if err := h.Verify(opts.Verifier); err != nil {
			return nil, fail(signatureKind(err), fmt.Errorf("export hold %d: %w", i, err))
		}
		exported = append(exported, h.Statement)
	}
	for i, h := range opts.AnchoredHolds {
		if !ours(h.Statement) {
			continue
		}
		if err := h.Verify(opts.Verifier); err != nil {
			return nil, fail(signatureKind(err), fmt.Errorf("anchored hold %d: %w", i, err))
		}
		anchored = append(anchored, h.Statement)
	}
	holds := append(exported, anchored...)
	if opts.HoldsAnchored {
		// The anchor is the record; the export may only repeat it. A statement
		// the anchor lacks (say, a release signed by the audited operator)
		// must not lift an anchored hold.
		for i, h := range exported {
			if !slices.ContainsFunc(anchored, func(a HoldStatement) bool { return bytes.Equal(a.message(), h.message()) }) {
				return nil, fail(ErrStatementMismatch, fmt.Errorf("export hold %d is not in the anchor", i))
			}
		}
		holds = anchored
	}
	for _, r := range holds {
		if r.Action == HoldRelease && !slices.ContainsFunc(holds, func(p HoldStatement) bool { return releases(r, p) }) {
			return nil, fail(ErrStatementMismatch, fmt.Errorf("release of hold %q matches no earlier placement of the same range", r.HoldID))
		}
	}
	return holds, nil
}

// releases reports whether release r lifts placement p: same hold and range,
// dated no earlier than the placement.
func releases(r, p HoldStatement) bool {
	return p.Action == HoldPlace && r.sameScope(p) && !r.At.Before(p.At)
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
		lifted := slices.ContainsFunc(v.holds, func(r HoldStatement) bool {
			return r.Action == HoldRelease && releases(r, p) && !r.At.After(t)
		})
		if !lifted {
			return true
		}
	}
	return false
}

func (v *verifier) window(ew *ExportWindow, want WindowID, first bool, prev Hash, rep *Report) (*WindowReport, error) {
	fail := func(kind, cause error) error { return &VerifyError{Window: want, Event: -1, Err: kind, Cause: cause} }

	var seal RootStatement
	switch {
	case ew.Tombstone != nil && ew.Root != nil:
		return nil, fail(ErrStatementMismatch, errors.New("a window carries a signed root or a tombstone, not both"))
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
			return nil, fail(signatureKind(err), err)
		}
	}
	if v.opts.MinRetention > 0 {
		if err := seal.checkRetention(v.opts.MinRetention); err != nil {
			return nil, fail(ErrRetentionTooShort, err)
		}
	}
	root := seal.Root()
	wr := &WindowReport{Window: want, Root: root, LeafCount: seal.LeafCount, ExpiresAt: seal.ExpiresAt}
	if ew.Tombstone != nil {
		if err := v.tombstone(ew, want, seal); err != nil {
			return nil, fail(ErrForgedTombstone, err)
		}
		wr.Tombstoned = true
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

// misplaced names the first revealed location field of d that contradicts
// the window it is proven in. Writers place an event by its arrival time, so
// "at" lies in [start, end + SealGrace).
func misplaced(d DisclosedEvent, e *Event, want WindowID) string {
	switch {
	case d.revealed("window") && e.Window != want.Key():
		return "window"
	case d.revealed("resource.space") && e.Resource.Space != want.Space:
		return "resource.space"
	case d.revealed("resource.project") && e.Resource.Project != want.Project:
		return "resource.project"
	case d.revealed("at") && (e.At.Before(want.Start) || !e.At.Before(want.End().Add(SealGrace))):
		return "at"
	}
	return ""
}

// outOfOrder reports whether two consecutive included events, both with
// "at" and "id" revealed, break the canonical leaf order.
func outOfOrder(a, b DisclosedEvent, ea, eb *Event, la, lb Hash) bool {
	for _, d := range []DisclosedEvent{a, b} {
		if !d.revealed("at") || !d.revealed("id") {
			return false
		}
	}
	c := CompareEvents(ea, eb)
	return c > 0 || c == 0 && bytes.Compare(la[:], lb[:]) > 0
}

func (v *verifier) events(ew *ExportWindow, want WindowID, seal RootStatement, wr *WindowReport) error {
	fail := func(j int, field string, kind, cause error) error {
		return &VerifyError{Window: want, Event: j, Field: field, Err: kind, Cause: cause}
	}
	leaves := make([]Hash, len(ew.Events))
	decoded := make([]Event, len(ew.Events))
	for j, ev := range ew.Events {
		if ev.Index >= seal.LeafCount {
			return fail(j, "", ErrLeafIndex, fmt.Errorf("index %d of %d", ev.Index, seal.LeafCount))
		}
		if j > 0 && ev.Index <= ew.Events[j-1].Index {
			return fail(j, "", ErrLeafOrder, fmt.Errorf("index %d after %d", ev.Index, ew.Events[j-1].Index))
		}
		e, leaf, err := ev.Event.verified()
		if err != nil {
			var fe *FieldError
			_ = errors.As(err, &fe)
			return fail(j, fe.Field, eventKind(err), err)
		}
		leaves[j], decoded[j] = leaf, e
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
	for j, ev := range ew.Events {
		if field := misplaced(ev.Event, &decoded[j], want); field != "" {
			return fail(j, field, ErrEventMisplaced, nil)
		}
		if j > 0 && outOfOrder(ew.Events[j-1].Event, ev.Event, &decoded[j-1], &decoded[j], leaves[j-1], leaves[j]) {
			return fail(j, "", ErrLeafOrder, errors.New("events break the canonical (at, id) leaf order"))
		}
	}
	all := uint64(len(ew.Events)) == seal.LeafCount
	if ew.Whole && !all {
		return fail(-1, "", ErrIncompleteWindow, fmt.Errorf("%d of %d events", len(ew.Events), seal.LeafCount))
	}
	wr.Complete, wr.Disclosed = all, true
	for j, ev := range ew.Events {
		er := EventReport{Index: ev.Index, Leaf: leaves[j], Redacted: ev.Event.Redacted(), Erased: ev.Event.Erased()}
		er.Disclosed = er.Redacted == nil && er.Erased == nil
		wr.Disclosed = wr.Disclosed && er.Disclosed
		wr.Events = append(wr.Events, er)
	}
	return nil
}

// tokens splits s into case-folded runs of letters and digits.
func tokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// valueStrings returns the text inside a canonical value (strings and string
// lists; numbers and times hold no text).
func valueStrings(v []byte) []string {
	switch {
	case len(v) == 0:
		return nil
	case v[0] == tagString:
		return []string{string(v[1:])}
	case v[0] == tagStrings:
		ss, _ := decStrings(v)
		return ss
	}
	return nil
}

// containsRun reports whether needle occurs as a contiguous run of hay.
func containsRun(hay, needle []string) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if slices.Equal(hay[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
}

// VerifyErasure verifies exp and then checks that the erased user id appears
// in none of its revealed values. Events name people only by pseudonymous
// [ActorRef], and erased free text is replaced by its commitment
// ([StoredEvent.Erase]), so deleting the identity-map entry leaves every
// event and root verifiable. A revealed value that contains the id is
// [ErrIdentityLeak].
//
// The id matches as a run of whole tokens, case-folded, where tokens are runs
// of letters and digits ("Firebase-UID-Alice" matches "firebase uid alice";
// "al" does not match "approval"). This is a check, not a proof of absence:
// it cannot see the id inside a longer token, in another encoding (base64,
// hashes, percent-encoding), in redacted values, or in events the export
// does not include.
func VerifyErasure(exp *Export, opts VerifyOptions, erasedUserID string) (*Report, error) {
	needle := tokens(erasedUserID)
	if len(needle) == 0 {
		return nil, ErrInvalidArgument
	}
	rep, err := VerifyExport(exp, opts)
	if err != nil {
		return nil, err
	}
	for i, w := range exp.Windows {
		for j, ev := range w.Events {
			for _, f := range ev.Event.Fields {
				for _, s := range valueStrings(f.Value) {
					if containsRun(tokens(s), needle) { // redacted fields carry no value
						return nil, &VerifyError{Window: rep.Windows[i].Window, Event: j, Field: f.Name, Err: ErrIdentityLeak}
					}
				}
			}
		}
	}
	return rep, nil
}
