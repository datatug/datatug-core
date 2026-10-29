package datatug

import (
	"errors"
	"strings"
	"testing"
)

func TestProjectLifecycleTransportValidationAndUnavailableTargets(t *testing.T) {
	if _, err := ParseProjectConnectionsJSON([]byte(`{"format":1}`)); err == nil {
		t.Fatal("wrong typed catalog field accepted")
	}
	valid := ProjectObjectRef{Kind: "query", ID: "folder/query"}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []ProjectObjectRef{{Kind: "unknown", ID: "q"}, {Kind: "query", ID: " "}, {Kind: "query", ID: "a/../b"}} {
		if err := ref.Validate(); !errors.Is(err, ErrProjectLifecycleInvalid) {
			t.Fatal(err)
		}
	}
	w := ProjectLifecycleWrite{OperationID: "op", ExpectedRevision: "revision", Change: ProjectLifecycleChange{Object: valid, Action: "archive"}}
	if err := w.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"operation", "revision", "target", "action", "history"} {
		bad := w
		switch mode {
		case "operation":
			bad.OperationID = ""
		case "revision":
			bad.ExpectedRevision = ""
		case "target":
			bad.Change.Object.ID = "bad\\id"
		case "action":
			bad.Change.Action = "cascade"
		case "history":
			bad.Change.Object.Kind = "run"
		}
		if err := bad.Validate(); !errors.Is(err, ErrProjectLifecycleInvalid) {
			t.Fatal(mode, err)
		}
	}
	snapshot := ProjectUsageSnapshot{Revision: "revision", Complete: true}
	if plan, err := PlanProjectLifecycle(snapshot, w); !errors.Is(err, ErrProjectLifecycleConflict) || len(plan.Blockers) != 0 {
		t.Fatal("missing target admitted", err)
	}
	snapshot.Objects = []ProjectObjectUsage{{Ref: valid, ContentRevision: "v1", References: []ProjectObjectRef{{Kind: "query", ID: "bad/../ref"}}}}
	if _, err := PlanProjectLifecycle(snapshot, w); !errors.Is(err, ErrProjectUsageIncomplete) {
		t.Fatal("invalid dependency admitted", err)
	}
	if _, err := ParseProjectConnectionsJSON([]byte(`{"format":"datatug-demo-connections/v1","format":"datatug-demo-connections/v1"}`)); err == nil {
		t.Fatal("duplicate catalog field accepted")
	}
	if validLogDatabase("https://bad%zz.example/v1/databases/db") {
		t.Fatal("malformed origin accepted")
	}
	if _, catalog, err := ParsePublicProjectConnectionDirectoryJSON("catalog.json", []byte(strings.Repeat("x", MaxPublicConnectionCatalogBytes+1))); err == nil || catalog {
		t.Fatal("oversized directory entry admitted")
	}
}

func TestDependencyColumnRevalidationAndEncodingFailure(t *testing.T) {
	q := QueryDefWithFolderPath{QueryDef: queryDefWithEveryPersistedField()}
	q.Recordsets[0].ForeignKeys[0].RefTable = NewTableKey("Customer", "public", "sales", nil)
	q.Recordsets[0].Columns[0].Meta = &EntityFieldRef{Entity: "ColumnEntity", Field: "ID"}
	validate, marshal := validateProjectDependencyEntityField, marshalProjectDependency
	t.Cleanup(func() { validateProjectDependencyEntityField = validate; marshalProjectDependency = marshal })
	validateProjectDependencyEntityField = func(EntityFieldRef) error { return errors.New("column metadata unavailable") }
	if out, err := ProjectQueryDependencies(q); !errors.Is(err, ErrProjectUsageIncomplete) || out != nil {
		t.Fatal("column revalidation failure admitted")
	}
	validateProjectDependencyEntityField = validate
	reached := false
	marshalProjectDependency = func(v any) ([]byte, error) {
		if d, ok := v.(ProjectDefinitionDependency); ok && d.EntityField.Entity == "ColumnEntity" {
			reached = true
			return nil, errors.New("codec")
		}
		return marshal(v)
	}
	if out, err := ProjectQueryDependencies(q); !reached || !errors.Is(err, ErrProjectUsageIncomplete) || out != nil {
		t.Fatal("column encoding failure admitted", err)
	}
}

func TestMeaningEmptyEntityDeleteStillRequiresCompleteInventory(t *testing.T) {
	d := meaningsTestDocument()
	d.Entities[0].Fields = nil
	c, err := BuildProjectMeaningsCatalog(d, nil, nil, MeaningScope{Storage: "local", Project: "p"}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	w := ProjectMeaningWrite{OperationID: "delete-empty-entity", IfMatch: c.Revision, Change: ProjectMeaningAction{Action: "delete-entity", Entity: MeaningIdentity{"project", "person"}}}
	if _, _, err := ApplyProjectMeaningChange(d, c, w); !errors.Is(err, ErrProjectUsageIncomplete) {
		t.Fatal("narrow catalog granted empty entity deletion", err)
	}
	if len(d.Entities) != 1 {
		t.Fatal("denied mutation changed input")
	}
}

func TestLifecycleProtectsRetainedUseWithoutInventingAuditIdentityEdges(t *testing.T) {
	target := ProjectObjectRef{Kind: "query", ID: "q"}
	for _, scenario := range []struct {
		name                          string
		references                    []ProjectObjectRef
		historical                    bool
		archiveBlocked, deleteBlocked bool
	}{
		{name: "identity-only provenance", historical: true},
		{name: "retained content", references: []ProjectObjectRef{target}, historical: true, deleteBlocked: true},
		{name: "active use", references: []ProjectObjectRef{target}, archiveBlocked: true, deleteBlocked: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			snapshot := ProjectUsageSnapshot{Complete: true, Revision: "r", Objects: []ProjectObjectUsage{{Ref: target, ContentRevision: "q1"}, {Ref: ProjectObjectRef{Kind: "run", ID: "history"}, ContentRevision: "h1", Historical: scenario.historical, References: scenario.references}}}
			for _, action := range []string{"archive", "delete"} {
				_, err := PlanProjectLifecycle(snapshot, ProjectLifecycleWrite{OperationID: "op", ExpectedRevision: "r", Change: ProjectLifecycleChange{Object: target, Action: action}})
				blocked := scenario.archiveBlocked
				if action == "delete" {
					blocked = scenario.deleteBlocked
				}
				if blocked && !errors.Is(err, ErrProjectObjectReferenced) || !blocked && err != nil {
					t.Fatal("content/use classification violated", action, err)
				}
			}
		})
	}
}
