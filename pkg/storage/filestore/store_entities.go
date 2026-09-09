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
	"github.com/strongo/validation"
)

var _ datatug.EntitiesStore = (*fsEntitiesStore)(nil)

func newFsEntitiesStore(projectPath string) fsEntitiesStore {
	return fsEntitiesStore{
		fsProjectItemsStore: newFileProjectItemsStore[datatug.Entities, *datatug.Entity, datatug.Entity](
			path.Join(projectPath, storage.EntitiesFolder), storage.EntityFileSuffix,
		),
	}
}

// fsEntitiesStore loads and saves entities from two on-disk layouts:
//   - flat: "<entitiesDir>/<id>.entity.json" (the legacy layout, still used by
//     the generic fsProjectItemsStore other stores in this package embed).
//   - nested: "<entitiesDir>/<id>/<id>.entity.json" (one directory per entity;
//     what datatug-cli's vendored model copy and datatug-demo-projects use).
//
// When an id exists in both layouts, nested wins if their content agrees;
// disagreement is a clear load error, never a silent pick of one side. A
// save keeps whichever layout an entity was loaded from; a brand-new entity
// defaults to nested, matching datatug-cli and datatug-demo-projects.
type fsEntitiesStore struct {
	fsProjectItemsStore[datatug.Entities, *datatug.Entity, datatug.Entity]
}

// flatEntityFilePath is the legacy layout: "<entitiesDir>/<id>.entity.json".
func (s fsEntitiesStore) flatEntityFilePath(id string) string {
	return path.Join(s.dirPath, storage.JsonFileName(id, s.itemFileSuffix))
}

// nestedEntityFilePath is the per-entity-directory layout: "<entitiesDir>/<id>/<id>.entity.json".
func (s fsEntitiesStore) nestedEntityFilePath(id string) string {
	return path.Join(s.dirPath, id, storage.JsonFileName(id, s.itemFileSuffix))
}

// readEntityFile reads one entity file, returning (nil, nil) - not an error -
// when the file does not exist, so callers can tell "absent" apart from a
// real read/parse failure.
func readEntityFile(filePath string) (*datatug.Entity, error) {
	entity := new(datatug.Entity)
	if err := readJSONFile(filePath, true, entity); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return entity, nil
}

// loadOneEntity reads id from both layouts and reconciles them: nested wins
// when both exist and their content matches; a content mismatch is a clear
// error. Returns (nil, nil) when the entity exists in neither layout.
func (s fsEntitiesStore) loadOneEntity(id string) (*datatug.Entity, error) {
	nestedPath, flatPath := s.nestedEntityFilePath(id), s.flatEntityFilePath(id)
	nested, err := readEntityFile(nestedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load entity[%s] from %s: %w", id, nestedPath, err)
	}
	flat, err := readEntityFile(flatPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load entity[%s] from %s: %w", id, flatPath, err)
	}
	switch {
	case nested != nil && flat != nil:
		if !reflect.DeepEqual(nested, flat) {
			return nil, fmt.Errorf(
				"entity[%s] is stored in both %s and %s with different content; remove one",
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

func (s fsEntitiesStore) LoadEntity(_ context.Context, id string, o ...datatug.StoreOption) (*datatug.Entity, error) {
	_ = datatug.GetStoreOptions(o...)
	entity, err := s.loadOneEntity(id)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, fmt.Errorf("failed to load entity[%s] from project: %w", id, os.ErrNotExist)
	}
	return entity, nil
}

// listEntityIDs returns every entity id found in either layout: flat
// "<entitiesDir>/<id>.entity.json" files and nested
// "<entitiesDir>/<id>/<id>.entity.json" directories.
func (s fsEntitiesStore) listEntityIDs() ([]string, error) {
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
			if _, statErr := os.Stat(s.nestedEntityFilePath(name)); statErr == nil {
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

func (s fsEntitiesStore) LoadEntities(_ context.Context, o ...datatug.StoreOption) (datatug.Entities, error) {
	_ = datatug.GetStoreOptions(o...)
	ids, err := s.listEntityIDs()
	if err != nil {
		return nil, err
	}
	entities := make(datatug.Entities, 0, len(ids))
	for _, id := range ids {
		entity, err := s.loadOneEntity(id)
		if err != nil {
			return nil, err
		}
		if entity == nil {
			continue // named by the directory scan but vanished/unreadable in a benign race
		}
		entities = append(entities, entity)
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].GetID() < entities[j].GetID()
	})
	return entities, nil
}

func (s fsEntitiesStore) DeleteEntity(_ context.Context, id string) error {
	var removedAny bool
	for _, filePath := range []string{s.nestedEntityFilePath(id), s.flatEntityFilePath(id)} {
		if _, err := os.Stat(filePath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := os.Remove(filePath); err != nil {
			return err
		}
		removedAny = true
	}
	_ = removedAny // no error either way: deleting an absent entity is a no-op, matching prior behavior
	return nil
}

func (s fsEntitiesStore) SaveEntities(ctx context.Context, entities datatug.Entities) (err error) {
	return saveItems(s.dirPath, len(entities), func(i int) func() error {
		return func() error {
			return s.SaveEntity(ctx, entities[i])
		}
	})
}

// saveTarget returns the directory+filename an entity should be (re)written
// to: nested if a nested file already exists for this id (nested wins, same
// as loadOneEntity); else flat if a flat file already exists (a save
// preserves the layout an entity was loaded from); else nested as the
// default for a brand-new entity, matching datatug-cli and
// datatug-demo-projects.
func (s fsEntitiesStore) saveTarget(id string) (dirPath, fileName string) {
	fileName = storage.JsonFileName(id, s.itemFileSuffix)
	if _, err := os.Stat(s.nestedEntityFilePath(id)); err == nil {
		return path.Join(s.dirPath, id), fileName
	}
	if _, err := os.Stat(s.flatEntityFilePath(id)); err == nil {
		return s.dirPath, fileName
	}
	return path.Join(s.dirPath, id), fileName
}

func (s fsEntitiesStore) SaveEntity(_ context.Context, entity *datatug.Entity) (err error) {
	if entity == nil {
		return validation.NewErrRequestIsMissingRequiredField("entity")
	}
	if entity.ID == "" {
		return validation.NewErrBadRequestFieldValue("entity", validation.NewErrRecordIsMissingRequiredField("GetID").Error())
	}
	if len(entity.Fields) == 0 && entity.Fields != nil {
		entity.Fields = nil
	}
	dirPath, fileName := s.saveTarget(entity.ID)
	if err = saveJSONFile(dirPath, fileName, entity); err != nil {
		return fmt.Errorf("failed to save entity file: %w", err)
	}
	return nil
}
