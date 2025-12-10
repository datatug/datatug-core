package storage

import (
	"context"

	"github.com/datatug/datatug-core/pkg/datatug"
)

type ProjectStoreRef interface {
	Project() ProjectStore
}

type ProjectStore interface {
	ProjectID() string
	Folders() FoldersStore
	Queries() QueriesStore
	Boards() BoardsStore

	Environments() EnvironmentsStore
	Entities() EntitiesStore
	DbModels() DbModelsStore
	DbServers() DbServersStore
	Recordsets() RecordsetsStore

	SaveProject(ctx context.Context, project datatug.Project) (err error)
	// LoadProject returns full DataTug project
	LoadProject(ctx context.Context) (*datatug.Project, error)

	// LoadProjectSummary return summary metadata about DataTug project
	LoadProjectSummary(ctx context.Context) (datatug.ProjectSummary, error)
}
