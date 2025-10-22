package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	"main/internal/domain"
	"main/internal/middleware"

	"github.com/gorilla/websocket"
)

// HandleMessage routes messages to appropriate handlers based on message type
func HandleMessage(room *domain.Room, user *domain.User, msg []byte, config *middleware.RateLimit) error {
	var data map[string]interface{}
	if err := json.Unmarshal(msg, &data); err != nil {
		return fmt.Errorf("unmarshal base message: %w", err)
	}

	messageType, ok := data["type"].(string)
	if !ok {
		return fmt.Errorf("missing message type")
	}

	switch messageType {
	case "getUserId":
		return handleGetUserID(user)
	case "objectAdded":
		return handleObjectAdded(room, user, data, config)
	case "objectUpdated":
		return handleObjectUpdated(room, user, data)
	case "objectDeleted":
		return handleObjectDeleted(room, user, data)
	case "cursor":
		return handleCursor(room, user, data)
	default:
		return fmt.Errorf("unknown message type: %s", messageType)
	}
}

// handleGetUserID returns the user ID
func handleGetUserID(user *domain.User) error {
	response := map[string]interface{}{
		"type":   "userId",
		"userId": user.ID,
	}

	responseMsg, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal user ID response: %w", err)
	}

	return user.Connection.WriteMessage(websocket.TextMessage, responseMsg)
}

// handleObjectAdded adds an object to the room and broadcasts to other users
func handleObjectAdded(room *domain.Room, user *domain.User, data map[string]interface{}, config *middleware.RateLimit) error {
	// Check object limit before adding
	if !config.CanAddObject(room) {
		return fmt.Errorf("room at maximum object capacity")
	}

	object, ok := data["object"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing object data")
	}

	id, ok := object["id"].(string)
	if !ok {
		return fmt.Errorf("missing object id")
	}

	objType, ok := object["type"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid object type")
	}

	objData, ok := object["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing or invalid object data")
	}

	zIndexFloat, ok := object["zIndex"].(float64)
	if !ok {
		return fmt.Errorf("missing or invalid zIndex")
	}

	// Create object
	obj := &domain.DrawingObject{
		ID:     id,
		Type:   objType,
		Data:   objData,
		UserID: user.ID,
		ZIndex: int(zIndexFloat),
	}

	// Add to room
	room.AddObject(obj)

	// Broadcast
	data["userId"] = user.ID
	msg, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal broadcast message: %w", err)
	}
	room.Broadcast(msg, user.Connection)
	return nil
}

// handleObjectUpdated updates object data and broadcasts
func handleObjectUpdated(room *domain.Room, user *domain.User, data map[string]interface{}) error {
	object, ok := data["object"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing object data")
	}

	id, ok := object["id"].(string)
	if !ok {
		return fmt.Errorf("missing object id")
	}

	objData, ok := object["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing or invalid object data")
	}

	// Update object in room
	room.UpdateObject(id, objData)

	// Broadcast
	data["userId"] = user.ID
	msg, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal broadcast message: %w", err)
	}
	room.Broadcast(msg, user.Connection)
	return nil
}

// handleObjectDeleted removes an object from the room and broadcasts
func handleObjectDeleted(room *domain.Room, user *domain.User, data map[string]interface{}) error {
	objectID, ok := data["objectId"].(string)
	if !ok {
		return fmt.Errorf("missing objectId")
	}

	// Delete object from room
	room.DeleteObject(objectID)

	// Broadcast
	data["userId"] = user.ID
	msg, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal broadcast message: %w", err)
	}
	room.Broadcast(msg, user.Connection)
	return nil
}

// handleCursor updates cursor position and broadcasts
func handleCursor(room *domain.Room, user *domain.User, data map[string]interface{}) error {
	now := time.Now()
	lastTime, exists := domain.GetSessionLastSeen(user.ID)
	if !exists {
		return fmt.Errorf("session not found")
	}

	// Throttle cursor updates to ~30fps
	if now.Sub(lastTime) < 33*time.Millisecond {
		return nil // Ignore to throttle
	}

	domain.UpdateSessionLastSeen(user.ID, now)

	data["color"] = user.Session.Color
	data["userId"] = user.ID

	msg, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal cursor message: %w", err)
	}

	room.Broadcast(msg, user.Connection)
	return nil
}
