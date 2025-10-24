package handlers 

import (
	"encoding/json"
	"fmt"
	"time"

	"main/internal/room"
	"main/internal/user"
)


// User session cursor functions
type SessionProvider interface {
	LastCursor(userID string) (time.Time, bool)
	UpdateLastCursor(userID string, t time.Time)
}


// CursorHandler handles cursor position update messages
type CursorHandler struct {
	sessionMgr  SessionProvider
	broadcaster *room.Broadcaster
}

// NewCursorHandler creates a new cursor handler with dependencies
func NewCursorHandler(sessionMgr SessionProvider, broadcaster *room.Broadcaster) *CursorHandler {
	return &CursorHandler{
		sessionMgr:  sessionMgr,
		broadcaster: broadcaster,
	}
}

// Handle processes cursor messages with server-side throttling
func (h *CursorHandler) Handle(rm *room.Room, u *user.User, data map[string]interface{}) error {
	now := time.Now()
	lastCursorTime, exists := h.sessionMgr.LastCursor(u.ID)
	if !exists {
		return fmt.Errorf("session not found")
	}

	// Throttle cursor updates (~30fps)
	if !lastCursorTime.IsZero() && now.Sub(lastCursorTime) < 33*time.Millisecond {
		return nil // Ignore to throttle
	}

	h.sessionMgr.UpdateLastCursor(u.ID, now)

	// Get user's color from the room (room-specific color)
	data["color"] = rm.GetUserColor(u.ID)
	data["userId"] = u.ID

	msg, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal cursor message: %w", err)
	}

	h.broadcaster.Broadcast(rm, msg, u.Connection)
	return nil
}
