package filestore

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	"github.com/datatug/datatug-core/pkg/models"
	"github.com/datatug/datatug-core/pkg/storage"
)

var _ storage.EnvServerStore = (*fsEnvServerStore)(nil)

type fsEnvServerStore struct {
	serverID string
	fsEnvServersStore
}

func (store fsEnvServerStore) LoadEnvServer() (*models.EnvDbServer, error) {
	panic("implement me")
}

func (store fsEnvServerStore) SaveEnvServer(envServer *models.EnvDbServer) error {
	panic("implement me")
}

func newFsEnvServerStore(serverID string, fsEnvServersStore fsEnvServersStore) fsEnvServerStore {
	return fsEnvServerStore{
		serverID:          serverID,
		fsEnvServersStore: fsEnvServersStore,
	}
}

func (store fsEnvServerStore) Catalogs() storage.EnvDbCatalogsStore {
	return newFsEnvCatalogsStore(store)
}

func (store fsEnvServersStore) saveEnvServers(servers []*models.EnvDbServer) (err error) {
	log.Printf("saving %v servers for env %v to %v...", len(servers), store.envID, store.envServersPath)
	if err = os.MkdirAll(store.envServersPath, 0777); err != nil {
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
	dirPath := "<NOT implemented;/>"
	return saveItems("servers", len(hostsWithServers), func(i int) func() error {
		return func() error {
			hostWithServer := hostsWithServers[i]

			return store.saveEnvServerHost(dirPath, hostWithServer.host, hostWithServer.servers)
		}
	})
}

func (store fsEnvServersStore) saveEnvServerHost(dirPath, host string, servers models.EnvDbServers) (err error) {
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
	if err = saveJSONFile(dirPath, fileName, servers); err != nil {
		return fmt.Errorf("failed to write environment's server info into JSON file: %w", err)
	}
	return nil
}
