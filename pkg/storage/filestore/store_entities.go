package filestore

import (
	"context"
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/strongo/validation"
)

func (s fsProjectStore) deleteEntity(_ context.Context) (err error) {
	panic("not implemented")
	//deleteFile := func() (err error) {
	//	filePath := path.Join(s.entitiesDirPath(), jsonFileName(entityID, entityFileSuffix))
	//	if _, err := os.Stat(filePath); err != nil {
	//		if os.IsNotExist(err) {
	//			return nil
	//		}
	//		return err
	//	}
	//	return os.Remove(filePath)
	//}
	//deleteFromProjectSummary := func() error {
	//	projectSummary, err := s.loadProjectFile()
	//	if err != nil {
	//		return err
	//	}
	//
	//	var entityIds []string
	//	if err := loadDir(nil, s.entitiesDirPath(), processFiles, func(files []os.FileInfo) {
	//		entityIds = make([]string, 0, len(files))
	//	}, func(f os.FileInfo, i int, mutex *sync.Mutex) (err error) {
	//		fileName := f.Name()
	//		if strings.HasSuffix(fileName, entityFileSuffix+".json") {
	//			entityIds = append(entityIds, strings.Replace(fileName, entityFileSuffix+".json", "", 1))
	//		}
	//		return nil
	//	}); err != nil {
	//		return fmt.Errorf("failed to load names of entity files: %w", err)
	//	}
	//	shift := 0
	//	for i, entity := range projectSummary.Entities {
	//		if entity.ID == entityID || slice.Index(entityIds, entity.ID) < 0 {
	//			shift++
	//			continue
	//		}
	//		projectSummary.Entities[i-shift] = entity
	//	}
	//	projectSummary.Entities = projectSummary.Entities[0 : len(projectSummary.Entities)-shift]
	//	if err := s.putProjectFile(projectSummary); err != nil {
	//		return fmt.Errorf("failed to save project file: %w", err)
	//	}
	//	return nil
	//}
	//if err := deleteFile(); err != nil {
	//	return fmt.Errorf("failed to delete entity file: %w", err)
	//}
	//if err := deleteFromProjectSummary(); err != nil {
	//	fmt.Printf("Failed to remove entity record from project summary: %v\n", err) // TODO: Log as an error
	//}
	//return nil
}

func (s fsProjectStore) loadEntity(_ context.Context, id string, _ ...datatug.StoreOption) (*datatug.Entity, error) {
	entitiesDirPath := path.Join(s.projectPath, DatatugFolder, EntitiesFolder)
	fileName := path.Join(entitiesDirPath, id, jsonFileName(id, entityFileSuffix))
	var entity datatug.Entity
	if err := readJSONFile(fileName, true, &entity); err != nil {
		err = fmt.Errorf("failed to load entity [%v] from project [%v]: %w", id, s.projectID, err)
		return nil, err
	}
	return &entity, nil
}

func (s fsProjectStore) loadEntities(_ context.Context, _ ...datatug.StoreOption) (entities datatug.Entities, err error) {
	entitiesDirPath := path.Join(s.projectPath, DatatugFolder, EntitiesFolder)
	if err = loadDir(nil, entitiesDirPath, processDirs,
		func(files []os.FileInfo) {
			entities = make(datatug.Entities, 0, len(files))
		},
		func(f os.FileInfo, i int, mutex *sync.Mutex) error {
			if !f.IsDir() {
				return nil
			}
			entityID := f.Name()
			entity := new(datatug.Entity)
			entity.ID = entityID
			entityFileName := jsonFileName(entity.ID, entityFileSuffix)
			entityFilePath := path.Join(entitiesDirPath, entity.ID, entityFileName)
			if err := readJSONFile(entityFilePath, true, entity); err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return fmt.Errorf("failed to read JSON file for entity [%v]: %w", entityID, err)
			}
			if entity.ID != entityID {
				entity.ID = entityID
			}
			mutex.Lock()
			defer mutex.Unlock()
			entities = append(entities, entity)
			return nil
		}); err != nil {
		err = fmt.Errorf("failed to load entities: %w", err)
		return
	}
	return
}

func (s fsProjectStore) saveEntities(ctx context.Context, entities datatug.Entities) (err error) {
	return saveItems(EntitiesFolder, len(entities), func(i int) func() error {
		return func() error {
			return s.SaveEntity(ctx, entities[i])
		}
	})
}

// SaveEntity saves entity
func (s fsProjectStore) saveEntity(_ context.Context, entity *datatug.Entity) (err error) {
	if entity == nil {
		return validation.NewErrRequestIsMissingRequiredField("entity")
	}
	if entity.ID == "" {
		return validation.NewErrBadRequestFieldValue("entity", validation.NewErrRecordIsMissingRequiredField("ID").Error())
	}
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
	fileName := jsonFileName(entity.ID, entityFileSuffix)
	if len(entity.Fields) == 0 && entity.Fields != nil {
		entity.Fields = nil
	}
	entitiesDirPath := path.Join(s.projectPath, DatatugFolder, EntitiesFolder)
	dirPath := path.Join(entitiesDirPath, entity.ID)
	if err = saveJSONFile(
		dirPath,
		fileName,
		entity,
	); err != nil {
		return fmt.Errorf("failed to save entity file: %w", err)
	}
	return err
}
