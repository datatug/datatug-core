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
	envsDir := filepath.Join(projectPath, EnvironmentsFolder)
	err = os.MkdirAll(envsDir, 0777)
	assert.NoError(t, err)

	envID := "dev"
	envFile := datatug.EnvironmentFile{ID: envID}
	envData, _ := json.Marshal(envFile)

	envDir := filepath.Join(envsDir, envID)
	err = os.MkdirAll(envDir, 0777)
	assert.NoError(t, err)

	err = os.WriteFile(filepath.Join(envDir, environmentSummaryFileName), envData, 0644)
	assert.NoError(t, err)

	err = os.MkdirAll(filepath.Join(envDir, "dev", "servers"), 0777)
	assert.NoError(t, err)

	store := newFsProjectStore(projectID, projectPath)
	ctx := context.Background()

	t.Run("loadEnvironments", func(t *testing.T) {
		envs, err := store.LoadEnvironments(ctx)
		assert.NoError(t, err)
		assert.Len(t, envs, 1)
		assert.Equal(t, envID, envs[0].ID)
	})

	t.Run("fsEnvironmentsStore", func(t *testing.T) {
		envsStore := newFsEnvironmentsStore(projectPath)

		t.Run("LoadEnvironmentSummary", func(t *testing.T) {
			summary, err := envsStore.LoadEnvironmentSummary(ctx, envID)
			if err != nil {
				t.Logf("dirPath: %v", envsStore.dirPath)
				t.Fatalf("failed to load environment summary: %v", err)
			}
			assert.Equal(t, envID, summary.ID)
		})

		t.Run("SaveEnvironments", func(t *testing.T) {
			env := datatug.Environment{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "prod"}},
			}
			err := envsStore.SaveEnvironments(ctx, datatug.Environments{&env})
			assert.NoError(t, err)
			assert.FileExists(t, filepath.Join(envsDir, "prod", environmentSummaryFileName))
		})
	})
}
