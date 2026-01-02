package filestore

import (
	"context"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

var _ datatug.DbCatalogsStore = (*fsDbCatalogsStore)(nil)

type fsDbCatalogsStore struct {
	serverRef datatug.ServerRef
	fsProjectItemsStore[datatug.DbCatalogs, *datatug.DbCatalog, datatug.DbCatalog]
}

func (f fsDbCatalogsStore) Server() datatug.ServerRef {
	return f.serverRef
}

func (f fsDbCatalogsStore) LoadDbCatalogs(ctx context.Context, o ...datatug.StoreOption) (datatug.DbCatalogs, error) {
	return f.loadProjectItems(ctx, f.dirPath, o...)
}

func (f fsDbCatalogsStore) SaveDbCatalog(ctx context.Context, catalog *datatug.DbCatalog) error {
	return f.saveProjectItem(ctx, f.dirPath, catalog)
}

func (f fsDbCatalogsStore) DeleteDbCatalog(ctx context.Context, id string) error {
	return f.deleteProjectItem(ctx, f.dirPath, id)
}

func newFsDbCatalogsStore(dbServersPath string, serverRef datatug.ServerRef) fsDbCatalogsStore {
	return fsDbCatalogsStore{
		serverRef: serverRef,
		fsProjectItemsStore: newFileProjectItemsStore[datatug.DbCatalogs, *datatug.DbCatalog, datatug.DbCatalog](
			path.Join(dbServersPath, storage.ServersFolder, storage.DbsFolder), ""),
	}
}
