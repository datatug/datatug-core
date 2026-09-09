package filestore

import (
	"context"
	"fmt"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

var _ datatug.DbModelsStore = (*fsDbModelsStore)(nil)

func newFsDbModelsStore(projectPath string) fsDbModelsStore {
	return fsDbModelsStore{
		fsProjectItemsStore: newFileProjectItemsStore[datatug.DbModels, *datatug.DbModel, datatug.DbModel](
			path.Join(projectPath, storage.DbModelsFolder), storage.DbModelFileSuffix,
		),
	}
}

// fsDbModelsStore loads and saves DB models from two on-disk layouts, the
// same rules store_entities.go applies to entities: flat
// "<dbModelsDir>/<id>.dbmodel.json" and nested
// "<dbModelsDir>/<id>/<id>.dbmodel.json" (what datatug-demo-projects uses,
// e.g. dbmodels/chinook/chinook.dbmodel.json). Nested wins when both exist
// and agree; disagreement is a load error. A save keeps the layout a model
// was loaded from; a brand-new model defaults to nested.
type fsDbModelsStore struct {
	fsProjectItemsStore[datatug.DbModels, *datatug.DbModel, datatug.DbModel]
}

func (s fsDbModelsStore) flatDbModelFilePath(id string) string {
	return path.Join(s.dirPath, storage.JsonFileName(id, s.itemFileSuffix))
}

func (s fsDbModelsStore) nestedDbModelFilePath(id string) string {
	return path.Join(s.dirPath, id, storage.JsonFileName(id, s.itemFileSuffix))
}

func readDbModelFile(filePath string) (*datatug.DbModel, error) {
	dbModel := new(datatug.DbModel)
	if err := readJSONFile(filePath, true, dbModel); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return dbModel, nil
}

// loadOneDbModel reads id from both layouts and reconciles them: nested wins
// when both exist and their content matches; a content mismatch is a clear
// error. Returns (nil, nil) when the model exists in neither layout.
func (s fsDbModelsStore) loadOneDbModel(id string) (*datatug.DbModel, error) {
	nestedPath, flatPath := s.nestedDbModelFilePath(id), s.flatDbModelFilePath(id)
	nested, err := readDbModelFile(nestedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load db model[%s] from %s: %w", id, nestedPath, err)
	}
	flat, err := readDbModelFile(flatPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load db model[%s] from %s: %w", id, flatPath, err)
	}
	switch {
	case nested != nil && flat != nil:
		if !reflect.DeepEqual(nested, flat) {
			return nil, fmt.Errorf(
				"db model[%s] is stored in both %s and %s with different content; remove one",
				id, nestedPath, flatPath)
		}
		nested.SetID(id)
		return nested, nil
	case nested != nil:
		nested.SetID(id)
		return nested, nil
	case flat != nil:
		flat.SetID(id)
		return flat, nil
	default:
		return nil, nil
	}
}

func (s fsDbModelsStore) LoadDbModel(_ context.Context, id string, o ...datatug.StoreOption) (*datatug.DbModel, error) {
	_ = datatug.GetStoreOptions(o...)
	dbModel, err := s.loadOneDbModel(id)
	if err != nil {
		return nil, err
	}
	if dbModel == nil {
		return nil, fmt.Errorf("failed to load db model[%s] from project: %w", id, os.ErrNotExist)
	}
	return dbModel, nil
}

// listDbModelIDs returns every db model id found in either layout: flat
// "<dbModelsDir>/<id>.dbmodel.json" files and nested
// "<dbModelsDir>/<id>/<id>.dbmodel.json" directories.
func (s fsDbModelsStore) listDbModelIDs() ([]string, error) {
	dirEntries, err := os.ReadDir(s.dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	suffix := "." + s.itemFileSuffix + ".json"
	seen := make(map[string]struct{}, len(dirEntries))
	var ids []string
	add := func(id string) {
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, de := range dirEntries {
		name := de.Name()
		if de.IsDir() {
			if _, statErr := os.Stat(s.nestedDbModelFilePath(name)); statErr == nil {
				add(name)
			}
			continue
		}
		if id, ok := strings.CutSuffix(name, suffix); ok {
			add(id)
		}
	}
	return ids, nil
}

func (s fsDbModelsStore) LoadDbModels(_ context.Context, o ...datatug.StoreOption) (datatug.DbModels, error) {
	_ = datatug.GetStoreOptions(o...)
	ids, err := s.listDbModelIDs()
	if err != nil {
		return nil, err
	}
	dbModels := make(datatug.DbModels, 0, len(ids))
	for _, id := range ids {
		dbModel, err := s.loadOneDbModel(id)
		if err != nil {
			return nil, err
		}
		if dbModel == nil {
			continue // named by the directory scan but vanished/unreadable in a benign race
		}
		dbModels = append(dbModels, dbModel)
	}
	sort.Slice(dbModels, func(i, j int) bool {
		return dbModels[i].GetID() < dbModels[j].GetID()
	})
	return dbModels, nil
}

// saveTarget mirrors fsEntitiesStore.saveTarget: nested if a nested file
// already exists (nested wins, same as loadOneDbModel), else flat if a flat
// file already exists (preserve the layout it was loaded from), else nested
// as the default for a brand-new model.
func (s fsDbModelsStore) saveTarget(id string) (dirPath, fileName string) {
	fileName = storage.JsonFileName(id, s.itemFileSuffix)
	if _, err := os.Stat(s.nestedDbModelFilePath(id)); err == nil {
		return path.Join(s.dirPath, id), fileName
	}
	if _, err := os.Stat(s.flatDbModelFilePath(id)); err == nil {
		return s.dirPath, fileName
	}
	return path.Join(s.dirPath, id), fileName
}

func (s fsDbModelsStore) SaveDbModel(_ context.Context, dbModel *datatug.DbModel) error {
	dirPath, fileName := s.saveTarget(dbModel.ID)
	if err := saveJSONFile(dirPath, fileName, dbModel); err != nil {
		return fmt.Errorf("failed to save db model file: %w", err)
	}
	return nil
}

func (s fsDbModelsStore) SaveDbModels(ctx context.Context, dbModels datatug.DbModels) error {
	return saveItems(s.dirPath, len(dbModels), func(i int) func() error {
		return func() error {
			return s.SaveDbModel(ctx, dbModels[i])
		}
	})
}

func (s fsDbModelsStore) DeleteDbModel(_ context.Context, id string) error {
	for _, filePath := range []string{s.nestedDbModelFilePath(id), s.flatDbModelFilePath(id)} {
		if _, err := os.Stat(filePath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := os.Remove(filePath); err != nil {
			return err
		}
	}
	return nil
}
