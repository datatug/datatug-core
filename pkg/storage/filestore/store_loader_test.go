package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func TestLoadProjectFile(t *testing.T) {
	projectTempDir, err := os.MkdirTemp("", "datatug_test_project")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(projectTempDir)
	}()

	t.Run("not_exists", func(t *testing.T) {
		_, err := LoadProjectFile(projectTempDir)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, datatug.ErrProjectDoesNotExist))
	})

	t.Run("exists", func(t *testing.T) {
		projFile := datatug.ProjectFile{
			ProjectItem: datatug.ProjectItem{
				ProjItemBrief: datatug.ProjItemBrief{
					Title: "Test Project",
				},
			},
		}
		data, _ := json.Marshal(projFile)
		err = os.WriteFile(filepath.Join(projectTempDir, ProjectSummaryFileName), data, 0644)
		assert.NoError(t, err)

		v, err := LoadProjectFile(projectTempDir)
		assert.NoError(t, err)
		assert.Equal(t, "Test Project", v.Title)
	})
}

func TestFsProjectStore_LoadProjectSummary(t *testing.T) {
	projTmpDir, err := os.MkdirTemp("", "datatug_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(projTmpDir)
	}()

	projFile := datatug.ProjectFile{
		ProjectItem: datatug.ProjectItem{
			ProjItemBrief: datatug.ProjItemBrief{
				Title: "Test Project",
			},
		},
	}
	data, _ := json.Marshal(projFile)
	err = os.WriteFile(filepath.Join(projTmpDir, ProjectSummaryFileName), data, 0644)
	assert.NoError(t, err)

	ps := newFsProjectStore("p1", projTmpDir)
	summary, err := ps.LoadProjectSummary(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "p1", ps.projectID)
	assert.Equal(t, "p1", summary.ID)
	assert.Equal(t, "Test Project", summary.Title)
}

func TestFileSystemLoader_GetProjectPath(t *testing.T) {
	loader := fileSystemLoader{
		pathByID: map[string]string{
			"p1": "/path/p1",
		},
	}

	t.Run("exists", func(t *testing.T) {
		id, path, err := loader.GetProjectPath("p1")
		assert.NoError(t, err)
		assert.Equal(t, "p1", id)
		assert.Equal(t, "/path/p1", path)
	})

	t.Run("not_exists", func(t *testing.T) {
		_, _, err := loader.GetProjectPath("p2")
		assert.Error(t, err)
	})
}
