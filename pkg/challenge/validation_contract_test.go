package challenge

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/detective"
	"github.com/stretchr/testify/require"
)

func cloneManifest(t *testing.T, source Manifest) Manifest {
	t.Helper()
	encoded, err := json.Marshal(source)
	require.NoError(t, err)
	var cloned Manifest
	require.NoError(t, json.Unmarshal(encoded, &cloned))
	return cloned
}

func TestLoadManifestRefusesUnusableBytesBeforeDecoding(t *testing.T) {
	for name, data := range map[string][]byte{
		"empty":      nil,
		"oversized":  make([]byte, MaxDefinitionBytes+1),
		"malformed":  []byte(`{"schemaVersion":`),
		"not object": []byte(`[]`),
	} {
		t.Run(name, func(t *testing.T) {
			manifest, err := LoadManifest(data)
			refusal := requireRefused(t, err, CodeInvalidManifest)
			require.Equal(t, "manifest", refusal.Field)
			require.Empty(t, manifest.Parts)
		})
	}

	manifest, err := LoadManifest([]byte(`{"schemaVersion":1}`))
	require.Error(t, err)
	require.Equal(t, CodeInvalidManifest, ErrorCode(err))
	require.Empty(t, manifest.Parts)
}

func TestFinalizeOpenedRefusesManifestThatCannotBeCanonicalized(t *testing.T) {
	opened := openGolden(t, "deduplicate-customers")
	manifest := opened.Manifest
	sqlPart := findPart(t, manifest, "delete-duplicates").SQL
	require.NotNil(t, sqlPart)
	// An unknown kind makes the closed oracle union fail its own encoder. This
	// is a genuine canonicalization failure after manifest and fixture
	// validation, so finalizeOpened must refuse it without producing an Opened.
	sqlPart.Cases[0].Oracle = Oracle{Kind: "bogus"}
	_, marshalErr := json.Marshal(manifest)
	require.Error(t, marshalErr, "the invalid oracle union must fail at the encoding boundary")

	finalized, err := finalizeOpened(manifest, opened.fixtures)
	refusal := requireRefused(t, err, CodeInvalidManifest)
	require.Equal(t, "manifest", refusal.Field)
	require.Contains(t, err.Error(), "cannot canonicalize")
	require.Nil(t, finalized)
}

