package filestore

import (
	"context"
	"os"
	"path"
	"testing"

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
	serverID := "sqlserver:localhost"
	catalogID := "db1"

	store := newFsEnvCatalogsStore(path.Join(projectPath, DatatugFolder, EnvironmentsFolder))

	t.Run("LoadEnvDbCatalogs", func(t *testing.T) {
		ctx := context.Background()
		items, err := store.LoadEnvDbCatalogs(ctx, envID)
		assert.NoError(t, err)
		assert.Empty(t, items)
	})

	t.Run("LoadEnvDbCatalog", func(t *testing.T) {
		ctx := context.Background()
		assert.Panics(t, func() {
			_, _ = store.LoadEnvDbCatalog(ctx, envID, serverID, catalogID)
		})
	})

	t.Run("SaveEnvDbCatalog", func(t *testing.T) {
		ctx := context.Background()
		assert.Panics(t, func() {
			_ = store.SaveEnvDbCatalog(ctx, envID, serverID, catalogID, nil)
		})
	})
}
