package main

import (
	"context"
	cliv3 "github.com/urfave/cli/v3"
	"testing"
)

func TestMainFunc(t *testing.T) {
	t.Run("getCommand_no_error", func(t *testing.T) {
		getCommand = func() *cliv3.Command {
			return &cliv3.Command{Action: func(ctx context.Context, c *cliv3.Command) error { return nil }}
		}
		main()
	})
	t.Run("getCommand_nil", func(t *testing.T) {
		getCommand = func() *cliv3.Command { return nil }
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic")
			}
		}()
		main()
	})
}
