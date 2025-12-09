package endpoints

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/strongo/validation"
)

func handleError(err error, w http.ResponseWriter, r *http.Request) bool {
	if err == nil {
		return false
	}
	_, _ = fmt.Fprintln(os.Stderr, err)
	//_, _ = fmt.Println("Error:", err)
	responseHeader := w.Header()
	origin := r.Header.Get("Origin")
	if origin != "" {
		responseHeader.Set("Access-Control-Allow-Origin", origin)
	}
	responseHeader.Set("Content-Type", "application/json")
	if validation.IsBadRequestError(err) {
		w.WriteHeader(http.StatusBadRequest)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
	encoder := json.NewEncoder(w)
	response := ErrorResponse{Error: err.Error()}
	if err2 := encoder.Encode(response); err2 != nil {
		log.Printf("Failed to encode error to response stream: %v.\nOriginal error: %v", err2, err)
	}
	return true
}

// ErrorResponse defines format of error response body
type ErrorResponse struct {
	Error string `json:"error"`
}
