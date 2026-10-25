package challenge

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/stretchr/testify/require"
)

func feedbackPart(columns []apicontract.Column, expectedRows [][]apicontract.TypedValue, ordering string) SQLPart {
	return SQLPart{Cases: []SQLCase{{
		Fixture: "fixture",
		Oracle: Oracle{Kind: OracleResult, Result: &ResultOracle{
			Columns:  columns,
			Rows:     expectedRows,
			Ordering: ordering,
		}},
	}}}
}

func gradeFeedbackCase(t *testing.T, columns []apicontract.Column, expectedRows, observedRows [][]apicontract.TypedValue, ordering string) SQLPartResult {
	t.Helper()
	return GradeSQLPart(
		feedbackPart(columns, expectedRows, ordering),
		"SELECT value FROM t",
		map[string]Observation{"fixture": {Rows: &apicontract.Recordset{
			Columns: columns,
			Rows:    observedRows,
		}}},
	)
}

func TestGradeSQLPartCarriesBoundedRowFeedbackThroughJSON(t *testing.T) {
	columns := columnsOf("integer", "string")
	result := gradeFeedbackCase(t,
		columns,
		[][]apicontract.TypedValue{row(integer("1"), text("a"))},
		[][]apicontract.TypedValue{row(integer("2"), text("a"))},
		OrderUnordered,
	)
	require.Equal(t, OutcomeFail, result.Outcome)
	require.Equal(t, StatusCompleted, result.ExecutionStatus)
	require.Equal(t, ProblemRowContent, result.Problem)
	require.Len(t, result.Cases, 1)

	item := result.Cases[0]
	require.Equal(t, OutcomeFail, item.Outcome)
	require.Equal(t, 1, item.Missing)
	require.Equal(t, 1, item.Unexpected)
	require.Equal(t, []string{
		`missing: [{"type":"integer","value":"1"},{"type":"string","value":"a"}]`,
		`unexpected: [{"type":"integer","value":"2"},{"type":"string","value":"a"}]`,
	}, item.Examples)

	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	var wire struct {
		Cases []struct {
			Missing    int      `json:"missing"`
			Unexpected int      `json:"unexpected"`
			Examples   []string `json:"examples"`
		} `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(encoded, &wire))
	require.Len(t, wire.Cases, 1)
	require.Equal(t, 1, wire.Cases[0].Missing)
	require.Equal(t, 1, wire.Cases[0].Unexpected)
	require.Equal(t, item.Examples, wire.Cases[0].Examples)
}

func TestGradeSQLPartOmitsFeedbackMetadataOnPass(t *testing.T) {
	columns := columnsOf("integer")
	result := gradeFeedbackCase(t,
		columns,
		[][]apicontract.TypedValue{row(integer("1"))},
		[][]apicontract.TypedValue{row(integer("1"))},
		OrderUnordered,
	)
	require.Equal(t, OutcomePass, result.Outcome)

	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	var wire struct {
		Cases []map[string]json.RawMessage `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(encoded, &wire))
	require.Len(t, wire.Cases, 1)
	require.Len(t, wire.Cases[0], 3, "a pass case keeps the previous fixture/outcome/status shape")
	for _, absent := range []string{"missing", "unexpected", "examples"} {
		require.NotContains(t, wire.Cases[0], absent)
	}
}

func TestGradeSQLPartOmitsZeroAndNonRowFeedback(t *testing.T) {
	encodedCase := func(t *testing.T, result SQLPartResult) map[string]json.RawMessage {
		t.Helper()
		encoded, err := json.Marshal(result)
		require.NoError(t, err)
		var wire struct {
			Cases []map[string]json.RawMessage `json:"cases"`
		}
		require.NoError(t, json.Unmarshal(encoded, &wire))
		require.Len(t, wire.Cases, 1)
		return wire.Cases[0]
	}

	t.Run("a zero missing count stays omitted", func(t *testing.T) {
		columns := columnsOf("integer")
		result := gradeFeedbackCase(t,
			columns,
			[][]apicontract.TypedValue{},
			[][]apicontract.TypedValue{row(integer("2"))},
			OrderUnordered,
		)
		require.Equal(t, OutcomeFail, result.Outcome)
		require.Zero(t, result.Cases[0].Missing)
		require.Equal(t, 1, result.Cases[0].Unexpected)
		item := encodedCase(t, result)
		require.NotContains(t, item, "missing")
		require.Contains(t, item, "unexpected")
		require.Contains(t, item, "examples")
	})

	t.Run("a column-count failure has no row metadata", func(t *testing.T) {
		result := GradeSQLPart(
			feedbackPart(columnsOf("integer"), [][]apicontract.TypedValue{row(integer("1"))}, OrderUnordered),
			"SELECT value FROM t",
			map[string]Observation{"fixture": {Rows: &apicontract.Recordset{
				Columns: columnsOf("integer", "integer"),
				Rows:    [][]apicontract.TypedValue{row(integer("1"), integer("2"))},
			}}},
		)
		require.Equal(t, OutcomeFail, result.Outcome)
		require.Equal(t, ProblemColumnCount, result.Cases[0].Problem)
		item := encodedCase(t, result)
		for _, absent := range []string{"missing", "unexpected", "examples"} {
			require.NotContains(t, item, absent)
		}
	})
}

