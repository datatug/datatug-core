package endpoints

import (
	"context"
	"net/http"
)

// ProjectEndpoints defines project endpoints
type ProjectEndpoints interface {
	CreateProject(w http.ResponseWriter, r *http.Request)
	DeleteProject(w http.ResponseWriter, r *http.Request)
}

var Context = func(w http.ResponseWriter, r *http.Request) (context.Context, error) {
	return r.Context(), nil
}

// RequestDTO defines an interface that should be implemented by request DTO struct
type RequestDTO interface {
	Validate() error
}

// ResponseDTO common interface for response objects
type ResponseDTO interface {
	// Validate validates response
	Validate() error
}

// VerifyRequestOptions - options for request verification
type VerifyRequestOptions interface {
	MinContentLength() int64
	MaxContentLength() int64
	AuthRequired() bool
}

var _ VerifyRequestOptions = (*verifyRequestOptions)(nil)

type verifyRequestOptions struct {
	minContentLength int64
	maxContentLength int64
	authRequired     bool
}

func (v verifyRequestOptions) MinContentLength() int64 {
	return v.minContentLength
}

func (v verifyRequestOptions) MaxContentLength() int64 {
	return v.maxContentLength
}

func (v verifyRequestOptions) AuthRequired() bool {
	return v.authRequired
}

// Handler is responsible for creating context and call `handler()` func that should use
// provided context along with `requestDTO` that was populated from request body
// Its is exposed publicly so it can be replaced with custom implementation
type Handler = func(
	w http.ResponseWriter,
	r *http.Request,
	requestDTO RequestDTO,
	verifyOptions VerifyRequestOptions,
	statusCode int,
	handler func(ctx context.Context) (responseDTO ResponseDTO, err error),
)

func SetHandler(handler Handler) {
	if handler == nil {
		panic("handler is not provided")
	}
	handle = handler
}

var handle Handler = func(w http.ResponseWriter, r *http.Request, requestDTO RequestDTO, verifyOptions VerifyRequestOptions, statusCode int, handler func(ctx context.Context) (responseDTO ResponseDTO, err error)) {
	panic("not initialized properly")
}
