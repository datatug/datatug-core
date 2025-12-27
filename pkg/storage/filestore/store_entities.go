package filestore

import (
	"context"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/strongo/validation"
)

type fsEntitiesStore struct {
	fsProjectItemsStore[datatug.Entities, *datatug.Entity, datatug.Entity]
}

func (s fsEntitiesStore) loadEntity(ctx context.Context, id string, o ...datatug.StoreOption) (*datatug.Entity, error) {
	return s.loadProjectItem(ctx, id, s.itemFileName(id), o...)
}

func (s fsEntitiesStore) loadEntities(ctx context.Context, o ...datatug.StoreOption) (datatug.Entities, error) {
	return s.loadProjectItems(ctx, o...)
}

func (s fsEntitiesStore) deleteEntity(ctx context.Context, id string) error {
	return s.deleteProjectItem(ctx, id)
}

func (s fsEntitiesStore) saveEntities(ctx context.Context, entities datatug.Entities) (err error) {
	return s.saveProjectItems(ctx, EntitiesFolder, entities)
}

func (s fsEntitiesStore) saveEntity(ctx context.Context, entity *datatug.Entity) (err error) {
	if entity == nil {
		return validation.NewErrRequestIsMissingRequiredField("entity")
	}
	if entity.ID == "" {
		return validation.NewErrBadRequestFieldValue("entity", validation.NewErrRecordIsMissingRequiredField("ID").Error())
	}
	/*
		updateProjFileWithEntity := func(projFile *datatug.ProjectFile) error {
			for _, item := range projFile.Entities {
				if item.ID == entity.ID {
					if item.Title == entity.Title {
						return nil
					}
					item.Title = entity.Title
					break
				}
			}
			projFile.Entities = append(projFile.Entities, &datatug.ProjEntityBrief{
				ProjItemBrief: datatug.ProjItemBrief{ID: entity.ID, Title: entity.Title},
			})
			return nil
		}
		err = s.updateProjectFile(updateProjFileWithEntity)
		if err != nil {
			return fmt.Errorf("failed to update project file with entity: %w", err)
		}
	*/
	if len(entity.Fields) == 0 && entity.Fields != nil {
		entity.Fields = nil
	}
	return s.saveProjectItem(ctx, entity)
}
