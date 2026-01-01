package dtprojcreator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"gopkg.in/yaml.v3"
)

var yamlMarshal = yaml.Marshal

type creator struct {
	ctx          context.Context
	p            *datatug.Project
	projPath     string
	s            Storage
	reportStatus datatug.StatusReporter
}

func CreateProjectFiles(
	ctx context.Context,
	p *datatug.Project,
	projPath string,
	fs Storage,
	reportStatus datatug.StatusReporter,
) error {
	c := creator{ctx: ctx, s: fs, p: p, projPath: projPath, reportStatus: reportStatus}

	var wg sync.WaitGroup
	var mutex sync.Mutex
	var errs []error

	type worker struct {
		name string
		fn   func() error
	}

	execute := func(workers ...worker) {
		wg.Add(len(workers))
		for _, w := range workers {
			go func(w worker) {
				defer wg.Done()
				reportStatus(w.name, "...")
				if fErr := w.fn(); fErr != nil {
					mutex.Lock()
					errs = append(errs, fErr)
					mutex.Unlock()
					reportStatus(w.name, fmt.Sprintf(" - failed: %s", fErr.Error()))
					return
				}
				reportStatus(w.name, ". [green]Done![-]")
			}(w)
		}
	}

	execute( // TODO: reuse parallel runner or document why not?
		worker{
			name: "Add project to .datatug.yaml",
			fn: func() error {
				return c.addProjectToRootRepoFile()
			},
		},
		worker{
			name: "Creating project README.md",
			fn: func() error {
				return c.createProjectReadmeMD()
			},
		},
		worker{
			name: "Creating .datatug-project.json",
			fn: func() error {
				return c.createProjectSummaryFile()
			},
		},
	)

	wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("%d errors: %w", len(errs), errors.Join(errs...))
	}
	return nil
}

const ProjectReadmeContent = `# DataTug Project

This directory contains DataTug project configuration.`

func (c creator) createProjectReadmeMD() error {
	filePath := path.Join(c.projPath, "README.md")
	reader := io.NopCloser(strings.NewReader(ProjectReadmeContent))
	return c.s.WriteFile(c.ctx, filePath, reader)
}

func (c creator) createProjectSummaryFile() error {
	projectFile := datatug.ProjectFile{
		ProjectItem: c.p.ProjectItem,
		Created: &datatug.ProjectCreated{
			At: time.Now().UTC(),
		},
	}
	content, err := yamlMarshal(projectFile)
	if err != nil {
		return err
	}
	return c.writeFile(storage.ProjectSummaryFileName, content)
}

func (c creator) addProjectToRootRepoFile() error {
	var repoRootFile datatug.RepoRootFile
	repoRootFile.Projects = append(repoRootFile.Projects, c.projPath)
	content, err := yamlMarshal(repoRootFile)
	if err != nil {
		return fmt.Errorf("failed to marshal repoRootFile: %w", err)
	}
	return c.writeFile(storage.RepoRootDataTugFileName, content)
}

func (c creator) writeFile(name string, content []byte) error {
	filePath := path.Join(c.projPath, name)
	reader := io.NopCloser(bytes.NewReader(content))
	return c.s.WriteFile(c.ctx, filePath, reader)
}
