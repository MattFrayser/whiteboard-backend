package websocket
 
import (
	"fmt"
	"main/internal/middleware"
	"main/internal/room"
	"main/internal/user"
	"main/internal/util"
	"net/http"
)
 
// ensures all resources are properly released
func cleanup(rm *room.Room, u *user.User, sessionMgr *user.SessionManager, connTracker *middleware.ConnectionTracker) {
	if rm != nil {
		rm.Leave(u)
	}
	// Release global connection slot
	if connTracker != nil {
		connTracker.Disconnect()
	}
 
	// Note: Sessions are not removed here, they persist until cookie expires
	// Allows for reconnection w/o creating new user IDs
}
 
func checkConnectionCapacity(r *http.Request, cfg *WebSocketConfig) error {
	clientIP := util.GetClientIP(r, cfg.BehindProxy)
	if !cfg.IPRateLimiter.Allow(clientIP) {
		return fmt.Errorf("rate_limit_exceeded")
	}
 
	if cfg.ConnTracker != nil && !cfg.ConnTracker.CanConnect() {
		return fmt.Errorf("connection_limit_reached")
	}
 
	return nil
}
 
func acquireConnectionSlot(cfg *WebSocketConfig) error {
	if cfg.ConnTracker != nil && !cfg.ConnTracker.Connect() {
		return fmt.Errorf("connection_slot_failed")
	}
	return nil
}
