package filestore

//import (
//	"context"
//
//	"github.com/datatug/datatug-core/pkg/datatug"
//)

//type fsDbServerStore struct {
//	dbServer datatug.ServerRef
//	fsProjDbServersStore
//}
//
//func newFsDbServerStore(dbServer datatug.ServerRef, fsDbServersStore fsProjDbServersStore) fsDbServerStore {
//	return fsDbServerStore{
//		dbServer:             dbServer,
//		fsProjDbServersStore: fsDbServersStore,
//	}
//}
//
//// LoadDbServerSummary returns ProjDbServerSummary
//func (s fsDbServerStore) LoadDbServerSummary(_ context.Context, dbServer datatug.ServerRef) (summary *datatug.ProjDbServerSummary, err error) {
//	//summary, err = loadDbServerForDbServerSummary(s.projectPath, dbServer)
//	return
//}

//func loadDbServerForDbServerSummary(projPath string, dbServer datatug.ServerRef) (summary *datatug.ProjDbServerSummary, err error) {
//	dbServerPath := path.Join(projPath, "servers", "db", dbServer.Driver, dbServer.FileName())
//	summary = new(datatug.ProjDbServerSummary)
//	summary.DbServer = dbServer
//	var dbsByEnv map[string][]string
//	if dbsByEnv, err = loadDbServerCatalogNamesByEnvironments(projPath, dbServer); err != nil {
//		return
//	}
//	log.Printf("dbsByEnv: %+v", dbsByEnv)
//	summary.Catalogs, err = loadDbCatalogsForDbServerSummary(dbServerPath, dbsByEnv)
//	return
//}

//func loadDbCatalogsForDbServerSummary(dbServerPath string, dbsByEnv map[string][]string) (catalogSummaries []*datatug.DbCatalogSummary, err error) {
//	catalogsPath := path.Join(dbServerPath, "catalogs")
//	err = loadDir(nil, catalogsPath, "", processDirs, func(files []os.FileInfo) {
//		catalogSummaries = make([]*datatug.DbCatalogSummary, 0, len(files))
//	}, func(f os.FileInfo, i int, mutex *sync.Mutex) (err error) {
//		catalogSummary, err := loadDbCatalogSummary(catalogsPath, f.Name())
//		if err != nil {
//			return fmt.Errorf("failed to laoad DB catalog summary: %w", err)
//		}
//		catalogSummaries = append(catalogSummaries, catalogSummary)
//		for env, dbs := range dbsByEnv {
//			if slice.Index(dbs, catalogSummaries[i].ID) >= 0 {
//				catalogSummaries[i].Environments = append(catalogSummaries[i].Environments, env)
//			} else {
//				catalogSummaries[i].Environments = []string{}
//			}
//		}
//		return err
//	})
//	return
//}

//func loadDbCatalogSummary(catalogsDirPath, dirName string) (*datatug.DbCatalogSummary, error) {
//	dirPath := path.Join(catalogsDirPath, dirName)
//	jsonFilePath := path.Join(dirPath, storage.JsonFileName(dirName, "db"))
//	var catalogSummary datatug.DbCatalogSummary
//	if err := readJSONFile(jsonFilePath, true, &catalogSummary); err != nil {
//		return nil, fmt.Errorf("failed to read DB catalog summary from JSON file: %w", err)
//	}
//	return &catalogSummary, nil
//}

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

//func loadDbServerCatalogNamesByEnvironments(projPath string, dbServer datatug.ServerRef) (dbsByEnv map[string][]string, err error) {
//	envsPath := path.Join(projPath, "environments")
//	err = loadDir(nil, envsPath, "", processDirs, func(files []os.FileInfo) {
//		dbsByEnv = make(map[string][]string, len(files))
//	}, func(f os.FileInfo, i int, mutex *sync.Mutex) (err error) {
//		env := f.Name()
//		dbServersPath := path.Join(envsPath, env, "servers", "db")
//		filePath := path.Join(dbServersPath, storage.JsonFileName(dbServer.FileName(), storage.ServerFileSuffix))
//		var envDbServer = new(datatug.EnvDbServer)
//		if err = readJSONFile(filePath, false, envDbServer); err != nil {
//			return err
//		}
//		log.Println("file:", filePath)
//		log.Printf("envDbServer: %+v", envDbServer)
//		if len(envDbServer.Catalogs) > 0 {
//			dbsByEnv[env] = envDbServer.Catalogs
//		}
//		return
//	})
//	return
//}

//// DeleteDbServer deletes DB server
//func (s fsDbServerStore) DeleteDbServer(ctx context.Context, dbServer datatug.ServerRef) (err error) {
//	id := dbServer.GetID()
//	return s.deleteProjectItem(ctx, s.dirPath, id)
//}
