package server

import (
	"net/http"
	"strings"
	"time"

	"main/internal/config"
	"main/internal/logger"
)

// New creates and configures an http.Server based on configuration
func New(cfg *config.Config) *http.Server {
	// Enforce TLS in production unless behind a proxy
	if cfg.IsProduction && !cfg.UseTLS && !cfg.BehindProxy {
		logger.Fatal("TLS is required in production mode").
			Str("environment", "production").
			Msg("Set TLS_CERT_FILE and TLS_KEY_FILE environment variables, or set BEHIND_TLS_PROXY=true")
	}

	// Configure server address and timeouts
	addr := ":8080"
	if cfg.UseTLS {
		addr = ":8443"
	}

	return &http.Server{
		Addr:         addr,
		Handler:      nil, // Uses DefaultServeMux
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

// NewRedirectServer creates an HTTP to HTTPS redirect server
// Only used in production with TLS enabled
func NewRedirectServer() *http.Server {
	return &http.Server{
		Addr:         ":8080",
		Handler:      http.HandlerFunc(redirectToHTTPS),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
}

// redirectToHTTPS redirects HTTP requests to HTTPS
func redirectToHTTPS(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	// Remove port if present (strip :8080)
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}
	// Redirect to HTTPS on port 8443
	target := "https://" + host + ":8443" + r.URL.Path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// Start begins serving requests (blocking)
func Start(server *http.Server, cfg *config.Config, errChan chan<- error) {
	if cfg.UseTLS {
		logger.Info("Server started").
			Str("addr", server.Addr).
			Str("protocol", "HTTPS/WSS").
			Msg("Press Ctrl+C to shutdown gracefully")
		errChan <- server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
	} else {
		logger.Info("Server started").
			Str("addr", server.Addr).
			Str("protocol", "HTTP/WS").
			Msg("Press Ctrl+C to shutdown gracefully")
		logger.Warn("TLS not enabled").
			Str("environment", cfg.Environment).
			Msg("Use ENVIRONMENT=production with TLS_CERT_FILE and TLS_KEY_FILE for production")
		errChan <- server.ListenAndServe()
	}
}

// StartRedirectServer starts the HTTP to HTTPS redirect server (blocking)
func StartRedirectServer(server *http.Server, errChan chan<- error) {
	logger.Info("HTTP redirect server started").
		Str("from", ":8080").
		Str("to", ":8443").
		Msg("")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("Redirect server error").
			Err(err).
			Msg("")
		if errChan != nil {
			errChan <- err
		}
	}
}
