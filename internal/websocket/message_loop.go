package websocket
 
import (
	"context"
	"encoding/json"
	"main/internal/handlers"
	"main/internal/logger"
	"main/internal/middleware"
	"main/internal/room"
	"main/internal/user"
	"time"
 
	"github.com/gorilla/websocket"
)
 
// message loop for WebSocket connections ( context for graceful shutdown)
func run(ctx context.Context, conn *websocket.Conn, rm *room.Room, u *user.User, config *middleware.RateLimit, msgRouter *handlers.MessageRouter) {
	const (
		pongWait   = 60 * time.Second
		pingPeriod = (pongWait * 9) / 10 //  90% of pong deadline
		readWait   = 60 * time.Second
	)
 
	// Set up pong handler to extend deadline when pong received
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
 
	// Start ping ticker in background
	pingTicker := time.NewTicker(pingPeriod)
	defer pingTicker.Stop()
 
	// Channel to signal when read loop exits
	done := make(chan struct{})
	defer close(done)
 
	// Ping goroutine
	go func() {
		for {
			select {
			case <-pingTicker.C:
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return // Connection dead, ping goroutine exits
				}
			case <-done:
				return // Main loop exited, stop pinging
			}
		}
	}()
 
	// Main read loop
	for {
		// Check if context was cancelled (shutdown signal received)
		select {
		case <-ctx.Done():
			logger.Debug("Connection context cancelled, closing gracefully").
				Str("user_id", u.ID).
				Msg("")
			return
		default:
			// Continue with normal message reading
		}
 
		_, msg, err := conn.ReadMessage()
		if err != nil {
			logger.Error("Error reading message").
				Err(err).
				Str("user_id", u.ID).
				Msg("")
			break // Connection dead
		}
 
		// Validate message size
		if !config.ValidateMessageSize(len(msg)) {
			logger.Warn("Message too large from user").
				Str("user_id", u.ID).
				Int("size_bytes", len(msg)).
				Msg("")
			continue // Drop oversized message
		}
 
		// Check rate limit based on message type
		var data map[string]interface{}
		if err := json.Unmarshal(msg, &data); err != nil {
			logger.Warn("Failed to parse message for rate limiting").
				Err(err).
				Str("user_id", u.ID).
				Msg("")
			continue
		}
 
		messageType, ok := data["type"].(string)
		if !ok {
			logger.Warn("Message missing type field").
				Str("user_id", u.ID).
				Msg("")
			continue
		}
 
		// Route to appropriate rate limiter
		var rateLimitExceeded bool
		if messageType == "cursor" {
			rateLimitExceeded = !u.Session.CursorRateLimiter.Allow()
		} else {
			rateLimitExceeded = !u.Session.ObjectRateLimiter.Allow()
		}
 
		if rateLimitExceeded {
			logger.Warn("Rate limit exceeded for user").
				Str("user_id", u.ID).
				Str("message_type", messageType).
				Msg("")
			continue // Drop message
		}
 
		if err := msgRouter.Route(rm, u, msg); err != nil {
			logger.Error("Error handling message from user").
				Str("user_id", u.ID).
				Err(err).
				Msg("")
			continue // Skip message
		}
	}
}
