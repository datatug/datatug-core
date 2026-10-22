package datatug

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func comparisonQuery(t *testing.T) QueryDef {
	t.Helper()
	data, err := os.ReadFile("testdata/query_comparison.json")
	require.NoError(t, err)
	var q QueryDef
	require.NoError(t, json.Unmarshal(data, &q))
	return q
}
func TestComparisonQueryWireAndYAML(t *testing.T) {
	q := comparisonQuery(t)
	require.NoError(t, q.Validate())
	for _, codec := range []struct {
		name      string
		marshal   func(any) ([]byte, error)
		unmarshal func([]byte, any) error
	}{
		{"json", json.Marshal, json.Unmarshal}, {"yaml", yaml.Marshal, yaml.Unmarshal},
	} {
		t.Run(codec.name, func(t *testing.T) {
			data, err := codec.marshal(q)
			require.NoError(t, err)
			var decoded QueryDef
			require.NoError(t, codec.unmarshal(data, &decoded))
			// Existing YAML normalizes omitted nil slices to empty slices. Compare
			// their persisted JSON meaning, while keeping the new metadata exact.
			before, err := json.Marshal(q)
			require.NoError(t, err)
			after, err := json.Marshal(decoded)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after))
			require.Equal(t, q.Comparison, decoded.Comparison)
			require.NoError(t, decoded.Validate())
			scoped := QueryDefWithFolderPath{FolderPath: "queries", QueryDef: q}
			data, err = codec.marshal(scoped)
			require.NoError(t, err)
			var restored QueryDefWithFolderPath
			require.NoError(t, codec.unmarshal(data, &restored))
			require.Equal(t, scoped.FolderPath, restored.FolderPath)
			require.Equal(t, scoped.Comparison, restored.Comparison)
			before, err = json.Marshal(scoped)
			require.NoError(t, err)
			after, err = json.Marshal(restored)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after))
		})
	}
	// Core does not enforce business positivity, source kinds or actual anchors.
	q.Parameters[0].DefaultValue = -1
	require.NoError(t, q.Validate())
}
func TestComparisonQueryDeclaredShape(t *testing.T) {
	cases := map[string]func(*QueryDef){
		"missing input":        func(q *QueryDef) { q.Parameters = q.Parameters[:1] },
		"duplicate input":      func(q *QueryDef) { q.Parameters = append(q.Parameters, q.Parameters[0]) },
		"optional input":       func(q *QueryDef) { q.Parameters[0].IsRequired = false },
		"multivalue input":     func(q *QueryDef) { q.Parameters[1].IsMultiValue = true },
		"wrong parameter type": func(q *QueryDef) { q.Parameters[0].Type = "number" },
		"same inputs":          func(q *QueryDef) { q.Comparison.ControlInput = q.Comparison.PrimaryInput },
		"absent result":        func(q *QueryDef) { q.Recordsets = nil },
		"multiple results":     func(q *QueryDef) { q.Recordsets = append(q.Recordsets, q.Recordsets[0]) },
		"missing column":       func(q *QueryDef) { q.Recordsets[0].Columns = q.Recordsets[0].Columns[1:] },
		"wrong column type":    func(q *QueryDef) { q.Recordsets[0].Columns[0].Type = "number" },
		"duplicate column": func(q *QueryDef) {
			q.Recordsets[0].Columns = append(q.Recordsets[0].Columns, q.Recordsets[0].Columns[0])
		},
		"padded role":      func(q *QueryDef) { q.Comparison.Projection.LaneColumn = " lane" },
		"credential role":  func(q *QueryDef) { q.Comparison.Projection.LaneColumn = "Password=secret" },
		"credential input": func(q *QueryDef) { q.Comparison.PrimaryInput = "Password=secret" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) { q := comparisonQuery(t); mutate(&q); require.Error(t, q.Validate()) })
	}
}
func TestComparisonInvalidSharedFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/comparison_invalid.json")
	require.NoError(t, err)
	var cases []struct{ Name, Target, Wire string }
	require.NoError(t, json.Unmarshal(data, &cases))
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			var value any
			if tc.Target == "query" {
				value = &QueryDef{}
			} else {
				value = &TimelineChartSpec{}
			}
			require.Error(t, json.Unmarshal([]byte(tc.Wire), value))
			if tc.Target == "query" {
				require.Error(t, json.Unmarshal([]byte(tc.Wire), &QueryDefWithFolderPath{}))
				require.Error(t, yaml.Unmarshal([]byte("folderpath: exact\nquerydef: "+tc.Wire), &QueryDefWithFolderPath{}))
			}
			// JSON is valid YAML: the same canonical/required/null/type rules apply.
			if tc.Target == "query" {
				value = &QueryDef{}
			} else {
				value = &TimelineChartSpec{}
			}
			require.Error(t, yaml.Unmarshal([]byte(tc.Wire), value))
		})
	}
}
func TestTimelineElapsedWireAndAbsoluteDefault(t *testing.T) {
	data, err := os.ReadFile("testdata/timeline_elapsed.json")
	require.NoError(t, err)
	var elapsed TimelineChartSpec
	require.NoError(t, json.Unmarshal(data, &elapsed))
	require.NoError(t, elapsed.Validate())
	wire, err := yaml.Marshal(elapsed)
	require.NoError(t, err)
	var restored TimelineChartSpec
	require.NoError(t, yaml.Unmarshal(wire, &restored))
	require.Equal(t, elapsed, restored)
	for _, mode := range []string{"", "absolute"} {
		absolute := elapsed
		absolute.TimeMode = mode
		absolute.AnchorColumn = ""
		require.NoError(t, absolute.Validate())
		data, err = json.Marshal(absolute)
		require.NoError(t, err)
		require.NotContains(t, string(data), "anchorColumn")
		require.NoError(t, json.Unmarshal(data, &restored))
		require.Equal(t, absolute, restored)
	}
}

func TestComparisonQueryStrictEnvelope(t *testing.T) {
	q := comparisonQuery(t)
	scoped := QueryDefWithFolderPath{FolderPath: "exact/folder", QueryDef: q}
	data, err := json.Marshal(scoped)
	require.NoError(t, err)
	var envelope struct {
		Query QueryDefWithFolderPath `json:"query"`
	}
	require.NoError(t, json.Unmarshal(append(append([]byte(`{"query":`), data...), '}'), &envelope))
	require.Equal(t, scoped.FolderPath, envelope.Query.FolderPath)
	require.Equal(t, q.Comparison, envelope.Query.Comparison)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &fields))
	fields["inventedAuthority"] = json.RawMessage(`true`)
	bad, err := json.Marshal(fields)
	require.NoError(t, err)
	require.Error(t, json.Unmarshal(bad, &QueryDefWithFolderPath{}))
	delete(fields, "folderPath")
	bad, err = json.Marshal(fields)
	require.NoError(t, err)
	require.Error(t, json.Unmarshal(bad, &QueryDef{}))
}

func TestComparisonYAMLNullCannotHideBehindAliasOrMerge(t *testing.T) {
	for _, wire := range []string{
		"base: &base {comparison: null}\n<<: *base\ntype: SQL\n",
		"empty: &empty null\ncomparison: *empty\ntype: SQL\n",
		"comparison: null\ncomparison: {}\n",
	} {
		require.Error(t, yaml.Unmarshal([]byte(wire), &QueryDef{}))
	}
}
