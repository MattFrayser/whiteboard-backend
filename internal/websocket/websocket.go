package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"main/internal/auth"
	"main/internal/handlers"
	"main/internal/middleware"
	"main/internal/object"
	"main/internal/room"
	"main/internal/user"

	"github.com/gorilla/websocket"
)

// setSessionCookie creates an HTTP-only, Secure session cookie
func setSessionCookie(w http.ResponseWriter, token string, secure bool) {
	// For localhost development (different ports), SameSite=Lax works for same-site requests
	// For production with HTTPS, use SameSite=None with Secure=true for cross-origin
	sameSite := http.SameSiteLaxMode
	if secure {
		// Production: HTTPS enabled, use None for cross-origin support
		sameSite = http.SameSiteNoneMode
	}

	cookie := &http.Cookie{
		Name:     "whiteboard_session_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,          // Prevent JavaScript access (XSS protection)
		Secure:   secure,        // Only send over HTTPS in production
		SameSite: sameSite,      // Lax for dev (localhost), None for prod (cross-origin)
		MaxAge:   3600,          // 1 hour (matches session cleanup interval)
	}
	http.SetCookie(w, cookie)
}

// HandleSession establishes a session and sets the session cookie
// This should be called by the frontend BEFORE opening a WebSocket connection
// This ensures cookies are reliably stored via regular HTTP response
func HandleSession(w http.ResponseWriter, r *http.Request, sessionMgr *user.SessionManager) {
	// Only allow GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract session token from cookie (for returning users)
	var existingToken string
	cookie, err := r.Cookie("whiteboard_session_token")
	if err == nil {
		existingToken = cookie.Value
	}

	// Generate fingerprint from User-Agent for validation
	userAgent := r.Header.Get("User-Agent")
	fingerprint := user.GenerateFingerprint(userAgent)

	var sessionToken string
	var userID string
	var isNewUser bool

	if existingToken != "" {
		// Validate existing token
		validUserID, valid := sessionMgr.ValidateToken(existingToken)
		if valid {
			// Validate fingerprint to prevent token theft
			session, sessionExists := sessionMgr.GetSessionByToken(existingToken)
			if sessionExists && session.Fingerprint != "" && session.Fingerprint != fingerprint {
				// Fingerprint mismatch - possible token theft
				log.Printf("Fingerprint mismatch for user %s - generating new session", validUserID)
				sessionToken = user.GenerateSessionToken()
				userID = user.GenerateUUID()
				isNewUser = true
			} else {
				// Valid returning user
				sessionToken = existingToken
				userID = validUserID
				isNewUser = false
				log.Printf("Returning user session validated: %s", userID)
			}
		} else {
			// Invalid token, create new one
			sessionToken = user.GenerateSessionToken()
			userID = user.GenerateUUID()
			isNewUser = true
			log.Printf("Invalid session token, generating new session")
		}
	} else {
		// No existing token, create new one
		sessionToken = user.GenerateSessionToken()
		userID = user.GenerateUUID()
		isNewUser = true
		log.Printf("No session cookie found, generating new session")
	}

	// Create or update session
	if isNewUser {
		session := sessionMgr.GetOrCreate(userID, "", fingerprint)
		session.SessionToken = sessionToken
		session.TokenCreatedAt = time.Now()
		sessionMgr.UpdateTokenMapping(sessionToken, userID)
	}

	// Set cookie
	isSecure := r.TLS != nil
	setSessionCookie(w, sessionToken, isSecure)

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success": true,
		"userId":  userID,
	}
	json.NewEncoder(w).Encode(response)
}

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
	ConnTracker   *middleware.ConnectionTracker
	ConnRegistry  *ConnectionRegistry
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
	connTracker *middleware.ConnectionTracker,
	connRegistry *ConnectionRegistry,
) *WebSocketConfig {
	upgrader := &websocket.Upgrader{
		ReadBufferSize:    4096, // 4KB read buffer
		WriteBufferSize:   4096, // 4KB write buffer
		EnableCompression: true, // Enable permessage-deflate compression
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("origin")
			for _, allowed := range allowedDomains {
				if origin == strings.TrimSpace(allowed) {
					log.Printf("WebSocket connection accepted from origin: %s", origin)
					return true
				}
			}
			// Log rejected origins to help detect misconfigurations or attacks
			log.Printf("WebSocket connection rejected from unauthorized origin: %s (allowed: %v)", origin, allowedDomains)
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
		ConnTracker:   connTracker,
		ConnRegistry:  connRegistry,
	}
}

