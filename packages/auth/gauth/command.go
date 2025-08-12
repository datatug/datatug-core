package gauth

import (
	"context"
	"github.com/urfave/cli/v3"
)

func GoogleAuthCommand() *cli.Command {
	return &cli.Command{
		Name:        "google",
		Description: "Manages authentication with Google",
		Action: func(ctx context.Context, command *cli.Command) error {
			return nil
		},
	}
}
