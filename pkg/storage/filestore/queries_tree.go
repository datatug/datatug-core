package filestore

import (
	"context"
	"fmt"
	"os"
	"path"
	"sort"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// loadQueriesTree recursively loads every queries folder and its items
// under relFolderPath (empty for the project's queries root), the walk
// LoadProject needs since fsQueriesStore.LoadQueries only scans one folder
// at a time. A folder with no items of its own and no sub-folders that
// themselves have any is omitted rather than returned as an empty entry -
// this is what keeps a folder holding only unrelated files (e.g.
// datatug-demo-projects/demo-project-1's legacy albums/artists/tracks
// *.sql.json queries, which predate the "<id>.query.json" suffix
// convention and so match nothing here) out of the tree entirely. Returns
// (nil, nil) when relFolderPath itself has nothing loadable.
//
// This is LoadProject's sole entry point into the query store: once a
// project has been touched by a write, it acquires the query store lock
// once for the whole recursive walk and holds it throughout, so a
// concurrent writer can never be observed mid-install partway through the
// tree. Before that (see withQueryReadLock), a fresh or read-only project
// has nothing to coordinate against and loads directly. Internal recursion
// calls loadQueriesTreeLocked/loadQueriesLocked directly - never the
// public LoadQueries - since the lock is not reentrant.
func (s fsQueriesStore) loadQueriesTree(ctx context.Context, relFolderPath string) (folder *datatug.QueriesFolder, err error) {
	err = s.withQueryReadLock(ctx, func(g queryLockGuard) error {
		var lockedErr error
		folder, lockedErr = s.loadQueriesTreeLocked(ctx, g, relFolderPath)
		return lockedErr
	})
	return folder, err
}

func (s fsQueriesStore) loadQueriesTreeLocked(ctx context.Context, g queryLockGuard, relFolderPath string) (*datatug.QueriesFolder, error) {
	if relFolderPath == "" {
		// A symlinked queries root would redirect the whole walk. Below it,
		// os.ReadDir reports a symlinked sub-folder as a non-directory,
		// which the loop skips, and each file is read by the query store's
		// regular-file-only reader (readQueryItemJSON).
		if _, err := walkQueryDir(s.dirPath, "", "", false); err != nil {
			return nil, err
		}
	}
	dirPath := path.Join(s.dirPath, relFolderPath)
	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	own, err := s.loadQueriesLocked(ctx, relFolderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load queries in %s: %w", relFolderPath, err)
	}

	var subFolders datatug.QueryFolders
	for _, de := range dirEntries {
		if !de.IsDir() {
			continue
		}
		if relFolderPath == "" && de.Name() == reservedQueryTxnDirName {
			// The query transaction namespace is internal bookkeeping - a
			// lock file, journal and staging area - never a user folder or
			// query (see query_lock.go/query_txn.go, and
			// validateQuerySegmentReason, which already refuses it as a
			// folder/ID a caller could ever address). It only ever lives
			// exactly at the queries root, so that is the only place this
			// skips it; a coincidentally-named ordinary sub-folder deeper in
			// the tree is never silently hidden.
			continue
		}
		sub, err := s.loadQueriesTreeLocked(ctx, g, path.Join(relFolderPath, de.Name()))
		if err != nil {
			return nil, err
		}
		if sub == nil {
			continue
		}
		sub.ID = de.Name()
		subFolders = append(subFolders, sub)
	}
	sort.Slice(subFolders, func(i, j int) bool {
		return subFolders[i].GetID() < subFolders[j].GetID()
	})

	if len(own.Items) == 0 && len(subFolders) == 0 {
		return nil, nil
	}
	own.Folders = subFolders
	return own, nil
}

// saveQueriesTree recursively saves every item in folder and its
// sub-folders under relFolderPath (empty for the project's queries root) -
// "the query saver must round-trip" against loadQueriesTree. A nil folder
// (a project with no queries) is a no-op.
//
// This is SaveProject's sole entry point into the query store: like
// loadQueriesTree, it acquires the query store lock once for the whole
// recursive save - so a project save writing several query pairs holds one
// coordination while each pair still gets its own recoverable transaction,
// per the plan's Approach.
func (s fsQueriesStore) saveQueriesTree(ctx context.Context, relFolderPath string, folder *datatug.QueriesFolder) error {
	if folder == nil {
		return nil
	}
	return s.withQueryLock(ctx, func(g queryLockGuard) error {
		return s.saveQueriesTreeLocked(ctx, g, relFolderPath, folder)
	})
}

func (s fsQueriesStore) saveQueriesTreeLocked(ctx context.Context, g queryLockGuard, relFolderPath string, folder *datatug.QueriesFolder) error {
	for _, item := range folder.Items {
		if item == nil {
			continue
		}
		if err := item.Validate(); err != nil {
			return fmt.Errorf("invalid query[%s] in %s: %w", item.ID, relFolderPath, err)
		}
		dir, err := s.resolveQueryLocation(relFolderPath, item.ID)
		if err != nil {
			return fmt.Errorf("failed to save query[%s] in %s: %w", item.ID, relFolderPath, err)
		}
		current, err := readCurrentQueryPair(dir, item.ID)
		if err != nil {
			return fmt.Errorf("failed to read existing query[%s] in %s: %w", item.ID, relFolderPath, err)
		}
		if _, err := s.stageAndInstallQueryPair(g, dir, relFolderPath, *item, current); err != nil {
			return fmt.Errorf("failed to save query[%s] in %s: %w", item.ID, relFolderPath, err)
		}
	}
	for _, sub := range folder.Folders {
		if sub == nil {
			continue
		}
		if err := s.saveQueriesTreeLocked(ctx, g, path.Join(relFolderPath, sub.GetID()), sub); err != nil {
			return err
		}
	}
	return nil
}
