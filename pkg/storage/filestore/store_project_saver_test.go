package filestore

import (
	"context"
	"os"
	"path"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
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
				ProjEntityBrief: datatug.ProjEntityBrief{
					ProjItemBrief: datatug.ProjItemBrief{
						ID:    "entity1",
						Title: "Entity 1",
					},
				},
			},
		},
		Boards: datatug.Boards{
			{
				ProjBoardBrief: datatug.ProjBoardBrief{
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
		assert.FileExists(t, path.Join(projectPath, ProjectSummaryFileName))
	})

	t.Run("SaveProject_MissingProjectID", func(t *testing.T) {
		err := store.SaveProject(context.Background(), &datatug.Project{})
		assert.Error(t, err)
	})
}
