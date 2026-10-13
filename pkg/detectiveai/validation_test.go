package detectiveai

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/detective"
	"github.com/datatug/datatug-core/pkg/investigation"
)

func cited() []Provenance {
	return []Provenance{{SourceID: "ticket", Start: 0, End: 9, Quote: "invoice 7", Confidence: .8}}
}

func typedDrafts() []Draft {
	value := investigation.NewIntegerValue("7")
	return []Draft{
		{Kind: KindContext, Title: "Extract invoice ID", Context: map[string]ExtractedValue{"invoice_id": {Value: detective.InputValue{Scalar: &value}, Provenance: cited()}}, Sources: cited()},
		{Kind: KindQuery, Title: "Save detail query", Query: &QueryDraft{ID: "invoice-detail", Title: "Invoice detail", Type: "SQL", Text: "SELECT id FROM invoices", Inputs: []detective.Input{{Name: "invoice_id", Type: investigation.ValueTypeInteger, Cardinality: "scalar"}}}, Sources: cited()},
		{Kind: KindCheck, Title: "Save composite Check", Check: &detective.Check{ID: "new-check", Title: "Check detail", Steps: []detective.Step{{ID: "get-detail", Kind: "query", QueryID: "invoice-list", Validators: []detective.Validator{{ID: "nonnull", Scope: "row", Kind: "non-null", Severity: "error", Field: "id"}}}}}, Sources: cited()},
		{Kind: KindValidator, Title: "Promote Validator", Validator: &detective.PublishedValidator{ID: "required-id", Title: "Required ID", Validator: detective.Validator{ID: "nonnull", Scope: "row", Kind: "non-null", Severity: "error", Field: "id"}}, Sources: cited()},
		{Kind: KindScenario, Title: "Save guide", Scenario: &detective.Scenario{ID: "new-guide", Title: "Investigate invoice", Steps: []detective.ScenarioStep{{ID: "list", Kind: "query", Ref: "invoice-list"}, {ID: "check", Kind: "check", Ref: "invoice-check"}}}, Sources: cited()},
		{Kind: KindCase, Title: "Edit case", Case: &CaseEdit{CaseID: "case-a", ExpectedVersion: 3, Title: "Invoice issue", Status: "active", Context: map[string]detective.InputValue{"invoice_id": {Scalar: &value}}, Findings: []detective.Finding{{ID: "finding-a", Text: "Total mismatch", Status: "hypothesis"}}}, Sources: cited()},
		runDraft(),
		{Kind: KindExplain, Title: "Explain observed failure", Explanation: &FailureExplanation{RunID: "run-a", Summary: "The observed row differed", Basis: "run-a Validator result"}, Sources: cited()},
	}
}

