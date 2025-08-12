package firestoreviewer

import (
	"context"
	"github.com/urfave/cli/v3"
)

func FirestoreCommand() *cli.Command {
	return &cli.Command{
		Name:        "firestore",
		Aliases:     []string{"fs"},
		Usage:       "View and edit data in Firestore databases",
		Description: "Firestore Viewer allows you to view & edit Firestore databases.",
		Action: func(ctx context.Context, command *cli.Command) error {
			Run()
			return nil
		},
	}
}
