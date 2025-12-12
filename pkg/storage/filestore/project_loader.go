package filestore

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/datatug/datatug-core/pkg/datatug"
)

func NewProjectsLoader(projectsDirPath string) datatug.ProjectsLoader {
	return &projectsLoader{projectsDirPath: projectsDirPath}
}

func NewProjectLoader(projectDir string) datatug.ProjectLoader {
	return &projectLoader{projectDir: projectDir}
}

type projectsLoader struct {
	projectsDirPath string
}

func (l projectsLoader) LoadProject(_ context.Context, projectID string) (project *datatug.Project, err error) {
	projectDir := ExpandHome(filepath.Join(l.projectsDirPath, projectID))
	project = datatug.NewProject(projectID, projectLoader{projectDir: projectDir})
	project.ID = projectID
	if err = loadProjectFile(projectDir, project); err != nil {
		err = fmt.Errorf("failed to load project: %w", err)
	}
	return
}

type projectLoader struct {
	projectDir string
}

func (loader projectLoader) LoadEnvironments(_ context.Context) (datatug.Environments, error) {
	return loadEnvironments(loader.projectDir)
}

func (loader projectLoader) LoadDbServers(_ context.Context) (datatug.ProjDbServers, error) {
	//TODO implement me
	panic("implement me")
}
