package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"main/internal/cleanup"
	"main/internal/config"
	"main/internal/handlers"
	"main/internal/logger"
	"main/internal/middleware"
	"main/internal/object"
	"main/internal/room"
	"main/internal/server"
	"main/internal/user"
	"main/internal/websocket"
)

// Dependencies holds all initialized application dependencies
type Dependencies struct {
	SessionMgr    *user.SessionManager
	IPRateLimiter *middleware.IPRateLimit
	RoomMgr       *room.Manager
	ConnRegistry  *transport.ConnectionRegistry
	ConnTracker   *middleware.ConnectionTracker
	WSConfig      *transport.WebSocketConfig
}

// initializeDependencies initializes and wires up all application dependencies
func initializeDependencies(cfg *config.Config) *Dependencies {
	// Initialize rate limiting configuration
	rateLimitConfig := middleware.NewRateLimit()

	// Initialize all managers and dependencies
	ipRateLimiter := middleware.NewIPRateLimit()
	connTracker := middleware.NewConnectionTracker()
	connRegistry := transport.NewConnectionRegistry()
	sessionMgr := user.NewSessionManager()
	validator := object.NewValidator()
	roomMgr := room.NewManager()
	broadcaster := room.NewBroadcaster()
	synchronizer := room.NewSynchronizer()
	msgRouter := handlers.NewMessageRouter(validator, rateLimitConfig, sessionMgr, broadcaster, synchronizer)
	authenticator := transport.NewAuthenticator(sessionMgr)

	// Create WebSocket configuration with required dependencies and optional features
	wsConfig := transport.NewWebSocketConfig(
		cfg.AllowedDomains,
		sessionMgr,
		roomMgr,
		msgRouter,
		transport.WithIPRateLimiter(ipRateLimiter),
		transport.WithRateLimit(rateLimitConfig),
		transport.WithValidator(validator),
		transport.WithSynchronizer(synchronizer),
		transport.WithAuthenticator(authenticator),
		transport.WithConnectionTracker(connTracker),
		transport.WithConnectionRegistry(connRegistry),
		transport.WithBehindProxy(cfg.BehindProxy), // Trust Proxy
	)

	return &Dependencies{
		SessionMgr:    sessionMgr,
		IPRateLimiter: ipRateLimiter,
		RoomMgr:       roomMgr,
		ConnRegistry:  connRegistry,
		ConnTracker:   connTracker,
		WSConfig:      wsConfig,
	}
}

func main() {
	// Create cancellable context for cleanup goroutines
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load configuration from environment
	cfg := config.Load()

	// Initialize structured logging
	logger.Init()

	// Initialize all application dependencies
	deps := initializeDependencies(cfg)

	// Setup HTTP routes
	setupRoutes(cfg, deps)

	// Start periodic cleanup tasks
	go cleanup.StartRoomCleanup(ctx, deps.RoomMgr)
	go cleanup.StartSessionCleanup(ctx, deps.SessionMgr)
	go cleanup.StartIPLimiterCleanup(ctx, deps.IPRateLimiter)


	startServer(cfg, deps)

	// Cancel context to stop cleanup goroutines
	logger.Info("Stopping background cleanup routines").Msg("")
	cancel()

	// Wait a moment for cleanup goroutines to exit gracefully
	time.Sleep(100 * time.Millisecond)

	logger.Info("Shutdown complete").Msg("")
}

func startServer(cfg *config.Config, deps *Dependencies) {

	// Create main HTTP server
	mainServer := server.New(cfg)

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start HTTP to HTTPS redirect server in production with TLS
	var redirectServer *http.Server
	if cfg.UseTLS && cfg.IsProduction {
		redirectServer = server.NewRedirectServer()
		go server.StartRedirectServer(redirectServer, nil)
	}

	// Start main server in goroutine
	serverErr := make(chan error, 1)
	go server.Start(mainServer, cfg, serverErr)

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
	if err := mainServer.Shutdown(shutdownCtx); err != nil {
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
	deps.ConnRegistry.CloseAll(wsShutdownCtx)

}

// setupRoutes registers all HTTP handlers
func setupRoutes(cfg *config.Config, deps *Dependencies) {
	// Serve static frontend files
	fileServer := middleware.WrapHandler(http.FileServer(http.Dir("./frontend")))
	http.Handle("/", fileServer)

	// Session establishment endpoint (called before WebSocket connection)
	sessionHandler := middleware.WrapHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check IP rate limit for session endpoint
		clientIP := transport.GetClientIP(r, cfg.BehindProxy)
		if !deps.IPRateLimiter.Allow(clientIP) {
			logger.Warn("Session endpoint rate limit exceeded").
				Str("ip", clientIP).
				Msg("")
			http.Error(w, "Too many requests. Please try again later.", http.StatusTooManyRequests)
			return
		}
		transport.HandleSession(w, r, deps.SessionMgr)
	})
	// Apply CORS to session endpoint for cross-origin requests
	sessionHandlerWithCORS := middleware.WrapWithCORS(sessionHandler, cfg.AllowedDomains)
	http.Handle("/api/session", sessionHandlerWithCORS)

	// WebSocket endpoint
	wsHandler := middleware.WrapHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		transport.HandleWebSocket(w, r, deps.WSConfig)
	})
	http.Handle("/ws", wsHandler)

	http.Handle("/health", handlers.HandleHealth())
}
