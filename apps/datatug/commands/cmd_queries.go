package commands

import (
	"context"
	cliv3 "github.com/urfave/cli/v3"
)

// queriesCommand returns the CLI command for managing queries
func queriesCommand() *cliv3.Command {
	return &cliv3.Command{
		Name:        "queries",
		Usage:       "Lists queries if no sub-consoleCommand provided",
		Description: "Lists queries if no sub-consoleCommand provided",
		Action:      queriesCommandAction,
	}
}

func queriesCommandAction(_ context.Context, _ *cliv3.Command) error {
	// Future implementation will go here; keeping the previous panic to preserve behavior
	panic("not implemented")
}
