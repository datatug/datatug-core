package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/models2md"
	"github.com/datatug/datatug/packages/storage"
	"sync"
)

// NewStore create a storage for multiple projects by their dir paths
func NewStore(id string, pathsByID map[string]string) (fsStore *fsStore, err error) {
	return newStore(id, pathsByID), nil
}

var _ storage.Store = (*fsStore)(nil)

// fsStore provides implementation of file system storage
type fsStore struct {
	id               string
	pathByID         map[string]string
	fileSystemLoader // TODO: To be deleted
	//storeSaver       // TODO: To be deleted
}

func (store fsStore) Project(id string) storage.ProjectStore {
	path := store.pathByID[id]
	return newFsProjectStore(id, path)
}

var _ storage.ProjectStore = (*fsProjectStore)(nil)

type fsProjectStore struct {
	projectID     string
	projectPath   string
	projFileMutex *sync.Mutex
	readmeEncoder models.ReadmeEncoder
}

type fsProjectStoreRef struct {
	fsProjectStore
}

func (ps fsProjectStoreRef) Project() storage.ProjectStore {
	return ps.fsProjectStore
}

func (store fsProjectStore) ID() string {
	return store.projectID
}

func (store fsProjectStore) DbModels() storage.DbModelsStore {
	return newFsDbModelsStore(store)
}

func (store fsProjectStore) Environments() storage.EnvironmentsStore {
	return newFsEnvironmentsStore(store)
}

func (store fsProjectStore) Boards() storage.BoardsStore {
	return newFsBoardsStore(store)
}

func (store fsProjectStore) Entities() storage.EntitiesStore {
	panic("implement me")
}

func (store fsProjectStore) DbServers() storage.DbServersStore {
	return newFsDbServersStore(store)
}

func (store fsProjectStore) Recordsets() storage.RecordsetsStore {
	panic("implement me")
}

func newFsProjectStore(id string, projectPath string) fsProjectStore {
	return fsProjectStore{
		projectID:     id,
		projectPath:   projectPath,
		readmeEncoder: models2md.NewEncoder(),
	}
}

func (store fsProjectStore) Queries() storage.QueriesStore {
	return newFsQueriesStore(store)
}

func (store fsProjectStore) Query(id string) storage.QueryStore {
	return newFsQueryStore(id, newFsQueriesStore(store))
}

// GetProjects returns list of projects
func (store fsStore) GetProjects() (projectBriefs []models.ProjectBrief, err error) {
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
		projectBriefs[i].Repository = projFile.Repository
		i++
	}
	return
}

// newStore creates an instance of storage that implements storage.Store
func newStore(id string, pathByID map[string]string) *fsStore {
	return &fsStore{
		id:       id,
		pathByID: pathByID,
	}
}

// NewSingleProjectStore creates an instance of storage that implements storage.Store for a single project
func NewSingleProjectStore(projectPath, projectID string) (storeInterface *fsStore, projID string) {
	if projectID == "" {
		projID = storage.SingleProjectID
	} else {
		projID = projectID
	}
	const storeID =  "single_project_file_store"
	storeInterface = newStore(storeID, map[string]string{projID: projectPath})
	return
}
