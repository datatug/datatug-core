package storage

import (
	"context"

	"github.com/datatug/datatug-core/pkg/datatug"
)

type DbServersStore interface {
	ProjectStoreRef
	DbServer(id datatug.ServerRef) DbServerStore
}

type DbServerStore interface {
	ID() datatug.ServerRef

	// LoadDbServerSummary loads summary on DB server
	LoadDbServerSummary(ctx context.Context, dbServer datatug.ServerRef) (summary *datatug.ProjDbServerSummary, err error)
	SaveDbServer(ctx context.Context, dbServer datatug.ProjDbServer, project datatug.Project) (err error)
	DeleteDbServer(ctx context.Context, dbServer datatug.ServerRef) (err error)
}
