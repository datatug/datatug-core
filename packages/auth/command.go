package auth

import (
	"github.com/datatug/datatug/packages/auth/gauth"
	"github.com/urfave/cli/v3"
)

func AuthCommand() *cli.Command {
	return &cli.Command{
		Name: "auth",
		Commands: []*cli.Command{
			gauth.GoogleAuthCommand(),
		},
	}
}
