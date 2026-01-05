package filestore

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/parallel"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/strongo/validation"
)

// fileSystemLoader implements storage.Loader interface
type fileSystemLoader struct {
	pathByID map[string]string
}

// LoadProject loads project
func (s fsProjectStore) LoadProject(ctx context.Context, o ...datatug.StoreOption) (*datatug.Project, error) {
	opts := datatug.GetStoreOptions(o...)
	project := new(datatug.Project)
	if err := loadProjectFile(s.projectPath, project); err != nil {
		return nil, fmt.Errorf("failed to load project file: %w", err)
	}
	if opts.Depth() == 0 || opts.Depth() > 1 {
		o = opts.Next().ToSlice()
		if err := parallel.Run(
			func() error {
				environments, err := s.LoadEnvironments(ctx, o...)
				if err != nil {
					return fmt.Errorf("failed to load environments: %w", err)
				}
				project.Environments = environments
				return nil
			},
			func() error {
				entities, err := s.LoadEntities(ctx, o...)
				if err != nil {
					return fmt.Errorf("failed to load entities: %v", err)
				}
				project.Entities = entities
				return nil
			},
			func() error {
				boards, err := s.LoadBoards(ctx, o...)
				if err != nil {
					return fmt.Errorf("failed to load boards: %w", err)
				}
				project.Boards = boards
				return nil
			},
			func() error {
				dbModels, err := s.LoadDbModels(ctx, o...)
				if err != nil {
					return fmt.Errorf("failed to load db models: %v", err)
				}
				project.DbModels = dbModels
				return err
			},
			func() error {
				projDbDrivers, err := s.LoadProjDbDrivers(ctx, o...)
				if err != nil {
					return fmt.Errorf("failed to load db servers: %w", err)
				}
				project.DbDrivers = projDbDrivers
				return nil
			},
		); err != nil {
			err = fmt.Errorf("failed to load project by GetID=[%v]: %w", s.projectID, err)
			return nil, err
		}
	}

	return project, nil
}

// LoadProjectSummary loads project summary
func (s fsProjectStore) LoadProjectFile(context.Context) (projectFile datatug.ProjectFile, err error) {
	if projectFile, err = LoadProjectFile(s.projectPath); err != nil {
		return projectFile, fmt.Errorf("failed to load project file: %w", err)
	}
	projectFile.ID = s.projectID
	return
}

// LoadProjectFile loads project file
func LoadProjectFile(projPath string) (v datatug.ProjectFile, err error) {
	fileName := path.Join(projPath, storage.ProjectSummaryFileName)
	if err = readJSONFile(fileName, true, &v); os.IsNotExist(err) {
		err = fmt.Errorf("%w: %v", datatug.ErrProjectDoesNotExist, err)
	}
	return
}

func (loader fileSystemLoader) GetFolderPath(projectID string, folder ...string) (folderPath string, err error) {
	_, projectPath, err := loader.GetProjectPath(projectID)
	if err != nil {
		return "", err
	}
	return path.Join(projectPath, path.Join(folder...)), nil
}

// GetProjectPath returns project projDirPath by project GetID
func (loader fileSystemLoader) GetProjectPath(projectID string) (projID string, projPath string, err error) {
	if projectID == "" && len(projectPaths) == 1 {
		projID = storage.SingleProjectID
	} else {
		projID = projectID
	}
	projPath, knownProjectID := loader.pathByID[projID]
	if !knownProjectID {
		err = validation.NewErrBadRequestFieldValue("projectID", fmt.Sprintf("unknown: [%v]ro", projectID))
		return
	}
	return
}
