package ingitdbschema

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/ingitdb/ingitdb-go/ingitdb"
	"github.com/ingitdb/ingitdb-go/ingitdb/datavalidator"
	"github.com/ingitdb/ingitdb-go/ingitdb/validator"
)

// TestColumnsMatchStructFields marshals a representative, fully-populated
// instance of each project-item type's real datatug-core Go struct (the
// exact type the filestore package persists today — see the mapping table
// and citations below) and validates the resulting record data against this
// package's schema with ingitdb-go v0.6.1's datavalidator. It asserts no
// "undeclared field" finding: every JSON key the struct actually produces
// has a matching declared column.
//
// Collection -> struct mapping (also documented at the top of each
// definition.yaml):
//
//	projects                      datatug.ProjectFile      (project.go)      — filestore's saveProjectFile writes this, not the richer datatug.Project
//	queries                       datatug.QueryDef          (query.go)        — pkg/storage/filestore/store_queries.go
//	entities                      datatug.Entity            (entities.go)     — pkg/storage/filestore/store_entities.go
//	environments                  datatug.Environment       (environment.go)  — pkg/storage/filestore/environments_store.go
//	environments/servers          datatug.EnvDbServer       (env_db_server.go)— pkg/storage/filestore/env_db_servers_store.go
//	environments/catalogs         datatug.DbCatalog         (dbcatalog.go)    — pkg/storage/filestore/env_db_catalogs_store.go
//	dbmodels                      datatug.DbModel           (db_model.go)     — pkg/storage/filestore/db_models_store.go
//	boards                        datatug.Board             (boards.go)       — pkg/storage/filestore/boards_store.go
//	recordsets                    datatug.RecordsetDefinition (recordset_def.go) — pkg/storage/filestore/recordset_definitions_store.go
//	folders                       datatug.Folder            (folder.go)       — pkg/storage/filestore/store_folders.go
//	dbdrivers                     datatug.ProjDbDriver      (server.go)       — pkg/storage/filestore/proj_dbdrivers_store.go
//	dbdrivers/dbservers           datatug.ProjDbServer      (server.go)       — pkg/storage/filestore/proj_dbservers_store.go
//
// ext and credentials are deliberately excluded: neither has a clear
// backing Go struct (ext is a scoping parent with no record of its own;
// credentials' shape and storage are owned by secrets-vault), so their
// definitions keep the placeholder `id` column and are not asserted here.
func TestColumnsMatchStructFields(t *testing.T) {
	dir := t.TempDir()
	if err := WriteSchema(dir); err != nil {
		t.Fatalf("WriteSchema(%s): %v", dir, err)
	}
	def, err := validator.ReadDefinition(dir, ingitdb.Validate())
	if err != nil {
		t.Fatalf("ReadDefinition(%s): %v", dir, err)
	}
	projects := def.Collections["ext"].SubCollections["projects"]

	cases := []struct {
		collection string
		colDef     *ingitdb.CollectionDef
		value      any
	}{
		{"projects", projects, representativeProjectFile()},
		{"queries", projects.SubCollections["queries"], representativeQueryDef()},
		{"entities", projects.SubCollections["entities"], representativeEntity()},
		{"environments", projects.SubCollections["environments"], representativeEnvironment()},
		{"environments/servers", projects.SubCollections["environments"].SubCollections["servers"], representativeEnvDbServer()},
		{"environments/catalogs", projects.SubCollections["environments"].SubCollections["catalogs"], representativeDbCatalog()},
		{"dbmodels", projects.SubCollections["dbmodels"], representativeDbModel()},
		{"boards", projects.SubCollections["boards"], representativeBoard()},
		{"recordsets", projects.SubCollections["recordsets"], representativeRecordsetDefinition()},
		{"folders", projects.SubCollections["folders"], representativeFolder()},
		{"dbdrivers", projects.SubCollections["dbdrivers"], representativeProjDbDriver()},
		{"dbdrivers/dbservers", projects.SubCollections["dbdrivers"].SubCollections["dbservers"], representativeProjDbServer()},
	}

	for _, c := range cases {
		t.Run(c.collection, func(t *testing.T) {
			if c.colDef == nil {
				t.Fatalf("collection %q not found in loaded definition", c.collection)
			}
			data := marshalToRecordData(t, c.value)
			errs := datavalidator.ValidateRecordData(c.colDef, "rec1", data)
			var undeclared []string
			for _, e := range errs {
				if strings.Contains(e.Message, "undeclared") || strings.Contains(e.Message, "no column") {
					undeclared = append(undeclared, e.FieldName+": "+e.Message)
				}
			}
			if len(undeclared) > 0 {
				t.Errorf("undeclared-field findings for %q: %v", c.collection, undeclared)
			}
		})
	}
}

// marshalToRecordData round-trips v through JSON, exactly like a filestore
// project item is persisted, to get the plain map[string]any datavalidator
// operates on.
func marshalToRecordData(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal(%T): %v", v, err)
	}
	var data map[string]any
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("json.Unmarshal into map for %T: %v", v, err)
	}
	return data
}

