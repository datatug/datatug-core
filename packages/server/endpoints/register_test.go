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
		RegisterDatatugHandlers("", nil, RegisterWriteOnlyHandlers, nil, nil, nil)
	})

	t.Run("should_pass", func(t *testing.T) {
		requestHandler := func(w http.ResponseWriter, r *http.Request, requestDTO RequestDTO,
			verifyOptions VerifyRequestOptions, statusCode int,
			getContext ContextProvider,
			doWork Worker,
		) {
			ctx, err := getContext(r)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = doWork(ctx); err != nil {
				t.Fatal(err)
			}
		}
		RegisterDatatugHandlers("", httprouter.New(), RegisterAllHandlers, nil, func(r *http.Request) (context.Context, error) {
			return r.Context(), nil
		}, requestHandler)
	})
}
