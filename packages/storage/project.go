package storage

import (
	"context"
	"github.com/datatug/datatug/packages/models"
)

type ProjectStoreRef interface {
	Project() ProjectStore
}

type ProjectStore interface {
	ID() string
	Environments() EnvironmentsStore
	Queries() QueriesStore
	Boards() BoardsStore
	Entities() EntitiesStore
	DbModels() DbModelsStore
	DbServers() DbServersStore
	Recordsets() RecordsetsStore

	SaveProject(ctx context.Context, project models.DatatugProject) (err error)
	// LoadProject returns full DataTug project
	LoadProject(ctx context.Context) (*models.DatatugProject, error)

	// LoadProjectSummary return summary metadata about DataTug project
	LoadProjectSummary(ctx context.Context) (models.ProjectSummary, error)
}
