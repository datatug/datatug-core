package meaninglabels

import (
	"time"
)

// Action names a label change.
type Action string

const (
	ActionSet       Action = "set"
	ActionConfirm   Action = "confirm"
	ActionDismiss   Action = "dismiss"
	ActionClear     Action = "clear"
	ActionPropose   Action = "propose"
	ActionClassMark Action = "class"
)

// Actor is who makes a change. An agent may only propose (tighten).
type Actor struct {
	ID        string `json:"id"`
	Agent     bool   `json:"agent,omitempty"`
	AgentUser string `json:"agentUser,omitempty"`
}

// Change is one requested label change. Targets name the exact names the
// source reports; the edge checks loosening targets with ExactTarget first.
// For confirm and dismiss of a read-time suggestion (which is never stored)
// the caller passes the suggestion's Class and Kind.
type Change struct {
	Action        Action `json:"action"`
	Target        Target `json:"target"`
	Class         Class  `json:"class,omitempty"`
	Kind          Kind   `json:"kind,omitempty"`
	NonRevealable bool   `json:"nonRevealable,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

// Event is the audit record of one applied change.
type Event struct {
	Type   string    `json:"type"`
	Actor  Actor     `json:"actor"`
	Target Target    `json:"target"`
	Before *Label    `json:"before,omitempty"`
	After  *Label    `json:"after,omitempty"`
	Reason string    `json:"reason,omitempty"`
	At     time.Time `json:"at"`
	// Loosens marks a change that removes or weakens protection.
	Loosens bool `json:"loosens,omitempty"`
}

// Apply applies one change to a copy of doc and returns the new document with
// a fresh revision and the audit event. doc is not modified.
func Apply(doc Document, change Change, actor Actor, at time.Time) (Document, Event, error) {
	at = at.UTC()
	if actor.Agent && change.Action != ActionPropose {
		return doc, Event{}, errorf(CodeAgentNotAllowed, "an agent may only propose labels; %q needs the owner", change.Action)
	}
	if change.Action != ActionPropose && change.Action != ActionConfirm && change.Reason == "" {
		return doc, Event{}, errorf(CodeReasonRequired, "a label change needs a reason")
	}
	out := doc.clone()
	out.Format = Format
	event := Event{Actor: actor, Target: change.Target, Reason: change.Reason, At: at}
	if change.Action == ActionClassMark {
		return applyClassPolicy(out, change, event)
	}
	if err := change.Target.Validate(); err != nil {
		return doc, Event{}, err
	}
	var err error
	switch change.Action {
	case ActionSet:
		err = out.set(change, actor, at, &event)
	case ActionConfirm:
		err = out.confirm(change, actor, at, &event)
	case ActionDismiss:
		err = out.dismiss(change, actor, at, &event)
	case ActionClear:
		err = out.clear(change, &event)
	case ActionPropose:
		err = out.propose(change, actor, at, &event)
	default:
		err = errorf(CodeUnknownAction, "unknown action %q", change.Action)
	}
	if err != nil {
		return doc, Event{}, err
	}
	out.Revision = out.ComputeRevision()
	return out, event, nil
}

func (d *Document) remove(i int) {
	d.Labels = append(d.Labels[:i], d.Labels[i+1:]...)
}

func (d *Document) set(change Change, actor Actor, at time.Time, event *Event) error {
	if err := ValidateClassKind(change.Class, change.Kind); err != nil {
		return err
	}
	owner := d.labelIndex(change.Target, OriginOwner)
	if owner >= 0 {
		b := d.Labels[owner]
		event.Before = &b
	}
	if change.Class == ClassSecret && !change.NonRevealable {
		if event.Before != nil && event.Before.Class == ClassSecret {
			return errorf(CodeSecretNonRevealable, "secret is always non-revealable")
		}
		change.NonRevealable = true
	}
	if change.Class == ClassNone && change.NonRevealable {
		return errorf(CodeInvalidDocument, "a none label cannot be non-revealable")
	}
	l := Label{Target: change.Target, Class: change.Class, Kind: change.Kind, NonRevealable: change.NonRevealable,
		State: StateConfirmed, Origin: OriginOwner, By: actor.ID, At: at, Reason: change.Reason}
	d.put(owner, l)
	// The owner decided again: a pending proposal on the target is settled.
	if p := d.labelIndex(change.Target, OriginProposed); p >= 0 {
		d.remove(p)
	}
	event.Type = "meaning.label.set"
	event.After = &l
	// An owner column label decides over suggestions, so any column label
	// below the strictest class may hide one; treat it as loosening.
	event.Loosens = loosens(event.Before, &l) || (change.Target.Kind() == TargetColumn && change.Class != ClassSecret)
	return nil
}

func (d *Document) confirm(change Change, actor Actor, at time.Time, event *Event) error {
	var l Label
	if p := d.labelIndex(change.Target, OriginProposed); p >= 0 {
		l = d.Labels[p]
		d.remove(p)
	} else if change.Class.Protected() {
		if err := ValidateClassKind(change.Class, change.Kind); err != nil {
			return err
		}
		l = Label{Target: change.Target, Class: change.Class, Kind: change.Kind, NonRevealable: change.Class == ClassSecret}
	} else {
		return errorf(CodeSuggestionNotPending, "nothing pending to confirm on %s", change.Target)
	}
	owner := d.labelIndex(change.Target, OriginOwner)
	if owner >= 0 {
		b := d.Labels[owner]
		event.Before = &b
	}
	l.State, l.Origin, l.By, l.At = StateConfirmed, OriginOwner, actor.ID, at
	if change.Reason != "" {
		l.Reason = change.Reason
	}
	d.put(owner, l)
	event.Type = "meaning.label.confirmed"
	event.After = &l
	return nil
}

func (d *Document) dismiss(change Change, actor Actor, at time.Time, event *Event) error {
	var x Dismissal
	if p := d.labelIndex(change.Target, OriginProposed); p >= 0 {
		b := d.Labels[p]
		event.Before = &b
		x = Dismissal{Target: b.Target, Class: b.Class, Kind: b.Kind}
		d.remove(p)
	} else if change.Class.Protected() && change.Target.Kind() == TargetColumn {
		if err := ValidateClassKind(change.Class, change.Kind); err != nil {
			return err
		}
		if d.dismissed(change.Target, change.Class) {
			return errorf(CodeSuggestionNotPending, "the %s suggestion on %s is already dismissed", change.Class, change.Target)
		}
		x = Dismissal{Target: change.Target, Class: change.Class, Kind: change.Kind}
	} else {
		return errorf(CodeSuggestionNotPending, "nothing pending to dismiss on %s", change.Target)
	}
	x.By, x.At, x.Reason = actor.ID, at, change.Reason
	d.Dismissed = append(d.Dismissed, x)
	event.Type = "meaning.label.dismissed"
	event.Loosens = true
	return nil
}

func (d *Document) clear(change Change, event *Event) error {
	if i := d.labelIndex(change.Target, OriginOwner); i >= 0 {
		b := d.Labels[i]
		event.Before = &b
		d.remove(i)
		event.Type = "meaning.label.cleared"
		event.Loosens = b.Class.Protected()
		return nil
	}
	kept := []Dismissal{}
	for _, x := range d.Dismissed {
		if x.Target != change.Target {
			kept = append(kept, x)
		}
	}
	if len(kept) == len(d.Dismissed) {
		return errorf(CodeLabelNotFound, "no owner label or dismissal on %s", change.Target)
	}
	d.Dismissed = kept
	event.Type = "meaning.label.dismissal_cleared"
	return nil
}

func (d *Document) propose(change Change, actor Actor, at time.Time, event *Event) error {
	if err := ValidateClassKind(change.Class, change.Kind); err != nil {
		return err
	}
	if !change.Class.Protected() {
		return errorf(CodeClassUnknown, "a proposal must name a protected class")
	}
	event.Type = "meaning.label.proposed"
	// A proposal only ever tightens: one no stricter than the stored owner
	// label or proposal on the target is recorded as ignored.
	p := d.labelIndex(change.Target, OriginProposed)
	for _, i := range []int{d.labelIndex(change.Target, OriginOwner), p} {
		if i >= 0 && d.Labels[i].Class.Rank() >= change.Class.Rank() {
			b := d.Labels[i]
			event.Type = "meaning.label.proposal_ignored"
			event.Before, event.After = &b, &b
			return nil
		}
	}
	if p >= 0 {
		b := d.Labels[p]
		event.Before = &b
	}
	l := Label{Target: change.Target, Class: change.Class, Kind: change.Kind, NonRevealable: change.Class == ClassSecret || change.NonRevealable,
		State: StatePending, Origin: OriginProposed, By: actor.ID, At: at, Reason: change.Reason}
	d.put(p, l)
	event.After = &l
	return nil
}

func applyClassPolicy(out Document, change Change, event Event) (Document, Event, error) {
	if change.Target.Source == "" || change.Target.Table != "" || change.Target.Column != "" || !change.Class.Protected() {
		return Document{}, Event{}, errorf(CodeInvalidDocument, "a class policy names only a source and a protected class")
	}
	if change.Class == ClassSecret && !change.NonRevealable {
		return Document{}, Event{}, errorf(CodeSecretNonRevealable, "secret is always non-revealable")
	}
	event.Type = "meaning.class_policy.changed"
	kept := []ClassPolicy{}
	for _, p := range out.ClassPolicies {
		if p.Class == change.Class && p.Source == change.Target.Source {
			event.Loosens = !change.NonRevealable
			continue
		}
		kept = append(kept, p)
	}
	out.ClassPolicies = kept
	if change.NonRevealable {
		out.ClassPolicies = append(out.ClassPolicies, ClassPolicy{Source: change.Target.Source, Class: change.Class, NonRevealable: true})
	}
	out.Revision = out.ComputeRevision()
	return out, event, nil
}

func (d *Document) put(i int, l Label) {
	if i >= 0 {
		d.Labels[i] = l
		return
	}
	d.Labels = append(d.Labels, l)
}

func loosens(before, after *Label) bool {
	if before == nil {
		return false
	}
	return after.Class.Rank() < before.Class.Rank() || (before.NonRevealable && !after.NonRevealable)
}

// ExactTarget checks that a loosening write names exactly one table the source
// reports (schema-qualified, case-exact), and for a column target one of its
// columns exactly. tables maps reported qualified table names to column names.
func ExactTarget(t Target, tables map[string][]string) error {
	columns, ok := tables[t.Table]
	if !ok {
		return errorf(CodeTargetAmbiguous, "%s does not name exactly one table the source reports; use the schema-qualified, case-exact name", t)
	}
	if t.Column == "" {
		return nil
	}
	for _, c := range columns {
		if c == t.Column {
			return nil
		}
	}
	return errorf(CodeTargetAmbiguous, "%s does not name a column the source reports exactly", t)
}

// Loosening reports whether a change can remove or weaken protection, so the
// edge must check its target with ExactTarget and require a step-up under a
// control plane.
func Loosening(c Change) bool {
	switch c.Action {
	case ActionDismiss, ActionClear:
		return true
	case ActionSet:
		return c.Target.Kind() == TargetColumn || c.Class == ClassNone
	case ActionClassMark:
		return !c.NonRevealable
	}
	return false
}
