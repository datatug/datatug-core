package filestore

import (
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

	projectID := "test_p"
	projectPath := path.Join(tmpDir, projectID)
	envID := "dev"

	dbServer := datatug.ServerReference{
		Driver: "sqlserver",
		Host:   "localhost",
	}
	catalogID := "db1"

	fsProjectStore := newFsProjectStore(projectID, projectPath)
	envsDirPath := path.Join(projectPath, DatatugFolder, EnvironmentsFolder)

	fsEnvironmentsStore := fsEnvironmentsStore{
		fsProjectStoreRef: fsProjectStoreRef{
			fsProjectStore: fsProjectStore,
		},
		envsDirPath: envsDirPath,
	}

	envStore := newFsEnvironmentStore(envID, fsEnvironmentsStore)
	envServersStore := newFsEnvServersStore(envStore)
	envServerStore := newFsEnvServerStore(dbServer.FileName(), envServersStore)
	envCatalogsStore := newFsEnvCatalogsStore(envServerStore)
	store := newFsEnvCatalogStore(catalogID, envCatalogsStore)

	t.Run("Catalogs", func(t *testing.T) {
		assert.NotNil(t, store.Catalogs())
	})

	//t.Run("LoadEnvironmentCatalog", func(t *testing.T) {
	//	envPath := path.Join(envsDirPath, envID, ServersFolder, DbFolder, dbServer.FileName(), DbCatalogsFolder)
	//	err := os.MkdirAll(envPath, 0755)
	//	assert.NoError(t, err)
	//
	//	envDbCatalog := datatug.DbCatalog{
	//		DbCatalogBase: datatug.DbCatalogBase{
	//			ProjectItem: datatug.ProjectItem{
	//				ProjItemBrief: datatug.ProjItemBrief{
	//					ID: catalogID,
	//				},
	//			},
	//		},
	//	}
	//	data, _ := json.Marshal(envDbCatalog)
	//	err = os.WriteFile(path.Join(envPath, jsonFileName(catalogID, dbCatalogFileSuffix)), data, 0644)
	//	assert.NoError(t, err)
	//
	//	loaded, err := store.LoadEnvironmentCatalog()
	//	assert.NoError(t, err)
	//	assert.Equal(t, catalogID, loaded.ID)
	//})

	t.Run("SaveDbCatalog", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = store.SaveDbCatalog(nil)
		})
	})
}
