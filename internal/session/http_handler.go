package session
 
import (
	"encoding/json"
	"main/internal/user"
	"net/http"
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
	var isNewUser bool
 
	if existingToken != "" {
		// Validate existing token
		validUserID, valid := sessionMgr.ValidateToken(existingToken)
		if valid {
			sessionToken = existingToken
			userID = validUserID
			isNewUser = false
		} else {
			sessionToken = user.GenerateSessionToken()
			userID = user.GenerateUUID()
			isNewUser = true
		}
	} else {
		// No existing token, create new one
		sessionToken = user.GenerateSessionToken()
		userID = user.GenerateUUID()
		isNewUser = true
	}
 
	// Create or update session
	if isNewUser {
		session := sessionMgr.GetOrCreate(userID)
		session.SessionToken = sessionToken
	}
 
	// Set cookie
	isSecure := isSecureConnection(r, behindProxy)
	setSessionCookie(w, sessionToken, isSecure)
 
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success": true,
		"userId":  userID,
	}
	json.NewEncoder(w).Encode(response)
}
