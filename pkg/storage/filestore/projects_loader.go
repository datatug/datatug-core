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

type projectsLoader struct {
	projectsDirPath string
}

func (l projectsLoader) LoadProject(_ context.Context, projectID string) (project *datatug.Project, err error) {
	projectDir := ExpandHome(filepath.Join(l.projectsDirPath, projectID))
	project = datatug.NewProject(projectID, func(p *datatug.Project) datatug.ProjectStore {
		return newFsProjectStore(projectID, projectDir)
	})
	project.ID = projectID
	if err = loadProjectFile(projectDir, project); err != nil {
		err = fmt.Errorf("failed to load project file: %w", err)
	}
	return
}
