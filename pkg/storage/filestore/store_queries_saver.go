package filestore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/strongo/validation"
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
	if lastDotIndex == -1 {
		return "", "", "", "", "", fmt.Errorf("queryID must have an extension: %v", queryID)
	}
	queryType = strings.ToLower(qID[lastDotIndex+1:])
	qID = qID[:lastDotIndex]
	queryFileName = jsonFileName(qID, queryType)
	queryPath = path.Join(queriesDirPath, queryDir, queryFileName)
	return
}

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

func (s fsQueriesStore) CreateQuery(_ context.Context, query datatug.QueryDefWithFolderPath) (*datatug.QueryDefWithFolderPath, error) {
	return &query, s.saveQuery(query.FolderPath, query.QueryDef, true)
}

func (s fsQueriesStore) saveQuery(folderPath string, query datatug.QueryDef, isNew bool) (err error) {
	if err := query.Validate(); err != nil {
		return fmt.Errorf("invalid query (isNew=%v): %w", isNew, err)
	}
	_, queryType, queryFileName, _, queryPath, err := getQueryPaths(path.Join(folderPath, query.ID), s.dirPath)
	if err != nil {
		return err
	}
	queryText := query.Text
	queryType = strings.ToLower(queryType)
	if queryType == "sql" {
		query.Text = ""
	}

	if err = saveJSONFile(path.Dir(queryPath), queryFileName, query); err != nil {
		return fmt.Errorf("failed to save query to json file: %w", err)
	}

	if queryType == "sql" {
		sqlFilePath := queryPath[:len(queryPath)-len(".json")]

		if err = os.WriteFile(sqlFilePath, []byte(queryText), 0644); err != nil {
			return fmt.Errorf("faile to write query text to .sql file: %w", err)
		}
	}

	return nil
}
