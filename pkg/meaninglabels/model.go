// Package meaninglabels is the Meaning Graph's sensitivity vocabulary: labels
// that say how sensitive a table, column or entity field is. A label's class is
// the field's Perimeter protection class (backstage spec
// features/meaning-sensitivity). The package is engine-free: it imports no
// DALgo driver, and its store works over any dal.DB the edge composes.
package meaninglabels

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Format is the label document's format identifier.
const Format = "datatug-labels/v1"

// Class is a sensitivity class; it is also the Perimeter protection class.
type Class string

const (
	// ClassSecret is security-critical data: it grants access or proves identity.
	ClassSecret Class = "secret"
	// ClassFinancial is protected financial data such as card numbers.
	ClassFinancial Class = "financial"
	// ClassPII is personal or private data about a person.
	ClassPII Class = "pii"
	// ClassNone records an owner's "reviewed and not sensitive" decision.
	ClassNone Class = "none"
)

// Rank orders classes from least to most strict; unknown classes rank -1.
func (c Class) Rank() int {
	switch c {
	case ClassNone:
		return 0
	case ClassPII:
		return 1
	case ClassFinancial:
		return 2
	case ClassSecret:
		return 3
	}
	return -1
}

// Protected reports whether the class masks a field.
func (c Class) Protected() bool {
	return c.Rank() > 0
}

// Kind says what a labelled value is; it never changes enforcement.
type Kind string

// Kinds lists the published kinds of each protected class.
var Kinds = map[Class][]Kind{
	ClassSecret:    {"password", "password-hash", "api-key", "token", "private-key", "session-id", "otp-seed"},
	ClassFinancial: {"card-number", "card-cvv", "bank-account", "iban", "routing-number"},
	ClassPII:       {"person-name", "email", "phone", "postal-address", "birth-date", "national-id", "passport-number", "ip-address", "geo-location"},
}

// Error codes returned on the wire.
const (
	CodeClassUnknown         = "LABEL_CLASS_UNKNOWN"
	CodeKindClassMismatch    = "LABEL_KIND_CLASS_MISMATCH"
	CodeSecretNonRevealable  = "LABEL_SECRET_ALWAYS_NON_REVEALABLE"
	CodeAgentNotAllowed      = "LABEL_CHANGE_NOT_ALLOWED_FOR_AGENT"
	CodeRevisionConflict     = "LABELS_REVISION_CONFLICT"
	CodeInvalidTarget        = "LABEL_TARGET_INVALID"
	CodeInvalidDocument      = "LABELS_DOCUMENT_INVALID"
	CodeLabelNotFound        = "LABEL_NOT_FOUND"
	CodeReasonRequired       = "LABEL_REASON_REQUIRED"
	CodeFieldProtected       = "FIELD_PROTECTED"
	CodeRawRefused           = "FIELD_PROTECTION_RAW_REFUSED"
	CodeRevealNotForAgent    = "REVEAL_NOT_ALLOWED_FOR_AGENT"
	CodeFieldNonRevealable   = "FIELD_NON_REVEALABLE"
	CodeUnknownAction        = "LABEL_ACTION_UNKNOWN"
	CodeSuggestionNotPending = "LABEL_NOT_PENDING"
	CodeTargetAmbiguous      = "LABEL_TARGET_AMBIGUOUS"
)

// Error is a coded label error.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Message
}

