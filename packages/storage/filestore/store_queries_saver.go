package filestore

import (
	"context"
	"errors"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/strongo/validation"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func getQueryPaths(queryID, queriesDirPath string) (
	qID,       // without directory and .json extension
	queryType, // Usually sql or HTTP
	queryFileName,
	queryDir,
	queryPath string, // Full file name
	err error,
) {
	if strings.TrimSpace(queryID) == "" {
		return "", "", "", "", "", validation.NewErrRequestIsMissingRequiredField("queryID")
	}
	queryDir = filepath.Dir(queryID)
	qID = filepath.Base(queryID)
	lastDotIndex := strings.LastIndex(qID, ".")
	queryType = qID[lastDotIndex+1:]
	qID = qID[:lastDotIndex]
	queryFileName = jsonFileName(qID, strings.ToLower(queryType))
	queryPath = path.Join(queriesDirPath, queryDir, queryFileName)
	return
}

func (store fsQueriesStore) CreateQueryFolder(_ context.Context, parentPath, name string) (err error) {
	folderPath := path.Join(store.queriesPath, parentPath, name)
	if err = os.MkdirAll(folderPath, 0666); err != nil {
		err = fmt.Errorf("failed to create folder: %w", err)
		return
	}
	readmePath := path.Join(folderPath, "README.md")
	if _, err = os.Stat(readmePath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			err = fmt.Errorf("failed to check README.md: %w", err)
			return
		}
		if err = ioutil.WriteFile(readmePath, []byte(fmt.Sprintf("# %v", name)), 0666); err != nil {
			err = fmt.Errorf("failed to write to README.md file: %w", err)
			return
		}
	}
	return
}

func (store fsQueriesStore) CreateQuery(_ context.Context, query models.QueryDefWithFolderPath) (*models.QueryDefWithFolderPath, error) {
	return &query, store.saveQuery(query.FolderPath, query.QueryDef, true)
}

func (store fsQueriesStore) saveQuery(folderPath string, query models.QueryDef, isNew bool) (err error) {
	if err := query.Validate(); err != nil {
		return fmt.Errorf("invalid query (isNew=%v): %w", isNew, err)
	}
	_, queryType, queryFileName, _, queryPath, err := getQueryPaths(folderPath+query.ID, store.queriesPath)

	queryText := query.Text
	queryType = strings.ToLower(queryType)
	if queryType == "sql" {
		query.Text = ""
	}

	if err = saveJSONFile(path.Base(queryPath), queryFileName, query); err != nil {
		return fmt.Errorf("failed to save query to json file: %w", err)
	}

	if queryType == "sql" {
		sqlFilePath := queryPath[:len(queryPath)-len(".json")]

		if err = ioutil.WriteFile(sqlFilePath, []byte(queryText), 0666); err != nil {
			return fmt.Errorf("faile to write query text to .sql file: %w", err)
		}
	}

	return nil
}
