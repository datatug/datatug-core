package datatug

import (
	"encoding/json"
	"errors"
	"testing"
)

func meaningsTestDocument() ProjectMeaningsDocument {
	return ProjectMeaningsDocument{Version: 1, Entities: []MeaningEntity{{MeaningIdentity: MeaningIdentity{"project", "person"}, Origin: "project", Name: "Person", Fields: []MeaningField{{MeaningIdentity: MeaningIdentity{"project", "person-name"}, Name: "Name"}}}}}
}
func TestProjectMeaningCatalogIdentityAndReferences(t *testing.T) {
	d := meaningsTestDocument()
	scope := MeaningScope{Storage: "local", Project: "p", Branch: "main"}
	model := validSemantic(t)
	model.Meaning.Concepts[0].MeaningRef = &MeaningReference{MeaningIdentity: MeaningIdentity{"project", "person"}, Scope: scope}
	model.Meaning.Concepts[1].MeaningRef = &MeaningReference{MeaningIdentity: MeaningIdentity{"project", "person-name"}, Scope: scope}
	encoded, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	var decoded QuerySemanticModel
	if err = json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err = decoded.Validate(); err != nil {
		t.Fatal(err)
	}
	if err = ValidateProjectMeaningReferences(&model, d, scope); err != nil {
		t.Fatal(err)
	}
	queries := []MeaningQuerySource{{ID: "a:b", SemanticModel: &model}, {ID: "a", SemanticModel: &model}}
	boards := []MeaningBoardSource{{ID: "board", Title: "Board", QueryIDs: []string{"a:b"}}}
	c, err := BuildProjectMeaningsCatalog(d, queries, boards, scope, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Entities) != 3 {
		t.Fatalf("merged documents: %#v", c.Entities)
	}
	for _, e := range c.Entities {
		if e.Origin == "project" {
			if len(e.Resources) != 3 || len(e.Fields[0].Resources) != 3 {
				t.Fatalf("missing inbound resources: %#v", e)
			}
		}
	}
	w := ProjectMeaningWrite{OperationID: "delete-1", IfMatch: c.Revision, Change: ProjectMeaningAction{Action: "delete-entity", Entity: MeaningIdentity{"project", "person"}}}
	if _, _, err = ApplyProjectMeaningChange(d, c, w); !errors.Is(err, ErrMeaningReferenced) {
		t.Fatalf("delete referenced: %v", err)
	}
	w.Change = ProjectMeaningAction{Action: "delete-field", Entity: MeaningIdentity{"project", "person"}, Field: MeaningIdentity{"project", "person-name"}}
	if _, _, err = ApplyProjectMeaningChange(d, c, w); !errors.Is(err, ErrMeaningReferenced) {
		t.Fatalf("delete referenced field: %v", err)
	}
	foreign := scope
	foreign.Project = "other"
	if err = ValidateProjectMeaningReferences(&model, d, foreign); err == nil {
		t.Fatal("cross project ref accepted")
	}
	model.Meaning.Concepts[0].MeaningRef.ConceptID = "person-name"
	if err = ValidateProjectMeaningReferences(&model, d, scope); err == nil {
		t.Fatal("property accepted as entity")
	}
	if QueryMeaningDocumentID("a:b", "c") == QueryMeaningDocumentID("a", "b:c") {
		t.Fatal("ambiguous document identity")
	}
}
func TestProjectMeaningMutationRevisionReceiptAndCompleteness(t *testing.T) {
	d := ProjectMeaningsDocument{Version: 1}
	scope := MeaningScope{Storage: "local", Project: "p"}
	c, err := BuildProjectMeaningsCatalog(d, nil, nil, scope, true, true)
	if err != nil {
		t.Fatal(err)
	}
	w := ProjectMeaningWrite{OperationID: "operation-1", IfMatch: c.Revision, Change: ProjectMeaningAction{Action: "add-entity", ConceptID: "person", Name: "Person"}}
	next, ack, err := ApplyProjectMeaningChange(d, c, w)
	if err != nil || ack.Receipt != w.OperationID || len(next.Entities) != 1 {
		t.Fatalf("apply: %#v %v", ack, err)
	}
	updated, _ := BuildProjectMeaningsCatalog(next, nil, nil, scope, true, true)
	if updated.Revision == c.Revision {
		t.Fatal("revision unchanged")
	}
	replay, ack, err := ApplyProjectMeaningChange(next, updated, w)
	if err != nil || len(replay.Entities) != 1 || ack.Receipt != w.OperationID {
		t.Fatalf("replay %v", err)
	}
	w.Change.Name = "Changed"
	if _, _, err = ApplyProjectMeaningChange(next, updated, w); !errors.Is(err, ErrMeaningOperationConflict) {
		t.Fatalf("digest mismatch %v", err)
	}
	w.OperationID = "operation-2"
	if _, _, err = ApplyProjectMeaningChange(next, updated, w); !errors.Is(err, ErrMeaningConflict) {
		t.Fatalf("stale revision %v", err)
	}
	updated.ReferencesComplete = false
	w.IfMatch = updated.Revision
	if _, _, err = ApplyProjectMeaningChange(next, updated, w); !errors.Is(err, ErrMeaningIncomplete) {
		t.Fatalf("incomplete %v", err)
	}
}
func TestMeaningDatabasesUseExactDeclaredTargets(t *testing.T) {
	env := &Environment{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "dev"}}, DbServers: EnvDbServers{{ServerRef: ServerRef{Driver: "postgres", Host: "db", Port: 5432}, Catalogs: []string{"sales"}}}}
	q := QueryDef{Targets: []QueryDefTarget{{Driver: "postgres", Host: "db", Port: 5432, Catalog: "sales"}, {Driver: "postgres", Host: "other", Port: 5432, Catalog: "sales"}}}
	q.Targets = append(q.Targets, QueryDefTarget{})
	r := MeaningDatabasesForQueryTargets(q, Environments{nil, env})
	if len(r) != 1 || r[0].ID != "sales" || r[0].Environment != "dev" {
		t.Fatalf("bad provenance %#v", r)
	}
	q.Targets[0].Host = "other"
	if len(MeaningDatabasesForQueryTargets(q, Environments{env})) != 0 {
		t.Fatal("matched database by label")
	}
}
