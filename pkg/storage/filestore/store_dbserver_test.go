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

func TestDbServerStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_dbserver")
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	projectID := "test_project"
	projectPath := path.Join(tmpDir, projectID)

	dbServer := datatug.ServerReference{
		Driver: "sqlserver",
		Host:   "localhost",
	}

	serverPath := path.Join(projectPath, storage.ServersFolder, storage.DbFolder, dbServer.Driver, dbServer.FileName())
	err = os.MkdirAll(path.Join(serverPath, "catalogs", "db1"), 0755)
	assert.NoError(t, err)

	catalogSummary := datatug.DbCatalogSummary{
		DbCatalogBase: datatug.DbCatalogBase{
			ProjectItem: datatug.ProjectItem{
				ProjItemBrief: datatug.ProjItemBrief{
					ID: "db1",
				},
			},
		},
	}
	data, _ := json.Marshal(catalogSummary)
	err = os.WriteFile(path.Join(serverPath, "catalogs", "db1", "db1.db.json"), data, 0644)
	assert.NoError(t, err)

	// Environment setup
	envName := "dev"
	envDbServerPath := path.Join(projectPath, storage.EnvironmentsFolder, envName, storage.ServersFolder, storage.DbFolder)
	err = os.MkdirAll(envDbServerPath, 0755)
	assert.NoError(t, err)

	envDbServer := datatug.EnvDbServer{
		ServerReference: dbServer,
		Catalogs:        []string{"db1"},
	}
	envData, _ := json.Marshal(envDbServer)
	err = os.WriteFile(path.Join(envDbServerPath, storage.JsonFileName(dbServer.FileName(), storage.ServerFileSuffix)), envData, 0644)
	assert.NoError(t, err)

	store := fsDbServerStore{
		dbServer: dbServer,
		fsDbServersStore: fsDbServersStore{
			fsProjectStoreRef: fsProjectStoreRef{
				fsProjectStore: fsProjectStore{
					projectID:   projectID,
					projectPath: projectPath,
				},
			},
		},
	}

	t.Run("GetID", func(t *testing.T) {
		assert.Equal(t, dbServer, store.ID())
	})

	t.Run("LoadDbServerSummary", func(t *testing.T) {
		summary, err := store.LoadDbServerSummary(context.Background(), dbServer)
		assert.NoError(t, err)
		assert.NotNil(t, summary)
		assert.Equal(t, dbServer, summary.DbServer)
		assert.Len(t, summary.Catalogs, 1)
		assert.Equal(t, "db1", summary.Catalogs[0].ID)
		assert.Contains(t, summary.Catalogs[0].Environments, envName)
	})

	t.Run("DeleteDbServer", func(t *testing.T) {
		err := store.DeleteDbServer(context.Background(), dbServer)
		assert.NoError(t, err)
		_, err = os.Stat(serverPath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("fsDbServersStore", func(t *testing.T) {
		dbServersStore := store.fsDbServersStore

		t.Run("DbServer", func(t *testing.T) {
			assert.NotNil(t, dbServersStore.DbServer(dbServer))
		})

		t.Run("dbServer", func(t *testing.T) {
			assert.NotNil(t, dbServersStore.dbServer(dbServer))
		})
	})

	//t.Run("SaveDbServer", func(t *testing.T) {
	//	project := datatug.Project{
	//		Repository: &datatug.ProjectRepository{},
	//	}
	//	projDbServer := datatug.ProjDbServer{
	//		Server: dbServer,
	//	}
	//	err := store.SaveDbServer(context.Background(), projDbServer, project)
	//	assert.NoError(t, err)
	//	assert.DirExists(t, path.Join(projectPath, DatatugFolder, ServersFolder, DbFolder, dbServer.Driver, dbServer.FileName()))
	//})
}
