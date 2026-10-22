package datatug

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const sourceQueryJSON = `{"id":"q","title":"Query","type":"SQL","text":"SELECT 1","parameters":[{"id":"exact","type":"integer","defaultValue":9223372036854775807}],"federation":{"ovdbBaseUrl":"https://current.example/api","expectedServerIdentity":{"baseUrl":"https://current.example/api","serverId":"https://historic.example/identity"},"tables":[{"name":"Invoice","fields":["id"]}]},"relationshipBindings":[{"id":"FK:Invoice/Customer","version":"fixture:main.Invoice.id:INTEGER->main.Customer.id:INTEGER:PRIMARY_KEY:v1","from":{"schema":"main","table":"Invoice"},"to":{"schema":"main","table":"Customer"},"pairs":[{"fromField":"CustomerId","toField":"ID"}]}]}`

func sourceQuery(t *testing.T) QueryDef {
	t.Helper()
	var q QueryDef
	require.NoError(t, json.Unmarshal([]byte(sourceQueryJSON), &q))
	return q
}
func TestQuerySourceMetadataJSONRoundTripAndExactNumbers(t *testing.T) {
	q := sourceQuery(t)
	require.NoError(t, q.Validate())
	require.Equal(t, json.Number("9223372036854775807"), q.Parameters[0].DefaultValue)
	data, err := json.Marshal(q)
	require.NoError(t, err)
	var again QueryDef
	require.NoError(t, json.Unmarshal(data, &again))
	require.Equal(t, q, again)
	for _, value := range []string{"9223372036854775807", "9007199254740993"} {
		raw := strings.Replace(sourceQueryJSON, "9223372036854775807", value, 1)
		var scoped QueryDefWithFolderPath
		require.NoError(t, json.Unmarshal([]byte(strings.Replace(raw, `{"id":`, `{"folderPath":"folder","id":`, 1)), &scoped))
		require.Equal(t, "folder", scoped.FolderPath)
		require.Equal(t, json.Number(value), scoped.Parameters[0].DefaultValue)
		saved, err := json.Marshal(scoped)
		require.NoError(t, err)
		require.Contains(t, string(saved), `"defaultValue":`+value)
	}
}
func TestQuerySourceMetadataStrictJSON(t *testing.T) {
	for name, changed := range map[string]string{
		"unknown-root":          strings.Replace(sourceQueryJSON, `"id":"q"`, `"id":"q","future":true`, 1),
		"root-alias":            strings.Replace(sourceQueryJSON, `"relationshipBindings":`, `"RelationshipBindings":`, 1),
		"root-null":             strings.Replace(sourceQueryJSON, `"relationshipBindings":[`, `"relationshipBindings":null,"unused":[`, 1),
		"identity-null":         strings.Replace(sourceQueryJSON, `"expectedServerIdentity":{"baseUrl":"https://current.example/api","serverId":"https://historic.example/identity"}`, `"expectedServerIdentity":null`, 1),
		"identity-alias":        strings.Replace(sourceQueryJSON, `"expectedServerIdentity":`, `"ExpectedServerIdentity":`, 1),
		"identity-member-alias": strings.Replace(sourceQueryJSON, `"baseUrl":`, `"BaseUrl":`, 1),
		"identity-duplicate":    strings.Replace(sourceQueryJSON, `"serverId":`, `"serverId":"first","serverId":`, 1),
		"identity-missing":      strings.Replace(sourceQueryJSON, `"baseUrl":"https://current.example/api",`, "", 1),
		"identity-unknown":      strings.Replace(sourceQueryJSON, `"serverId":`, `"grant":true,"serverId":`, 1),
		"federation-sibling":    strings.Replace(sourceQueryJSON, `"tables":`, `"silentGrant":true,"tables":`, 1),
		"federation-duplicate":  strings.Replace(sourceQueryJSON, `"ovdbBaseUrl":`, `"ovdbBaseUrl":"https://other.example","OVDBBaseUrl":`, 1),
		"endpoint-null":         strings.Replace(sourceQueryJSON, `"from":{"schema":"main","table":"Invoice"}`, `"from":null`, 1),
		"endpoint-unknown":      strings.Replace(sourceQueryJSON, `"from":{"schema":`, `"from":{"database":"other","schema":`, 1),
		"pair-null":             strings.Replace(sourceQueryJSON, `"pairs":[{"fromField":"CustomerId","toField":"ID"}]`, `"pairs":[null]`, 1),
		"pair-alias":            strings.Replace(sourceQueryJSON, `"fromField":`, `"FromField":`, 1),
		"pair-number":           strings.Replace(sourceQueryJSON, `"toField":"ID"`, `"toField":123`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			var q QueryDef
			require.Error(t, json.Unmarshal([]byte(changed), &q))
			var scoped QueryDefWithFolderPath
			require.Error(t, json.Unmarshal([]byte(strings.Replace(changed, `{"id":`, `{"folderPath":"safe","id":`, 1)), &scoped))
		})
	}
	// Pure null (without another unknown field) cannot silently become omission.
	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(sourceQueryJSON), &root))
	root["relationshipBindings"] = json.RawMessage("null")
	data, _ := json.Marshal(root)
	var q QueryDef
	require.Error(t, json.Unmarshal(data, &q))
}
func TestQuerySourceMetadataValidationAndCredentialSweep(t *testing.T) {
	mutations := map[string]func(*QueryDef){
		"identity-base":     func(q *QueryDef) { q.Federation.ExpectedServerIdentity.BaseURL = "https://user:secret@example.com" },
		"identity-value":    func(q *QueryDef) { q.Federation.ExpectedServerIdentity.ServerID = "Password=secret" },
		"binding-id":        func(q *QueryDef) { q.RelationshipBindings[0].ID = "Password=secret" },
		"binding-version":   func(q *QueryDef) { q.RelationshipBindings[0].Version = "Password=secret" },
		"from-schema":       func(q *QueryDef) { q.RelationshipBindings[0].From.Schema = "Password=secret" },
		"from-table":        func(q *QueryDef) { q.RelationshipBindings[0].From.Table = "Password=secret" },
		"to-schema":         func(q *QueryDef) { q.RelationshipBindings[0].To.Schema = "Password=secret" },
		"to-table":          func(q *QueryDef) { q.RelationshipBindings[0].To.Table = "Password=secret" },
		"from-field":        func(q *QueryDef) { q.RelationshipBindings[0].Pairs[0].FromField = "Password=secret" },
		"to-field":          func(q *QueryDef) { q.RelationshipBindings[0].Pairs[0].ToField = "Password=secret" },
		"duplicate-binding": func(q *QueryDef) { q.RelationshipBindings = append(q.RelationshipBindings, q.RelationshipBindings[0]) },
		"duplicate-field": func(q *QueryDef) {
			q.RelationshipBindings[0].Pairs = append(q.RelationshipBindings[0].Pairs, QueryRelationshipFieldPair{FromField: "CustomerId", ToField: "Other"})
		},
		"missing-pairs":    func(q *QueryDef) { q.RelationshipBindings[0].Pairs = nil },
		"version-controls": func(q *QueryDef) { q.RelationshipBindings[0].Version = "pin\nnext" },
		"version-bound":    func(q *QueryDef) { q.RelationshipBindings[0].Version = strings.Repeat("v", 4097) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) { q := sourceQuery(t); mutate(&q); require.Error(t, q.Validate()) })
	}
	for _, base := range []string{"https://example.com/api", "http://localhost:8080/path", "http://127.0.0.1:8080", "http://[::1]:8080"} {
		require.NoError(t, (QueryExpectedServerIdentity{BaseURL: base, ServerID: "opaque historic identity"}).Validate())
	}
	for _, base := range []string{"", "ftp://example.com", "http://example.com", "https://example.com/?token=1", "https://example.com/#x", "https://example.com/?", "https://example.com/#", "https://user@example.com"} {
		require.Error(t, (QueryExpectedServerIdentity{BaseURL: base, ServerID: "id"}).Validate())
	}
	for _, id := range []string{"", " ", "id\x00", strings.Repeat("x", 4097)} {
		require.Error(t, (QueryExpectedServerIdentity{BaseURL: "https://example.com", ServerID: id}).Validate())
	}
	q := sourceQuery(t)
	q.RelationshipBindings[0].Version = "001:2026-10-21→雪"
	q.RelationshipBindings[0].Pairs = append(q.RelationshipBindings[0].Pairs, QueryRelationshipFieldPair{FromField: "Tenant:名", ToField: "Tenant ID"})
	q.RelationshipBindings[0].From.Schema = ""
	require.NoError(t, q.Validate())
}

