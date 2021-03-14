package filestore

import (
	"errors"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"log"
	"os"
	"path"
	"strings"
)

func (s fileSystemSaver) saveEnvServers(env string, servers []*models.EnvDbServer) (err error) {
	dirPath := path.Join(s.projDirPath, DatatugFolder, EnvironmentsFolder, env, ServersFolder, DbFolder)
	log.Printf("saving %v servers for env %v to %v...", len(servers), env, dirPath)
	if err = os.MkdirAll(dirPath, 0777); err != nil {
		return fmt.Errorf("failed to create environment servers folder: %w", err)
	}
	serversByHost := make(map[string][]*models.EnvDbServer, len(servers))
	for i, server := range servers {
		if strings.TrimSpace(server.Host) == "" {
			return fmt.Errorf("env DB server has empty host for entry at index %v", i)
		}
		serversByHost[server.Host] = append(serversByHost[server.Host], server)
	}
	type hostWithServer struct {
		host    string
		servers []*models.EnvDbServer
	}
	hostsWithServers := make([]*hostWithServer, 0, len(serversByHost))
	for host, servers := range serversByHost {
		hostsWithServers = append(hostsWithServers, &hostWithServer{host: host, servers: servers})
	}
	return s.saveItems("servers", len(hostsWithServers), func(i int) func() error {
		return func() error {
			return s.saveEnvServerHost(dirPath, hostsWithServers[i].host, hostsWithServers[i].servers)
		}
	})
}

func (s fileSystemSaver) saveEnvServerHost(dirPath, host string, servers models.EnvDbServers) (err error) {
	if host == "" {
		return errors.New("func saveEnvServerHost can not accept empty string for `host` parameter")
	}
	fileName := jsonFileName(host, serverFileSuffix)
	if len(servers) == 0 {
		filePath := path.Join(dirPath, fileName)
		fileInfo, err := os.Stat(filePath)
		if os.IsNotExist(err) {
			return nil
		}
		if !fileInfo.IsDir() {
			if err = os.Remove(filePath); err != nil {
				return fmt.Errorf("failed to remove DB server host file that have no DB instances: %w", err)
			}
		}
		return nil
	}
	if err = s.saveJSONFile(dirPath, fileName, servers); err != nil {
		return fmt.Errorf("failed to write environment's server info into JSON file: %w", err)
	}
	return nil
}
