package endpoints

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"log"
	"net/http"
)

// ReturnJSON returns provided response as a JSON and sets "Content-Type" as "application/json"
func ReturnJSON(w http.ResponseWriter, r *http.Request, statusCode int, err error, content interface{}) {
	if handleError(err, w, r) {
		return
	}
	responseHeader := w.Header()
	origin := r.Header.Get("Origin")
	if origin != "" {
		responseHeader.Set("Access-Control-Allow-Origin", origin)
	}
	switch r.Method {
	case http.MethodOptions:
		panic("An attempt to return JSON content on OPTIONS request")
	case http.MethodGet:
		writeResponseJSONForGetRequestWithCacheControl(w, r, statusCode, content)
		return
	default:
		responseHeader.Add("Content-Type", "application/json")
		if err = json.NewEncoder(w).Encode(content); err != nil {
			log.Printf("failed to encode response content to JSON: %v", err)
			return
		}
		return
	}
}

func writeResponseJSONForGetRequestWithCacheControl(w http.ResponseWriter, r *http.Request, statusCode int, content interface{}) {
	// For GET request we encode to a buffer first so we can calculate ETag for HTTP caching
	var buffer bytes.Buffer
	if err := json.NewEncoder(&buffer).Encode(content); err != nil {
		log.Printf("failed to encode response content to JSON: %v", err)
		return
	}
	encoded := buffer.Bytes()
	checksum := generateEtagValue(encoded)
	if eTag := r.Header.Get("If-None-Match"); eTag == checksum {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	responseHeader := w.Header()
	responseHeader.Add("Content-Type", "application/json")
	// Cache only in browser for 60 secs * 60 minutes * 24 hours * 7 days = 604800
	responseHeader.Add("Cache-Control", "private; max-age=604800")
	responseHeader.Add("ETag", checksum)
	w.WriteHeader(statusCode)
	if _, err := w.Write(encoded); err != nil {
		log.Printf("failed to write content to response stream: %v", err)
	}
}

func generateEtagValue(data []byte) string {
	return fmt.Sprintf(`"%X"`, crc32.ChecksumIEEE(data)) // Double quotes are required by HTTP standard
}
