package filestore

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
)

func TestFsEnvServerStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_envserver")
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	projectPath := tmpDir
	envID := "dev"

	store := newFsEnvDbServersStore(projectPath)
	ctx := context.Background()

	server1 := &datatug.EnvDbServer{
		ServerRef: datatug.ServerRef{
			Driver: "sqlserver",
			Host:   "localhost",
			Port:   1433,
		},
	}

	t.Run("SaveEnvDbServer", func(t *testing.T) {
		err := store.SaveEnvDbServer(ctx, envID, server1)
		assert.NoError(t, err)

		// Verify file exists
		serverPath := path.Join(projectPath, storage.EnvironmentsFolder, envID, "localhost:1433."+storage.ServerFileSuffix+".json")
		_, err = os.Stat(serverPath)
		assert.NoError(t, err)
	})

	t.Run("LoadEnvDbServer", func(t *testing.T) {
		loadedServer, err := store.LoadEnvDbServer(ctx, envID, "localhost:1433")
		assert.NoError(t, err)
		assert.Equal(t, server1.Host, loadedServer.Host)
		assert.Equal(t, server1.Port, loadedServer.Port)
	})

	t.Run("LoadEnvDbServers", func(t *testing.T) {
		servers, err := store.LoadEnvDbServers(ctx, envID)
		assert.NoError(t, err)
		assert.Len(t, servers, 1)
		assert.Equal(t, server1.Host, servers[0].Host)
	})

	t.Run("SaveEnvServers", func(t *testing.T) {
		server2 := &datatug.EnvDbServer{
			ServerRef: datatug.ServerRef{
				Driver: "sqlserver",
				Host:   "remotehost",
				Port:   1433,
			},
		}
		err := store.SaveEnvServers(ctx, envID, datatug.EnvDbServers{server1, server2})
		assert.NoError(t, err)

		servers, err := store.LoadEnvDbServers(ctx, envID)
		assert.NoError(t, err)
		assert.Len(t, servers, 2)
	})

	t.Run("DeleteEnvDbServer", func(t *testing.T) {
		err := store.DeleteEnvDbServer(ctx, envID, "localhost:1433")
		assert.NoError(t, err)

		servers, err := store.LoadEnvDbServers(ctx, envID)
		assert.NoError(t, err)
		assert.Len(t, servers, 1)
	})
}
