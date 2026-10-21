package datatug

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func requireMeaningCapacity(t *testing.T, c ProjectMeaningsCatalog, err error) {
	t.Helper()
	if !errors.Is(err, ErrMeaningIncomplete) || c.Version != 0 {
		t.Fatalf("oversized projection escaped: %d %v", c.Version, err)
	}
}
func TestMeaningCatalogCombinedConceptBounds(t *testing.T) {
	scope := MeaningScope{Storage: "local", Project: "p"}
	model := validSemantic(t)
	d := meaningsTestDocument()
	d.Entities[0].Fields = nil
	for i := 0; i < 4093; i++ {
		d.Entities[0].Fields = append(d.Entities[0].Fields, MeaningField{MeaningIdentity: MeaningIdentity{"project", fmt.Sprintf("field%d", i)}, Name: "Field"})
	}
	q := []MeaningQuerySource{{ID: "q", SemanticModel: &model}}
	if c, err := BuildProjectMeaningsCatalog(d, q, nil, scope, true, true); err != nil || len(c.Entities) != 2 {
		t.Fatalf("boundary rejected: %v", err)
	}
	d.Entities[0].Fields = append(d.Entities[0].Fields, MeaningField{MeaningIdentity: MeaningIdentity{"project", "extra"}, Name: "Extra"})
	c, err := BuildProjectMeaningsCatalog(d, q, nil, scope, true, true)
	requireMeaningCapacity(t, c, err)
	// Entity-heavy projection exercises entity capacity before any query append.
	d = ProjectMeaningsDocument{Version: 1}
	for i := 0; i < 4096; i++ {
		d.Entities = append(d.Entities, MeaningEntity{MeaningIdentity: MeaningIdentity{"project", fmt.Sprintf("entity%d", i)}, Name: "Entity", Origin: "project"})
	}
	c, err = BuildProjectMeaningsCatalog(d, q, nil, scope, true, true)
	requireMeaningCapacity(t, c, err)
}
func TestMeaningCatalogResourceBounds(t *testing.T) {
	scope := MeaningScope{Storage: "local", Project: "p"}
	model := validSemantic(t)
	q := []MeaningQuerySource{{ID: "q", SemanticModel: &model}}
	boards := []MeaningBoardSource{}
	for i := 0; i < 4095; i++ {
		boards = append(boards, MeaningBoardSource{ID: fmt.Sprintf("board%d", i), QueryIDs: []string{"q"}})
	}
	empty := ProjectMeaningsDocument{Version: 1}
	c, err := BuildProjectMeaningsCatalog(empty, q, boards, scope, true, true)
	if err != nil || len(c.Entities[0].Resources) != 4096 {
		t.Fatalf("resource boundary: %v", err)
	}
	boards = append(boards, MeaningBoardSource{ID: "one-more", QueryIDs: []string{"q"}})
	c, err = BuildProjectMeaningsCatalog(empty, q, boards, scope, true, true)
	requireMeaningCapacity(t, c, err)
	// Two query snapshots can overflow the same standalone target independently
	// of each query's own valid resource count.
	model.Meaning.Concepts[0].MeaningRef = &MeaningReference{MeaningIdentity: MeaningIdentity{"project", "person"}, Scope: scope}
	q = append(q, MeaningQuerySource{ID: "second", SemanticModel: &model})
	boards = boards[:4094]
	boards = append(boards, MeaningBoardSource{ID: "second-board", QueryIDs: []string{"second"}})
	c, err = BuildProjectMeaningsCatalog(meaningsTestDocument(), q, boards, scope, true, true)
	requireMeaningCapacity(t, c, err)
}
func TestMeaningCatalogEncodedBudget(t *testing.T) {
	scope := MeaningScope{Storage: "local", Project: "p"}
	for _, description := range []string{strings.Repeat("x", 4096), strings.Repeat("\"", 4096)} {
		d := ProjectMeaningsDocument{Version: 1}
		for i := 0; i < 2100; i++ {
			d.Entities = append(d.Entities, MeaningEntity{MeaningIdentity: MeaningIdentity{"project", fmt.Sprintf("entity%d", i)}, Origin: "project", Name: "Entity", Description: description})
		}
		c, err := BuildProjectMeaningsCatalog(d, nil, nil, scope, true, true)
		requireMeaningCapacity(t, c, err)
	}
	hugeScope := scope
	hugeScope.Project = strings.Repeat("x", maxMeaningCatalogBytes+1)
	c, err := BuildProjectMeaningsCatalog(ProjectMeaningsDocument{Version: 1}, nil, nil, hugeScope, true, true)
	requireMeaningCapacity(t, c, err)
	model := validSemantic(t)
	c, err = BuildProjectMeaningsCatalog(ProjectMeaningsDocument{Version: 1}, []MeaningQuerySource{{ID: strings.Repeat("x", maxMeaningCatalogBytes+1), SemanticModel: &model}}, nil, scope, true, true)
	requireMeaningCapacity(t, c, err)
	// Long escaped identities consume their actual encoded bytes rather than
	// their raw string length; the skeleton remains bounded before allocation.
	hugeScope.Project = strings.Repeat("\"", maxMeaningCatalogBytes/2)
	c, err = BuildProjectMeaningsCatalog(ProjectMeaningsDocument{Version: 1}, nil, nil, hugeScope, true, true)
	requireMeaningCapacity(t, c, err)
	budget, err := newMeaningCatalogBudget(scope, true, true)
	if err != nil {
		t.Fatal(err)
	}
	resource := MeaningResource{Kind: "query", ID: "q", Title: "Q"}
	var list []MeaningResource
	if err = budget.resources(&list, []MeaningResource{resource, resource}); err != nil || len(list) != 1 {
		t.Fatal("resource dedupe")
	}
	if err = budget.charge(strings.Repeat("x", maxMeaningCatalogBytes), maxMeaningCatalogBytes, false); !errors.Is(err, ErrMeaningIncomplete) {
		t.Fatal("raw budget not enforced")
	}
	data, _ := json.Marshal(ProjectMeaningsCatalog{Scope: scope, Version: 1, Revision: strings.Repeat("0", 64), Writable: true, ReferencesComplete: true, Entities: []MeaningEntity{}})
	if budget.bytes != len(data)+len(`{"kind":"query","id":"q","title":"Q"}`) {
		t.Fatal("unexpected encoded accounting")
	}
}

