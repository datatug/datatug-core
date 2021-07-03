package storage

import (
	"context"
	"github.com/datatug/datatug/packages/models"
)

type DbServersStore interface {
	ProjectStoreRef
	DbServer(id models.ServerReference) DbServerStore
}

type DbServerStore interface {
	ID() models.ServerReference
	Catalogs() DbCatalogsStore

	// LoadDbServerSummary loads summary on DB server
	LoadDbServerSummary(ctx context.Context, dbServer models.ServerReference) (summary *models.ProjDbServerSummary, err error)
	SaveDbServer(ctx context.Context, dbServer models.ProjDbServer, project models.DatatugProject) (err error)
	DeleteDbServer(ctx context.Context, dbServer models.ServerReference) (err error)
}
