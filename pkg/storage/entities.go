package storage

import (
	"context"

	"github.com/datatug/datatug-core/pkg/datatug"
)

type EntitiesStore interface {
	ProjectStoreRef
	Entity(id string) EntityStore
	// LoadEntities loads entities
	LoadEntities(ctx context.Context) (entities datatug.Entities, err error)
}

// EntityStore defines DAL for entities
type EntityStore interface {
	ID() string
	Entities() EntitiesStore
	// LoadEntity loads entity
	LoadEntity(ctx context.Context) (entity *datatug.Entity, err error)
	DeleteEntity(ctx context.Context) (err error)
	SaveEntity(ctx context.Context, entity *datatug.Entity) (err error)
}
