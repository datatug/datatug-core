package store

import (
	"github.com/datatug/datatug/packages/models"
)

// Loader defines methods for projects loader
type Loader interface {
	BoardsLoader
	DbServerLoader
	DbCatalogLoader
	EntitiesLoader
	EnvironmentsLoader
	QueriesLoader
	ProjectLoader
	RecordsetsLoader
}

// ProjectLoader loads projects
type ProjectLoader interface {
	// LoadProject returns full DataTug project
	LoadProject(projectID string) (*models.DatatugProject, error)

	// LoadProjectSummary return summary metadata about DataTug project
	LoadProjectSummary(projectID string) (models.ProjectSummary, error)
}

// EnvironmentsLoader loads environments
type EnvironmentsLoader interface {
	// LoadEnvironmentSummary return summary metadata about environment
	LoadEnvironmentSummary(projectID, environmentID string) (models.EnvironmentSummary, error)

	// GetEnvironmentDbSummary returns summary of environment
	LoadEnvironmentDbSummary(projectID, environmentID, databaseID string) (models.DbCatalogSummary, error)
	// GetEnvironmentDbSummary returns DB info for a specific environment
	LoadEnvironmentCatalog(projID, environmentID, databaseID string) (*models.EnvDb, error)
}

// BoardsLoader loads boards
type BoardsLoader interface {
	// LoadBoard loads board
	LoadBoard(projectID, boardID string) (board models.Board, err error)
}

// EntitiesLoader loads entities
type EntitiesLoader interface {
	// LoadEntity loads entity
	LoadEntity(projectID, entityID string) (entity models.Entity, err error)
	// LoadEntities loads entities
	LoadEntities(projectID string) (entities models.Entities, err error)
}

// RecordsetsLoader loads recordsets
type RecordsetsLoader interface {
	// LoadRecordsetDefinitions loads list of recordsets summary
	LoadRecordsetDefinitions(projectID string) (datasets []*models.RecordsetDefinition, err error)
	// LoadRecordsetDefinition loads recordset definition
	LoadRecordsetDefinition(projectID, datasetName string) (dataset *models.RecordsetDefinition, err error)
	// LoadRecordsetData loads recordset data
	LoadRecordsetData(projectID, datasetName, fileName string) (recordset *models.Recordset, err error)
}

// DbServerLoader loads db servers
type DbServerLoader interface {
	// LoadDbServerSummary loads summary on DB server
	LoadDbServerSummary(projectID string, dbServer models.ServerReference) (summary *models.ProjDbServerSummary, err error)
}

// DbCatalogLoader loads db catalogs
type DbCatalogLoader interface {
	LoadDbCatalogSummary(projectID string, dbServer models.ServerReference, catalogID string) (catalog *models.DbCatalogSummary, err error)
}

// QueriesLoader loads queries
type QueriesLoader interface {
	// LoadQueries loads tree of queries
	LoadQueries(projectID, folderPath string) (folder *models.QueryFolder, err error)

	//
	LoadQuery(projectID, queryID string) (query *models.QueryDef, err error)
}

var _ QueriesLoader = (*NotSupportedQueriesLoader)(nil)
