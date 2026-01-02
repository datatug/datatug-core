package filestore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

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

	queryText := query.Text
	query.Text = ""
	defer func() {
		query.Text = queryText
	}()

	queryDirPath := path.Join(s.dirPath, folderPath)

	jsonFileName := fmt.Sprintf("%s.%s.json", query.ID, storage.QueryFileSuffix)
	if err = saveJSONFile(queryDirPath, jsonFileName, query); err != nil {
		return fmt.Errorf("failed to save query to json file: %w", err)
	}

	if queryText != "" {
		fileExt := strings.ToLower(string(query.Type))
		fileName := fmt.Sprintf("%s.%s.%s", query.ID, storage.QueryFileSuffix, fileExt)
		filePath := path.Join(queryDirPath, fileName)

		if err = os.WriteFile(filePath, []byte(queryText), 0644); err != nil {
			return fmt.Errorf("failed to write query text to file %s: %w", filePath, err)
		}
	}
	return nil
}
