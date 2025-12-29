package filestore

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sync"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func TestLoader(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_loader")
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	projID := "test_project"
	projPath := path.Join(tmpDir, projID)
	datatugDir := path.Join(projPath, DatatugFolder)
	err = os.MkdirAll(datatugDir, 0755)
	assert.NoError(t, err)

	// Create project file
	projFile := datatug.ProjectFile{
		ProjectItem: datatug.ProjectItem{
			ProjItemBrief: datatug.ProjItemBrief{
				ID:    projID,
				Title: "Test Project",
			},
		},
	}
	projData, _ := json.Marshal(projFile)
	err = os.WriteFile(path.Join(datatugDir, ProjectSummaryFileName), projData, 0644)
	assert.NoError(t, err)

	// Create a board
	boardsDir := path.Join(datatugDir, BoardsFolder)
	err = os.MkdirAll(boardsDir, 0755)
	assert.NoError(t, err)
	board := datatug.Board{
		ProjBoardBrief: datatug.ProjBoardBrief{
			ProjItemBrief: datatug.ProjItemBrief{
				ID: "board1",
			},
		},
	}
	boardData, _ := json.Marshal(board)
	err = os.WriteFile(path.Join(boardsDir, "board1.board.json"), boardData, 0644)
	assert.NoError(t, err)

	// Create a DB model
	dbModelsDir := path.Join(datatugDir, DbModelsFolder)
	modelID := "model1"
	modelDir := path.Join(dbModelsDir, modelID)
	err = os.MkdirAll(modelDir, 0755)
	assert.NoError(t, err)
	dbModel := datatug.DbModel{}
	dbModel.ID = modelID
	modelData, _ := json.Marshal(dbModel)
	err = os.WriteFile(path.Join(modelDir, "model1.dbmodel.json"), modelData, 0644)
	assert.NoError(t, err)

	// Create a schema in DB model
	schemaID := "public"
	schemaDir := path.Join(modelDir, schemaID)
	err = os.MkdirAll(path.Join(schemaDir, "tables"), 0755)
	assert.NoError(t, err)
	err = os.MkdirAll(path.Join(schemaDir, "views"), 0755)
	assert.NoError(t, err)

	// Create environments
	envsDir := path.Join(datatugDir, EnvironmentsFolder)
	envID := "dev"
	envDir := path.Join(envsDir, envID)
	err = os.MkdirAll(envDir, 0755)
	assert.NoError(t, err)
	env := datatug.EnvironmentSummary{
		ProjectItem: datatug.ProjectItem{
			ProjItemBrief: datatug.ProjItemBrief{
				ID: envID,
			},
		},
	}
	envData, _ := json.Marshal(env)
	err = os.WriteFile(path.Join(envDir, "dev.env.json"), envData, 0644)
	assert.NoError(t, err)

	// Create DB servers
	serversDir := path.Join(datatugDir, ServersFolder, DbFolder)
	driver := "postgres"
	serverName := "localhost"
	serverDir := path.Join(serversDir, driver, serverName)
	err = os.MkdirAll(serverDir, 0755)
	assert.NoError(t, err)
	dbServer := datatug.ProjDbServer{}
	dbServer.ID = serverName
	dbServerData, _ := json.Marshal(dbServer)
	err = os.WriteFile(path.Join(serverDir, "postgres.localhost.dbserver.json"), dbServerData, 0644)
	assert.NoError(t, err)

	// Create DB catalog
	catalogID := "testdb"
	catalogDir := path.Join(serverDir, DbCatalogsFolder, catalogID)
	err = os.MkdirAll(catalogDir, 0755)
	assert.NoError(t, err)
	catalog := datatug.DbCatalog{}
	catalog.ID = catalogID
	catalogData, _ := json.Marshal(catalog)
	err = os.WriteFile(path.Join(catalogDir, "testdb.db.json"), catalogData, 0644)
	assert.NoError(t, err)

	// Create a table in catalog
	catalogSchemasDir := path.Join(catalogDir, SchemasFolder)
	catalogSchemaID := "public"
	catalogSchemaDir := path.Join(catalogSchemasDir, catalogSchemaID)
	err = os.MkdirAll(path.Join(catalogSchemaDir, "tables", "users"), 0755)
	assert.NoError(t, err)
	table := datatug.CollectionInfo{
		DBCollectionKey: datatug.NewCollectionKey(datatug.CollectionTypeTable, "users", catalogSchemaID, "", nil),
	}
	tableData, _ := json.Marshal(table)
	err = os.WriteFile(path.Join(catalogSchemaDir, "tables", "users", "public.users.json"), tableData, 0644)
	assert.NoError(t, err)

	// Test LoadProject
	//store := newFsProjectStore(projID, projPath)
	//project, err := store.LoadProject(context.Background())
	//assert.NoError(t, err)
	//assert.NotNil(t, project)
	//assert.Equal(t, "Test Project", project.Title)
	//assert.NotEmpty(t, project.Boards)
	//assert.NotEmpty(t, project.DbModels)
	//assert.NotEmpty(t, project.Environments)
	//assert.NotEmpty(t, project.DbServers)

	// Test loadDir errors
	t.Run("loadDir_errors", func(t *testing.T) {
		err := loadDir(nil, path.Join(tmpDir, "nonexistent"), "", processDirs, nil, nil)
		assert.NoError(t, err) // Should return nil for nonexistent dir

		// Not a directory error
		filePath := path.Join(tmpDir, "file.txt")
		_ = os.WriteFile(filePath, []byte("test"), 0644)
		err = loadDir(nil, filePath, "", processDirs, nil, nil)
		assert.Error(t, err)

		// Loader error
		err = loadDir(nil, boardsDir, "*.json", processFiles, nil, func(f os.FileInfo, i int, mutex *sync.Mutex) error {
			return fmt.Errorf("loader error")
		})
		assert.Error(t, err)
	})

	t.Run("loadDbModel_errors", func(t *testing.T) {
		_, err := loadDbModel(dbModelsDir, "invalid")
		assert.Error(t, err)

		// ID mismatch
		wrongIDDir := path.Join(dbModelsDir, "wrong")
		err = os.MkdirAll(wrongIDDir, 0755)
		assert.NoError(t, err)
		err = os.WriteFile(path.Join(wrongIDDir, "wrong.dbmodel.json"), modelData, 0644) // modelData has id "model1"
		assert.NoError(t, err)
		_, err = loadDbModel(dbModelsDir, "wrong")
		assert.Error(t, err)
	})

	t.Run("loadDbServer_errors", func(t *testing.T) {
		// ID mismatch
		wrongServerDir := path.Join(serversDir, driver, "wrong")
		err = os.MkdirAll(wrongServerDir, 0755)
		assert.NoError(t, err)
		err = os.WriteFile(path.Join(wrongServerDir, "postgres.wrong.dbserver.json"), dbServerData, 0644) // dbServerData has ID "" currently or from prev marshal
		assert.NoError(t, err)

		// Set a different ID to trigger error
		dbServerWithID := datatug.ProjDbServer{}
		dbServerWithID.ID = "actual_id"
		data, _ := json.Marshal(dbServerWithID)
		err = os.WriteFile(path.Join(wrongServerDir, "postgres.wrong.dbserver.json"), data, 0644)
		assert.NoError(t, err)

		_, err = loadDbServer(path.Join(serversDir, driver), driver, "wrong")
		assert.Error(t, err)
	})
}
