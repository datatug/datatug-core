package dtconfig

import "sync/atomic"

var explicitConfigPath atomic.Pointer[string]

// SetConfigFilePath selects an explicit settings file for this process. A CLI
// sets it once before executing a command. An empty path restores the default
// ~/.datatug.yaml path. It does not create or read either file.
func SetConfigFilePath(configPath string) {
	explicitConfigPath.Store(&configPath)
}
