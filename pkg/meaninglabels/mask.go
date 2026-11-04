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

// AskText is the hint attached to a masked revealable column.
const AskText = "Masked by a Meaning Graph sensitivity label. Ask your user for an elevated approval that names this field; do not retry the read."

// NeverText is the hint attached to a masked non-revealable column.
const NeverText = "Masked by a Meaning Graph sensitivity label and never revealed: no approval opens a non-revealable field. Work without it; do not retry the read."

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
		plan.Masked[l.Column] = l
	}
	return plan
}

// Empty reports whether nothing is masked.
func (p Plan) Empty() bool {
	return len(p.Masked) == 0
}

// IsMasked reports whether a column is masked. The reference is compared
// ignoring case, under every spelling it could stand for (see Candidates), so
// the gate fails closed on qualification, quoting and escapes. One entry is
// one identifier reference, not an expression.
func (p Plan) IsMasked(reference string) (ColumnLabel, bool) {
	for _, name := range Candidates(reference) {
		if l, ok := p.Masked[name]; ok {
			return l, true
		}
		for masked, l := range p.Masked {
			if strings.EqualFold(masked, name) {
				return l, true
			}
		}
	}
	return ColumnLabel{}, false
}

// BareName is the most likely column name a reference stands for: the last
// segment, unquoted and unescaped ("a""b" is a"b, [e.mail] is e.mail).
func BareName(reference string) string {
	return Candidates(reference)[0]
}

// Candidates lists every column name a reference could stand for, most likely
// first: the unescaped quoted last segment, the last dot segment with quotes
// trimmed, and the raw last segment.
func Candidates(reference string) []string {
	reference = strings.TrimSpace(reference)
	var out []string
	add := func(v string) {
		for _, x := range out {
			if x == v {
				return
			}
		}
		out = append(out, v)
	}
	if n := len(reference); n >= 2 {
		closing := reference[n-1]
		if opening, ok := map[byte]byte{'"': '"', '`': '`', ']': '[', '\'': '\''}[closing]; ok {
			if i := quoteStart(reference[:n-1], opening, closing); i >= 0 {
				inner := reference[i+1 : n-1]
				add(strings.ReplaceAll(inner, string([]byte{closing, closing}), string(closing)))
				add(inner)
			}
		}
	}
	trimmed := strings.TrimRight(reference, ".")
	last := trimmed
	if i := strings.LastIndex(trimmed, "."); i >= 0 {
		last = trimmed[i+1:]
	}
	add(strings.Trim(last, "\"`[]' "))
	add(last)
	return out
}

// quoteStart finds the opening quote of a quoted segment that ends just before
// body's end, skipping doubled closing quotes inside it; -1 if there is none.
func quoteStart(body string, opening, closing byte) int {
	for i := len(body) - 1; i >= 0; i-- {
		if body[i] != opening {
			continue
		}
		if opening == closing && i > 0 && body[i-1] == closing {
			i-- // a doubled quote is an escape inside the segment
			continue
		}
		return i
	}
	return -1
}

// Protection returns the wire protection of a masked column.
func (p Plan) Protection(column string) *Protection {
	l, ok := p.IsMasked(column)
	if !ok {
		return nil
	}
	ask := AskText
	if !l.Revealable() {
		ask = NeverText
	}
	return &Protection{Class: l.Class, Kind: l.Kind, State: "masked", Revealable: l.Revealable(), Origin: string(l.Origin), Ask: ask}
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
// An empty tables map refuses too: a boundary that could not enumerate the
// source's columns cannot show that the source is unlabelled.
func RawRefusal(doc Document, source string, tables map[string][]Column, agent bool) error {
	if agent && (len(tables) == 0 || anyEmpty(tables) || AnyProtected(doc, source, tables)) {
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

// anyEmpty reports a table whose columns the boundary could not enumerate.
func anyEmpty(tables map[string][]Column) bool {
	for _, columns := range tables {
		if len(columns) == 0 {
			return true
		}
	}
	return false
}
