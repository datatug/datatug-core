package firestoreviewer

import (
	"github.com/datatug/datatug/packages/cli"
)

func AddFirestoreCommand(p cli.Parser) {
	_, _ = p.AddCommand("firestore", "runs Firestore Viewer", "runs Firestore Viewer",
		&firestoreCommand{})
}

type firestoreCommand struct {
}

func (v *firestoreCommand) Execute(_ []string) error {
	Run()
	return nil
}
