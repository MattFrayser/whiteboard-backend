package session
 
import (
	"net/http"
)
 
// setSessionCookie creates an HTTP-only, Secure session cookie
func setSessionCookie(w http.ResponseWriter, token string, secure bool) {
	sameSite := http.SameSiteLaxMode
	if secure {
		sameSite = http.SameSiteNoneMode
	}
 
	cookie := &http.Cookie{
		Name:     "whiteboard_session_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
		MaxAge:   3600, // 1 hour
	}
	http.SetCookie(w, cookie)
}
 
// isSecureConnection checks if the connection is over HTTPS
// Handles both direct TLS and proxy scenarios
func isSecureConnection(r *http.Request, behindProxy bool) bool {
	if r.TLS != nil {
		return true
	}
 
	if !behindProxy {
		return false
	}
 
	// Only use if behind trusted proxy
    	if r.Header.Get("X-Forwarded-Proto") == "https" {
        	return true
    	}
 
	return false
}
