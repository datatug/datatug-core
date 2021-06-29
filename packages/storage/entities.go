package storage

import "github.com/datatug/datatug/packages/models"

// EntitiesStore defines DAL for entities
type EntitiesStore interface {
	Loader() EntitiesLoader
	Saver() EntitiesSaver
}

// EntitiesSaver saves entity
type EntitiesSaver interface {
	DeleteEntity(id string) (err error)
	SaveEntity(entity *models.Entity) (err error)
}

// EntitiesLoader loads entities
type EntitiesLoader interface {
	// LoadEntity loads entity
	LoadEntity(id string) (entity models.Entity, err error)
	// LoadEntities loads entities
	LoadEntities() (entities models.Entities, err error)
}
