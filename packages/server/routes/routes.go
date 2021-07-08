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

func registerRoutes(path string, router router, wrapper wrapper, writeOnly bool) {
	if router == nil {
		panic("router == nil")
	}
	path = strings.TrimRight(path, "/") + "/datatug"
	handle(router, wrapper, http.MethodGet, path+"/ping", endpoints.Ping)
	handle(router, wrapper, http.MethodGet, path+"/agent-info", endpoints.AgentInfo)
	projectsRoutes(path, router, wrapper, writeOnly)
	foldersRoutes(path, router, wrapper, writeOnly)
	queriesRoutes(path, router, wrapper, writeOnly)
	boardsRoutes(path, router, wrapper, writeOnly)
	environmentsRoutes(path, router, wrapper, writeOnly)
	dbServerRoutes(path, router, wrapper, writeOnly)
	entitiesRoutes(path, router, wrapper, writeOnly)
	recordsetsRoutes(path, router, wrapper, writeOnly)
	executeRoutes(path, router, wrapper, writeOnly)

}

func foldersRoutes(path string, r router, w wrapper, writeOnly bool) {
	handle(r, w, http.MethodPut, path+"/folders/create_folder", endpoints.CreateFolder)
	handle(r, w, http.MethodDelete, path+"/folders/delete_folder", endpoints.DeleteFolder)
}

func queriesRoutes(path string, r router, w wrapper, writeOnly bool) {
	if !writeOnly {
		//handle(r, w, http.MethodGet, path+"/queries/all_queries", endpoints.GetQueries)
		handle(r, w, http.MethodGet, path+"/queries/get_query", endpoints.GetQuery)
	}
	handle(r, w, http.MethodPost, path+"/queries/create_query", endpoints.CreateQuery)
	handle(r, w, http.MethodPut, path+"/queries/update_query", endpoints.UpdateQuery)
	handle(r, w, http.MethodDelete, path+"/queries/delete_query", endpoints.DeleteQuery)
}

func boardsRoutes(path string, r router, w wrapper, writeOnly bool) {
	if !writeOnly {
		handle(r, w, http.MethodGet, path+"/boards/board", endpoints.GetBoard)
	}
	handle(r, w, http.MethodPost, path+"/boards/create_board", endpoints.CreateBoard)
	handle(r, w, http.MethodPut, path+"/boards/save_board", endpoints.SaveBoard)
	handle(r, w, http.MethodDelete, path+"/boards/delete_board", endpoints.DeleteBoard)
}

func projectsRoutes(path string, r router, w wrapper, writeOnly bool) {
	if !writeOnly {
		handle(r, w, http.MethodGet, path+"/projects/projects_summary", endpoints.GetProjects)
		handle(r, w, http.MethodGet, path+"/projects/project_summary", endpoints.GetProjectSummary)
		handle(r, w, http.MethodGet, path+"/projects/project_full", endpoints.GetProjectFull)
	}
	projectEndpoints := endpoints.ProjectAgentEndpoints{}
	handle(r, w, http.MethodPost, path+"/projects/create_project", projectEndpoints.CreateProject)
	handle(r, w, http.MethodDelete, path+"/projects/delete_project", projectEndpoints.DeleteProject)
}

func environmentsRoutes(path string, r router, w wrapper, writeOnly bool) {
	if !writeOnly {
		handle(r, w, http.MethodGet, path+"/environment-summary", endpoints.GetEnvironmentSummary)
	}
}

func dbServerRoutes(path string, r router, w wrapper, writeOnly bool) {
	if !writeOnly {
		handle(r, w, http.MethodGet, path+"/dbserver-summary", endpoints.GetDbServerSummary)
		handle(r, w, http.MethodGet, path+"/dbserver-databases", endpoints.GetServerDatabases)
	}
	handle(r, w, http.MethodPost, path+"/dbserver-add", endpoints.AddDbServer)
	handle(r, w, http.MethodDelete, path+"/dbserver-delete", endpoints.DeleteDbServer)
}

func entitiesRoutes(path string, r router, w wrapper, writeOnly bool) {
	if !writeOnly {
		handle(r, w, http.MethodGet, path+"/entities/all_entities", endpoints.GetEntities)
		handle(r, w, http.MethodGet, path+"/entities/entity", endpoints.GetEntity)
	}
	handle(r, w, http.MethodPost, path+"/entities/create_entity", endpoints.SaveEntity)
	handle(r, w, http.MethodPut, path+"/entities/save_entity", endpoints.SaveEntity)
	handle(r, w, http.MethodDelete, path+"/entities/delete_entity", endpoints.DeleteEntity)
}

func recordsetsRoutes(path string, r router, w wrapper, writeOnly bool) {
	if !writeOnly {
		handle(r, w, http.MethodGet, path+"/recordsets/recordsets_summary", endpoints.GetRecordsetsSummary)
		handle(r, w, http.MethodGet, path+"/recordsets/recordset_definition", endpoints.GetRecordsetDefinition)
		handle(r, w, http.MethodGet, path+"/recordsets/recordset_data", endpoints.GetRecordsetData)
	}
	handle(r, w, http.MethodPost, path+"/recordsets/recordset_add_rows", endpoints.AddRowsToRecordset)
	handle(r, w, http.MethodPut, path+"/recordsets/recordset_update_rows", endpoints.UpdateRowsInRecordset)
	handle(r, w, http.MethodDelete, path+"/recordsets/recordset_delete_rows", endpoints.DeleteRowsFromRecordset)
}

func executeRoutes(path string, r router, w wrapper, writeOnly bool) {
	if !writeOnly {
		handle(r, w, http.MethodPost, path+"/exec/execute_commands", endpoints.ExecuteCommandsHandler)
		handle(r, w, http.MethodGet, path+"/exec/select", endpoints.ExecuteSelectHandler)
	}
}
