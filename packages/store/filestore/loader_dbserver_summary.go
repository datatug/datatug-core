package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/server/dto"
	"github.com/datatug/datatug/packages/slice"
	"github.com/datatug/datatug/packages/store"
	"log"
	"os"
	"path"
	"sync"
)

// GetDbServerSummary returns ProjDbServerSummary
func (loader fileSystemLoader) LoadDbServerSummary(projID string, dbServer models.DbServer) (summary *dto.ProjDbServerSummary, err error) {
	if projID == "" && len(projectPaths) == 1 {
		projID = store.SingleProjectID
	}
	projPath := path.Join(loader.pathByID[projID], "datatug")
	summary, err = loadDbServerForDbServerSummary(projPath, dbServer)
	return
}

func loadDbServerForDbServerSummary(projPath string, dbServer models.DbServer) (summary *dto.ProjDbServerSummary, err error) {
	dbServerPath := path.Join(projPath, "servers", "db", dbServer.Driver, dbServer.FileName())
	summary = new(dto.ProjDbServerSummary)
	summary.DbServer = dbServer
	var dbsByEnv map[string][]string
	if dbsByEnv, err = loadServerDatabaseNamesByEnvironments(projPath, dbServer); err != nil {
		return
	}
	log.Printf("dbsByEnv: %+v", dbsByEnv)
	summary.Databases, err = loadDatabasesForDbServerSummary(dbServerPath, dbsByEnv)
	return
}

func loadDatabasesForDbServerSummary(dbServerPath string, dbsByEnv map[string][]string) (databases []dto.DatabaseSummary, err error) {
	databasesPath := path.Join(dbServerPath, "databases")
	err = loadDir(nil, databasesPath, processDirs, func(files []os.FileInfo) {
		databases = make([]dto.DatabaseSummary, len(files))
	}, func(f os.FileInfo, i int, mutex *sync.Mutex) (err error) {
		databases[i] = dto.DatabaseSummary{
			ProjectItem: models.ProjectItem{ID: f.Name()},
		}
		for env, dbs := range dbsByEnv {
			if slice.IndexOfString(dbs, databases[i].ID) >= 0 {
				databases[i].Environments = append(databases[i].Environments, env)
			} else {
				databases[i].Environments = []string{}
			}
		}
		return err
	})
	return
}

//func loadEnvironmentIds(projPath string) (environments []string, err error) {
//	envsPath := path.Join(projPath, "environments")
//	err = loadDir(envsPath, processDirs, func(count int) {
//		environments = make([]string, count)
//	}, func(f os.FileInfo, i int, mutex *sync.Mutex) (err error) {
//		environments[i] = f.Name()
//		return
//	})
//	return
//}

func loadServerDatabaseNamesByEnvironments(projPath string, dbServer models.DbServer) (dbsByEnv map[string][]string, err error) {
	envsPath := path.Join(projPath, "environments")
	err = loadDir(nil, envsPath, processDirs, func(files []os.FileInfo) {
		dbsByEnv = make(map[string][]string, len(files))
	}, func(f os.FileInfo, i int, mutex *sync.Mutex) (err error) {
		env := f.Name()
		dbServersPath := path.Join(envsPath, env, "servers", "db")
		filePath := path.Join(dbServersPath, fmt.Sprintf("%v.server.json", dbServer.FileName()))
		var envDbServer = new(models.EnvDbServer)
		if err = readJSONFile(filePath, false, envDbServer); err != nil {
			return err
		}
		log.Println("file:", filePath)
		log.Printf("envDbServer: %+v", envDbServer)
		if len(envDbServer.Databases) > 0 {
			dbsByEnv[env] = envDbServer.Databases
		}
		return
	})
	return
}
