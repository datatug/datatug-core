package meaninggraph

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRetainedJSONSyntaxDepthAndExactValues(t *testing.T) {
	for _, body := range []string{`{"x":`, `{"x":1,}`, `[]`, `null`, `1`, `{"x":` + strings.Repeat(`[`, 129) + `0` + strings.Repeat(`]`, 129) + `}`} {
		_, err := parseJSON([]byte(body))
		require.Error(t, err, body[:min(len(body), 60)])
	}
	_, err := obj([]any{})
	require.Error(t, err)
	_, err = list(map[string]any{})
	require.Error(t, err)
	_, err = str("")
	require.Error(t, err)
	require.True(t, utf16Less("a", "aa"))
	raw, err := canonical([]any{nil, true, false, "\"\\\b\f\n\r\t\x01"})
	require.NoError(t, err)
	require.Equal(t, `[null,true,false,"\"\\\b\f\n\r\t\u0001"]`, string(raw))
	for _, unsupported := range []any{1.1, []any{1.1}, map[string]any{"rounded": 1.1}} {
		_, err = canonical(unsupported)
		require.ErrorContains(t, err, "rounded")
	}
	for _, tc := range []struct {
		a, b any
		same bool
	}{
		{json.Number("-0.00"), json.Number("0"), true},
		{json.Number("-1000"), json.Number("-1e3"), true},
		{json.Number("1"), "1", false},
		{map[string]any{"a": true}, []any{}, false},
		{map[string]any{"a": true}, map[string]any{"b": true}, false},
		{[]any{true}, []any{}, false},
		{[]any{true}, []any{false}, false},
	} {
		same, e := equal(tc.a, tc.b)
		require.NoError(t, e)
		require.Equal(t, tc.same, same)
	}
	for _, tc := range []struct{ a, b any }{
		{json.Number("1einvalid"), json.Number("1")},
		{json.Number("1"), json.Number("1einvalid")},
		{[]any{json.Number("1einvalid")}, []any{json.Number("1")}},
	} {
		_, err = equal(tc.a, tc.b)
		require.Error(t, err)
	}
}

func TestArchivedModelVocabularyAndShapeRefusals(t *testing.T) {
	original := map[string]any{"modelspec": "1.0-draft", "module": map[string]any{"id": "m", "version": "1"}, "components": map[string]any{"Part": map[string]any{"fields": map[string]any{"child": map[string]any{"entity": "m.Target"}}}}, "entities": map[string]any{"Target": map[string]any{"properties": map[string]any{"id": map[string]any{"type": "string"}}}}, "enums": map[string]any{"Status": map[string]any{"values": []any{"yes", "no"}}}}
	normalized, err := normalizeAST(original)
	require.NoError(t, err)
	require.Equal(t, "m.Target", normalized["components"].(map[string]any)["Part"].(map[string]any)["fields"].(map[string]any)["child"].(map[string]any)["record"])
	require.Contains(t, normalized["records"], "Target")
	require.Contains(t, normalized["enums"], "Status")
	cases := []map[string]any{
		{"modelspec": "future"},
		{"modelspec": "1.0-draft-2", "extra": true},
		{"modelspec": "1.0-draft-2", "records": []any{}},
		{"modelspec": "1.0-draft-2", "records": map[string]any{"A": true}},
		{"modelspec": "1.0-draft-2", "enums": map[string]any{"A": map[string]any{"extra": true}}},
		{"modelspec": "1.0-draft-2", "components": map[string]any{"A": map[string]any{"use": []any{"B"}}}},
		{"modelspec": "1.0-draft-2", "records": map[string]any{"A": map[string]any{"fields": []any{}}}},
		{"modelspec": "1.0-draft-2", "records": map[string]any{"A": map[string]any{"fields": map[string]any{"x": true}}}},
		{"modelspec": "1.0-draft-2", "records": map[string]any{"A": map[string]any{"fields": map[string]any{"x": map[string]any{"extra": true}}}}},
	}
	for _, source := range cases {
		_, err := normalizeAST(source)
		require.Error(t, err)
	}
	for _, address := range []struct{ kind, address string }{{"meaning", "meaning://github.com/o/r?query"}, {"meaning", "meaning://github.com/o/.."}, {"other", "modelspec://github.com/o/r/m"}} {
		_, err := normalizeAddress(address.kind, address.address)
		require.Error(t, err)
	}
}
