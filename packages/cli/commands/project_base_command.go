package commands

import (
	"errors"
	"github.com/datatug/datatug/packages/storage"
	"github.com/datatug/datatug/packages/storage/filestore"
	"strings"
)

type projectDirCommand struct {
	ProjectDir string `short:"d" long:"directory"  required:"false" description:"Project directory"`
}

// ProjectBaseCommand defines parameters for show project command
type projectBaseCommand struct {
	projectDirCommand
	ProjectName string `short:"p" long:"project"  required:"false" description:"Project name"`
	projectID   string
	loader      storage.Loader
}

type projectCommandOptions struct {
	projNameRequired, projDirRequired, projNameOrDirRequired bool
}

func (v *projectBaseCommand) initProjectCommand(o projectCommandOptions) error {
	if o.projNameRequired && v.ProjectName == "" {
		return errors.New("project name parameter is required")
	}
	if o.projDirRequired && v.ProjectDir == "" {
		return errors.New("project name parameter is required")
	}
	if o.projNameOrDirRequired && v.ProjectName == "" && v.ProjectDir == "" {
		return errors.New("either project name or project directory is required")
	}
	config, err := getConfig()
	if err != nil {
		return err
	}
	if v.ProjectName != "" {
		v.projectID = strings.ToLower(v.ProjectName)
		project, ok := config.Projects[v.projectID]
		if !ok {
			return ErrUnknownProjectName
		}
		v.ProjectDir = project.Path
	}
	if v.ProjectDir != "" && v.projectID == "" {
		v.loader, v.projectID = filestore.NewSingleProjectLoader(v.ProjectDir)
	} else {
		pathsByID := getProjPathsByID(config)
		v.loader, err = filestore.NewStore(pathsByID)
		if err != nil {
			return err
		}
	}

	return nil
}
