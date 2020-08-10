package store

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/server/dto"
)

// Loader defines methods for projects loader
type Loader interface {

	// GetProject returns full DataTug project
	GetProject(projectID string) (*models.DataTugProject, error)

	// GetProjectSummary return summary metadata about DataTug project
	GetProjectSummary(projectID string) (models.ProjectSummary, error)

	// GetProjectSummary return summary metadata about environment
	GetEnvironmentSummary(projectID, environmentID string) (dto.EnvironmentSummary, error)

	GetEnvironmentDbSummary(projectID, environmentID, databaseID string) (dto.DatabaseSummary, error)
	GetEnvironmentDb(projID, environmentID, databaseID string) (*dto.EnvDb, error)

	LoadBoard(projectID, boardID string) (board models.Board, err error)
	LoadEntity(projectID, entityID string) (entity models.Entity, err error)
	LoadEntities(projectID string) (entities []models.Entity, err error)

	GetDbServerSummary(projectID string, dbServer models.DbServer) (summary *dto.ProjDbServerSummary, err error)
}

type dbServerSaver interface {
	SaveDbServer(projID string, dbServer *models.ProjDbServer) (err error)
	DeleteDbServer(projID string, dbServer models.DbServer) (err error)
}

type boardsSaver interface {
	DeleteBoard(projID, boardID string) (err error)
	SaveBoard(projID string, board models.Board) (err error)
}

type entitySaver interface {
	DeleteEntity(projID, entityID string) (err error)
	SaveEntity(projID string, entity *models.Entity) (err error)
}

// Saver defines interface for saving DataTug project
type Saver interface {
	dbServerSaver
	boardsSaver
	entitySaver
	Save(project models.DataTugProject) (err error)
}

// Interface defines interface for loading & saving DataTug projects
type Interface interface {
	Loader
	Saver
	GetProjects() (projectBriefs []models.ProjectBrief, err error)
}
