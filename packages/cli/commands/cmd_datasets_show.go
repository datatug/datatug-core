package commands

// datasetsCommand defines parameters for validate command
type showDatasetsCommand struct {
	projectBaseCommand
	Dataset string `long:"dataset" required:"true"`
}

// Execute executes validate command
func (v *showDatasetsCommand) Execute([]string) error {
	if err := v.initProjectCommand(projectCommandOptions{projNameOrDirRequired: true}); err != nil {
		return err
	}
	// TODO: Implement "datasets show" command
	return nil
}
