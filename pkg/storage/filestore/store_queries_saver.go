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

type QueryLoc struct {
	LocalID  string `json:"id"`
	Folder   string `json:"folder"`
	FileName string `json:"fileName"`
	Path     string `json:"path"`
}

func getQueryPaths(queryID, queriesDirPath string) (ql QueryLoc, err error) {
	//(
	//qID, // without directory and .json extension
	//queryFileName,
	//queryDir,
	//queryPath string, // Full file name
	//err error,
	//)

	if strings.TrimSpace(queryID) == "" {
		err = validation.NewErrRequestIsMissingRequiredField("queryID")
		return
	}

	ql.Folder = filepath.Dir(queryID)
	ql.LocalID = filepath.Base(queryID)
	//lastDotIndex := strings.LastIndex(ql.LocalID, ".")
	//ql.LocalID = ql.LocalID[:lastDotIndex]
	ql.FileName = jsonFileName(ql.LocalID, "")
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
	if err = query.Validate(); err != nil {
		return fmt.Errorf("invalid query (isNew=%v): %w", isNew, err)
	}
	var ql QueryLoc
	if ql, err = getQueryPaths(path.Join(folderPath, query.ID), s.dirPath); err != nil {
		return err
	}
	queryText := query.Text

	if err = saveJSONFile(path.Join(s.dirPath, ql.Folder), ql.FileName, query); err != nil {
		return fmt.Errorf("failed to save query to json file: %w", err)
	}

	if query.Text != "" {
		fileExt := strings.ToLower(string(query.Type))
		queryTextFilePath := path.Join(s.dirPath, ql.Folder, query.ID+"."+fileExt)

		if err = os.WriteFile(queryTextFilePath, []byte(queryText), 0644); err != nil {
			return fmt.Errorf("failed to write query text to file %s: %w", queryTextFilePath, err)
		}
	}

	return nil
}
