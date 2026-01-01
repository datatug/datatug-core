package filestore

import (
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

func loadEnvServers(dirPath string, env *datatug.Environment) error {
	return loadDir(nil, dirPath, "*.json", processFiles, func(files []os.FileInfo) {
		env.DbServers = make([]*datatug.EnvDbServer, 0, len(files))
	}, func(f os.FileInfo, i int, mutex *sync.Mutex) error {
		fileName := f.Name()
		serverName, suffix := storage.GetProjItemIDFromFileName(fileName)
		if suffix != storage.ServerFileSuffix {
			return nil
		}
		servers := make([]*datatug.EnvDbServer, 0, 1)
		if err := readJSONFile(path.Join(dirPath, fileName), false, &servers); err != nil {
			return err
		}
		for i, server := range servers {
			if server.Host == "" {
				server.Host = serverName
			} else if server.Host != serverName {
				return fmt.Errorf("server file GetID is different from server host at index %v: %v != %v",
					i, serverName, server.Host)
			}
		}
		env.DbServers = append(env.DbServers, servers...)
		return nil
	})
}
