package commands

import (
	"fmt"
	config2 "github.com/datatug/datatug/apps/datatugcli/config"
	"os"
)

type configClientCommand struct {
	urlConfigCommand
}

func (v *configClientCommand) Execute(_ []string) error {
	settings, err := config2.GetSettings()
	if err != nil {
		return fmt.Errorf("failed to get settings: %w", err)
	}
	if changed := v.updateUrlConfig(&settings.Client.UrlConfig); changed {
		if err = saveConfig(settings); err != nil {
			return fmt.Errorf("failed to save settings: %w", err)
		}
	}
	return config2.PrintSection(settings.Client, config2.FormatYaml, os.Stdout)
}
