package filestore

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

var _ datatug.EnvDbCatalogStore = (*fsEnvDbCatalogStore)(nil)

func newFsEnvCatalogsStore(environmentsDirPath string) fsEnvDbCatalogStore {
	s := fsEnvDbCatalogStore{
		fsProjectItemsStore: newFileProjectItemsStore[datatug.DbCatalogs, *datatug.DbCatalog, datatug.DbCatalog](
			environmentsDirPath, storage.DbCatalogFileSuffix),
	}
	s.dirPath = environmentsDirPath
	return s
}

// fsEnvDbCatalogStore loads and saves environment DB catalogs from two
// on-disk layouts, the same rules store_entities.go applies to entities:
// flat "<catalogsDir>/<id>.db.json" and nested "<catalogsDir>/<id>/<id>.db.json"
// (what datatug-demo-projects uses, e.g.
// environments/local/catalogs/chinook-local/chinook-local.db.json). Nested
// wins when both exist and agree; disagreement is a load error. A save keeps
// the layout a catalog was loaded from; a brand-new catalog defaults to
// nested.
//
// serverID is accepted (it is part of the datatug.EnvDbCatalogStore
// interface) but not used to build the path: datatug-demo-projects' catalogs
// live directly under environments/<envID>/catalogs/, with no
// servers/<serverID> segment, and LoadEnvDbCatalogs (no serverID parameter)
// already used that same location - LoadEnvDbCatalog previously used a
// different, servers/<serverID>-qualified path that could never agree with
// it (the existing test's own comments flagged this before this fix).
type fsEnvDbCatalogStore struct {
	fsProjectItemsStore[datatug.DbCatalogs, *datatug.DbCatalog, datatug.DbCatalog]
}

func (s fsEnvDbCatalogStore) getDirPath(envID string) string {
	return filepath.Join(s.dirPath, storage.EnvironmentsFolder, envID, storage.EnvDbCatalogsFolder)
}

func (s fsEnvDbCatalogStore) flatCatalogFilePath(envID, catalogID string) string {
	return path.Join(s.getDirPath(envID), storage.JsonFileName(catalogID, storage.DbCatalogFileSuffix))
}

func (s fsEnvDbCatalogStore) nestedCatalogFilePath(envID, catalogID string) string {
	return path.Join(s.getDirPath(envID), catalogID, storage.JsonFileName(catalogID, storage.DbCatalogFileSuffix))
}

func readDbCatalogFile(filePath string) (*datatug.DbCatalog, error) {
	catalog := new(datatug.DbCatalog)
	if err := readJSONFile(filePath, true, catalog); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return catalog, nil
}

// loadOneEnvDbCatalog reads catalogID from both layouts and reconciles them:
// nested wins when both exist and their content matches; a content mismatch
// is a clear error. Returns (nil, nil) when the catalog exists in neither.
func (s fsEnvDbCatalogStore) loadOneEnvDbCatalog(envID, catalogID string) (*datatug.DbCatalog, error) {
	nestedPath, flatPath := s.nestedCatalogFilePath(envID, catalogID), s.flatCatalogFilePath(envID, catalogID)
	nested, err := readDbCatalogFile(nestedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load env db catalog[%s/%s] from %s: %w", envID, catalogID, nestedPath, err)
	}
	flat, err := readDbCatalogFile(flatPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load env db catalog[%s/%s] from %s: %w", envID, catalogID, flatPath, err)
	}
	switch {
	case nested != nil && flat != nil:
		if !reflect.DeepEqual(nested, flat) {
			return nil, fmt.Errorf(
				"env db catalog[%s/%s] is stored in both %s and %s with different content; remove one",
				envID, catalogID, nestedPath, flatPath)
		}
		nested.SetID(catalogID)
		return nested, nil
	case nested != nil:
		nested.SetID(catalogID)
		return nested, nil
	case flat != nil:
		flat.SetID(catalogID)
		return flat, nil
	default:
		return nil, nil
	}
}

