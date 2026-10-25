package challenge

import (
	"testing"
	"unicode/utf8"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/stretchr/testify/require"
)

// sharedFeedbackVectors is the compact, language-neutral golden for the
// bounded metadata attached to a failed unordered row comparison. Its inputs
// reuse the TypedValue wire shape; expected examples are literal strings so a
// browser implementation can verify the same truncation boundary.
type sharedFeedbackVectors struct {
	GradingVersion string                 `json:"gradingVersion"`
	Vectors        []sharedFeedbackVector `json:"vectors"`
}

type sharedFeedbackVector struct {
	ID          string                 `json:"id"`
	Description string                 `json:"description"`
	Options     RowOptions             `json:"options"`
	Expected    sharedGradingRecordset `json:"expected"`
	Observed    sharedGradingRecordset `json:"observed"`
	Want        sharedFeedbackWant     `json:"want"`
}

type sharedFeedbackWant struct {
	Match      bool     `json:"match"`
	Problem    string   `json:"problem,omitempty"`
	Missing    int      `json:"missing,omitempty"`
	Unexpected int      `json:"unexpected,omitempty"`
	Examples   []string `json:"examples,omitempty"`
}

func TestSharedFeedbackVectorsMatchGoComparison(t *testing.T) {
	var vectors sharedFeedbackVectors
	require.NoError(t, apicontract.DecodeStrict(readTestFile(t, "testdata/feedback-vectors.json"), &vectors))
	require.Equal(t, GradingVersion, vectors.GradingVersion)
	require.NotEmpty(t, vectors.Vectors)

	seen := map[string]bool{}
	for _, vector := range vectors.Vectors {
		vector := vector
		require.NotEmpty(t, vector.ID)
		require.False(t, seen[vector.ID], "duplicate vector id %q", vector.ID)
		seen[vector.ID] = true
		require.NotEmpty(t, vector.Description)
		t.Run(vector.ID, func(t *testing.T) {
			diff := CompareTypedRows(
				vector.Expected.Columns, vector.Observed.Columns,
				vector.Expected.typedRows(), vector.Observed.typedRows(),
				vector.Options,
			)
			require.Equal(t, vector.Want.Match, diff.Match, diff.Message)
			require.Equal(t, vector.Want.Problem, diff.Problem)
			require.Equal(t, vector.Want.Missing, diff.Missing)
			require.Equal(t, vector.Want.Unexpected, diff.Unexpected)
			require.Equal(t, vector.Want.Examples, diff.Examples)
			require.LessOrEqual(t, len(diff.Examples), maxFeedbackExamples)
			for _, example := range diff.Examples {
				require.True(t, utf8.ValidString(example), "example must be valid UTF-8")
				require.NotContains(t, example, "\uFFFD")
				require.LessOrEqual(t, utf8.RuneCountInString(example), maxFeedbackExampleRunes+1)
			}
		})
	}
}
