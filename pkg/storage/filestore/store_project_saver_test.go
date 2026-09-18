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
	"github.com/stretchr/testify/require"
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

// TestSaveProject_PersistsQueries proves SaveProject writes project.Queries
// to disk (folders and items) and that they round-trip back through
// LoadProject - "the query saver must round-trip".
func TestSaveProject_PersistsQueries(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_save_queries")
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
		Queries: &datatug.QueriesFolder{
			Folders: datatug.QueryFolders{
				{
					ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "customers"}},
					Items: datatug.QueryDefs{
						{
							ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "customer-invoices", Title: "Customer invoices"}},
							Type:        datatug.QueryTypeSQL,
							Text:        "SELECT 1",
						},
					},
				},
			},
		},
	}

	err = store.SaveProject(context.Background(), project)
	assert.NoError(t, err)

	queryFile := path.Join(projectPath, storage.QueriesFolder, "customers", "customer-invoices.query.json")
	assert.FileExists(t, queryFile)

	loadedProject, err := store.LoadProject(context.Background())
	assert.NoError(t, err)
	if assert.NotNil(t, loadedProject.Queries) && assert.Len(t, loadedProject.Queries.Folders, 1) {
		customers := loadedProject.Queries.Folders[0]
		if assert.Len(t, customers.Items, 1) {
			assert.Equal(t, "customer-invoices", customers.Items[0].ID)
			assert.Equal(t, "SELECT 1", customers.Items[0].Text)
		}
	}
}

// TestSaveProject_PersistsTitle mirrors dalgostore's own round-trip test:
// saveProjectFile used to drop project.Title although FsStore.GetProjects
// reads a project's title back out of the very file it writes
// (store.go:49), so an unmodified save wiped the title and produced a
// brief that fails its own datatug.ProjectBrief.Validate().
func TestSaveProject_PersistsTitle(t *testing.T) {
	const projectID = "test_save_project_title"
	projectPath := path.Join(t.TempDir(), projectID)
	store := newFsProjectStore(projectID, projectPath)

	project := &datatug.Project{
		ProjectItem: datatug.ProjectItem{
			Access: "private",
			ProjItemBrief: datatug.ProjItemBrief{
				ID:    projectID,
				Title: "Round Trip",
			},
		},
		Created: &datatug.ProjectCreated{At: time.Now()},
	}
	require.NoError(t, store.SaveProject(context.Background(), project))

	projFile, err := LoadProjectFile(projectPath)
	require.NoError(t, err)
	assert.Equal(t, "Round Trip", projFile.Title, "the title must survive a save round-trip")
	assert.Equal(t, "private", projFile.Access)

	// ...and the brief the project lists under is valid on its own terms.
	fsStore := newStore("s1", map[string]string{projectID: projectPath})
	projects, err := fsStore.GetProjects(context.Background())
	require.NoError(t, err)
	require.Len(t, projects, 1)
	assert.Equal(t, projectID, projects[0].ID)
	assert.Equal(t, "Round Trip", projects[0].Title, "an unmodified save must not wipe the title")
	assert.NoError(t, projects[0].Validate())
}
