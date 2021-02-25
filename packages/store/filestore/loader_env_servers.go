package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"os"
	"path"
	"sync"
)

func loadEnvServers(dirPath string, env *models.Environment) error {
	return loadDir(nil, dirPath, processFiles, func(files []os.FileInfo) {
		env.DbServers = make([]*models.EnvDbServer, 0, len(files))
	}, func(f os.FileInfo, i int, mutex *sync.Mutex) error {
		fileName := f.Name()
		serverName, suffix := getProjItemIdFromFileName(fileName)
		if suffix != serverFileSuffix {
			return nil
		}
		servers := make([]*models.EnvDbServer, 0, 1)
		if err := readJSONFile(path.Join(dirPath, fileName), false, &servers); err != nil {
			return err
		}
		for i, server := range servers {
			if server.Host == "" {
				server.Host = serverName
			} else if server.Host != serverName {
				return fmt.Errorf("server file ID is different from server host at index %v: %v != %v",
					i, serverName, server.Host)
			}
		}
		env.DbServers = append(env.DbServers, servers...)
		return nil
	})
}
