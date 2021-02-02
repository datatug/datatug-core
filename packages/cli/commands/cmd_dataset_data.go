package commands

// datasetCommand defines parameters for validate command
type datasetDataCommand struct {
	datasetBaseCommand
}

// Execute executes validate command
func (v *datasetDataCommand) Execute([]string) error {
	if err := v.initProjectCommand(projectCommandOptions{projNameOrDirRequired: true}); err != nil {
		return err
	}
	// TODO: Implement "datasets show" command
	return nil
}