func TestMeaningDatabaseResourceCollectionCapacity(t *testing.T) {
	m := validSemantic(t)
	q := MeaningQuerySource{ID: "q", SemanticModel: &m}
	for i := 0; i < 4096; i++ {
		q.Databases = append(q.Databases, MeaningResource{Kind: "database", ID: fmt.Sprintf("db%d", i), Title: "DB", Environment: "dev"})
	}
	d := ProjectMeaningsDocument{Version: 1}
	scope := MeaningScope{Storage: "local", Project: "p"}
	c, err := BuildProjectMeaningsCatalog(d, []MeaningQuerySource{q}, nil, scope, true, true)
	requireMeaningCapacity(t, c, err)
	q.Databases = q.Databases[:4095]
	q.Database = &MeaningResource{Kind: "database", ID: "extra", Title: "DB", Environment: "dev"}
	c, err = BuildProjectMeaningsCatalog(d, []MeaningQuerySource{q}, nil, scope, true, true)
	requireMeaningCapacity(t, c, err)
	q.Database = &q.Databases[0]
	if _, err = BuildProjectMeaningsCatalog(d, []MeaningQuerySource{q}, nil, scope, true, true); err != nil {
		t.Fatal("dedupe refused", err)
	}
	r := MeaningResource{Kind: "query", ID: "q", Title: "Q"}
	if got := appendMeaningResource([]MeaningResource{r}, r); len(got) != 1 {
		t.Fatal("duplicate database association")
	}
}
func TestMeaningStandaloneFieldEncodedCapacity(t *testing.T) {
	d := meaningsTestDocument()
	for i := 0; i < 2100; i++ {
		d.Entities[0].Fields = append(d.Entities[0].Fields, MeaningField{MeaningIdentity: MeaningIdentity{"project", fmt.Sprintf("padding%d", i)}, Name: "Padding", Description: strings.Repeat("x", 4096)})
	}
	c, err := BuildProjectMeaningsCatalog(d, nil, nil, MeaningScope{Storage: "local", Project: "p"}, true, true)
	requireMeaningCapacity(t, c, err)
}