func TestManifestValidateRefusesEveryStructuralAndRangeRule(t *testing.T) {
	opened := openGolden(t, "deduplicate-customers")
	base := opened.Manifest
	sqlPart := findPart(t, base, "delete-duplicates")

	tests := []struct {
		name        string
		mutate      func(*Manifest)
		wantCode    Code
		wantField   string
		wantMessage string
	}{
		{
			name:        "unsupported grading semantics",
			mutate:      func(m *Manifest) { m.GradingVersion = "challenge-grading-v2" },
			wantCode:    CodeUnsupportedGradingVersion,
			wantField:   "gradingVersion",
			wantMessage: "supported semantics are",
		},
		{
			name:        "challenge id is not a slug",
			mutate:      func(m *Manifest) { m.ChallengeID = "Upper" },
			wantField:   "challengeId",
			wantMessage: "lowercase slug",
		},
		{
			name:        "objective is empty",
			mutate:      func(m *Manifest) { m.Objective = "" },
			wantField:   "objective",
			wantMessage: "1..4000",
		},
		{
			name:        "difficulty is unknown",
			mutate:      func(m *Manifest) { m.Difficulty = "expert" },
			wantField:   "difficulty",
			wantMessage: "intro, intermediate or advanced",
		},
		{
			name: "too many hints",
			mutate: func(m *Manifest) {
				m.Hints = make([]Hint, maxHints+1)
				for index := range m.Hints {
					m.Hints[index] = Hint{ID: "hint", Text: "h"}
				}
			},
			wantField:   "hints",
			wantMessage: "at most",
		},
		{
			name: "too many reference solutions",
			mutate: func(m *Manifest) {
				m.ReferenceSolutions = make([]ReferenceSolution, maxReferenceSols+1)
			},
			wantField:   "referenceSolutions",
			wantMessage: "at most",
		},
		{
			name:        "fixture suite id is not a slug",
			mutate:      func(m *Manifest) { m.FixtureSuite.ID = "Bad_ID" },
			wantField:   "fixtureSuite.id",
			wantMessage: "lowercase slug",
		},
		{
			name:        "fixture suite has no fixtures",
			mutate:      func(m *Manifest) { m.FixtureSuite.Fixtures = nil },
			wantField:   "fixtureSuite.fixtures",
			wantMessage: "must contain between",
		},
		{
			name: "fixture ids are duplicated",
			mutate: func(m *Manifest) {
				m.FixtureSuite.Fixtures = append(m.FixtureSuite.Fixtures, m.FixtureSuite.Fixtures[0])
			},
			wantField:   "fixtureSuite.fixtures",
			wantMessage: "valid and unique",
		},
		{
			name:        "fixture label is empty",
			mutate:      func(m *Manifest) { m.FixtureSuite.Fixtures[0].Label = "" },
			wantField:   "fixtureSuite.fixtures.label",
			wantMessage: "1..200",
		},
		{
			name:        "fixture bytes are out of range",
			mutate:      func(m *Manifest) { m.FixtureSuite.Fixtures[0].Bytes = 0 },
			wantField:   "fixtureSuite.fixtures.bytes",
			wantMessage: "between 1 and",
		},
		{
			name:        "visible schema is empty",
			mutate:      func(m *Manifest) { m.VisibleSchema.Tables = nil },
			wantField:   "visibleSchema.tables",
			wantMessage: "must contain between",
		},
		{
			name: "visible schema duplicates a table",
			mutate: func(m *Manifest) {
				m.VisibleSchema.Tables = append(m.VisibleSchema.Tables, m.VisibleSchema.Tables[0])
			},
			wantField:   "visibleSchema.tables",
			wantMessage: "valid and unique",
		},
		{
			name: "visible schema table has no columns",
			mutate: func(m *Manifest) {
				m.VisibleSchema.Tables[0].Columns = nil
			},
			wantField:   "visibleSchema.tables.columns",
			wantMessage: "1..",
		},
		{
			name:        "part title is empty",
			mutate:      func(m *Manifest) { m.Parts[0].Title = "" },
			wantField:   "parts",
			wantMessage: "1..200 character title",
		},
		{
			name:        "part answer kind is unknown",
			mutate:      func(m *Manifest) { m.Parts[0].AnswerKind = "free-text" },
			wantField:   "parts." + sqlPart.ID + ".answerKind",
			wantMessage: "must be sql, structured or check-definition",
		},
		{
			name: "part has no answer body",
			mutate: func(m *Manifest) {
				m.Parts[0].SQL = nil
				m.Parts[0].Structured = nil
				m.Parts[0].CheckDefinition = nil
			},
			wantField:   "parts." + sqlPart.ID,
			wantMessage: "exactly one answer body",
		},
		{
			name: "sql part carries a structured body",
			mutate: func(m *Manifest) {
				m.Parts[0].SQL = nil
				m.Parts[0].Structured = &StructuredPart{}
			},
			wantField:   "parts." + sqlPart.ID,
			wantMessage: "requires only the sql body",
		},
		{
			name: "structured part carries a sql body",
			mutate: func(m *Manifest) {
				m.Parts[0].AnswerKind = AnswerStructured
				m.Parts[0].Structured = nil
				m.Parts[0].CheckDefinition = nil
			},
			wantField:   "parts." + sqlPart.ID,
			wantMessage: "requires only the structured body",
		},
		{
			name: "check-definition part carries a sql body",
			mutate: func(m *Manifest) {
				m.Parts[0].AnswerKind = AnswerCheckDefinition
				m.Parts[0].CheckDefinition = nil
			},
			wantField:   "parts." + sqlPart.ID,
			wantMessage: "requires only the checkDefinition body",
		},
		{
			name:        "sql part has no cases",
			mutate:      func(m *Manifest) { m.Parts[0].SQL.Cases = nil },
			wantField:   "sql.cases",
			wantMessage: "must contain between",
		},
		{
			name: "sql case oracle is invalid",
			mutate: func(m *Manifest) {
				m.Parts[0].SQL.Cases[0].Oracle = Oracle{Kind: "bogus"}
			},
			wantField:   "sql.cases." + firstSQLCaseFixture(t, base, sqlPart.ID) + ".oracle",
			wantMessage: "unknown oracle kind",
		},
		{
			name: "fixture hash is not lowercase hexadecimal",
			mutate: func(m *Manifest) {
				m.FixtureSuite.Fixtures[0].SHA256 = strings.Repeat("g", 64)
			},
			wantField:   "fixtureSuite.fixtures.sha256",
			wantMessage: "64 lowercase hexadecimal",
		},
		{
			name:        "title has surrounding whitespace",
			mutate:      func(m *Manifest) { m.Title = "Deduplicate customers " },
			wantField:   "title",
			wantMessage: "without surrounding whitespace",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := cloneManifest(t, base)
			test.mutate(&manifest)
			wantCode := test.wantCode
			if wantCode == "" {
				wantCode = CodeInvalidManifest
			}
			err := manifest.Validate()
			refusal := requireRefused(t, err, wantCode)
			require.Equal(t, test.wantField, refusal.Field)
			require.Contains(t, refusal.Message, test.wantMessage)
		})
	}
}

