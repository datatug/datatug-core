package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"log"
)

func (s FileSystemSaver) saveEntities(entities models.Entities) (err error) {
	return s.saveItems(EntitiesFolder, len(entities), func(i int) func() error {
		return func() error {
			return s.SaveEntity(entities[i])
		}
	})
}

// SaveEntity saves entity
func (s FileSystemSaver) SaveEntity(entity *models.Entity) (err error) {
	log.Printf("FileSystemSaver.SaveEntity: %+v", entity)
	if err = s.updateProjectFileWithEntity(*entity); err != nil {
		return fmt.Errorf("failed to update project file with entity: %w", err)
	}
	fileName := projItemFileName(entity.ID, EntityPrefix)
	if len(entity.Fields) == 0 && entity.Fields != nil {
		entity.Fields = nil
	}
	if err = s.saveJSONFile(
		s.entitiesDirPath(),
		fileName,
		entity,
	); err != nil {
		return fmt.Errorf("failed to save entity file: %w", err)
	}
	return err
}
