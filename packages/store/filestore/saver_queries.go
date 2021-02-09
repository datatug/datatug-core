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

func (s fileSystemSaver) getQueryPaths(queryID string) (qID, queryFileName, queryDir, queryPath string, err error) {
	if strings.TrimSpace(queryID) == "" {
		return "", "", "", "", validation.NewErrRequestIsMissingRequiredField("queryID")
	}
	queryDir = filepath.Dir(queryID)
	qID = filepath.Base(queryID)
	queryFileName = fmt.Sprintf("%v.json", qID)
	queryPath = path.Join(s.queriesDirPath(), queryDir, queryFileName)
	return
}

func (s fileSystemSaver) DeleteQuery(queryID string) error {
	_, queryFileName, queryDir, queryPath, err := s.getQueryPaths(queryID)
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
	_, queryFileName, queryDir, queryPath, err := s.getQueryPaths(query.ID)

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