func errorf(code, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// ErrorCode returns the code of a label error, or "" for any other error.
func ErrorCode(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// ValidateClassKind checks that class is known and kind, if set, belongs to it.
func ValidateClassKind(class Class, kind Kind) error {
	if class.Rank() < 0 {
		return errorf(CodeClassUnknown, "unknown class %q: expected secret, financial, pii or none", class)
	}
	if kind == "" {
		return nil
	}
	for _, k := range Kinds[class] {
		if k == kind {
			return nil
		}
	}
	return errorf(CodeKindClassMismatch, "kind %q does not belong to class %q", kind, class)
}

// Target is what a label attaches to: a column {source, table, column} or a
// table {source, table}.
type Target struct {
	Source string `json:"source"`
	Table  string `json:"table"`
	Column string `json:"column,omitempty"`
}

// TargetKind names a target's shape.
type TargetKind string

const (
	TargetColumn TargetKind = "column"
	TargetTable  TargetKind = "table"
)

// Kind returns the target's shape, or "" when it is not a valid shape.
func (t Target) Kind() TargetKind {
	switch {
	case t.Source == "" || t.Table == "":
		return ""
	case t.Column != "":
		return TargetColumn
	}
	return TargetTable
}

// Validate checks the target's shape and bounds.
func (t Target) Validate() error {
	if t.Kind() == "" {
		return errorf(CodeInvalidTarget, "a target is {source, table, column} or {source, table}")
	}
	for _, v := range []string{t.Source, t.Table, t.Column} {
		if len(v) > 256 || strings.TrimSpace(v) != v || strings.ContainsAny(v, "\x00\n\r") {
			return errorf(CodeInvalidTarget, "target part %q is not a plain name of at most 256 characters", v)
		}
	}
	return nil
}

// String renders a target for messages.
func (t Target) String() string {
	if t.Column != "" {
		return t.Source + "/" + t.Table + "." + t.Column
	}
	return t.Source + "/" + t.Table
}

// State says whether an owner has decided a label.
type State string

const (
	StateConfirmed State = "confirmed"
	StatePending   State = "pending"
)

// Origin says who recorded a stored label.
type Origin string

const (
	OriginOwner    Origin = "owner"
	OriginProposed Origin = "proposed"
)

// Label is one stored label.
type Label struct {
	Target        Target    `json:"target"`
	Class         Class     `json:"class"`
	Kind          Kind      `json:"kind,omitempty"`
	NonRevealable bool      `json:"nonRevealable,omitempty"`
	State         State     `json:"state"`
	Origin        Origin    `json:"origin"`
	By            string    `json:"by,omitempty"`
	At            time.Time `json:"at"`
	Reason        string    `json:"reason,omitempty"`
}

// Validate checks one stored label.
func (l Label) Validate() error {
	if err := l.Target.Validate(); err != nil {
		return err
	}
	if err := ValidateClassKind(l.Class, l.Kind); err != nil {
		return err
	}
	if l.Class == ClassSecret && !l.NonRevealable {
		return errorf(CodeSecretNonRevealable, "secret label on %s must be non-revealable", l.Target)
	}
	if l.Class == ClassNone && (l.NonRevealable || l.Origin != OriginOwner) {
		return errorf(CodeInvalidDocument, "a none label is an owner decision and cannot be non-revealable")
	}
	switch {
	case l.Origin == OriginOwner && l.State == StateConfirmed:
	case l.Origin == OriginProposed && l.State == StatePending:
	default:
		return errorf(CodeInvalidDocument, "label on %s has origin %q and state %q", l.Target, l.Origin, l.State)
	}
	return nil
}

// Dismissal records an owner's dismissal of a suggested or proposed label, so
// the same suggestion is not made again.
type Dismissal struct {
	Target Target    `json:"target"`
	Class  Class     `json:"class"`
	Kind   Kind      `json:"kind,omitempty"`
	By     string    `json:"by,omitempty"`
	At     time.Time `json:"at"`
	Reason string    `json:"reason,omitempty"`
}

// ClassPolicy marks a whole class non-revealable on one source.
type ClassPolicy struct {
	Source        string `json:"source"`
	Class         Class  `json:"class"`
	NonRevealable bool   `json:"nonRevealable"`
}

// Document is a project's label document.
type Document struct {
	Format        string        `json:"format"`
	Revision      string        `json:"revision,omitempty"`
	Labels        []Label       `json:"labels"`
	Dismissed     []Dismissal   `json:"dismissed,omitempty"`
	ClassPolicies []ClassPolicy `json:"classPolicies,omitempty"`
}

// NewDocument returns an empty document.
func NewDocument() Document {
	d := Document{Format: Format, Labels: []Label{}}
	d.Revision = d.ComputeRevision()
	return d
}

// Validate checks the whole document, including that each target has at most
// one label and the revision matches the content.
func (d Document) Validate() error {
	if d.Format != Format {
		return errorf(CodeInvalidDocument, "format must be %s", Format)
	}
	for i, l := range d.Labels {
		if err := l.Validate(); err != nil {
			return fmt.Errorf("labels[%d]: %w", i, err)
		}
		for _, other := range d.Labels[:i] {
			if other.Target == l.Target && other.Origin == l.Origin {
				return errorf(CodeInvalidDocument, "two %s labels on %s", l.Origin, l.Target)
			}
		}
	}
	for i, x := range d.Dismissed {
		if err := x.Target.Validate(); err != nil {
			return fmt.Errorf("dismissed[%d]: %w", i, err)
		}
		if err := ValidateClassKind(x.Class, x.Kind); err != nil {
			return fmt.Errorf("dismissed[%d]: %w", i, err)
		}
	}
	for i, p := range d.ClassPolicies {
		if p.Source == "" || !p.Class.Protected() {
			return errorf(CodeInvalidDocument, "classPolicies[%d] needs a source and a protected class", i)
		}
		if p.Class == ClassSecret && !p.NonRevealable {
			return errorf(CodeSecretNonRevealable, "classPolicies[%d]: secret is always non-revealable", i)
		}
	}
	if d.Revision != d.ComputeRevision() {
		return errorf(CodeInvalidDocument, "revision does not match the content")
	}
	return nil
}

// ComputeRevision is the sha256 of the document's content without its revision.
func (d Document) ComputeRevision() string {
	d.Revision = ""
	if d.Labels == nil {
		d.Labels = []Label{}
	}
	body, _ := json.Marshal(d) // only plain values; cannot fail
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (d Document) clone() Document {
	c := d
	c.Labels = append([]Label{}, d.Labels...)
	c.Dismissed = append([]Dismissal(nil), d.Dismissed...)
	c.ClassPolicies = append([]ClassPolicy(nil), d.ClassPolicies...)
	return c
}

func (d Document) labelIndex(t Target, origin Origin) int {
	for i, l := range d.Labels {
		if l.Target == t && l.Origin == origin {
			return i
		}
	}
	return -1
}

// classNonRevealable reports whether an owner marked class non-revealable on source.
func (d Document) classNonRevealable(source string, class Class) bool {
	for _, p := range d.ClassPolicies {
		if strings.EqualFold(p.Source, source) && p.Class == class && p.NonRevealable {
			return true
		}
	}
	return false
}
