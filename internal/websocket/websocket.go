package transport

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"main/internal/handlers"
	"main/internal/middleware"
	"main/internal/object"
	"main/internal/room"
	"main/internal/user"

	"github.com/gorilla/websocket"
)

// WebSocketConfig: holds configuration and dependencies for WebSocket handling
type WebSocketConfig struct {
	Upgrader      *websocket.Upgrader
	IPRateLimiter *middleware.IPRateLimit
	RateLimit     *middleware.RateLimit
	SessionMgr    *user.SessionManager
	Validator     *object.Validator
	RoomManager   *room.Manager
	MsgRouter     *handlers.MessageRouter
	Synchronizer  *room.Synchronizer
	Authenticator *Authenticator
}

// NewWebSocketConfig: creates a new WebSocketConfig with upgrader configured for allowed domains
func NewWebSocketConfig(
	allowedDomains []string,
	ipRateLimiter *middleware.IPRateLimit,
	rateLimit *middleware.RateLimit,
	sessionMgr *user.SessionManager,
	validator *object.Validator,
	roomManager *room.Manager,
	msgRouter *handlers.MessageRouter,
	synchronizer *room.Synchronizer,
	authenticator *Authenticator,
) *WebSocketConfig {
	upgrader := &websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("origin")
			for _, allowed := range allowedDomains {
				if origin == strings.TrimSpace(allowed) {
					return true
				}
			}
			return false
		},
	}

	return &WebSocketConfig{
		Upgrader:      upgrader,
		IPRateLimiter: ipRateLimiter,
		RateLimit:     rateLimit,
		SessionMgr:    sessionMgr,
		Validator:     validator,
		RoomManager:   roomManager,
		MsgRouter:     msgRouter,
		Synchronizer:  synchronizer,
		Authenticator: authenticator,
	}
}

// GetClientIP: extracts the real client IP from the request
func GetClientIP(r *http.Request) string {

	// basic, will need to change if behind proxy
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
func HandleWebSocket(w http.ResponseWriter, r *http.Request, cfg *WebSocketConfig) {
	// Check if rate limited
	clientIP := GetClientIP(r)
	if !cfg.IPRateLimiter.Allow(clientIP) {
		log.Printf("Rate limit exceeded for IP: %s", clientIP)
		http.Error(w, "Too many connections", http.StatusTooManyRequests)
		return
	}

	// Set security headers before upgrade
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// Upgrade connection
	conn, err := cfg.Upgrader.Upgrade(w, r, nil)
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
	authResult, err := cfg.Authenticator.Authenticate(conn, 5*time.Second)
	if err != nil {
		log.Printf("Error: Authentication failed - %v", err)
		return
	}

	// Get or create session
	var session *user.UserSession
	if authResult.IsNewUser {
		// Create new session with the generated token
		session = cfg.SessionMgr.GetOrCreate(authResult.UserID, "")
		// Override the token with the one we generated during auth
		// (GetOrCreate generates its own, but we want to use the auth one)
		session.SessionToken = authResult.SessionToken
		cfg.SessionMgr.UpdateTokenMapping(authResult.SessionToken, authResult.UserID)
	} else {
		// Get existing session for returning user
		session, _ = cfg.SessionMgr.GetSessionByToken(authResult.SessionToken)
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
	defer cleanup(rm, u, cfg.SessionMgr)

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
	rm, joinErr = cfg.RoomManager.JoinRoom(roomCode, session, u, cfg.RateLimit)
	if joinErr != nil {
		log.Printf("Error: Failed to join room (%s) - %v", roomCode, joinErr)
		return
	}

	// Send room-specific color after joining
	userColor := rm.GetUserColor(u.ID)

	colorResponse := map[string]interface{}{
		"type":  "room_joined",
		"color": userColor,
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

	// Sync room state to new user
	if err := cfg.Synchronizer.SyncNewUser(rm, u); err != nil {
		log.Printf("Error: Failed to sync room state to user %s - %v", u.ID, err)
		// Don't return - allow user to continue even if sync fails
	}

	// Start message processing loop
	run(conn, rm, u, cfg.RateLimit, cfg.MsgRouter)
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

		// Check rate limit based on message type
		var data map[string]interface{}
		if err := json.Unmarshal(msg, &data); err != nil {
			log.Printf("Failed to parse message for rate limiting: %v", err)
			continue
		}

		messageType, ok := data["type"].(string)
		if !ok {
			log.Printf("Message missing type field from user %s", u.ID)
			continue
		}

		// Route to appropriate rate limiter
		var rateLimitExceeded bool
		if messageType == "cursor" {
			rateLimitExceeded = !u.Session.CursorRateLimiter.Allow()
		} else {
			rateLimitExceeded = !u.Session.ObjectRateLimiter.Allow()
		}

		if rateLimitExceeded {
			log.Printf("Rate limit exceeded for user %s (type: %s)", u.ID, messageType)
			continue // Drop message
		}

		if err := msgRouter.Route(rm, u, msg); err != nil {
			log.Printf("Error handling message from user %s: %v", u.ID, err)
			continue // Skip message
		}
	}
}
