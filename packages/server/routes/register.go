package routes

import (
	"github.com/datatug/datatug/packages/server/endpoints"
	"github.com/julienschmidt/httprouter"
	"net/http"
	"strings"
)

func RegisterDatatugWriteOnlyHandlers(path string, router *httprouter.Router) {
	registerRoutes(path, router, true)
}

func registerRoutes(path string, router *httprouter.Router, writeOnly bool) {
	if router == nil {
		panic("router == nil")
	}
	path = strings.TrimRight(path, "/")
	if !writeOnly {
		handlerFunc(router, http.MethodGet, path+"/ping", endpoints.Ping)
		handlerFunc(router, http.MethodGet, path+"/agent-info", endpoints.AgentInfo)
	}
	projectsRoutes(path, router, writeOnly)
	queriesRoutes(path, router, writeOnly)
	boardsRoutes(path, router, writeOnly)
	environmentsRoutes(path, router, writeOnly)
	dbServerRoutes(path, router, writeOnly)
	entitiesRoutes(path, router, writeOnly)
	dataRoutes(path, router, writeOnly)
	executeRoutes(path, router, writeOnly)

}

func RegisterAllDatatugHandlers(path string, router *httprouter.Router) {
	registerRoutes(path, router, false)
}
