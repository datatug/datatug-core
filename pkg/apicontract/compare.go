package apicontract

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/datatug/datatug-core/pkg/incidents"
)

type CompareSideKind string

const (
	CompareSideScope  CompareSideKind = "scope"
	CompareSideFacts  CompareSideKind = "facts"
	CompareSideRecord CompareSideKind = "record"

	CompareCohortAffected = "affected"
	// CompareCohortControl is deliberately distinct from incidents.FactRoleHealthyControl.
	// The service maps this public spelling to the internal incident role.
	CompareCohortControl = "control"

	CompareColumnLeft  = "left"
	CompareColumnRight = "right"

	CompareDefaultLimit              = 100
	CompareMaximumLimit              = 500
	CompareDistributionMaximumValues = 50
)

// CompareSideSpec is a strict, flattened discriminated union. Each kind accepts
// only its own fields: scope parameters, an incident-bound cohort, or a record.
type CompareSideSpec struct {
	Kind        CompareSideKind       `json:"kind"`
	StoreID     string                `json:"storeId,omitempty"`
	Project     string                `json:"project,omitempty"`
	Environment string                `json:"environment,omitempty"`
	Parameters  map[string]TypedValue `json:"parameters,omitempty"`
	CohortRole  string                `json:"cohortRole,omitempty"`
	Execution   *ExecutionRef         `json:"execution,omitempty"`
}

