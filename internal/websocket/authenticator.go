package websocket

import (
	"encoding/json"
	"fmt"
	"time"

	"main/internal/user"
	"github.com/gorilla/websocket"
)

type Authenticator struct {
	sessionMgr *user.SessionManager
}

func NewAuthenticator(sessionMgr *user.SessionManager) *Authenticator {
	return &Authenticator{
		sessionMgr: sessionMgr,
	}
}

type AuthResult struct {
	UserID       string
	SessionToken string
	IsNewUser    bool
}

// completes the authentication handshake
// Token has already been pre-validated from HTTP cookie in HandleWebSocket
// reads the client's authenticate message and returns the pre-validated data
func (a *Authenticator) Authenticate(conn *websocket.Conn, token string, timeout time.Duration) (*AuthResult, error) {
	// Read authenticate message from client (protocol handshake)
	conn.SetReadDeadline(time.Now().Add(timeout))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to receive auth message: %w", err)
	}
	conn.SetReadDeadline(time.Time{}) // Clear timeout

	var authMsg struct {
		Type 	  string `json:"type"`
		CSRFToken string `json:"csrfToken"`
		
	}

	if err := json.Unmarshal(msg, &authMsg); err != nil {
		return nil, fmt.Errorf("invalid auth message format: %w", err)
	}

	if authMsg.Type != "authenticate" {
		return nil, fmt.Errorf("expected authenticate message, got: %s", authMsg.Type)
	}

	// Token already validated in HandleWebSocket, so we trust it
	// Check if this is a returning user with existing session
	if token != "" {
		session, exists := a.sessionMgr.GetSessionByToken(token)  // ← Single lookup
		if exists {
			// Validate CSRF token
			if authMsg.CSRFToken == "" || authMsg.CSRFToken != session.CSRFToken {
				return nil, fmt.Errorf("invalid or missing CSRF token")
			}

			a.sessionMgr.UpdateLastSeen(session.UserID, time.Now())

			// Returning user with valid session
			return &AuthResult{
				UserID:       session.UserID,  
				SessionToken: token,
				IsNewUser:    false,
			}, nil
		}
	}

	// New user 
	userID := user.GenerateUUID()
	return &AuthResult{
		UserID:       userID,
		SessionToken: token, // pre-generated token from HandleWebSocket
		IsNewUser:    true,
	}, nil
}

// completes the auth handshake and creates/retrieves user session
func authenticateConnection(
	conn *websocket.Conn,
	sessionToken string,
	isNewUser bool,
	roomCode string,
	cfg *WebSocketConfig,
) (*user.User, *user.UserSession, error) {
	authResult, err := cfg.Authenticator.Authenticate(conn, sessionToken, 5*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("authentication failed: %w", err)
	}

	// Override pre-validation
	authResult.IsNewUser = isNewUser

	var session *user.UserSession
	if authResult.IsNewUser {
		session = cfg.SessionMgr.GetOrCreate(authResult.UserID)
	} else {
		session, _ = cfg.SessionMgr.GetSessionByToken(authResult.SessionToken)
	}

	cfg.SessionMgr.UpdateLastRoom(authResult.UserID, roomCode)

	// Create user with session
	u := &user.User{
		ID:         authResult.UserID,
		Session:    session,
		Connection: conn,
	}

	response := map[string]interface{}{
		"type":   "authenticated",
		"userId": authResult.UserID,
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
