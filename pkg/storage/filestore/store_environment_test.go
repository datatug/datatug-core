package filestore

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func TestFsEnvironmentStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_envstore")
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	projectID := "test_p"
	projectPath := path.Join(tmpDir, projectID)
	datatugDir := path.Join(projectPath, DatatugFolder)
	envID := "dev"

	envsStore := newFsEnvironmentsStore(datatugDir)
	envsDirPath := path.Join(datatugDir, EnvironmentsFolder)
	ctx := context.Background()

	t.Run("LoadEnvironmentSummary", func(t *testing.T) {
		envPath := path.Join(envsDirPath, envID)
		err = os.MkdirAll(envPath, 0755)
		assert.NoError(t, err)

		envFile := datatug.EnvironmentFile{ID: envID}
		data, _ := json.Marshal(envFile)
		err = os.WriteFile(path.Join(envPath, environmentSummaryFileName), data, 0644)
		assert.NoError(t, err)

		summary, err := envsStore.LoadEnvironmentSummary(ctx, envID)
		assert.NoError(t, err)
		assert.NotNil(t, summary)
		assert.Equal(t, envID, summary.ID)
	})

	t.Run("loadEnvServers", func(t *testing.T) {
		envPath := path.Join(envsDirPath, envID, ServersFolder, DbFolder)
		err := os.MkdirAll(envPath, 0755)
		assert.NoError(t, err)

		servers := []*datatug.EnvDbServer{
			{
				ServerReference: datatug.ServerReference{Driver: "sqlserver", Host: "localhost"},
			},
		}
		data, _ := json.Marshal(servers)
		err = os.WriteFile(path.Join(envPath, "localhost.server.json"), data, 0644)
		assert.NoError(t, err)

		env := &datatug.Environment{}
		err = loadEnvServers(envPath, env)
		assert.NoError(t, err)
		assert.Len(t, env.DbServers, 1)
		assert.Equal(t, "localhost", env.DbServers[0].Host)

		// Test mismatching host
		servers2 := []*datatug.EnvDbServer{
			{
				ServerReference: datatug.ServerReference{Driver: "sqlserver", Host: "mismatch"},
			},
		}
		data2, _ := json.Marshal(servers2)
		err = os.WriteFile(path.Join(envPath, "localhost2.server.json"), data2, 0644)
		assert.NoError(t, err)
		err = loadEnvServers(envPath, env)
		assert.Error(t, err)
	})

	t.Run("SaveEnvironment", func(t *testing.T) {
		env := &datatug.Environment{
			ProjectItem: datatug.ProjectItem{
				ProjItemBrief: datatug.ProjItemBrief{
					ID: "prod",
				},
			},
		}
		err := envsStore.SaveEnvironment(ctx, env)
		assert.NoError(t, err)

		envFilePath := path.Join(envsDirPath, envID, environmentSummaryFileName)
		assert.FileExists(t, envFilePath)
	})
}
