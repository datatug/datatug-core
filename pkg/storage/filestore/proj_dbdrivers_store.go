package filestore

import (
	"context"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/parallel"
	"github.com/datatug/datatug-core/pkg/storage"
)

var _ datatug.ProjDbDriversStore = (*fsProjDbDriversStore)(nil)

func newFsProjDbDriversStore(projectPath string) fsProjDbDriversStore {
	return fsProjDbDriversStore{
		fsProjectItemsStore: newDirProjectItemsStore[datatug.ProjDbDrivers, *datatug.ProjDbDriver, datatug.ProjDbDriver](
			path.Join(projectPath, storage.DbsFolder), "",
		),
	}
}

type fsProjDbDriversStore struct {
	fsProjectItemsStore[datatug.ProjDbDrivers, *datatug.ProjDbDriver, datatug.ProjDbDriver]
}

func (s fsProjDbDriversStore) DbServersStore(driverID string) datatug.ProjDbServersStore {
	return newFsProjDbServersStore(s.dirPath, driverID)
}

func (s fsProjDbDriversStore) LoadProjDbDrivers(ctx context.Context, o ...datatug.StoreOption) (dbs datatug.ProjDbDrivers, err error) {
	dbs, err = s.loadProjectItems(ctx, s.dirPath, o...)
	if len(dbs) > 0 {
		workers := make([]func() error, len(dbs))
		for i, db := range dbs {
			workers[i] = func() (err error) {
				dbStore := s.DbServersStore(db.ID)
				db.Servers, err = dbStore.LoadProjDbServers(ctx, o...)
				return
			}
		}
		err = parallel.Run(workers...)
	}
	return
}

func (s fsProjDbDriversStore) LoadProjDbDriver(ctx context.Context, id string, o ...datatug.StoreOption) (*datatug.ProjDbDriver, error) {
	return s.loadProjectItem(ctx, s.dirPath, id, "", o...)
}

func (s fsProjDbDriversStore) SaveProjDbDriver(ctx context.Context, dbDriver *datatug.ProjDbDriver, o ...datatug.StoreOption) error {
	return s.saveProjectItem(ctx, s.dirPath, dbDriver, o...)
}

func (s fsProjDbDriversStore) DeleteProjDbDriver(ctx context.Context, id string) error {
	return s.deleteProjectItem(ctx, s.dirPath, id)
}
