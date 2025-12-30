package filestore

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/datatug2md"
	"github.com/stretchr/testify/assert"
)

func TestFsDbServersStore_SaveDbServers(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_dbservers")
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	projectID := "test_project"
	projectPath := path.Join(tmpDir, projectID)

	fsProjectStore := fsProjectStore{
		projectID:     projectID,
		projectPath:   projectPath,
		readmeEncoder: datatug2md.NewEncoder(),
	}
	store := newFsDbServersStore(fsProjectStore)

	dbServer1 := &datatug.ProjDbServer{
		ProjectItem: datatug.ProjectItem{
			ProjItemBrief: datatug.ProjItemBrief{
				ID: "sqlserver:localhost",
			},
		},
		Server: datatug.ServerReference{
			Driver: "sqlserver",
			Host:   "localhost",
		},
	}
	dbServers := datatug.ProjDbServers{dbServer1}
	project := datatug.Project{
		Repository: &datatug.ProjectRepository{},
	}

	t.Run("saveDbServers", func(t *testing.T) {
		err := store.saveDbServers(context.Background(), dbServers, project)
		assert.NoError(t, err)
	})

	t.Run("saveDbServers_empty", func(t *testing.T) {
		err := store.saveDbServers(context.Background(), nil, project)
		assert.NoError(t, err)
	})

	t.Run("saveDbServersReadme", func(t *testing.T) {
		err := store.saveDbServersReadme(dbServers)
		assert.NoError(t, err)
	})
}
