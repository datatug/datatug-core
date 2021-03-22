package store

import (
	"github.com/datatug/datatug/packages/models"
)

// Loader defines methods for projects loader
type Loader interface {
	boardsLoader
	dbServerLoader
	dbCatalogLoader
	entitiesLoader
	environmentsLoader
	queriesLoader
	projectLoader
	recordsetsLoader
}

type projectLoader interface {
	// LoadProject returns full DataTug project
	LoadProject(projectID string) (*models.DatatugProject, error)

	// LoadProjectSummary return summary metadata about DataTug project
	LoadProjectSummary(projectID string) (models.ProjectSummary, error)
}

type environmentsLoader interface {
	// LoadEnvironmentSummary return summary metadata about environment
	LoadEnvironmentSummary(projectID, environmentID string) (models.EnvironmentSummary, error)

	// GetEnvironmentDbSummary returns summary of environment
	LoadEnvironmentDbSummary(projectID, environmentID, databaseID string) (models.DbCatalogSummary, error)
	// GetEnvironmentDbSummary returns DB info for a specific environment
	LoadEnvironmentCatalog(projID, environmentID, databaseID string) (*models.EnvDb, error)
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
	LoadDbServerSummary(projectID string, dbServer models.ServerReference) (summary *models.ProjDbServerSummary, err error)
}

type dbCatalogLoader interface {
	LoadDbCatalogSummary(projectID string, dbServer models.ServerReference, catalogID string) (catalog *models.DbCatalogSummary, err error)
}

type queriesLoader interface {
	// LoadQueries loads tree of queries
	LoadQueries(projectID, folder string) (datasets []models.QueryDef, err error)

	//
	LoadQuery(projectID, queryID string) (query models.QueryDef, err error)
}
