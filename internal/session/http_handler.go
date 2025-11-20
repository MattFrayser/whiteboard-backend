package session

import (
	"encoding/json"
	"main/internal/user"
	"net/http"
	"time"
)

// Establish a session and set session cookie
// should be called by the frontend BEFORE opening a WebSocket connection
func HandleSession(w http.ResponseWriter, r *http.Request, sessionMgr *user.SessionManager, behindProxy bool) {

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
 
	var sessionToken string
	var userID string
	var session *user.UserSession
 
	if existingToken != "" {
		var exists bool
		session, exists = sessionMgr.GetSessionByToken(existingToken)
		if exists {
			sessionToken = existingToken
			userID = session.UserID  
			sessionMgr.UpdateLastSeen(session.UserID, time.Now())
		} else {
			// Invalid token -> new session
			userID = user.GenerateUUID()
			session = sessionMgr.GetOrCreate(userID)
			session.SessionToken = sessionToken
		}
	} else {
		// No token -> create new one
		userID = user.GenerateUUID()
		session = sessionMgr.GetOrCreate(userID)
		session.SessionToken = sessionToken
	}
 
 
	// Set cookie
	isSecure := isSecureConnection(r, behindProxy)
	setSessionCookie(w, sessionToken, isSecure)
 
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success": true,
		"userId":  userID,
		"csrfToken": session.CSRFToken,
	}
	json.NewEncoder(w).Encode(response)
}
