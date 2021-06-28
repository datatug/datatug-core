package routes

import (
	"github.com/julienschmidt/httprouter"
	"net/http"
)

type wrapper = func(f http.HandlerFunc) http.HandlerFunc

// RegisterDatatugWriteOnlyHandlers register write-only datatug handlers
func RegisterDatatugWriteOnlyHandlers(
	path string,
	router *httprouter.Router,
	wrap wrapper,
) {
	registerRoutes(path, router, wrap, true)
}

// RegisterAllDatatugHandlers registers all datatug handlers
func RegisterAllDatatugHandlers(path string, router *httprouter.Router, wrap wrapper) {
	registerRoutes(path, router, wrap, false)
}
