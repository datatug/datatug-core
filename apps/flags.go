package apps

import "github.com/urfave/cli/v3"

var TUIFlag cli.Flag = &cli.BoolFlag{
	Name:    "tui",
	Aliases: []string{"t"},
	Usage:   "Start terminal UI",
}
