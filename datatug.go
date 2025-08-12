package main

import (
	_ "embed"
	"errors"
	"fmt"
	"github.com/datatug/datatug/packages/cli/commands"
	_ "github.com/datatug/datatug/packages/cli/console"
	_ "github.com/denisenkom/go-mssqldb"
	"github.com/jessevdk/go-flags"
	_ "github.com/mattn/go-sqlite3"
	"os"
)

func main() {
	if _, err := commands.Parser.Parse(); err != nil {
		var flagsErr *flags.Error
		switch {
		case errors.As(err, &flagsErr):
			if errors.Is(flagsErr.Type, flags.ErrHelp) {
				os.Exit(0)
			}
			os.Exit(1)
		default:
			_, _ = fmt.Fprintf(os.Stderr, "failed to execute command: %s", err)
			os.Exit(1)
		}
	}
}
