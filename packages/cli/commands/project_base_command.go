package commands

import (
	"errors"
	config2 "github.com/datatug/datatug/packages/cli/config"
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
	store       storage.Store
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
	config, err := config2.GetSettings()
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
		v.store, v.projectID = filestore.NewSingleProjectStore(v.ProjectDir, v.projectID)
	} else {
		pathsByID := getProjPathsByID(config)
		v.store, err = filestore.NewStore("local_file_store_from_user_config", pathsByID)
		if err != nil {
			return err
		}
	}

	return nil
}
