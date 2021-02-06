package store

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/server/dto"
)

// Loader defines methods for projects loader
type Loader interface {

	// LoadProject returns full DataTug project
	LoadProject(projectID string) (*models.DataTugProject, error)

	// LoadProjectSummary return summary metadata about DataTug project
	LoadProjectSummary(projectID string) (models.ProjectSummary, error)

	// LoadEnvironmentSummary return summary metadata about environment
	LoadEnvironmentSummary(projectID, environmentID string) (dto.EnvironmentSummary, error)

	// GetEnvironmentDbSummary returns summary of environment
	LoadEnvironmentDbSummary(projectID, environmentID, databaseID string) (dto.DatabaseSummary, error)
	// GetEnvironmentDbSummary returns DB info for a specific environment
	LoadEnvironmentDb(projID, environmentID, databaseID string) (*dto.EnvDb, error)

	// LoadBoard loads board
	LoadBoard(projectID, boardID string) (board models.Board, err error)
	// LoadEntity loads entity
	LoadEntity(projectID, entityID string) (entity models.Entity, err error)
	// LoadEntities loads entities
	LoadEntities(projectID string) (entities []models.Entity, err error)

	// LoadRecordsetDefinitions loads list of recordsets summary
	LoadRecordsetDefinitions(projectID string) (datasets []*models.RecordsetDefinition, err error)
	// LoadRecordsetDefinition loads recordset definition
	LoadRecordsetDefinition(projectID, datasetName string) (dataset *models.RecordsetDefinition, err error)
	// LoadRecordsetData loads recordset data
	LoadRecordsetData(projectID, datasetName, fileName string) (recordset *models.Recordset, err error)

	// LoadDbServerSummary loads summary on DB server
	LoadDbServerSummary(projectID string, dbServer models.DbServer) (summary *dto.ProjDbServerSummary, err error)
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
