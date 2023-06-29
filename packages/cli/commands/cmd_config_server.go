package commands

import (
	"fmt"
	config2 "github.com/datatug/datatug/packages/cli/config"
	"os"
)

type configServerCommand struct {
	urlConfigCommand
}

func (v *configServerCommand) Execute(_ []string) error {
	config, err := config2.GetSettings()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}
	if changed := v.updateUrlConfig(&config.Server.UrlConfig); changed {
		if err = saveConfig(config); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
	}
	return config2.PrintSection(config.Server, config2.FormatYaml, os.Stdout)
}
