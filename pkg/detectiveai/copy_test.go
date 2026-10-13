package detectiveai

import (
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/detective"
	"github.com/datatug/datatug-core/pkg/investigation"
)

func TestProposalCopiesOwnEveryNestedDraftKind(t *testing.T) {
	scope, snapshot, sources := baseline()
	for _, draft := range typedDrafts() {
		t.Run(draft.Kind, func(t *testing.T) {
			original := Proposal{ID: "p", Scope: scope, Basis: cloneSnapshot(snapshot), Draft: cloneDraft(draft), Sources: sourceEvidence(sources), ExpectedCounts: map[string]int{"invoice_id": 1}, CreatedAt: time.Now()}
			owned := cloneProposal(original)
			original.Basis.Queries[0].Inputs[0].Name = "changed"
			original.Sources[0].ID = "changed"
			original.ExpectedCounts["invoice_id"] = 2
			original.Draft.Sources[0].SourceID = "changed"
			if owned.Basis.Queries[0].Inputs[0].Name != "invoice_id" || owned.Sources[0].ID != "ticket" || owned.ExpectedCounts["invoice_id"] != 1 || owned.Draft.Sources[0].SourceID != "ticket" {
				t.Fatal("owned proposal changed with caller")
			}
			switch draft.Kind {
			case KindContext:
				item := original.Draft.Context["invoice_id"]
				item.Value.Scalar.Str = "99"
				item.Provenance[0].SourceID = "changed"
				if owned.Draft.Context["invoice_id"].Value.Scalar.Str != "7" || owned.Draft.Context["invoice_id"].Provenance[0].SourceID != "ticket" {
					t.Fatal("extracted context alias")
				}
			case KindQuery:
				original.Draft.Query.Inputs[0].Name = "changed"
				if owned.Draft.Query.Inputs[0].Name != "invoice_id" {
					t.Fatal("query input alias")
				}
			case KindCheck:
				original.Draft.Check.Steps[0].Validators[0].ID = "changed"
				if owned.Draft.Check.Steps[0].Validators[0].ID != "nonnull" {
					t.Fatal("nested Validator alias")
				}
			case KindValidator:
				original.Draft.Validator.Validator.ID = "changed"
				if owned.Draft.Validator.Validator.ID != "nonnull" {
					t.Fatal("published Validator alias")
				}
			case KindScenario:
				original.Draft.Scenario.Steps[0].Ref = "changed"
				if owned.Draft.Scenario.Steps[0].Ref != "invoice-list" {
					t.Fatal("Scenario step alias")
				}
			case KindCase:
				original.Draft.Case.Context["invoice_id"].Scalar.Str = "99"
				original.Draft.Case.Findings[0].Text = "changed"
				if owned.Draft.Case.Context["invoice_id"].Scalar.Str != "7" || owned.Draft.Case.Findings[0].Text != "Total mismatch" {
					t.Fatal("Case edit alias")
				}
			case KindRun:
				original.Draft.Run.Inputs["invoice_id"].Scalar.Str = "99"
				if owned.Draft.Run.Inputs["invoice_id"].Scalar.Str != "7" {
					t.Fatal("run input alias")
				}
			case KindExplain:
				original.Draft.Explanation.Summary = "changed"
				if owned.Draft.Explanation.Summary != "The observed row differed" {
					t.Fatal("explanation alias")
				}
			}
		})
	}
}

func TestCopySetRangeValidatorAndCaseReferences(t *testing.T) {
	set, err := apicontract.NewTypedValueSet(investigation.NewIntegerValue("1"), investigation.NewIntegerValue("2"))
	if err != nil {
		t.Fatal(err)
	}
	setValue := detective.InputValue{Set: &set}
	setCopy := cloneInput(setValue)
	setValue.Set.Values[0].Str = "changed"
	if setCopy.Set.Values[0].Str == "changed" {
		t.Fatal("set copy aliases original")
	}
	rangeValue := detective.InputValue{Range: &detective.ValueRange{Type: "range", Start: investigation.NewIntegerValue("1"), End: investigation.NewIntegerValue("9")}}
	rangeCopy := cloneInput(rangeValue)
	rangeValue.Range.End.Str = "changed"
	if rangeCopy.Range.End.Str != "9" {
		t.Fatal("range copy aliases original")
	}
	if cloneInputs(nil) != nil || cloneCounts(nil) != nil {
		t.Fatal("nil collections changed shape")
	}
	expected := investigation.NewIntegerValue("2")
	validator := detective.Validator{Expected: &expected, Bindings: []detective.Binding{{Input: "invoice_id", FromInput: "invoice_id"}}}
	validatorCopy := cloneValidator(validator)
	validator.Expected.Str = "changed"
	validator.Bindings[0].Input = "changed"
	if validatorCopy.Expected.Str != "2" || validatorCopy.Bindings[0].Input != "invoice_id" {
		t.Fatal("Validator copy aliases original")
	}
	caseEdit := Draft{Kind: KindCase, Case: &CaseEdit{Findings: []detective.Finding{{EvidenceRefs: []string{"e-1"}}}, IncidentRefs: []string{"incident-1"}}}
	caseCopy := cloneDraft(caseEdit)
	caseEdit.Case.Findings[0].EvidenceRefs[0] = "changed"
	caseEdit.Case.IncidentRefs[0] = "changed"
	if caseCopy.Case.Findings[0].EvidenceRefs[0] != "e-1" || caseCopy.Case.IncidentRefs[0] != "incident-1" {
		t.Fatal("Case references alias original")
	}
}
