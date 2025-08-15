package storage

import (
	"context"
	"github.com/datatug/datatug-core/pkg/models"
)

type EntitiesStore interface {
	ProjectStoreRef
	Entity(id string) EntityStore
	// LoadEntities loads entities
	LoadEntities(ctx context.Context) (entities models.Entities, err error)
}

// EntityStore defines DAL for entities
type EntityStore interface {
	ID() string
	Entities() EntitiesStore
	// LoadEntity loads entity
	LoadEntity(ctx context.Context) (entity *models.Entity, err error)
	DeleteEntity(ctx context.Context) (err error)
	SaveEntity(ctx context.Context, entity *models.Entity) (err error)
}
