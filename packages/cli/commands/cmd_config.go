package commands

import (
	"fmt"
	"github.com/datatug/datatug/packages/cli/config"
	"log"
	"os"
)

func init() {
	configCmd, err := Parser.AddCommand("config", "Prints config", "", &configCommand{})
	if err != nil {
		log.Fatal(err)
	}
	configCmd.SubcommandsOptional = true

	if _, err = configCmd.AddCommand("server", "Configures server", "", &configServerCommand{}); err != nil {
		log.Fatal(err)
	}
	if _, err = configCmd.AddCommand("client", "Configures client", "", &configClientCommand{}); err != nil {
		log.Fatal(err)
	}
}

// configCommand prints whole DataTug config
type configCommand struct {
}

func (v *configCommand) Execute(_ []string) error {
	settings, err := config.GetSettings()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}
	if err = config.PrintSettings(settings, config.FormatYaml, os.Stdout); err != nil {
		return err
	}
	return nil
}