// Pad a valid standalone document to an exact serialized catalog-base size,
// without exceeding a concept's text limit. This exercises the byte boundary
// through Build, including all call sites that append inherited resources.
func padMeaningDocument(t *testing.T, d ProjectMeaningsDocument, scope MeaningScope, target int) ProjectMeaningsDocument {
	t.Helper()
	budget, err := newMeaningCatalogBudget(scope, true, true)
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range d.Entities {
		if err = budget.entity(e, i > 0); err != nil {
			t.Fatal(err)
		}
		for j, f := range e.Fields {
			if err = budget.field(f, j > 0); err != nil {
				t.Fatal(err)
			}
		}
	}
	for budget.bytes < target {
		e := MeaningEntity{MeaningIdentity: MeaningIdentity{"project", fmt.Sprintf("padding%d", len(d.Entities))}, Origin: "project", Name: "Padding", Fields: []MeaningField{}, Resources: []MeaningResource{}}
		data, _ := json.Marshal(e)
		base := len(data) + 1
		remaining := target - budget.bytes
		if remaining < base {
			last := &d.Entities[len(d.Entities)-1]
			last.Description += strings.Repeat("x", remaining)
			budget.bytes += remaining
			break
		}
		size := remaining - base
		if size > 3500 {
			size = 3500
		}
		e.Description = strings.Repeat("x", size)
		if err = budget.entity(e, true); err != nil {
			t.Fatal(err)
		}
		d.Entities = append(d.Entities, e)
	}
	return d
}
func TestMeaningCatalogExactEncodedBoundaryAndResourceAppends(t *testing.T) {
	scope := MeaningScope{Storage: "local", Project: "p"}
	model := validSemantic(t)
	model.Meaning.Concepts[0].MeaningRef = &MeaningReference{MeaningIdentity: MeaningIdentity{"project", "person"}, Scope: scope}
	model.Meaning.Concepts[1].MeaningRef = &MeaningReference{MeaningIdentity: MeaningIdentity{"project", "person-name"}, Scope: scope}
	resource := MeaningResource{Kind: "query", ID: "q", Title: "Q"}
	resourceJSON, _ := json.Marshal(resource)
	entity := MeaningEntity{MeaningIdentity: MeaningIdentity{QueryMeaningDocumentID("q", model.Meaning.ID), "person"}, Origin: "query", Name: "Person", Description: "A person", Fields: []MeaningField{}, Resources: []MeaningResource{}}
	field := MeaningField{MeaningIdentity: MeaningIdentity{entity.DocumentID, "person-name"}, Name: "Name", Description: "Name of a person", Resources: []MeaningResource{}}
	entityJSON, _ := json.Marshal(entity)
	fieldJSON, _ := json.Marshal(field)
	queryBase := len(entityJSON) + 1 + len(fieldJSON)
	for _, tc := range []struct {
		name          string
		allowed       int
		propertyFirst bool
	}{{"query entity", 0, false}, {"query field", 1, false}, {"standalone field", 3, false}, {"standalone parent", 3, true}, {"exact boundary", 4, false}} {
		t.Run(tc.name, func(t *testing.T) {
			d := padMeaningDocument(t, meaningsTestDocument(), scope, maxMeaningCatalogBytes-queryBase-tc.allowed*len(resourceJSON))
			m := model
			if tc.propertyFirst {
				m.Meaning.Concepts = append([]QueryMeaningConcept{}, model.Meaning.Concepts...)
				m.Meaning.Concepts[0], m.Meaning.Concepts[1] = m.Meaning.Concepts[1], m.Meaning.Concepts[0]
			}
			c, err := BuildProjectMeaningsCatalog(d, []MeaningQuerySource{{ID: "q", Title: "Q", SemanticModel: &m}}, nil, scope, true, true)
			if tc.allowed == 4 {
				if err != nil {
					t.Fatal(err)
				}
				data, _ := json.Marshal(c)
				if len(data) != maxMeaningCatalogBytes {
					t.Fatalf("boundary size=%d", len(data))
				}
				d.Entities[1].Description += "x"
				c, err = BuildProjectMeaningsCatalog(d, []MeaningQuerySource{{ID: "q", Title: "Q", SemanticModel: &m}}, nil, scope, true, true)
				requireMeaningCapacity(t, c, err)
			} else {
				requireMeaningCapacity(t, c, err)
			}
		})
	}
}
