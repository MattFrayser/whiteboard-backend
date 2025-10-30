package handlers

import (
	"fmt"
	"log"

	"main/internal/room"
	"main/internal/user"
)

// SyncHandler handles sync request messages from clients
type SyncHandler struct {
	synchronizer *room.Synchronizer
}

// NewSyncHandler creates a new sync handler
func NewSyncHandler(synchronizer *room.Synchronizer) *SyncHandler {
	return &SyncHandler{
		synchronizer: synchronizer,
	}
}

// Handle processes sync request messages
// Sends the current room state (all objects) to the requesting user
func (h *SyncHandler) Handle(rm *room.Room, u *user.User, data map[string]interface{}) error {
	log.Printf("Sync requested by user %s in room", u.ID)

	if err := h.synchronizer.SyncNewUser(rm, u); err != nil {
		return fmt.Errorf("failed to sync room state: %w", err)
	}

	return nil
}
