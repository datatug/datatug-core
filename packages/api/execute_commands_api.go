package api

import (
	"fmt"
	"github.com/datatug/datatug/packages/execute"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/store"
)

// ExecuteCommands executes command
func ExecuteCommands(request execute.Request) (response execute.Response, err error) {
	dbs := make(map[string]*models.EnvDb)

	var getEnvDbByID = func(envID, dbID string) (envDb *models.EnvDb, err error) {
		key := fmt.Sprintf("%v/%v", envDb, dbID)
		if db, cached := dbs[key]; cached {
			return db, err
		}
		if envDb, err = store.Current.LoadEnvironmentCatalog(request.Project, envID, dbID); err != nil {
			return
		}
		dbs[key] = envDb
		return
	}

	executor := execute.NewExecutor(getEnvDbByID)
	return executor.Execute(request)
}