// GetClientIP: extracts the real client IP from the request
// Handles proxy environments by checking X-Forwarded-For and X-Real-IP headers
func GetClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (set by proxies like nginx, Cloudflare)
	// Format: X-Forwarded-For: client, proxy1, proxy2
	// We want the first IP (the original client)
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// Take the first IP in the chain (client's real IP)
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header (used by some proxies)
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fallback to RemoteAddr (direct connection, no proxy)
	// RemoteAddr format: "ip:port" or "[ipv6]:port"
	ip := r.RemoteAddr

	// Handle IPv6 with brackets: [2001:db8::1]:port
	if strings.HasPrefix(ip, "[") {
		if idx := strings.LastIndex(ip, "]"); idx != -1 {
			return ip[:idx+1] // Keep the brackets
		}
	}

	// Handle IPv4: 192.168.1.1:port
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		// Check if this looks like IPv6 (multiple colons) vs IPv4:port (single colon)
		if strings.Count(ip[:idx], ":") == 0 {
			return ip[:idx] // IPv4, remove port
		}
	}

	// Return as-is (IPv6 without port, or malformed)
	return ip
}

// cleanup ensures all resources are properly released
// Sessions persist after disconnect to allow reconnection until cookie expires
func cleanup(rm *room.Room, u *user.User, sessionMgr *user.SessionManager, connTracker *middleware.ConnectionTracker) {
	if rm != nil {
		rm.Leave(u)
	}
	// Release global connection slot
	if connTracker != nil {
		connTracker.Disconnect()
		log.Printf("Global connection released. Active connections: %d/%d", connTracker.Count(), connTracker.GetMaxConnections())
	}
	// Note: We don't remove the session here - it persists until cookie expires (2 hours)
	// This allows seamless reconnection without creating new user IDs
}

// sendError sends an error message to the client and logs it
func sendError(u *user.User, code string, message string) {
	errorResponse := map[string]interface{}{
		"type":    "error",
		"code":    code,
		"message": message,
	}
	errorMsg, err := json.Marshal(errorResponse)
	if err != nil {
		log.Printf("Error: Failed to marshal error response - %v", err)
		return
	}
	if err := u.WriteMessage(websocket.TextMessage, errorMsg); err != nil {
		log.Printf("Error: Failed to send error response - %v", err)
	}
}

// handleCreateRoom processes room creation with settings (password, permissions)
// Called when client sends createRoom message after authentication
func handleCreateRoom(roomCode string, userID string, msgData map[string]interface{}, cfg *WebSocketConfig) error {
	log.Printf("Creating room %s with settings for user %s", roomCode, userID)

	// Extract settings from message
	settings := &room.RoomSettings{
		CreatedBy: userID,
	}

	// Extract password if provided
	if password, ok := msgData["password"].(string); ok && password != "" {
		// Hash the password
		hashedPassword, err := auth.HashPassword(password)
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}
		settings.Password = hashedPassword
		log.Printf("Room %s: Password protection enabled", roomCode)
	}

	// Extract permissions if provided
	if perms, ok := msgData["permissions"].(map[string]interface{}); ok {
		if viewOnly, ok := perms["viewOnly"].(bool); ok {
			settings.Permissions.ViewOnly = viewOnly
			log.Printf("Room %s: ViewOnly=%v", roomCode, viewOnly)
		}
		if onlyEditOwn, ok := perms["onlyEditOwn"].(bool); ok {
			settings.Permissions.OnlyEditOwn = onlyEditOwn
			log.Printf("Room %s: OnlyEditOwn=%v", roomCode, onlyEditOwn)
		}
	}

	// Create room with settings
	_, err := cfg.RoomManager.CreateRoomWithSettingsPublic(roomCode, settings, cfg.RateLimit)
	if err != nil {
		return fmt.Errorf("failed to create room: %w", err)
	}

	log.Printf("Room %s created successfully with settings", roomCode)
	return nil
}

