package filestore

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/models2md"
	"github.com/datatug/datatug/packages/store"
	"github.com/strongo/validation"
	"log"
)

type storeSaver struct {
	pathByID map[string]string
}

var _ store.Saver = (*storeSaver)(nil)

func (saver storeSaver) newProjectSaver(projID string) (projSaver fileSystemSaver, err error) {
	projPath, knownProjID := saver.pathByID[projID]
	if !knownProjID {
		err = validation.NewErrBadRequestFieldValue("projectID", "unknown project ID")
		return
	}
	log.Println("Saving to: ", projPath)
	projSaver = newSaver(projPath, models2md.NewEncoder())
	return
}
func (saver storeSaver) Save(project models.DataTugProject) (err error) {
	projSaver, err := saver.newProjectSaver(project.ID)
	if err != nil {
		return err
	}
	return projSaver.Save(project)
}

func (saver storeSaver) SaveDbServer(projID string, dbServer *models.ProjDbServer) (err error) {
	projSaver, err := saver.newProjectSaver(projID)
	if err != nil {
		return err
	}
	return projSaver.SaveDbServer(dbServer)
}

func (saver storeSaver) DeleteDbServer(projID string, dbServer models.DbServer) (err error) {
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
