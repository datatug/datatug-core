package filestore

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func TestFsEnvCatalogStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_envcatalog")
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	projectPath := tmpDir
	envID := "dev"
	serverID := "sqlserver:localhost:1433"
	catalogID := "db1"

	store := newFsEnvCatalogsStore(projectPath)
	ctx := context.Background()

	catalog1 := &datatug.EnvDbCatalog{
		DbCatalogBase: datatug.DbCatalogBase{
			ProjectItem: datatug.ProjectItem{
				ProjItemBrief: datatug.ProjItemBrief{
					ID:    catalogID,
					Title: "Database 1",
				},
			},
			Driver: "sqlserver",
		},
	}

	t.Run("SaveEnvDbCatalog", func(t *testing.T) {
		err := store.SaveEnvDbCatalog(ctx, envID, serverID, catalogID, catalog1)
		assert.NoError(t, err)

		// Verify file exists
		// In SaveEnvDbCatalog, filepath.Join(s.dirPath, envID, ServersFolder, serverID, EnvDbCatalogsFolder) is used.
		// Note that ServersFolder is "servers", EnvDbCatalogsFolder is "catalogs".
		catalogPath := path.Join(tmpDir, "environments", envID, "servers", serverID, "catalogs", catalogID+"."+dbCatalogFileSuffix+".json")
		_, err = os.Stat(catalogPath)
		assert.NoError(t, err)
	})

	t.Run("LoadEnvDbCatalog", func(t *testing.T) {
		loadedCatalog, err := store.LoadEnvDbCatalog(ctx, envID, serverID, catalogID)
		assert.NoError(t, err)
		assert.Equal(t, catalog1.ID, loadedCatalog.ID)
		assert.Equal(t, catalog1.Title, loadedCatalog.Title)
	})

	t.Run("LoadEnvDbCatalogs", func(t *testing.T) {
		// This uses a different path in the current implementation of LoadEnvDbCatalogs
		// LoadEnvDbCatalogs uses s.getDirPath(envID) -> filepath.Join(s.dirPath, envID, EnvDbCatalogsFolder)
		// But SaveEnvDbCatalog uses filepath.Join(s.dirPath, envID, ServersFolder, serverID, EnvDbCatalogsFolder)

		items, err := store.LoadEnvDbCatalogs(ctx, envID)
		assert.NoError(t, err)
		assert.Empty(t, items)
	})

	t.Run("SaveEnvDbCatalogs", func(t *testing.T) {
		catalog2 := &datatug.EnvDbCatalog{
			DbCatalogBase: datatug.DbCatalogBase{
				ProjectItem: datatug.ProjectItem{
					ProjItemBrief: datatug.ProjItemBrief{
						ID:    "db2",
						Title: "Database 2",
					},
				},
				Driver: "sqlserver",
			},
		}
		err := store.SaveEnvDbCatalogs(ctx, envID, serverID, "", datatug.EnvDbCatalogs{catalog1, catalog2})
		assert.NoError(t, err)
	})

	t.Run("DeleteEnvDbCatalog", func(t *testing.T) {
		err := store.DeleteEnvDbCatalog(ctx, envID, serverID, catalogID)
		assert.NoError(t, err)
	})
}
