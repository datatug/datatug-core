package endpoints

import (
	"context"
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
		RegisterDatatugHandlers("", nil, WriteOnlyHandlers, nil, nil, nil)
	})

	t.Run("should_pass", func(t *testing.T) {
		requestHandler := func(w http.ResponseWriter, r *http.Request, requestDTO RequestDTO,
			verifyOptions VerifyRequestOptions, statusCode int,
			handler func(ctx context.Context) (responseDTO ResponseDTO, err error),
		) {
			handler(r.Context())
		}
		RegisterDatatugHandlers("", httprouter.New(), AllHandlers, nil, func(r *http.Request) (context.Context, error) {
			return r.Context(), nil
		}, requestHandler)
	})
}
