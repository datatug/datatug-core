package commands

import (
	"fmt"
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

type urlConfigCommand struct {
	Host string `short:"h" long:"host" description:"Host name"`
	Port int    `short:"o" long:"port" description:"Port number"`
}

func (v *urlConfigCommand) execute(urlConfig *UrlConfig) (changed bool) {
	if v.Host != "" {
		urlConfig.Host = v.Host
		changed = true
	}
	if v.Port != 0 {
		urlConfig.Port = v.Port
		changed = true
	}
	return changed
}

type configServerCommand struct {
	urlConfigCommand
}

func (v *configServerCommand) Execute(_ []string) error {
	config, err := getConfig()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}
	if config.Server == nil {
		config.Server = &ServerConfig{}
	}
	if changed := v.execute(&config.Server.UrlConfig); changed {
		if err = saveConfig(config); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
	}
	return printConfigSection(config, ConfigFormatYaml, os.Stdout)
}

type configClientCommand struct {
	urlConfigCommand
}

func (v *configClientCommand) Execute(_ []string) error {
	config, err := getConfig()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}
	if config.Client == nil {
		config.Client = &ClientConfig{}
	}
	if changed := v.execute(&config.Client.UrlConfig); changed {
		if err = saveConfig(config); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
	}
	return printConfigSection(config, ConfigFormatYaml, os.Stdout)
}
