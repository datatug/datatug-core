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

func TestLoaderInternals(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datatug_test_loader_internals")
	assert.NoError(t, err)
	defer func(path string) {
		_ = os.RemoveAll(path)
	}(tempDir)

	t.Run("loadDbModel", func(t *testing.T) {
		dbModelsDir := path.Join(tempDir, "dbmodels")
		dbModelID := "model1"
		dbModelDir := path.Join(dbModelsDir, dbModelID)
		err := os.MkdirAll(dbModelDir, 0777)
		assert.NoError(t, err)

		dbModel := datatug.DbModel{
			ProjectItem: datatug.ProjectItem{
				ProjItemBrief: datatug.ProjItemBrief{
					ID: dbModelID,
				},
			},
		}
		data, _ := json.Marshal(dbModel)
		err = os.WriteFile(path.Join(dbModelDir, dbModelID+"."+storage.DbModelFileSuffix+".json"), data, 0666)
		assert.NoError(t, err)

		// Create a schema directory
		schemaID := "schema1"
		err = os.MkdirAll(path.Join(dbModelDir, schemaID), 0777)
		assert.NoError(t, err)

		loadedModel, err := loadDbModel(dbModelsDir, dbModelID)
		assert.NoError(t, err)
		assert.NotNil(t, loadedModel)
		assert.Equal(t, dbModelID, loadedModel.ID)
		assert.Len(t, loadedModel.Schemas, 1)
		assert.Equal(t, schemaID, loadedModel.Schemas[0].ID)
	})

	t.Run("loadDbCatalog", func(t *testing.T) {
		catalogDir := path.Join(tempDir, "catalogs", "db1")
		err := os.MkdirAll(catalogDir, 0777)
		assert.NoError(t, err)

		catalog := datatug.EnvDbCatalog{
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

		loadedCatalog := &datatug.EnvDbCatalog{}
		loadedCatalog.ID = "db1"
		err = loadDbCatalog(catalogDir, loadedCatalog)
		assert.NoError(t, err)
		assert.Len(t, loadedCatalog.Schemas, 1)
		assert.Equal(t, "dbo", loadedCatalog.Schemas[0].ID)
	})

	t.Run("loadSchema", func(t *testing.T) {
		schemasDir := path.Join(tempDir, "schemas")
		schemaID := "dbo"
		schemaDir := path.Join(schemasDir, schemaID)
		err := os.MkdirAll(schemaDir, 0777)
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
		tablesDir := path.Join(tempDir, "tables")
		tableName := "table1"
		tableDir := path.Join(tablesDir, tableName)
		err := os.MkdirAll(tableDir, 0777)
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
}
