package filestore

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
)

func TestProjectsLoader(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_projects_loader")
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	projectID := "test_project"
	projectDir := path.Join(tmpDir, projectID)
	err = os.MkdirAll(projectDir, 0755)
	assert.NoError(t, err)

	project := datatug.Project{
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
	}
	data, _ := json.Marshal(project)
	err = os.WriteFile(path.Join(projectDir, storage.ProjectSummaryFileName), data, 0644)
	assert.NoError(t, err)

	loader := NewProjectsLoader(tmpDir)
	loadedProject, err := loader.LoadProject(context.Background(), projectID)
	assert.NoError(t, err)
	assert.NotNil(t, loadedProject)
	assert.Equal(t, projectID, loadedProject.ID)
	assert.Equal(t, "Test Project", loadedProject.Title)

	// Test missing project
	_, err = loader.LoadProject(context.Background(), "missing_project")
	assert.Error(t, err)
}
