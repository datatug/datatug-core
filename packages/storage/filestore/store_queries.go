package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
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

func (store fsQueriesStore) LoadQueries(folderPath string) (folder *models.QueryFolder, err error) {
	return store.loadQueriesDir(store.queriesPath)
}

func (store fsQueriesStore) loadQueriesDir(dirPath string) (folder *models.QueryFolder, err error) {
	err = loadDir(nil, dirPath, processDirs|processFiles, func(files []os.FileInfo) {
		folder = new(models.QueryFolder)
		folder.Items = make(models.QueryDefs, 0, len(files))
	}, func(f os.FileInfo, i int, mutex *sync.Mutex) error {
		fileName := f.Name()
		if f.IsDir() {
			subFolder, err := store.loadQueriesDir(path.Join(dirPath, fileName))
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
		queryStore := newFsQueryStore(query.ID, store)
		if err = queryStore.loadQuery(dirPath, &query); err != nil {
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

func (store fsQueriesStore) DeleteQueryFolder(folderPath string) error {
	fullPath := path.Join(store.queriesPath, folderPath)
	if err := os.RemoveAll(fullPath); err != nil {
		return fmt.Errorf("failed to remove query folder %v: %w", folderPath, err)
	}
	return nil
}
