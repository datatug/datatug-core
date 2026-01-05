package filestore

import (
	"context"
	"encoding/json"
	"errors"
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
	fsStoreTmpDir, err := os.MkdirTemp("", "datatug_test_fsstore")
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(fsStoreTmpDir)
	}()

	projectID := "test_project"
	projectPath := path.Join(fsStoreTmpDir, projectID)
	assert.NoError(t, os.MkdirAll(projectPath, 0777))

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
	summaryFilePath := path.Join(projectPath, storage.ProjectSummaryFileName)
	err = os.WriteFile(summaryFilePath, data, 0644)
	if !assert.NoError(t, err) {
		return
	}

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
		projPath := "/projPath/p1"
		id := "p1"
		store, projID := NewSingleProjectStore(projPath, id)
		assert.Equal(t, id, projID)
		assert.Equal(t, projPath, store.pathByID[projID])
	})

	t.Run("without_id", func(t *testing.T) {
		projPath := "/projPath/p1"
		store, projID := NewSingleProjectStore(projPath, "")
		assert.Equal(t, storage.SingleProjectID, projID)
		assert.Equal(t, projPath, store.pathByID[projID])
	})
}

func TestNewProjectStore(t *testing.T) {
	type args struct {
		id   string
		path string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "p1",
			args: args{id: "p1", path: "/projPath/p1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewProjectStore(tt.args.id, tt.args.path)
			assert.NotNil(t, store)
			_, err := store.LoadProjectFile(context.Background())
			assert.Error(t, err)
			assert.True(t, errors.Is(err, datatug.ErrProjectDoesNotExist), err)
		})
	}
}
