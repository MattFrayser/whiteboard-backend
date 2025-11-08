package transport

import (
	"encoding/json"
	"fmt"
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

// Authenticate: completes the authentication handshake
// Token has already been pre-validated from HTTP cookie in HandleWebSocket
// This function just reads the client's authenticate message and returns the pre-validated data
func (a *Authenticator) Authenticate(conn *websocket.Conn, token string, timeout time.Duration) (*AuthResult, error) {
	// Read authenticate message from client (protocol handshake)
	conn.SetReadDeadline(time.Now().Add(timeout))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to receive auth message: %w", err)
	}
	conn.SetReadDeadline(time.Time{}) // Clear timeout

	var authMsg struct {
		Type string `json:"type"`
	}

	if err := json.Unmarshal(msg, &authMsg); err != nil {
		return nil, fmt.Errorf("invalid auth message format: %w", err)
	}

	if authMsg.Type != "authenticate" {
		return nil, fmt.Errorf("expected authenticate message, got: %s", authMsg.Type)
	}

	// Token was already validated in HandleWebSocket, so we trust it here
	// Check if this is a returning user with existing session
	if token != "" {
		userID, valid := a.sessionMgr.ValidateToken(token)
		if valid {
			// Returning user with valid session
			return &AuthResult{
				UserID:       userID,
				SessionToken: token,
				IsNewUser:    false,
			}, nil
		}
	}

	// New user - token will be added to session manager after this returns
	userID := user.GenerateUUID()
	return &AuthResult{
		UserID:       userID,
		SessionToken: token, // Use the pre-generated token from HandleWebSocket
		IsNewUser:    true,
	}, nil
}