func (s fsEnvDbCatalogStore) listEnvDbCatalogIDs(envID string) ([]string, error) {
	dirPath := s.getDirPath(envID)
	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	suffix := "." + storage.DbCatalogFileSuffix + ".json"
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
			if _, statErr := os.Stat(path.Join(dirPath, name, storage.JsonFileName(name, storage.DbCatalogFileSuffix))); statErr == nil {
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

func (s fsEnvDbCatalogStore) LoadEnvDbCatalogs(_ context.Context, envID string, o ...datatug.StoreOption) (datatug.DbCatalogs, error) {
	_ = datatug.GetStoreOptions(o...)
	ids, err := s.listEnvDbCatalogIDs(envID)
	if err != nil {
		return nil, err
	}
	catalogs := make(datatug.DbCatalogs, 0, len(ids))
	for _, id := range ids {
		catalog, err := s.loadOneEnvDbCatalog(envID, id)
		if err != nil {
			return nil, err
		}
		if catalog == nil {
			continue // named by the directory scan but vanished/unreadable in a benign race
		}
		catalogs = append(catalogs, catalog)
	}
	sort.Slice(catalogs, func(i, j int) bool {
		return catalogs[i].GetID() < catalogs[j].GetID()
	})
	return catalogs, nil
}

func (s fsEnvDbCatalogStore) LoadEnvDbCatalog(_ context.Context, envID, _, catalogID string, o ...datatug.StoreOption) (datatug.DbCatalog, error) {
	_ = datatug.GetStoreOptions(o...)
	catalog, err := s.loadOneEnvDbCatalog(envID, catalogID)
	if err != nil {
		return datatug.DbCatalog{}, err
	}
	if catalog == nil {
		return datatug.DbCatalog{}, fmt.Errorf("failed to load env db catalog[%s/%s] from project: %w", envID, catalogID, os.ErrNotExist)
	}
	return *catalog, nil
}

// saveTarget mirrors fsEntitiesStore.saveTarget: nested if a nested file
// already exists (nested wins, same as loadOneEnvDbCatalog), else flat if a
// flat file already exists (preserve the layout it was loaded from), else
// nested as the default for a brand-new catalog.
func (s fsEnvDbCatalogStore) saveTarget(envID, catalogID string) (dirPath, fileName string) {
	fileName = storage.JsonFileName(catalogID, storage.DbCatalogFileSuffix)
	if _, err := os.Stat(s.nestedCatalogFilePath(envID, catalogID)); err == nil {
		return path.Join(s.getDirPath(envID), catalogID), fileName
	}
	if _, err := os.Stat(s.flatCatalogFilePath(envID, catalogID)); err == nil {
		return s.getDirPath(envID), fileName
	}
	return path.Join(s.getDirPath(envID), catalogID), fileName
}

func (s fsEnvDbCatalogStore) saveOneEnvDbCatalog(envID string, catalog *datatug.DbCatalog) error {
	dirPath, fileName := s.saveTarget(envID, catalog.ID)
	if err := saveJSONFile(dirPath, fileName, catalog); err != nil {
		return fmt.Errorf("failed to save env db catalog file: %w", err)
	}
	return nil
}

func (s fsEnvDbCatalogStore) SaveEnvDbCatalog(_ context.Context, envID, _, _ string, catalog *datatug.DbCatalog) error {
	return s.saveOneEnvDbCatalog(envID, catalog)
}

func (s fsEnvDbCatalogStore) SaveEnvDbCatalogs(_ context.Context, envID, _, _ string, catalogs datatug.DbCatalogs) error {
	return saveItems(s.getDirPath(envID), len(catalogs), func(i int) func() error {
		return func() error {
			return s.saveOneEnvDbCatalog(envID, catalogs[i])
		}
	})
}

func (s fsEnvDbCatalogStore) DeleteEnvDbCatalog(_ context.Context, envID, _, catalogID string) error {
	for _, filePath := range []string{s.nestedCatalogFilePath(envID, catalogID), s.flatCatalogFilePath(envID, catalogID)} {
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

//// LoadEnvironmentCatalog return information about environment DB
//func (store fsEnvDbCatalogStore) LoadEnvironmentCatalog() (envDb *datatug.EnvDb, err error) {
//	filePath := path.Join(store.envsDirPath, store.envID, EnvDbCatalogsFolder, store.catalogID, JsonFileName(store.catalogID, DbCatalogFileSuffix))
//	envDb = new(datatug.EnvDb)
//	if err = readJSONFile(filePath, true, envDb); err != nil {
//		err = fmt.Errorf("failed to load environment DB catalog [%v] from env [%v] from project [%v]: %w", store.catalogID, store.envID, store.projectID, err)
//		return nil, err
//	}
//	envDb.ID = store.catalogID
//	if err = envDb.ValidateWithOptions(); err != nil {
//		return nil, fmt.Errorf("loaded environmend DB catalog file is invalid: %w", err)
//	}
//	return
//}
