package storage

import "github.com/datatug/datatug/packages/models"

type EnvDbCatalogsStore interface {
	Catalog(id string) EnvDbCatalogStore
}

type EnvDbCatalogStore interface {
	Catalogs() EnvDbCatalogsStore
	// LoadEnvironmentCatalog returns DB info for a specific environment
	LoadEnvironmentCatalog() (*models.EnvDb, error)
	//LoadDbCatalog(id string) (*models.EnvDbServer, error)
	//SaveDbCatalog(id string, envServer *models.EnvDbServer) error
}
