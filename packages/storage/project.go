package storage

import "github.com/datatug/datatug/packages/models"

type ProjectStore interface {
	ProjectLoader
	ProjectSaver
	Environments() EnvironmentStore
	Queries() QueriesStore
	Boards() BoardsStore
	Entities() EntitiesStore
	DbServers() DbServerStore
	Recordsets() RecordsetsStore
}

// ProjectSaver defines interface for saving DataTug project
type ProjectSaver interface {
	Save(project models.DatatugProject) (err error)
}

// ProjectLoader loads projects
type ProjectLoader interface {
	// LoadProject returns full DataTug project
	LoadProject() (*models.DatatugProject, error)

	// LoadProjectSummary return summary metadata about DataTug project
	LoadProjectSummary() (models.ProjectSummary, error)
}
