package commands

import (
	"context"
	cliv3 "github.com/urfave/cli/v3"
)

func queriesCommandArgs() *cliv3.Command {
	return &cliv3.Command{
		Name:        "queries",
		Usage:       "Lists queries if no sub-command provided",
		Description: "Lists queries if no sub-command provided",
		Action: func(ctx context.Context, c *cliv3.Command) error {
			v := &queriesCommand{}
			return v.Execute(nil)
		},
	}
}

// datasetsCommand defines parameters for test command
type queriesCommand struct {
	projectBaseCommand
	Folder string `short:"f" long:"folder"  required:"false" description:"Folder path"`
}

// Execute executes test command
func (v *queriesCommand) Execute([]string) error {
	//if err := v.initProjectCommand(projectCommandOptions{projNameOrDirRequired: true}); err != nil {
	//	return err
	//}
	//ctx := context.Background()
	//queries, err := v.store.GetProjectStore(v.projectID).Queries().LoadQueries(ctx, v.Folder)
	//if err != nil {
	//	return fmt.Errorf("failed to load datasets from [%v]: %w", v.ProjectDir, err)
	//}
	//encoder := yaml.NewEncoder(os.Stdout)
	//if err = encoder.Encode(queries); err != nil {
	//	return err
	//}
	//return nil
	panic("not implemented")
}
