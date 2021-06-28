package main

import (
	_ "embed"
	"github.com/datatug/datatug/packages/cli/commands"
	_ "github.com/datatug/datatug/packages/cli/console"
	_ "github.com/denisenkom/go-mssqldb"
	_ "github.com/mattn/go-sqlite3"
	"os"
)

func main() {
	if _, err := commands.Parser.Parse(); err != nil {
		switch flagsErr := err.(type) {
		case *flags.Error:
			if flagsErr.Type == flags.ErrHelp {
				os.Exit(0)
			}
			os.Exit(1)
		default:
			os.Exit(1)
		}
	}
}
