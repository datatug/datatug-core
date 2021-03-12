package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"os"
	"path"
	"strings"
	"sync"
)

func (loader fileSystemLoader) LoadQueries(projectID, folder string) (queries []models.QueryDef, err error) {
	var projPath string
	if _, projPath, err = loader.GetProjectPath(projectID); err != nil {
		return
	}
	queriesDirPath := path.Join(projPath, DatatugFolder, QueriesFolder)
	if folder != "" {
		queriesDirPath = path.Join(queriesDirPath, folder)
	}
	return loader.loadQueriesDir(queriesDirPath)
}

func (loader fileSystemLoader) LoadQuery(projectID, queryID string) (query models.QueryDef, err error) {
	var projPath string
	if _, projPath, err = loader.GetProjectPath(projectID); err != nil {
		return
	}
	queriesDirPath := path.Join(projPath, DatatugFolder, QueriesFolder)
	_, queryFileName, queryDir, queryPath, err := getQueryPaths(queryID, queriesDirPath)
	if err = readJSONFile(queryPath, true, &query); err != nil {
		return query, fmt.Errorf("failed to load query definition from file: %v: %w", path.Join(queryDir, queryFileName), err)
	}
	query.ID = queryID
	return
}

func (loader fileSystemLoader) loadQueriesDir(dirPath string) (queries []models.QueryDef, err error) {
	err = loadDir(nil, dirPath, processDirs|processFiles, func(files []os.FileInfo) {
		queries = make([]models.QueryDef, 0, len(files))
	}, func(f os.FileInfo, i int, mutex *sync.Mutex) error {
		fileName := f.Name()
		if f.IsDir() {
			subQueries, err := loader.loadQueriesDir(path.Join(dirPath, fileName))
			if err != nil {
				return err
			}
			folder := models.QueryDef{
				Type: "folder",
				ProjectItem: models.ProjectItem{
					ID: fileName,
				},
				Queries: subQueries,
			}
			if mutex != nil {
				mutex.Lock()
			}
			queries = append(queries, folder)
			if mutex != nil {
				mutex.Unlock()
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(fileName), ".json") {
			return nil
		}
		filePath := path.Join(dirPath, fileName)
		var query models.QueryDef
		if err := readJSONFile(filePath, true, &query); err != nil {
			return err
		}
		query.ID = fileName[:len(fileName)-len(".json")]
		if mutex != nil {
			mutex.Lock()
		}
		queries = append(queries, query)
		if mutex != nil {
			mutex.Unlock()
		}
		return nil
	})
	return
}
