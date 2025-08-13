package commands

import (
	"context"
	"github.com/urfave/cli/v3"
)

// queriesCommand returns the CLI command for managing queries
func queriesCommand() *cli.Command {
	return &cli.Command{
		Name:        "queries",
		Usage:       "Lists queries if no sub-consoleCommand provided",
		Description: "Lists queries if no sub-consoleCommand provided",
		Action:      queriesCommandAction,
	}
}

func queriesCommandAction(_ context.Context, _ *cli.Command) error {
	// Future implementation will go here; keeping the previous panic to preserve behavior
	panic("not implemented")
}
