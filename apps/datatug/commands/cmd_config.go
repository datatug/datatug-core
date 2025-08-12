package commands

import (
	"context"
	"fmt"
	"github.com/datatug/datatug/packages/appconfig"
	cliv3 "github.com/urfave/cli/v3"
	"os"
)

// configCommand prints whole DataTug config
type configCommand struct {
}

func (v *configCommand) Execute(_ []string) error {
	settings, err := appconfig.GetSettings()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}
	if err = appconfig.PrintSettings(settings, appconfig.FormatYaml, os.Stdout); err != nil {
		return err
	}
	return nil
}

func configCommandArgs() *cliv3.Command {
	return &cliv3.Command{
		Name:        "config",
		Usage:       "Prints config",
		Description: "",
		Action: func(ctx context.Context, c *cliv3.Command) error {
			cmd := &configCommand{}
			return cmd.Execute(nil)
		},
	}
}
