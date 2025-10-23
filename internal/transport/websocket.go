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

// GetClientIP extracts the real client IP from the request
func GetClientIP(r *http.Request) string {
	// X-Forwarded-For
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx] // Remove port
	}
	return ip
}

// HandleWebSocket upgrades HTTP to WebSocket and joins the room
func HandleWebSocket(w http.ResponseWriter, r *http.Request, ipRateLimiter *middleware.IPRateLimit, config *middleware.RateLimit, sessionMgr *user.SessionManager) {
	// Check if rate limited
	clientIP := GetClientIP(r)
	if !ipRateLimiter.Allow(clientIP) {
		log.Printf("Rate limit exceeded for IP: %s", clientIP)
		http.Error(w, "Too many connections", http.StatusTooManyRequests)
		return
	}

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

	// Wait for authentication message with timeout
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Println("Error: Failed to receive auth message:", err)
		return
	}
	conn.SetReadDeadline(time.Time{}) // Clear timeout

	// Parse authentication message
	var authMsg struct {
		Type   string `json:"type"`
		UserID string `json:"userId"`
	}

	if err := json.Unmarshal(msg, &authMsg); err != nil {
		log.Println("Error: Invalid auth message format:", err)
		return
	}

	if authMsg.Type != "authenticate" {
		log.Println("Error: Expected authenticate message, got:", authMsg.Type)
		return
	}

	// Get or generate userID
	userID := authMsg.UserID
	if userID == "" {
		userID = user.GenerateUUID()
	}

	// Get session or create a new one
	session := sessionMgr.GetOrCreate(userID)
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
		"color":  session.Color,
	}
	responseMsg, _ := json.Marshal(response)
	conn.WriteMessage(websocket.TextMessage, responseMsg)

	// Join room
	var rm *room.Room

	// Check if user is rejoining their last room and it still exists
	if session.LastRoom == roomCode {
		if existingRoom, active := room.GetRoomIfActive(roomCode); active {
			rm = existingRoom
			rm.Join(u, config.MaxRoomSize)
			log.Printf("User %s rejoined room %s", userID, roomCode)
		} else {
			// Room expired, make new
			rm, err = room.JoinRoom(roomCode, u, config.MaxRooms, config.MaxRoomSize)
			if err != nil {
				log.Printf("Error: Connection to room (%s) - %v", roomCode, err)
				return
			}
		}
	} else {
		// Joining a different room or first time
		rm, err = room.JoinRoom(roomCode, u, config.MaxRooms, config.MaxRoomSize)
		if err != nil {
			log.Printf("Error: Connection to room (%s) - %v", roomCode, err)
			return
		}
	}

	run(conn, rm, u, config, sessionMgr)
}

// run handles the message loop for WebSocket connections
func run(conn *websocket.Conn, rm *room.Room, u *user.User, config *middleware.RateLimit, sessionMgr *user.SessionManager) {
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

		if err := handlers.HandleMessage(rm, u, msg, config, sessionMgr); err != nil {
			log.Printf("Error handling message from user %s: %v", u.ID, err)
			continue // Skip message
		}
	}
}
