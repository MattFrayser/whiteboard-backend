package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"main/internal/handlers"
	"main/internal/middleware"
	"main/internal/room"
	"main/internal/websocket"
	"main/internal/user"
	"main/internal/object"

	"github.com/joho/godotenv"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	godotenv.Load()

	// Initialize rate limiting configuration
	config := middleware.NewRateLimit(
		10,     // maxRoomSize
		1000,   // maxObjects (reduced from 3000)
		250000, // maxMessageSize (250KB, increased from 100KB)
		100,    // maxRooms (reduced from 1000)
		5,      // maxObjectDepth
		1000,   // maxObjectElements (unique keys)
		30,     // messagesPerSecond
		10,     // burstSize
	)

	// Initialize managers
	ipRateLimiter := middleware.NewIPRateLimit()
	sessionMgr := user.NewSessionManager()
	validator := object.NewValidator()
	roomMgr := room.NewManager()
	broadcaster := room.NewBroadcaster()
	synchronizer := room.NewSynchronizer()
	msgRouter := handlers.NewMessageRouter(validator, config, sessionMgr, broadcaster)
	authenticator := transport.NewAuthenticator()

	// Setup HTTP handlers
	http.Handle("/", http.FileServer(http.Dir("./frontend")))
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		transport.HandleWebSocket(w, r, ipRateLimiter, config, sessionMgr, validator, roomMgr, msgRouter, synchronizer, authenticator)
	})

	// Start periodic cleanups
	go cleanupRooms(ctx, roomMgr)
	go cleanupSessions(ctx, sessionMgr)
	go cleanupIPLimiters(ctx, ipRateLimiter)

	// Run server
	log.Println("Server Started on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
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
