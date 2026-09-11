package filestore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

// readQueryTextSidecar reads a query's body sidecar file, following the same
// "<id>.query.<lowercase type>" convention saveQuery writes (see
// pkg/datatug/doc.go): the JSON sidecar never carries Text (the saver strips
// it before writing), so the loader must read the body back separately.
// Returns ("", nil) when there is no sidecar - not every query has a text
// body (e.g. one with only structured parameters/recordsets so far).
func readQueryTextSidecar(dirPath string, query *datatug.QueryDef) (string, error) {
	if query.Type == "" {
		return "", nil
	}
	fileName := fmt.Sprintf("%s.%s.%s", query.ID, storage.QueryFileSuffix, strings.ToLower(string(query.Type)))
	data, err := os.ReadFile(path.Join(dirPath, fileName))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func newFsQueriesStore(projectPath string) fsQueriesStore {
	return fsQueriesStore{
		fsProjectItemsStore: newFileProjectItemsStore[datatug.QueryDefs, *datatug.QueryDef, datatug.QueryDef](
			path.Join(projectPath, storage.QueriesFolder), storage.QueryFileSuffix,
		),
	}
}

var _ datatug.QueriesStore = (*fsQueriesStore)(nil)

type fsQueriesStore struct {
	fsProjectItemsStore[datatug.QueryDefs, *datatug.QueryDef, datatug.QueryDef]
}

// LoadQueries implements datatug.QueriesStore. Once a project has ever
// been touched by a revisioned/legacy write, it acquires the query store
// lock (recovering any earlier interrupted transaction first) so a
// concurrent revisioned write can never be observed half-installed - see
// the plan's "participating readers include direct LoadQuery/LoadQueries
// calls". Before that, a project has nothing to coordinate against - see
// withQueryReadLock - so a project opened read-only, or one this store has
// simply never written to yet, stays fully readable without requiring
// write access.
func (s fsQueriesStore) LoadQueries(ctx context.Context, folderPath string, o ...datatug.StoreOption) (folder *datatug.QueriesFolder, err error) {
	err = s.withQueryReadLock(ctx, func(_ queryLockGuard) error {
		var lockedErr error
		folder, lockedErr = s.loadQueriesLocked(ctx, folderPath, o...)
		return lockedErr
	})
	return folder, err
}

// loadQueriesLocked is LoadQueries' body, callable by a caller that already
// holds the query store lock - loadQueriesTreeLocked's per-folder walk -
// without reacquiring it (the lock is not reentrant).
func (s fsQueriesStore) loadQueriesLocked(ctx context.Context, folderPath string, o ...datatug.StoreOption) (folder *datatug.QueriesFolder, err error) {
	_ = datatug.GetStoreOptions(o...)
	dirPath := path.Join(s.dirPath, folderPath)
	items, err := s.loadProjectItems(ctx, dirPath)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		text, err := readQueryTextSidecar(dirPath, item)
		if err != nil {
			return nil, fmt.Errorf("failed to load text sidecar for query[%s]: %w", item.ID, err)
		}
		item.Text = text
	}
	folder = &datatug.QueriesFolder{
		Items: make(datatug.QueryDefs, len(items)),
	}
	copy(folder.Items, items)
	return folder, nil
}

// LoadQuery implements datatug.QueriesStore. It keeps the legacy tolerant
// behavior (a missing body sidecar loads as an empty Text, not an error),
// but - like LoadQueries - now goes through the query store lock once a
// project has something for it to coordinate against (see
// withQueryReadLock).
func (s fsQueriesStore) LoadQuery(ctx context.Context, id string, o ...datatug.StoreOption) (query *datatug.QueryDef, err error) {
	err = s.withQueryReadLock(ctx, func(_ queryLockGuard) error {
		var lockedErr error
		query, lockedErr = s.loadQueryLocked(ctx, id, o...)
		return lockedErr
	})
	return query, err
}

