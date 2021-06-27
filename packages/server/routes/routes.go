package routes

import (
	"github.com/datatug/datatug/packages/server/endpoints"
	"net/http"
	"strings"
)

type router interface {
	HandlerFunc(method, path string, handler http.HandlerFunc)
}

func handle(r router, wrap wrapper, method, path string, handler http.HandlerFunc) {
	if wrap != nil {
		handler = wrap(handler)
	}
	r.HandlerFunc(method, path, handler)
}

func registerRoutes(path string, r router, w wrapper, writeOnly bool) {
	if r == nil {
		panic("r == nil")
	}
	path = strings.TrimRight(path, "/")
	if !writeOnly {
		handle(r, w, http.MethodGet, path+"/ping", endpoints.Ping)
		handle(r, w, http.MethodGet, path+"/agent-info", endpoints.AgentInfo)
	}
	projectsRoutes(path, r, w, writeOnly)
	queriesRoutes(path, r, w, writeOnly)
	boardsRoutes(path, r, w, writeOnly)
	environmentsRoutes(path, r, w, writeOnly)
	dbServerRoutes(path, r, w, writeOnly)
	entitiesRoutes(path, r, w, writeOnly)
	dataRoutes(path, r, w, writeOnly)
	executeRoutes(path, r, w, writeOnly)

}

func queriesRoutes(path string, r router, w wrapper, writeOnly bool) {
	if !writeOnly {
		handle(r, w, http.MethodGet, path+"/datatug/queries/all_queries", endpoints.GetQueries)
		handle(r, w, http.MethodGet, path+"/datatug/queries/get_query", endpoints.GetQuery)
	}
	handle(r, w, http.MethodPut, path+"/datatug/queries/create_folder", endpoints.CreateQueryFolder)
	handle(r, w, http.MethodPost, path+"/datatug/queries/create_query", endpoints.CreateQuery)
	handle(r, w, http.MethodPut, path+"/datatug/queries/update_query", endpoints.UpdateQuery)
	handle(r, w, http.MethodDelete, path+"/datatug/queries/delete_query", endpoints.DeleteQuery)
	handle(r, w, http.MethodDelete, path+"/datatug/queries/delete_folder", endpoints.DeleteQueryFolder)
}

func boardsRoutes(path string, r router, w wrapper, writeOnly bool) {
	if !writeOnly {
		handle(r, w, http.MethodGet, path+"/datatug/boards/board", endpoints.GetBoard)
	}
	handle(r, w, http.MethodPost, path+"/datatug/boards/create_board", endpoints.CreateBoard)
	handle(r, w, http.MethodPut, path+"/datatug/boards/save_board", endpoints.SaveBoard)
	handle(r, w, http.MethodDelete, path+"/datatug/boards/delete_board", endpoints.DeleteBoard)
}

func projectsRoutes(path string, r router, w wrapper, writeOnly bool) {
	if !writeOnly {
		handle(r, w, http.MethodGet, path+"/datatug/projects/projects-summary", endpoints.GetProjects)
		handle(r, w, http.MethodGet, path+"/datatug/projects/project-summary", endpoints.GetProjectSummary)
		handle(r, w, http.MethodGet, path+"/datatug/projects/project-full", endpoints.GetProjectFull)
	}
	projectEndpoints := endpoints.ProjectAgentEndpoints{}
	handle(r, w, http.MethodPost, path+"/datatug/projects/create-project", projectEndpoints.CreateProject)
	handle(r, w, http.MethodDelete, path+"/datatug/projects/create-project", projectEndpoints.DeleteProject)
}

func environmentsRoutes(path string, r router, w wrapper, writeOnly bool) {
	if !writeOnly {
		handle(r, w, http.MethodGet, path+"/datatug/environment-summary", endpoints.GetEnvironmentSummary)
	}
}

func dbServerRoutes(path string, r router, w wrapper, writeOnly bool) {
	if !writeOnly {
		handle(r, w, http.MethodGet, path+"/datatug/dbserver-summary", endpoints.GetDbServerSummary)
		handle(r, w, http.MethodGet, path+"/datatug/dbserver-databases", endpoints.GetServerDatabases)
	}
	handle(r, w, http.MethodPost, path+"/datatug/dbserver-add", endpoints.AddDbServer)
	handle(r, w, http.MethodDelete, path+"/datatug/dbserver-delete", endpoints.DeleteDbServer)
}

func entitiesRoutes(path string, r router, w wrapper, writeOnly bool) {
	if !writeOnly {
		handle(r, w, http.MethodGet, path+"/datatug/entities/all_entities", endpoints.GetEntities)
		handle(r, w, http.MethodGet, path+"/datatug/entities/entity", endpoints.GetEntity)
	}
	handle(r, w, http.MethodPost, path+"/datatug/entities/create_entity", endpoints.SaveEntity)
	handle(r, w, http.MethodPut, path+"/datatug/entities/save_entity", endpoints.SaveEntity)
	handle(r, w, http.MethodDelete, path+"/datatug/entities/delete_entity", endpoints.DeleteEntity)
}

func dataRoutes(path string, r router, w wrapper, writeOnly bool) {
	if !writeOnly {
		handle(r, w, http.MethodGet, path+"/datatug/data/recordsets_summary", endpoints.GetRecordsetsSummary)
		handle(r, w, http.MethodGet, path+"/datatug/data/recordset_definition", endpoints.GetRecordsetDefinition)
		handle(r, w, http.MethodGet, path+"/datatug/data/recordset_data", endpoints.GetRecordsetData)
	}
	handle(r, w, http.MethodPost, path+"/datatug/data/recordset_add_rows", endpoints.AddRowsToRecordset)
	handle(r, w, http.MethodPut, path+"/datatug/data/recordset_update_rows", endpoints.UpdateRowsInRecordset)
	handle(r, w, http.MethodDelete, path+"/datatug/data/recordset_delete_rows", endpoints.DeleteRowsFromRecordset)
}

func executeRoutes(path string, r router, w wrapper, writeOnly bool) {
	if !writeOnly {
		handle(r, w, http.MethodPost, path+"/datatug/exec/execute_commands", endpoints.ExecuteCommandsHandler)
		handle(r, w, http.MethodGet, path+"/datatug/exec/select", endpoints.ExecuteSelectHandler)
	}
}
