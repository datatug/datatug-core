package commands

import (
	"context"
	"fmt"
	cliv3 "github.com/urfave/cli/v3"
	"os"
)

func consoleCommandArgs() *cliv3.Command {
	return &cliv3.Command{
		Name:        "console",
		Usage:       "Starts interactive console",
		Description: "Starts interactive console with autocomplete",
		Action: func(ctx context.Context, c *cliv3.Command) error {
			v := &command{}
			return v.Execute(nil)
		},
	}
}

// command defines parameters for console command
type command struct {
}

// Execute executes serve command
func (v *command) Execute(_ []string) (err error) {
	if err = os.Setenv("GO_FLAGS_COMPLETION", "1"); err != nil {
		return err
	}
	_, _ = fmt.Println("To be implemented")
	return nil
}
