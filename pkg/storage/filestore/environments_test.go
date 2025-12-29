package filestore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func TestEnvironmentsStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_envs_test")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	projectID := "p1"
	projectPath := filepath.Join(tmpDir, projectID)
	envsDir := filepath.Join(projectPath, DatatugFolder, EnvironmentsFolder)
	err = os.MkdirAll(envsDir, 0777)
	assert.NoError(t, err)

	envID := "dev"
	envDir := filepath.Join(envsDir, envID)
	err = os.MkdirAll(envDir, 0777)
	assert.NoError(t, err)

	envFile := datatug.EnvironmentFile{ID: envID}
	envData, _ := json.Marshal(envFile)
	err = os.WriteFile(filepath.Join(envDir, "dev.env.json"), envData, 0644)
	assert.NoError(t, err)

	err = os.MkdirAll(filepath.Join(envsDir, "dev", "servers"), 0777)
	assert.NoError(t, err)

	store := newFsProjectStore(projectID, projectPath)
	store.projectPath = projectPath // Ensure projectPath is set correctly
	ctx := context.Background()

	t.Run("loadEnvironments", func(t *testing.T) {
		envs, err := store.loadEnvironments(ctx)
		if err != nil {
			t.Fatalf("failed to load environments: %v", err)
		}
		assert.Len(t, envs, 1)
		assert.Equal(t, envID, envs[0].ID)
	})

	t.Run("fsEnvironmentsStore", func(t *testing.T) {
		envsStore := newFsEnvironmentsStore(store)
		envStore := envsStore.environment(envID)
		envStore.envPath = filepath.Join(envsDir, envID) // Manually set envPath as newFsEnvironmentStore doesn't set it
		assert.Equal(t, envID, envStore.envID)
		assert.NotNil(t, envStore.Project())
		assert.NotNil(t, envStore.Servers())

		t.Run("LoadEnvironmentSummary", func(t *testing.T) {
			summary, err := envStore.LoadEnvironmentSummary()
			if err != nil {
				t.Logf("envsDirPath: %v", envStore.envsDirPath)
				t.Logf("envID: %v", envStore.envID)
				t.Logf("envPath: %v", envStore.envPath)
				t.Fatalf("failed to load environment summary: %v", err)
			}
			assert.Equal(t, envID, summary.ID)
		})

		t.Run("saveEnvironment", func(t *testing.T) {
			env := datatug.Environment{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "prod"}},
			}
			err := envsStore.saveEnvironments(ctx, datatug.Project{
				Environments: []*datatug.Environment{&env},
			})
			assert.NoError(t, err)
			assert.DirExists(t, filepath.Join(envsDir, "prod"))
		})
	})
}
