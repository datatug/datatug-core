package filestore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
)

func (s fsQueriesStore) CreateQueryFolder(_ context.Context, parentPath, name string) (err error) {
	folderPath := path.Join(s.dirPath, parentPath, name)
	if err = os.MkdirAll(folderPath, 0777); err != nil {
		err = fmt.Errorf("failed to create folder: %w", err)
		return
	}
	readmePath := path.Join(folderPath, "README.md")
	if _, err = os.Stat(readmePath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			err = fmt.Errorf("failed to check README.md: %w", err)
			return
		}
		if err = os.WriteFile(readmePath, []byte(fmt.Sprintf("# %v", name)), 0644); err != nil {
			err = fmt.Errorf("failed to write to README.md file: %w", err)
			return
		}
	}
	return
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
	dir, err := s.resolveQueryLocation(query.FolderPath, query.ID)
	if err != nil {
		return nil, err
	}
	err = s.withQueryLock(ctx, func(g queryLockGuard) error {
		current, err := readCurrentQueryPair(dir, query.ID)
		if err != nil {
			return err
		}
		_, err = s.stageAndInstallQueryPair(g, dir, query.FolderPath, query.QueryDef, current)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &query, nil
}
