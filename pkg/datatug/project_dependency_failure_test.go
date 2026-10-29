package datatug

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestDependencyProjectionDiscardsAllOutputWhenEncodingFails(t *testing.T) {
	original := marshalProjectDependency
	t.Cleanup(func() { marshalProjectDependency = original })
	q := QueryDefWithFolderPath{QueryDef: queryDefWithEveryPersistedField()}
	q.Recordsets[0].ForeignKeys[0].RefTable = NewTableKey("Customer", "public", "sales", nil)
	semantic := validSemantic(t)
	semantic.Meaning.Concepts[0].MeaningRef = &MeaningReference{MeaningIdentity: MeaningIdentity{"project", "person"}, Scope: MeaningScope{Storage: "local", Project: "p"}}
	q.SemanticModel = &semantic
	q.RelationshipBindings = QueryRelationshipBindings{{ID: "r", Version: "v1", From: QueryRelationshipEndpoint{Table: "a"}, To: QueryRelationshipEndpoint{Table: "b"}, Pairs: []QueryRelationshipFieldPair{{FromField: "id", ToField: "id"}}}}
	for _, kind := range []string{"connection", "entity-field", "lookup", "recordset", "foreign-key", "meaning", "source", "target", "federation", "query-source", "relationships"} {
		t.Run(kind, func(t *testing.T) {
			reached := false
			marshalProjectDependency = func(v any) ([]byte, error) {
				if d, ok := v.(ProjectDefinitionDependency); ok && d.Kind == kind {
					reached = true
					return nil, errors.New("codec unavailable")
				}
				return original(v)
			}
			out, err := ProjectQueryDependencies(q)
			if !reached || !errors.Is(err, ErrProjectUsageIncomplete) || out != nil {
				t.Fatalf("failed %s encoding exposed partial dependencies: %v %v", kind, out, err)
			}
		})
	}
	marshalProjectDependency = original
	invalid := newProjectDependencyCollector()
	if err := invalid.add(ProjectDefinitionDependency{Data: json.RawMessage(`{`)}); !errors.Is(err, ErrProjectUsageIncomplete) {
		t.Fatal(err)
	}
	if err := invalid.opaque("unsupported", make(chan int)); !errors.Is(err, ErrProjectUsageIncomplete) {
		t.Fatal(err)
	}
	if len(invalid.result()) != 0 {
		t.Fatal("failed codec changed collector")
	}
	if err := invalid.add(ProjectDefinitionDependency{Kind: "query", ID: "q"}); err != nil {
		t.Fatal(err)
	}
	if err := invalid.add(ProjectDefinitionDependency{Kind: "query", ID: "q"}); err != nil || len(invalid.result()) != 1 {
		t.Fatal("duplicate request consumed capacity")
	}
}

