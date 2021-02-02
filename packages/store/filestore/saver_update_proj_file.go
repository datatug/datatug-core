package filestore

import (
	"github.com/datatug/datatug/packages/models"
)

func (s FileSystemSaver) loadProjectFile() (v models.ProjectFile, err error) {
	return LoadProjectFile(s.path)
}

func (s FileSystemSaver) updateProjectFileWithBoard(board models.Board) (err error) {
	projFile, err := s.loadProjectFile()
	if err != nil {
		return err
	}
	for _, b := range projFile.Boards {
		if b.ID == board.ID {
			if b.Title == board.Title {
				goto SAVED
			}
			b.Title = board.Title
			goto UPDATED
		}
	}
	projFile.Boards = append(projFile.Boards, &models.ProjBoardBrief{
		ProjectItem: models.ProjectItem{ID: board.ID, Title: board.Title},
		Parameters:  board.Parameters,
	})
UPDATED:
	err = s.putProjectFile(projFile)
SAVED:
	return err
}

func (s FileSystemSaver) updateProjectFileWithEntity(entity models.Entity) (err error) {
	projFile, err := s.loadProjectFile()
	if err != nil {
		return err
	}
	for _, item := range projFile.Entities {
		if item.ID == entity.ID {
			if item.Title == entity.Title {
				goto SAVED
			}
			item.Title = entity.Title
			goto UPDATED
		}
	}
	projFile.Entities = append(projFile.Entities, &models.ProjEntityBrief{
		ProjectItem: models.ProjectItem{ID: entity.ID, Title: entity.Title},
	})
UPDATED:
	err = s.putProjectFile(projFile)
SAVED:
	return err
}

/*
func (s FileSystemSaver) updateProjectFileDeleteEntity(id string) (err error) {
	// Almost duplicates updateProjectFileDeleteBoard and other deletes
	projFile, err := s.loadProjectFile()
	if err != nil {
		return err
	}
	if len(projFile.Entities) == 0 {
		return
	}
	entities := make([]*models.ProjEntityBrief, len(projFile.Entities))
	for _, item := range projFile.Entities {
		if item.ID == id {
			continue
		}
	}
	if len(projFile.Entities) > len(entities) {
		projFile.Entities = entities
		err = s.putProjectFile(projFile)
	}
	return err
}

func (s FileSystemSaver) updateProjectFileDeleteBoard(id string) (err error) {
	// Almost duplicates updateProjectFileDeleteEntity and other deletes
	projFile, err := s.loadProjectFile()
	if err != nil {
		return err
	}
	if len(projFile.Entities) == 0 {
		return
	}
	items := make([]*models.ProjBoardBrief, len(projFile.Boards))
	for _, item := range projFile.Boards {
		if item.ID == id {
			continue
		}
	}
	if len(projFile.Boards) > len(items) {
		projFile.Boards = items
		err = s.putProjectFile(projFile)
	}
	return err
}
*/
