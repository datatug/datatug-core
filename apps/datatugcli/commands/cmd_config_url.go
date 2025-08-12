package commands

import (
	"github.com/datatug/datatug/apps/datatugcli/config"
)

// urlConfigCommand is embedded into configServerCommand and configClientCommand
type urlConfigCommand struct {
	Reset bool   `short:"r" long:"reset" description:"Reset server config"`
	Host  string `short:"h" long:"host" description:"Host name"`
	Port  int    `short:"o" long:"port" description:"Port number"`
}

func (v *urlConfigCommand) updateUrlConfig(urlConfig *config.UrlConfig) (changed bool) {
	if v.Reset {
		urlConfig.Host = ""
		urlConfig.Port = 0
		changed = true
	}
	if v.Host != "" {
		urlConfig.Host = v.Host
		changed = true
	}
	if v.Port != 0 || urlConfig.Port != 0 {
		urlConfig.Port = v.Port
		changed = true
	}
	return changed
}
