package apicontract

import "fmt"

// knownColumnTypes is the closed set of type names a recordset column may
// declare - "Type names for parameters/columns map explicitly to existing
// core definitions; unsupported types fail validation rather than being
// converted silently." A column's declared type is always one of these
// content kinds; individual cells carry TypedValue's own "null" state
// independently of the column's declared type.
var knownColumnTypes = map[string]bool{
	string(ValueTypeString):   true,
	string(ValueTypeNumber):   true,
	string(ValueTypeInteger):  true,
	string(ValueTypeDecimal):  true,
	string(ValueTypeBoolean):  true,
	string(ValueTypeDate):     true,
	string(ValueTypeDatetime): true,
}

// Column names one recordset column and its declared type.
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func (c Column) Validate() error {
	if err := requireNonEmpty("name", c.Name); err != nil {
		return err
	}
	if !knownColumnTypes[c.Type] {
		return &ValidationError{Field: "type", Message: fmt.Sprintf("unsupported column type %q", c.Type)}
	}
	return nil
}

// Recordset is Result's tabular payload. "Arrays are ordered; rows have
// exactly one value per returned column." api-contract.md "Shared JSON
// types".
type Recordset struct {
	Columns []Column       `json:"columns"`
	Rows    [][]TypedValue `json:"rows"`
}

// Validate enforces every column is itself valid, every row has exactly as
// many values as there are columns, and every cell value is itself valid.
func (r Recordset) Validate() error {
	for i, c := range r.Columns {
		if err := c.Validate(); err != nil {
			return &ValidationError{Field: "columns", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	for i, row := range r.Rows {
		if len(row) != len(r.Columns) {
			return &ValidationError{Field: "rows", Message: fmt.Sprintf("row %d has %d values, want %d (one per column)", i, len(row), len(r.Columns))}
		}
		for j, cell := range row {
			if err := cell.Validate(); err != nil {
				return &ValidationError{Field: "rows", Message: fmt.Sprintf("row %d column %d: %s", i, j, err)}
			}
		}
	}
	return nil
}

const (
	ProvenanceModeLive     = "live"
	ProvenanceModeSnapshot = "snapshot"
)

const (
	ExecutionProfileProtected        = "protected"
	ExecutionProfileOpaquePrivileged = "opaque-privileged"
)

// Provenance describes where a Result actually came from: "Result.provenance
// .executionProfile reports the actual executor used for that run" - never
// the session's allowed capabilities, and a privileged principal running
// protected DTQL still receives protected provenance. api-contract.md
// "Shared JSON types" / "Security and errors".
type Provenance struct {
	Source           string `json:"source"`
	Collection       string `json:"collection,omitempty"`
	QueryID          string `json:"queryId,omitempty"`
	Mode             string `json:"mode"` // live | snapshot
	SnapshotID       string `json:"snapshotId,omitempty"`
	ObservedAt       string `json:"observedAt"`
	ExecutionProfile string `json:"executionProfile"` // protected | opaque-privileged
}

// Validate enforces Source and ObservedAt are required (ObservedAt must be
// an RFC3339-UTC-normalized instant, the same rule TypedValue's "datetime"
// type enforces), Mode is one of the closed set, snapshot Mode requires a
// SnapshotID ("a snapshot requires explicit separate selection... and a
// configured snapshotId"), and ExecutionProfile is one of the closed set.
func (p Provenance) Validate() error {
	if err := requireNonEmpty("source", p.Source); err != nil {
		return err
	}
	if err := requireOneOf("mode", p.Mode, ProvenanceModeLive, ProvenanceModeSnapshot); err != nil {
		return err
	}
	if p.Mode == ProvenanceModeSnapshot {
		if err := requireNonEmpty("snapshotId", p.SnapshotID); err != nil {
			return err
		}
	}
	if err := NewDatetimeValue(p.ObservedAt).Validate(); err != nil {
		return &ValidationError{Field: "observedAt", Message: err.Error()}
	}
	if err := requireOneOf("executionProfile", p.ExecutionProfile, ExecutionProfileProtected, ExecutionProfileOpaquePrivileged); err != nil {
		return err
	}
	return nil
}

// Result is the exact success envelope of POST exec/run_query and POST
// semantic/related/rows. "Denied requests return no recordset, sample, true
// count, SQL text or hidden value." api-contract.md "Shared JSON types".
type Result struct {
	Recordset       Recordset    `json:"recordset"`
	Limitations     []Limitation `json:"limitations"`
	BindingsApplied []Binding    `json:"bindingsApplied"`
	Provenance      Provenance   `json:"provenance"`
	Truncated       bool         `json:"truncated"`
}

// Validate enforces Recordset, every Limitation, every applied Binding and
// Provenance are each themselves valid.
func (r Result) Validate() error {
	if err := r.Recordset.Validate(); err != nil {
		return err
	}
	for i, l := range r.Limitations {
		if err := l.Validate(); err != nil {
			return &ValidationError{Field: "limitations", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	for i, b := range r.BindingsApplied {
		if err := b.Validate(); err != nil {
			return &ValidationError{Field: "bindingsApplied", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	if err := r.Provenance.Validate(); err != nil {
		return err
	}
	return nil
}
