package routes

import "strings"

// IsSupportedOrigin check provided origin is allowed
func IsSupportedOrigin(origin string) bool {
	if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "https://localhost:") {
		return true
	}
	switch origin {
	case "https://datatug.app":
		return true
	default:
		return strings.HasPrefix(origin, "https://") && strings.HasSuffix(origin, ".datatug.app")
	}
}
