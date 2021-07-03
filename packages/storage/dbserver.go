package storage

import "github.com/datatug/datatug/packages/models"

type DbServersStore interface {
	ProjectStoreRef
	DbServer(id models.ServerReference) DbServerStore
}

type DbServerStore interface {
	ID() models.ServerReference
	Catalogs() DbCatalogsStore

	// LoadDbServerSummary loads summary on DB server
	LoadDbServerSummary(dbServer models.ServerReference) (summary *models.ProjDbServerSummary, err error)
	SaveDbServer(dbServer models.ProjDbServer, project models.DatatugProject) (err error)
	DeleteDbServer(dbServer models.ServerReference) (err error)
}
