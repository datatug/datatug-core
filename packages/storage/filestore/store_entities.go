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

type fsEntitiesStore struct {
	fsProjectStoreRef
	entitiesDirPath string
}

func (store fsEntitiesStore) Entity(id string) storage.EntityStore {
	return store.entity(id)
}

func (store fsEntitiesStore) entity(id string) fsEntityStore {
	return newFsEntityStore(id, store)
}

func newFsEntitiesStore(fsProjectStore fsProjectStore) fsEntitiesStore {
	return fsEntitiesStore{
		fsProjectStoreRef: fsProjectStoreRef{fsProjectStore},
		entitiesDirPath:   path.Join(fsProjectStore.projectPath, EntitiesFolder),
	}
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
