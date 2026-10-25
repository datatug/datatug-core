package challenge

import (
	"encoding/json"
	"fmt"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

// Oracle kinds for a SQL answer part. Exactly one oracle is attached to each
// SQL case.
const (
	OracleResult     = "result"
	OracleFinalState = "final-state"
)

// Oracle is a closed union. It names either the typed rows a read answer must
// return or the final table state a write answer must leave behind.
type Oracle struct {
	Kind       string
	Result     *ResultOracle
	FinalState *FinalStateOracle
}

// ResultOracle declares the typed recordset a read answer must produce.
type ResultOracle struct {
	Columns            []apicontract.Column       `json:"columns"`
	Rows               [][]apicontract.TypedValue `json:"rows"`
	Ordering           string                     `json:"ordering"`
	RequireColumnNames bool                       `json:"requireColumnNames,omitempty"`
	NumericTolerance   string                     `json:"numericTolerance,omitempty"`
}

// FinalStateOracle declares expected table contents plus named invariants that
// explain a violation. Exact row comparison already covers expected survivors
// and unrelated rows; invariants add the violated retention rule to feedback.
type FinalStateOracle struct {
	Tables     []TableExpectation `json:"tables"`
	Invariants []Invariant        `json:"invariants,omitempty"`
}

// TableExpectation is the exact expected final content of one table.
type TableExpectation struct {
	Name    string                     `json:"name"`
	Columns []apicontract.Column       `json:"columns"`
	Rows    [][]apicontract.TypedValue `json:"rows"`
}

// TableState is an observed final table supplied by the disposable runtime.
type TableState struct {
	Name    string                     `json:"name"`
	Columns []apicontract.Column       `json:"columns"`
	Rows    [][]apicontract.TypedValue `json:"rows"`
}

// Invariant kinds are deliberately small and deterministic. A future rule
// needs a new grading version instead of an open-ended predicate language.
const InvariantGroupMin = "group-min"

// Invariant names a retention rule the final state must satisfy. GroupMin
// requires exactly one surviving row per declared group and requires that
// row's Column to equal the lowest value in the expected table.
type Invariant struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Table       string   `json:"table"`
	GroupBy     []string `json:"groupBy"`
	Column      string   `json:"column"`
	Description string   `json:"description"`
}

func (o *Oracle) UnmarshalJSON(data []byte) error {
	var probe struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	switch probe.Kind {
	case OracleResult:
		var wire struct {
			Kind string `json:"kind"`
			ResultOracle
		}
		if err := apicontract.DecodeStrict(data, &wire); err != nil {
			return err
		}
		*o = Oracle{Kind: OracleResult, Result: &wire.ResultOracle}
	case OracleFinalState:
		var wire struct {
			Kind string `json:"kind"`
			FinalStateOracle
		}
		if err := apicontract.DecodeStrict(data, &wire); err != nil {
			return err
		}
		*o = Oracle{Kind: OracleFinalState, FinalState: &wire.FinalStateOracle}
	default:
		return fmt.Errorf("oracle kind must be %q or %q", OracleResult, OracleFinalState)
	}
	return o.Validate()
}

func (o Oracle) MarshalJSON() ([]byte, error) {
	switch o.Kind {
	case OracleResult:
		if o.Result == nil || o.FinalState != nil {
			return nil, fmt.Errorf("result oracle is missing or ambiguous")
		}
		return json.Marshal(struct {
			Kind string `json:"kind"`
			ResultOracle
		}{Kind: OracleResult, ResultOracle: *o.Result})
	case OracleFinalState:
		if o.FinalState == nil || o.Result != nil {
			return nil, fmt.Errorf("final-state oracle is missing or ambiguous")
		}
		return json.Marshal(struct {
			Kind string `json:"kind"`
			FinalStateOracle
		}{Kind: OracleFinalState, FinalStateOracle: *o.FinalState})
	default:
		return nil, fmt.Errorf("unknown oracle kind %q", o.Kind)
	}
}

// Validate checks the oracle shape and its internal answer semantics.
func (o Oracle) Validate() error {
	switch o.Kind {
	case OracleResult:
		if o.Result == nil || o.FinalState != nil {
			return fmt.Errorf("result oracle must be the only oracle")
		}
		return o.Result.Validate()
	case OracleFinalState:
		if o.FinalState == nil || o.Result != nil {
			return fmt.Errorf("final-state oracle must be the only oracle")
		}
		return o.FinalState.Validate()
	default:
		return fmt.Errorf("unknown oracle kind %q", o.Kind)
	}
}

func (r ResultOracle) Validate() error {
	if diff := validateColumnShape(r.Columns); diff != nil {
		return fmt.Errorf("%s: %s", diff.Problem, diff.Message)
	}
	if err := validateRows(r.Columns, r.Rows); err != nil {
		return err
	}
	if !oneOf(r.Ordering, OrderUnordered, OrderOrdered) {
		return fmt.Errorf("ordering must be %q or %q", OrderUnordered, OrderOrdered)
	}
	if _, err := parseTolerance(r.NumericTolerance); err != nil {
		return err
	}
	return nil
}

