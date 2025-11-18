package middleware

import (
	"net/http"
	"strings"
)


type CORS struct {
    allowedOrigins map[string]bool 
}

func NewCORS(origins []string) *CORS {
    allowed := make(map[string]bool)
    for _, origin := range origins {
        allowed[strings.TrimSpace(origin)] = true
    }
    return &CORS{allowedOrigins: allowed}
}

// ServeHTTP implements the http.Handler interface
func (c *CORS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")

	// Check if origin is allowed
	allowed := c.allowedOrigins[origin]

	if allowed {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Cookie")
		w.Header().Set("Access-Control-Max-Age", "3600") // Cache preflight for 1 hour
	}

	// Handle preflight OPTIONS request
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

}

// WrapWithCORS wraps an http.Handler with CORS headers
func WrapWithCORS(handler http.Handler, allowedOrigins []string) http.Handler {
	return NewCORS(allowedOrigins)
}
