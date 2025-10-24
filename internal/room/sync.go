package room

import (
	"encoding/json"
	"fmt"

	"main/internal/user"

	"github.com/gorilla/websocket"
)

// Synchronizer: handles synchronizing room state to new users
type Synchronizer struct{}

// NewSynchronizer: creates new synchronizer
func NewSynchronizer() *Synchronizer {
	return &Synchronizer{}
}

// SyncNewUser sends the current room state (all objects) to a newly joined user
func (s *Synchronizer) SyncNewUser(rm *Room, u *user.User) error {
	rm.mu.RLock()
	// Build list of objects to sync
	objects := make([]map[string]interface{}, 0, len(rm.Objects))
	for _, obj := range rm.Objects {
		objects = append(objects, map[string]interface{}{
			"id":     obj.ID,
			"type":   obj.Type,
			"data":   obj.Data,
			"userId": obj.UserID,
			"zIndex": obj.ZIndex,
		})
	}
	rm.mu.RUnlock()

	syncMsg := map[string]interface{}{
		"type":    "sync",
		"objects": objects,
	}

	msgBytes, err := json.Marshal(syncMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal sync message: %w", err)
	}

	if err := u.WriteMessage(websocket.TextMessage, msgBytes); err != nil {
		return fmt.Errorf("failed to send sync message: %w", err)
	}

	return nil
}
