package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
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
		err = os.WriteFile(filepath.Join(projectTempDir, storage.ProjectSummaryFileName), data, 0644)
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
	err = os.WriteFile(filepath.Join(projTmpDir, storage.ProjectSummaryFileName), data, 0644)
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

func TestFileSystemLoader_GetFolderPath(t *testing.T) {
	loader := fileSystemLoader{
		pathByID: map[string]string{
			"p1": "/path/p1",
		},
	}
	t.Run("exists", func(t *testing.T) {
		p, err := loader.GetFolderPath("p1", "f1", "f2")
		assert.NoError(t, err)
		assert.Equal(t, "/path/p1/f1/f2", p)
	})
	t.Run("not_exists", func(t *testing.T) {
		_, err := loader.GetFolderPath("p2", "f1")
		assert.Error(t, err)
	})
}

func TestFsProjectStore_LoadProject(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datatug_test_LoadProject")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	projFile := datatug.ProjectFile{
		ProjectItem: datatug.ProjectItem{
			ProjItemBrief: datatug.ProjItemBrief{
				Title: "Test Project",
			},
		},
	}
	data, _ := json.Marshal(projFile)
	err = os.WriteFile(filepath.Join(tempDir, storage.ProjectSummaryFileName), data, 0644)
	assert.NoError(t, err)

	ps := newFsProjectStore("p1", tempDir)
	project, err := ps.LoadProject(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, project)
	assert.Equal(t, "Test Project", project.Title)

	t.Run("with_db_servers", func(t *testing.T) {
		dbServersDir := filepath.Join(tempDir, storage.ServersFolder, storage.DbFolder)
		driverName := "sqlserver"
		serverName := "server1"
		serverDir := filepath.Join(dbServersDir, driverName, serverName)
		err = os.MkdirAll(serverDir, 0755)
		assert.NoError(t, err)

		dbServer := datatug.ProjDbServer{
			ProjectItem: datatug.ProjectItem{
				ProjItemBrief: datatug.ProjItemBrief{
					ID: serverName,
				},
			},
		}
		data, _ := json.Marshal(dbServer)
		jsonFileName := storage.JsonFileName(fmt.Sprintf("%v.%v", driverName, serverName), storage.DbServerFileSuffix)
		err = os.WriteFile(filepath.Join(serverDir, jsonFileName), data, 0644)
		assert.NoError(t, err)

		project, err := ps.LoadProject(context.Background())
		assert.NoError(t, err)
		assert.NotNil(t, project)
		assert.Len(t, project.DbServers, 1)
		assert.Equal(t, serverName, project.DbServers[0].ID)
	})
}
