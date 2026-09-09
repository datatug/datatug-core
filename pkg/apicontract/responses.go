package apicontract

import (
	"fmt"
	"slices"
)

const (
	SemanticProvenanceDeclared = "declared"
	SemanticProvenanceInferred = "inferred"
)

// SemanticColumnMapping is one physical column's resolved entity/field
// mapping. Unmapped columns are omitted from the response entirely, never
// listed with an empty entity/field.
type SemanticColumnMapping struct {
	Column     string `json:"column"`
	Entity     string `json:"entity"`
	Field      string `json:"field"`
	Provenance string `json:"provenance"` // declared | inferred
}

func (m SemanticColumnMapping) Validate() error {
	if err := requireNonEmpty("column", m.Column); err != nil {
		return err
	}
	if err := requireNonEmpty("entity", m.Entity); err != nil {
		return err
	}
	if err := requireNonEmpty("field", m.Field); err != nil {
		return err
	}
	return requireOneOf("provenance", m.Provenance, SemanticProvenanceDeclared, SemanticProvenanceInferred)
}

// SemanticColumnsResponse is the exact success envelope of GET
// semantic/columns. api-contract.md "Endpoint table".
type SemanticColumnsResponse struct {
	Columns []SemanticColumnMapping `json:"columns"`
}

func (r SemanticColumnsResponse) Validate() error {
	for i, c := range r.Columns {
		if err := c.Validate(); err != nil {
			return &ValidationError{Field: "columns", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	return nil
}

// relatedMaxItems: "Related discovery returns at most 50 targets; reaching
// the cap sets truncated."
const relatedMaxItems = 50

// RelatedItem is one related-lookup target. Count is optional: nil means
// "return null if an exact authorized count cannot be obtained within a
// 2-second budget" - never the unrestricted count.
type RelatedItem struct {
	LookupID   string `json:"lookupId"`
	Label      string `json:"label"`
	Source     string `json:"source"`
	Collection string `json:"collection"`
	Count      *int64 `json:"count"` // number | null
}

func (i RelatedItem) Validate() error {
	if err := requireNonEmpty("lookupId", i.LookupID); err != nil {
		return err
	}
	if err := requireNonEmpty("label", i.Label); err != nil {
		return err
	}
	if err := requireNonEmpty("source", i.Source); err != nil {
		return err
	}
	if err := requireNonEmpty("collection", i.Collection); err != nil {
		return err
	}
	if i.Count != nil && *i.Count < 0 {
		return &ValidationError{Field: "count", Message: "must not be negative"}
	}
	return nil
}

// RelatedResponse is the exact success envelope of POST semantic/related.
// api-contract.md "Endpoint table".
type RelatedResponse struct {
	Related   []RelatedItem `json:"related"`
	Truncated bool          `json:"truncated"`
}

// Validate enforces every Related entry is itself valid and there are no
// more than relatedMaxItems of them.
func (r RelatedResponse) Validate() error {
	if len(r.Related) > relatedMaxItems {
		return &ValidationError{Field: "related", Message: fmt.Sprintf("must return at most %d targets, got %d", relatedMaxItems, len(r.Related))}
	}
	for i, item := range r.Related {
		if err := item.Validate(); err != nil {
			return &ValidationError{Field: "related", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	return nil
}

// ApplicableResponse is the exact success envelope of POST
// queries/applicable. "applicable contains only runnable candidates. notYet
// contains authorized query metadata for needs-input/needs-target/
// source-unavailable candidates... Candidates sort by queryId for
// deterministic tests." api-contract.md "Endpoint table".
type ApplicableResponse struct {
	Applicable []Candidate `json:"applicable"`
	NotYet     []Candidate `json:"notYet"`
}

// Validate enforces every candidate is itself valid, every Applicable entry
// has State "runnable", every NotYet entry does not, and both slices are
// sorted by QueryID.
func (r ApplicableResponse) Validate() error {
	for i, c := range r.Applicable {
		if err := c.Validate(); err != nil {
			return &ValidationError{Field: "applicable", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
		if c.State != CandidateStateRunnable {
			return &ValidationError{Field: "applicable", Message: fmt.Sprintf("index %d: state %q, want %q", i, c.State, CandidateStateRunnable)}
		}
	}
	for i, c := range r.NotYet {
		if err := c.Validate(); err != nil {
			return &ValidationError{Field: "notYet", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
		if c.State == CandidateStateRunnable {
			return &ValidationError{Field: "notYet", Message: fmt.Sprintf("index %d: state must not be %q", i, CandidateStateRunnable)}
		}
	}
	if !slices.IsSortedFunc(r.Applicable, compareCandidateByQueryID) {
		return &ValidationError{Field: "applicable", Message: "must be sorted by queryId"}
	}
	if !slices.IsSortedFunc(r.NotYet, compareCandidateByQueryID) {
		return &ValidationError{Field: "notYet", Message: "must be sorted by queryId"}
	}
	return nil
}

func compareCandidateByQueryID(a, b Candidate) int {
	switch {
	case a.QueryID < b.QueryID:
		return -1
	case a.QueryID > b.QueryID:
		return 1
	default:
		return 0
	}
}
