package storage

import "github.com/datatug/datatug/packages/models"

type DbServerStore interface {
	Loader() DbServerLoader
	Saver() DbServerSaver
	Catalogs() DbCatalogLoader
	DbServer() models.ServerReference
}

// DbServerLoader loads db servers
type DbServerLoader interface {
	// LoadDbServerSummary loads summary on DB server
	LoadDbServerSummary(dbServer models.ServerReference) (summary *models.ProjDbServerSummary, err error)
}

// DbServerSaver saves db servers
type DbServerSaver interface {
	SaveDbServer(dbServer models.ProjDbServer, project models.DatatugProject) (err error)
	DeleteDbServer(dbServer models.ServerReference) (err error)
}
