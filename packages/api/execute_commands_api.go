package api

import (
	"fmt"
	"github.com/datatug/datatug/packages/execute"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/store"
)

// ExecuteCommands executes command
func ExecuteCommands(storeID string, request execute.Request) (response execute.Response, err error) {

	var dal store.Interface
	dal, err = store.NewDatatugStore(storeID)
	if err != nil {
		return
	}

	dbs := make(map[string]*models.EnvDb)

	var getEnvDbByID = func(envID, dbID string) (envDb *models.EnvDb, err error) {
		key := fmt.Sprintf("%v/%v", envDb, dbID)
		if db, cached := dbs[key]; cached {
			return db, err
		}
		if envDb, err = dal.LoadEnvironmentCatalog(request.Project, envID, dbID); err != nil {
			return
		}
		dbs[key] = envDb
		return
	}

	var getCatalog = func(server models.ServerReference, catalogID string) (*models.DbCatalogSummary, error) {
		return dal.LoadDbCatalogSummary(request.Project, server, catalogID)
	}

	executor := execute.NewExecutor(getEnvDbByID, getCatalog)
	return executor.Execute(request)
}
