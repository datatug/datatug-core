package commands

import (
	"fmt"
	"log"
	"os"
)

func init() {
	_, err := Parser.AddCommand("config", "Prints config", "", &configCommand{})
	if err != nil {
		log.Fatal(err)
	}
}

type configCommand struct {
}

func (v *configCommand) Execute(_ []string) error {
	config, err := getConfig()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}
	if err = printConfig(config, ConfigFormatYaml, os.Stdout); err != nil {
		return err
	}
	return nil
}
