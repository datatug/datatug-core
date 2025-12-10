package storage

import "github.com/datatug/datatug-core/pkg/datatug"

type EnvDbCatalogsStore interface {
	Catalog(id string) EnvDbCatalogStore
}

type EnvDbCatalogStore interface {
	Catalogs() EnvDbCatalogsStore
	// LoadEnvironmentCatalog returns DB info for a specific environment
	LoadEnvironmentCatalog() (*datatug.EnvDb, error)
	//LoadDbCatalog(id string) (*models.EnvDbServer, error)
	//SaveDbCatalog(id string, envServer *models.EnvDbServer) error
}
