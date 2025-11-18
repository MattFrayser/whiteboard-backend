package websocket
 
import (
	"context"
	"main/internal/auth"
	"main/internal/handlers"
	"main/internal/logger"
	"main/internal/session"
	"main/internal/room"
	"net/http"
 
)
 
// upgrade HTTP to WebSocket and join room
func HandleWebSocket(w http.ResponseWriter, r *http.Request, cfg *WebSocketConfig) {
	// Pre-connection checks: rate limiting and capacity
	if err := checkConnectionCapacity(r, cfg); err != nil {
		if err.Error() == "rate_limit_exceeded" {
			http.Error(w, "Too many connections", http.StatusTooManyRequests)
		} else {
			http.Error(w, "Server at capacity. Please try again later.", http.StatusServiceUnavailable)
		}
		return
	}
 
	// Establish or validate session, set cookie
	sessionToken, isNewUser, err := session.EstablishSession(r, w, cfg.SessionMgr, cfg.BehindProxy)
	if err != nil {
		logger.Error("Failed to establish session").Err(err).Msg("")
		http.Error(w, "Session error", http.StatusInternalServerError)
		return
	}
 
	// Upgrade connection with Set-Cookie header already sent
	conn, err := cfg.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error("Failed to upgrade connection").Err(err).Msg("")
		return
	}
 
	if err := acquireConnectionSlot(cfg); err != nil {
		conn.Close()
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
 
	roomCode := r.URL.Query().Get("room")
	if roomCode == "" {
		logger.Error("No room code provided").Msg("")
		return
	}
 
	u, userSession, err := authenticateConnection(conn, sessionToken, isNewUser, roomCode, cfg)
	if err != nil {
		logger.Error("Failed to authenticate connection").
			Err(err).
			Str("room_code", roomCode).
			Msg("")
		return
	}
 
	// Ensure cleanup on all exit paths (before room join)
	var rm *room.Room
	defer cleanup(rm, u, cfg.SessionMgr, cfg.ConnTracker)
 
	// (createRoom or joinRoom)
	intentType, nextMsg, err := handlers.HandleRoomIntent(conn, u, roomCode, cfg.RoomManager, cfg.RateLimit)
	if err != nil {
		logger.Error("Failed to handle room intent").
			Err(err).
			Str("user_id", u.ID).
			Str("room_code", roomCode).
			Msg("")
		return
	}
 
	existingRoom, roomExists := cfg.RoomManager.GetRoom(roomCode)
	if roomExists && existingRoom.HasPassword() {
		if err := auth.VerifyRoomPassword(conn, existingRoom, userSession, roomCode, u, nextMsg); err != nil {
			logger.Error("Failed to verify room password").
				Err(err).
				Str("user_id", u.ID).
				Str("room_code", roomCode).
				Msg("")
			return
		}
	}
 
	rm, err = handlers.HandleRoomJoin(u, userSession, roomCode, intentType, cfg.RoomManager, cfg.RateLimit)
	if err != nil {
		logger.Error("Failed to handle room join").
			Err(err).
			Str("user_id", u.ID).
			Str("room_code", roomCode).
			Str("intent", intentType).
			Msg("")
		return
	}
 
	if err := handlers.SyncNewUser(rm, u, roomCode, cfg.Synchronizer); err != nil {
		logger.Error("Failed to sync new user").
			Err(err).
			Str("user_id", u.ID).
			Str("room_code", roomCode).
			Msg("")
		return
	}
 
	// Start message processing loop
	run(connCtx, conn, rm, u, cfg.RateLimit, cfg.MsgRouter)
}
