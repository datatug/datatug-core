package filestore

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/models2md"
	"github.com/datatug/datatug/packages/storage"
	"github.com/strongo/validation"
	"log"
)

type storeSaver struct {
	pathByID map[string]string
}

var _ storage.Saver = (*storeSaver)(nil)

func (saver fsProjectStore) Save(project models.DatatugProject) (err error) {
	return projSaver.Save(project)
}

func (saver storeSaver) SaveDbServer(projID string, dbServer models.ProjDbServer, project models.DatatugProject) (err error) {
	projSaver, err := saver.newProjectSaver(projID)
	if err != nil {
		return err
	}
	return projSaver.SaveDbServer(dbServer, project)
}

func (saver storeSaver) DeleteDbServer(projID string, dbServer models.ServerReference) (err error) {
	projSaver, err := saver.newProjectSaver(projID)
	if err != nil {
		return err
	}
	return projSaver.DeleteDbServer(dbServer)
}

func (saver storeSaver) DeleteBoard(projID, boardID string) (err error) {
	projSaver, err := saver.newProjectSaver(projID)
	if err != nil {
		return err
	}
	return projSaver.DeleteBoard(boardID)
}

func (saver storeSaver) DeleteEntity(projID, entityID string) (err error) {
	projSaver, err := saver.newProjectSaver(projID)
	if err != nil {
		return err
	}
	return projSaver.DeleteEntity(entityID)
}

func (saver storeSaver) SaveBoard(projID string, board models.Board) (err error) {
	projSaver, err := saver.newProjectSaver(projID)
	if err != nil {
		return err
	}
	return projSaver.SaveBoard(board)
}

func (saver storeSaver) SaveEntity(projID string, entity *models.Entity) (err error) {
	log.Printf("storeSaver.SaveEntity: %+v", entity)
	projSaver, err := saver.newProjectSaver(projID)
	if err != nil {
		return err
	}
	return projSaver.SaveEntity(entity)
}

// DeleteQuery deletes query
func (saver storeSaver) DeleteQueryFolder(projID, path string) (err error) {
	projSaver, err := saver.newProjectSaver(projID)
	if err != nil {
		return err
	}
	return projSaver.DeleteQueryFolder(path)
}

// DeleteQuery deletes query
func (saver storeSaver) DeleteQuery(projID, queryID string) (err error) {
	projSaver, err := saver.newProjectSaver(projID)
	if err != nil {
		return err
	}
	return projSaver.DeleteQuery(queryID)
}

// CreateQuery creates a new query
func (saver storeSaver) CreateQuery(projID string, query models.QueryDef) (err error) {
	log.Printf("storeSaver.CreateQuery: %+v", query)
	projSaver, err := saver.newProjectSaver(projID)
	if err != nil {
		return err
	}
	return projSaver.CreateQuery(query)
}

// CreateQuery creates a new folder
func (saver storeSaver) CreateQueryFolder(projID, path, id string) (folder *models.QueryFolder, err error) {
	log.Printf("storeSaver.CreateQueryFolder(%v, %v, %v)", projID, path, id)
	projSaver, err := saver.newProjectSaver(projID)
	if err != nil {
		return folder, err
	}
	return projSaver.CreateQueryFolder(path, id)
}

// UpdateQuery updates an existing query
func (saver storeSaver) UpdateQuery(projID string, query models.QueryDef) (err error) {
	log.Printf("storeSaver.UpdateQuery: %+v", query)
	projSaver, err := saver.newProjectSaver(projID)
	if err != nil {
		return err
	}
	return projSaver.UpdateQuery(query)
}
