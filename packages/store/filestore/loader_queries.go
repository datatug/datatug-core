package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"
)

func (loader fileSystemLoader) LoadQueries(projectID, folderPath string) (folder models.QueryFolder, err error) {
	var projPath string
	if _, projPath, err = loader.GetProjectPath(projectID); err != nil {
		return
	}
	queriesDirPath := path.Join(projPath, DatatugFolder, QueriesFolder)
	if folderPath != "" {
		queriesDirPath = path.Join(queriesDirPath, folderPath)
	}
	return loader.loadQueriesDir(queriesDirPath)
}

func (loader fileSystemLoader) LoadQuery(projectID, queryID string) (query models.QueryDef, err error) {
	var projPath string
	if _, projPath, err = loader.GetProjectPath(projectID); err != nil {
		return
	}
	queriesDirPath := path.Join(projPath, DatatugFolder, QueriesFolder)
	err = loader.loadQuery(queryID, queriesDirPath, &query)
	return
}

func (loader fileSystemLoader) loadQuery(queryID, dirPath string, query *models.QueryDef) error {
	if strings.HasSuffix(queryID, ".json") {
		return fmt.Errorf("queryID can't have .json suffix")
	}
	_, queryType, queryFileName, queryDir, queryPath, err := getQueryPaths(queryID, dirPath)
	if err = readJSONFile(queryPath, true, &query); err != nil {
		return fmt.Errorf("failed to load query definition from file: %v: %w", path.Join(queryDir, queryFileName), err)
	}
	if query.Text == "" && strings.HasSuffix(queryID, "."+querySqlFileSuffix) {
		content, err := ioutil.ReadFile(queryPath[:len(queryPath)-len("."+querySqlFileSuffix)])
		if err != nil {
			return fmt.Errorf("failed to load query text from .sql file: %w", err)
		}
		query.Text = string(content)
	}
	query.Type = queryType
	query.ID = queryID
	return nil
}

func (loader fileSystemLoader) loadQueriesDir(dirPath string) (folder models.QueryFolder, err error) {
	err = loadDir(nil, dirPath, processDirs|processFiles, func(files []os.FileInfo) {
		folder.Items = make(models.QueryDefs, 0, len(files))
	}, func(f os.FileInfo, i int, mutex *sync.Mutex) error {
		fileName := f.Name()
		if f.IsDir() {
			subFolder, err := loader.loadQueriesDir(path.Join(dirPath, fileName))
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
		if err = loader.loadQuery(fileName[:len(fileName)-len(".json")], dirPath, &query); err != nil {
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