// checkConnectionLimits verifies IP rate limits and global connection capacity
// Returns error if limits are exceeded
func checkConnectionLimits(r *http.Request, cfg *WebSocketConfig) error {
	// Check if rate limited
	clientIP := GetClientIP(r)
	if !cfg.IPRateLimiter.Allow(clientIP) {
		log.Printf("Rate limit exceeded for IP: %s", clientIP)
		return fmt.Errorf("rate_limit_exceeded")
	}

	// Check global connection limit
	if cfg.ConnTracker != nil && !cfg.ConnTracker.CanConnect() {
		log.Printf("Global connection limit reached. Current: %d/%d", cfg.ConnTracker.Count(), cfg.ConnTracker.GetMaxConnections())
		return fmt.Errorf("connection_limit_reached")
	}

	// Acquire global connection slot
	if cfg.ConnTracker != nil && !cfg.ConnTracker.Connect() {
		log.Printf("Failed to acquire connection slot")
		return fmt.Errorf("connection_slot_failed")
	}

	return nil
}

// establishSession validates existing session or creates new one, rotates tokens if needed, and sets cookies
// Returns sessionToken, isNewUser flag, and error
func establishSession(r *http.Request, w http.ResponseWriter, cfg *WebSocketConfig) (string, bool, error) {
	// Extract session token from cookie (for returning users)
	var existingToken string
	cookie, err := r.Cookie("whiteboard_session_token")
	if err == nil {
		existingToken = cookie.Value
	}

	// Generate fingerprint from User-Agent for validation
	userAgent := r.Header.Get("User-Agent")
	fingerprint := user.GenerateFingerprint(userAgent)

	// Pre-validate token or prepare new one BEFORE upgrade
	var sessionToken string
	var isNewUser bool
	var shouldRotate bool

	if existingToken != "" {
		// Validate existing token
		userID, valid := cfg.SessionMgr.ValidateToken(existingToken)
		if valid {
			// Validate fingerprint to prevent token theft
			session, sessionExists := cfg.SessionMgr.GetSessionByToken(existingToken)
			if sessionExists && session.Fingerprint != "" && session.Fingerprint != fingerprint {
				// Fingerprint mismatch - possible token theft
				log.Printf("Fingerprint mismatch for user %s - rejecting token", userID)
				sessionToken = user.GenerateSessionToken()
				isNewUser = true
			} else {
				// Fingerprint matches or not set (backward compatibility)
				// Check if token needs rotation (>1 hour old)
				shouldRotate = cfg.SessionMgr.ShouldRotateToken(userID)
				if shouldRotate {
					// Rotate token
					newToken, rotated := cfg.SessionMgr.RotateToken(userID)
					if rotated {
						sessionToken = newToken
						log.Printf("Token rotated for user: %s", userID)
					} else {
						sessionToken = existingToken
						log.Printf("Token rotation failed for user: %s, using existing token", userID)
					}
				} else {
					sessionToken = existingToken
				}
				isNewUser = false
				log.Printf("Returning user with valid cookie: %s", userID)
			}
		} else {
			// Invalid token, create new one
			sessionToken = user.GenerateSessionToken()
			isNewUser = true
			log.Printf("Invalid cookie token, generating new session")
		}
	} else {
		// No existing token, create new one
		sessionToken = user.GenerateSessionToken()
		isNewUser = true
		log.Printf("No cookie found, generating new session")
	}

	// Set cookie before upgrade (using response headers)
	// Secure flag should be true in production with TLS
	isSecure := r.TLS != nil
	setSessionCookie(w, sessionToken, isSecure)

	return sessionToken, isNewUser, nil
}

