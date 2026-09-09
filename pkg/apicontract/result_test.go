package apicontract

import (
	"encoding/json"
	"testing"
)

func validResult() Result {
	return Result{
		Recordset: Recordset{
			Columns: []Column{{Name: "GenreName", Type: "string"}, {Name: "TotalSpent", Type: "decimal"}},
			Rows: [][]TypedValue{
				{NewStringValue("Rock"), NewDecimalValue("14.85")},
			},
		},
		Limitations:     []Limitation{},
		BindingsApplied: []Binding{{ParameterID: "CustomerId", Value: NewIntegerValue("5"), Origin: "selection", OriginEvidence: "client-reported"}},
		Provenance: Provenance{
			Source:           "chinook",
			Mode:             "live",
			ObservedAt:       "2026-09-09T12:00:00Z",
			ExecutionProfile: "protected",
		},
		Truncated: false,
	}
}

func TestResult_JSONFieldNames(t *testing.T) {
	r := validResult()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var back Result
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}

	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"recordset", "limitations", "bindingsApplied", "provenance", "truncated"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("missing top-level key %q in %s", key, data)
		}
	}
	recordset, _ := generic["recordset"].(map[string]any)
	for _, key := range []string{"columns", "rows"} {
		if _, ok := recordset[key]; !ok {
			t.Errorf("missing recordset key %q", key)
		}
	}
	provenance, _ := generic["provenance"].(map[string]any)
	for _, key := range []string{"source", "mode", "observedAt", "executionProfile"} {
		if _, ok := provenance[key]; !ok {
			t.Errorf("missing provenance key %q", key)
		}
	}
}

func TestResult_ProvenanceOptionalFieldsOmittedWhenAbsent(t *testing.T) {
	r := validResult()
	data, err := json.Marshal(r.Provenance)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"source":"chinook","mode":"live","observedAt":"2026-09-09T12:00:00Z","executionProfile":"protected"}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestResult_EmptyRecordsetAndLimitationsMarshalAsEmptyArrays(t *testing.T) {
	r := validResult()
	r.Recordset.Rows = [][]TypedValue{}
	r.Limitations = []Limitation{}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]json.RawMessage
	_ = json.Unmarshal(data, &generic)
	if string(generic["limitations"]) != "[]" {
		t.Errorf("limitations = %s, want []", generic["limitations"])
	}
}

func TestResult_Validate(t *testing.T) {
	if err := validResult().Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}

func TestResult_Validate_RowColumnCountMismatch(t *testing.T) {
	r := validResult()
	r.Recordset.Rows = [][]TypedValue{{NewStringValue("only one value")}}
	if err := r.Validate(); err == nil {
		t.Error("expected an error: row has a different number of values than there are columns")
	}
}

func TestResult_Validate_UnknownColumnType(t *testing.T) {
	r := validResult()
	r.Recordset.Columns[0].Type = "currency"
	if err := r.Validate(); err == nil {
		t.Error("expected an error: unsupported column type")
	}
}

func TestResult_Validate_InvalidCellValue(t *testing.T) {
	r := validResult()
	r.Recordset.Rows[0][1] = NewDecimalValue("not-a-decimal")
	if err := r.Validate(); err == nil {
		t.Error("expected an error: invalid cell value")
	}
}

func TestColumn_Validate(t *testing.T) {
	if err := (Column{Name: "GenreName", Type: "string"}).Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
	if err := (Column{Type: "string"}).Validate(); err == nil {
		t.Error("expected an error: missing name")
	}
}

func TestResult_Validate_InvalidLimitation(t *testing.T) {
	r := validResult()
	r.Limitations = []Limitation{{HiddenColumns: []string{}}} // missing policy
	if err := r.Validate(); err == nil {
		t.Error("expected an error: invalid limitation")
	}
}

func TestResult_Validate_InvalidBindingApplied(t *testing.T) {
	r := validResult()
	r.BindingsApplied = []Binding{{ParameterID: "p", Value: NewIntegerValue("bad"), Origin: "selection", OriginEvidence: "client-reported"}}
	if err := r.Validate(); err == nil {
		t.Error("expected an error: invalid applied binding")
	}
}

func TestResult_Validate_InvalidProvenance(t *testing.T) {
	r := validResult()
	r.Provenance.Source = ""
	if err := r.Validate(); err == nil {
		t.Error("expected an error: invalid provenance embedded in result")
	}
}

func TestResult_Validate_Provenance(t *testing.T) {
	valid := Provenance{Source: "chinook", Mode: "live", ObservedAt: "2026-09-09T12:00:00Z", ExecutionProfile: "protected"}
	if err := valid.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	missingSource := valid
	missingSource.Source = ""
	if err := missingSource.Validate(); err == nil {
		t.Error("expected an error: missing source")
	}

	badMode := valid
	badMode.Mode = "cached"
	if err := badMode.Validate(); err == nil {
		t.Error("expected an error: invalid mode")
	}

	snapshotWithoutID := valid
	snapshotWithoutID.Mode = "snapshot"
	if err := snapshotWithoutID.Validate(); err == nil {
		t.Error("expected an error: snapshot mode requires snapshotId")
	}

	snapshotWithID := valid
	snapshotWithID.Mode = "snapshot"
	snapshotWithID.SnapshotID = "snap-1"
	if err := snapshotWithID.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	badObservedAt := valid
	badObservedAt.ObservedAt = "2026-09-09"
	if err := badObservedAt.Validate(); err == nil {
		t.Error("expected an error: observedAt must be RFC3339 UTC")
	}

	badExecutionProfile := valid
	badExecutionProfile.ExecutionProfile = "admin"
	if err := badExecutionProfile.Validate(); err == nil {
		t.Error("expected an error: invalid executionProfile")
	}

	opaquePrivileged := valid
	opaquePrivileged.ExecutionProfile = "opaque-privileged"
	if err := opaquePrivileged.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}
