package filestore

import (
	"context"
	"os"
	"path"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
)

func TestSaveProject(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_save_project")
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	projectID := "test_save_project"
	projectPath := path.Join(tmpDir, projectID)

	store := newFsProjectStore(projectID, projectPath)

	project := &datatug.Project{
		ProjectItem: datatug.ProjectItem{
			Access: "public",
			ProjItemBrief: datatug.ProjItemBrief{
				ID:    projectID,
				Title: "Test Project",
			},
		},
		Created: &datatug.ProjectCreated{
			At: time.Now(),
		},
		DbModels: datatug.DbModels{
			{
				ProjectItem: datatug.ProjectItem{
					ProjItemBrief: datatug.ProjItemBrief{
						ID:    "model1",
						Title: "Model 1",
					},
				},
			},
		},
		Environments: datatug.Environments{
			{
				ProjectItem: datatug.ProjectItem{
					ProjItemBrief: datatug.ProjItemBrief{
						ID:    "env1",
						Title: "Env 1",
					},
				},
			},
		},
		Entities: datatug.Entities{
			{
				ProjectItem: datatug.ProjectItem{
					ProjItemBrief: datatug.ProjItemBrief{
						ID:    "entity1",
						Title: "Entity 1",
					},
				},
			},
		},
		Boards: datatug.Boards{
			{
				ProjectItem: datatug.ProjectItem{
					ProjItemBrief: datatug.ProjItemBrief{
						ID:    "board1",
						Title: "Board 1",
					},
				},
			},
		},
	}

	t.Run("SaveProject_Full", func(t *testing.T) {
		err := store.SaveProject(context.Background(), project)
		assert.NoError(t, err)

		// Verify project file exists
		assert.FileExists(t, path.Join(projectPath, storage.ProjectSummaryFileName))
	})

	t.Run("SaveProject_MissingProjectID", func(t *testing.T) {
		err := store.SaveProject(context.Background(), &datatug.Project{})
		assert.Error(t, err)
	})
}

// TestSaveProject_PersistsDbModels proves SaveProject actually writes
// project.DbModels to disk and that they round-trip back through
// LoadDbModels - the DB-models block used to be a no-op stub
// ("IS NOT IMPLEMENTED YET") despite fsDbModelsStore.SaveDbModels already
// being implemented and embedded on the store.
func TestSaveProject_PersistsDbModels(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_save_dbmodels")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	const projectID = "p1"
	projectPath := path.Join(tmpDir, projectID)
	store := newFsProjectStore(projectID, projectPath)

	project := &datatug.Project{
		ProjectItem: datatug.ProjectItem{
			Access:        "private",
			ProjItemBrief: datatug.ProjItemBrief{ID: projectID},
		},
		Created: &datatug.ProjectCreated{At: time.Now()},
		DbModels: datatug.DbModels{
			{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "chinook", Title: "Chinook"}},
			},
		},
	}

	err = store.SaveProject(context.Background(), project)
	assert.NoError(t, err)

	modelFile := path.Join(projectPath, storage.DbModelsFolder, "chinook", storage.JsonFileName("chinook", storage.DbModelFileSuffix))
	assert.FileExists(t, modelFile)

	loaded, err := newFsDbModelsStore(projectPath).LoadDbModels(context.Background())
	assert.NoError(t, err)
	if assert.Len(t, loaded, 1) {
		assert.Equal(t, "chinook", loaded[0].ID)
		assert.Equal(t, "Chinook", loaded[0].Title)
	}
}
