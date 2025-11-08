package middleware

import (
	"net/http"
	"os"
	"strings"
)

// SecurityHeaders adds security headers to all HTTP responses
type SecurityHeaders struct {
	next http.Handler
}

// NewSecurityHeaders creates a new security headers middleware
func NewSecurityHeaders(next http.Handler) *SecurityHeaders {
	return &SecurityHeaders{next: next}
}

// ServeHTTP implements the http.Handler interface
func (sh *SecurityHeaders) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Content Security Policy - prevent XSS attacks
	// Use environment-aware CSP: strict in production, permissive only for Vite dev server
	environment := strings.ToLower(os.Getenv("ENVIRONMENT"))
	if environment == "" {
		environment = "development"
	}
	isDev := environment == "development" || environment == "dev"

	var scriptSrc, styleSrc string
	if isDev {
		// Development: Allow inline scripts and eval for Vite HMR (hot module reload)
		scriptSrc = "'self' 'unsafe-inline' 'unsafe-eval'"
		styleSrc = "'self' 'unsafe-inline'"
	} else {
		// Production: Strict CSP - no inline scripts or eval
		scriptSrc = "'self'"
		// Keep unsafe-inline for styles (color swatches) - low security risk
		styleSrc = "'self' 'unsafe-inline'"
	}

	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; "+
			"script-src "+scriptSrc+"; "+
			"style-src "+styleSrc+"; "+
			"img-src 'self' data: blob:; "+
			"font-src 'self' data:; "+
			"connect-src 'self' ws: wss:; "+
			"frame-ancestors 'none'")

	// Prevent clickjacking attacks
	w.Header().Set("X-Frame-Options", "DENY")

	// Prevent MIME type sniffing
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// Enable browser XSS protection
	w.Header().Set("X-XSS-Protection", "1; mode=block")

	// Strict Transport Security - enforce HTTPS (31536000 = 1 year)
	// Only add HSTS if request is already HTTPS
	if r.TLS != nil {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	}

	// Referrer Policy - limit information leakage
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

	// Permissions Policy - restrict browser features
	w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

	if sh.next != nil {
		sh.next.ServeHTTP(w, r)
	}
}

// WrapHandler wraps an http.Handler with security headers
func WrapHandler(handler http.Handler) http.Handler {
	return NewSecurityHeaders(handler)
}

// WrapHandlerFunc wraps an http.HandlerFunc with security headers
func WrapHandlerFunc(handlerFunc http.HandlerFunc) http.Handler {
	return NewSecurityHeaders(handlerFunc)
}
