package websocket
 
import (
	"main/internal/handlers"
	"main/internal/logger"
	"main/internal/middleware"
	"main/internal/object"
	"main/internal/room"
	"main/internal/user"
	"net/http"
	"strings"
 
	"github.com/gorilla/websocket"
)
 
// WebSocketConfig holds configuration and dependencies for WebSocket handling
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
	BehindProxy   bool
}
 
// Required parameters: allowedDomains, sessionMgr, roomManager, msgRouter
// Optional parameters: use With* functions (e.g., WithIPRateLimiter, WithValidator)
func NewWebSocketConfig(
	allowedDomains []string,
	sessionMgr *user.SessionManager,
	roomManager *room.Manager,
	msgRouter *handlers.MessageRouter,
	opts ...WebSocketConfigOption,
) *WebSocketConfig {
	// upgrader w/ security settings
	upgrader := &websocket.Upgrader{
		ReadBufferSize:    4096,
		WriteBufferSize:   4096,
		EnableCompression: true,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("origin")
			for _, allowed := range allowedDomains {
				if origin == strings.TrimSpace(allowed) {
					return true
				}
			}
			logger.Warn("WebSocket connection rejected from unauthorized origin").
				Str("origin", origin).
				Strs("allowed_domains", allowedDomains).
				Msg("")
			return false
		},
	}
 
	cfg := &WebSocketConfig{
		Upgrader:    upgrader,
		SessionMgr:  sessionMgr,
		RoomManager: roomManager,
		MsgRouter:   msgRouter,
	}
 
	// Apply optional configurations
	for _, opt := range opts {
		opt(cfg)
	}
 
	return cfg
}
