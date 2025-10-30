package middleware

import (
	"net/http"
	"strings"
)

// CORS middleware adds CORS headers for cross-origin requests
type CORS struct {
	next           http.Handler
	allowedOrigins []string
}

// NewCORS creates a new CORS middleware
func NewCORS(next http.Handler, allowedOrigins []string) *CORS {
	return &CORS{
		next:           next,
		allowedOrigins: allowedOrigins,
	}
}

// ServeHTTP implements the http.Handler interface
func (c *CORS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")

	// Check if origin is allowed
	allowed := false
	for _, allowedOrigin := range c.allowedOrigins {
		if origin == strings.TrimSpace(allowedOrigin) {
			allowed = true
			break
		}
	}

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

	if c.next != nil {
		c.next.ServeHTTP(w, r)
	}
}

// WrapWithCORS wraps an http.Handler with CORS headers
func WrapWithCORS(handler http.Handler, allowedOrigins []string) http.Handler {
	return NewCORS(handler, allowedOrigins)
}
