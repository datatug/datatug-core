package filestore

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
)

func TestLoaderInternals(t *testing.T) {
	t.Run("loadDbModel", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "datatug_test_loadDbModel")
		assert.NoError(t, err)
		defer func() { _ = os.RemoveAll(tempDir) }()

		dbModelsDir := path.Join(tempDir, "dbmodels")
		dbModelID := "model1"
		dbModelDir := path.Join(dbModelsDir, dbModelID)
		err = os.MkdirAll(dbModelDir, 0777)
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
		tempDir, err := os.MkdirTemp("", "datatug_test_loadDbCatalog")
		assert.NoError(t, err)
		defer func() { _ = os.RemoveAll(tempDir) }()

		catalogDir := path.Join(tempDir, "catalogs", "db1")
		err = os.MkdirAll(catalogDir, 0777)
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

	t.Run("loadBoards", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "datatug_test_loadBoards")
		assert.NoError(t, err)
		defer func() { _ = os.RemoveAll(tempDir) }()

		boardsDir := path.Join(tempDir, "boards")
		err = os.MkdirAll(boardsDir, 0777)
		assert.NoError(t, err)

		boardID := "board1"
		boardFile := path.Join(boardsDir, boardID+"."+storage.BoardFileSuffix+".json")
		board := datatug.Board{
			ProjBoardBrief: datatug.ProjBoardBrief{
				ProjItemBrief: datatug.ProjItemBrief{
					ID: boardID,
				},
			},
		}
		data, _ := json.Marshal(board)
		err = os.WriteFile(boardFile, data, 0666)
		assert.NoError(t, err)

		project := &datatug.Project{}
		err = loadBoards(context.TODO(), tempDir, project)
		assert.NoError(t, err)
		assert.Len(t, project.Boards, 1)
		assert.Equal(t, boardID, project.Boards[0].ID)
	})

	t.Run("loadDbModels", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "datatug_test_loadDbModels")
		assert.NoError(t, err)
		defer func() { _ = os.RemoveAll(tempDir) }()

		dbModelsDir := path.Join(tempDir, "dbmodels")
		err = os.MkdirAll(dbModelsDir, 0777)
		assert.NoError(t, err)

		modelID := "model2"
		modelDir := path.Join(dbModelsDir, modelID)
		err = os.MkdirAll(modelDir, 0777)
		assert.NoError(t, err)

		model := datatug.DbModel{
			ProjectItem: datatug.ProjectItem{
				ProjItemBrief: datatug.ProjItemBrief{
					ID: modelID,
				},
			},
		}
		data, _ := json.Marshal(model)
		err = os.WriteFile(path.Join(modelDir, modelID+"."+storage.DbModelFileSuffix+".json"), data, 0666)
		assert.NoError(t, err)

		project := &datatug.Project{}
		err = loadDbModels(context.TODO(), tempDir, project)
		assert.NoError(t, err)
		assert.Len(t, project.DbModels, 1)
		assert.Equal(t, modelID, project.DbModels[0].ID)
	})

	t.Run("loadDbCatalogs", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "datatug_test_loadDbCatalogs")
		assert.NoError(t, err)
		defer func() { _ = os.RemoveAll(tempDir) }()

		catalogsDir := path.Join(tempDir, "catalogs")
		err = os.MkdirAll(catalogsDir, 0777)
		assert.NoError(t, err)

		catalogID := "cat1"
		catalogDir := path.Join(catalogsDir, catalogID)
		err = os.MkdirAll(catalogDir, 0777)
		assert.NoError(t, err)

		catalog := datatug.EnvDbCatalog{
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

		dbServer := &datatug.ProjDbServer{}
		err = loadDbCatalogs(catalogsDir, dbServer)
		assert.NoError(t, err)
		assert.Len(t, dbServer.Catalogs, 1)
		assert.Equal(t, catalogID, dbServer.Catalogs[0].ID)
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

		tableID := "t1"
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

		tables, err := loadTables(schemasDir, schemaID, folder)
		assert.NoError(t, err)
		assert.Len(t, tables, 1)
		assert.Equal(t, tableID, tables[0].Name())
	})
}
