package walkthrough

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// manifestShapeIssues runs only the raw manifest envelope. It never calls
// challenge.LoadManifest, so these cases prove the package's exact-case/null/
// unknown/type wrapper independently of native semantics.
func manifestShapeIssues(body string) []Issue {
	c := newChecker(true)
	root, issue := parseJSON([]byte(body))
	if issue != nil {
		return []Issue{*issue}
	}
	c.validateChallengeManifestShape(root, "/assets/0")
	return c.issues
}

func TestChallengeManifestShapeEnvelope(t *testing.T) {
	bodies := map[string]string{
		"engine-null":                   `{"engine":null}`,
		"fixtures-not-array":            `{"fixtureSuite":{"fixtures":null}}`,
		"fixture-not-object":            `{"fixtureSuite":{"fixtures":["x"]}}`,
		"tables-not-array":              `{"visibleSchema":{"tables":null}}`,
		"table-not-object":              `{"visibleSchema":{"tables":["x"]}}`,
		"columns-not-array":             `{"visibleSchema":{"tables":[{"columns":null}]}}`,
		"column-not-object":             `{"visibleSchema":{"tables":[{"columns":["x"]}]}}`,
		"parts-not-array":               `{"parts":null}`,
		"part-not-object":               `{"parts":["x"]}`,
		"sql-cases-not-array":           `{"parts":[{"sql":{"cases":null}}]}`,
		"sql-case-not-object":           `{"parts":[{"sql":{"cases":["x"]}}]}`,
		"oracle-kind-missing":           `{"parts":[{"sql":{"cases":[{"oracle":{}}]}}]}`,
		"oracle-kind-not-string":        `{"parts":[{"sql":{"cases":[{"oracle":{"kind":1}}]}}]}`,
		"oracle-kind-unknown":           `{"parts":[{"sql":{"cases":[{"oracle":{"kind":"other"}}]}}]}`,
		"result-column-not-object":      `{"parts":[{"sql":{"cases":[{"oracle":{"kind":"result","columns":["x"]}}]}}]}`,
		"result-rows-not-array":         `{"parts":[{"sql":{"cases":[{"oracle":{"kind":"result","rows":null}}]}}]}`,
		"result-row-not-array":          `{"parts":[{"sql":{"cases":[{"oracle":{"kind":"result","rows":["x"]}}]}}]}`,
		"final-table-not-object":        `{"parts":[{"sql":{"cases":[{"oracle":{"kind":"final-state","tables":["x"]}}]}}]}`,
		"invariants-not-array":          `{"parts":[{"sql":{"cases":[{"oracle":{"kind":"final-state","invariants":null}}]}}]}`,
		"invariant-not-object":          `{"parts":[{"sql":{"cases":[{"oracle":{"kind":"final-state","invariants":["x"]}}]}}]}`,
		"group-by-not-array":            `{"parts":[{"sql":{"cases":[{"oracle":{"kind":"final-state","invariants":[{"groupBy":null}]}}]}}]}`,
		"group-by-not-string":           `{"parts":[{"sql":{"cases":[{"oracle":{"kind":"final-state","invariants":[{"groupBy":[1]}]}}]}}]}`,
		"structured-field-not-object":   `{"parts":[{"structured":{"fields":["x"]}}]}`,
		"structured-options-not-array":  `{"parts":[{"structured":{"fields":[{"options":null}]}}]}`,
		"structured-option-not-object":  `{"parts":[{"structured":{"fields":[{"options":["x"]}]}}]}`,
		"selections-not-object":         `{"parts":[{"structured":{"expected":{"selections":null}}}]}`,
		"selection-not-array":           `{"parts":[{"structured":{"expected":{"selections":{"x":"y"}}}}]}`,
		"selection-item-not-string":     `{"parts":[{"structured":{"expected":{"selections":{"x":[1]}}}}]}`,
		"check-query-id-not-string":     `{"parts":[{"checkDefinition":{"queryIds":[1]}}]}`,
		"check-cases-not-array":         `{"parts":[{"checkDefinition":{"cases":null}}]}`,
		"check-case-not-object":         `{"parts":[{"checkDefinition":{"cases":["x"]}}]}`,
		"hint-not-object":               `{"hints":["x"]}`,
		"reference-solution-not-object": `{"referenceSolutions":["x"]}`,
		"validator-bindings-not-array":  `{"parts":[{"checkDefinition":{"expectation":{"bindings":null}}}]}`,
		"validator-binding-not-object":  `{"parts":[{"checkDefinition":{"expectation":{"bindings":["x"]}}}]}`,
	}
	for name, body := range bodies {
		body := body
		t.Run(name, func(t *testing.T) {
			require.NotEmpty(t, manifestShapeIssues(body))
		})
	}
	// Omission is not the wrapper's business: native validation owns required
	// fields, so an empty object passes the raw envelope and is refused later.
	require.Empty(t, manifestShapeIssues(`{}`))
	require.Empty(t, manifestShapeIssues(`{"parts":[{"structured":{"expected":{}}}]}`))
}

func TestChallengeManifestShapeAcceptsCompleteUnions(t *testing.T) {
	// The corpus full and manifest-unions vectors prove admission. This case
	// also exercises a valid validator with a nonempty binding list through the
	// shape wrapper, which the native check-definition expectation rejects
	// later and is therefore not a corpus positive.
	body := `{"parts":[{"checkDefinition":{"expectation":{
		"id":"expectation","scope":"row","kind":"non-null","severity":"error",
		"field":"status","bindings":[{"input":"status","fromInput":"status","part":"start","fromColumn":"column"}],
		"expected":{"type":"string","value":"x"}}}}]}`
	require.Empty(t, manifestShapeIssues(body))
}