func firstSQLCaseFixture(t *testing.T, manifest Manifest, partID string) string {
	t.Helper()
	part := findPart(t, manifest, partID)
	require.NotEmpty(t, part.SQL.Cases)
	return part.SQL.Cases[0].Fixture
}

func TestPartValidateWalksItsOwnSQLCases(t *testing.T) {
	opened := openGolden(t, "deduplicate-customers")
	part := findPart(t, opened.Manifest, "delete-duplicates")
	require.NoError(t, part.Validate())

	invalid := Part{ID: "Bad", Title: "Title", AnswerKind: AnswerSQL}
	_ = requireRefused(t, invalid.Validate(), CodeInvalidManifest)

	for name, part := range map[string]Part{
		"unknown answer kind": {ID: "p", Title: "P", AnswerKind: "free-text"},
		"no answer body":      {ID: "p", Title: "P", AnswerKind: AnswerSQL},
		"sql with structured body": {
			ID: "p", Title: "P", AnswerKind: AnswerSQL, Structured: &StructuredPart{},
		},
		"structured with sql body": {
			ID: "p", Title: "P", AnswerKind: AnswerStructured, SQL: &SQLPart{},
		},
		"check-definition with sql body": {
			ID: "p", Title: "P", AnswerKind: AnswerCheckDefinition, SQL: &SQLPart{},
		},
		"sql without cases": {
			ID: "p", Title: "P", AnswerKind: AnswerSQL, SQL: &SQLPart{},
		},
		"sql case oracle is invalid": {
			ID: "p", Title: "P", AnswerKind: AnswerSQL,
			SQL: &SQLPart{Cases: []SQLCase{{Fixture: "f", Oracle: Oracle{Kind: "bogus"}}}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_ = requireRefused(t, part.Validate(), CodeInvalidManifest)
		})
	}
}

