package meaninglabels

import (
	"sort"
	"strings"
)

// MaskedValue is how text and table output render a masked cell.
const MaskedValue = "••••"

// Protection is the wire description of a masked column.
type Protection struct {
	Class      Class  `json:"class"`
	Kind       Kind   `json:"kind,omitempty"`
	State      string `json:"state"`
	Revealable bool   `json:"revealable"`
	Origin     string `json:"origin,omitempty"`
	// Ask tells an agent how to get the value.
	Ask string `json:"ask,omitempty"`
}

// Plan says which columns of one table the boundary must not fetch.
type Plan struct {
	Masked map[string]ColumnLabel
}

// AskText is the hint attached to a masked column.
const AskText = "Masked by a Meaning Graph sensitivity label. Ask your user for an elevated approval that names this field; do not retry the read."

// NewPlan returns the masking plan of one table: every protected column is
// masked unless revealed names it and it is revealable. Revealed holds column
// names (any case); non-revealable columns stay masked whatever it says.
func NewPlan(labels []ColumnLabel, revealed []string) Plan {
	plan := Plan{Masked: map[string]ColumnLabel{}}
	for _, l := range labels {
		if !l.Protected() {
			continue
		}
		if l.Revealable() && containsFold(revealed, l.Column) {
			continue
		}
		plan.Masked[strings.ToLower(l.Column)] = l
	}
	return plan
}

// Empty reports whether nothing is masked.
func (p Plan) Empty() bool {
	return len(p.Masked) == 0
}

// IsMasked reports whether a column is masked.
func (p Plan) IsMasked(column string) (ColumnLabel, bool) {
	l, ok := p.Masked[strings.ToLower(column)]
	return l, ok
}

// Protection returns the wire protection of a masked column.
func (p Plan) Protection(column string) *Protection {
	l, ok := p.IsMasked(column)
	if !ok {
		return nil
	}
	return &Protection{Class: l.Class, Kind: l.Kind, State: "masked", Revealable: l.Revealable(), Origin: string(l.Origin), Ask: AskText}
}

// Columns lists masked column names, sorted.
func (p Plan) Columns() []string {
	out := make([]string, 0, len(p.Masked))
	for _, l := range p.Masked {
		out = append(out, l.Column)
	}
	sort.Strings(out)
	return out
}

// CheckReferenced refuses a query whose filter, sort, grouping, alias or
// expression names a masked column: a predicate on a masked value discloses it.
func (p Plan) CheckReferenced(table string, referenced []string) error {
	for _, name := range referenced {
		if l, ok := p.IsMasked(name); ok {
			return errorf(CodeFieldProtected, "%s.%s is protected (%s); a query may not filter, sort, group, alias or compute on it", table, l.Column, l.Class)
		}
	}
	return nil
}

// CheckReveal refuses a reveal request for an agent, or one that names a
// non-revealable column.
func CheckReveal(labels []ColumnLabel, revealed []string, agent bool) error {
	if len(revealed) == 0 {
		return nil
	}
	if agent {
		return errorf(CodeRevealNotForAgent, "an agent reaches a protected field only through an elevated approval")
	}
	for _, l := range labels {
		if l.Protected() && !l.Revealable() && containsFold(revealed, l.Column) {
			return errorf(CodeFieldNonRevealable, "%s is non-revealable (%s)", l.Column, l.Class)
		}
	}
	return nil
}

// AnyProtected reports whether any column of the source has a protected
// effective label (owner label, proposal or read-time suggestion), or any
// protected label is stored for the source. An agent's raw read of such a
// source is refused. tables maps reported table names to their columns.
func AnyProtected(doc Document, source string, tables map[string][]Column) bool {
	for _, l := range doc.Labels {
		if l.Class.Protected() && strings.EqualFold(l.Target.Source, source) {
			return true
		}
	}
	for table, columns := range tables {
		for _, c := range Effective(doc, source, table, columns) {
			if c.Protected() {
				return true
			}
		}
	}
	return false
}

// RawRefusal returns FIELD_PROTECTION_RAW_REFUSED for an agent on a source
// with protected columns, and nil otherwise.
func RawRefusal(doc Document, source string, tables map[string][]Column, agent bool) error {
	if agent && AnyProtected(doc, source, tables) {
		return errorf(CodeRawRefused, "source %s has protected columns; agents read it with table reads or TugQL, which mask", source)
	}
	return nil
}

func containsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}
