package main

import (
	_ "embed"
	"errors"
	"fmt"
	"github.com/datatug/datatug/apps/datatug/commands"
	_ "github.com/denisenkom/go-mssqldb"
	"github.com/jessevdk/go-flags"
	_ "github.com/mattn/go-sqlite3"
	"os"
)

type parser interface {
	Parse() ([]string, error)
}

func main() {
	var p = getParser()
	if _, err := p.Parse(); err != nil {
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

var getParser = func() parser {
	return commands.GetParser()
}
