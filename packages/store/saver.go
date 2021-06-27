package store

import (
	"github.com/datatug/datatug/packages/models"
)

type QuerySaver interface {
	DeleteQuery(projID, queryID string) (err error)
	DeleteQueryFolder(projID, path string) (err error)
	CreateQueryFolder(projID, path, id string) (folder models.QueryFolder, err error)
	CreateQuery(projID string, query models.QueryDef) (err error)
	UpdateQuery(projID string, query models.QueryDef) (err error)
}

var _ QuerySaver = (*NotSupportedQuerySaver)(nil)

type DbServerSaver interface {
	SaveDbServer(projID string, dbServer models.ProjDbServer, project models.DatatugProject) (err error)
	DeleteDbServer(projID string, dbServer models.ServerReference) (err error)
}

type BoardsSaver interface {
	DeleteBoard(projID, boardID string) (err error)
	SaveBoard(projID string, board models.Board) (err error)
}

type EntitySaver interface {
	DeleteEntity(projID, entityID string) (err error)
	SaveEntity(projID string, entity *models.Entity) (err error)
}

// Saver defines interface for saving DataTug project
type Saver interface {
	QuerySaver
	DbServerSaver
	BoardsSaver
	EntitySaver
	Save(project models.DatatugProject) (err error)
}
