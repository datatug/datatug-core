package detective

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateFindingFields(t *testing.T) {
	valid := Finding{ID: "finding-1", Text: "Fictional finding.", Status: "hypothesis", EvidenceRefs: []string{"e1"}}
	require.NoError(t, ValidateFindingFields(valid))

	cases := []struct {
		name    string
		finding Finding
	}{
		{"id", Finding{ID: "BAD_ID", Text: "t", Status: "hypothesis"}},
		{"text", Finding{ID: "finding-1", Status: "hypothesis"}},
		{"status", Finding{ID: "finding-1", Text: "t", Status: "maybe"}},
		{"empty-ref", Finding{ID: "finding-1", Text: "t", Status: "hypothesis", EvidenceRefs: []string{""}}},
		{"duplicate-ref", Finding{ID: "finding-1", Text: "t", Status: "hypothesis", EvidenceRefs: []string{"e1", "e1"}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			require.ErrorIs(t, ValidateFindingFields(testCase.finding), ErrInvalid)
		})
	}
}

// TestValidateCaseContentPreservesFindingLimits proves the narrow helper
// extraction did not change the ordinary native 200-finding bound or Case
// evidence resolution.
func TestValidateCaseContentPreservesFindingLimits(t *testing.T) {
	twoHundred := make([]Finding, 200)
	for index := range twoHundred {
		twoHundred[index] = Finding{
			ID:           fmt.Sprintf("finding-%03d", index),
			Text:         "Fictional finding.",
			Status:       "hypothesis",
			EvidenceRefs: []string{"e1"},
		}
	}
	base := Case{
		ID:       "case-1",
		Title:    "Case",
		Status:   "active",
		Context:  map[string]InputValue{},
		Evidence: []EvidenceRef{{ID: "e1", Kind: "external", Ref: "fictional"}},
		Findings: twoHundred,
	}
	require.NoError(t, validateCaseContent(base))

	over := base
	over.Findings = append(append([]Finding{}, twoHundred...), Finding{
		ID: "finding-200", Text: "Fictional finding.", Status: "hypothesis",
	})
	require.ErrorIs(t, validateCaseContent(over), ErrInvalid)

	dangling := base
	dangling.Findings = []Finding{{
		ID: "finding-dangling", Text: "Fictional finding.", Status: "confirmed",
		EvidenceRefs: []string{"missing"},
	}}
	require.ErrorIs(t, validateCaseContent(dangling), ErrInvalid)

	duplicate := base
	duplicate.Findings = []Finding{twoHundred[0], twoHundred[0]}
	require.ErrorIs(t, validateCaseContent(duplicate), ErrInvalid)
}