func (s *CompareSideSpec) UnmarshalJSON(data []byte) error {
	type plain CompareSideSpec
	var value plain
	if err := DecodeStrict(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	has := func(name string) bool { _, ok := fields[name]; return ok }
	switch value.Kind {
	case CompareSideScope:
		if has("cohortRole") || has("execution") {
			return &ValidationError{Field: "kind", Message: "scope side contains fields from another variant"}
		}
	case CompareSideFacts:
		if has("parameters") || has("execution") {
			return &ValidationError{Field: "kind", Message: "facts side contains fields from another variant"}
		}
	case CompareSideRecord:
		if has("storeId") || has("project") || has("environment") || has("parameters") || has("cohortRole") {
			return &ValidationError{Field: "kind", Message: "record side contains fields from another variant"}
		}
	}
	*s = CompareSideSpec(value)
	return s.Validate()
}

func (s CompareSideSpec) Validate() error {
	scopeSet := s.StoreID != "" || s.Project != "" || s.Environment != ""
	switch s.Kind {
	case CompareSideScope:
		if err := (ExecutionRecordScope{StoreID: s.StoreID, Project: s.Project, Environment: s.Environment}).Validate(); err != nil {
			return err
		}
		if s.CohortRole != "" || s.Execution != nil {
			return &ValidationError{Field: "kind", Message: "scope side accepts only scope fields and parameters"}
		}
		for name, value := range s.Parameters {
			if err := requireCanonicalString("parameters", name); err != nil {
				return err
			}
			if err := value.Validate(); err != nil {
				return &ValidationError{Field: "parameters", Message: fmt.Sprintf("%s: %s", name, err)}
			}
		}
	case CompareSideFacts:
		if err := (ExecutionRecordScope{StoreID: s.StoreID, Project: s.Project, Environment: s.Environment}).Validate(); err != nil {
			return err
		}
		if s.CohortRole != CompareCohortAffected && s.CohortRole != CompareCohortControl {
			return &ValidationError{Field: "cohortRole", Message: "must be affected or control"}
		}
		if len(s.Parameters) != 0 || s.Execution != nil {
			return &ValidationError{Field: "kind", Message: "facts side accepts only scope fields and cohortRole"}
		}
	case CompareSideRecord:
		if scopeSet || len(s.Parameters) != 0 || s.CohortRole != "" || s.Execution == nil {
			return &ValidationError{Field: "kind", Message: "record side requires only execution"}
		}
		if err := s.Execution.Validate(); err != nil {
			return &ValidationError{Field: "execution", Message: err.Error()}
		}
	default:
		return &ValidationError{Field: "kind", Message: "must be scope, facts, or record"}
	}
	return nil
}

type CompareRequest struct {
	SecurityContextID  string          `json:"securityContextId"`
	QueryID            string          `json:"queryId"`
	Left               CompareSideSpec `json:"left"`
	Right              CompareSideSpec `json:"right"`
	Incident           *IncidentRef    `json:"incident,omitempty"`
	Key                []string        `json:"key,omitempty"`
	DistributionColumn string          `json:"distributionColumn,omitempty"`
	Limit              *int            `json:"limit,omitempty"`
	MutationID         string          `json:"mutationId,omitempty"`
}

func (r CompareRequest) Validate() error {
	if err := requireNonEmpty("securityContextId", r.SecurityContextID); err != nil {
		return err
	}
	if err := requireCanonicalString("queryId", r.QueryID); err != nil {
		return err
	}
	if err := r.Left.Validate(); err != nil {
		return &ValidationError{Field: "left", Message: err.Error()}
	}
	if err := r.Right.Validate(); err != nil {
		return &ValidationError{Field: "right", Message: err.Error()}
	}
	if r.Incident != nil {
		if err := r.Incident.Validate(); err != nil {
			return &ValidationError{Field: "incident", Message: err.Error()}
		}
		if err := incidents.ValidateMutationID(r.MutationID); err != nil {
			return &ValidationError{Field: "mutationId", Message: err.Error()}
		}
	} else if r.MutationID != "" {
		return &ValidationError{Field: "mutationId", Message: "requires incident"}
	}
	if (r.Left.Kind == CompareSideFacts || r.Right.Kind == CompareSideFacts) && r.Incident == nil {
		return &ValidationError{Field: "incident", Message: "is required for a facts side"}
	}
	if (r.Left.Kind == CompareSideRecord || r.Right.Kind == CompareSideRecord) && len(r.Key) == 0 {
		return &ValidationError{Field: "key", Message: "is required for a record side"}
	}
	seen := map[string]bool{}
	for _, key := range r.Key {
		if err := requireCanonicalString("key", key); err != nil {
			return err
		}
		if seen[key] {
			return &ValidationError{Field: "key", Message: fmt.Sprintf("duplicate column %q", key)}
		}
		seen[key] = true
	}
	if r.DistributionColumn != "" {
		if err := requireCanonicalString("distributionColumn", r.DistributionColumn); err != nil {
			return err
		}
	}
	if r.Limit != nil && (*r.Limit < 1 || *r.Limit > CompareMaximumLimit) {
		return &ValidationError{Field: "limit", Message: fmt.Sprintf("must be between 1 and %d", CompareMaximumLimit)}
	}
	return nil
}

type CompareSideReceipt struct {
	Execution    ExecutionRef `json:"execution"`
	ExecutedAt   string       `json:"executedAt"`
	RowCount     int          `json:"rowCount"`
	Limitations  []Limitation `json:"limitations"`
	Reproducible bool         `json:"reproducible"`
}

func (r CompareSideReceipt) Validate() error {
	if err := r.Execution.Validate(); err != nil {
		return &ValidationError{Field: "execution", Message: err.Error()}
	}
	if _, err := validateExecutionTime("executedAt", r.ExecutedAt); err != nil {
		return err
	}
	if r.RowCount < 0 {
		return &ValidationError{Field: "rowCount", Message: "must not be negative"}
	}
	for i, l := range r.Limitations {
		if err := l.Validate(); err != nil {
			return &ValidationError{Field: "limitations", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	return nil
}

type CompareRow struct {
	Key []TypedValue `json:"key"`
	Row []TypedValue `json:"row"`
}

type CompareColumnChange struct {
	Column string     `json:"column"`
	Left   TypedValue `json:"left"`
	Right  TypedValue `json:"right"`
}

type CompareChangedRow struct {
	Key     []TypedValue          `json:"key"`
	Columns []CompareColumnChange `json:"columns"`
}

type CompareOneSidedColumn struct {
	Column string `json:"column"`
	Side   string `json:"side"`
}

type CompareSummary struct {
	Added                int                     `json:"added"`
	Removed              int                     `json:"removed"`
	Changed              int                     `json:"changed"`
	Unchanged            int                     `json:"unchanged"`
	ColumnsOnlyOnOneSide []CompareOneSidedColumn `json:"columnsOnlyOnOneSide"`
}

type CompareDistributionSide struct {
	Count int     `json:"count"`
	Pct   float64 `json:"pct"`
}

type CompareDistributionValue struct {
	Value TypedValue              `json:"value"`
	Left  CompareDistributionSide `json:"left"`
	Right CompareDistributionSide `json:"right"`
	Ratio *float64                `json:"ratio"`
}

type CompareDistribution struct {
	Column    string                     `json:"column"`
	Values    []CompareDistributionValue `json:"values"`
	Truncated bool                       `json:"truncated"`
}

type CompareResult struct {
	Left          CompareSideReceipt   `json:"left"`
	Right         CompareSideReceipt   `json:"right"`
	Columns       []Column             `json:"columns"`
	Key           []string             `json:"key"`
	Added         []CompareRow         `json:"added"`
	Removed       []CompareRow         `json:"removed"`
	Changed       []CompareChangedRow  `json:"changed"`
	Summary       CompareSummary       `json:"summary"`
	Distribution  *CompareDistribution `json:"distribution,omitempty"`
	PolicyLimited bool                 `json:"policyLimited"`
	Truncated     bool                 `json:"truncated"`
}

func (r CompareResult) Validate() error {
	if err := r.Left.Validate(); err != nil {
		return &ValidationError{Field: "left", Message: err.Error()}
	}
	if err := r.Right.Validate(); err != nil {
		return &ValidationError{Field: "right", Message: err.Error()}
	}
	hiddenColumnSet := compareHiddenColumns(r.Left.Limitations, r.Right.Limitations)
	columnSet := map[string]bool{}
	columnByName := map[string]Column{}
	for i, c := range r.Columns {
		if hiddenColumnSet[c.Name] {
			return &ValidationError{Field: "columns", Message: fmt.Sprintf("shared column index %d is hidden by policy", i)}
		}
		if err := c.Validate(); err != nil {
			return &ValidationError{Field: "columns", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
		if columnSet[c.Name] {
			return &ValidationError{Field: "columns", Message: fmt.Sprintf("duplicate column %q", c.Name)}
		}
		if i > 0 && r.Columns[i-1].Name >= c.Name {
			return &ValidationError{Field: "columns", Message: "must be in stable name order"}
		}
		columnSet[c.Name] = true
		columnByName[c.Name] = c
	}
	if len(r.Key) == 0 {
		return &ValidationError{Field: "key", Message: "is required"}
	}
	keySet := map[string]bool{}
	keyColumns := make([]Column, 0, len(r.Key))
	keyIndexes := make([]int, 0, len(r.Key))
	for _, key := range r.Key {
		if strings.TrimSpace(key) == "" || !columnSet[key] {
			return &ValidationError{Field: "key", Message: fmt.Sprintf("column %q is not shared", key)}
		}
		if keySet[key] {
			return &ValidationError{Field: "key", Message: fmt.Sprintf("duplicate column %q", key)}
		}
		keySet[key] = true
		keyColumns = append(keyColumns, columnByName[key])
		for index, column := range r.Columns {
			if column.Name == key {
				keyIndexes = append(keyIndexes, index)
				break
			}
		}
	}
	emitted := len(r.Added) + len(r.Removed) + len(r.Changed)
	if emitted > CompareMaximumLimit {
		return &ValidationError{Field: "added/removed/changed", Message: fmt.Sprintf("emitted rows exceed maximum %d", CompareMaximumLimit)}
	}
	seenKeys := map[string]string{}
	for _, rows := range []struct {
		name string
		rows []CompareRow
	}{{"added", r.Added}, {"removed", r.Removed}} {
		var priorKey []TypedValue
		for i, row := range rows.rows {
			if err := validateCompareValues(rows.name, i, "key", row.Key, len(r.Key)); err != nil {
				return err
			}
			if err := validateCompareValues(rows.name, i, "row", row.Row, len(r.Columns)); err != nil {
				return err
			}
			if err := validateCompareColumnTypes(rows.name, i, "key", row.Key, keyColumns); err != nil {
				return err
			}
			if err := validateCompareColumnTypes(rows.name, i, "row", row.Row, r.Columns); err != nil {
				return err
			}
			for keyIndex, rowIndex := range keyIndexes {
				if row.Key[keyIndex] != row.Row[rowIndex] {
					return &ValidationError{Field: rows.name, Message: fmt.Sprintf("row %d key does not match row key-column cells", i)}
				}
			}
			if err := validateCompareKey(rows.name, i, row.Key, priorKey, seenKeys); err != nil {
				return err
			}
			priorKey = append([]TypedValue(nil), row.Key...)
		}
	}
	var priorChangedKey []TypedValue
	for i, row := range r.Changed {
		if err := validateCompareValues("changed", i, "key", row.Key, len(r.Key)); err != nil {
			return err
		}
		if len(row.Columns) == 0 {
			return &ValidationError{Field: "changed", Message: fmt.Sprintf("row %d has no changed columns", i)}
		}
		if err := validateCompareColumnTypes("changed", i, "key", row.Key, keyColumns); err != nil {
			return err
		}
		if err := validateCompareKey("changed", i, row.Key, priorChangedKey, seenKeys); err != nil {
			return err
		}
		priorChangedKey = append([]TypedValue(nil), row.Key...)
		seen := map[string]bool{}
		for j, change := range row.Columns {
			if !columnSet[change.Column] || keySet[change.Column] || seen[change.Column] {
				return &ValidationError{Field: "changed", Message: fmt.Sprintf("row %d column %d is inconsistent", i, j)}
			}
			if j > 0 && row.Columns[j-1].Column >= change.Column {
				return &ValidationError{Field: "changed", Message: fmt.Sprintf("row %d columns are not stable", i)}
			}
			if err := change.Left.Validate(); err != nil {
				return &ValidationError{Field: "changed", Message: fmt.Sprintf("row %d left: %s", i, err)}
			}
			if err := change.Right.Validate(); err != nil {
				return &ValidationError{Field: "changed", Message: fmt.Sprintf("row %d right: %s", i, err)}
			}
			column := columnByName[change.Column]
			if !typedValueMatchesColumn(change.Left, column) || !typedValueMatchesColumn(change.Right, column) {
				return &ValidationError{Field: "changed", Message: fmt.Sprintf("row %d column %d does not match declared column type", i, j)}
			}
			if change.Left == change.Right {
				return &ValidationError{Field: "changed", Message: fmt.Sprintf("row %d column %q is unchanged", i, change.Column)}
			}
			seen[change.Column] = true
		}
	}
	if r.Summary.Added < len(r.Added) || r.Summary.Removed < len(r.Removed) || r.Summary.Changed < len(r.Changed) || r.Summary.Unchanged < 0 {
		return &ValidationError{Field: "summary", Message: "counts must be non-negative and cover emitted rows"}
	}
	totalDifferences := r.Summary.Added + r.Summary.Removed + r.Summary.Changed
	if r.Truncated != (emitted < totalDifferences) {
		return &ValidationError{Field: "truncated", Message: "must equal emitted rows less than summary differences"}
	}
	if r.Left.RowCount != r.Summary.Removed+r.Summary.Changed+r.Summary.Unchanged || r.Right.RowCount != r.Summary.Added+r.Summary.Changed+r.Summary.Unchanged {
		return &ValidationError{Field: "summary", Message: "counts do not reconcile with side receipts"}
	}
	oneSidedNames := map[string]bool{}
	for i, oneSided := range r.Summary.ColumnsOnlyOnOneSide {
		if strings.TrimSpace(oneSided.Column) == "" || (oneSided.Side != CompareColumnLeft && oneSided.Side != CompareColumnRight) {
			return &ValidationError{Field: "columnsOnlyOnOneSide", Message: fmt.Sprintf("index %d is invalid", i)}
		}
		if columnSet[oneSided.Column] {
			return &ValidationError{Field: "columnsOnlyOnOneSide", Message: fmt.Sprintf("column %q is shared", oneSided.Column)}
		}
		if hiddenColumnSet[oneSided.Column] {
			return &ValidationError{Field: "columnsOnlyOnOneSide", Message: fmt.Sprintf("index %d is hidden by policy", i)}
		}
		if oneSidedNames[oneSided.Column] {
			return &ValidationError{Field: "columnsOnlyOnOneSide", Message: fmt.Sprintf("duplicate column %q", oneSided.Column)}
		}
		oneSidedNames[oneSided.Column] = true
		if i > 0 {
			prior := r.Summary.ColumnsOnlyOnOneSide[i-1]
			if prior.Column > oneSided.Column || (prior.Column == oneSided.Column && prior.Side >= oneSided.Side) {
				return &ValidationError{Field: "columnsOnlyOnOneSide", Message: "must be stable and unique"}
			}
		}
	}
	if r.PolicyLimited != (compareRowsFiltered(r.Left.Limitations) || compareRowsFiltered(r.Right.Limitations)) {
		return &ValidationError{Field: "policyLimited", Message: "must reflect rowsFiltered limitations"}
	}
	if r.Distribution != nil {
		if !columnSet[r.Distribution.Column] {
			return &ValidationError{Field: "distribution", Message: "column must be shared"}
		}
		if len(r.Distribution.Values) > CompareDistributionMaximumValues {
			return &ValidationError{Field: "distribution", Message: fmt.Sprintf("must contain at most %d values", CompareDistributionMaximumValues)}
		}
		leftCount, rightCount := 0, 0
		priorKey := ""
		for i, value := range r.Distribution.Values {
			if err := value.Value.Validate(); err != nil {
				return &ValidationError{Field: "distribution", Message: fmt.Sprintf("value %d: %s", i, err)}
			}
			if !typedValueMatchesColumn(value.Value, columnByName[r.Distribution.Column]) {
				return &ValidationError{Field: "distribution", Message: fmt.Sprintf("value %d does not match declared column type", i)}
			}
			stableKey := compareTypedStableKey(value.Value)
			if i > 0 && priorKey >= stableKey {
				return &ValidationError{Field: "distribution", Message: "values must be stable and unique"}
			}
			priorKey = stableKey
			if value.Left.Count < 0 || value.Right.Count < 0 || value.Left.Pct < 0 || value.Left.Pct > 100 || value.Right.Pct < 0 || value.Right.Pct > 100 {
				return &ValidationError{Field: "distribution", Message: fmt.Sprintf("value %d has invalid count or pct", i)}
			}
			leftCount += value.Left.Count
			rightCount += value.Right.Count
			if value.Left.Pct != comparePercentage(value.Left.Count, r.Left.RowCount) || value.Right.Pct != comparePercentage(value.Right.Count, r.Right.RowCount) {
				return &ValidationError{Field: "distribution", Message: fmt.Sprintf("value %d pct does not match count and side total", i)}
			}
			if (value.Ratio == nil) != (value.Right.Pct == 0) {
				return &ValidationError{Field: "distribution", Message: fmt.Sprintf("value %d ratio presence must follow right pct", i)}
			}
			if value.Ratio != nil && (math.IsNaN(*value.Ratio) || math.IsInf(*value.Ratio, 0) || *value.Ratio != value.Left.Pct/value.Right.Pct) {
				return &ValidationError{Field: "distribution", Message: fmt.Sprintf("value %d ratio must equal left pct divided by right pct", i)}
			}
		}
		if leftCount > r.Left.RowCount || rightCount > r.Right.RowCount || (!r.Distribution.Truncated && (leftCount != r.Left.RowCount || rightCount != r.Right.RowCount)) {
			return &ValidationError{Field: "distribution", Message: "counts do not reconcile with side totals"}
		}
		if r.Distribution.Truncated && len(r.Distribution.Values) != CompareDistributionMaximumValues {
			return &ValidationError{Field: "distribution", Message: fmt.Sprintf("truncated distribution must emit exactly %d values", CompareDistributionMaximumValues)}
		}
		if r.Distribution.Truncated && leftCount == r.Left.RowCount && rightCount == r.Right.RowCount {
			return &ValidationError{Field: "distribution", Message: "truncated distribution must have at least one omitted side count"}
		}
	}
	return nil
}

func validateCompareValues(group string, row int, field string, values []TypedValue, expected int) error {
	if len(values) != expected {
		return &ValidationError{Field: group, Message: fmt.Sprintf("row %d %s has %d values, want %d", row, field, len(values), expected)}
	}
	for i, value := range values {
		if err := value.Validate(); err != nil {
			return &ValidationError{Field: group, Message: fmt.Sprintf("row %d %s %d: %s", row, field, i, err)}
		}
	}
	return nil
}

func validateCompareColumnTypes(group string, row int, field string, values []TypedValue, columns []Column) error {
	for index, value := range values {
		if !typedValueMatchesColumn(value, columns[index]) {
			return &ValidationError{Field: group, Message: fmt.Sprintf("row %d %s %d does not match declared column type", row, field, index)}
		}
	}
	return nil
}

func typedValueMatchesColumn(value TypedValue, column Column) bool {
	return value.Type == ValueTypeNull || string(value.Type) == column.Type
}

func validateCompareKey(group string, row int, key, prior []TypedValue, seen map[string]string) error {
	for _, value := range key {
		if value.Type == ValueTypeNull {
			return &ValidationError{Field: group, Message: fmt.Sprintf("row %d has a null/missing key", row)}
		}
	}
	identity := compareKeyIdentity(key)
	if priorGroup, exists := seen[identity]; exists {
		return &ValidationError{Field: group, Message: fmt.Sprintf("row %d has duplicate key already emitted in %s", row, priorGroup)}
	}
	if prior != nil && compareTypedTuples(prior, key) >= 0 {
		return &ValidationError{Field: group, Message: fmt.Sprintf("row %d is not in stable key order", row)}
	}
	seen[identity] = group
	return nil
}

func compareKeyIdentity(values []TypedValue) string {
	normalized := append([]TypedValue(nil), values...)
	for index := range normalized {
		if normalized[index].Type == ValueTypeNumber && normalized[index].Num == 0 {
			normalized[index].Num = 0
		}
	}
	data, _ := json.Marshal(normalized)
	return string(data)
}

func compareTypedTuples(left, right []TypedValue) int {
	for index := range left {
		if order := strings.Compare(compareTypedStableKey(left[index]), compareTypedStableKey(right[index])); order != 0 {
			return order
		}
	}
	return 0
}

func compareRowsFiltered(limitations []Limitation) bool {
	for _, limitation := range limitations {
		if limitation.RowsFiltered {
			return true
		}
	}
	return false
}

func compareHiddenColumns(groups ...[]Limitation) map[string]bool {
	hidden := map[string]bool{}
	for _, limitations := range groups {
		for _, limitation := range limitations {
			for _, column := range limitation.HiddenColumns {
				hidden[column] = true
			}
		}
	}
	return hidden
}

func comparePercentage(count, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(count) * 100 / float64(total)
}

func compareTypedStableKey(value TypedValue) string {
	switch value.Type {
	case ValueTypeNumber:
		if value.Num == 0 {
			return string(value.Type) + ":0"
		}
		return string(value.Type) + ":" + strconv.FormatFloat(value.Num, 'g', -1, 64)
	case ValueTypeBoolean:
		return string(value.Type) + ":" + strconv.FormatBool(value.Bool)
	default:
		return string(value.Type) + ":" + value.Str
	}
}

// CompareErrorResponse preserves successfully persisted live-side receipts
// when comparison is incomplete. Comparison identifies an already-applied
// incident mutation; it does not imply a cached CompareResult exists.
type CompareErrorResponse struct {
	Error      ErrorBody                `json:"error"`
	Left       *CompareSideReceipt      `json:"left,omitempty"`
	Right      *CompareSideReceipt      `json:"right,omitempty"`
	Comparison *incidents.ComparisonRef `json:"comparison,omitempty"`
}

func (r CompareErrorResponse) Validate() error {
	if err := r.Error.Validate(); err != nil {
		return err
	}
	if r.Left != nil {
		if err := r.Left.Validate(); err != nil {
			return &ValidationError{Field: "left", Message: err.Error()}
		}
	}
	if r.Right != nil {
		if err := r.Right.Validate(); err != nil {
			return &ValidationError{Field: "right", Message: err.Error()}
		}
	}
	if r.Comparison != nil {
		if err := r.Comparison.Validate(); err != nil {
			return &ValidationError{Field: "comparison", Message: err.Error()}
		}
	}
	return nil
}
