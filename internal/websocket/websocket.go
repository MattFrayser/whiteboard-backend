package transport

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"main/internal/handlers"
	"main/internal/middleware"
	"main/internal/object"
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

// GetClientIP: extracts the real client IP from the request
func GetClientIP(r *http.Request) string {
	// Use RemoteAddr only - cannot be spoofed by client
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx] // Remove port
	}
	return ip
}

// cleanup ensures all resources are properly released
func cleanup(rm *room.Room, u *user.User, sessionMgr *user.SessionManager) {
	if rm != nil {
		rm.Leave(u)
	}
	if sessionMgr != nil && u != nil {
		sessionMgr.Remove(u.ID)
	}
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
		log.Printf("Error: Failed to upgrade connection - %v", err)
		return
	}
	defer conn.Close()

	// Retrieve roomCode from URL
	roomCode := r.URL.Query().Get("room")
	if roomCode == "" {
		log.Println("Error: No room code provided")
		return
	}

	// Authenticate user (validates token or creates new user)
	authResult, err := authenticator.Authenticate(conn, 5*time.Second)
	if err != nil {
		log.Printf("Error: Authentication failed - %v", err)
		return
	}

	// Get or create session
	var session *user.UserSession
	if authResult.IsNewUser {
		// Create new session with the generated token
		session = sessionMgr.GetOrCreate(authResult.UserID, "")
		// Override the token with the one we generated during auth
		// (GetOrCreate generates its own, but we want to use the auth one)
		session.SessionToken = authResult.SessionToken
		sessionMgr.UpdateTokenMapping(authResult.SessionToken, authResult.UserID)
	} else {
		// Get existing session for returning user
		session, _ = sessionMgr.GetSessionByToken(authResult.SessionToken)
	}

	session.LastRoom = roomCode // Track last room for resumption

	// Create user with session
	u := &user.User{
		ID:         authResult.UserID,
		Session:    session,
		Connection: conn,
	}
	// Ensure cleanup on all exit paths (before room join)
	var rm *room.Room
	defer cleanup(rm, u, sessionMgr)

	// Send authentication response with token to client
	response := map[string]interface{}{
		"type":   "authenticated",
		"userId": authResult.UserID,
		"token":  authResult.SessionToken, // Client must store this token
	}
	responseMsg, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error: Failed to marshal auth response - %v", err)
		return
	}
	if err := u.WriteMessage(websocket.TextMessage, responseMsg); err != nil {
		log.Printf("Error: Failed to send auth response - %v", err)
		return
	}

	// Join room using room joiner
	var joinErr error
	rm, joinErr = roomManager.JoinRoom(roomCode, session, u, config)
	if joinErr != nil {
		log.Printf("Error: Failed to join room (%s) - %v", roomCode, joinErr)
		return
	}

	// Send room-specific color after joining
	colorResponse := map[string]interface{}{
		"type":  "room_joined",
		"color": rm.GetUserColor(u.ID),
		"room":  roomCode,
	}
	colorMsg, err := json.Marshal(colorResponse)
	if err != nil {
		log.Printf("Error: Failed to marshal room joined response - %v", err)
		return
	}
	if err := u.WriteMessage(websocket.TextMessage, colorMsg); err != nil {
		log.Printf("Error: Failed to send room joined response - %v", err)
		return
	}

	// Start message processing loop
	run(conn, rm, u, config, msgRouter)
}

// run: message loop for WebSocket connections
func run(conn *websocket.Conn, rm *room.Room, u *user.User, config *middleware.RateLimit, msgRouter *handlers.MessageRouter) {
	const (
		pongWait   = 60 * time.Second
		pingPeriod = (pongWait * 9) / 10 // Send pings at 90% of pong deadline
		readWait   = 60 * time.Second
	)

	// Set up pong handler to extend deadline when pong received
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Start ping ticker in background
	pingTicker := time.NewTicker(pingPeriod)
	defer pingTicker.Stop()

	// Channel to signal when read loop exits
	done := make(chan struct{})
	defer close(done)

	// Ping goroutine
	go func() {
		for {
			select {
			case <-pingTicker.C:
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return // Connection dead, ping goroutine exits
				}
			case <-done:
				return // Main loop exited, stop pinging
			}
		}
	}()

	// Main read loop
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("Error: Reading message", err)
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
