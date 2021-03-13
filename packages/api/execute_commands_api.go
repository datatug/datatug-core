package api

import (
	"fmt"
	"github.com/datatug/datatug/packages/execute"
	"github.com/datatug/datatug/packages/server/dto"
	"github.com/datatug/datatug/packages/store"
)

// ExecuteCommands executes command
func ExecuteCommands(request execute.Request) (response execute.Response, err error) {
	dbs := make(map[string]*dto.EnvDb)

	var getEnvDbByID = func(envID, dbID string) (envDb *dto.EnvDb, err error) {
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
