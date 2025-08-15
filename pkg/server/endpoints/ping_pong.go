package endpoints

import "net/http"

// Ping return "pong" - is a simplest
func Ping(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("pong"))
}
