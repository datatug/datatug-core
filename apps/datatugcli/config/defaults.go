package config

const (
	DefaultHost       = "localhost"
	DefaultClientPort = 4200
	DefaultServerPort = 8989
)

func setDefault(config *Settings) {
	setDefaultServer(config)
	setDefaultClient(config)
}

func setDefaultServer(config *Settings) {
	if config.Server == nil {
		config.Server = new(ServerConfig)
	}
	if config.Server.Host == "" {
		config.Server.Host = DefaultHost
	}
	if config.Server.Port == 0 {
		config.Server.Port = DefaultServerPort
	}
}

func setDefaultClient(config *Settings) {
	if config.Client == nil {
		config.Client = new(ClientConfig)
	}
	if config.Server.Host == "" {
		config.Server.Host = DefaultHost
	}
	if config.Server.Port == 0 {
		config.Server.Port = DefaultClientPort
	}
}
