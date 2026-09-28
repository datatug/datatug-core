package datatug

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCoverageProjectStore struct {
	ProjectStore
	err error
	dbs ProjDbDrivers
}

func (m *mockCoverageProjectStore) LoadProjDbDrivers(ctx context.Context, o ...StoreOption) (ProjDbDrivers, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.dbs, nil
}

func TestCoverage_ProjectAndStore(t *testing.T) {
	driver := &ProjDbDriver{
		ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "pg"}},
		Servers: ProjDbServers{
			{
				ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "pg/host"}},
				Server:      ServerRef{Driver: "pg", Host: "host"},
			},
		},
	}
	store := &mockCoverageProjectStore{dbs: ProjDbDrivers{driver}}
	p := NewProjectWithStore("p1", store)
	require.NotNil(t, p)
	assert.Equal(t, "p1", p.ID)

	// GetDBs
	dbs, err := p.GetDBs(context.Background())
	require.NoError(t, err)
	assert.Len(t, dbs, 1)

	// GetProjDbServer found
	srv, err := p.GetProjDbServer(context.Background(), ServerRef{Driver: "pg", Host: "host"})
	require.NoError(t, err)
	assert.NotNil(t, srv)

	// GetProjDbServer server not found in driver
	srv, err = p.GetProjDbServer(context.Background(), ServerRef{Driver: "pg", Host: "other"})
	require.NoError(t, err)
	assert.Nil(t, srv)

	// GetProjDbServer driver not found
	srv, err = p.GetProjDbServer(context.Background(), ServerRef{Driver: "mysql", Host: "host"})
	require.NoError(t, err)
	assert.Nil(t, srv)

	// AddProjDbServer new driver
	err = p.AddProjDbServer(context.Background(), &ProjDbServer{
		Server: ServerRef{Driver: "sqlite", Host: "file.db"},
	})
	require.NoError(t, err)

	// AddProjDbServer existing driver
	err = p.AddProjDbServer(context.Background(), &ProjDbServer{
		Server: ServerRef{Driver: "pg", Host: "host2"},
	})
	require.NoError(t, err)

	// Store error branches
	storeErr := &mockCoverageProjectStore{err: errors.New("load failed")}
	pErr := NewProjectWithStore("p2", storeErr)
	_, err = pErr.GetDBs(context.Background())
	assert.Error(t, err)
	_, err = pErr.GetProjDbServer(context.Background(), ServerRef{})
	assert.Error(t, err)
	err = pErr.AddProjDbServer(context.Background(), &ProjDbServer{})
	assert.Error(t, err)
}

func TestCoverage_DatatugTypes(t *testing.T) {
	// boards.go:187 widget.Validate error
	w := &BoardWidget{Name: "tabs", Data: &TabsWidgetDef{WidgetBase: WidgetBase{Parameters: Parameters{{}}}}}
	assert.Error(t, w.Validate())

	// db_collection.go:20, 23, 34
	c := CollectionInfo{DBCollectionKey: DBCollectionKey{t: "invalid"}}
	assert.Error(t, c.Validate())
	ciValid := CollectionInfo{
		DBCollectionKey: NewTableKey("t", "s", "c", nil),
		TableProps:      TableProps{DbType: "BASE TABLE"},
	}
	assert.NoError(t, ciValid.Validate())
	ciInvalidProps := CollectionInfo{
		DBCollectionKey: NewTableKey("t", "s", "c", nil),
		TableProps:      TableProps{DbType: ""},
	}
	assert.Error(t, ciInvalidProps.Validate())

	// db_model.go:18, 211
	assert.Nil(t, DbModels{}.GetByID("id"))
	tm := &TableModel{DBCollectionKey: DBCollectionKey{t: "invalid"}}
	assert.Error(t, tm.Validate())

	// db_objects.go:133, 504
	assert.Error(t, TableKeys{DBCollectionKey{t: "invalid"}}.Validate())
	ci := ColumnInfo{DbColumnProps: DbColumnProps{Name: "col", OrdinalPosition: -1}}
	assert.Error(t, ci.Validate())

	// dbcatalog.go:46
	assert.Empty(t, DbCatalogs{}.IDs())

	// entities.go:58
	e := Entity{
		ProjectItem: ProjectItem{
			ProjItemBrief: ProjItemBrief{ID: "e", Title: "E"},
			Access:        "private",
		},
		Tables: TableKeys{DBCollectionKey{t: "invalid"}},
	}
	assert.Error(t, e.Validate())

	// env_db_server.go:45, 49
	s := &EnvDbServer{ServerRef: ServerRef{Host: "localhost", Port: 8080}}
	assert.Equal(t, "localhost:8080", s.GetID())
	s.SetID("remote:5432")
	assert.Equal(t, "remote", s.Host)
	assert.Equal(t, 5432, s.Port)

	// environment.go:27
	assert.Empty(t, Environments{}.IDs())

	// folder.go:24, 28
	f := &Folder{}
	f.SetID("f1")
	assert.Equal(t, "f1", f.GetID())

	// proj_item.go:143
	pi := ProjectItem{ProjItemBrief: ProjItemBrief{ID: "item1"}}
	assert.Equal(t, pi, pi.GetProjectItem())

	// recordset_def.go:139
	rd := RecordsetDefinition{
		Type:             "recordset",
		ProjectItem:      ProjectItem{ProjItemBrief: ProjItemBrief{ID: "rs", Title: "RS"}},
		RecordsetBaseDef: RecordsetBaseDef{PrimaryKey: &UniqueKey{Columns: []string{"id"}}},
		Columns:          RecordsetColumnDefs{{Name: "id", Type: "integer"}},
	}
	assert.NoError(t, rd.Validate())

	// server.go:110, 113, 116, 126, 130
	assert.Empty(t, ProjDbDrivers{}.IDs())
	assert.Nil(t, ProjDbDrivers{}.GetByID("none"))

	pds := ProjDbServer{
		ProjectItem: ProjectItem{
			ProjItemBrief: ProjItemBrief{ID: "sqlserver:localhost"},
			Access:        "invalid_access",
		},
		Server: ServerRef{Driver: "sqlserver", Host: "localhost"},
	}
	assert.Error(t, pds.Validate()) // ValidateWithOptions fails

	pds.ProjectItem.Access = ""
	pds.Server.Port = -1
	pds.ID = "sqlserver:localhost:-1"
	assert.Error(t, pds.Validate()) // Server.Validate fails

	pds.Server.Port = 0
	pds.ID = "sqlserver:localhost"
	pds.Catalogs = DbCatalogs{{}}
	assert.Error(t, pds.Validate()) // Catalogs.Validate fails

	// store_options.go:15, 26
	opts := GetStoreOptions(Depth(2))
	slice := opts.ToSlice()
	assert.Len(t, slice, 1)
	next := opts.Next()
	assert.Equal(t, 1, next.Depth())
	assert.Equal(t, 0, GetStoreOptions(Depth(0)).Next().Depth())

	// query_revision_errors.go:70, 103
	incErr := &IncompleteQueryRecordError{FolderPath: "f", ID: "q", Reason: "bad"}
	assert.Contains(t, incErr.Error(), "incomplete query record")
	revErr := &QueryRevisionConflictError{FolderPath: "f", ID: "q", Reason: "conflict"}
	assert.Contains(t, revErr.Error(), "query revision conflict")
}

