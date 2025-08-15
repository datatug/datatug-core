package endpoints

import (
	"context"
	"github.com/julienschmidt/httprouter"
	"log"
	"net/http"
)

type wrapper = func(f http.HandlerFunc) http.HandlerFunc

type RegisterMode = int

const (
	RegisterWriteOnlyHandlers RegisterMode = iota
	RegisterAllHandlers
)

// RegisterDatatugHandlers registers datatug HTTP handlers
func RegisterDatatugHandlers(
	pathPrefix string,
	router *httprouter.Router,
	mode RegisterMode,
	wrap wrapper,
	contextProvider func(r *http.Request) (context.Context, error),
	handler Handler,
) {
	log.Println("Registering DataTug handlers on pathPrefix:", pathPrefix)
	if handler == nil {
		panic("handler is not provided")
	}
	handle = handler
	getContextFromRequest = contextProvider
	registerRoutes(pathPrefix, router, wrap, mode == RegisterWriteOnlyHandlers)
}
