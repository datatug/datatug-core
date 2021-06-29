package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"os"
	"path"
	"sync"
)

var _ storage.EntitiesStore = (*fsEntitiesStore)(nil)
var _ storage.EntitiesSaver = (*fsEntitiesStore)(nil)
var _ storage.EntitiesLoader = (*fsEntitiesStore)(nil)

type fsEntitiesStore struct {
	fsProjectStore
	entitiesDirPath string
}

func (store fsEntitiesStore) DeleteEntity(id string) (err error) {
	panic("implement me")
}

func (store fsEntitiesStore) SaveEntity(entity *models.Entity) (err error) {
	panic("implement me")
}

func (store fsEntitiesStore) Loader() storage.EntitiesLoader {
	return store
}

func (store fsEntitiesStore) Saver() storage.EntitiesSaver {
	return store
}

func newFsEntitiesStore(fsProjectStore fsProjectStore) fsEntitiesStore {
	return fsEntitiesStore{
		fsProjectStore:  fsProjectStore,
		entitiesDirPath: path.Join(fsProjectStore.projectPath, EntitiesFolder),
	}
}

func (store fsEntitiesStore) LoadEntity(entityID string) (entity models.Entity, err error) {
	fileName := path.Join(store.entitiesDirPath, entityID, jsonFileName(entityID, entityFileSuffix))
	if err = readJSONFile(fileName, true, &entity); err != nil {
		err = fmt.Errorf("faile to load entity [%v] from project [%v]: %w", entityID, store.projectID, err)
		return
	}
	return
}

func loadEntities(projPath string) (entities models.Entities, err error) {
	entitiesDirPath := path.Join(projPath, DatatugFolder, "entities")
	if err = loadDir(nil, entitiesDirPath, processDirs,
		func(files []os.FileInfo) {
			entities = make(models.Entities, 0, len(files))
		},
		func(f os.FileInfo, i int, mutex *sync.Mutex) error {
			if !f.IsDir() {
				return nil
			}
			entityID := f.Name()
			entity := new(models.Entity)
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

func (store fsEntitiesStore) LoadEntities() (entities models.Entities, err error) {
	return loadEntities(store.projectPath)
}
