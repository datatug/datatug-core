package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"github.com/datatug/datatug/packages/slice"
	"github.com/datatug/datatug/packages/storage"
	"io"
	"log"
	"os"
	"path"
	"sync"
)

var _ storage.DbServerStore = (*fsDbServerStore)(nil)

type fsDbServerStore struct {
	fsProjectStore
	dbServer models.ServerReference
}

func (store fsDbServerStore) DbServer() models.ServerReference {
	return store.dbServer
}

func (store fsDbServerStore) Loader() storage.DbServerLoader {
	return store
}

func (store fsDbServerStore) Saver() storage.DbServerSaver {
	return store
}

func (store fsDbServerStore) Catalogs() storage.DbCatalogStore {
	return newFsDbCatalogStore(store)
}

// GetDbServerSummary returns ProjDbServerSummary
func (store fsDbServerStore) LoadDbServerSummary(dbServer models.ServerReference) (summary *models.ProjDbServerSummary, err error) {
	summary, err = loadDbServerForDbServerSummary(store.projectPath, dbServer)
	return
}

func loadDbServerForDbServerSummary(projPath string, dbServer models.ServerReference) (summary *models.ProjDbServerSummary, err error) {
	dbServerPath := path.Join(projPath, "servers", "db", dbServer.Driver, dbServer.FileName())
	summary = new(models.ProjDbServerSummary)
	summary.DbServer = dbServer
	var dbsByEnv map[string][]string
	if dbsByEnv, err = loadDbServerCatalogNamesByEnvironments(projPath, dbServer); err != nil {
		return
	}
	log.Printf("dbsByEnv: %+v", dbsByEnv)
	summary.Catalogs, err = loadDbCatalogsForDbServerSummary(dbServerPath, dbsByEnv)
	return
}

func loadDbCatalogsForDbServerSummary(dbServerPath string, dbsByEnv map[string][]string) (catalogSummaries []*models.DbCatalogSummary, err error) {
	catalogsPath := path.Join(dbServerPath, "catalogs")
	err = loadDir(nil, catalogsPath, processDirs, func(files []os.FileInfo) {
		catalogSummaries = make([]*models.DbCatalogSummary, 0, len(files))
	}, func(f os.FileInfo, i int, mutex *sync.Mutex) (err error) {
		catalogSummary, err := loadDbCatalogSummary(catalogsPath, f.Name())
		if err != nil {
			return fmt.Errorf("failed to laoad DB catalog summary: %w", err)
		}
		catalogSummaries = append(catalogSummaries, catalogSummary)
		for env, dbs := range dbsByEnv {
			if slice.IndexOfString(dbs, catalogSummaries[i].ID) >= 0 {
				catalogSummaries[i].Environments = append(catalogSummaries[i].Environments, env)
			} else {
				catalogSummaries[i].Environments = []string{}
			}
		}
		return err
	})
	return
}

func loadDbCatalogSummary(catalogsDirPath, dirName string) (*models.DbCatalogSummary, error) {
	dirPath := path.Join(catalogsDirPath, dirName)
	jsonFilePath := path.Join(dirPath, jsonFileName(dirName, "db"))
	var catalogSummary models.DbCatalogSummary
	if err := readJSONFile(jsonFilePath, true, &catalogSummary); err != nil {
		return nil, fmt.Errorf("failed to read DB catalog summary from JSON file: %w", err)
	}
	return &catalogSummary, nil
}

//func loadEnvironmentIds(projPath string) (environments []string, err error) {
//	envsPath := projDirPath.Join(projPath, "environments")
//	err = loadDir(envsPath, processDirs, func(count int) {
//		environments = make([]string, count)
//	}, func(f os.FileInfo, i int, mutex *sync.Mutex) (err error) {
//		environments[i] = f.Name()
//		return
//	})
//	return
//}

func loadDbServerCatalogNamesByEnvironments(projPath string, dbServer models.ServerReference) (dbsByEnv map[string][]string, err error) {
	envsPath := path.Join(projPath, "environments")
	err = loadDir(nil, envsPath, processDirs, func(files []os.FileInfo) {
		dbsByEnv = make(map[string][]string, len(files))
	}, func(f os.FileInfo, i int, mutex *sync.Mutex) (err error) {
		env := f.Name()
		dbServersPath := path.Join(envsPath, env, "servers", "db")
		filePath := path.Join(dbServersPath, jsonFileName(dbServer.FileName(), serverFileSuffix))
		var envDbServer = new(models.EnvDbServer)
		if err = readJSONFile(filePath, false, envDbServer); err != nil {
			return err
		}
		log.Println("file:", filePath)
		log.Printf("envDbServer: %+v", envDbServer)
		if len(envDbServer.Catalogs) > 0 {
			dbsByEnv[env] = envDbServer.Catalogs
		}
		return
	})
	return
}

