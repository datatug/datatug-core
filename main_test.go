package main

import (
	"context"
	"github.com/urfave/cli/v3"
	"testing"
)

func TestMainFunc(t *testing.T) {
	t.Run("getCommand_no_error", func(t *testing.T) {
		getCommand = func() *cli.Command {
			return &cli.Command{Action: func(ctx context.Context, c *cli.Command) error { return nil }}
		}
		main()
	})
	t.Run("getCommand_nil", func(t *testing.T) {
		getCommand = func() *cli.Command { return nil }
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic")
			}
		}()
		main()
	})
}
