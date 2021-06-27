package routes

import (
	"github.com/datatug/datatug/packages/server/endpoints"
	"github.com/julienschmidt/httprouter"
	"log"
	"net/http"
)

var handlerFunc = func(r *httprouter.Router, method, path string, handler http.HandlerFunc) {
	wrappedHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agent-info" {
			log.Println(method, r.RequestURI)
		}
		handler(w, r)
	}
	r.HandlerFunc(method, path, wrappedHandler)
}

func queriesRoutes(path string, r *httprouter.Router, writeOnly bool) {
	if !writeOnly {
		handlerFunc(r, http.MethodGet, path+"/datatug/queries/all_queries", endpoints.GetQueries)
		handlerFunc(r, http.MethodGet, path+"/datatug/queries/get_query", endpoints.GetQuery)
	}
	handlerFunc(r, http.MethodPut, path+"/datatug/queries/create_folder", endpoints.CreateQueryFolder)
	handlerFunc(r, http.MethodPost, path+"/datatug/queries/create_query", endpoints.CreateQuery)
	handlerFunc(r, http.MethodPut, path+"/datatug/queries/update_query", endpoints.UpdateQuery)
	handlerFunc(r, http.MethodDelete, path+"/datatug/queries/delete_query", endpoints.DeleteQuery)
	handlerFunc(r, http.MethodDelete, path+"/datatug/queries/delete_folder", endpoints.DeleteQueryFolder)
}

func boardsRoutes(path string, r *httprouter.Router, writeOnly bool) {
	if !writeOnly {
		handlerFunc(r, http.MethodGet, path+"/datatug/boards/board", endpoints.GetBoard)
	}
	handlerFunc(r, http.MethodPost, path+"/datatug/boards/create_board", endpoints.CreateBoard)
	handlerFunc(r, http.MethodPut, path+"/datatug/boards/save_board", endpoints.SaveBoard)
	handlerFunc(r, http.MethodDelete, path+"/datatug/boards/delete_board", endpoints.DeleteBoard)
}

func projectsRoutes(path string, r *httprouter.Router, writeOnly bool) {
	if !writeOnly {
		handlerFunc(r, http.MethodGet, path+"/datatug/projects/projects-summary", endpoints.GetProjects)
		handlerFunc(r, http.MethodGet, path+"/datatug/projects/project-summary", endpoints.GetProjectSummary)
		handlerFunc(r, http.MethodGet, path+"/datatug/projects/project-full", endpoints.GetProjectFull)
	}
	projectEndpoints := endpoints.ProjectAgentEndpoints{}
	handlerFunc(r, http.MethodPost, path+"/datatug/projects/create-project", projectEndpoints.CreateProject)
	handlerFunc(r, http.MethodDelete, path+"/datatug/projects/create-project", projectEndpoints.DeleteProject)
}

func environmentsRoutes(path string, r *httprouter.Router, writeOnly bool) {
	if !writeOnly {
		handlerFunc(r, http.MethodGet, path+"/datatug/environment-summary", endpoints.GetEnvironmentSummary)
	}
}

func dbServerRoutes(path string, r *httprouter.Router, writeOnly bool) {
	if !writeOnly {
		handlerFunc(r, http.MethodGet, path+"/datatug/dbserver-summary", endpoints.GetDbServerSummary)
		handlerFunc(r, http.MethodGet, path+"/datatug/dbserver-databases", endpoints.GetServerDatabases)
	}
	handlerFunc(r, http.MethodPost, path+"/datatug/dbserver-add", endpoints.AddDbServer)
	handlerFunc(r, http.MethodDelete, path+"/datatug/dbserver-delete", endpoints.DeleteDbServer)
}

func entitiesRoutes(path string, r *httprouter.Router, writeOnly bool) {
	if !writeOnly {
		handlerFunc(r, http.MethodGet, path+"/datatug/entities/all_entities", endpoints.GetEntities)
		handlerFunc(r, http.MethodGet, path+"/datatug/entities/entity", endpoints.GetEntity)
	}
	handlerFunc(r, http.MethodPost, path+"/datatug/entities/create_entity", endpoints.SaveEntity)
	handlerFunc(r, http.MethodPut, path+"/datatug/entities/save_entity", endpoints.SaveEntity)
	handlerFunc(r, http.MethodDelete, path+"/datatug/entities/delete_entity", endpoints.DeleteEntity)
}

func dataRoutes(path string, r *httprouter.Router, writeOnly bool) {
	if !writeOnly {
		handlerFunc(r, http.MethodGet, path+"/datatug/data/recordsets_summary", endpoints.GetRecordsetsSummary)
		handlerFunc(r, http.MethodGet, path+"/datatug/data/recordset_definition", endpoints.GetRecordsetDefinition)
		handlerFunc(r, http.MethodGet, path+"/datatug/data/recordset_data", endpoints.GetRecordsetData)
	}
	handlerFunc(r, http.MethodPost, path+"/datatug/data/recordset_add_rows", endpoints.AddRowsToRecordset)
	handlerFunc(r, http.MethodPut, path+"/datatug/data/recordset_update_rows", endpoints.UpdateRowsInRecordset)
	handlerFunc(r, http.MethodDelete, path+"/datatug/data/recordset_delete_rows", endpoints.DeleteRowsFromRecordset)
}

func executeRoutes(path string, r *httprouter.Router, writeOnly bool) {
	if !writeOnly {
		handlerFunc(r, http.MethodPost, path+"/datatug/exec/execute_commands", endpoints.ExecuteCommandsHandler)
		handlerFunc(r, http.MethodGet, path+"/datatug/exec/select", endpoints.ExecuteSelectHandler)
	}
}
