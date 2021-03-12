package filestore

import (
	"encoding/json"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/strongo/validation"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func getQueryPaths(queryID, queriesDirPath string) (qID, queryFileName, queryDir, queryPath string, err error) {
	if strings.TrimSpace(queryID) == "" {
		return "", "", "", "", validation.NewErrRequestIsMissingRequiredField("queryID")
	}
	queryDir = filepath.Dir(queryID)
	qID = filepath.Base(queryID)
	lastDotIndex := strings.LastIndex(queryID, ".")
	queryType := queryID[lastDotIndex+1:]
	qID = queryID[:lastDotIndex]
	queryFileName = jsonFileName(qID, queryType)
	queryPath = path.Join(queriesDirPath, queryDir, queryFileName)
	return
}

func (s fileSystemSaver) DeleteQuery(queryID string) error {
	_, queryFileName, queryDir, queryPath, err := getQueryPaths(queryID, queriesDirPath(s.projDirPath))
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

func (s fileSystemSaver) CreateQuery(query models.QueryDef) (err error) {
	return s.saveQuery(query, true)
}

func (s fileSystemSaver) saveQuery(query models.QueryDef, isNew bool) (err error) {
	if err := query.Validate(); err != nil {
		return fmt.Errorf("invalid query: %w", err)
	}
	_, queryFileName, queryDir, queryPath, err := getQueryPaths(query.ID, queriesDirPath(s.projDirPath))

	if isNew {
		if _, err := os.Stat(queryPath); err == nil {
			return fmt.Errorf("query already exists: %v", query.ID)
		}
	}
	file, err := os.Create(queryPath)
	if err != nil {
		return fmt.Errorf("failed to open query file for writing: %v: %w", path.Join(queryDir, queryFileName), err)
	}
	defer func() {
		_ = file.Close()
	}()
	encoder := json.NewEncoder(file)
	if err := encoder.Encode(query); err != nil {
		return err
	}
	return nil
}
