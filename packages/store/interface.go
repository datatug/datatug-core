package store

import "github.com/datatug/datatug/packages/models"

// Interface defines interface for loading & saving DataTug projects
type Interface interface {
	Loader
	Saver
	
	// GetProjects returns list of projects
	GetProjects() (projectBriefs []models.ProjectBrief, err error)
}
