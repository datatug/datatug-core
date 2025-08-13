package commands

import (
	"context"
	"fmt"
	"github.com/urfave/cli/v3"
	"os"
)

func consoleCommandArgs() *cli.Command {
	return &cli.Command{
		Name:        "console",
		Usage:       "Starts interactive console",
		Description: "Starts interactive console with autocomplete",
		Action: func(ctx context.Context, c *cli.Command) error {
			v := &consoleCommand{}
			return v.Execute(nil)
		},
	}
}

// consoleCommand defines parameters for console consoleCommand
type consoleCommand struct {
}

// Execute executes serve consoleCommand
func (v *consoleCommand) Execute(_ []string) (err error) {
	if err = os.Setenv("GO_FLAGS_COMPLETION", "1"); err != nil {
		return err
	}
	_, _ = fmt.Println("To be implemented")
	return nil
}
