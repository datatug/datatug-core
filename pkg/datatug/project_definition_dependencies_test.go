package datatug

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dependencyKeys(requests []ProjectDefinitionDependency) map[string]bool {
	out := map[string]bool{}
	for _, request := range requests {
		out[request.Kind+":"+request.ID] = true
	}
	return out
}

func TestProjectDefinitionDependenciesTraverseBothTabFormsAndCardRefs(t *testing.T) {
	var board Board
	require.NoError(t, json.Unmarshal([]byte(`{"id":"overview","title":"Overview","rows":[{"cards":[{"id":"a","ref":{"cardId":"customer"}},{"id":"b","widget":{"name":"tabs","data":{"parameters":[{"id":"widgetParam","type":"string","meta":{"entity":"Customer","field":"Name"}}],"tabs":[{"title":"Single","widget":{"name":"SQL","data":{"sql":{"queryId":"same-name"}}}},{"title":"Multiple","widgets":[{"name":"SQL","data":{"sql":{"queryId":"reports/same-name"}}},{"name":"chart","data":{"sources":[{"id":"chart","queryId":"reports/chart"}],"chart":{"type":"bar","categoryColumn":"country","valueColumn":"total"}}}]}]}}}]}]}`), &board))
	requests, err := ProjectBoardDependencies(board)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"card:customer": true, "query:same-name": true, "query:reports/same-name": true, "query:reports/chart": true, "entity-field:": true}, dependencyKeys(requests))
	for _, request := range requests {
		if request.Kind == "entity-field" {
			assert.Equal(t, EntityFieldRef{Entity: "Customer", Field: "Name"}, request.EntityField)
		}
	}
}

func TestProjectDefinitionDependenciesPreserveSourceContracts(t *testing.T) {
	query := QueryDefWithFolderPath{FolderPath: "", QueryDef: queryDefWithEveryPersistedField()}
	query.Recordsets[0].ForeignKeys[0].RefTable = NewTableKey("Customer", "public", "sales", nil)
	requests, err := ProjectQueryDependencies(query)
	require.NoError(t, err)
	keys := dependencyKeys(requests)
	for _, key := range []string{"connection:chinook-sqlite", "source:" + query.Capture.Source, "entity-field:", "lookup:", "recordset:", "target:", "federation:", "foreign-key:", "query-source:"} {
		assert.True(t, keys[key], key)
	}
	for _, request := range requests {
		if request.Kind == "federation" {
			var source QueryFederation
			require.NoError(t, json.Unmarshal(request.Data, &source))
			assert.Equal(t, *query.Federation, source)
		}
		if request.Kind == "foreign-key" {
			assert.Contains(t, string(request.Data), `"name":"Customer"`)
			assert.Contains(t, string(request.Data), `"catalog":"sales"`)
		}
	}
	// A bare physical name or a URL is never converted into a project database ID.
	assert.False(t, keys["database:chinook"])
	assert.False(t, keys["entity:Customer"])
	query.Recordsets[0].ForeignKeys[0].RefTable = DBCollectionKey{}
	out, err := ProjectQueryDependencies(query)
	require.ErrorIs(t, err, ErrProjectUsageIncomplete)
	assert.Nil(t, out)
}

func TestProjectDefinitionHTTPNestedRequestParameters(t *testing.T) {
	widget := BoardWidget{Name: "HTTP", Data: &HTTPWidgetDef{Request: HTTPRequest{Method: "GET", URL: "https://example.com/query-id", Parameters: Parameters{{ID: "id", Type: "string", Meta: &EntityFieldRef{Entity: "Customer", Field: "ID"}, Lookup: &ParameterLookup{DB: "db", SQL: "select 1"}}}}}}
	card := Card{ID: "http", Title: "HTTP", Widget: &widget}
	requests, err := ProjectCardDependencies(card)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"entity-field:": true, "lookup:": true}, dependencyKeys(requests))
}

func TestProjectDefinitionDependenciesRejectInvalidMetaAndUnknownNestedFields(t *testing.T) {
	query := QueryDefWithFolderPath{QueryDef: newQueryDef(QueryTypeSQL, "select 1")}
	query.Parameters = Parameters{{ID: "id", Type: "string", Meta: &EntityFieldRef{Entity: "Customer"}}}
	out, err := ProjectQueryDependencies(query)
	require.ErrorIs(t, err, ErrProjectUsageIncomplete)
	assert.Nil(t, out)
	for _, widget := range []string{
		`{"name":"SQL","data":{"sql":{"queryId":"query","futureReference":"hidden"}}}`,
		`{"name":"chart","data":{"sources":[{"id":"source","queryId":"query"}],"chart":{"type":"bar","categoryColumn":"country","valueColumn":"total","futureReference":"hidden"}}}`,
	} {
		var value BoardWidget
		require.NoError(t, json.Unmarshal([]byte(widget), &value))
		card := Card{ID: "card", Title: "Card", Widget: &value}
		out, err = ProjectCardDependencies(card)
		require.ErrorIs(t, err, ErrProjectUsageIncomplete)
		assert.Nil(t, out)
	}
}

func TestProjectDefinitionDependenciesDoNotInferMatchOrProseReferences(t *testing.T) {
	card := validCard()
	requests, err := ProjectCardDependencies(*card)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"query:customer-by-id": true}, dependencyKeys(requests))
	card.Parameters[0].Meta = &EntityFieldRef{Entity: "Customer", Field: "ID"}
	requests, err = ProjectCardDependencies(*card)
	require.NoError(t, err)
	assert.True(t, dependencyKeys(requests)["entity-field:"])
	card.Parameters[0].Meta.Field = ""
	requests, err = ProjectCardDependencies(*card)
	require.ErrorIs(t, err, ErrProjectUsageIncomplete)
	assert.Nil(t, requests)
}

func TestProjectDefinitionDependencyCollectorBoundsAndCopies(t *testing.T) {
	c := newProjectDependencyCollector()
	body := json.RawMessage(`{"source":"original"}`)
	require.NoError(t, c.add(ProjectDefinitionDependency{Kind: "target", Data: body}))
	body[11] = 'x'
	assert.JSONEq(t, `{"source":"original"}`, string(c.result()[0].Data))
	require.ErrorIs(t, c.add(ProjectDefinitionDependency{Kind: "target", Data: json.RawMessage(`"` + strings.Repeat("x", 4<<20) + `"`)}), ErrProjectUsageIncomplete)
	c.widgets = 2000
	require.ErrorIs(t, c.widget(*sqlWidget("query"), 0), ErrProjectUsageIncomplete)
	require.ErrorIs(t, newProjectDependencyCollector().widget(*sqlWidget("query"), maxWidgetValidationDepth+1), ErrProjectUsageIncomplete)
}
