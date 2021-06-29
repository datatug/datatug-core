package storage

import "github.com/datatug/datatug/packages/models"

// Store defines interface for loading & saving DataTug projects
type Store interface {
	// GetProjects returns list of projects
	GetProjects() (projectBriefs []models.ProjectBrief, err error)
	// Project returns project store
	Project(id string) ProjectStore
}
