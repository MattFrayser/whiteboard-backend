package handlers

import (
	"encoding/json"
	"fmt"
	"main/internal/logger"
	"main/internal/room"
	"main/internal/user"
 
	"github.com/gorilla/websocket"
)

// handles sync request messages from clients
type SyncHandler struct {
	synchronizer *room.Synchronizer
}

func NewSyncHandler(synchronizer *room.Synchronizer) *SyncHandler {
	return &SyncHandler{
		synchronizer: synchronizer,
	}
}

// Sends the current room state (all objects) to the requesting user
func (h *SyncHandler) Handle(rm *room.Room, u *user.User, data map[string]interface{}) error {

	if err := h.synchronizer.SyncNewUser(rm, u); err != nil {
		return fmt.Errorf("failed to sync room state: %w", err)
	}

	return nil
}

// sends the room_joined message with user color and syncs room state to the new user
// Errors on sync failures are non-fatal
func SyncNewUser(room *room.Room, user *user.User, roomCode string, synchronizer *room.Synchronizer) error {
	// Send room-specific color after joining
	userColor := room.GetUserColor(user.ID)
 
	colorResponse := map[string]interface{}{
		"type":  "room_joined",
		"color": userColor,
		"room":  roomCode,
	}
	colorMsg, err := json.Marshal(colorResponse)
	if err != nil {
		logger.Error("Failed to marshal room joined response").
			Err(err).
			Msg("")
		return fmt.Errorf("failed to marshal room joined response: %w", err)
	}
	if err := user.WriteMessage(websocket.TextMessage, colorMsg); err != nil {
		logger.Error("Failed to send room joined response").
			Err(err).
			Msg("")
		return fmt.Errorf("failed to send room joined response: %w", err)
	}
 
	// Sync room state to new user
	if err := synchronizer.SyncNewUser(room, user); err != nil {
		logger.Error("Failed to sync room state to user").
			Str("user_id", user.ID).
			Err(err).
			Msg("")
		// Just log the error, and allow
	}
 
	return nil
}
