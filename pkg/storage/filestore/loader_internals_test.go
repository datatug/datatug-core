package filestore

import (
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
)

func setupDbModel(t *testing.T, dbModelsDir, dbModelID string) (dbModel datatug.DbModel) {
	dbModelDir := path.Join(dbModelsDir, dbModelID)
	err := os.MkdirAll(dbModelDir, 0777)
	assert.NoError(t, err)

	dbModel.ID = dbModelID
	data, _ := json.Marshal(dbModel)
	err = os.WriteFile(path.Join(dbModelDir, dbModelID+"."+storage.DbModelFileSuffix+".json"), data, 0666)
	assert.NoError(t, err)

	return dbModel
}

func TestLoaderInternals(t *testing.T) {
	t.Run("loadDbModel", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "datatug_test_loadDbModel")
		assert.NoError(t, err)
		defer func() { _ = os.RemoveAll(tempDir) }()

		dbModelsDir := path.Join(tempDir, "dbmodels")
		const dbModelID = "model1"
		setupDbModel(t, dbModelsDir, dbModelID)

		// Create two schema directories, each with two "tables" and two "views"
		// subdirectories, so the fan-out in loadDbModel (one goroutine per schema)
		// and in loadSchemaModel (one goroutine for tables, one for views) both have
		// real concurrent work to do. This is what makes this test a reliable `-race`
		// regression test for the data races that used to live in loadSchemaModel
		// (shared schemaModel.Tables field) and loadDbModel (unguarded
		// dbModel.Schemas append) - a single schema/table was not enough to trigger
		// them deterministically.
		schemaIDs := []string{"schema1", "schema2"}
		for _, schemaID := range schemaIDs {
			schemaDir := path.Join(dbModelsDir, dbModelID, schemaID)
			for _, folder := range []string{"tables", "views"} {
				for _, name := range []string{folder + "1", folder + "2"} {
					err = os.MkdirAll(path.Join(schemaDir, folder, name), 0777)
					assert.NoError(t, err)
				}
			}
		}

		loadedModel, err := loadDbModel(dbModelsDir, dbModelID)
		assert.NoError(t, err)
		assert.NotNil(t, loadedModel)
		assert.Equal(t, dbModelID, loadedModel.ID)
		assert.Len(t, loadedModel.Schemas, len(schemaIDs))

		byID := make(map[string]*datatug.Schema, len(loadedModel.Schemas))
		for _, schema := range loadedModel.Schemas {
			byID[schema.ID] = schema
		}
		for _, schemaID := range schemaIDs {
			schema, ok := byID[schemaID]
			assert.Truef(t, ok, "missing schema %v", schemaID)
			if !ok {
				continue
			}
			// Both the 2 base tables and the 2 views must survive the merge - this
			// is the correctness half of the bug: before the fix, whichever
			// goroutine finished last silently clobbered the other's results, so
			// the schema was missing either its tables or its views.
			assert.Len(t, schema.Tables, 4)
			var baseTables, views int
			for _, table := range schema.Tables {
				switch table.DbType {
				case "BASE TABLE":
					baseTables++
				case "VIEW":
					views++
				}
			}
			assert.Equal(t, 2, baseTables)
			assert.Equal(t, 2, views)
		}
	})

	t.Run("loadDbCatalog", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "datatug_test_loadDbCatalog")
		assert.NoError(t, err)
		defer func() { _ = os.RemoveAll(tempDir) }()

		catalogDir := path.Join(tempDir, "catalogs", "db1")
		err = os.MkdirAll(catalogDir, 0777)
		assert.NoError(t, err)

		catalog := datatug.DbCatalog{
			DbCatalogBase: datatug.DbCatalogBase{
				ProjectItem: datatug.ProjectItem{
					ProjItemBrief: datatug.ProjItemBrief{
						ID: "db1",
					},
				},
				Driver: "sqlserver",
			},
		}
		data, _ := json.Marshal(catalog)
		err = os.WriteFile(path.Join(catalogDir, "db1."+storage.DbCatalogFileSuffix+".json"), data, 0666)
		assert.NoError(t, err)

		// Create schemas dir
		schemasDir := path.Join(catalogDir, "schemas")
		err = os.MkdirAll(path.Join(schemasDir, "dbo"), 0777)
		assert.NoError(t, err)

		loadedCatalog := &datatug.DbCatalog{}
		loadedCatalog.ID = "db1"
		err = loadDbCatalog(catalogDir, loadedCatalog)
		assert.NoError(t, err)
		assert.Len(t, loadedCatalog.Schemas, 1)
		assert.Equal(t, "dbo", loadedCatalog.Schemas[0].ID)
	})

	t.Run("loadSchema", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "datatug_test_loadSchema")
		assert.NoError(t, err)
		defer func() { _ = os.RemoveAll(tempDir) }()

		schemasDir := path.Join(tempDir, "schemas")
		schemaID := "dbo"
		schemaDir := path.Join(schemasDir, schemaID)
		err = os.MkdirAll(schemaDir, 0777)
		assert.NoError(t, err)

		schema := datatug.DbSchema{
			ProjectItem: datatug.ProjectItem{
				ProjItemBrief: datatug.ProjItemBrief{
					ID: schemaID,
				},
			},
		}
		data, _ := json.Marshal(schema)
		err = os.WriteFile(path.Join(schemaDir, schemaID+".schema.json"), data, 0666)
		assert.NoError(t, err)

		// Create tables dir
		err = os.MkdirAll(path.Join(schemaDir, "tables"), 0777)
		assert.NoError(t, err)

		loadedSchema, err := loadSchema(schemasDir, schemaID)
		assert.NoError(t, err)
		assert.NotNil(t, loadedSchema)
		assert.Equal(t, schemaID, loadedSchema.ID)
	})

	t.Run("loadTable", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "datatug_test_loadTable")
		assert.NoError(t, err)
		defer func() { _ = os.RemoveAll(tempDir) }()

		tablesDir := path.Join(tempDir, "tables")
		tableName := "table1"
		tableDir := path.Join(tablesDir, tableName)
		err = os.MkdirAll(tableDir, 0777)
		assert.NoError(t, err)

		table := datatug.CollectionInfo{
			TableProps: datatug.TableProps{
				DbType: "BASE TABLE",
			},
		}
		data, _ := json.Marshal(table)
		err = os.WriteFile(path.Join(tableDir, "dbo."+tableName+".json"), data, 0666)
		assert.NoError(t, err)

		loadedTable, err := loadTable(tablesDir, "dbo", tableName)
		assert.NoError(t, err)
		assert.NotNil(t, loadedTable)
		assert.Equal(t, tableName, loadedTable.Name())
	})

	t.Run("loadTableModel", func(t *testing.T) {
		tm, err := loadTableModel("test")
		assert.NoError(t, err)
		assert.NotNil(t, tm)
		assert.Equal(t, "test", tm.Name())
	})

	t.Run("loadDbCatalogs", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "datatug_test_loadDbCatalogs")
		assert.NoError(t, err)
		defer func() { _ = os.RemoveAll(tempDir) }()

		catalogsDir := path.Join(tempDir, "catalogs")
		err = os.MkdirAll(catalogsDir, 0777)
		assert.NoError(t, err)

		// Two catalog directories so loadDbCatalogs' loadDir fan-out (one goroutine
		// per catalog) has real concurrent appends to dbServer.Catalogs - a single
		// catalog would not exercise the race that used to live here.
		catalogIDs := []string{"cat1", "cat2"}
		for _, catalogID := range catalogIDs {
			catalogDir := path.Join(catalogsDir, catalogID)
			err = os.MkdirAll(catalogDir, 0777)
			assert.NoError(t, err)

			catalog := datatug.DbCatalog{
				DbCatalogBase: datatug.DbCatalogBase{
					ProjectItem: datatug.ProjectItem{
						ProjItemBrief: datatug.ProjItemBrief{
							ID: catalogID,
						},
					},
					Driver: "sqlserver",
				},
			}
			data, _ := json.Marshal(catalog)
			err = os.WriteFile(path.Join(catalogDir, catalogID+"."+storage.DbCatalogFileSuffix+".json"), data, 0666)
			assert.NoError(t, err)
		}

		dbServer := &datatug.ProjDbServer{}
		err = loadDbCatalogs(catalogsDir, dbServer)
		assert.NoError(t, err)
		assert.Len(t, dbServer.Catalogs, len(catalogIDs))
		gotIDs := make([]string, 0, len(dbServer.Catalogs))
		for _, catalog := range dbServer.Catalogs {
			gotIDs = append(gotIDs, catalog.ID)
		}
		assert.ElementsMatch(t, catalogIDs, gotIDs)
	})

	t.Run("loadTables", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "datatug_test_loadTables")
		assert.NoError(t, err)
		defer func() { _ = os.RemoveAll(tempDir) }()

		schemasDir := path.Join(tempDir, "schemas")
		schemaID := "s1"
		folder := "tables"
		tablesDir := path.Join(schemasDir, schemaID, folder)
		err = os.MkdirAll(tablesDir, 0777)
		assert.NoError(t, err)

		// Two table directories so loadTables' loadDir fan-out (one goroutine per
		// table) has real concurrent appends to `tables` - a single table would not
		// exercise the race that used to live here.
		tableIDs := []string{"t1", "t2"}
		for _, tableID := range tableIDs {
			tableDir := path.Join(tablesDir, tableID)
			err = os.MkdirAll(tableDir, 0777)
			assert.NoError(t, err)

			table := datatug.CollectionInfo{
				TableProps: datatug.TableProps{
					DbType: "BASE TABLE",
				},
			}
			data, _ := json.Marshal(table)
			err = os.WriteFile(path.Join(tableDir, schemaID+"."+tableID+".json"), data, 0666)
			assert.NoError(t, err)
		}

		tables, err := loadTables(schemasDir, schemaID, folder)
		assert.NoError(t, err)
		assert.Len(t, tables, len(tableIDs))
		gotIDs := make([]string, 0, len(tables))
		for _, table := range tables {
			gotIDs = append(gotIDs, table.Name())
		}
		assert.ElementsMatch(t, tableIDs, gotIDs)
	})
}
