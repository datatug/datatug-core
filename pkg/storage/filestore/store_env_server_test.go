package filestore

import (
	"os"
	"path"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
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

	dbServer := datatug.ServerReference{
		Driver: "sqlserver",
		Host:   "localhost",
	}

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
	store := newFsEnvServerStore(dbServer.FileName(), envServersStore)

	t.Run("Catalogs", func(t *testing.T) {
		assert.NotNil(t, store.Catalogs())
	})

	//t.Run("LoadEnvServer", func(t *testing.T) {
	//	envPath := path.Join(envsDirPath, envID, ServersFolder, DbFolder)
	//	err := os.MkdirAll(envPath, 0755)
	//	assert.NoError(t, err)
	//
	//	envDbServer := datatug.EnvDbServer{
	//		ServerReference: dbServer,
	//		Catalogs:        []string{"db1"},
	//	}
	//	data, _ := json.Marshal(envDbServer)
	//	err = os.WriteFile(path.Join(envPath, jsonFileName(dbServer.FileName(), serverFileSuffix)), data, 0644)
	//	assert.NoError(t, err)
	//
	//	loaded, err := store.LoadEnvServer()
	//	assert.NoError(t, err)
	//	assert.Equal(t, dbServer.Host, loaded.Host)
	//})

	//t.Run("SaveEnvServer", func(t *testing.T) {
	//	envDbServer := &datatug.EnvDbServer{
	//		ServerReference: dbServer,
	//	}
	//	err := store.SaveEnvServer(envDbServer)
	//	assert.NoError(t, err)
	//})

	t.Run("saveEnvServers", func(t *testing.T) {
		servers := []*datatug.EnvDbServer{
			{ServerReference: dbServer},
		}
		err := envServersStore.saveEnvServers(servers)
		assert.NoError(t, err)
	})
}
