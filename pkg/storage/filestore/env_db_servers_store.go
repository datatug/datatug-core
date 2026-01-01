package filestore

import (
	"context"
	"path"
	"path/filepath"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

var _ datatug.EnvDbServersStore = (*fsEnvDbServersStore)(nil)

func newFsEnvDbServersStore(projectPath string) fsEnvDbServersStore {
	return fsEnvDbServersStore{
		fsProjectItemsStore: newFileProjectItemsStore[datatug.EnvDbServers, *datatug.EnvDbServer, datatug.EnvDbServer](
			path.Join(projectPath, storage.EnvironmentsFolder), storage.BoardFileSuffix,
		),
	}
}

type fsEnvDbServersStore struct {
	fsProjectItemsStore[datatug.EnvDbServers, *datatug.EnvDbServer, datatug.EnvDbServer]
}

func (f fsEnvDbServersStore) LoadEnvDbServers(ctx context.Context, envID string, o ...datatug.StoreOption) (datatug.EnvDbServers, error) {
	dirPath := filepath.Join(f.dirPath, envID)
	return f.loadProjectItems(ctx, dirPath, o...)
}

func (f fsEnvDbServersStore) LoadEnvDbServer(ctx context.Context, envID, serverID string, o ...datatug.StoreOption) (*datatug.EnvDbServer, error) {
	dirPath := filepath.Join(f.dirPath, envID)
	return f.loadProjectItem(ctx, dirPath, serverID, "", o...)
}

func (f fsEnvDbServersStore) SaveEnvServers(ctx context.Context, envID string, servers datatug.EnvDbServers) error {
	dirPath := filepath.Join(f.dirPath, envID)
	return f.saveProjectItems(ctx, dirPath, servers)
}

func (f fsEnvDbServersStore) SaveEnvDbServer(ctx context.Context, envID string, server *datatug.EnvDbServer) error {
	dirPath := filepath.Join(f.dirPath, envID)
	return f.saveProjectItem(ctx, dirPath, server)
}

func (f fsEnvDbServersStore) DeleteEnvDbServer(ctx context.Context, envID, serverID string) error {
	dirPath := filepath.Join(f.dirPath, envID)
	return f.deleteProjectItem(ctx, dirPath, serverID)
}

//func (store fsEnvDbServersStore) saveEnvServers(servers []*datatug.EnvDbServer) (err error) {
//	log.Printf("saving %v servers for env %v to %v...", len(servers), store.envID, store.envServersPath)
//	if err = os.MkdirAll(store.envServersPath, 0777); err != nil {
//		return fmt.Errorf("failed to create environment servers folder: %w", err)
//	}
//	serversByHost := make(map[string][]*datatug.EnvDbServer, len(servers))
//	for i, server := range servers {
//		if strings.TrimSpace(server.Host) == "" {
//			return fmt.Errorf("env DB server has empty host for entry at index %v", i)
//		}
//		serversByHost[server.Host] = append(serversByHost[server.Host], server)
//	}
//	type hostWithServer struct {
//		host    string
//		servers []*datatug.EnvDbServer
//	}
//	hostsWithServers := make([]*hostWithServer, 0, len(serversByHost))
//	for host, servers := range serversByHost {
//		hostsWithServers = append(hostsWithServers, &hostWithServer{host: host, servers: servers})
//	}
//	dirPath := path.Join(store.envsDirPath, store.envServersPath)
//	return saveItems("servers", len(hostsWithServers), func(i int) func() error {
//		return func() error {
//			hostWithServer := hostsWithServers[i]
//
//			return store.saveEnvServerHost(dirPath, hostWithServer.host, hostWithServer.servers)
//		}
//	})
//}
//
//func (store fsEnvDbServersStore) saveEnvServerHost(dirPath, host string, servers datatug.EnvDbServers) (err error) {
//	if host == "" {
//		return errors.New("func saveEnvServerHost can not accept empty string for `host` parameter")
//	}
//	fileName := JsonFileName(host, ServerFileSuffix)
//	if len(servers) == 0 {
//		filePath := path.Join(dirPath, fileName)
//		fileInfo, err := os.Stat(filePath)
//		if os.IsNotExist(err) {
//			return nil
//		}
//		if !fileInfo.IsDir() {
//			if err = os.Remove(filePath); err != nil {
//				return fmt.Errorf("failed to remove DB server host file that have no DB instances: %w", err)
//			}
//		}
//		return nil
//	}
//	if err = saveJSONFile(dirPath, fileName, servers); err != nil {
//		return fmt.Errorf("failed to write environment's server info into JSON file: %w", err)
//	}
//	return nil
//}
