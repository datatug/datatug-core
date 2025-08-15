package filestore

import (
	"context"
	"fmt"
	"github.com/datatug/datatug-core/pkg/models"
	"github.com/datatug/datatug-core/pkg/storage"
	"os"
	"path"
	"strings"
	"sync"
)

var _ storage.QueriesStore = (*fsQueriesStore)(nil)

type fsQueriesStore struct {
	fsProjectStore
	queriesPath string
}

func (store fsQueriesStore) Project() storage.ProjectStore {
	return store.fsProjectStore
}

func newFsQueriesStore(fsProjectStore fsProjectStore) fsQueriesStore {
	return fsQueriesStore{fsProjectStore: fsProjectStore, queriesPath: path.Join(fsProjectStore.projectPath, DatatugFolder, QueriesFolder)}
}

func (store fsQueriesStore) LoadQueries(ctx context.Context, folderPath string) (folder *models.QueryFolder, err error) {
	return store.loadQueriesDir(ctx, path.Join(store.queriesPath, folderPath))
}

func (store fsQueriesStore) loadQueriesDir(ctx context.Context, dirPath string) (folder *models.QueryFolder, err error) {
	err = loadDir(nil, dirPath, processDirs|processFiles, func(files []os.FileInfo) {
		folder = new(models.QueryFolder)
		folder.Items = make(models.QueryDefs, 0, len(files))
	}, func(f os.FileInfo, i int, mutex *sync.Mutex) error {
		fileName := f.Name()
		if f.IsDir() {
			subFolder, err := store.loadQueriesDir(ctx, path.Join(dirPath, fileName))
			if err != nil {
				return err
			}
			if mutex != nil {
				mutex.Lock()
				defer mutex.Unlock()
			}
			subFolder.ID = fileName
			folder.Folders = append(folder.Folders, subFolder)
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(fileName), ".json") {
			return nil
		}
		var query models.QueryDef
		query.ID = fileName[:len(fileName)-len(".json")]
		if err = store.loadQuery(dirPath, &query); err != nil {
			return err
		}
		if mutex != nil {
			mutex.Lock()
			defer mutex.Unlock()
		}
		folder.Items = append(folder.Items, query)
		return nil
	})
	return
}

func (store fsQueriesStore) DeleteQueryFolder(_ context.Context, folderPath string) error {
	fullPath := path.Join(store.queriesPath, folderPath)
	if err := os.RemoveAll(fullPath); err != nil {
		return fmt.Errorf("failed to remove query folder %v: %w", folderPath, err)
	}
	return nil
}

func (store fsQueriesStore) GetQuery(_ context.Context, id string) (query *models.QueryDefWithFolderPath, err error) {
	query = new(models.QueryDefWithFolderPath)
	query.ID = id
	if query.FolderPath, err = store.getQueryFolderPath(id); err != nil {
		return
	}
	dirPath := path.Join(store.queriesPath, strings.Trim(query.FolderPath, "~"))
	err = store.loadQuery(dirPath, &query.QueryDef)
	return
}

func (store fsQueriesStore) loadQuery(dirPath string, query *models.QueryDef) error {
	if strings.HasSuffix(query.ID, ".json") {
		return fmt.Errorf("queryID can't have .json suffix")
	}
	_, queryType, queryFileName, queryDir, queryPath, err := getQueryPaths(query.ID, dirPath)
	if err != nil {
		return fmt.Errorf("failed to get query paths: %w", err)
	}
	if err = readJSONFile(queryPath, true, &query); err != nil {
		return fmt.Errorf("failed to load query definition from file: %v: %w", path.Join(queryDir, queryFileName), err)
	}
	if query.Text == "" && strings.HasSuffix(query.ID, "."+querySQLFileSuffix) {
		content, err := os.ReadFile(queryPath[:len(queryPath)-len("."+querySQLFileSuffix)])
		if err != nil {
			return fmt.Errorf("failed to load query text from .sql file: %w", err)
		}
		query.Text = string(content)
	}
	query.Type = queryType
	return nil
}

func (store fsQueriesStore) DeleteQuery(_ context.Context, id string) (err error) {
	_, _, queryFileName, queryDir, queryPath, err := getQueryPaths(id, store.queriesPath)
	if err != nil {
		return err
	}
	if err = os.Remove(queryPath); err != nil {
		return fmt.Errorf("failed to remove query file %v: %w", path.Join(queryDir, queryFileName), err)
	}
	return err
}

func (store fsQueriesStore) getQueryFolderPath(queryID string) (folderPath string, err error) {
	panic("not implemented")
}

func (store fsQueriesStore) UpdateQuery(_ context.Context, query models.QueryDef) (q *models.QueryDefWithFolderPath, err error) {
	q = new(models.QueryDefWithFolderPath)
	q.QueryDef = query
	if q.FolderPath, err = store.getQueryFolderPath(query.ID); err != nil {
		return
	}
	err = store.saveQuery(q.FolderPath, query, false)
	return
}
