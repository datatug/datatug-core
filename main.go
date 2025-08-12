package main

import (
	"context"
	_ "embed"
	"github.com/datatug/datatug/apps/datatug/commands"
	_ "github.com/denisenkom/go-mssqldb"
	_ "github.com/mattn/go-sqlite3"
	cliv3 "github.com/urfave/cli/v3"
	"log"
	"os"
	"strings"
)

func main() {
	cmd := getCommand()
	args := os.Args
	// When running under `go test`, os.Args contains testing flags that urfave/cli doesn't recognize.
	// Detect test binary by suffix and strip args to avoid parsing test flags.
	if len(args) > 0 && strings.HasSuffix(args[0], ".test") {
		args = args[:1]
	}
	if err := cmd.Run(context.Background(), args); err != nil {
		log.Fatal(err)
	}
	//var p = getParser()
	//if _, err := p.Parse(); err != nil {
	//	var flagsErr *flags.Error
	//	switch {
	//	case errors.As(err, &flagsErr):
	//		if errors.Is(flagsErr.Type, flags.ErrHelp) {
	//			os.Exit(0)
	//		}
	//		os.Exit(1)
	//	default:
	//		_, _ = fmt.Fprintf(os.Stderr, "failed to execute command: %s", err)
	//		os.Exit(1)
	//	}
	//}
}

var getCommand = func() *cliv3.Command {
	return commands.GetCommand()
}