func TestTypedProposalKindsAndMissingReferences(t *testing.T) {
	_, snapshot, sources := baseline()
	for _, draft := range typedDrafts() {
		t.Run(draft.Kind, func(t *testing.T) {
			if err := validateDraft(draft, snapshot, sources); err != nil {
				t.Fatalf("valid typed proposal rejected: %v", err)
			}
		})
	}
	for _, tc := range []struct {
		name string
		edit func(*Draft)
		want error
	}{
		{"two payloads", func(d *Draft) { d.Query = typedDrafts()[1].Query }, ErrInvalid},
		{"unknown kind", func(d *Draft) { d.Kind = "close-everything" }, ErrUnsupported},
		{"missing provenance", func(d *Draft) { d.Sources = nil }, ErrInvalid},
		{"ambiguous without description", func(d *Draft) { d.Ambiguities = []string{" "} }, ErrInvalid},
		{"invalid typed value", func(d *Draft) { d.Context["invoice_id"] = ExtractedValue{} }, ErrInvalid},
		{"invalid context key", func(d *Draft) { d.Context["bad key"] = d.Context["invoice_id"] }, ErrInvalid},
		{"query missing type", func(d *Draft) { *d = typedDrafts()[1]; d.Query.Type = "" }, ErrInvalid},
		{"query stale revision", func(d *Draft) { *d = typedDrafts()[1]; d.Query.ID = "invoice-list" }, ErrStale},
		{"query duplicate inputs", func(d *Draft) { *d = typedDrafts()[1]; d.Query.Inputs = append(d.Query.Inputs, d.Query.Inputs[0]) }, ErrInvalid},
		{"check missing query", func(d *Draft) { *d = typedDrafts()[2]; d.Check.Steps[0].QueryID = "missing" }, ErrInvalid},
		{"check invalid definition", func(d *Draft) { *d = typedDrafts()[2]; d.Check.Steps[0].ID = "BAD" }, ErrInvalid},
		{"validator missing child", func(d *Draft) {
			*d = typedDrafts()[3]
			d.Validator.Validator = detective.Validator{ID: "child", Scope: "row", Kind: "check", Severity: "error", CheckID: "missing"}
		}, ErrInvalid},
		{"validator invalid definition", func(d *Draft) { *d = typedDrafts()[3]; d.Validator.Validator.Kind = "green-by-default" }, ErrInvalid},
		{"scenario missing check", func(d *Draft) { *d = typedDrafts()[4]; d.Scenario.Steps[1].Ref = "missing" }, ErrInvalid},
		{"case stale version", func(d *Draft) { *d = typedDrafts()[5]; d.Case.ExpectedVersion = 2 }, ErrStale},
		{"case invalid context", func(d *Draft) { *d = typedDrafts()[5]; d.Case.Context["bad key"] = d.Case.Context["invoice_id"] }, ErrInvalid},
		{"run missing check", func(d *Draft) { *d = runDraft(); d.Run.CheckID = "missing" }, ErrStale},
		{"run invalid environment", func(d *Draft) { *d = runDraft(); d.Run.Environment = "production" }, ErrInvalid},
		{"run missing required input", func(d *Draft) { *d = runDraft(); d.Run.Inputs = nil }, ErrInvalid},
		{"explanation unknown run", func(d *Draft) { *d = typedDrafts()[7]; d.Explanation.RunID = "missing" }, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			draft := typedDrafts()[0]
			tc.edit(&draft)
			if err := validateDraft(draft, snapshot, sources); !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestSourceEvidenceAndStrictJSON(t *testing.T) {
	_, _, sources := baseline()
	allowed, ok := sourceSet(sources)
	if !ok || !validateProvenance(cited(), allowed) {
		t.Fatal("valid source span rejected")
	}
	for _, citation := range []Provenance{
		{SourceID: "unknown", Start: 0, End: 1, Confidence: .9},
		{SourceID: "ticket", Start: -1, End: 1, Confidence: .9},
		{SourceID: "ticket", Start: 2, End: 2, Confidence: .9},
		{SourceID: "ticket", Start: 0, End: 10, Confidence: .9},
		{SourceID: "ticket", Start: 0, End: 9, Quote: "invoice 8", Confidence: .9},
		{SourceID: "ticket", Start: 0, End: 9, Confidence: -1},
		{SourceID: "ticket", Start: 0, End: 9, Confidence: 2},
		{SourceID: "ticket", Start: 0, End: 9, Confidence: math.NaN()},
	} {
		if validateProvenance([]Provenance{citation}, allowed) {
			t.Fatalf("unresolved citation accepted: %#v", citation)
		}
	}
	evidence := sourceEvidence(sources)
	if !validStoredSources(evidence) || evidence[0].Digest == "" {
		t.Fatalf("source metadata invalid: %#v", evidence)
	}
	evidence[0].Digest = "not-a-digest"
	if validStoredSources(evidence) {
		t.Fatal("invalid digest accepted")
	}
	for _, raw := range [][]byte{nil, []byte(`{"kind":"run-check","kind":"edit-case"}`), []byte(`{"kind":"run-check"} {}`), []byte(`{"kind":`), []byte(`{"kind":"edit-case","title":"x","case":{"caseId":"case-a","expectedVersion":3,"incidentRefs":["foreign"]},"sources":[]}`)} {
		var draft Draft
		if err := strictDecode(raw, &draft); !errors.Is(err, ErrInvalid) {
			t.Fatalf("bad model JSON accepted: %s", raw)
		}
	}
	var draft Draft
	valid, _ := json.Marshal(runDraft())
	if err := strictDecode(valid, &draft); err != nil || draft.Kind != KindRun {
		t.Fatalf("valid strict JSON rejected: %v", err)
	}
}

func TestBoundedCatalogAndProposalTampering(t *testing.T) {
	scope, snapshot, sources := baseline()
	if !validScope(scope) || !validSnapshot(snapshot) {
		t.Fatal("baseline invalid")
	}
	for _, tc := range []struct {
		name string
		edit func(*Snapshot)
	}{
		{"too many environments", func(s *Snapshot) { s.Environments = make([]string, 33) }},
		{"empty environment", func(s *Snapshot) { s.Environments = []string{""} }},
		{"too many resources", func(s *Snapshot) { s.Queries = make([]Resource, maxResources+1) }},
		{"duplicate resource", func(s *Snapshot) { s.Checks = append(s.Checks, s.Checks[0]) }},
		{"missing revision", func(s *Snapshot) { s.Checks[0].Revision = "" }},
		{"too many inputs", func(s *Snapshot) { s.Checks[0].Inputs = make([]detective.Input, 65) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := cloneSnapshot(snapshot)
			tc.edit(&copy)
			if validSnapshot(copy) {
				t.Fatal("invalid catalog accepted")
			}
		})
	}
	now := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	p := Proposal{ID: "p", Scope: scope, Basis: snapshot, Draft: runDraft(), Sources: sourceEvidence(sources), CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	p.Digest, _ = proposalDigest(p)
	if err := validateProposal(p, now); err != nil {
		t.Fatal(err)
	}
	p.Draft.Title = "Changed after review"
	if err := validateProposal(p, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("tampered proposal accepted: %v", err)
	}
	p.Digest, _ = proposalDigest(p)
	p.Sources[0].Length = 0
	if err := validateProposal(p, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid source metadata accepted: %v", err)
	}
	if err := strictDecode([]byte(strings.Repeat("x", maxDraftJSON+1)), &Draft{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversize model payload accepted: %v", err)
	}
}

func TestExplicitCountRejectsIncompleteTypedIDSet(t *testing.T) {
	_, snapshot, sources := baseline()
	values := []investigation.TypedValue{}
	for _, id := range []string{"1", "2", "3", "4", "5"} {
		values = append(values, investigation.NewIntegerValue(id))
	}
	set := apicontract.TypedValueSet{Values: values}
	draft := Draft{Kind: KindContext, Title: "Five order IDs", Context: map[string]ExtractedValue{"order_ids": {Value: detective.InputValue{Set: &set}, Provenance: cited()}}, Sources: cited()}
	if err := validateDraftWithExpectations(draft, snapshot, sources, map[string]int{"order_ids": 5}); err != nil {
		t.Fatalf("complete five-ID set rejected: %v", err)
	}
	set.Values = set.Values[:4]
	if err := validateDraftWithExpectations(draft, snapshot, sources, map[string]int{"order_ids": 5}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("four IDs presented as five accepted: %v", err)
	}
	if err := validateDraftWithExpectations(draft, snapshot, sources, map[string]int{"missing_ids": 5}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("omitted expected key accepted: %v", err)
	}
}

func TestAdditionalEvidenceAndCaseEditBoundaries(t *testing.T) {
	_, snapshot, sources := baseline()
	if _, ok := sourceSet(make([]Source, maxSources+1)); ok {
		t.Fatal("source count cap bypassed")
	}
	longSources := []Source{{ID: "one", Kind: "paste", Revision: "1", Text: strings.Repeat("a", maxText)}, {ID: "two", Kind: "paste", Revision: "1", Text: strings.Repeat("b", maxText)}, {ID: "three", Kind: "paste", Revision: "1", Text: "x"}}
	if _, ok := sourceSet(longSources); ok {
		t.Fatal("aggregate source byte cap bypassed")
	}
	evidence := sourceEvidence(sources)
	if validStoredSources(make([]SourceEvidence, maxSources+1)) {
		t.Fatal("stored source count cap bypassed")
	}
	evidence[0].Digest = strings.Repeat("z", 64)
	if validStoredSources(evidence) {
		t.Fatal("non-hex source digest accepted")
	}
	for _, edit := range []CaseEdit{
		{Title: strings.Repeat("x", 257)},
		{Findings: []detective.Finding{{ID: "bad", Text: " ", Status: "hypothesis"}}},
		{Status: "archived"},
	} {
		if validCaseEdit(edit) {
			t.Fatalf("invalid Case edit accepted: %#v", edit)
		}
	}
	if validExpectedCounts(map[string]int{"bad key": 1}) || validExpectedCounts(map[string]int{"good": 101}) || validExpectedCounts(map[string]int{"good": 0}) || validExpectedCounts(map[string]int{"ok": 1, "bad": -1}) {
		t.Fatal("invalid user-entered counts accepted")
	}
	largeCounts := map[string]int{}
	for i := 0; i < 65; i++ {
		largeCounts["id_"+strings.Repeat("x", i)] = 1
	}
	if validExpectedCounts(largeCounts) {
		t.Fatal("count catalog cap bypassed")
	}
	context := typedDrafts()[0]
	context.Context = map[string]ExtractedValue{}
	if err := validateDraft(context, snapshot, sources); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty extraction accepted: %v", err)
	}
	check := typedDrafts()[2]
	check.Check.Steps = []detective.Step{{ID: "child", Kind: "check", CheckID: "missing"}}
	if err := validateDraft(check, snapshot, sources); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing child Check accepted: %v", err)
	}
	check = typedDrafts()[2]
	check.Check.Steps[0].Validators = []detective.Validator{{ID: "child", Scope: "row", Kind: "check", Severity: "error", CheckID: "missing"}}
	if err := validateDraft(check, snapshot, sources); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing inline child Check accepted: %v", err)
	}
	if _, err := proposalDigest(Proposal{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unscoped proposal digest accepted: %v", err)
	}
	if inputCount(detective.InputValue{Scalar: &investigation.TypedValue{Type: investigation.ValueTypeString, Str: "x"}}) != 1 {
		t.Fatal("scalar cardinality changed")
	}
}
