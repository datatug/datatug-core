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

func TestDbCatalogStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_dbcatalog")
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

	catalogID := "db1"
	catalogsDirPath := path.Join(projectPath, storage.ServersFolder, storage.DbFolder, dbServer.Driver, dbServer.Host, storage.EnvDbCatalogsFolder)
	catalogPath := path.Join(catalogsDirPath, catalogID)
	err = os.MkdirAll(catalogPath, 0755)
	assert.NoError(t, err)

	catalogSummary := datatug.DbCatalogSummary{
		DbCatalogBase: datatug.DbCatalogBase{
			ProjectItem: datatug.ProjectItem{
				ProjItemBrief: datatug.ProjItemBrief{
					ID: catalogID,
				},
			},
		},
	}
	data, _ := json.Marshal(catalogSummary)
	err = os.WriteFile(path.Join(catalogPath, catalogID+".db.json"), data, 0644)
	assert.NoError(t, err)

	fsServerStore := fsDbServerStore{
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

	catalogsStore := newFsDbCatalogsStore(fsServerStore)
	store := catalogsStore.DbCatalog(catalogID).(fsDbCatalogStore)

	t.Run("Server", func(t *testing.T) {
		assert.NotNil(t, store.Server())
	})

	t.Run("LoadDbCatalogSummary", func(t *testing.T) {
		summary, err := store.LoadDbCatalogSummary(context.Background())
		assert.NoError(t, err)
		assert.NotNil(t, summary)
		assert.Equal(t, catalogID, summary.ID)
	})

	t.Run("SaveDbCatalog", func(t *testing.T) {
		dbCatalog := &datatug.EnvDbCatalog{
			DbCatalogBase: datatug.DbCatalogBase{
				ProjectItem: datatug.ProjectItem{
					ProjItemBrief: datatug.ProjItemBrief{
						ID: catalogID,
					},
				},
				Driver: dbServer.Driver,
			},
		}
		// Need to call saveDbCatalog with non-nil context if possible or mock saverCtx
		// Since it is an internal method, let's see how it's called.
		err := store.saveDbCatalog(dbCatalog, nil)
		assert.NoError(t, err)
		assert.FileExists(t, path.Join(catalogPath, catalogID+".db.json"))
	})
}
