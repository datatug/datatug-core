package challenge

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/stretchr/testify/require"
)

// sharedGradingVectors is the compact, language-neutral golden shared with the
// browser/TypeScript grading implementation. Values use the TypedValue wire
// shape but are decoded locally so malformed cells can be represented and
// rejected by grading instead of failing at JSON decode time.
type sharedGradingVectors struct {
	GradingVersion string                `json:"gradingVersion"`
	Vectors        []sharedGradingVector `json:"vectors"`
}

type sharedGradingVector struct {
	ID          string                 `json:"id"`
	Description string                 `json:"description"`
	Options     RowOptions             `json:"options"`
	Expected    sharedGradingRecordset `json:"expected"`
	Observed    sharedGradingRecordset `json:"observed"`
	Want        sharedGradingWant      `json:"want"`
}

type sharedGradingRecordset struct {
	Columns []apicontract.Column  `json:"columns"`
	Rows    [][]sharedGradingCell `json:"rows"`
}

type sharedGradingCell struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

type sharedGradingWant struct {
	Match      bool   `json:"match"`
	Problem    string `json:"problem,omitempty"`
	Missing    int    `json:"missing,omitempty"`
	Unexpected int    `json:"unexpected,omitempty"`
}

func (r sharedGradingRecordset) typedRows() [][]apicontract.TypedValue {
	rows := make([][]apicontract.TypedValue, len(r.Rows))
	for rowIndex, row := range r.Rows {
		rows[rowIndex] = make([]apicontract.TypedValue, len(row))
		for columnIndex, cell := range row {
			rows[rowIndex][columnIndex] = cell.typedValue()
		}
	}
	return rows
}

func (c sharedGradingCell) typedValue() apicontract.TypedValue {
	switch c.Type {
	case string(apicontract.ValueTypeString), string(apicontract.ValueTypeInteger),
		string(apicontract.ValueTypeDecimal), string(apicontract.ValueTypeDate),
		string(apicontract.ValueTypeDatetime):
		var value string
		_ = json.Unmarshal(c.Value, &value)
		return apicontract.TypedValue{Type: apicontract.ValueType(c.Type), Str: value}
	case string(apicontract.ValueTypeNumber):
		var value float64
		if err := json.Unmarshal(c.Value, &value); err != nil {
			// Preserve a non-finite/invalid number so Validate, not JSON
			// decoding, reports the malformed typed cell.
			value = math.NaN()
		}
		return apicontract.TypedValue{Type: apicontract.ValueTypeNumber, Num: value}
	case string(apicontract.ValueTypeBoolean):
		var value bool
		_ = json.Unmarshal(c.Value, &value)
		return apicontract.TypedValue{Type: apicontract.ValueTypeBoolean, Bool: value}
	case string(apicontract.ValueTypeNull):
		return apicontract.NewNullValue()
	default:
		return apicontract.TypedValue{Type: apicontract.ValueType(c.Type)}
	}
}

func TestSharedGradingVectorsMatchGoComparison(t *testing.T) {
	var vectors sharedGradingVectors
	require.NoError(t, apicontract.DecodeStrict(readTestFile(t, "testdata/grading-vectors.json"), &vectors))
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
		})
	}
}
