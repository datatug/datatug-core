package routes

import (
	"context"
	"github.com/datatug/datatug/packages/server/endpoints"
	"github.com/julienschmidt/httprouter"
	"net/http"
	"testing"
)

func TestRegisterDatatugWriteOnlyHandlers(t *testing.T) {
	t.Run("should_fail", func(t *testing.T) {
		defer func() {
			if err := recover(); err == nil {
				t.Fatal("a panic expected for nil router")
			}
		}()
		RegisterDatatugHandlers("", nil, WriteOnlyHandlers, nil, nil)
	})

	t.Run("should_pass", func(t *testing.T) {
		contextProvider := func(w http.ResponseWriter, r *http.Request, requestDTO endpoints.RequestDTO,
			verifyOptions endpoints.VerifyRequestOptions, statusCode int,
			handler func(ctx context.Context) (responseDTO endpoints.ResponseDTO, err error),
		) {
			handler(r.Context())
		}
		RegisterDatatugHandlers("", httprouter.New(), AllHandlers, nil, contextProvider)
	})
}
