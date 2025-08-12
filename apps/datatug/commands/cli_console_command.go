package commands

import (
	"fmt"
	"github.com/datatug/datatug/packages/cli"
	"log"
	"os"
)

func consoleCommandArgs(p cli.Parser) {
	_, err := p.AddCommand("console",
		"Starts interactive console",
		"Starts interactive console with autocomplete",
		&command{})
	if err != nil {
		log.Fatal(err)
	}
}

// command defines parameters for console command
type command struct {
}

// Execute executes serve command
func (v *command) Execute(_ []string) (err error) {
	if err = os.Setenv("GO_FLAGS_COMPLETION", "1"); err != nil {
		return err
	}
	_, _ = fmt.Println("To be implemented")
	return nil
}
