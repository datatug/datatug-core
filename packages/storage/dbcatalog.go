package storage

import "github.com/datatug/datatug/packages/models"

type DbCatalogsStore interface {
	Server() DbServerStore
	DbCatalog(id string) DbCatalogStore
}

type DbCatalogStore interface {
	Server() DbServerStore
	LoadDbCatalogSummary() (catalog *models.DbCatalogSummary, err error)
}
