package commands

import (
	"fmt"
	"github.com/datatug/datatug/packages/appconfig"
	"os"
)

type configClientCommand struct {
	urlConfigCommand
}

func (v *configClientCommand) Execute(_ []string) error {
	settings, err := appconfig.GetSettings()
	if err != nil {
		return fmt.Errorf("failed to get settings: %w", err)
	}
	if changed := v.updateUrlConfig(&settings.Client.UrlConfig); changed {
		if err = saveConfig(settings); err != nil {
			return fmt.Errorf("failed to save settings: %w", err)
		}
	}
	return appconfig.PrintSection(settings.Client, appconfig.FormatYaml, os.Stdout)
}
