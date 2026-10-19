package datatug_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage/filestore"
	"github.com/stretchr/testify/require"
)

const environmentWithMetadata = `{
  "id":"local","title":"Local","folder":"~/environments","tags":["demo"],
  "userIds":["alice"],"access":"private","editionConnections":["sqlite"],
  "dbServers":[{"driver":"sqlite3","catalogs":["chinook-local","orders","countries","chinook","geo"]}],
  "presentation":{"label":"LOCAL","icon":"desktop-outline","tone":"neutral"},
  "opaque":{"nested":[null,{"integer":9007199254740993,"enabled":true}]},
  "futureNull":null
}`

func environmentDocument(t *testing.T, value any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var document map[string]any
	require.NoError(t, decoder.Decode(&document))
	return document
}

func TestEnvironmentJSONPreservesOpaqueMetadataAndTypedChanges(t *testing.T) {
	input := []byte(environmentWithMetadata)
	var environment datatug.Environment
	require.NoError(t, json.Unmarshal(input, &environment))
	before := environmentDocument(t, &environment)
	require.Equal(t, map[string]any{"label": "LOCAL", "icon": "desktop-outline", "tone": "neutral"}, before["presentation"])
	// Retaining metadata must not retain the caller's input buffer.
	for i := range input {
		input[i] = 'x'
	}
	environment.ID = "edited"
	environment.Title = ""
	environment.Folder = ""
	environment.Tags = nil
	environment.UserIDs = nil
	environment.Access = ""
	environment.EditionConnections = nil
	environment.DbServers[0].Catalogs = append(environment.DbServers[0].Catalogs, "chinook-browser")
	after := environmentDocument(t, &environment)
	require.Equal(t, "edited", after["id"])
	for _, key := range []string{"title", "folder", "tags", "userIds", "access", "editionConnections"} {
		require.NotContains(t, after, key, "cleared typed fields must not reappear from retained JSON")
	}
	for _, key := range []string{"presentation", "opaque", "futureNull"} {
		require.Contains(t, after, key)
		require.Equal(t, before[key], after[key])
	}
	opaque := after["opaque"].(map[string]any)["nested"].([]any)[1].(map[string]any)
	require.Equal(t, json.Number("9007199254740993"), opaque["integer"])
	require.Equal(t, []any{"chinook-local", "orders", "countries", "chinook", "geo", "chinook-browser"}, after["dbServers"].([]any)[0].(map[string]any)["catalogs"])
	environment.ID = ""
	require.NotContains(t, environmentDocument(t, &environment), "id")
}

func TestEnvironmentJSONReusedDecodeAndNull(t *testing.T) {
	var environment datatug.Environment
	require.NoError(t, json.Unmarshal([]byte(environmentWithMetadata), &environment))
	before := environmentDocument(t, &environment)
	require.NoError(t, json.Unmarshal([]byte(" \n null\t"), &environment))
	require.Equal(t, before, environmentDocument(t, &environment), "legacy root null is a no-op")
	require.NoError(t, json.Unmarshal([]byte(`{"id":"next","dbServers":[]}`), &environment))
	after := environmentDocument(t, &environment)
	require.Equal(t, "next", after["id"])
	require.Equal(t, "Local", after["title"], "retain encoding/json's existing typed-field update semantics")
	for _, key := range []string{"presentation", "opaque", "futureNull"} {
		require.NotContains(t, after, key, "metadata from the previous object must not leak")
	}
}

func TestEnvironmentJSONKnownAliasesAreCanonicalAndAuthoritative(t *testing.T) {
	for _, field := range []struct{ key, value string }{
		{"id", `"local"`}, {"title", `"Local"`}, {"folder", `"~/envs"`},
		{"tags", `["demo"]`}, {"userIds", `["alice"]`}, {"access", `"private"`},
		{"dbServers", `[]`}, {"editionConnections", `["sqlite"]`},
	} {
		t.Run(field.key, func(t *testing.T) {
			var environment datatug.Environment
			input := `{"` + strings.ToUpper(field.key) + `":` + field.value + `,"opaque":null}`
			require.NoError(t, json.Unmarshal([]byte(input), &environment))
			document := environmentDocument(t, &environment)
			require.Contains(t, document, field.key)
			require.NotContains(t, document, strings.ToUpper(field.key))
			conflict := `{"` + field.key + `":` + field.value + `,"` + strings.ToUpper(field.key) + `":` + field.value + `}`
			require.ErrorContains(t, json.Unmarshal([]byte(conflict), &environment), "case-aliased")
		})
	}
	var environment datatug.Environment
	require.ErrorContains(t, json.Unmarshal([]byte(`{"access":"private","acceſs":"public"}`), &environment), "case-aliased")
}

func TestEnvironmentJSONPreservesCaseDistinctOpaqueKeys(t *testing.T) {
	const input = `{"dbServers":null,"custom":{"nested":[null,9007199254740993]},"Custom":null,"presentation":{"label":"LOCAL"},"PRESENTATION":{"label":"opaque"}}`
	var environment datatug.Environment
	require.NoError(t, json.Unmarshal([]byte(input), &environment))
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()
	var expected map[string]any
	require.NoError(t, decoder.Decode(&expected))
	require.Equal(t, expected, environmentDocument(t, &environment))
}

