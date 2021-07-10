package server

import (
	"fmt"
	"github.com/datatug/datatug/packages/server/endpoints"
	"net/http"
)

// globalOptionsHandler handles OPTIONS requests
func globalOptionsHandler(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	accessControlRequestMethod := r.Header.Get("Access-Control-Request-Method")
	if origin == "" || accessControlRequestMethod == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintln(w, "origin: ", origin)
		_, _ = fmt.Fprintln(w, "accessControlRequestMethod: ", accessControlRequestMethod)
		return
	}
	if !endpoints.IsSupportedOrigin(origin) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprintf(w, "Unsupported origin: %v", origin)
		return
	}
	// Set CORS headers BEFORE calling w.WriteHeader() or w.Write()
	responseHeader := w.Header()
	responseHeader.Set("Access-Control-Allow-Origin", origin)
	responseHeader.Set("Access-Control-Allow-Methods", accessControlRequestMethod)
	accessControlRequestHeaders := r.Header.Get("Access-Control-Request-Headers")
	if accessControlRequestHeaders != "" {
		responseHeader.Set("Access-Control-Allow-Headers", accessControlRequestHeaders)
	}
	w.WriteHeader(http.StatusNoContent) // Set response status code to 204
}
