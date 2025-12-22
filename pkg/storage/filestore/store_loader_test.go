package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
)

func TestNewSingleProjectLoader(t *testing.T) {
	path := "/path/p1"
	loader, projectID := NewSingleProjectLoader(path)
	assert.Equal(t, storage.SingleProjectID, projectID)
	assert.NotNil(t, loader)

	fsps, ok := loader.(fsProjectStore)
	if !ok {
		t.Fatalf("expected fsProjectStore, got %T", loader)
	}
	assert.Equal(t, path, fsps.projectPath)
	assert.Equal(t, storage.SingleProjectID, fsps.projectID)
}

func TestLoadProjectFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	t.Run("not_exists", func(t *testing.T) {
		_, err := LoadProjectFile(tmpDir)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, datatug.ErrProjectDoesNotExist))
	})

	t.Run("exists", func(t *testing.T) {
		datatugDir := filepath.Join(tmpDir, DatatugFolder)
		err := os.MkdirAll(datatugDir, 0777)
		assert.NoError(t, err)

		projFile := datatug.ProjectFile{
			ProjectItem: datatug.ProjectItem{
				ProjItemBrief: datatug.ProjItemBrief{
					Title: "Test Project",
				},
			},
		}
		data, _ := json.Marshal(projFile)
		err = os.WriteFile(filepath.Join(datatugDir, ProjectSummaryFileName), data, 0644)
		assert.NoError(t, err)

		v, err := LoadProjectFile(tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "Test Project", v.Title)
	})
}

func TestFsProjectStore_LoadProjectSummary(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	datatugDir := filepath.Join(tmpDir, DatatugFolder)
	err = os.MkdirAll(datatugDir, 0777)
	assert.NoError(t, err)

	projFile := datatug.ProjectFile{
		ProjectItem: datatug.ProjectItem{
			ProjItemBrief: datatug.ProjItemBrief{
				Title: "Test Project",
			},
		},
	}
	data, _ := json.Marshal(projFile)
	err = os.WriteFile(filepath.Join(datatugDir, ProjectSummaryFileName), data, 0644)
	assert.NoError(t, err)

	ps := newFsProjectStore("p1", tmpDir)
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
