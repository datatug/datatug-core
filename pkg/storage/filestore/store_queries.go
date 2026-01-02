package filestore

import (
	"context"
	"errors"
	"path"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

func newFsQueriesStore(projectPath string) fsQueriesStore {
	return fsQueriesStore{
		fsProjectItemsStore: newFileProjectItemsStore[datatug.QueryDefs, *datatug.QueryDef, datatug.QueryDef](
			path.Join(projectPath, storage.QueriesFolder), storage.QueryFileSuffix,
		),
	}
}

var _ datatug.QueriesStore = (*fsQueriesStore)(nil)

type fsQueriesStore struct {
	fsProjectItemsStore[datatug.QueryDefs, *datatug.QueryDef, datatug.QueryDef]
}

func (s fsQueriesStore) LoadQueries(ctx context.Context, folderPath string, o ...datatug.StoreOption) (folder *datatug.QueriesFolder, err error) {
	_ = datatug.GetStoreOptions(o...)
	dirPath := path.Join(s.dirPath, folderPath)
	items, err := s.loadProjectItems(ctx, dirPath)
	if err != nil {
		return nil, err
	}
	folder = &datatug.QueriesFolder{
		Items: make(datatug.QueryDefs, len(items)),
	}
	copy(folder.Items, items)
	return folder, nil
}

func (s fsQueriesStore) LoadQuery(ctx context.Context, id string, o ...datatug.StoreOption) (query *datatug.QueryDef, err error) {
	ids := strings.Split(id, "/")
	folder := path.Join(ids[:len(ids)-1]...)
	dirPath := path.Join(s.dirPath, folder)
	id = ids[len(ids)-1]
	query, err = s.loadProjectItem(ctx, dirPath, id, "", o...)
	return query, err
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
	_ = folderPath
	return errors.New("not implemented yet")
}

func (s fsQueriesStore) SaveQuery(ctx context.Context, query *datatug.QueryDefWithFolderPath) error {
	return s.saveProjectItem(ctx, s.dirPath, &query.QueryDef)
}
