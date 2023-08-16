package endpoints

import (
	"context"
	"github.com/sneat-co/sneat-go-core/apicore"
	"log"
	"net/http"
)

// ProjectEndpoints defines project endpoints
type ProjectEndpoints interface {
	CreateProject(w http.ResponseWriter, r *http.Request)
	DeleteProject(w http.ResponseWriter, r *http.Request)
}

// getContextFromRequest creates context for a given request
// We need a request for example to decode Firebase token
// Probably can be done better then being hard-depending on an *http.Request
var getContextFromRequest = func(r *http.Request) (context.Context, error) {
	return r.Context(), nil
}

// VerifyRequest implements VerifyRequestOptions
type VerifyRequest struct { // TODO: move to shared Sneat package
	MinContentLength int64
	MaxContentLength int64
	AuthRequired     bool
}

// MinimumContentLength defines min content length
func (v VerifyRequest) MinimumContentLength() int64 {
	return v.MinContentLength
}

// MaximumContentLength defines max content length, if < 0 no limit
func (v VerifyRequest) MaximumContentLength() int64 {
	return v.MaxContentLength
}

// AuthenticationRequired specifies if authentication is mandatory
func (v VerifyRequest) AuthenticationRequired() bool {
	return v.AuthRequired
}

// Handler is responsible for creating context and call `handler()` func that should use
// provided context along with `requestDTO` that was populated from request body
// Its is exposed publicly so it can be replaced with custom implementation
type Handler = func(
	w http.ResponseWriter,
	r *http.Request,
	requestDTO apicore.RequestDTO,
	verifyOptions apicore.VerifyRequestOptions,
	successStatusCode int,
	getContext apicore.ContextProvider,
	handler apicore.Worker,
)

//goland:noinspection GoVarAndConstTypeMayBeOmitted
var handle Handler = func(
	w http.ResponseWriter,
	r *http.Request,
	requestDTO apicore.RequestDTO,
	verifyOptions apicore.VerifyRequestOptions,
	successStatusCode int,
	getContext apicore.ContextProvider,
	doWork apicore.Worker,
) {
	panic("handle is not initialized properly")
}

func route(r router, wrap wrapper, method, path string, handler http.HandlerFunc) {
	log.Printf("Registering %v %v", method, path)
	if wrap != nil {
		handler = wrap(handler)
	}
	r.HandlerFunc(method, path, handler)
}
