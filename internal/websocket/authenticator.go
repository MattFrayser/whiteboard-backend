package transport

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"main/internal/user"

	"github.com/gorilla/websocket"
)

// Authenticator: handles WebSocket authentication
type Authenticator struct {
	sessionMgr *user.SessionManager
}

// NewAuthenticator: creates a new authenticator
func NewAuthenticator(sessionMgr *user.SessionManager) *Authenticator {
	return &Authenticator{
		sessionMgr: sessionMgr,
	}
}

// AuthResult contains the results of authentication
type AuthResult struct {
	UserID       string
	SessionToken string
	IsNewUser    bool
}

// Authenticate: reads and validates authentication message from new connection
// Returns userID and session token. For new users, generates both.
// For returning users, validates token and retrieves userID.
func (a *Authenticator) Authenticate(conn *websocket.Conn, timeout time.Duration) (*AuthResult, error) {
	// Read deadline
	conn.SetReadDeadline(time.Now().Add(timeout))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to receive auth message: %w", err)
	}
	conn.SetReadDeadline(time.Time{}) // Clear timeout

	var authMsg struct {
		Type  string `json:"type"`
		Token string `json:"token"` // Session token for returning users
	}

	if err := json.Unmarshal(msg, &authMsg); err != nil {
		return nil, fmt.Errorf("invalid auth message format: %w", err)
	}

	if authMsg.Type != "authenticate" {
		return nil, fmt.Errorf("expected authenticate message, got: %s", authMsg.Type)
	}

	// Case 1: Returning user with valid token
	if authMsg.Token != "" {
		userID, valid := a.sessionMgr.ValidateToken(authMsg.Token)
		if valid {
			log.Printf("Returning user authenticated: %s", userID)
			return &AuthResult{
				UserID:       userID,
				SessionToken: authMsg.Token,
				IsNewUser:    false,
			}, nil
		}
		log.Printf("Invalid or expired token provided, treating as new user")
	}

	// Case 2: New user (empty token or invalid token)
	userID := user.GenerateUUID()
	sessionToken := user.GenerateSessionToken()

	log.Printf("New user created: %s", userID)
	return &AuthResult{
		UserID:       userID,
		SessionToken: sessionToken,
		IsNewUser:    true,
	}, nil
}
