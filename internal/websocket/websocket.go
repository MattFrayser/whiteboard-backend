package transport

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"main/internal/handlers"
	"main/internal/middleware"
	"main/internal/room"
	"main/internal/user"
	"main/internal/object"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	// CORS
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("origin")

		allowedDomains := strings.Split(os.Getenv("DOMAINS"), ",")

		for _, allowed := range allowedDomains {
			if origin == strings.TrimSpace(allowed) {
				return true
			}
		}

		return false
	},
}

// GetClientIP: extracts the real client IP from the request
func GetClientIP(r *http.Request) string {
	// Use RemoteAddr only - cannot be spoofed by client
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx] // Remove port
	}
	return ip
}

// HandleWebSocket: upgrades HTTP to WebSocket and joins the room
func HandleWebSocket(
	w http.ResponseWriter,
	r *http.Request,
	ipRateLimiter *middleware.IPRateLimit,
	config *middleware.RateLimit,
	sessionMgr *user.SessionManager,
	validator *object.Validator,
	roomManager *room.Manager,
	msgRouter *handlers.MessageRouter,
	synchronizer *room.Synchronizer,
	authenticator *Authenticator,
) {
	// Check if rate limited
	clientIP := GetClientIP(r)
	if !ipRateLimiter.Allow(clientIP) {
		log.Printf("Rate limit exceeded for IP: %s", clientIP)
		http.Error(w, "Too many connections", http.StatusTooManyRequests)
		return
	}

	// Set security headers before upgrade
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// Upgrade connection
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error upgrading connection - ", err)
		return
	}
	defer conn.Close()

	// Retrieve roomCode from URL
	roomCode := r.URL.Query().Get("room")
	if roomCode == "" {
		log.Println("Error: No room code provided")
		return
	}

	// Authenticate user
	userID, err := authenticator.Authenticate(conn, 5*time.Second)
	if err != nil {
		log.Printf("Error: Authentication failed - %v", err)
		return
	}

	// Get or generate userID
	if userID == "" {
		userID = user.GenerateUUID()
	}

	// Get session or create a new one (no color needed - color is per-room)
	session := sessionMgr.GetOrCreate(userID, "")
	session.LastRoom = roomCode // Track last room for resumption

	// Create user with session
	u := &user.User{
		ID:         userID,
		Session:    session,
		Connection: conn,
	}

	// Send userID back to client (for new users or confirmation)
	response := map[string]interface{}{
		"type":   "authenticated",
		"userId": userID,
	}
	responseMsg, _ := json.Marshal(response)
	u.WriteMessage(websocket.TextMessage, responseMsg)

	// Join room using room joiner
	rm, err := roomManager.JoinRoom(roomCode, session, u, config)
	if err != nil {
		log.Printf("Error: Failed to join room (%s) - %v", roomCode, err)
		return
	}

	// Send room-specific color after joining
	colorResponse := map[string]interface{}{
		"type":  "room_joined",
		"color": rm.GetUserColor(userID),
		"room":  roomCode,
	}
	colorMsg, _ := json.Marshal(colorResponse)
	u.WriteMessage(websocket.TextMessage, colorMsg)

	// Start message processing loop
	run(conn, rm, u, config, msgRouter)
}

// run: message loop for WebSocket connections
func run(conn *websocket.Conn, rm *room.Room, u *user.User, config *middleware.RateLimit, msgRouter *handlers.MessageRouter) {
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("Error: Reading message", err)
			rm.Leave(u)
			break // Connection dead
		}

		// Validate message size
		if !config.ValidateMessageSize(len(msg)) {
			log.Printf("Message too large from user %s: %d bytes", u.ID, len(msg))
			continue // Drop oversized message
		}

		// Check rate limit from session
		if !u.Session.RateLimiter.Allow() {
			log.Printf("Rate limit exceeded for user: %s", u.ID)
			continue // Drop message
		}

		if err := msgRouter.Route(rm, u, msg); err != nil {
			log.Printf("Error handling message from user %s: %v", u.ID, err)
			continue // Skip message
		}
	}
}
