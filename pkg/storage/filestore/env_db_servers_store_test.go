package filestore

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFsEnvServerStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_envserver")
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	projectID := "test_p"
	projectPath := path.Join(tmpDir, projectID)
	envID := "dev"
	serverID := "sqlserver:localhost"

	store := newFsEnvDbServersStore(projectPath)

	t.Run("LoadEnvDbServers", func(t *testing.T) {
		ctx := context.Background()
		_, err := store.LoadEnvDbServers(ctx, envID)
		assert.Nil(t, err)
	})

	t.Run("LoadEnvDbServer", func(t *testing.T) {
		ctx := context.Background()
		_, err := store.LoadEnvDbServer(ctx, envID, serverID)
		assert.Error(t, err)
	})

	t.Run("SaveEnvDbServer", func(t *testing.T) {
		ctx := context.Background()
		err := store.SaveEnvDbServer(ctx, envID, nil)
		assert.Error(t, err)
	})
}
