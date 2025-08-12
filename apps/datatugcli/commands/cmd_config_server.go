package commands

import (
	"fmt"
	config3 "github.com/datatug/datatug/apps/datatugcli/config"
	"os"
)

type configServerCommand struct {
	urlConfigCommand
}

func (v *configServerCommand) Execute(_ []string) error {
	config, err := config3.GetSettings()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}
	if changed := v.updateUrlConfig(&config.Server.UrlConfig); changed {
		if err = saveConfig(config); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
	}
	return config3.PrintSection(config.Server, config3.FormatYaml, os.Stdout)
}