func TestGradeSQLPartBoundsLongAndAstralExamples(t *testing.T) {
	columns := columnsOf("string")

	t.Run("astral characters are counted as code points", func(t *testing.T) {
		long := strings.Repeat("😀", 300)
		result := gradeFeedbackCase(t,
			columns,
			[][]apicontract.TypedValue{row(text(long))},
			[][]apicontract.TypedValue{row(text("different"))},
			OrderUnordered,
		)
		require.Equal(t, OutcomeFail, result.Outcome)
		require.Len(t, result.Cases[0].Examples, 2)
		example := result.Cases[0].Examples[0]
		require.True(t, strings.HasPrefix(example, "missing: "))
		require.True(t, strings.HasSuffix(example, feedbackEllipsis))
		require.True(t, utf8.ValidString(example))
		require.NotContains(t, example, "\uFFFD")
		require.Equal(t, maxFeedbackExampleRunes+1, utf8.RuneCountInString(example))

		encoded, err := json.Marshal(result)
		require.NoError(t, err)
		require.True(t, utf8.Valid(encoded))
		var roundTrip SQLPartResult
		require.NoError(t, json.Unmarshal(encoded, &roundTrip))
		require.Equal(t, result.Cases[0].Examples, roundTrip.Cases[0].Examples)
	})

	t.Run("a huge BMP cell is truncated", func(t *testing.T) {
		long := strings.Repeat("x", 5_000)
		result := gradeFeedbackCase(t,
			columns,
			[][]apicontract.TypedValue{row(text(long))},
			[][]apicontract.TypedValue{row(text("y"))},
			OrderUnordered,
		)
		require.Equal(t, OutcomeFail, result.Outcome)
		require.Len(t, result.Cases[0].Examples, 2)
		example := result.Cases[0].Examples[0]
		require.True(t, strings.HasSuffix(example, feedbackEllipsis))
		require.True(t, utf8.ValidString(example))
		require.Equal(t, maxFeedbackExampleRunes+1, utf8.RuneCountInString(example))
		require.LessOrEqual(t, len(example), maxFeedbackExampleRunes+len(feedbackEllipsis))
	})
}

func TestGradeSQLPartPreservesDuplicateAndNullFeedback(t *testing.T) {
	t.Run("duplicate multiplicity", func(t *testing.T) {
		columns := columnsOf("integer")
		expected := [][]apicontract.TypedValue{row(integer("1")), row(integer("1"))}
		require.Equal(t, OutcomePass, gradeFeedbackCase(t, columns, expected, expected, OrderUnordered).Outcome)

		result := gradeFeedbackCase(t,
			columns,
			expected,
			[][]apicontract.TypedValue{row(integer("1")), row(integer("2"))},
			OrderUnordered,
		)
		require.Equal(t, OutcomeFail, result.Outcome, "duplicate multiplicity stays significant")
		item := result.Cases[0]
		require.Equal(t, 1, item.Missing)
		require.Equal(t, 1, item.Unexpected)
		require.Equal(t, []string{
			`missing: [{"type":"integer","value":"1"}]`,
			`unexpected: [{"type":"integer","value":"2"}]`,
		}, item.Examples)
	})

	t.Run("NULL differs from an empty string", func(t *testing.T) {
		columns := columnsOf("string")
		nullRows := [][]apicontract.TypedValue{row(null())}
		require.Equal(t, OutcomePass, gradeFeedbackCase(t, columns, nullRows, nullRows, OrderUnordered).Outcome)

		result := gradeFeedbackCase(t,
			columns,
			nullRows,
			[][]apicontract.TypedValue{row(text(""))},
			OrderUnordered,
		)
		require.Equal(t, OutcomeFail, result.Outcome, "NULL never becomes an empty string")
		item := result.Cases[0]
		require.Equal(t, 1, item.Missing)
		require.Equal(t, 1, item.Unexpected)
		require.Contains(t, item.Examples[0], `"type":"null","value":null`)
		require.Contains(t, item.Examples[1], `"type":"string","value":""`)
	})
}

func TestGradeSQLPartCopiesExamplesBetweenResults(t *testing.T) {
	columns := columnsOf("integer")
	expected := [][]apicontract.TypedValue{row(integer("1"))}
	observed := [][]apicontract.TypedValue{row(integer("2"))}

	first := gradeFeedbackCase(t, columns, expected, observed, OrderUnordered)
	second := gradeFeedbackCase(t, columns, expected, observed, OrderUnordered)
	require.Equal(t, second.Cases[0].Examples, first.Cases[0].Examples)

	first.Cases[0].Examples[0] = "mutated"
	require.NotEqual(t, first.Cases[0].Examples, second.Cases[0].Examples)

	third := gradeFeedbackCase(t, columns, expected, observed, OrderUnordered)
	require.NotEqual(t, first.Cases[0].Examples, third.Cases[0].Examples)
	require.Equal(t, second.Cases[0].Examples, third.Cases[0].Examples)
}

func TestOrderedComparisonKeepsFirstMismatchFeedback(t *testing.T) {
	columns := columnsOf("integer")
	diff := CompareTypedRows(
		columns,
		columns,
		[][]apicontract.TypedValue{row(integer("1")), row(integer("2"))},
		[][]apicontract.TypedValue{row(integer("1")), row(integer("3"))},
		RowOptions{Ordering: OrderOrdered},
	)
	require.False(t, diff.Match)
	require.Equal(t, ProblemRowOrder, diff.Problem)
	require.Equal(t, `row 2 column 1 differs: expected {"type":"integer","value":"2"}, observed {"type":"integer","value":"3"}`, diff.Message)
	require.Zero(t, diff.Missing)
	require.Zero(t, diff.Unexpected)
	require.Empty(t, diff.Examples)
}
