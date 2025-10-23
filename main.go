package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"main/internal/middleware"
	"main/internal/room"
	"main/internal/transport"
	"main/internal/user"

	"github.com/joho/godotenv"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	godotenv.Load()

	// Initialize rate limiting configuration
	config := middleware.NewRateLimit(
		10,     // maxRoomSize
		3000,   // maxObjects
		100000, // maxMessageSize (100KB)
		1000,   // maxRooms
		30,     // messagesPerSecond
		10,     // burstSize
	)

	ipRateLimiter := middleware.NewIPRateLimit()
	sessionMgr := user.NewSessionManager()

	// Setup HTTP handlers
	http.Handle("/", http.FileServer(http.Dir("./frontend")))
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		transport.HandleWebSocket(w, r, ipRateLimiter, config, sessionMgr)
	})

	// Start periodic cleanups
	go cleanupRooms(ctx)
	go cleanupSessions(ctx, sessionMgr)
	go cleanupIPLimiters(ctx, ipRateLimiter)

	// Run server
	log.Println("Server Started on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}

// cleanupRooms periodically removes expired rooms
func cleanupRooms(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			room.Cleanup()
			log.Println("Cleaned up expired rooms")
		}
	}
}

// cleanupSessions periodically removes expired user sessions
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

// cleanupIPLimiters periodically clears IP rate limiters
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
