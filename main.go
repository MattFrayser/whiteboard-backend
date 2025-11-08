package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"main/internal/handlers"
	"main/internal/logger"
	"main/internal/middleware"
	"main/internal/object"
	"main/internal/room"
	"main/internal/user"
	"main/internal/websocket"

	"github.com/joho/godotenv"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	godotenv.Load()

	// Initialize structured logging
	logger.Init()

	// Parse allowed domains from environment
	allowedDomains := strings.Split(os.Getenv("DOMAINS"), ",")

	// Determine environment mode
	environment := os.Getenv("ENVIRONMENT")
	if environment == "" {
		environment = "development"
	}
	isProduction := environment == "production"

	// Initialize rate limiting configuration
	config := middleware.NewRateLimit(
		10,     // maxRoomSize
		1000,   // maxObjects
		250000, // maxMessageSize (250KB)
		100,    // maxRooms
		5,      // maxObjectDepth
		1000,   // maxObjectElements (unique keys)
		30,     // messagesPerSecond
		10,     // burstSize
	)

	// Initialize managers
	ipRateLimiter := middleware.NewIPRateLimit()
	connTracker := middleware.NewConnectionTracker(1000) // Max 1000 concurrent connections
	connRegistry := transport.NewConnectionRegistry()    // Track active WebSocket connections for graceful shutdown
	sessionMgr := user.NewSessionManager()
	validator := object.NewValidator()
	roomMgr := room.NewManager()
	broadcaster := room.NewBroadcaster()
	synchronizer := room.NewSynchronizer()
	msgRouter := handlers.NewMessageRouter(validator, config, sessionMgr, broadcaster, synchronizer)
	authenticator := transport.NewAuthenticator(sessionMgr)

	// Create WebSocket configuration with all dependencies
	wsConfig := transport.NewWebSocketConfig(
		allowedDomains,
		ipRateLimiter,
		config,
		sessionMgr,
		validator,
		roomMgr,
		msgRouter,
		synchronizer,
		authenticator,
		connTracker,
		connRegistry,
	)

	// Setup HTTP handlers with security headers middleware
	fileServer := middleware.WrapHandler(http.FileServer(http.Dir("./frontend")))
	http.Handle("/", fileServer)

	// Session establishment endpoint (called before WebSocket connection)
	sessionHandler := middleware.WrapHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check IP rate limit for session endpoint
		clientIP := transport.GetClientIP(r)
		if !ipRateLimiter.Allow(clientIP) {
			logger.Warn("Session endpoint rate limit exceeded").
				Str("ip", clientIP).
				Msg("")
			http.Error(w, "Too many requests. Please try again later.", http.StatusTooManyRequests)
			return
		}
		transport.HandleSession(w, r, sessionMgr)
	})
	// Apply CORS to session endpoint for cross-origin requests (e.g., localhost:5173 -> localhost:8080)
	sessionHandlerWithCORS := middleware.WrapWithCORS(sessionHandler, allowedDomains)
	http.Handle("/api/session", sessionHandlerWithCORS)

	wsHandler := middleware.WrapHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		transport.HandleWebSocket(w, r, wsConfig)
	})
	http.Handle("/ws", wsHandler)

	// Start periodic cleanups
	go cleanupRooms(ctx, roomMgr)
	go cleanupSessions(ctx, sessionMgr)
	go cleanupIPLimiters(ctx, ipRateLimiter)

	// Check if TLS is enabled
	certFile := os.Getenv("TLS_CERT_FILE")
	keyFile := os.Getenv("TLS_KEY_FILE")
	useTLS := certFile != "" && keyFile != ""

	// Enforce TLS in production
	if isProduction && !useTLS {
		logger.Fatal("TLS is required in production mode").
			Str("environment", "production").
			Msg("Set TLS_CERT_FILE and TLS_KEY_FILE environment variables")
	}

	// Create http.Server instance for graceful shutdown support
	var server *http.Server
	if useTLS {
		server = &http.Server{
			Addr:         ":8443",
			Handler:      nil, // Uses DefaultServeMux
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
	} else {
		server = &http.Server{
			Addr:         ":8080",
			Handler:      nil, // Uses DefaultServeMux
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start HTTP to HTTPS redirect server in production
	var redirectServer *http.Server
	if useTLS && isProduction {
		redirectServer = &http.Server{
			Addr:         ":8080",
			Handler:      http.HandlerFunc(redirectToHTTPS),
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		}
		go func() {
			logger.Info("HTTP redirect server started").
				Str("from", ":8080").
				Str("to", ":8443").
				Msg("")
			if err := redirectServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("Redirect server error").
					Err(err).
					Msg("")
			}
		}()
	}

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		if useTLS {
			logger.Info("Server started").
				Str("addr", ":8443").
				Str("protocol", "HTTPS/WSS").
				Msg("Press Ctrl+C to shutdown gracefully")
			serverErr <- server.ListenAndServeTLS(certFile, keyFile)
		} else {
			logger.Info("Server started").
				Str("addr", ":8080").
				Str("protocol", "HTTP/WS").
				Msg("Press Ctrl+C to shutdown gracefully")
			logger.Warn("TLS not enabled").
				Str("environment", environment).
				Msg("Use ENVIRONMENT=production with TLS_CERT_FILE and TLS_KEY_FILE for production")
			serverErr <- server.ListenAndServe()
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case <-sigChan:
		logger.Info("Shutdown signal received").
			Msg("Initiating graceful shutdown")
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server error").
				Err(err).
				Msg("")
		}
		return
	}

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Shutdown HTTP servers (stops accepting new connections)
	logger.Info("Stopping HTTP server").Msg("")
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown error").
			Err(err).
			Msg("")
	}

	// Shutdown redirect server if running
	if redirectServer != nil {
		logger.Info("Stopping HTTP redirect server").Msg("")
		if err := redirectServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("Redirect server shutdown error").
				Err(err).
				Msg("")
		}
	}

	// Close all WebSocket connections gracefully
	logger.Info("Closing all WebSocket connections").Msg("")
	wsShutdownCtx, wsShutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer wsShutdownCancel()
	connRegistry.CloseAll(wsShutdownCtx)

	// Cancel context to stop cleanup goroutines
	logger.Info("Stopping background cleanup routines").Msg("")
	cancel()

	// Wait a moment for cleanup goroutines to exit gracefully
	time.Sleep(100 * time.Millisecond)

	logger.Info("Shutdown complete").Msg("")
}

// cleanupRooms: periodically removes expired rooms
func cleanupRooms(ctx context.Context, roomMgr *room.Manager) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			roomMgr.Cleanup()
			logger.Debug("Cleaned up expired rooms").Msg("")
		}
	}
}

// cleanupSessions: periodically removes expired user sessions
func cleanupSessions(ctx context.Context, sessionMgr *user.SessionManager) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sessionMgr.Cleanup()
			logger.Debug("Cleaned up expired sessions").Msg("")
		}
	}
}

// cleanupIPLimiters: periodically clears IP rate limiters
func cleanupIPLimiters(ctx context.Context, ipRateLimiter *middleware.IPRateLimit) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ipRateLimiter.Cleanup()
			logger.Debug("IP rate limiters cleared").Msg("")
		}
	}
}

// redirectToHTTPS: redirects HTTP requests to HTTPS
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