func representativeProjItemBrief() datatug.ProjItemBrief {
	return datatug.ProjItemBrief{
		ID:     "rec1",
		Title:  "Title",
		Folder: "~",
		ListOfTags: datatug.ListOfTags{
			Tags: []string{"tag1"},
		},
	}
}

func representativeProjectItem() datatug.ProjectItem {
	return datatug.ProjectItem{
		ProjItemBrief: representativeProjItemBrief(),
		UserIDs:       []string{"user1"},
		Access:        "private",
	}
}

func representativeProjectFile() datatug.ProjectFile {
	return datatug.ProjectFile{
		Created: &datatug.ProjectCreated{At: time.Now()},
		ProjectItem: datatug.ProjectItem{
			ProjItemBrief: representativeProjItemBrief(),
			UserIDs:       []string{"user1"},
			Access:        "private",
		},
		Repository: &datatug.ProjectRepository{Type: "git", WebURL: "https://example.com/repo"},
	}
}

func representativeQueryDef() datatug.QueryDef {
	return datatug.QueryDef{
		ProjectItem: representativeProjectItem(),
		Type:        datatug.QueryTypeSQL,
		Text:        "SELECT 1",
		Draft:       false,
		Parameters:  datatug.Parameters{{ID: "p1", Type: "string"}},
		Targets:     []datatug.QueryDefTarget{{Driver: "sqlite3"}},
		Recordsets:  []datatug.RecordsetDefinition{{Type: "recordset"}},
		Purpose:     "answers a question",
		Capture:     &datatug.QueryCapture{Environment: "local", Source: "src1"},
	}
}

func representativeEntity() datatug.Entity {
	return datatug.Entity{
		ProjectItem: representativeProjectItem(),
		Fields: datatug.EntityFields{
			{ID: "f1", Type: "string"},
		},
		Tables: datatug.TableKeys{},
	}
}

func representativeEnvironment() datatug.Environment {
	return datatug.Environment{
		ProjectItem: representativeProjectItem(),
		DbServers: datatug.EnvDbServers{
			{ServerRef: datatug.ServerRef{Driver: "sqlite3"}},
		},
	}
}

func representativeEnvDbServer() datatug.EnvDbServer {
	return datatug.EnvDbServer{
		ServerRef: datatug.ServerRef{Driver: "sqlserver", Host: "localhost", Port: 1433},
		Catalogs:  []string{"cat1"},
	}
}

func representativeDbCatalog() datatug.DbCatalog {
	return datatug.DbCatalog{
		DbCatalogBase: datatug.DbCatalogBase{
			ProjectItem: representativeProjectItem(),
			Driver:      "sqlserver",
			DbModel:     "model1",
		},
		Schemas: datatug.DbSchemas{},
	}
}

func representativeDbModel() datatug.DbModel {
	return datatug.DbModel{
		ProjectItem:  representativeProjectItem(),
		Schemas:      datatug.SchemaModels{{ProjectItem: representativeProjectItem()}},
		Environments: datatug.DbModelEnvironments{{ID: "env1"}},
	}
}

func representativeBoard() datatug.Board {
	return datatug.Board{
		ProjectItem:    representativeProjectItem(),
		Parameters:     datatug.Parameters{{ID: "p1", Type: "string"}},
		RequiredParams: [][]string{{"p1"}},
		Rows:           datatug.BoardRows{},
	}
}

func representativeRecordsetDefinition() datatug.RecordsetDefinition {
	return datatug.RecordsetDefinition{
		ProjectItem: representativeProjectItem(),
		RecordsetBaseDef: datatug.RecordsetBaseDef{
			PrimaryKey:    &datatug.UniqueKey{},
			ForeignKeys:   datatug.ForeignKeys{},
			AlternateKeys: []datatug.UniqueKey{},
			ActiveIssues:  &datatug.Issues{},
		},
		Columns: datatug.RecordsetColumnDefs{
			{Name: "col1", Type: "string"},
		},
		Type:       "recordset",
		JSONSchema: "",
		Files:      []string{"f1.csv"},
		Errors:     []string{},
	}
}

func representativeFolder() datatug.Folder {
	return datatug.Folder{
		Name:     "budget",
		Note:     "note",
		NumberOf: map[string]int{"boards": 2},
	}
}

func representativeProjDbDriver() datatug.ProjDbDriver {
	return datatug.ProjDbDriver{
		ProjectItem: representativeProjectItem(),
		Servers: datatug.ProjDbServers{
			{ProjectItem: representativeProjectItem(), Server: datatug.ServerRef{Driver: "sqlserver"}},
		},
	}
}

func representativeProjDbServer() datatug.ProjDbServer {
	return datatug.ProjDbServer{
		ProjectItem: representativeProjectItem(),
		Server:      datatug.ServerRef{Driver: "sqlserver", Host: "localhost", Port: 1433},
		Catalogs:    datatug.DbCatalogs{},
	}
}
