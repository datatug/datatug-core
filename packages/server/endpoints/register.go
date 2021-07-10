package endpoints

import (
	"context"
	"github.com/julienschmidt/httprouter"
	"log"
	"net/http"
)

type wrapper = func(f http.HandlerFunc) http.HandlerFunc

type Mode = int

const (
	WriteOnlyHandlers Mode = iota
	AllHandlers
)

// RegisterDatatugHandlers registers datatug HTTP handlers
func RegisterDatatugHandlers(
	path string,
	router *httprouter.Router,
	mode Mode,
	wrap wrapper,
	contextProvider func(r *http.Request) (context.Context, error),
	handler Handler,
) {
	log.Println("Registering DataTug handlers on path:", path)
	if handler == nil {
		panic("handler is not provided")
	}
	handle = handler
	getContextFromRequest = contextProvider
	registerRoutes(path, router, wrap, mode == WriteOnlyHandlers)
}
