package storage

import "github.com/datatug/datatug/packages/models"

type EnvironmentStore interface {
	Loader() EnvironmentsLoader
	Saver() EnvironmentsSaver
}

// EnvironmentsLoader loads environments
type EnvironmentsLoader interface {
	// LoadEnvironmentSummary return summary metadata about environment
	LoadEnvironmentSummary(environmentID string) (models.EnvironmentSummary, error)

	// GetEnvironmentDbSummary returns summary of environment
	LoadEnvironmentDbSummary(environmentID, databaseID string) (models.DbCatalogSummary, error)
	// GetEnvironmentDbSummary returns DB info for a specific environment
	LoadEnvironmentCatalog(projID, environmentID, databaseID string) (*models.EnvDb, error)
}

type EnvironmentsSaver interface {
	DeleteEnvironment(id string) (err error)
	SaveEnvironment(environment *models.Environment) (err error)
}
