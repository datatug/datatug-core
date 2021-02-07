package store

import "github.com/datatug/datatug/packages/models"

type querySaver interface {
	DeleteQuery(projID, queryID string) (err error)
	SaveQuery(projID string, query models.Query) (err error)
}

type dbServerSaver interface {
	SaveDbServer(projID string, dbServer *models.ProjDbServer) (err error)
	DeleteDbServer(projID string, dbServer models.DbServer) (err error)
}

type boardsSaver interface {
	DeleteBoard(projID, boardID string) (err error)
	SaveBoard(projID string, board models.Board) (err error)
}

type entitySaver interface {
	DeleteEntity(projID, entityID string) (err error)
	SaveEntity(projID string, entity *models.Entity) (err error)
}

// Saver defines interface for saving DataTug project
type Saver interface {
	querySaver
	dbServerSaver
	boardsSaver
	entitySaver
	Save(project models.DataTugProject) (err error)
}
