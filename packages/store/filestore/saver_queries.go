package filestore

import (
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
	qID, // without directory and .json extension
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

func (s fileSystemSaver) DeleteQueryFolder(folderPath string) error {
	fullPath := path.Join(queriesDirPath(s.projDirPath), folderPath)
	if err := os.RemoveAll(fullPath); err != nil {
		return fmt.Errorf("failed to remove query folder %v: %w", folderPath, err)
	}
	return nil
}

func (s fileSystemSaver) DeleteQuery(queryID string) error {
	_, _, queryFileName, queryDir, queryPath, err := getQueryPaths(queryID, queriesDirPath(s.projDirPath))
	if err != nil {
		return err
	}
	if err = os.Remove(queryPath); err != nil {
		return fmt.Errorf("failed to remove query file %v: %w", path.Join(queryDir, queryFileName), err)
	}
	return err
}

func (s fileSystemSaver) UpdateQuery(query models.QueryDef) (err error) {
	return s.saveQuery(query, false)
}

func (s fileSystemSaver) CreateQueryFolder(parentPath, id string) (folder models.QueryFolder, err error) {
	folderPath := path.Join(queriesDirPath(s.projDirPath), parentPath, id)
	if err = os.MkdirAll(folderPath, 0666); err != nil {
		return
	}
	readmePath := path.Join(folderPath, "README.md")
	if _, err = os.Stat(readmePath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return
		}
		if err = ioutil.WriteFile(readmePath, []byte(fmt.Sprintf("# %v", id)), 066); err != nil {
			return
		}
	}
	folder.ID = id
	return
}

func (s fileSystemSaver) CreateQuery(query models.QueryDef) (err error) {
	return s.saveQuery(query, true)
}

func (s fileSystemSaver) saveQuery(query models.QueryDef, isNew bool) (err error) {
	if err := query.Validate(); err != nil {
		return fmt.Errorf("invalid query: %w", err)
	}
	_, queryType, queryFileName, _, queryPath, err := getQueryPaths(query.ID, queriesDirPath(s.projDirPath))

	queryText := query.Text
	queryType = strings.ToLower(queryType)
	if queryType == "sql" {
		query.Text = ""
	}

	if err = s.saveJSONFile(path.Base(queryPath), queryFileName, query); err != nil {
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
