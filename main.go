package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"main/internal/handlers"
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

	// Parse allowed domains from environment
	allowedDomains := strings.Split(os.Getenv("DOMAINS"), ",")

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
			log.Printf("Session endpoint rate limit exceeded for IP: %s", clientIP)
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

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		if useTLS {
			log.Println("Server started on :8443 (HTTPS/WSS)")
			log.Println("Press Ctrl+C to shutdown gracefully")
			serverErr <- server.ListenAndServeTLS(certFile, keyFile)
		} else {
			log.Println("Server started on :8080 (HTTP/WS)")
			log.Println("WARNING: TLS not enabled. Set TLS_CERT_FILE and TLS_KEY_FILE environment variables for production.")
			log.Println("Press Ctrl+C to shutdown gracefully")
			serverErr <- server.ListenAndServe()
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case <-sigChan:
		log.Println("\nShutdown signal received, initiating graceful shutdown...")
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
		return
	}

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Shutdown HTTP server (stops accepting new connections)
	log.Println("Stopping HTTP server...")
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	// Close all WebSocket connections gracefully
	log.Println("Closing all WebSocket connections...")
	wsShutdownCtx, wsShutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer wsShutdownCancel()
	connRegistry.CloseAll(wsShutdownCtx)

	// Cancel context to stop cleanup goroutines
	log.Println("Stopping background cleanup routines...")
	cancel()

	// Wait a moment for cleanup goroutines to exit gracefully
	time.Sleep(100 * time.Millisecond)

	log.Println("Shutdown complete")
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
			log.Println("Cleaned up expired rooms")
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
			log.Println("Cleaned up expired sessions")
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
			log.Println("IP rate limiters cleared")
		}
	}
}
