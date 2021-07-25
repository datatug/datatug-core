package filestore

import (
	"context"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"path"
)

var _ storage.EntityStore = (*fsEntityStore)(nil)

type fsEntityStore struct {
	entityID string
	fsEntitiesStore
}

func (store fsEntityStore) ID() string {
	return store.ID()
}

func newFsEntityStore(id string, fsEntitiesStore fsEntitiesStore) fsEntityStore {
	return fsEntityStore{entityID: id, fsEntitiesStore: fsEntitiesStore}
}

func (store fsEntityStore) DeleteEntity(_ context.Context) (err error) {
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
	//		if entity.ID == entityID || slice.IndexOfString(entityIds, entity.ID) < 0 {
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

func (store fsEntityStore) LoadEntity(_ context.Context) (*models.Entity, error) {
	fileName := path.Join(store.entitiesDirPath, store.entityID, jsonFileName(store.entityID, entityFileSuffix))
	var entity models.Entity
	if err := readJSONFile(fileName, true, &entity); err != nil {
		err = fmt.Errorf("faile to load entity [%v] from project [%v]: %w", store.entityID, store.projectID, err)
		return nil, err
	}
	return &entity, nil
}
