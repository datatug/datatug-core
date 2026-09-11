package filestore

import (
	"context"
	"errors"
	"fmt"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

// readQueryTextSidecar reads a query's body sidecar file, following the same
// "<id>.query.<lowercase type>" convention saveQuery writes (see
// pkg/datatug/doc.go): the JSON sidecar never carries Text (the saver strips
// it before writing), so the loader must read the body back separately.
// Returns ("", nil) when there is no sidecar - not every query has a text
// body (e.g. one with only structured parameters/recordsets so far). The
// file name is derived by queryBodyFileName, which refuses a type from the
// JSON metadata that could not safely name a file (a type is project
// content, and "/../../x" would otherwise address a file outside the
// project).
//
// budget, when not nil, is the calling listing's byte budget
// (maxQueryListingBytes): the body's size is charged to it before the file
// is opened.
func readQueryTextSidecar(dirPath string, query *datatug.QueryDef, budget *queryReadBudget) (string, error) {
	if query.Type == "" {
		return "", nil
	}
	fileName, err := queryBodyFileName(query.ID, query.Type)
	if err != nil {
		return "", err
	}
	data, exists, err := readRegularFileBudgeted(path.Join(dirPath, fileName), maxQueryFileSize, budget)
	if err != nil || !exists {
		return "", err
	}
	return string(data), nil
}

func newFsQueriesStore(projectPath string) fsQueriesStore {
	items := newFileProjectItemsStore[datatug.QueryDefs, *datatug.QueryDef, datatug.QueryDef](
		path.Join(projectPath, storage.QueriesFolder), storage.QueryFileSuffix,
	)
	// Legacy query loads read metadata only from regular files within the
	// read cap (readQueryItemJSON), like every other query-store read.
	items.readItemJSON = readQueryItemJSON
	return fsQueriesStore{fsProjectItemsStore: items, listingBudget: maxQueryListingBytes}
}

var _ datatug.QueriesStore = (*fsQueriesStore)(nil)

type fsQueriesStore struct {
	fsProjectItemsStore[datatug.QueryDefs, *datatug.QueryDef, datatug.QueryDef]

	// listingBudget is the byte budget of one listing call
	// (maxQueryListingBytes; tests lower it).
	listingBudget int64
}

// newListingBudget returns a fresh budget for one listing call. It is
// created inside the read closure, so a read that withQueryReadLock runs
// again under the lock starts from a full budget.
func (s fsQueriesStore) newListingBudget() *queryReadBudget {
	limit := s.listingBudget
	if limit <= 0 { // a zero-value store gets the default, never an empty budget
		limit = maxQueryListingBytes
	}
	return &queryReadBudget{remaining: limit, limit: limit}
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
//
// folderPath is validated and resolved by resolveQueryReadFolder (under the
// lock, when one is taken): a legacy read can address only a real
// directory inside the queries root, never "..", an absolute path or a
// symlinked folder.
func (s fsQueriesStore) LoadQueries(ctx context.Context, folderPath string, o ...datatug.StoreOption) (folder *datatug.QueriesFolder, err error) {
	err = s.withQueryReadLock(ctx, func(g queryLockGuard) error {
		relFolderPath, lockedErr := s.resolveQueryReadFolder(folderPath)
		if lockedErr != nil {
			return lockedErr
		}
		folder, lockedErr = s.loadQueriesLocked(ctx, g, relFolderPath, s.newListingBudget(), o...)
		return lockedErr
	})
	return folder, err
}

// loadQueriesLocked is LoadQueries' body, callable by a caller that already
// holds the query store lock - loadQueriesTreeLocked's per-folder walk -
// without reacquiring it (the lock is not reentrant). Every metadata and
// body file it reads is charged to budget (maxQueryListingBytes) before it
// is opened. A folder holding a query whose committed write cannot be
// completed yet is refused (g.refuseStuckFolder): a listing never omits a
// query, and never serves one half installed.
func (s fsQueriesStore) loadQueriesLocked(ctx context.Context, g queryLockGuard, folderPath string, budget *queryReadBudget, o ...datatug.StoreOption) (folder *datatug.QueriesFolder, err error) {
	_ = datatug.GetStoreOptions(o...)
	dirPath := path.Join(s.dirPath, folderPath)
	if err := g.refuseStuckFolder(folderPath, dirPath); err != nil {
		return nil, err
	}
	items := s.fsProjectItemsStore // a copy: its reader is bound to this call's budget
	items.readItemJSON = func(filePath string, dst any) error {
		return readQueryItemJSONBudgeted(filePath, dst, budget)
	}
	loaded, err := items.loadProjectItems(ctx, dirPath)
	if err != nil {
		return nil, err
	}
	for _, item := range loaded {
		text, err := readQueryTextSidecar(dirPath, item, budget)
		if err != nil {
			return nil, fmt.Errorf("failed to load text sidecar for query[%s]: %w", item.ID, err)
		}
		item.Text = text
	}
	folder = &datatug.QueriesFolder{
		Items: make(datatug.QueryDefs, len(loaded)),
	}
	copy(folder.Items, loaded)
	return folder, nil
}

// LoadQuery implements datatug.QueriesStore. It keeps the legacy tolerant
// behavior (a missing body sidecar loads as an empty Text, not an error),
// but - like LoadQueries - now goes through the query store lock once a
// project has something for it to coordinate against (see
// withQueryReadLock).
func (s fsQueriesStore) LoadQuery(ctx context.Context, id string, o ...datatug.StoreOption) (query *datatug.QueryDef, err error) {
	err = s.withQueryReadLock(ctx, func(g queryLockGuard) error {
		var lockedErr error
		query, lockedErr = s.loadQueryLocked(ctx, g, id, o...)
		return lockedErr
	})
	return query, err
}

// loadQueryLocked resolves id's folder with resolveQueryReadFolder and
// requires the item ID to be a single segment (validateQueryReadSegmentReason),
// so a legacy LoadQuery can never read outside the queries root.
func (s fsQueriesStore) loadQueryLocked(ctx context.Context, g queryLockGuard, id string, o ...datatug.StoreOption) (query *datatug.QueryDef, err error) {
	folderPath, itemID := splitQueryFullID(id)
	if reason, ok := validateQueryReadSegmentReason(itemID); !ok {
		return nil, invalidQueryLocation(folderPath, itemID, "id: "+reason)
	}
	relFolderPath, err := s.resolveQueryReadFolder(folderPath)
	if err != nil {
		return nil, err
	}
	dirPath := path.Join(s.dirPath, relFolderPath)
	if err := g.refuseStuckQuery(relFolderPath, dirPath, itemID); err != nil {
		return nil, err
	}
	query, err = s.loadProjectItem(ctx, dirPath, itemID, "", o...)
	if err != nil {
		return nil, err
	}
	text, err := readQueryTextSidecar(dirPath, query, nil)
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
	if _, err := s.resolveQueryLocation(folderPath, itemID); err != nil {
		return nil, err
	}

	err = s.withQueryLock(ctx, func(g queryLockGuard) error {
		dir, err := s.resolveQueryLocation(folderPath, itemID) // again, under the lock (N3)
		if err != nil {
			return err
		}
		current, err := g.readQueryPair(folderPath, dir, itemID)
		if err != nil {
			return err
		}
		_, err = s.stageAndInstallQueryPair(g, folderPath, query, current)
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
	if _, err := s.resolveQueryLocation(folderPath, itemID); err != nil {
		return err
	}
	return s.withQueryLock(ctx, func(g queryLockGuard) error {
		dir, err := s.resolveQueryLocation(folderPath, itemID) // again, under the lock (N3)
		if err != nil {
			return err
		}
		current, err := g.readQueryPair(folderPath, dir, itemID)
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
	if _, err := s.resolveQueryLocation(query.FolderPath, query.ID); err != nil {
		return err
	}
	return s.withQueryLock(ctx, func(g queryLockGuard) error {
		dir, err := s.resolveQueryLocation(query.FolderPath, query.ID) // again, under the lock (N3)
		if err != nil {
			return err
		}
		current, err := g.readQueryPair(query.FolderPath, dir, query.ID)
		if err != nil {
			return err
		}
		_, err = s.stageAndInstallQueryPair(g, query.FolderPath, query.QueryDef, current)
		return err
	})
}
