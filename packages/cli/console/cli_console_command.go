package console

import (
	"github.com/datatug/datatug/packages/cli/commands"
	"log"
	"os"
)

func init() {
	_, err := commands.Parser.AddCommand("console",
		"Starts interactive console",
		"Starts interactive console with autocomplete",
		&command{})
	if err != nil {
		log.Fatal(err)
	}
}

// command defines parameters for console command
type command struct {
	ProjectDir string `short:"f" long:"folder" description:"Project directory"`
}

// Execute executes serve command
func (v *command) Execute(_ []string) (err error) {
	if err = os.Setenv("GO_FLAGS_COMPLETION", "1"); err != nil {
		return err
	}
	//p := NewCommandsPrompt()
	//p.Run()
	return nil
}