const sourceQueryYAML = `projectitem:
  projitembrief: {id: q, title: Query}
type: SQL
text: SELECT 1
federation:
  ovdbBaseUrl: https://current.example
  expectedServerIdentity:
    baseUrl: https://current.example
    serverId: 001
relationshipBindings:
  - id: 001
    version: 2026-10-21
    from: {schema: 001, table: 2026-10-21}
    to: {schema: main, table: Other}
    pairs: [{fromField: 001, toField: 2026-10-21}]
`

func TestQuerySourceMetadataYAMLLexemesAndClosedShape(t *testing.T) {
	var q QueryDef
	require.NoError(t, yaml.Unmarshal([]byte(sourceQueryYAML), &q))
	require.NoError(t, q.Validate())
	require.Equal(t, "001", q.Federation.ExpectedServerIdentity.ServerID)
	require.Equal(t, "001", q.RelationshipBindings[0].ID)
	require.Equal(t, "2026-10-21", q.RelationshipBindings[0].Version)
	require.Equal(t, "2026-10-21", q.RelationshipBindings[0].From.Table)
	require.Equal(t, "2026-10-21", q.RelationshipBindings[0].Pairs[0].ToField)
	saved, err := yaml.Marshal(q)
	require.NoError(t, err)
	var again QueryDef
	require.NoError(t, yaml.Unmarshal(saved, &again))
	require.Equal(t, q.Federation, again.Federation)
	require.Equal(t, q.RelationshipBindings, again.RelationshipBindings)
	scoped := "folderpath: 001\nquerydef:\n" + "  " + strings.ReplaceAll(strings.TrimSuffix(sourceQueryYAML, "\n"), "\n", "\n  ") + "\n"
	var wrapper QueryDefWithFolderPath
	require.NoError(t, yaml.Unmarshal([]byte(scoped), &wrapper))
	require.Equal(t, "001", wrapper.FolderPath)
	require.Equal(t, q, wrapper.QueryDef)
	for _, changed := range []string{
		strings.Replace(sourceQueryYAML, "expectedServerIdentity:", "ExpectedServerIdentity:", 1),
		strings.Replace(sourceQueryYAML, "    serverId: 001", "    serverId: null", 1),
		strings.Replace(sourceQueryYAML, "    baseUrl:", "    BaseUrl:", 1),
		strings.Replace(sourceQueryYAML, "relationshipBindings:", "relationshipBindings: null\nother:", 1),
		strings.Replace(sourceQueryYAML, "pairs: [{fromField: 001, toField: 2026-10-21}]", "pairs: [null]", 1),
		strings.Replace(sourceQueryYAML, "  ovdbBaseUrl:", "  extra: true\n  ovdbBaseUrl:", 1),
	} {
		var bad QueryDef
		require.Error(t, yaml.Unmarshal([]byte(changed), &bad))
	}
}
func TestQuerySourceMetadataYAMLExternalAliasesAndShadowIdentity(t *testing.T) {
	data := `base: &base
  value: &label outer
querydef:
  projectitem:
    projitembrief: {id: q, title: Query}
  type: SQL
  text: &label SELECT 1
  parameters: [{id: custom, type: custom, defaultvalue: *base}]
  purpose: *label
  federation:
    expectedServerIdentity: &identity {baseUrl: https://example.com, serverId: 001}
    ovdbBaseUrl: https://example.com
  relationshipBindings:
    - id: 001
      version: 2026-10-21
      from: &endpoint {schema: main, table: First}
      to: *endpoint
      pairs: [{fromField: id, toField: id}]
folderpath: 001
`
	// Decode the owning nested node, including anchors outside its subtree.
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(data), &doc))
	var scoped QueryDefWithFolderPath
	require.NoError(t, doc.Content[0].Decode(&struct {
		Base       any       `yaml:"base"`
		QueryDef   *QueryDef `yaml:"querydef"`
		FolderPath string    `yaml:"folderpath"`
	}{QueryDef: &scoped.QueryDef}))
	require.Equal(t, "SELECT 1", scoped.Purpose)
	require.Equal(t, "001", scoped.RelationshipBindings[0].ID)
	require.Equal(t, "001", scoped.Federation.ExpectedServerIdentity.ServerID)
	require.Equal(t, scoped.RelationshipBindings[0].From, scoped.RelationshipBindings[0].To)
}
