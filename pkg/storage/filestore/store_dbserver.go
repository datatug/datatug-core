package filestore

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"sync"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/strongo/slice"
)

var _ storage.DbServerStore = (*fsDbServerStore)(nil)

type fsDbServerStore struct {
	dbServer datatug.ServerReference
	fsDbServersStore
}

func newFsDbServerStore(dbServer datatug.ServerReference, fsDbServersStore fsDbServersStore) fsDbServerStore {
	return fsDbServerStore{
		dbServer:         dbServer,
		fsDbServersStore: fsDbServersStore,
	}
}

func (store fsDbServerStore) ID() datatug.ServerReference {
	return store.dbServer
}

func (store fsDbServerStore) Catalogs() storage.DbCatalogsStore {
	return newFsDbCatalogsStore(store)
}

// LoadDbServerSummary returns ProjDbServerSummary
func (store fsDbServerStore) LoadDbServerSummary(_ context.Context, dbServer datatug.ServerReference) (summary *datatug.ProjDbServerSummary, err error) {
	summary, err = loadDbServerForDbServerSummary(store.projectPath, dbServer)
	return
}

func loadDbServerForDbServerSummary(projPath string, dbServer datatug.ServerReference) (summary *datatug.ProjDbServerSummary, err error) {
	dbServerPath := path.Join(projPath, "servers", "db", dbServer.Driver, dbServer.FileName())
	summary = new(datatug.ProjDbServerSummary)
	summary.DbServer = dbServer
	var dbsByEnv map[string][]string
	if dbsByEnv, err = loadDbServerCatalogNamesByEnvironments(projPath, dbServer); err != nil {
		return
	}
	log.Printf("dbsByEnv: %+v", dbsByEnv)
	summary.Catalogs, err = loadDbCatalogsForDbServerSummary(dbServerPath, dbsByEnv)
	return
}

func loadDbCatalogsForDbServerSummary(dbServerPath string, dbsByEnv map[string][]string) (catalogSummaries []*datatug.DbCatalogSummary, err error) {
	catalogsPath := path.Join(dbServerPath, "catalogs")
	err = loadDir(nil, catalogsPath, "", processDirs, func(files []os.FileInfo) {
		catalogSummaries = make([]*datatug.DbCatalogSummary, 0, len(files))
	}, func(f os.FileInfo, i int, mutex *sync.Mutex) (err error) {
		catalogSummary, err := loadDbCatalogSummary(catalogsPath, f.Name())
		if err != nil {
			return fmt.Errorf("failed to laoad DB catalog summary: %w", err)
		}
		catalogSummaries = append(catalogSummaries, catalogSummary)
		for env, dbs := range dbsByEnv {
			if slice.Index(dbs, catalogSummaries[i].ID) >= 0 {
				catalogSummaries[i].Environments = append(catalogSummaries[i].Environments, env)
			} else {
				catalogSummaries[i].Environments = []string{}
			}
		}
		return err
	})
	return
}

func loadDbCatalogSummary(catalogsDirPath, dirName string) (*datatug.DbCatalogSummary, error) {
	dirPath := path.Join(catalogsDirPath, dirName)
	jsonFilePath := path.Join(dirPath, jsonFileName(dirName, "db"))
	var catalogSummary datatug.DbCatalogSummary
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

func loadDbServerCatalogNamesByEnvironments(projPath string, dbServer datatug.ServerReference) (dbsByEnv map[string][]string, err error) {
	envsPath := path.Join(projPath, "environments")
	err = loadDir(nil, envsPath, "", processDirs, func(files []os.FileInfo) {
		dbsByEnv = make(map[string][]string, len(files))
	}, func(f os.FileInfo, i int, mutex *sync.Mutex) (err error) {
		env := f.Name()
		dbServersPath := path.Join(envsPath, env, "servers", "db")
		filePath := path.Join(dbServersPath, jsonFileName(dbServer.FileName(), serverFileSuffix))
		var envDbServer = new(datatug.EnvDbServer)
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

// DeleteDbServer deletes DB server
func (store fsDbServerStore) DeleteDbServer(_ context.Context, dbServer datatug.ServerReference) (err error) {
	dbServerDirPath := path.Join(store.projectPath, "servers", "db", dbServer.Driver, dbServer.FileName())
	log.Println("Deleting folder:", dbServerDirPath)
	if err = os.RemoveAll(dbServerDirPath); err != nil {
		return fmt.Errorf("failed to remove db server directory: %w", err)
	}
	return
}