func TestDependencyWidgetContractsAndCapacityErrors(t *testing.T) {
	original := marshalProjectDependency
	t.Cleanup(func() { marshalProjectDependency = original })
	widgets := []BoardWidget{
		{Name: "SQL", Data: map[string]any{"sql": map[string]string{"queryId": "q"}}},
		{Name: "chart", Data: map[string]any{"sources": []map[string]string{{"id": "source", "queryId": "q"}}, "chart": map[string]string{"type": "bar", "categoryColumn": "x", "valueColumn": "y"}}},
		{Name: "fields", Data: map[string]any{"source": map[string]string{"queryId": "q"}}},
		{Name: "text", Data: map[string]string{"markdown": "prose is not a query"}},
	}
	for _, w := range widgets {
		t.Run(w.Name, func(t *testing.T) {
			c := newProjectDependencyCollector()
			if err := c.widget(w, 0); err != nil {
				t.Fatal(err)
			}
			if w.Name != "text" && !dependencyKeys(c.result())["query:q"] {
				t.Fatal("typed query lost")
			}
			bad := BoardWidget{Name: w.Name, Data: map[string]any{"futureReference": "hidden"}}
			if err := newProjectDependencyCollector().widget(bad, 0); !errors.Is(err, ErrProjectUsageIncomplete) {
				t.Fatal("unknown widget fields accepted")
			}
			if w.Name != "text" {
				c = newProjectDependencyCollector()
				c.bytes = 4 << 20
				if err := c.widget(w, 0); !errors.Is(err, ErrProjectUsageIncomplete) {
					t.Fatal("capacity overflow accepted")
				}
			}
		})
	}
	for _, name := range []string{"tabs", "HTTP"} {
		if err := newProjectDependencyCollector().widget(BoardWidget{Name: name, Data: map[string]bool{"future": true}}, 0); !errors.Is(err, ErrProjectUsageIncomplete) {
			t.Fatal(name)
		}
	}
	for _, w := range []BoardWidget{{Name: "future"}, {Name: "text", Data: make(chan int)}, {Name: "text", Data: strings.Repeat("x", 1<<20)}} {
		if err := newProjectDependencyCollector().widget(w, 0); !errors.Is(err, ErrProjectUsageIncomplete) {
			t.Fatal("unprojectable widget accepted")
		}
	}
	for _, data := range []map[string]any{
		{"tabs": []map[string]any{{"title": "single", "widget": map[string]any{"name": "future"}}}},
		{"tabs": []map[string]any{{"title": "multi", "widgets": []map[string]any{{"name": "future"}}}}},
	} {
		if err := newProjectDependencyCollector().widget(BoardWidget{Name: "tabs", Data: data}, 0); !errors.Is(err, ErrProjectUsageIncomplete) {
			t.Fatal("nested failure swallowed")
		}
	}
	c := newProjectDependencyCollector()
	c.bytes = 4 << 20
	for _, p := range []Parameters{
		{{Meta: &EntityFieldRef{Entity: "Customer", Field: "ID"}}},
		{{Lookup: &ParameterLookup{DB: "db", SQL: "select 1"}}},
	} {
		if err := c.parameters(p); !errors.Is(err, ErrProjectUsageIncomplete) {
			t.Fatal("parameter overflow swallowed")
		}
	}
	http := BoardWidget{Name: "HTTP", Data: &HTTPWidgetDef{Request: HTTPRequest{Parameters: Parameters{{Meta: &EntityFieldRef{Entity: "Customer"}}}}}}
	if err := newProjectDependencyCollector().widget(http, 0); !errors.Is(err, ErrProjectUsageIncomplete) {
		t.Fatal("invalid HTTP parameter accepted")
	}
}

