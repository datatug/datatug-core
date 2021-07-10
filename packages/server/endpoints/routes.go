package endpoints

import (
	"net/http"
	"strings"
)

type router interface {
	HandlerFunc(method, path string, handler http.HandlerFunc)
}

func registerRoutes(path string, router router, wrapper wrapper, writeOnly bool) {
	if router == nil {
		panic("router == nil")
	}
	path = strings.TrimRight(path, "/") + "/datatug"
	route(router, wrapper, http.MethodGet, path+"/ping", Ping)
	route(router, wrapper, http.MethodGet, path+"/agent-info", AgentInfo)
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

func foldersRoutes(path string, router router, wrap wrapper, writeOnly bool) {
	route(router, wrap, http.MethodPut, path+"/folders/create_folder", createFolder)
	route(router, wrap, http.MethodDelete, path+"/folders/delete_folder", deleteFolder)
}

func queriesRoutes(path string, router router, wrap wrapper, writeOnly bool) {
	if !writeOnly {
		//route(router, wrap, http.MethodGet, path+"/queries/all_queries", endpoints.GetQueries)
		route(router, wrap, http.MethodGet, path+"/queries/get_query", getQueryHandler)
	}
	route(router, wrap, http.MethodPost, path+"/queries/create_query", createQuery)
	route(router, wrap, http.MethodPut, path+"/queries/update_query", updateQuery)
	route(router, wrap, http.MethodDelete, path+"/queries/delete_query", deleteQuery)
}

func boardsRoutes(path string, router router, wrap wrapper, writeOnly bool) {
	if !writeOnly {
		route(router, wrap, http.MethodGet, path+"/boards/board", getBoard)
	}
	route(router, wrap, http.MethodPost, path+"/boards/create_board", createBoard)
	route(router, wrap, http.MethodPut, path+"/boards/save_board", saveBoard)
	route(router, wrap, http.MethodDelete, path+"/boards/delete_board", deleteBoard)
}

func projectsRoutes(path string, router router, wrap wrapper, writeOnly bool) {
	if !writeOnly {
		route(router, wrap, http.MethodGet, path+"/projects/projects_summary", getProjects)
		route(router, wrap, http.MethodGet, path+"/projects/project_summary", getProjectSummary)
		route(router, wrap, http.MethodGet, path+"/projects/project_full", getProjectFull)
	}
	projectEndpoints := ProjectAgentEndpoints{}
	route(router, wrap, http.MethodPost, path+"/projects/create_project", projectEndpoints.createProject)
	route(router, wrap, http.MethodDelete, path+"/projects/delete_project", projectEndpoints.deleteProject)
}

func environmentsRoutes(path string, router router, wrap wrapper, writeOnly bool) {
	if !writeOnly {
		route(router, wrap, http.MethodGet, path+"/environment-summary", getEnvironmentSummary)
	}
}

func dbServerRoutes(path string, router router, wrap wrapper, writeOnly bool) {
	if !writeOnly {
		route(router, wrap, http.MethodGet, path+"/dbserver-summary", getDbServerSummary)
		route(router, wrap, http.MethodGet, path+"/dbserver-databases", getServerDatabases)
	}
	route(router, wrap, http.MethodPost, path+"/dbserver-add", addDbServer)
	route(router, wrap, http.MethodDelete, path+"/dbserver-delete", deleteDbServer)
}

func entitiesRoutes(path string, router router, wrap wrapper, writeOnly bool) {
	if !writeOnly {
		route(router, wrap, http.MethodGet, path+"/entities/all_entities", getEntities)
		route(router, wrap, http.MethodGet, path+"/entities/entity", getEntity)
	}
	route(router, wrap, http.MethodPost, path+"/entities/create_entity", saveEntity)
	route(router, wrap, http.MethodPut, path+"/entities/save_entity", saveEntity)
	route(router, wrap, http.MethodDelete, path+"/entities/delete_entity", deleteEntity)
}

func recordsetsRoutes(path string, router router, wrap wrapper, writeOnly bool) {
	if !writeOnly {
		route(router, wrap, http.MethodGet, path+"/recordsets/recordsets_summary", getRecordsetsSummary)
		route(router, wrap, http.MethodGet, path+"/recordsets/recordset_definition", getRecordsetDefinition)
		route(router, wrap, http.MethodGet, path+"/recordsets/recordset_data", getRecordsetData)
	}
	route(router, wrap, http.MethodPost, path+"/recordsets/recordset_add_rows", addRowsToRecordset)
	route(router, wrap, http.MethodPut, path+"/recordsets/recordset_update_rows", updateRowsInRecordset)
	route(router, wrap, http.MethodDelete, path+"/recordsets/recordset_delete_rows", deleteRowsFromRecordset)
}

func executeRoutes(path string, router router, wrap wrapper, writeOnly bool) {
	if !writeOnly {
		route(router, wrap, http.MethodPost, path+"/exec/execute_commands", executeCommandsHandler)
		route(router, wrap, http.MethodGet, path+"/exec/select", executeSelectHandler)
	}
}
