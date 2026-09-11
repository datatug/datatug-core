package filestore

import (
	"context"
	"fmt"
	"os"
	"path"
	"strconv"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// CreateQueryFolder creates folder name under parentPath, plus a README.md
// naming it unless one is already there. parentPath and name are validated
// like every query write location (validateQueryFolderPath,
// validateQuerySegmentReason), and each segment is checked or created by
// walkQueryDir, so nothing is ever created outside the queries root or
// through a symlinked or non-directory segment. README.md is created
// exclusively: an existing entry - a file, or a symlink whether dangling or
// not - is left alone and never followed.
func (s fsQueriesStore) CreateQueryFolder(_ context.Context, parentPath, name string) error {
	if err := validateQueryFolderPath(parentPath); err != nil {
		return err
	}
	if reason, ok := validateQuerySegmentReason(name); !ok {
		return invalidQueryLocation(parentPath, "", "folder name "+strconv.Quote(name)+": "+reason)
	}
	folderPath := name
	if parentPath != "" {
		folderPath = parentPath + "/" + name
	}
	dir, err := walkQueryDir(s.dirPath, folderPath, "", true)
	if err != nil {
		return fmt.Errorf("failed to create folder: %w", err)
	}
	f, err := os.OpenFile(path.Join(dir, "README.md"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return fmt.Errorf("failed to create README.md file: %w", err)
	}
	if _, err := fmt.Fprintf(f, "# %v", name); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to write to README.md file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to write to README.md file: %w", err)
	}
	return nil
}

// CreateQuery is a legacy save path used by saveQueriesTree during project
// saves and by existing callers directly. Despite its name it has always
// been an unconditional upsert (nothing here ever checked whether a record
// already existed) - a caller that needs true create-only or
// stale-revision-safe semantics uses the revisioned PutQuery instead. It
// now routes through the same pair transaction every other write uses,
// with the same location validation SaveQuery applies.
func (s fsQueriesStore) CreateQuery(ctx context.Context, query datatug.QueryDefWithFolderPath) (*datatug.QueryDefWithFolderPath, error) {
	if err := query.QueryDef.Validate(); err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}
	if _, err := s.resolveQueryLocation(query.FolderPath, query.ID); err != nil {
		return nil, err
	}
	err := s.withQueryLock(ctx, func(g queryLockGuard) error {
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
	if err != nil {
		return nil, err
	}
	return &query, nil
}
