package storage

import "github.com/datatug/datatug/packages/models"

type EntitiesStore interface {
	ProjectStoreRef
	Entity(id string) EntityStore
	// LoadEntities loads entities
	LoadEntities() (entities models.Entities, err error)
}

// EntityStore defines DAL for entities
type EntityStore interface {
	ID() string
	Entities() EntitiesStore
	// LoadEntity loads entity
	LoadEntity() (entity *models.Entity, err error)
	DeleteEntity() (err error)
	SaveEntity(entity *models.Entity) (err error)
}
