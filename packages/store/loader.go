package store

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/server/dto"
)

// Loader defines methods for projects loader
type Loader interface {
	boardsLoader
	dbServerLoader
	entitiesLoader
	environmentsLoader
	queriesLoader
	projectLoader
	recordsetsLoader
}

type projectLoader interface {
	// LoadProject returns full DataTug project
	LoadProject(projectID string) (*models.DataTugProject, error)

	// LoadProjectSummary return summary metadata about DataTug project
	LoadProjectSummary(projectID string) (models.ProjectSummary, error)
}

type environmentsLoader interface {
	// LoadEnvironmentSummary return summary metadata about environment
	LoadEnvironmentSummary(projectID, environmentID string) (dto.EnvironmentSummary, error)

	// GetEnvironmentDbSummary returns summary of environment
	LoadEnvironmentDbSummary(projectID, environmentID, databaseID string) (dto.DatabaseSummary, error)
	// GetEnvironmentDbSummary returns DB info for a specific environment
	LoadEnvironmentDb(projID, environmentID, databaseID string) (*dto.EnvDb, error)
}

type boardsLoader interface {
	// LoadBoard loads board
	LoadBoard(projectID, boardID string) (board models.Board, err error)
}

type entitiesLoader interface {
	// LoadEntity loads entity
	LoadEntity(projectID, entityID string) (entity models.Entity, err error)
	// LoadEntities loads entities
	LoadEntities(projectID string) (entities models.Entities, err error)
}

type recordsetsLoader interface {
	// LoadRecordsetDefinitions loads list of recordsets summary
	LoadRecordsetDefinitions(projectID string) (datasets []*models.RecordsetDefinition, err error)
	// LoadRecordsetDefinition loads recordset definition
	LoadRecordsetDefinition(projectID, datasetName string) (dataset *models.RecordsetDefinition, err error)
	// LoadRecordsetData loads recordset data
	LoadRecordsetData(projectID, datasetName, fileName string) (recordset *models.Recordset, err error)
}

type dbServerLoader interface {
	// LoadDbServerSummary loads summary on DB server
	LoadDbServerSummary(projectID string, dbServer models.DbServer) (summary *dto.ProjDbServerSummary, err error)
}

type queriesLoader interface {
	// LoadQueries loads tree of queries
	LoadQueries(projectID, folder string) (datasets []models.QueryDef, err error)

	//
	LoadQuery(projectID, queryID string) (query models.QueryDef, err error)
}