// authenticateConnection completes the authentication handshake and creates/retrieves user session
// Returns User object, UserSession, and error
func authenticateConnection(conn *websocket.Conn, sessionToken string, isNewUser bool, roomCode string, cfg *WebSocketConfig) (*user.User, *user.UserSession, error) {
	// Generate fingerprint from the connection's User-Agent
	// Note: We already generated it in establishSession, but we need it again here for session creation
	// In a future refactor, we could pass this through to avoid regeneration
	userAgent := "" // We don't have direct access to r here, fingerprint was already validated in establishSession
	fingerprint := user.GenerateFingerprint(userAgent)

	// Complete authentication handshake (read authenticate message)
	authResult, err := cfg.Authenticator.Authenticate(conn, sessionToken, 5*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("authentication failed: %w", err)
	}

	// Override isNewUser flag from our pre-validation
	authResult.IsNewUser = isNewUser

	// Get or create session (fingerprint already generated above)
	var session *user.UserSession
	if authResult.IsNewUser {
		// Create new session with the generated token
		session = cfg.SessionMgr.GetOrCreate(authResult.UserID, "", fingerprint)
		// Override the token with the one we generated during auth
		// (GetOrCreate generates its own, but we want to use the auth one)
		session.SessionToken = authResult.SessionToken
		session.TokenCreatedAt = time.Now()
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

	// Send authentication response (token is in HTTP-only cookie, not in message)
	response := map[string]interface{}{
		"type":   "authenticated",
		"userId": authResult.UserID,
		// Token is now in HTTP-only cookie, not sent in message for security
	}
	responseMsg, err := json.Marshal(response)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal auth response: %w", err)
	}
	if err := u.WriteMessage(websocket.TextMessage, responseMsg); err != nil {
		return nil, nil, fmt.Errorf("failed to send auth response: %w", err)
	}

	return u, session, nil
}

// handleRoomIntent reads the client's intent (createRoom or joinRoom) and handles room creation if needed
// Returns intent type, the message data for password verification, and error
func handleRoomIntent(conn *websocket.Conn, user *user.User, roomCode string, cfg *WebSocketConfig) (string, []byte, error) {
	// Wait for client to declare intent: createRoom or joinRoom
	// Client has 3 seconds to send intent message
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, nextMsg, err := conn.ReadMessage()
	conn.SetReadDeadline(time.Time{}) // Reset deadline immediately

	var intentType string = "joinRoom" // Default to join if no message or timeout

	if err == nil {
		// Got a message - parse intent
		var msgData map[string]interface{}
		if json.Unmarshal(nextMsg, &msgData) == nil {
			if msgType, ok := msgData["type"].(string); ok {
				intentType = msgType

				// Handle createRoom message
				if msgType == "createRoom" {
					log.Printf("Received createRoom message from user %s", user.ID)
					if err := handleCreateRoom(roomCode, user.ID, msgData, cfg); err != nil {
						log.Printf("Error creating room with settings: %v", err)
						sendError(user, "ROOM_CREATE_FAILED", "Failed to create room: "+err.Error())
						return "", nil, fmt.Errorf("room creation failed: %w", err)
					}
				}
			}
		}
	} else {
		// Timeout or read error - default to joinRoom behavior
		log.Printf("No intent message received from user %s (timeout), defaulting to joinRoom", user.ID)
	}

	return intentType, nextMsg, nil
}

