package apicontract

import (
	"encoding/json"
	"testing"
)

func TestSemanticColumnsResponse_JSONFieldNames(t *testing.T) {
	r := SemanticColumnsResponse{
		Columns: []SemanticColumnMapping{
			{Column: "CustomerId", Entity: "Customer", Field: "ID", Provenance: "declared"},
		},
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"columns":[{"column":"CustomerId","entity":"Customer","field":"ID","provenance":"declared"}]}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestSemanticColumnsResponse_EmptyColumnsMarshalsAsEmptyArray(t *testing.T) {
	r := SemanticColumnsResponse{Columns: []SemanticColumnMapping{}}
	data, _ := json.Marshal(r)
	if string(data) != `{"columns":[]}` {
		t.Errorf("got %s", data)
	}
}

func TestSemanticColumnsResponse_Validate(t *testing.T) {
	valid := SemanticColumnsResponse{Columns: []SemanticColumnMapping{
		{Column: "CustomerId", Entity: "Customer", Field: "ID", Provenance: "declared"},
	}}
	if err := valid.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	badProvenance := SemanticColumnsResponse{Columns: []SemanticColumnMapping{
		{Column: "CustomerId", Entity: "Customer", Field: "ID", Provenance: "guessed"},
	}}
	if err := badProvenance.Validate(); err == nil {
		t.Error("expected an error: invalid provenance")
	}
}

func TestSemanticColumnMapping_Validate(t *testing.T) {
	cases := []SemanticColumnMapping{
		{Entity: "Customer", Field: "ID", Provenance: "declared"},
		{Column: "CustomerId", Field: "ID", Provenance: "declared"},
		{Column: "CustomerId", Entity: "Customer", Provenance: "declared"},
	}
	for _, c := range cases {
		if err := c.Validate(); err == nil {
			t.Errorf("expected an error for %+v", c)
		}
	}
}

func TestRelatedResponse_JSONFieldNames_CountPresent(t *testing.T) {
	count := int64(3)
	r := RelatedResponse{
		Related: []RelatedItem{
			{LookupID: "l1", Label: "Invoices", Source: "chinook", Collection: "Invoice", Count: &count},
		},
		Truncated: false,
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"related":[{"lookupId":"l1","label":"Invoices","source":"chinook","collection":"Invoice","count":3}],"truncated":false}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestRelatedResponse_CountNullWhenUnavailable(t *testing.T) {
	r := RelatedResponse{
		Related:   []RelatedItem{{LookupID: "l1", Label: "Invoices", Source: "chinook", Collection: "Invoice", Count: nil}},
		Truncated: false,
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"related":[{"lookupId":"l1","label":"Invoices","source":"chinook","collection":"Invoice","count":null}],"truncated":false}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestRelatedResponse_EmptyRelatedMarshalsAsEmptyArray(t *testing.T) {
	r := RelatedResponse{Related: []RelatedItem{}, Truncated: false}
	data, _ := json.Marshal(r)
	if string(data) != `{"related":[],"truncated":false}` {
		t.Errorf("got %s", data)
	}
}

func TestRelatedResponse_Validate(t *testing.T) {
	count := int64(3)
	valid := RelatedResponse{Related: []RelatedItem{
		{LookupID: "l1", Label: "Invoices", Source: "chinook", Collection: "Invoice", Count: &count},
	}}
	if err := valid.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	nilCount := RelatedResponse{Related: []RelatedItem{
		{LookupID: "l1", Label: "Invoices", Source: "chinook", Collection: "Invoice", Count: nil},
	}}
	if err := nilCount.Validate(); err != nil {
		t.Errorf("expected a nil count to be valid (unavailable, per the contract), got: %v", err)
	}

	negativeCount := int64(-1)
	badCount := RelatedResponse{Related: []RelatedItem{
		{LookupID: "l1", Label: "Invoices", Source: "chinook", Collection: "Invoice", Count: &negativeCount},
	}}
	if err := badCount.Validate(); err == nil {
		t.Error("expected an error: negative count")
	}

	missingLookupID := RelatedResponse{Related: []RelatedItem{
		{Label: "Invoices", Source: "chinook", Collection: "Invoice"},
	}}
	if err := missingLookupID.Validate(); err == nil {
		t.Error("expected an error: missing lookupId")
	}
}

func TestRelatedItem_Validate(t *testing.T) {
	cases := []RelatedItem{
		{LookupID: "l1", Source: "chinook", Collection: "Invoice"}, // missing label
		{LookupID: "l1", Label: "Invoices", Collection: "Invoice"}, // missing source
		{LookupID: "l1", Label: "Invoices", Source: "chinook"},     // missing collection
	}
	for _, c := range cases {
		if err := c.Validate(); err == nil {
			t.Errorf("expected an error for %+v", c)
		}
	}
}

func TestRelatedResponse_Validate_AtMost50Targets(t *testing.T) {
	items := make([]RelatedItem, 51)
	for i := range items {
		items[i] = RelatedItem{LookupID: "l", Label: "l", Source: "s", Collection: "c"}
	}
	r := RelatedResponse{Related: items, Truncated: true}
	if err := r.Validate(); err == nil {
		t.Error("expected an error: more than 50 related targets")
	}
}

func TestApplicableResponse_JSONFieldNames(t *testing.T) {
	r := ApplicableResponse{Applicable: []Candidate{}, NotYet: []Candidate{}}
	data, _ := json.Marshal(r)
	if string(data) != `{"applicable":[],"notYet":[]}` {
		t.Errorf("got %s", data)
	}
}

func TestApplicableResponse_Validate_ApplicableMustBeRunnable(t *testing.T) {
	notRunnable := validCandidate()
	notRunnable.State = CandidateStateNeedsInput
	r := ApplicableResponse{Applicable: []Candidate{notRunnable}, NotYet: []Candidate{}}
	if err := r.Validate(); err == nil {
		t.Error("expected an error: applicable contains a non-runnable candidate")
	}
}

func TestApplicableResponse_Validate_NotYetMustNotBeRunnable(t *testing.T) {
	runnable := validCandidate() // state: runnable
	r := ApplicableResponse{Applicable: []Candidate{}, NotYet: []Candidate{runnable}}
	if err := r.Validate(); err == nil {
		t.Error("expected an error: notYet contains a runnable candidate")
	}
}

func TestApplicableResponse_Validate_SortedByQueryID(t *testing.T) {
	a := validCandidate()
	a.QueryID = "zzz-last"
	b := validCandidate()
	b.QueryID = "aaa-first"
	r := ApplicableResponse{Applicable: []Candidate{a, b}, NotYet: []Candidate{}}
	if err := r.Validate(); err == nil {
		t.Error("expected an error: applicable is not sorted by queryId")
	}
}

func TestApplicableResponse_Validate_NotYetSortedByQueryID(t *testing.T) {
	a := validCandidate()
	a.State = CandidateStateNeedsInput
	a.QueryID = "zzz-last"
	b := validCandidate()
	b.State = CandidateStateNeedsInput
	b.QueryID = "aaa-first"
	r := ApplicableResponse{Applicable: []Candidate{}, NotYet: []Candidate{a, b}}
	if err := r.Validate(); err == nil {
		t.Error("expected an error: notYet is not sorted by queryId")
	}
}

func TestApplicableResponse_Validate_InvalidCandidateEmbedded(t *testing.T) {
	badApplicable := validCandidate()
	badApplicable.QueryID = ""
	r := ApplicableResponse{Applicable: []Candidate{badApplicable}, NotYet: []Candidate{}}
	if err := r.Validate(); err == nil {
		t.Error("expected an error: invalid candidate in applicable")
	}

	badNotYet := validCandidate()
	badNotYet.State = CandidateStateNeedsInput
	badNotYet.QueryID = ""
	r2 := ApplicableResponse{Applicable: []Candidate{}, NotYet: []Candidate{badNotYet}}
	if err := r2.Validate(); err == nil {
		t.Error("expected an error: invalid candidate in notYet")
	}
}

func TestApplicableResponse_Validate_EqualQueryIDsAreSorted(t *testing.T) {
	// Equal adjacent keys are non-decreasing, so still "sorted".
	a := validCandidate()
	a.QueryID = "same-id"
	b := validCandidate()
	b.QueryID = "same-id"
	r := ApplicableResponse{Applicable: []Candidate{a, b}, NotYet: []Candidate{}}
	if err := r.Validate(); err != nil {
		t.Errorf("expected equal adjacent queryIds to count as sorted, got: %v", err)
	}
}

func TestApplicableResponse_Validate_Valid(t *testing.T) {
	a := validCandidate()
	a.QueryID = "aaa-first"
	b := validCandidate()
	b.QueryID = "zzz-last"
	needsInput := validCandidate()
	needsInput.QueryID = "needs-input-query"
	needsInput.State = CandidateStateNeedsInput
	r := ApplicableResponse{Applicable: []Candidate{a, b}, NotYet: []Candidate{needsInput}}
	if err := r.Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}
}
