package routes

import (
	"github.com/julienschmidt/httprouter"
	"net/http"
)

type wrapper = func(f http.HandlerFunc) http.HandlerFunc

func RegisterDatatugWriteOnlyHandlers(
	path string,
	router *httprouter.Router,
	wrap wrapper,
) {
	registerRoutes(path, router, wrap, true)
}

func RegisterAllDatatugHandlers(path string, router *httprouter.Router, wrap wrapper) {
	registerRoutes(path, router, wrap, false)
}