func TestReferenceAnswersMustMatchTheDeclaredPart(t *testing.T) {
	dedup := openGolden(t, "deduplicate-customers").Manifest
	sqlPart := findPart(t, dedup, "delete-duplicates")
	checkPart := findPart(t, dedup, "prevention-check")

	t.Run("no answer body", func(t *testing.T) {
		manifest := cloneManifest(t, dedup)
		manifest.ReferenceSolutions = []ReferenceSolution{{PartID: sqlPart.ID}}
		refusal := requireRefused(t, manifest.Validate(), CodeInvalidManifest)
		require.Contains(t, refusal.Message, "exactly one reference answer")
	})

	t.Run("structured reference carries sql", func(t *testing.T) {
		retry := openGolden(t, "retry-evidence").Manifest
		structured := findPart(t, retry, "evidence-pairs")
		manifest := cloneManifest(t, retry)
		manifest.ReferenceSolutions = []ReferenceSolution{{
			PartID: structured.ID,
			Answer: ReferenceAnswer{SQL: "SELECT 1"},
		}}
		refusal := requireRefused(t, manifest.Validate(), CodeInvalidManifest)
		require.Contains(t, refusal.Message, "must be structured")
	})

	t.Run("structured reference differs from expected", func(t *testing.T) {
		retry := openGolden(t, "retry-evidence").Manifest
		structured := findPart(t, retry, "evidence-pairs")
		manifest := cloneManifest(t, retry)
		manifest.ReferenceSolutions = []ReferenceSolution{{
			PartID: structured.ID,
			Answer: ReferenceAnswer{Structured: &StructuredAnswer{
				Selections: map[string][]string{"evidence-pairs": {"r1-a1-trace", "r2-a2-a3-trace"}},
			}},
		}}
		refusal := requireRefused(t, manifest.Validate(), CodeInvalidManifest)
		require.Contains(t, refusal.Message, "must equal the declared expected selection")
	})

	t.Run("check-definition reference carries sql", func(t *testing.T) {
		manifest := cloneManifest(t, dedup)
		manifest.ReferenceSolutions = []ReferenceSolution{{
			PartID: checkPart.ID,
			Answer: ReferenceAnswer{SQL: "SELECT 1"},
		}}
		refusal := requireRefused(t, manifest.Validate(), CodeInvalidManifest)
		require.Contains(t, refusal.Message, "must be a Check definition")
	})

	t.Run("check-definition reference names an undeclared query", func(t *testing.T) {
		manifest := cloneManifest(t, dedup)
		manifest.ReferenceSolutions = []ReferenceSolution{{
			PartID: checkPart.ID,
			Answer: ReferenceAnswer{CheckDefinition: &CheckDefinitionAnswer{
				QueryID:     "queries/elsewhere",
				Expectation: checkPart.CheckDefinition.Expectation,
			}},
		}}
		refusal := requireRefused(t, manifest.Validate(), CodeInvalidManifest)
		require.Contains(t, refusal.Message, "declared saved queries")
	})
}

