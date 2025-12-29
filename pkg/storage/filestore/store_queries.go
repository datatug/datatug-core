package filestore

import (
	"context"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// var _ storage.QueriesStore = (*fsQueriesStore)(nil)

func newFsQueriesStore(projectPath string) fsQueriesStore {
	return fsQueriesStore{
		fsProjectItemsStore: newFsProjectItemsStore[datatug.QueryDefs, *datatug.QueryDef, datatug.QueryDef](
			path.Join(projectPath, QueriesFolder), querySQLFileSuffix,
		),
	}
}

type fsQueriesStore struct {
	fsProjectItemsStore[datatug.QueryDefs, *datatug.QueryDef, datatug.QueryDef]
}

func (s fsQueriesStore) LoadQueries(ctx context.Context, folderPath string) (folder *datatug.QueryFolder, err error) {
	dirPath := path.Join(s.dirPath, folderPath)
	items, err := s.loadProjectItems(ctx, dirPath)
	if err != nil {
		return nil, err
	}
	folder = &datatug.QueryFolder{
		Items: make(datatug.QueryDefs, len(items)),
	}
	copy(folder.Items, items)
	return folder, nil
}

func (s fsQueriesStore) GetQuery(ctx context.Context, id string) (query *datatug.QueryDefWithFolderPath, err error) {
	qID, _, queryFileName, queryDir, _, err := getQueryPaths(id, s.dirPath)
	if err != nil {
		return nil, err
	}
	queryDef, err := s.loadProjectItem(ctx, path.Join(queryDir, queryFileName), qID, "")
	if err != nil {
		return nil, err
	}
	return &datatug.QueryDefWithFolderPath{
		QueryDef: *queryDef,
	}, nil
}

func (s fsQueriesStore) UpdateQuery(ctx context.Context, query datatug.QueryDef) (q *datatug.QueryDefWithFolderPath, err error) {
	err = s.saveProjectItem(ctx, s.dirPath, &query)
	if err != nil {
		return nil, err
	}
	return &datatug.QueryDefWithFolderPath{
		QueryDef: query,
	}, nil
}

func (s fsQueriesStore) DeleteQuery(ctx context.Context, id string) (err error) {
	return s.deleteProjectItem(ctx, s.dirPath, id)
}

func (s fsQueriesStore) DeleteQueryFolder(_ context.Context, folderPath string) error {
	// This might need more implementation if we support folders
	return nil
}

func (s fsQueriesStore) LoadQuery(ctx context.Context, id string) (*datatug.QueryDefWithFolderPath, error) {
	return s.GetQuery(ctx, id)
}

func (s fsQueriesStore) SaveQuery(ctx context.Context, query *datatug.QueryDefWithFolderPath) error {
	return s.saveProjectItem(ctx, s.fsProjectItemsStore.dirPath, &query.QueryDef)
}
