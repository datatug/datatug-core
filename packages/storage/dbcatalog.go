package storage

import "github.com/datatug/datatug/packages/models"

type DbCatalogStore interface {
	Loader() DbCatalogLoader
	Saver() DbCatalogSaver
}

// DbCatalogLoader loads db catalogs
type DbCatalogLoader interface {
	LoadDbCatalogSummary(catalogID string) (catalog *models.DbCatalogSummary, err error)
}

type DbCatalogSaver interface {
}
