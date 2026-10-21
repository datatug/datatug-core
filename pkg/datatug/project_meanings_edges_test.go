package datatug

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestMeaningReferenceAndDocumentBoundaries(t *testing.T) {
	scope := MeaningScope{Storage: "local", Project: "p"}
	base := MeaningReference{MeaningIdentity: MeaningIdentity{"project", "person"}, Scope: scope}
	for name, mutate := range map[string]func(*MeaningReference){"document": func(r *MeaningReference) { r.DocumentID = "other" }, "scope": func(r *MeaningReference) { r.Scope.Project = "" }, "space": func(r *MeaningReference) { r.Scope.SpaceID = "space" }, "branch": func(r *MeaningReference) { r.Scope.Storage = "firestore"; r.Scope.Branch = "main" }, "storage": func(r *MeaningReference) { r.Scope.Storage = "unknown" }} {
		t.Run(name, func(t *testing.T) {
			r := base
			mutate(&r)
			if r.Validate() == nil {
				t.Fatal("invalid ref accepted")
			}
		})
	}
	base.Scope.Storage = "firestore"
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*ProjectMeaningsDocument){"version": func(d *ProjectMeaningsDocument) { d.Version = 2 }, "entity origin": func(d *ProjectMeaningsDocument) { d.Entities[0].Origin = "query" }, "entity text": func(d *ProjectMeaningsDocument) { d.Entities[0].Name = "" }, "entity duplicate": func(d *ProjectMeaningsDocument) { d.Entities = append(d.Entities, d.Entities[0]) }, "field origin": func(d *ProjectMeaningsDocument) { d.Entities[0].Fields[0].DocumentID = "query" }, "field text": func(d *ProjectMeaningsDocument) { d.Entities[0].Fields[0].Name = "" }, "field duplicate": func(d *ProjectMeaningsDocument) { d.Entities[0].Fields[0].ConceptID = "person" }, "capacity": func(d *ProjectMeaningsDocument) {
		d.Receipts = map[string]MeaningOperationReceipt{}
		for i := 0; i < 10001; i++ {
			d.Receipts[fmt.Sprint(i)] = MeaningOperationReceipt{}
		}
	}}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			d := meaningsTestDocument()
			mutate(&d)
			if d.Validate() == nil {
				t.Fatal("invalid doc accepted")
			}
		})
	}
	model := validSemantic(t)
	model.Meaning.Concepts[0].MeaningRef = &MeaningReference{}
	if model.Validate() == nil {
		t.Fatal("invalid model reference accepted")
	}
	if err := semanticRequired(map[string]any{}, reflect.TypeOf(MeaningReference{})); err == nil {
		t.Fatal("missing promoted field accepted")
	}
	if validateMeaningText("person", "https://user:password@example.com", "") == nil {
		t.Fatal("credential in text accepted")
	}
}
func TestMeaningProjectionRejectsIncompleteProvenance(t *testing.T) {
	scope := MeaningScope{Storage: "local", Project: "p"}
	d := meaningsTestDocument()
	d.Entities = append(d.Entities, MeaningEntity{MeaningIdentity: MeaningIdentity{"project", "account"}, Name: "Account", Origin: "project"})
	d.Entities[0].Fields = append(d.Entities[0].Fields, MeaningField{MeaningIdentity: MeaningIdentity{"project", "person-age"}, Name: "Age"})
	model := validSemantic(t)
	sources := []MeaningQuerySource{{ID: "query", SemanticModel: &model, Database: &MeaningResource{Kind: "database", ID: "db", Environment: "dev", Title: "DB"}, Databases: []MeaningResource{{Kind: "database", ID: "db2", Environment: "dev"}}}, {ID: "empty"}}
	c, err := BuildProjectMeaningsCatalog(d, sources, []MeaningBoardSource{{ID: "b", QueryIDs: []string{"query"}}}, scope, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Entities) != 3 {
		t.Fatal("lost entities")
	}
	bad := d
	bad.Version = 2
	if _, err = BuildProjectMeaningsCatalog(bad, nil, nil, scope, true, true); err == nil {
		t.Fatal("invalid doc accepted")
	}
	broken := model
	broken.Version = 2
	if _, err = BuildProjectMeaningsCatalog(d, []MeaningQuerySource{{SemanticModel: &broken}}, nil, scope, true, true); err == nil {
		t.Fatal("invalid source accepted")
	}
	sources[0].Databases[0].Environment = ""
	if _, err = BuildProjectMeaningsCatalog(d, sources, nil, scope, true, true); err == nil {
		t.Fatal("invalid plural provenance")
	}
	sources[0].Databases = nil
	sources[0].Database.Environment = ""
	if _, err = BuildProjectMeaningsCatalog(d, sources, nil, scope, true, true); err == nil {
		t.Fatal("invalid provenance")
	}
	sources[0].Database = nil
	model.Meaning.Concepts[0].MeaningRef = &MeaningReference{MeaningIdentity: MeaningIdentity{"project", "missing"}, Scope: scope}
	if _, err = BuildProjectMeaningsCatalog(d, sources, nil, scope, true, true); err == nil {
		t.Fatal("dangling target accepted")
	}
	model.Meaning.Concepts[0].MeaningRef.ConceptID = "account"
	model.Meaning.Concepts[1].MeaningRef = &MeaningReference{MeaningIdentity: MeaningIdentity{"project", "person-name"}, Scope: scope}
	if _, err = BuildProjectMeaningsCatalog(d, sources, nil, scope, true, true); err == nil {
		t.Fatal("wrong owner accepted")
	}
}
func TestMeaningMutationAdmissionAndCapacity(t *testing.T) {
	scope := MeaningScope{Storage: "local", Project: "p"}
	build := func(d ProjectMeaningsDocument) ProjectMeaningsCatalog {
		c, err := BuildProjectMeaningsCatalog(d, nil, nil, scope, true, true)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	d := meaningsTestDocument()
	c := build(d)
	base := ProjectMeaningWrite{OperationID: "operation-test", IfMatch: c.Revision}
	check := func(name string, doc ProjectMeaningsDocument, cat ProjectMeaningsCatalog, w ProjectMeaningWrite) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			if _, _, err := ApplyProjectMeaningChange(doc, cat, w); err == nil {
				t.Fatal("invalid mutation accepted")
			}
		})
	}
	ro := c
	ro.Writable = false
	check("read-only", d, ro, base)
	badID := base
	badID.OperationID = "short"
	check("operation ID", d, c, badID)
	badDoc := d
	badDoc.Version = 2
	check("invalid doc", badDoc, c, base)
	for _, a := range []ProjectMeaningAction{{Action: "add-entity", ConceptID: "bad", Name: ""}, {Action: "add-field", ConceptID: "field", Name: "Field"}, {Action: "add-field", Entity: MeaningIdentity{"project", "person"}, ConceptID: "bad", Name: ""}, {Action: "delete-entity", Entity: MeaningIdentity{"project", "missing"}}, {Action: "delete-field", Entity: MeaningIdentity{"project", "person"}, Field: MeaningIdentity{"project", "missing"}}, {Action: "unknown"}, {Action: "add-entity", ConceptID: "person", Name: "Duplicate"}} {
		w := base
		w.Change = a
		check(a.Action+a.ConceptID, d, c, w)
	}
	missing := c
	missing.Entities = nil
	w := base
	w.Change = ProjectMeaningAction{Action: "delete-entity", Entity: MeaningIdentity{"project", "person"}}
	check("missing catalog entity", d, missing, w)
	child := c
	child.Entities[0].Fields[0].Resources = []MeaningResource{{Kind: "query", ID: "q", Title: "Q"}}
	check("child references", d, child, w)
	c = build(d)
	w.Change = ProjectMeaningAction{Action: "add-field", Entity: MeaningIdentity{"project", "person"}, ConceptID: "person-age", Name: "Age"}
	next, _, err := ApplyProjectMeaningChange(d, c, w)
	if err != nil || len(next.Entities[0].Fields) != 2 {
		t.Fatalf("add field: %v", err)
	}
	w.Change = ProjectMeaningAction{Action: "delete-field", Entity: MeaningIdentity{"project", "person"}, Field: MeaningIdentity{"project", "person-name"}}
	next, _, err = ApplyProjectMeaningChange(d, c, w)
	if err != nil || len(next.Entities[0].Fields) != 0 {
		t.Fatalf("delete field %v", err)
	}
	w.Change = ProjectMeaningAction{Action: "delete-entity", Entity: MeaningIdentity{"project", "person"}}
	next, _, err = ApplyProjectMeaningChange(d, c, w)
	if err != nil || len(next.Entities) != 0 {
		t.Fatalf("delete entity %v", err)
	}
	d.Receipts = map[string]MeaningOperationReceipt{}
	for i := 0; i < 10000; i++ {
		d.Receipts[fmt.Sprint(i)] = MeaningOperationReceipt{}
	}
	c = build(d)
	w.IfMatch = c.Revision
	w.Change = ProjectMeaningAction{Action: "add-entity", ConceptID: "account", Name: "Account"}
	check("receipt capacity", d, c, w)
	if len(d.Receipts) != 10000 {
		t.Fatal("input mutated")
	}
}
func TestMeaningReferenceTargetFailures(t *testing.T) {
	scope := MeaningScope{Storage: "local", Project: "p"}
	d := meaningsTestDocument()
	if err := ValidateProjectMeaningReferences(nil, d, scope); err != nil {
		t.Fatal(err)
	}
	m := validSemantic(t)
	if err := ValidateProjectMeaningReferences(&m, d, scope); err != nil {
		t.Fatal(err)
	}
	bad := d
	bad.Version = 2
	if ValidateProjectMeaningReferences(&m, bad, scope) == nil {
		t.Fatal("bad doc accepted")
	}
	m.Meaning.Concepts[0].MeaningRef = &MeaningReference{}
	if ValidateProjectMeaningReferences(&m, d, scope) == nil {
		t.Fatal("bad ref accepted")
	}
	m.Meaning.Concepts[0].MeaningRef = nil
	m.Meaning.Concepts[1].MeaningRef = &MeaningReference{MeaningIdentity: MeaningIdentity{"project", "missing"}, Scope: scope}
	if ValidateProjectMeaningReferences(&m, d, scope) == nil {
		t.Fatal("missing target accepted")
	}
	m.Meaning.Concepts[1].MeaningRef.ConceptID = "person-name"
	if ValidateProjectMeaningReferences(&m, d, scope) == nil {
		t.Fatal("missing owner accepted")
	}
	m.Meaning.Concepts[1].Kind = "unknown"
	if ValidateProjectMeaningReferences(&m, d, scope) == nil {
		t.Fatal("wrong kind accepted")
	}
}
func TestMeaningBoardProjectionSupportedAndMalformedWidgets(t *testing.T) {
	board := func(w *BoardWidget) Board {
		return Board{Rows: BoardRows{{Cards: BoardCards{{Widget: w}, {Widget: nil}}}}}
	}
	sql := BoardWidget{Name: "SQL", Data: map[string]any{"sql": map[string]any{"queryId": "q"}}}
	chart := BoardWidget{Name: "chart", Data: map[string]any{"sources": []any{map[string]any{"id": "s", "queryId": "q"}}, "chart": map[string]any{}, "defaultView": ""}}
	tabs := BoardWidget{Name: "tabs", Data: map[string]any{"tabs": []any{map[string]any{"widget": sql}, map[string]any{"widget": chart}, map[string]any{"widget": BoardWidget{}}, map[string]any{"widget": BoardWidget{Name: "HTTP"}}}}}
	result, err := MeaningBoardSourceFromBoard(board(&tabs))
	if err != nil || len(result.QueryIDs) != 1 {
		t.Fatalf("supported projection %#v %v", result, err)
	}
	for _, w := range []BoardWidget{{Name: "SQL", Data: 0}, {Name: "chart", Data: 0}, {Name: "tabs", Data: 0}, {Name: "other"}, {Name: "SQL", Data: make(chan int)}} {
		if _, err = MeaningBoardSourceFromBoard(board(&w)); err == nil {
			t.Fatalf("malformed accepted %#v", w)
		}
	}
	nested := BoardWidget{Name: "SQL", Data: map[string]any{"sql": map[string]any{"queryId": ""}}}
	if _, err = MeaningBoardSourceFromBoard(board(&nested)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 34; i++ {
		nested = BoardWidget{Name: "tabs", Data: map[string]any{"tabs": []any{map[string]any{"widget": nested}}}}
	}
	if _, err = MeaningBoardSourceFromBoard(board(&nested)); err == nil || !strings.Contains(err.Error(), "depth") {
		t.Fatalf("depth limit: %v", err)
	}
	raw, _ := json.Marshal(result)
	if len(raw) == 0 {
		t.Fatal("empty source")
	}
}
