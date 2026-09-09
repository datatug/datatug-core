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
func (s fsQueriesStore) loadQueriesTree(ctx context.Context, relFolderPath string) (*datatug.QueriesFolder, error) {
	dirPath := path.Join(s.dirPath, relFolderPath)
	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	own, err := s.LoadQueries(ctx, relFolderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load queries in %s: %w", relFolderPath, err)
	}

	var subFolders datatug.QueryFolders
	for _, de := range dirEntries {
		if !de.IsDir() {
			continue
		}
		sub, err := s.loadQueriesTree(ctx, path.Join(relFolderPath, de.Name()))
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
func (s fsQueriesStore) saveQueriesTree(ctx context.Context, relFolderPath string, folder *datatug.QueriesFolder) error {
	if folder == nil {
		return nil
	}
	for _, item := range folder.Items {
		if item == nil {
			continue
		}
		if _, err := s.CreateQuery(ctx, datatug.QueryDefWithFolderPath{FolderPath: relFolderPath, QueryDef: *item}); err != nil {
			return fmt.Errorf("failed to save query[%s] in %s: %w", item.ID, relFolderPath, err)
		}
	}
	for _, sub := range folder.Folders {
		if sub == nil {
			continue
		}
		if err := s.saveQueriesTree(ctx, path.Join(relFolderPath, sub.GetID()), sub); err != nil {
			return err
		}
	}
	return nil
}
