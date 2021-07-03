package filestore

import (
	"github.com/datatug/datatug/packages/storage"
	"path"
)

var _ storage.EnvServersStore = (*fsEnvServersStore)(nil)

type fsEnvServersStore struct {
	envServersPath string
	fsEnvironmentStore
}

func newFsEnvServersStore(fsEnvironmentStore fsEnvironmentStore) fsEnvServersStore {
	return fsEnvServersStore{
		envServersPath:     path.Join(fsEnvironmentStore.envPath, ServersFolder, DbFolder),
		fsEnvironmentStore: fsEnvironmentStore,
	}
}

func (store fsEnvServersStore) Server(id string) storage.EnvServerStore {
	return store.server(id)
}

func (store fsEnvServersStore) server(id string) fsEnvServerStore {
	return newFsEnvServerStore(id, store)
}