func TestDependencyDefinitionsValidateAndPropagateProviderFailures(t *testing.T) {
	if out, err := ProjectQueryDependencies(QueryDefWithFolderPath{}); !errors.Is(err, ErrProjectUsageIncomplete) || out != nil {
		t.Fatal("invalid query accepted")
	}
	if out, err := ProjectBoardDependencies(Board{}); !errors.Is(err, ErrProjectUsageIncomplete) || out != nil {
		t.Fatal("invalid board accepted")
	}
	if out, err := ProjectEntityDependencies(Entity{}); !errors.Is(err, ErrProjectUsageIncomplete) || out != nil {
		t.Fatal("invalid entity accepted")
	}
	entity := Entity{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "Customer"}}, Fields: EntityFields{{ID: "id", Type: "string"}}, Tables: TableKeys{NewTableKey("Customer", "public", "sales", nil)}}
	out, err := ProjectEntityDependencies(entity)
	if err != nil || len(out) != 1 || out[0].Kind != "entity-source" {
		t.Fatalf("physical entity lost: %v %v", out, err)
	}
	entity.Tables = nil
	out, err = ProjectEntityDependencies(entity)
	if err != nil || len(out) != 0 {
		t.Fatal("logical entity inferred physical dependency", err)
	}
	entity.Tables = TableKeys{NewTableKey("Customer", "public", "sales", nil)}
	original := marshalProjectDependency
	t.Cleanup(func() { marshalProjectDependency = original })
	marshalProjectDependency = func(v any) ([]byte, error) {
		if _, ok := v.(ProjectDefinitionDependency); ok {
			return nil, errors.New("quota")
		}
		return original(v)
	}
	if out, err = ProjectEntityDependencies(entity); !errors.Is(err, ErrProjectUsageIncomplete) || out != nil {
		t.Fatal("entity error swallowed")
	}
	// Independently valid boards exercise both reference and inline-widget failures.
	for _, raw := range []string{
		`{"id":"b","title":"B","parameters":[{"id":"p","type":"string","meta":{"entity":"Customer","field":"ID"}}]}`,
		`{"id":"b","title":"B","rows":[{"cards":[{"id":"c","ref":{"cardId":"card"}}]}]}`,
		`{"id":"b","title":"B","rows":[{"cards":[{"id":"c","widget":{"name":"SQL","data":{"sql":{"queryId":"q"}}}}]}]}`,
	} {
		var b Board
		if err = json.Unmarshal([]byte(raw), &b); err != nil {
			t.Fatal(err)
		}
		if out, err = ProjectBoardDependencies(b); !errors.Is(err, ErrProjectUsageIncomplete) || out != nil {
			t.Fatal("board error swallowed", err)
		}
	}
	marshalProjectDependency = original
	query := QueryDefWithFolderPath{QueryDef: queryDefWithEveryPersistedField()}
	query.Recordsets[0].ForeignKeys[0] = nil
	if out, err = ProjectQueryDependencies(query); !errors.Is(err, ErrProjectUsageIncomplete) || out != nil {
		t.Fatal("nil foreign key accepted")
	}
	query = QueryDefWithFolderPath{QueryDef: queryDefWithEveryPersistedField()}
	query.Recordsets[0].Columns[0].Meta = &EntityFieldRef{Entity: "Customer"}
	if out, err = ProjectQueryDependencies(query); !errors.Is(err, ErrProjectUsageIncomplete) || out != nil {
		t.Fatal("invalid column meta accepted")
	}
	semantic := validSemantic(t)
	semantic.Meaning.Concepts[0].MeaningRef = &MeaningReference{MeaningIdentity: MeaningIdentity{"project", "person"}, Scope: MeaningScope{Storage: "local", Project: "p"}}
	query = QueryDefWithFolderPath{QueryDef: newQueryDef(QueryTypeSQL, "select 1")}
	query.SemanticModel = &semantic
	oldValidate := validateProjectDependencyMeaning
	t.Cleanup(func() { validateProjectDependencyMeaning = oldValidate })
	validateProjectDependencyMeaning = func(MeaningReference) error { return errors.New("meaning registry unavailable") }
	if out, err = ProjectQueryDependencies(query); !errors.Is(err, ErrProjectUsageIncomplete) || out != nil {
		t.Fatal("meaning revalidation failure swallowed")
	}
}

func TestUsageRevisionCanonicalReferencesAndCodecFailure(t *testing.T) {
	a := ProjectObjectUsage{Ref: ProjectObjectRef{Kind: "query", ID: "a"}, ContentRevision: "1"}
	b := ProjectObjectUsage{Ref: ProjectObjectRef{Kind: "query", ID: "b"}, ContentRevision: "1"}
	c := ProjectObjectUsage{Ref: ProjectObjectRef{Kind: "board", ID: "c"}, ContentRevision: "1", References: []ProjectObjectRef{b.Ref, a.Ref}}
	before := append([]ProjectObjectRef(nil), c.References...)
	first, err := ProjectUsageRevision([]ProjectObjectUsage{a, b, c})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, c.References) {
		t.Fatal("revision sort mutated input")
	}
	c.References = []ProjectObjectRef{a.Ref, b.Ref}
	second, err := ProjectUsageRevision([]ProjectObjectUsage{c, b, a})
	if err != nil || first != second {
		t.Fatal("reference order changed digest", err)
	}
	old := marshalProjectUsageRevision
	t.Cleanup(func() { marshalProjectUsageRevision = old })
	failure := errors.New("codec")
	marshalProjectUsageRevision = func(any) ([]byte, error) { return nil, failure }
	if rev, err := ProjectUsageRevision([]ProjectObjectUsage{a}); rev != "" || !errors.Is(err, failure) {
		t.Fatal("failed digest exposed revision", rev, err)
	}
}
