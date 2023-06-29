package commands

import (
	"fmt"
	"github.com/datatug/datatug/packages/cli/config"
	"os"
)

type configClientCommand struct {
	urlConfigCommand
}

func (v *configClientCommand) Execute(_ []string) error {
	settings, err := config.GetSettings()
	if err != nil {
		return fmt.Errorf("failed to get settings: %w", err)
	}
	if changed := v.updateUrlConfig(&settings.Client.UrlConfig); changed {
		if err = saveConfig(settings); err != nil {
			return fmt.Errorf("failed to save settings: %w", err)
		}
	}
	return config.PrintSection(settings.Client, config.FormatYaml, os.Stdout)
}
