package api

import (
	"fmt"
	"github.com/datatug/datatug/packages/execute"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
)

// ExecuteCommands executes command
func ExecuteCommands(storeID string, request execute.Request) (response execute.Response, err error) {

	var dal storage.Store
	dal, err = storage.NewDatatugStore(storeID)
	if err != nil {
		return
	}

	dbs := make(map[string]*models.EnvDb)

	var getEnvDbByID = func(envID, dbID string) (envDb *models.EnvDb, err error) {
		key := fmt.Sprintf("%v/%v", envDb, dbID)
		if db, cached := dbs[key]; cached {
			return db, err
		}
		envServerStore := dal.Project(request.Project).Environments().Environment(envID).Servers().Server(dbID)
		if envDb, err = envServerStore.Catalogs().Catalog(dbID).LoadEnvironmentCatalog(); err != nil {
			return
		}
		dbs[key] = envDb
		return
	}

	var getCatalog = func(server models.ServerReference, catalogID string) (*models.DbCatalogSummary, error) {
		serverStore := dal.Project(request.Project).DbServers().DbServer(server)
		return serverStore.Catalogs().DbCatalog(catalogID).LoadDbCatalogSummary()
	}

	executor := execute.NewExecutor(getEnvDbByID, getCatalog)
	return executor.Execute(request)
}