func (f FinalStateOracle) Validate() error {
	if len(f.Tables) == 0 || len(f.Tables) > maxManifestTables {
		return fmt.Errorf("final state requires between 1 and %d tables", maxManifestTables)
	}
	tables := map[string]TableExpectation{}
	for _, table := range f.Tables {
		if !validSQLName(table.Name) || tables[table.Name].Name != "" {
			return fmt.Errorf("final-state table names must be valid and unique")
		}
		if err := table.Validate(); err != nil {
			return fmt.Errorf("table %q: %w", table.Name, err)
		}
		tables[table.Name] = table
	}
	if len(f.Invariants) > maxManifestParts {
		return fmt.Errorf("too many final-state invariants")
	}
	seen := map[string]bool{}
	for _, invariant := range f.Invariants {
		if err := invariant.Validate(tables); err != nil {
			return err
		}
		if seen[invariant.ID] {
			return fmt.Errorf("duplicate invariant id %q", invariant.ID)
		}
		seen[invariant.ID] = true
	}
	return nil
}

func (t TableExpectation) Validate() error {
	if !validSQLName(t.Name) {
		return fmt.Errorf("invalid table name")
	}
	if diff := validateColumnShape(t.Columns); diff != nil {
		return fmt.Errorf("%s: %s", diff.Problem, diff.Message)
	}
	return validateRows(t.Columns, t.Rows)
}

func (i Invariant) Validate(tables map[string]TableExpectation) error {
	if !validID(i.ID) {
		return fmt.Errorf("invalid invariant id")
	}
	if i.Kind != InvariantGroupMin {
		return fmt.Errorf("unsupported invariant kind %q", i.Kind)
	}
	if !boundedText(i.Description, 1, 500) {
		return fmt.Errorf("invariant %q requires a bounded description", i.ID)
	}
	table, ok := tables[i.Table]
	if !ok {
		return fmt.Errorf("invariant %q references missing table %q", i.ID, i.Table)
	}
	if len(i.GroupBy) == 0 || len(i.GroupBy) > MaxResultColumns {
		return fmt.Errorf("invariant %q requires group columns", i.ID)
	}
	columnIndex := map[string]int{}
	for index, column := range table.Columns {
		columnIndex[column.Name] = index
	}
	seen := map[string]bool{}
	for _, name := range i.GroupBy {
		index, ok := columnIndex[name]
		if !ok || seen[name] {
			return fmt.Errorf("invariant %q has an unknown or duplicate group column %q", i.ID, name)
		}
		if name == i.Column {
			return fmt.Errorf("invariant %q cannot group by its retention column", i.ID)
		}
		_ = index
		seen[name] = true
	}
	retentionIndex, ok := columnIndex[i.Column]
	if !ok {
		return fmt.Errorf("invariant %q has an unknown retention column %q", i.ID, i.Column)
	}
	if !isNumericType(table.Columns[retentionIndex].Type) {
		return fmt.Errorf("invariant %q requires a numeric retention column", i.ID)
	}

	// The expected table must itself embody the invariant: exactly one survivor
	// per group, and that survivor carries the group's lowest retention value.
	groups := map[string][][]apicontract.TypedValue{}
	for _, row := range table.Rows {
		key, err := groupKey(row, i.GroupBy, columnIndex)
		if err != nil {
			return fmt.Errorf("invariant %q: %w", i.ID, err)
		}
		groups[key] = append(groups[key], row)
	}
	for _, rows := range groups {
		if len(rows) != 1 {
			return fmt.Errorf("invariant %q expects exactly one surviving row per group", i.ID)
		}
		if rows[0][retentionIndex].Type == apicontract.ValueTypeNull {
			return fmt.Errorf("invariant %q requires a non-null retention value", i.ID)
		}
	}
	return nil
}

func validateRows(columns []apicontract.Column, rows [][]apicontract.TypedValue) error {
	if len(rows) > MaxResultRows {
		return fmt.Errorf("row count exceeds %d", MaxResultRows)
	}
	for rowIndex, row := range rows {
		if len(row) != len(columns) {
			return fmt.Errorf("row %d has %d values, expected %d", rowIndex+1, len(row), len(columns))
		}
		for columnIndex, value := range row {
			if err := value.Validate(); err != nil {
				return fmt.Errorf("row %d column %d: %w", rowIndex+1, columnIndex+1, err)
			}
			if value.Type != apicontract.ValueTypeNull && string(value.Type) != columns[columnIndex].Type {
				return fmt.Errorf("row %d column %d has type %q but the column is %q", rowIndex+1, columnIndex+1, value.Type, columns[columnIndex].Type)
			}
		}
	}
	return nil
}

// valueAt reads a value at a declared column position. Rows supplied by the
// disposable runtime are untrusted: a shorter row must report a comparison
// failure instead of panicking on an out-of-range index.
func valueAt(row []apicontract.TypedValue, index int) (apicontract.TypedValue, bool) {
	if index < 0 || index >= len(row) {
		return apicontract.TypedValue{}, false
	}
	return row[index], true
}

func groupKey(row []apicontract.TypedValue, groupBy []string, columnIndex map[string]int) (string, error) {
	key := ""
	for _, name := range groupBy {
		index, ok := columnIndex[name]
		if !ok {
			return "", fmt.Errorf("unknown group column %q", name)
		}
		value, ok := valueAt(row, index)
		if !ok {
			return "", fmt.Errorf("row has %d values so group column %q at position %d is unavailable", len(row), name, index)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", err
		}
		key += fmt.Sprintf("%d:%s", len(encoded), encoded)
	}
	return key, nil
}
