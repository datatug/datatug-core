package storage

import (
	"context"
	"github.com/datatug/datatug/packages/models"
)

// Store defines interface for loading & saving DataTug projects
type Store interface {
	ID() string
	// GetProjects returns list of projects
	GetProjects(ctx context.Context) (projectBriefs []models.ProjectBrief, err error)
	// Project returns project store
	Project(id string) ProjectStore
}