func TestCheckDefinitionValidationCoversClosedReferences(t *testing.T) {
	part := checkPart()
	definition := part.CheckDefinition

	for name, mutate := range map[string]func(*CheckDefinitionPart){
		"no saved queries": func(definition *CheckDefinitionPart) { definition.QueryIDs = nil },
		"too many saved queries": func(definition *CheckDefinitionPart) {
			definition.QueryIDs = make([]string, maxQueryIDs+1)
			for index := range definition.QueryIDs {
				definition.QueryIDs[index] = "queries/q" + strings.Repeat("x", index+1)
			}
		},
		"no cases": func(definition *CheckDefinitionPart) { definition.Cases = nil },
		"duplicate fixture": func(definition *CheckDefinitionPart) {
			definition.Cases = []CheckCase{
				{Fixture: "broken", ExpectedOutcome: OutcomeFail},
				{Fixture: "broken", ExpectedOutcome: OutcomeFail},
			}
		},
		"invalid fixture id": func(definition *CheckDefinitionPart) {
			definition.Cases = []CheckCase{{Fixture: "Bad", ExpectedOutcome: OutcomeFail}}
		},
		"invalid shared validator": func(definition *CheckDefinitionPart) {
			definition.Expectation = detective.Validator{
				ID: "no-duplicates", Scope: "set", Kind: "row-count", Severity: "critical",
			}
		},
		"row count exceeds the result limit": func(definition *CheckDefinitionPart) {
			definition.Expectation = integerExpectation("1001")
		},
	} {
		t.Run(name, func(t *testing.T) {
			mutated := *definition
			mutated.QueryIDs = append([]string(nil), definition.QueryIDs...)
			mutated.Cases = append([]CheckCase(nil), definition.Cases...)
			mutate(&mutated)
			err := mutated.validate(map[string]bool{"broken": true, "repaired": true})
			_ = requireRefused(t, err, CodeInvalidManifest)
		})
	}

	t.Run("answer validation", func(t *testing.T) {
		correct := CheckDefinitionAnswer{QueryID: "queries/duplicate-emails", Expectation: definition.Expectation}
		require.NoError(t, validateCheckDefinitionAnswer(*definition, correct))

		unknown := correct
		unknown.QueryID = "queries/elsewhere"
		require.ErrorContains(t, validateCheckDefinitionAnswer(*definition, unknown), "declared saved queries")

		unsupported := correct
		unsupported.Expectation = detective.Validator{
			ID: "unique-email", Scope: "set", Kind: "unique", Severity: "critical", Field: "email",
		}
		require.ErrorContains(t, validateCheckDefinitionAnswer(*definition, unsupported), "row-count")

		different := correct
		different.Expectation = integerExpectation("3")
		require.ErrorContains(t, validateCheckDefinitionAnswer(*definition, different), "declared expected expectation")
	})

	t.Run("observation without rows", func(t *testing.T) {
		answer := CheckDefinitionAnswer{QueryID: "queries/duplicate-emails", Expectation: definition.Expectation}
		result := EvaluateCheckDefinition(*definition, answer, map[string]Observation{
			"broken":   {},
			"repaired": {},
		})
		require.Equal(t, OutcomeUnknown, result.Outcome)
		require.Equal(t, StatusError, result.ExecutionStatus)
		require.Equal(t, ProblemMissingObservation, result.Problem)
		require.Contains(t, result.Message, "returned no result rows")
	})
}

func TestPathHelpersRefuseNonSlugIdentifiers(t *testing.T) {
	_, err := AssetPath("a_b", 1)
	_ = requireRefused(t, err, CodeInvalidManifest)

	opened := openGolden(t, "deduplicate-customers")
	checkDefinition := findPart(t, opened.Manifest, "prevention-check").CheckDefinition
	for name, queryIDs := range map[string][]string{
		"empty reference":       {""},
		"oversized reference":   {strings.Repeat("q", 513)},
		"unsupported character": {"queries/bad id"},
	} {
		t.Run(name, func(t *testing.T) {
			mutated := *checkDefinition
			mutated.QueryIDs = queryIDs
			err := mutated.validate(map[string]bool{"broken": true, "repaired": true, "nonsequential": true, "repaired-mutated": true})
			_ = requireRefused(t, err, CodeInvalidManifest)
		})
	}

	for name, tableName := range map[string]string{
		"empty name":        "",
		"oversized name":    strings.Repeat("t", 129),
		"path separator":    "customers/archive",
		"control character": "customers\narchive",
	} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, TableExpectation{Name: tableName, Columns: []apicontract.Column{{Name: "id", Type: "integer"}}}.Validate())
		})
	}
}

func TestErrorsClassifyOnlyChallengeRefusals(t *testing.T) {
	require.Equal(t, Code(""), ErrorCode(errors.New("plain failure")))
	require.Equal(t, Code(""), ErrorCode(nil))

	withoutField := &Error{Code: CodeInvalidAnswer, Message: "answer cannot be graded"}
	require.Equal(t, "challenge: invalid_answer: answer cannot be graded", withoutField.Error())
	require.True(t, withoutField.Recoverable())
}

func TestOpenedFixtureHelpersReportAnAbsentID(t *testing.T) {
	opened := openGolden(t, "deduplicate-customers")
	data, ok := opened.Fixture("absent")
	require.False(t, ok)
	require.Nil(t, data)
	require.False(t, opened.FixtureBytesEqual("absent", nil))
}
