package commands

import (
	"fmt"
	config2 "github.com/datatug/datatug/apps/datatugcli/config"
	"log"
	"os"
)

// configCommand prints whole DataTug config
type configCommand struct {
}

func (v *configCommand) Execute(_ []string) error {
	settings, err := config2.GetSettings()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}
	if err = config2.PrintSettings(settings, config2.FormatYaml, os.Stdout); err != nil {
		return err
	}
	return nil
}

func configCommandArgs(p Parser) {
	if configCmd, err := p.AddCommand("config", "Prints config", "", &configCommand{}); err != nil {
		log.Fatal(err)
	} else {
		configCmd.SubcommandsOptional = true

		if _, err = configCmd.AddCommand("server", "Configures server", "", &configServerCommand{}); err != nil {
			log.Fatal(err)
		}
		if _, err = configCmd.AddCommand("client", "Configures client", "", &configClientCommand{}); err != nil {
			log.Fatal(err)
		}
	}
}
