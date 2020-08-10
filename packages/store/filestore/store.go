package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/store"
	"path/filepath"
)

var _ store.Interface = (*FileSystemStore)(nil)

// FileSystemStore provides implementation of file system store
type FileSystemStore struct {
	pathByID map[string]string
	fileSystemLoader
	storeSaver
}

// GetProjects returns list of projects
func (store FileSystemStore) GetProjects() (projectBriefs []models.ProjectBrief, err error) {
	projectBriefs = make([]models.ProjectBrief, len(store.pathByID))
	var i int
	for id, path := range store.pathByID {
		projectBriefs[i] = models.ProjectBrief{}
		projectBriefs[i].ID = id
		projFile, err := store.LoadProjectFile(path)
		if err != nil {
			return projectBriefs, fmt.Errorf("failed to load project file: %w", err)
		}
		projectBriefs[i].Title = projFile.Title
		i++
	}
	return
}

// NewStore create a store for multiple projects by their dir paths
func NewStore(paths []string) (fsStore *FileSystemStore, err error) {
	pathByID := make(map[string]string)
	for i, projPath := range paths {
		if projPath, err = filepath.Abs(projPath); err != nil {
			return nil, err
		}
		filesStore, _ := NewSingleProjectStore(projPath, store.SingleProjectID)
		var projFile models.ProjectFile
		if projFile, err = filesStore.LoadProjectFile(projPath); err != nil {
			return filesStore, err
		}
		SetProjectPath(projFile.ID, projPath)
		paths[i] = projPath
		pathByID[projFile.ID] = projPath
	}
	if len(projectPaths) == 1 {
		pathByID[store.SingleProjectID] = paths[0]
	}
	return newStore(pathByID), nil
}

// newStore creates an instance of store that implements store.Interface
func newStore(pathByID map[string]string) *FileSystemStore {
	return &FileSystemStore{
		pathByID:         pathByID,
		fileSystemLoader: newLoader(pathByID),
		storeSaver:       storeSaver{pathByID: pathByID},
	}
}

// NewSingleProjectStore creates an instance of store that implements store.Interface for a single project
func NewSingleProjectStore(path, projectID string) (storeInterface *FileSystemStore, projID string) {
	if projectID == "" {
		projID = store.SingleProjectID
	} else {
		projID = projectID
	}
	storeInterface = newStore(map[string]string{projID: path})
	return
}