// verifyRoomPassword handles password verification for password-protected rooms with retry logic
// Takes the initial message from handleRoomIntent that might contain the password
// Returns error if verification fails or max attempts exceeded
func verifyRoomPassword(conn *websocket.Conn, room *room.Room, session *user.UserSession, roomCode string, user *user.User, nextMsg []byte) error {
	log.Printf("Room %s is password-protected, checking credentials", roomCode)

	// Check if user has already verified this room in their session
	passwordVerified := false
	if verifiedTime, exists := session.VerifiedRooms[roomCode]; exists {
		// Check if verification is still valid (within session lifetime)
		if time.Since(verifiedTime) < 1*time.Hour {
			passwordVerified = true
			log.Printf("User %s: Already verified for room %s (verified at %v)", user.ID, roomCode, verifiedTime)
		} else {
			// Verification expired, remove it
			delete(session.VerifiedRooms, roomCode)
			log.Printf("User %s: Verification expired for room %s", user.ID, roomCode)
		}
	}

	const maxPasswordAttempts = 3
	passwordAttempts := 0

	// Password verification loop - allow multiple attempts
	for !passwordVerified && passwordAttempts < maxPasswordAttempts {
		// Extract password from message
		var password string
		var msgData map[string]interface{}
		if nextMsg != nil && json.Unmarshal(nextMsg, &msgData) == nil {
			if pwd, ok := msgData["password"].(string); ok {
				password = pwd
			}
		}

		// Check if password was provided
		if password == "" {
			log.Printf("User %s: No password provided for protected room %s (attempt %d)", user.ID, roomCode, passwordAttempts+1)
			sendError(user, "PASSWORD_REQUIRED", "This room requires a password")

			// Wait for client to send another joinRoom message with password
			conn.SetReadDeadline(time.Now().Add(30 * time.Second)) // 30 second timeout for password entry
			var err error
			_, nextMsg, err = conn.ReadMessage()
			conn.SetReadDeadline(time.Time{}) // Reset deadline

			if err != nil {
				log.Printf("User %s: Timeout waiting for password or connection error: %v", user.ID, err)
				sendError(user, "AUTH_TIMEOUT", "Password entry timeout")
				return fmt.Errorf("password entry timeout")
			}

			passwordAttempts++
			continue
		}

		// Verify password
		if !auth.VerifyPassword(room.Password, password) {
			passwordAttempts++
			log.Printf("User %s: Invalid password for room %s (attempt %d/%d)", user.ID, roomCode, passwordAttempts, maxPasswordAttempts)

			if passwordAttempts >= maxPasswordAttempts {
				log.Printf("User %s: Maximum password attempts exceeded for room %s", user.ID, roomCode)
				sendError(user, "MAX_ATTEMPTS_EXCEEDED", "Maximum password attempts exceeded")
				return fmt.Errorf("max password attempts exceeded")
			}

			sendError(user, "INVALID_PASSWORD", "Incorrect password")

			// Wait for client to send another joinRoom message with password
			conn.SetReadDeadline(time.Now().Add(30 * time.Second)) // 30 second timeout for password retry
			var err error
			_, nextMsg, err = conn.ReadMessage()
			conn.SetReadDeadline(time.Time{}) // Reset deadline

			if err != nil {
				log.Printf("User %s: Timeout waiting for password retry or connection error: %v", user.ID, err)
				sendError(user, "AUTH_TIMEOUT", "Password entry timeout")
				return fmt.Errorf("password retry timeout")
			}

			continue
		}

		// Password verified successfully
		passwordVerified = true
		// Store verification in session for seamless reconnection
		session.VerifiedRooms[roomCode] = time.Now()
		log.Printf("User %s: Password verified for room %s (stored in session)", user.ID, roomCode)
	}

	if !passwordVerified {
		// Should never reach here due to loop condition, but added for safety
		log.Printf("User %s: Password verification failed for room %s", user.ID, roomCode)
		sendError(user, "AUTH_FAILED", "Authentication failed")
		return fmt.Errorf("authentication failed")
	}

	return nil
}

// handleRoomJoin handles the logic of joining or creating a room
// Returns the room object and error
func handleRoomJoin(user *user.User, session *user.UserSession, roomCode string, intentType string, cfg *WebSocketConfig) (*room.Room, error) {
	// Handle join logic
	if intentType == "joinRoom" || intentType == "createRoom" {
		// Check if room exists
		existingRoom, roomExists := cfg.RoomManager.GetRoom(roomCode)

		// If intent is joinRoom and room doesn't exist, return error
		if intentType == "joinRoom" && !roomExists {
			log.Printf("User %s tried to join non-existent room: %s", user.ID, roomCode)
			sendError(user, "ROOM_NOT_FOUND", fmt.Sprintf("Room %s not found. Please check the room code.", roomCode))
			return nil, fmt.Errorf("room not found")
		}

		// Join room (will create if doesn't exist and was createRoom intent)
		var rm *room.Room
		var joinErr error
		if roomExists {
			rm = existingRoom
			joinErr = rm.Join(user, cfg.RateLimit.MaxRoomSize)
		} else {
			// Room doesn't exist, use JoinRoom which will create it
			rm, joinErr = cfg.RoomManager.JoinRoom(roomCode, session, user, cfg.RateLimit)
		}

		if joinErr != nil {
			log.Printf("Error: Failed to join room (%s) - %v", roomCode, joinErr)
			sendError(user, "ROOM_JOIN_FAILED", "Failed to join room: "+joinErr.Error())
			return nil, fmt.Errorf("failed to join room: %w", joinErr)
		}

		return rm, nil
	}

	// Unknown intent type
	log.Printf("Unknown intent type from user %s: %s", user.ID, intentType)
	sendError(user, "INVALID_INTENT", "Invalid intent type")
	return nil, fmt.Errorf("invalid intent type")
}

