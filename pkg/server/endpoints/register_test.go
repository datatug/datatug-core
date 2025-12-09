package endpoints

import (
	"context"
	"net/http"
	"testing"

	"github.com/julienschmidt/httprouter"
	"github.com/sneat-co/sneat-go-core/apicore"
	"github.com/sneat-co/sneat-go-core/apicore/verify"
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
		requestHandler := func(w http.ResponseWriter, r *http.Request, requestDTO apicore.RequestDTO,
			verifyOptions verify.RequestOptions, statusCode int,
			getContext apicore.ContextProvider,
			doWork apicore.Worker,
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
