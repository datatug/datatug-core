package storage

import "github.com/datatug/datatug-core/pkg/datatug"

type EnvironmentsStore interface {
	ProjectStoreRef
	Environment(id string) EnvironmentStore
}

// EnvironmentStore provides access to environment record
type EnvironmentStore interface {
	// ID returns environment ID
	ID() string

	Project() ProjectStore

	Servers() EnvServersStore
	//

	// LoadEnvironmentSummary return summary metadata about environment
	LoadEnvironmentSummary() (*datatug.EnvironmentSummary, error)

	DeleteEnvironment() (err error)
	SaveEnvironment(environment *datatug.Environment) (err error)

	// LoadEnvironmentDbSummary returns summary of environment
	// LoadEnvironmentDbSummary(environmentID, databaseID string) (*models.DbCatalogSummary, error)
}

// EnvironmentsLoader loads environments
type EnvironmentsLoader interface {
}

type EnvironmentsSaver interface {
}
