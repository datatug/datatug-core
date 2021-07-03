package storage

import "github.com/datatug/datatug/packages/models"

type EnvServersStore interface {
	Server(id string) EnvServerStore
}

type EnvServerStore interface {
	Catalogs() EnvDbCatalogsStore
	LoadEnvServer() (*models.EnvDbServer, error)
	SaveEnvServer(envServer *models.EnvDbServer) error
}
