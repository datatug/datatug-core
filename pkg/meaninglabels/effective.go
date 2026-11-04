package meaninglabels

import (
	"strings"
)

// EffectiveOrigin says what produced an effective label.
type EffectiveOrigin string

const (
	FromColumn    EffectiveOrigin = "column"
	FromTable     EffectiveOrigin = "table"
	FromSuggested EffectiveOrigin = "suggested"
	FromProposed  EffectiveOrigin = "proposed"
	// FromUnreviewedObject masks every column of a view or other non-table
	// object no owner has labelled, for agents.
	FromUnreviewedObject EffectiveOrigin = "unreviewed-object"
)

// ColumnLabel is a column's effective label. Class is "" when no label applies.
type ColumnLabel struct {
	Column        string          `json:"column"`
	Class         Class           `json:"class,omitempty"`
	Kind          Kind            `json:"kind,omitempty"`
	Origin        EffectiveOrigin `json:"origin,omitempty"`
	State         State           `json:"state,omitempty"`
	NonRevealable bool            `json:"nonRevealable,omitempty"`
	Rule          string          `json:"rule,omitempty"`
}

// Protected reports whether the column is masked by default.
func (c ColumnLabel) Protected() bool {
	return c.Class.Protected()
}

// Revealable reports whether an elevated approval may reveal the column.
func (c ColumnLabel) Revealable() bool {
	return c.Protected() && !c.NonRevealable
}

// TableMatches reports whether a label's table names the table being read.
// Names compare ignoring case; a label table without a schema matches the
// table's last segment, so "customers" also covers "public.customers". Both
// rules only ever widen what is masked.
func TableMatches(labelTable, table string) bool {
	if strings.EqualFold(labelTable, table) {
		return true
	}
	if strings.Contains(labelTable, ".") {
		return false
	}
	if i := strings.LastIndex(table, "."); i >= 0 {
		return strings.EqualFold(labelTable, table[i+1:])
	}
	return false
}

func (t Target) matches(source, table string) bool {
	return strings.EqualFold(t.Source, source) && TableMatches(t.Table, table)
}

// Effective computes the effective label of each column of one table. An owner
// label on the column decides alone (none included). Otherwise the result is
// the stricter of the table's owner label (protected classes only), pending
// proposals on the column or table, and the read-time suggestion unless an
// owner dismissed it on that column. Class policies of the source apply last.
func Effective(doc Document, source, table string, columns []Column) []ColumnLabel {
	out := make([]ColumnLabel, len(columns))
	for i, column := range columns {
		out[i] = effectiveColumn(doc, source, table, column)
		if out[i].Protected() && doc.classNonRevealable(source, out[i].Class) {
			out[i].NonRevealable = true
		}
	}
	return out
}

func effectiveColumn(doc Document, source, table string, column Column) ColumnLabel {
	exact := Target{Source: source, Table: table, Column: column.Name}
	var decided *ColumnLabel
	var best *ColumnLabel
	consider := func(c ColumnLabel) {
		if !c.Protected() {
			return
		}
		if best == nil {
			best = &c
			return
		}
		nonRevealable := best.NonRevealable || c.NonRevealable
		if c.Class.Rank() > best.Class.Rank() {
			*best = c
		}
		best.NonRevealable = nonRevealable
	}
	for _, l := range doc.Labels {
		if !l.Target.matches(source, table) {
			continue
		}
		onColumn := l.Target.Kind() == TargetColumn
		if onColumn && !strings.EqualFold(l.Target.Column, column.Name) {
			continue
		}
		c := ColumnLabel{Column: column.Name, Class: l.Class, Kind: l.Kind, State: l.State, NonRevealable: l.NonRevealable}
		switch {
		case onColumn && l.Origin == OriginOwner && l.Target == exact:
			// Only an exact match decides alone: it can loosen.
			c.Origin = FromColumn
			decided = &c
			continue
		case l.Origin == OriginOwner:
			c.Origin = FromTable
		default:
			c.Origin = FromProposed
		}
		consider(c)
	}
	if decided != nil {
		// The owner decided the column; only a stricter pending proposal on
		// the column itself still applies.
		result := *decided
		for _, l := range doc.Labels {
			if l.Origin == OriginProposed && l.Target.Kind() == TargetColumn && l.Target.matches(source, table) &&
				strings.EqualFold(l.Target.Column, column.Name) && l.Class.Rank() > result.Class.Rank() {
				result = ColumnLabel{Column: column.Name, Class: l.Class, Kind: l.Kind, Origin: FromProposed, State: StatePending,
					NonRevealable: l.NonRevealable || decided.NonRevealable}
			}
		}
		return result
	}
	if r, ok := SuggestColumn(column.Name, column.DbType); ok && !doc.dismissed(exact, r.Class) {
		consider(ColumnLabel{Column: column.Name, Class: r.Class, Kind: r.Kind, Origin: FromSuggested, State: StatePending,
			NonRevealable: r.Class == ClassSecret, Rule: r.ID})
	}
	if best == nil {
		return ColumnLabel{Column: column.Name}
	}
	return *best
}

// dismissed matches dismissals exactly: a dismissal loosens, so it never
// widens to another schema or another spelling.
func (d Document) dismissed(t Target, class Class) bool {
	for _, x := range d.Dismissed {
		if x.Class == class && x.Target == t {
			return true
		}
	}
	return false
}

// Reviewed reports whether an owner labelled the object (table-level, none
// included) or any of its columns. An unreviewed view or other non-table
// object is masked entirely for agents.
func Reviewed(doc Document, source, table string) bool {
	for _, l := range doc.Labels {
		if l.Origin == OriginOwner && l.Target.Source == source && l.Target.Table == table {
			return true
		}
	}
	return false
}

// MaskAllUnreviewed marks every column of an unreviewed non-table object as
// protected pii, revealable, so an agent sees no value until an owner reviews it.
func MaskAllUnreviewed(labels []ColumnLabel) []ColumnLabel {
	out := append([]ColumnLabel{}, labels...)
	for i := range out {
		if !out[i].Protected() {
			out[i] = ColumnLabel{Column: out[i].Column, Class: ClassPII, Origin: FromUnreviewedObject, State: StatePending}
		}
	}
	return out
}

// Tighten merges a second set of effective labels for the same columns into
// authoritative ones, only ever making them stricter: the control plane's
// labels are authoritative and a project file can add, never remove.
func Tighten(authoritative, extra []ColumnLabel) []ColumnLabel {
	out := append([]ColumnLabel{}, authoritative...)
	for i := range out {
		for _, e := range extra {
			if !strings.EqualFold(e.Column, out[i].Column) || !e.Protected() {
				continue
			}
			if e.Class.Rank() > out[i].Class.Rank() {
				nonRevealable := out[i].NonRevealable
				out[i] = e
				out[i].NonRevealable = out[i].NonRevealable || nonRevealable
			} else if e.NonRevealable {
				out[i].NonRevealable = true
			}
		}
	}
	return out
}
