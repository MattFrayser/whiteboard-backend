package session
 
import (
	"main/internal/user"
	"net/http"
)
 
// validate existing session or create new one
func EstablishSession(r *http.Request, w http.ResponseWriter, sessionMgr *user.SessionManager, behindProxy bool) (string, bool, error) {
	// Extract existing token from cookie
	var existingToken string
	cookie, err := r.Cookie("whiteboard_session_token")
	if err == nil {
		existingToken = cookie.Value
	}
 
	// Pre-validate token or prepare new one
	var sessionToken string
	var isNewUser bool
 
	if existingToken != "" {
		if sessionMgr.ValidateToken(existingToken) {
			sessionToken = existingToken
			isNewUser = false
		} else {
			sessionToken = user.GenerateSessionToken()
			isNewUser = true
		}
	} else {
		sessionToken = user.GenerateSessionToken()
		isNewUser = true
	}
 
	// Set cookie before returning
	isSecure := isSecureConnection(r, behindProxy)
	setSessionCookie(w, sessionToken, isSecure)
 
	return sessionToken, isNewUser, nil
}
