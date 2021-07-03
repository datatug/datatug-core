package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/strongo/validation"
	"path"
)

func (store fsEntitiesStore) saveEntities(entities models.Entities) (err error) {
	return saveItems(EntitiesFolder, len(entities), func(i int) func() error {
		return func() error {
			return store.SaveEntity(entities[i])
		}
	})
}

// SaveEntity saves entity
func (store fsEntitiesStore) SaveEntity(entity *models.Entity) (err error) {
	if entity == nil {
		return validation.NewErrRequestIsMissingRequiredField("entity")
	}
	if entity.ID == "" {
		return validation.NewErrBadRequestFieldValue("entity", validation.NewErrRecordIsMissingRequiredField("ID").Error())
	}
	updateProjFileWithEntity := func(projFile *models.ProjectFile) error {
		for _, item := range projFile.Entities {
			if item.ID == entity.ID {
				if item.Title == entity.Title {
					return nil
				}
				item.Title = entity.Title
				break
			}
		}
		projFile.Entities = append(projFile.Entities, &models.ProjEntityBrief{
			ProjectItem: models.ProjectItem{ID: entity.ID, Title: entity.Title},
		})
		return nil
	}
	err = store.updateProjectFile(updateProjFileWithEntity)
	if err != nil {
		return fmt.Errorf("failed to update project file with entity: %w", err)
	}
	fileName := jsonFileName(entity.ID, entityFileSuffix)
	if len(entity.Fields) == 0 && entity.Fields != nil {
		entity.Fields = nil
	}
	dirPath := path.Join(store.entitiesDirPath, entity.ID)
	if err = saveJSONFile(
		dirPath,
		fileName,
		entity,
	); err != nil {
		return fmt.Errorf("failed to save entity file: %w", err)
	}
	return err
}