// syncNewUser sends the room_joined message with user color and syncs room state to the new user
// Returns error if sync fails (non-fatal - user can continue)
func syncNewUser(room *room.Room, user *user.User, roomCode string, cfg *WebSocketConfig) error {
	// Send room-specific color after joining
	userColor := room.GetUserColor(user.ID)

	colorResponse := map[string]interface{}{
		"type":  "room_joined",
		"color": userColor,
		"room":  roomCode,
	}
	colorMsg, err := json.Marshal(colorResponse)
	if err != nil {
		log.Printf("Error: Failed to marshal room joined response - %v", err)
		return fmt.Errorf("failed to marshal room joined response: %w", err)
	}
	if err := user.WriteMessage(websocket.TextMessage, colorMsg); err != nil {
		log.Printf("Error: Failed to send room joined response - %v", err)
		return fmt.Errorf("failed to send room joined response: %w", err)
	}

	// Sync room state to new user
	if err := cfg.Synchronizer.SyncNewUser(room, user); err != nil {
		log.Printf("Error: Failed to sync room state to user %s - %v", user.ID, err)
		// Don't return error - allow user to continue even if sync fails
		// Just log the error
	}

	return nil
}

// HandleWebSocket: upgrades HTTP to WebSocket and joins the room
func HandleWebSocket(w http.ResponseWriter, r *http.Request, cfg *WebSocketConfig) {
	// Pre-connection checks: rate limiting and capacity
	if err := checkConnectionLimits(r, cfg); err != nil {
		if err.Error() == "rate_limit_exceeded" {
			http.Error(w, "Too many connections", http.StatusTooManyRequests)
		} else {
			http.Error(w, "Server at capacity. Please try again later.", http.StatusServiceUnavailable)
		}
		return
	}

	// Establish or validate session, set cookie
	sessionToken, isNewUser, err := establishSession(r, w, cfg)
	if err != nil {
		log.Printf("Error: Failed to establish session - %v", err)
		http.Error(w, "Session error", http.StatusInternalServerError)
		return
	}

	// Upgrade connection with Set-Cookie header already sent
	conn, err := cfg.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error: Failed to upgrade connection - %v", err)
		return
	}

	// Create cancellable context for this connection (for graceful shutdown)
	connCtx, connCancel := context.WithCancel(context.Background())
	defer connCancel()

	// Register connection for graceful shutdown tracking
	if cfg.ConnRegistry != nil {
		cfg.ConnRegistry.Register(conn, connCancel)
		defer cfg.ConnRegistry.Unregister(conn)
	}

	defer conn.Close()

	// Retrieve roomCode from URL
	roomCode := r.URL.Query().Get("room")
	if roomCode == "" {
		log.Println("Error: No room code provided")
		return
	}

	// Authenticate connection and create user
	u, session, err := authenticateConnection(conn, sessionToken, isNewUser, roomCode, cfg)
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	// Ensure cleanup on all exit paths (before room join)
	var rm *room.Room
	defer cleanup(rm, u, cfg.SessionMgr, cfg.ConnTracker)

	// Handle client intent (createRoom or joinRoom)
	intentType, nextMsg, err := handleRoomIntent(conn, u, roomCode, cfg)
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	// Check if room exists and is password-protected
	existingRoom, roomExists := cfg.RoomManager.GetRoom(roomCode)
	if roomExists && existingRoom.HasPassword() {
		if err := verifyRoomPassword(conn, existingRoom, session, roomCode, u, nextMsg); err != nil {
			log.Printf("Error: %v", err)
			return
		}
	}

	// Join or create room
	rm, err = handleRoomJoin(u, session, roomCode, intentType, cfg)
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	// Send room joined message and sync state to new user
	if err := syncNewUser(rm, u, roomCode, cfg); err != nil {
		log.Printf("Error: %v", err)
		return
	}

	// Start message processing loop
	run(connCtx, conn, rm, u, cfg.RateLimit, cfg.MsgRouter)
}

// run: message loop for WebSocket connections with context for graceful shutdown
func run(ctx context.Context, conn *websocket.Conn, rm *room.Room, u *user.User, config *middleware.RateLimit, msgRouter *handlers.MessageRouter) {
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
		// Check if context was cancelled (shutdown signal received)
		select {
		case <-ctx.Done():
			log.Printf("Connection context cancelled for user %s, closing gracefully", u.ID)
			return
		default:
			// Continue with normal message reading
		}

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
