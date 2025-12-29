package filestore

import (
	"context"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/strongo/validation"
)

var _ datatug.EntitiesStore = (*fsEntitiesStore)(nil)

func newFsEntitiesStore(projectPath string) fsEntitiesStore {
	return fsEntitiesStore{
		fsProjectItemsStore: newFileProjectItemsStore[datatug.Entities, *datatug.Entity, datatug.Entity](
			path.Join(projectPath, EntitiesFolder), entityFileSuffix,
		),
	}
}

type fsEntitiesStore struct {
	fsProjectItemsStore[datatug.Entities, *datatug.Entity, datatug.Entity]
}

func (s fsEntitiesStore) LoadEntity(ctx context.Context, id string, o ...datatug.StoreOption) (*datatug.Entity, error) {
	return s.loadProjectItem(ctx, s.dirPath, id, "", o...)

}

func (s fsEntitiesStore) LoadEntities(ctx context.Context, o ...datatug.StoreOption) (datatug.Entities, error) {
	return s.loadProjectItems(ctx, s.dirPath, o...)
}

func (s fsEntitiesStore) DeleteEntity(ctx context.Context, id string) error {
	return s.deleteProjectItem(ctx, s.dirPath, id)
}

func (s fsEntitiesStore) SaveEntities(ctx context.Context, entities datatug.Entities) (err error) {
	return s.saveProjectItems(ctx, s.dirPath, entities)
}

func (s fsEntitiesStore) SaveEntity(ctx context.Context, entity *datatug.Entity) (err error) {
	if entity == nil {
		return validation.NewErrRequestIsMissingRequiredField("entity")
	}
	if entity.ID == "" {
		return validation.NewErrBadRequestFieldValue("entity", validation.NewErrRecordIsMissingRequiredField("GetID").Error())
	}
	/*
		updateProjFileWithEntity := func(projFile *datatug.ProjectFile) error {
			for _, item := range projFile.Entities {
				if item.GetID == entity.GetID {
					if item.Title == entity.Title {
						return nil
					}
					item.Title = entity.Title
					break
				}
			}
			projFile.Entities = append(projFile.Entities, &datatug.ProjEntityBrief{
				ProjItemBrief: datatug.ProjItemBrief{GetID: entity.GetID, Title: entity.Title},
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
	return s.saveProjectItem(ctx, s.dirPath, entity)
}