func (store fsDbServerStore) saveDbServers(dbServers models.ProjDbServers, project models.DatatugProject) (err error) {
	if len(dbServers) == 0 {
		log.Println("Project have no DB servers to save.")
		return nil
	}
	log.Printf("Saving %v DB servers...\n", len(project.DbServers))
	dbServersDirPath := path.Join(store.projectPath, DatatugFolder, ServersFolder, DbFolder)
	err = parallel.Run(
		func() (err error) {
			return store.saveDbServersJSON(dbServersDirPath, dbServers)
		},
		func() (err error) {
			return store.saveDbServersReadme(dbServers)
		},
		func() (err error) {
			return saveItems("servers", len(dbServers), func(i int) func() error {
				return func() error {
					return store.SaveDbServer(*dbServers[i], project)
				}
			})
		},
	)
	if err != nil {
		return fmt.Errorf("failed to save DB servers: %w", err)
	}
	log.Printf("Saved %v DB servers.", len(project.DbServers))
	return nil
}

func (store fsDbServerStore) saveDbServersJSON(dbServersDirPath string, dbServers models.ProjDbServers) error {
	servers := make(models.ServerReferences, len(dbServers))
	for i, server := range dbServers {
		servers[i] = server.Server
	}
	if err := saveJSONFile(dbServersDirPath, "servers.json", servers); err != nil {
		return fmt.Errorf("failed to save list of servers as JSON file: %w", err)
	}
	return nil
}

func (store fsDbServerStore) saveDbServersReadme(dbServers models.ProjDbServers) error {
	return nil
}

// SaveDbServer saves ServerReference
func (store fsDbServerStore) SaveDbServer(dbServer models.ProjDbServer, project models.DatatugProject) (err error) {
	if err = dbServer.Validate(); err != nil {
		return fmt.Errorf("db server is not valid: %w", err)
	}
	dbServerDirPath := path.Join(store.projectPath, DatatugFolder, ServersFolder, DbFolder, dbServer.Server.Driver, dbServer.Server.FileName())
	if err := os.MkdirAll(dbServerDirPath, 0777); err != nil {
		return fmt.Errorf("failed to created server directory: %w", err)
	}
	err = parallel.Run(
		func() error {
			return store.saveDbServerJSON(dbServer, dbServerDirPath, project)
		},
		func() error {
			return store.saveDbServerReadme(dbServer, dbServerDirPath, project)
		},
		func() error {
			store.Catalogs().Loader().
			if err = store.saveDbCatalogs(dbServer, project.Repository); err != nil {
				return fmt.Errorf("failed to save DB catalogs: %w", err)
			}
			return nil
		},
	)
	if err != nil {
		return fmt.Errorf("failed to save DB server [%v]: %w", dbServer.ID, err)
	}
	return nil
}

func (store fsDbServerStore) saveDbServerReadme(dbServer models.ProjDbServer, dbServerDirPath string, project models.DatatugProject) error {
	return saveReadme(dbServerDirPath, "DB server", func(w io.Writer) error {
		if err := store.readmeEncoder.DbServerToReadme(w, project.Repository, dbServer); err != nil {
			return fmt.Errorf("failed to write README.md for DB server: %w", err)
		}
		return nil
	})
}

func (store fsDbServerStore) saveDbServerJSON(dbServer models.ProjDbServer, dbServerDirPath string, _ models.DatatugProject) error {
	log.Println("store.projDirPath:", store.projectPath)
	log.Println("dbServerDirPath:", dbServerDirPath)
	if err := os.MkdirAll(dbServerDirPath, 0777); err != nil {
		return fmt.Errorf("failed to create a directory for DB server files: %w", err)
	}
	serverFile := models.ProjDbServerFile{
		ServerReference: dbServer.Server,
	}
	if len(dbServer.Catalogs) > 0 {
		serverFile.Catalogs = make([]string, len(dbServer.Catalogs))
		for i, catalog := range dbServer.Catalogs {
			serverFile.Catalogs[i] = catalog.ID
		}
	}
	if err := saveJSONFile(dbServerDirPath, jsonFileName(dbServer.Server.FileName(), dbServerFileSuffix), serverFile); err != nil {
		return fmt.Errorf("failed to save DB server JSON file: %w", err)
	}
	return nil
}

// DeleteDbServer deletes DB server
func (store fsDbServerStore) DeleteDbServer(dbServer models.ServerReference) (err error) {
	dbServerDirPath := path.Join(store.projDirPath, "servers", "db", dbServer.Driver, dbServer.FileName())
	log.Println("Deleting folder:", dbServerDirPath)
	if err = os.RemoveAll(dbServerDirPath); err != nil {
		return fmt.Errorf("failed to remove db server directory: %w", err)
	}
	return
}
