package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/store"
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
		projFile, err := LoadProjectFile(path)
		if err != nil {
			return projectBriefs, fmt.Errorf("failed to load project file: %w", err)
		}
		projectBriefs[i].Title = projFile.Title
		i++
	}
	return
}

// NewStore create a store for multiple projects by their dir paths
func NewStore(pathsByID map[string]string) (fsStore *FileSystemStore, err error) {
	return newStore(pathsByID), nil
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
