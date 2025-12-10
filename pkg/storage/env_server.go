package storage

import "github.com/datatug/datatug-core/pkg/datatug"

type EnvServersStore interface {
	Server(id string) EnvServerStore
}

type EnvServerStore interface {
	Catalogs() EnvDbCatalogsStore
	LoadEnvServer() (*datatug.EnvDbServer, error)
	SaveEnvServer(envServer *datatug.EnvDbServer) error
}
