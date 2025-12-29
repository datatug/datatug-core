package filestore

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
)

func TestFsProjectStore_ProjectID(t *testing.T) {
	const projectID = "p1"
	store := fsProjectStore{projectID: projectID}
	assert.Equal(t, projectID, store.ProjectID())
}

func TestFsStore_Methods(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_fsstore")
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	projectID := "test_project"
	projectPath := path.Join(tmpDir, projectID)
	datatugPath := path.Join(projectPath, DatatugFolder)
	err = os.MkdirAll(datatugPath, 0755)
	assert.NoError(t, err)

	project := datatug.Project{
		ProjectItem: datatug.ProjectItem{
			ProjItemBrief: datatug.ProjItemBrief{
				ID:    projectID,
				Title: "Test Project",
			},
		},
		Created: &datatug.ProjectCreated{
			At: time.Now(),
		},
	}
	data, _ := json.Marshal(project)
	err = os.WriteFile(path.Join(datatugPath, ProjectSummaryFileName), data, 0644)
	assert.NoError(t, err)

	store, err := NewStore("test_store", map[string]string{projectID: projectPath})
	assert.NoError(t, err)

	t.Run("GetProjects", func(t *testing.T) {
		projects, err := store.GetProjects(context.Background())
		assert.NoError(t, err)
		assert.Len(t, projects, 1)
		assert.Equal(t, projectID, projects[0].ID)
		assert.Equal(t, "Test Project", projects[0].Title)
	})

	t.Run("GetProjectStore", func(t *testing.T) {
		pStore := store.GetProjectStore(projectID)
		assert.NotNil(t, pStore)
	})

	t.Run("DeleteProject", func(t *testing.T) {
		err := store.DeleteProject(context.Background(), projectID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not implemented yet")
	})

	t.Run("CreateProject", func(t *testing.T) {
		assert.Panics(t, func() {
			_, _ = store.CreateProject(context.Background(), dto.CreateProjectRequest{})
		})
	})
}

func TestNewSingleProjectStore(t *testing.T) {
	t.Run("with_id", func(t *testing.T) {
		path := "/path/p1"
		id := "p1"
		store, projID := NewSingleProjectStore(path, id)
		assert.Equal(t, id, projID)
		assert.Equal(t, path, store.pathByID[projID])
	})

	t.Run("without_id", func(t *testing.T) {
		path := "/path/p1"
		store, projID := NewSingleProjectStore(path, "")
		assert.Equal(t, storage.SingleProjectID, projID)
		assert.Equal(t, path, store.pathByID[projID])
	})
}
