package datatug_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/datatug/datatug-core/pkg/storage/filestore"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func numericQuery(kind, token string) string {
	return fmt.Sprintf(`{"id":"numbers","title":"Exact defaults","type":"SQL","text":"SELECT @value","parameters":[{"id":"value","type":%q,"defaultValue":%s}]}`, kind, token)
}
func saveEnvelope(query string) string {
	return `{"storage":"local","project":"training","operationId":"save-numbers","ifNoneMatch":true,"query":` + query + `}`
}
func TestComparisonDecodersPreserveExactDefaults(t *testing.T) {
	for _, tc := range []struct{ kind, token string }{{"integer", "9223372036854775807"}, {"number", "9007199254740993"}, {"integer", "42"}} {
		t.Run(tc.token, func(t *testing.T) {
			wire := numericQuery(tc.kind, tc.token)
			for _, target := range []any{&datatug.QueryDef{}, &datatug.QueryDefWithFolderPath{}} {
				dec := json.NewDecoder(strings.NewReader(wire))
				dec.UseNumber()
				require.NoError(t, dec.Decode(target))
				var query datatug.QueryDef
				switch value := target.(type) {
				case *datatug.QueryDef:
					query = *value
				case *datatug.QueryDefWithFolderPath:
					query = value.QueryDef
				}
				require.Equal(t, json.Number(tc.token), query.Parameters[0].DefaultValue)
				require.NoError(t, query.Validate())
				encoded, err := json.Marshal(target)
				require.NoError(t, err)
				require.Contains(t, string(encoded), `"defaultValue":`+tc.token)
			}
			var request dto.SaveQueryRequest
			dec := json.NewDecoder(strings.NewReader(saveEnvelope(`{"folderPath":"queries",` + wire[1:])))
			dec.UseNumber()
			dec.DisallowUnknownFields()
			require.NoError(t, dec.Decode(&request))
			require.NoError(t, request.Validate())
			require.Equal(t, json.Number(tc.token), request.Query.Parameters[0].DefaultValue)
			before, err := request.PayloadDigest()
			require.NoError(t, err)
			store := filestore.NewProjectStore("training", t.TempDir()).(datatug.RevisionedQueriesStore)
			first, err := store.PutQuery(context.Background(), &request.Query, datatug.QueryWriteCondition{IfNoneMatch: true})
			require.NoError(t, err)
			loaded, err := store.LoadQueryRevision(context.Background(), "queries/numbers")
			require.NoError(t, err)
			require.Equal(t, json.Number(tc.token), loaded.Query.Parameters[0].DefaultValue)
			request.Query = loaded.Query
			after, err := request.PayloadDigest()
			require.NoError(t, err)
			require.Equal(t, before, after)
			second, err := store.PutQuery(context.Background(), &loaded.Query, datatug.QueryWriteCondition{IfMatch: first.Revision})
			require.NoError(t, err)
			require.Equal(t, first.Revision, second.Revision)
		})
	}
}
func TestComparisonScopedDecoderRejectsFoldedScopeDuplicates(t *testing.T) {
	query := numericQuery("integer", "42")
	for _, prefix := range []string{`"folderPath":"safe","FolderPath":"other",`, `"FolderPath":"other","folderPath":"safe",`} {
		wire := `{` + prefix + query[1:]
		require.Error(t, json.Unmarshal([]byte(wire), &datatug.QueryDefWithFolderPath{}))
		require.Error(t, apicontract.DecodeStrict([]byte(saveEnvelope(wire)), &dto.SaveQueryRequest{}))
	}
	wire := `{"folderPath":"safe",` + query[1:]
	var request dto.SaveQueryRequest
	require.NoError(t, apicontract.DecodeStrict([]byte(saveEnvelope(wire)), &request))
	require.Equal(t, "safe", request.Query.FolderPath)
}
func TestComparisonYAMLDecodersRetainKnownFields(t *testing.T) {
	query := "type: SQL\ntext: SELECT 1\n"
	for _, tc := range []struct {
		wire   string
		target any
	}{
		{query + "inventedAuthority: true\n", &datatug.QueryDef{}},
		{"folderpath: safe\nquerydef:\n  type: SQL\ninventedAuthority: true\n", &datatug.QueryDefWithFolderPath{}},
		{"folderpath: safe\nquerydef:\n  type: SQL\n  inventedAuthority: true\n", &datatug.QueryDefWithFolderPath{}},
	} {
		decoder := yaml.NewDecoder(bytes.NewBufferString(tc.wire))
		decoder.KnownFields(true)
		require.Error(t, decoder.Decode(tc.target))
	}
	// Preserve the existing nested envelope and valid YAML alias use.
	var scoped datatug.QueryDefWithFolderPath
	decoder := yaml.NewDecoder(strings.NewReader("folderpath: safe\nquerydef:\n  type: SQL\n  text: &body SELECT 1\n  purpose: *body\n"))
	decoder.KnownFields(true)
	require.NoError(t, decoder.Decode(&scoped))
	require.Equal(t, "safe", scoped.FolderPath)
	require.Equal(t, "SELECT 1", scoped.Purpose)
}
