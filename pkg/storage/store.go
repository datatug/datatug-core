package storage

import (
	"context"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/dto"
)

// Store defines interface for loading & saving DataTug projects
// Each store can keep multiple projects.
// Projects can be stored locally on file system or on server on some database.
type Store interface {
	GetProjectStore(projectID string) datatug.ProjectStore

	// CreateProject creates a new DataTug project
	CreateProject(ctx context.Context, request dto.CreateProjectRequest) (summary *datatug.ProjectSummary, err error)
	DeleteProject(ctx context.Context, id string) error

	// GetProjects returns list of projects
	GetProjects(ctx context.Context) (projectBriefs []datatug.ProjectBrief, err error)
}

// NewNoOpStore creates a DataTug store that panics in all methods
//func NewNoOpStore() Store {
//	return noOpStore{}
//}

var _ Store = (*noOpStore)(nil)

func NewNoOpStore() Store {
	return noOpStore{}
}

// noOpStore implements Store and panics in all methods. At the moment is used in some unit tests.
type noOpStore struct {
}

// CreateProject - noOpStore panics in all methods
func (n noOpStore) CreateProject(_ context.Context, _ dto.CreateProjectRequest) (summary *datatug.ProjectSummary, err error) {
	panic("implement me")
}

func (n noOpStore) GetProjects(_ context.Context) (projectBriefs []datatug.ProjectBrief, err error) {
	panic("implement me")
}

// GetProjectStore - noOpStore panics in all methods
func (n noOpStore) GetProjectStore(id string) datatug.ProjectStore {
	panic("implement me, id=" + id)
}

// DeleteProject - noOpStore panics in all methods
func (n noOpStore) DeleteProject(_ context.Context, id string) error {
	panic("implement me, id=" + id)
}