func TestEnvironmentJSONDirectDecodeRejectsTrailingContent(t *testing.T) {
	var environment datatug.Environment
	require.NoError(t, environment.UnmarshalJSON([]byte(environmentWithMetadata)))
	before := environmentDocument(t, &environment)
	for _, input := range []string{`null trailinggarbage`, `null true`, `null {}`, `null ]`, `{} trailinggarbage`} {
		t.Run(input, func(t *testing.T) {
			require.Error(t, environment.UnmarshalJSON([]byte(input)))
			require.Equal(t, before, environmentDocument(t, &environment))
		})
	}
	require.NoError(t, environment.UnmarshalJSON([]byte("null \n\t")))
	require.Equal(t, before, environmentDocument(t, &environment))
}

func TestEnvironmentJSONRejectsMalformedOrInvalidTypedFields(t *testing.T) {
	for _, data := range []string{
		`{`, `[]`, `17`, `"local"`, `true`, `null true`, `{} {}`,
		`{"id":17}`, `{"title":[]}`, `{"folder":{}}`, `{"tags":17}`,
		`{"userIds":[false]}`, `{"access":false}`, `{"dbServers":"wrong"}`,
		`{"editionConnections":[{}]}`,
	} {
		t.Run(data, func(t *testing.T) {
			var environment datatug.Environment
			require.Error(t, json.Unmarshal([]byte(data), &environment))
		})
	}
}

func TestEnvironmentJSONCaseAliasRefusalDoesNotMutateExistingCatalogs(t *testing.T) {
	var environment datatug.Environment
	require.NoError(t, json.Unmarshal([]byte(environmentWithMetadata), &environment))
	before := environmentDocument(t, &environment)
	input := `{"dbServers":[{"driver":"sqlite3","catalogs":["changed"]}],"access":"private","ACCESS":"public"}`
	require.ErrorContains(t, json.Unmarshal([]byte(input), &environment), "case-aliased")
	require.Equal(t, before, environmentDocument(t, &environment), "new alias refusal must precede mutations through existing pointers or slices")
}

func TestEnvironmentMetadataSurvivesBothFilestoreLayouts(t *testing.T) {
	for _, file := range []string{"local.env.json", "environment-summary.json"} {
		t.Run(file, func(t *testing.T) {
			root := t.TempDir()
			directory := filepath.Join(root, "environments", "local")
			require.NoError(t, os.MkdirAll(directory, 0o755))
			filePath := filepath.Join(directory, file)
			require.NoError(t, os.WriteFile(filePath, []byte(environmentWithMetadata), 0o600))
			store := filestore.NewProjectStore("project", root)
			ctx := context.Background()
			environment, err := store.LoadEnvironment(ctx, "local")
			require.NoError(t, err)
			before := environmentDocument(t, environment)
			for _, key := range []string{"presentation", "opaque", "futureNull"} {
				require.Contains(t, before, key)
			}
			environment.DbServers[0].Catalogs = append(environment.DbServers[0].Catalogs, "chinook-browser")
			require.NoError(t, store.SaveEnvironment(ctx, environment))
			reloaded, err := store.LoadEnvironment(ctx, "local")
			require.NoError(t, err)
			after := environmentDocument(t, reloaded)
			for _, key := range []string{"presentation", "opaque", "futureNull"} {
				require.Equal(t, before[key], after[key])
			}
			require.Equal(t, []string{"chinook-local", "orders", "countries", "chinook", "geo", "chinook-browser"}, reloaded.DbServers[0].Catalogs)
			entries, err := os.ReadDir(directory)
			require.NoError(t, err)
			require.Len(t, entries, 1, "save must preserve the selected layout")
			require.Equal(t, file, entries[0].Name())
			summary, err := store.LoadEnvironmentSummary(ctx, "local")
			require.NoError(t, err)
			for _, key := range []string{"presentation", "opaque", "futureNull"} {
				require.NotContains(t, environmentDocument(t, summary), key, "opaque metadata is not a new summary wire field")
			}
		})
	}
}

func TestEnvironmentMetadataDualLayoutsCompareJSONValues(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "environments", "local")
	require.NoError(t, os.MkdirAll(directory, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "local.env.json"), []byte(environmentWithMetadata), 0o600))
	decoder := json.NewDecoder(strings.NewReader(environmentWithMetadata))
	decoder.UseNumber()
	var document map[string]any
	require.NoError(t, decoder.Decode(&document))
	// Map encoding sorts object keys and changes whitespace, but not JSON values.
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "environment-summary.json"), encoded, 0o600))
	environment, err := filestore.NewProjectStore("project", root).LoadEnvironment(context.Background(), "local")
	require.NoError(t, err)
	require.Contains(t, environmentDocument(t, environment), "presentation")
}
