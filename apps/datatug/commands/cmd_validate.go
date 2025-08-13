package commands

import (
	"context"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/urfave/cli/v3"
)

func testCommandArgs() *cli.Command {
	return &cli.Command{
		Name:        "test",
		Usage:       "Runs validation scripts",
		Description: "The `test` consoleCommand executes validation scripts.",
		Action: func(ctx context.Context, c *cli.Command) error {
			v := &validateCommand{}
			return v.Execute(nil)
		},
	}
}

// validateCommand defines parameters for test consoleCommand
type validateCommand struct {
	projectBaseCommand
}

// Execute executes test consoleCommand
func (v *validateCommand) Execute(_ []string) (err error) {
	if err = v.initProjectCommand(projectCommandOptions{projNameOrDirRequired: true}); err != nil {
		return err
	}

	var project *models.DatatugProject
	if project, err = v.store.GetProjectStore(v.projectID).LoadProject(context.Background()); err != nil {
		return fmt.Errorf("failed to load project from [%v]: %w", v.ProjectDir, err)
	}
	fmt.Println("Validating loaded project...")
	if err := project.Validate(); err != nil {
		return err
	}
	fmt.Println("GetProjectStore is valid.")
	return nil
}
