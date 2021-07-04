package filestore

import (
	"context"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func newFsQueryStore(queryID string, fsQueriesStore fsQueriesStore) fsQueryStore {
	return fsQueryStore{queryID: queryID, fsQueriesStore: fsQueriesStore}
}

var _ storage.QueryStore = (*fsQueryStore)(nil)

type fsQueryStore struct {
	queryID string
	fsQueriesStore
}

func (store fsQueryStore) ID() string {
	return store.queryID
}

func (store fsQueryStore) LoadQuery(context.Context) (query *models.QueryDef, err error) {
	queriesDirPath := path.Join(store.projectPath, DatatugFolder, QueriesFolder)
	query = new(models.QueryDef)
	err = store.loadQuery(queriesDirPath, query)
	return
}

func (store fsQueryStore) loadQuery(dirPath string, query *models.QueryDef) error {
	if strings.HasSuffix(store.queryID, ".json") {
		return fmt.Errorf("queryID can't have .json suffix")
	}
	_, queryType, queryFileName, queryDir, queryPath, err := getQueryPaths(store.queryID, dirPath)
	if err = readJSONFile(queryPath, true, &query); err != nil {
		return fmt.Errorf("failed to load query definition from file: %v: %w", path.Join(queryDir, queryFileName), err)
	}
	if query.Text == "" && strings.HasSuffix(store.queryID, "."+querySQLFileSuffix) {
		content, err := ioutil.ReadFile(queryPath[:len(queryPath)-len("."+querySQLFileSuffix)])
		if err != nil {
			return fmt.Errorf("failed to load query text from .sql file: %w", err)
		}
		query.Text = string(content)
	}
	query.Type = queryType
	query.ID = store.queryID
	return nil
}

func (store fsQueryStore) DeleteQuery(context.Context) (err error) {
	_, _, queryFileName, queryDir, queryPath, err := getQueryPaths(store.queryID, store.queriesPath)
	if err != nil {
		return err
	}
	if err = os.Remove(queryPath); err != nil {
		return fmt.Errorf("failed to remove query file %v: %w", path.Join(queryDir, queryFileName), err)
	}
	return err
}

func (store fsQueryStore) UpdateQuery(_ context.Context, query models.QueryDef) (err error) {
	folderPath := filepath.Dir(query.ID)
	query.ID = filepath.Base(query.ID)
	return store.saveQuery(folderPath, query, false)
}
