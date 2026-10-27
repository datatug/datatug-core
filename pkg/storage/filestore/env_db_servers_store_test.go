package filestore

import (
	"context"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// Two servers recorded without a host (their connection URLs stay in environment variables) are saved to
// two files, one per driver, and both come back.
func TestFsEnvServerStore_DriverOnlyServersDoNotCollide(t *testing.T) {
	projectPath := t.TempDir()
	store := newFsEnvDbServersStore(projectPath)
	ctx := context.Background()
	mysqlServer := &datatug.EnvDbServer{ServerRef: datatug.ServerRef{Driver: "mysql"}, Catalogs: []string{"shop"}}
	oracleServer := &datatug.EnvDbServer{ServerRef: datatug.ServerRef{Driver: "oracle"}, Catalogs: []string{"hr"}}
	assert.NoError(t, store.SaveEnvDbServer(ctx, "local", mysqlServer))
	assert.NoError(t, store.SaveEnvDbServer(ctx, "local", oracleServer))

	servers, err := store.LoadEnvDbServers(ctx, "local")
	assert.NoError(t, err)
	assert.Len(t, servers, 2)
	loaded, err := store.LoadEnvDbServer(ctx, "local", "oracle")
	assert.NoError(t, err)
	assert.Equal(t, "oracle", loaded.Driver)
	assert.Equal(t, []string{"hr"}, loaded.Catalogs)
}

// A project recording couchdb and postgrest servers natively - by driver
// alone, with the real connection URL carried by a connection descriptor or
// environment reference - must survive the persisted environment-server
// round trip, keep every server host/port-free, and still validate as a whole
// project. The persisted JSON must carry no credentials: ServerRef holds only
// driver/host/path/port and EnvDbServer its catalogs, so the account-free
// reference stays account-free on disk.
func TestFsEnvServerStore_NativeProviderServersRoundTripWithoutCredentials(t *testing.T) {
	projectPath := t.TempDir()
	store := newFsEnvDbServersStore(projectPath)
	ctx := context.Background()

	saved := datatug.EnvDbServers{
		{ServerRef: datatug.ServerRef{Driver: "couchdb"}, Catalogs: []string{"documents"}},
		{ServerRef: datatug.ServerRef{Driver: "postgrest"}, Catalogs: []string{"public"}},
	}
	require.NoError(t, store.SaveEnvServers(ctx, "local", saved))

	loaded, err := store.LoadEnvDbServers(ctx, "local")
	require.NoError(t, err)
	require.Len(t, loaded, 2)

	byDriver := make(map[string]*datatug.EnvDbServer, len(loaded))
	for _, server := range loaded {
		byDriver[server.Driver] = server
		assert.Empty(t, server.Host, server.Driver)
		assert.Zero(t, server.Port, server.Driver)
	}
	require.NotNil(t, byDriver["couchdb"])
	assert.Equal(t, []string{"documents"}, byDriver["couchdb"].Catalogs)
	require.NotNil(t, byDriver["postgrest"])
	assert.Equal(t, []string{"public"}, byDriver["postgrest"].Catalogs)

	// The persisted project validates through the shared model without any
	// account, token or URL being embedded.
	project := &datatug.Project{
		ProjectItem: datatug.ProjectItem{Access: "private"},
		Environments: datatug.Environments{{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "local"}},
			DbServers:   loaded,
		}},
	}
	require.NoError(t, project.Validate())

	for _, id := range []string{"couchdb", "postgrest"} {
		raw, err := os.ReadFile(path.Join(projectPath, storage.EnvironmentsFolder, "local", storage.JsonFileName(id, storage.ServerFileSuffix)))
		require.NoError(t, err, id)
		for _, credential := range []string{"password", "secret", "token", "://", "user"} {
			assert.NotContains(t, strings.ToLower(string(raw)), credential, id)
		}
	}
}
