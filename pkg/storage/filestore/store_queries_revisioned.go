package filestore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

var _ datatug.RevisionedQueriesStore = (*fsQueriesStore)(nil)

// splitQueryFullID splits a "<folder>/.../<id>" combined query id the same
// way the legacy LoadQuery/DeleteQuery already do: everything but the last
// "/"-separated part is the folder path, the last part is the id.
func splitQueryFullID(fullID string) (folderPath, id string) {
	parts := strings.Split(fullID, "/")
	folderPath = strings.Join(parts[:len(parts)-1], "/")
	id = parts[len(parts)-1]
	return folderPath, id
}

// LoadQueryRevision implements datatug.RevisionedQueriesStore. Location
// validation runs before the query store lock is even considered, so an
// invalid id never creates the reserved transaction directory. On a
// project no write has ever touched, it reads directly without requiring
// write access - see withQueryReadLock; there is categorically nothing to
// recover when that namespace has never existed.
func (s fsQueriesStore) LoadQueryRevision(ctx context.Context, id string, o ...datatug.StoreOption) (*datatug.StoredQuery, error) {
	_ = datatug.GetStoreOptions(o...)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	folderPath, itemID := splitQueryFullID(id)
	dir, err := s.resolveQueryLocation(folderPath, itemID)
	if err != nil {
		return nil, err
	}

	var result *datatug.StoredQuery
	err = s.withQueryReadLock(ctx, func(_ queryLockGuard) error {
		current, err := readCurrentQueryPair(dir, itemID)
		if err != nil {
			return err
		}
		if !current.exists {
			return fmt.Errorf("query %q not found: %w", id, os.ErrNotExist)
		}
		if !current.complete {
			return &datatug.IncompleteQueryRecordError{
				FolderPath: folderPath, ID: itemID,
				Reason: "committed pair is incomplete: JSON metadata exists without its expected body sidecar",
			}
		}
		var qd datatug.QueryDef
		if err := json.Unmarshal(current.jsonBytes, &qd); err != nil {
			return fmt.Errorf("failed to parse query metadata for %s: %w", id, err)
		}
		qd.ID = itemID
		qd.Text = string(current.bodyBytes)
		result = &datatug.StoredQuery{
			Query:    datatug.QueryDefWithFolderPath{FolderPath: folderPath, QueryDef: qd},
			Revision: current.revision,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// PutQuery implements datatug.RevisionedQueriesStore. condition, the query
// content (including its credential rules) and its location are all
// validated before the query store lock is acquired, so a rejected
// request never creates the reserved transaction directory and never
// touches the file system at all.
func (s fsQueriesStore) PutQuery(ctx context.Context, query *datatug.QueryDefWithFolderPath, condition datatug.QueryWriteCondition) (*datatug.StoredQuery, error) {
	if query == nil {
		return nil, fmt.Errorf("query must not be nil")
	}
	if err := validateQueryWriteCondition(condition); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateQueryForWrite(query.QueryDef); err != nil {
		return nil, err
	}
	dir, err := s.resolveQueryLocation(query.FolderPath, query.ID)
	if err != nil {
		return nil, err
	}

	var result *datatug.StoredQuery
	err = s.withQueryLock(ctx, func(g queryLockGuard) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := readCurrentQueryPair(dir, query.ID)
		if err != nil {
			return err
		}
		if err := checkQueryWriteCondition(query.FolderPath, query.ID, condition, current); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			// Still before the first target mutation: nothing but the
			// private staging area (cleaned up by stageAndInstallQueryPair
			// on a staging failure) has been touched.
			return err
		}

		// Commit point reached inside stageAndInstallQueryPair (once its
		// journal write returns): complete regardless of ctx from here.
		revision, err := s.stageAndInstallQueryPair(g, dir, query.FolderPath, query.QueryDef, current)
		if err != nil {
			return err
		}

		result = &datatug.StoredQuery{
			Query:    datatug.QueryDefWithFolderPath{FolderPath: query.FolderPath, QueryDef: query.QueryDef},
			Revision: revision,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// checkQueryWriteCondition enforces condition against current, returning a
// *datatug.QueryRevisionConflictError when it is not met. condition has
// already been validated (validateQueryWriteCondition) to set exactly one
// of IfNoneMatch/IfMatch.
func checkQueryWriteCondition(folderPath, id string, condition datatug.QueryWriteCondition, current currentQueryPair) error {
	if condition.IfNoneMatch {
		if current.exists {
			return &datatug.QueryRevisionConflictError{
				FolderPath: folderPath, ID: id, Actual: current.revision,
				Reason: "a query already exists at this location",
			}
		}
		return nil
	}
	if !current.exists || !current.complete {
		return &datatug.QueryRevisionConflictError{
			FolderPath: folderPath, ID: id, Expected: condition.IfMatch,
			Reason: "no matching query exists at this location",
		}
	}
	if current.revision != condition.IfMatch {
		return &datatug.QueryRevisionConflictError{
			FolderPath: folderPath, ID: id, Expected: condition.IfMatch, Actual: current.revision,
			Reason: "stale revision",
		}
	}
	return nil
}

// DeleteQueryRevision implements datatug.RevisionedQueriesStore. Location
// validation runs before the query store lock is acquired, so an invalid
// id never creates the reserved transaction directory.
func (s fsQueriesStore) DeleteQueryRevision(ctx context.Context, id string, expected datatug.QueryRevision) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	folderPath, itemID := splitQueryFullID(id)
	dir, err := s.resolveQueryLocation(folderPath, itemID)
	if err != nil {
		return err
	}

	return s.withQueryLock(ctx, func(g queryLockGuard) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := readCurrentQueryPair(dir, itemID)
		if err != nil {
			return err
		}
		if !current.exists || !current.complete || current.revision != expected {
			return &datatug.QueryRevisionConflictError{
				FolderPath: folderPath, ID: itemID, Expected: expected, Actual: current.revision,
				Reason: "stale or missing revision",
			}
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		j := queryTxnJournal{
			FolderPath:   folderPath,
			ID:           itemID,
			Operation:    queryTxnOpDelete,
			JSONFileName: storage.JsonFileName(itemID, storage.QueryFileSuffix),
			BodyFileName: current.bodyFileName,
		}
		if err := writeJournal(g.txnDir, j); err != nil {
			return err
		}
		// Commit point reached: complete regardless of ctx from here.
		return completeQueryTransaction(s.dirPath, g.txnDir)
	})
}