func TestCoverage_QueryAndScreen(t *testing.T) {
	// query.go:271, 291, 294, 298
	qUnsupported := QueryDef{
		ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "q", Title: "Q"}},
		Type:        "UNSUPPORTED",
	}
	assert.Error(t, qUnsupported.Validate())

	qMissingLookup := QueryDef{
		ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "q", Title: "Q"}},
		Type:        "SQL",
		Federation: &QueryFederation{
			Lookups: []QueryHTTPLookup{{}},
		},
	}
	assert.Error(t, qMissingLookup.Validate())

	qBadConcurrency := QueryDef{
		ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "q", Title: "Q"}},
		Type:        "SQL",
		Federation: &QueryFederation{
			Lookups: []QueryHTTPLookup{{
				Database: "db", Collection: "c", FromColumn: "col",
				Fields:      []QueryLookupField{{Source: "s", Target: "t"}},
				Concurrency: -1,
			}},
		},
	}
	assert.Error(t, qBadConcurrency.Validate())

	qMissingField := QueryDef{
		ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "q", Title: "Q"}},
		Type:        "SQL",
		Federation: &QueryFederation{
			Lookups: []QueryHTTPLookup{{
				Database: "db", Collection: "c", FromColumn: "col",
				Fields: []QueryLookupField{{Source: "", Target: ""}},
			}},
		},
	}
	assert.Error(t, qMissingField.Validate())

	// query_storage_screen.go: 291, 314, 387, 429
	qScreen := QueryDef{
		Type: "SQL",
		Text: "SELECT 1",
		Targets: []QueryDefTarget{
			{Driver: "db"},
		},
		Recordsets: []RecordsetDefinition{
			{
				JSONSchema: "",
				RecordsetBaseDef: RecordsetBaseDef{
					PrimaryKey:  nil,
					ForeignKeys: []*ForeignKey{nil},
				},
			},
		},
	}
	assert.NoError(t, screenQueryDefForStorage(qScreen))

	// jsonSchemaValueCredentialReason array recursion
	reason, found := jsonSchemaValueCredentialReason([]any{[]any{"password=secret_val"}})
	assert.True(t, found)
	assert.NotEmpty(t, reason)

	// query_credentials.go: 279, 316, 321, 371, 484, 488, 490, 492, 498-501, 504, 506
	assert.Equal(t, "unclosed", keyValueValue("'unclosed"))
	_, found = jsonCredentialReasonPass("{\"password\":")
	assert.False(t, found)
	_, found = jsonCredentialReasonPass("{\"password\": \"")
	assert.True(t, found)

	assert.True(t, isNonSecretHeaderValue("cookie", "a=true; ; b={param}"))

	assert.True(t, isNonSecretJSONValue(nil))
	assert.True(t, isNonSecretJSONValue(true))
	assert.True(t, isNonSecretJSONValue([]any{}))
	assert.True(t, isNonSecretJSONValue(map[string]any{}))
	assert.True(t, isNonSecretJSONValue(map[string]any{"type": "string"}))
	assert.False(t, isNonSecretJSONValue(map[string]any{"type": "custom"}))
	assert.False(t, isNonSecretJSONValue(123))
}