func (s fsQueriesStore) loadQueryLocked(ctx context.Context, id string, o ...datatug.StoreOption) (query *datatug.QueryDef, err error) {
	folderPath, itemID := splitQueryFullID(id)
	dirPath := path.Join(s.dirPath, folderPath)
	query, err = s.loadProjectItem(ctx, dirPath, itemID, "", o...)
	if err != nil {
		return nil, err
	}
	text, err := readQueryTextSidecar(dirPath, query)
	if err != nil {
		return nil, fmt.Errorf("failed to load text sidecar for query[%s]: %w", id, err)
	}
	query.Text = text
	return query, nil
}

// UpdateQuery is a legacy update path, not part of the datatug.QueriesStore
// interface but kept for existing callers with its original signature.
// query.ID may be a "<folder>/.../<id>" combined path exactly like
// LoadQuery/DeleteQuery - UpdateQuery has no separate FolderPath parameter,
// so that is how a caller has always had to reach a nested folder here.
// Unlike the original implementation (which serialized the whole QueryDef,
// Text included, straight into the JSON sidecar), this now routes through
// the same pair transaction every other write uses: Text is stripped from
// the JSON and persisted to its own body sidecar, and a type change
// removes the stale sidecar - see the plan's "correct update/delete so
// they do not serialize Text into JSON or leave stale body sidecars".
func (s fsQueriesStore) UpdateQuery(ctx context.Context, query datatug.QueryDef) (q *datatug.QueryDefWithFolderPath, err error) {
	folderPath, itemID := splitQueryFullID(query.ID)
	query.ID = itemID
	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}
	dir, err := s.resolveQueryLocation(folderPath, itemID)
	if err != nil {
		return nil, err
	}

	err = s.withQueryLock(ctx, func(g queryLockGuard) error {
		current, err := readCurrentQueryPair(dir, itemID)
		if err != nil {
			return err
		}
		_, err = s.stageAndInstallQueryPair(g, dir, folderPath, query, current)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &datatug.QueryDefWithFolderPath{FolderPath: folderPath, QueryDef: query}, nil
}

// DeleteQuery implements datatug.QueriesStore. id may be a
// "<folder>/.../<id>" combined path exactly like LoadQuery. It is a no-op,
// not an error, when no record exists at that location - the original
// behavior. Unlike the original implementation (which removed only the
// JSON sidecar, leaving any body sidecar orphaned - see the plan's "leave
// stale body sidecars"), this now removes both files of the pair as one
// recoverable transaction, and validates the location the same way every
// other write does.
func (s fsQueriesStore) DeleteQuery(ctx context.Context, id string) (err error) {
	folderPath, itemID := splitQueryFullID(id)
	dir, err := s.resolveQueryLocation(folderPath, itemID)
	if err != nil {
		return err
	}
	return s.withQueryLock(ctx, func(g queryLockGuard) error {
		current, err := readCurrentQueryPair(dir, itemID)
		if err != nil {
			return err
		}
		return s.deleteQueryPairIfExists(g, folderPath, itemID, current)
	})
}

func (s fsQueriesStore) DeleteQueryFolder(_ context.Context, folderPath string) error {
	// This might need more implementation if we support folders
	_ = folderPath
	return errors.New("not implemented yet")
}

// SaveQuery implements datatug.QueriesStore. Unlike the original
// implementation (which silently ignored query.FolderPath and always wrote
// to the queries root - see the plan's "correct SaveQuery to honor
// FolderPath"), this resolves and writes to the query's actual folder. It
// remains unconditional like CreateQuery: an existing pair at the same
// location is replaced, not rejected - a caller that needs a create-only
// or stale-revision-safe write uses the revisioned PutQuery instead.
func (s fsQueriesStore) SaveQuery(ctx context.Context, query *datatug.QueryDefWithFolderPath) error {
	if err := query.QueryDef.Validate(); err != nil {
		return fmt.Errorf("invalid query: %w", err)
	}
	dir, err := s.resolveQueryLocation(query.FolderPath, query.ID)
	if err != nil {
		return err
	}
	return s.withQueryLock(ctx, func(g queryLockGuard) error {
		current, err := readCurrentQueryPair(dir, query.ID)
		if err != nil {
			return err
		}
		_, err = s.stageAndInstallQueryPair(g, dir, query.FolderPath, query.QueryDef, current)
		return err
	})
}
