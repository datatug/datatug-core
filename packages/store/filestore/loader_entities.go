package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"os"
	"path"
	"strings"
	"sync"
)

func (loader fileSystemLoader) LoadEntity(projID, entityID string) (entity models.Entity, err error) {
	var projPath string
	if projID, projPath, err = loader.GetProjectPath(projID); err != nil {
		return
	}
	fileName := path.Join(projPath, DatatugFolder, EntitiesFolder, jsonFileName(entityID, entityFileSuffix))
	if err = readJSONFile(fileName, true, &entity); err != nil {
		err = fmt.Errorf("faile to load entity [%v] from project [%v]: %w", entityID, projID, err)
		return
	}
	return
}

func loadEntities(projPath string, project *models.DataTugProject) error {
	entitiesDirPath := path.Join(projPath, DatatugFolder, "entities")
	if err := loadDir(nil, entitiesDirPath, processDirs,
		func(files []os.FileInfo) {
			project.Entities = make(models.Entities, 0, len(files))
		},
		func(f os.FileInfo, i int, mutex *sync.Mutex) error {
			if f.IsDir() {
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
			project.Entities = append(project.Entities, entity)
			mutex.Unlock()
			return nil
		}); err != nil {
		return fmt.Errorf("failed to load entities: %w", err)
	}
	return nil
}

func isEntityFile(fileName string) bool {
	return strings.HasSuffix(fileName, entityFileSuffix+".json")
}

func (loader fileSystemLoader) LoadEntities(projID string) (entities models.Entities, err error) {
	var projPath string
	if projID, projPath, err = loader.GetProjectPath(projID); err != nil {
		return
	}
	project := new(models.DataTugProject)
	if err = loadEntities(projPath, project); err != nil {
		return nil, err
	}
	return project.Entities, nil
}
