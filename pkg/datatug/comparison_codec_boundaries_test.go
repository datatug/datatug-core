package datatug

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestComparisonCodecMalformedPublicBoundaries(t *testing.T) {
	for _, wire := range []string{"[]", "null", `{} true`} {
		t.Run(wire, func(t *testing.T) {
			var comparison QueryComparison
			require.Error(t, comparison.UnmarshalJSON([]byte(wire)))
			var timeline TimelineChartSpec
			require.Error(t, timeline.UnmarshalJSON([]byte(wire)))
		})
	}
	for _, wire := range []string{"[]", `{} true`} {
		var query QueryDef
		require.Error(t, query.UnmarshalJSON([]byte(wire)))
	}
	for _, wire := range []string{"kind: pair\nkind: pair\n", "1: pair\n", "kind: .nan\n"} {
		var comparison QueryComparison
		require.Error(t, yaml.Unmarshal([]byte(wire), &comparison))
	}
	for _, wire := range []string{"laneColumn: a\nlaneColumn: b\n", "1: invalid\n", "timeMode: .inf\n"} {
		var timeline TimelineChartSpec
		require.Error(t, yaml.Unmarshal([]byte(wire), &timeline))
	}
	for _, wire := range []string{"[]", "scalar"} {
		var query QueryDef
		require.Error(t, yaml.Unmarshal([]byte(wire), &query))
	}
}
func TestComparisonProjectionStandaloneYAML(t *testing.T) {
	query := comparisonQuery(t)
	data, err := yaml.Marshal(query.Comparison.Projection)
	require.NoError(t, err)
	var projection QueryComparisonProjection
	require.NoError(t, yaml.Unmarshal(data, &projection))
	require.Equal(t, query.Comparison.Projection, projection)
	require.Error(t, yaml.Unmarshal([]byte("memberColumn: x\n"), &projection))
}
func TestTimelineProgrammaticModeRefusals(t *testing.T) {
	data, err := os.ReadFile("testdata/timeline_elapsed.json")
	require.NoError(t, err)
	var timeline TimelineChartSpec
	require.NoError(t, json.Unmarshal(data, &timeline))
	timeline.TimeMode = "absolute"
	require.ErrorContains(t, timeline.Validate(), "requires elapsed")
	timeline.TimeMode = "relative"
	require.ErrorContains(t, timeline.Validate(), "unsupported")
}
func TestComparisonScopedYAMLRejectsExternalAliasCycles(t *testing.T) {
	var envelope struct {
		Base  yaml.Node
		Query QueryDefWithFolderPath
	}
	wire := "base: &base\n  self: *base\nquery:\n  folderpath: scope\n  querydef:\n    type: SQL\n    parameters:\n      - id: value\n        type: custom\n        defaultvalue: *base\n"
	require.Error(t, yaml.Unmarshal([]byte(wire), &envelope))
}
func TestComparisonScopedYAMLRejectsInvalidPublicNodes(t *testing.T) {
	var scoped QueryDefWithFolderPath
	require.Error(t, scoped.UnmarshalYAML(&yaml.Node{Kind: 99, Value: "invalid node"}))
}

func TestComparisonScopedYAMLEmptyNodeCannotAdmitQuery(t *testing.T) {
	var scoped QueryDefWithFolderPath
	// Empty YAML decoding can produce a zero model; it must never validate as
	// a project mutation or gain an inferred folder/type/comparison declaration.
	require.NoError(t, scoped.UnmarshalYAML(nil))
	require.Equal(t, QueryDefWithFolderPath{}, scoped)
	require.Error(t, scoped.Validate())
}

func TestComparisonScopedYAMLRejectsProgrammaticAliasGraphs(t *testing.T) {
	scalar := func(value string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value} }
	for _, cyclic := range []bool{false, true} {
		target := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Anchor: "later"}
		target.Content = []*yaml.Node{scalar("type"), scalar("SQL")}
		if cyclic {
			external := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Anchor: "external"}
			external.Content = []*yaml.Node{scalar("self"), {Kind: yaml.AliasNode, Value: "external", Alias: external}}
			target.Content = append(target.Content, scalar("defaults"), &yaml.Node{Kind: yaml.AliasNode, Value: "external", Alias: external})
		}
		// Programmatically constructed YAML can contain a forward reference even
		// though the text parser never emits one. Public decoding must refuse it
		// (including a cyclic target) without panic or a partially admitted scope.
		forward := &yaml.Node{Kind: yaml.AliasNode, Value: "later", Alias: target}
		mapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{scalar("folderpath"), forward, scalar("querydef"), target}}
		var scoped QueryDefWithFolderPath
		require.Error(t, scoped.UnmarshalYAML(mapping))
		require.Equal(t, QueryDefWithFolderPath{}, scoped)
	}
	var scoped QueryDefWithFolderPath
	require.Error(t, scoped.UnmarshalYAML(&yaml.Node{Kind: yaml.AliasNode, Value: "missing"}))
}
