package handlers

import (
	"encoding/json"
	"fmt"
	"time"

	"main/internal/middleware"
	"main/internal/room"
	"main/internal/user"

	"github.com/gorilla/websocket"
)

// HandleMessage routes messages to appropriate handlers based on message type
func HandleMessage(rm *room.Room, u *user.User, msg []byte, config *middleware.RateLimit, sessionMgr *user.SessionManager) error {
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
		return handleGetUserID(u)
	case "objectAdded":
		return handleObjectAdded(rm, u, data, config)
	case "objectUpdated":
		return handleObjectUpdated(rm, u, data)
	case "objectDeleted":
		return handleObjectDeleted(rm, u, data)
	case "cursor":
		return handleCursor(rm, u, data, sessionMgr)
	default:
		return fmt.Errorf("unknown message type: %s", messageType)
	}
}

// handleGetUserID returns the user ID
func handleGetUserID(u *user.User) error {
	response := map[string]interface{}{
		"type":   "userId",
		"userId": u.ID,
	}

	responseMsg, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal user ID response: %w", err)
	}

	return u.Connection.WriteMessage(websocket.TextMessage, responseMsg)
}

// handleObjectAdded adds an object to the room and broadcasts to other users
func handleObjectAdded(rm *room.Room, u *user.User, data map[string]interface{}, config *middleware.RateLimit) error {
	// Check object limit before adding
	if !config.CanAddObject(rm) {
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
	obj := &room.DrawingObject{
		ID:     id,
		Type:   objType,
		Data:   objData,
		UserID: u.ID,
		ZIndex: int(zIndexFloat),
	}

	// Add to room
	rm.AddObject(obj)

	// Broadcast
	data["userId"] = u.ID
	msg, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal broadcast message: %w", err)
	}
	rm.Broadcast(msg, u.Connection)
	return nil
}

// handleObjectUpdated updates object data and broadcasts
func handleObjectUpdated(rm *room.Room, u *user.User, data map[string]interface{}) error {
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
	rm.UpdateObject(id, objData)

	// Broadcast
	data["userId"] = u.ID
	msg, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal broadcast message: %w", err)
	}
	rm.Broadcast(msg, u.Connection)
	return nil
}

// handleObjectDeleted removes an object from the room and broadcasts
func handleObjectDeleted(rm *room.Room, u *user.User, data map[string]interface{}) error {
	objectID, ok := data["objectId"].(string)
	if !ok {
		return fmt.Errorf("missing objectId")
	}

	// Delete object from room
	rm.DeleteObject(objectID)

	// Broadcast
	data["userId"] = u.ID
	msg, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal broadcast message: %w", err)
	}
	rm.Broadcast(msg, u.Connection)
	return nil
}

// handleCursor updates cursor position and broadcasts
func handleCursor(rm *room.Room, u *user.User, data map[string]interface{}, sessionMgr *user.SessionManager) error {
	now := time.Now()
	lastTime, exists := sessionMgr.GetLastSeen(u.ID)
	if !exists {
		return fmt.Errorf("session not found")
	}

	// Throttle cursor updates to ~30fps
	if now.Sub(lastTime) < 33*time.Millisecond {
		return nil // Ignore to throttle
	}

	sessionMgr.UpdateLastSeen(u.ID, now)

	data["color"] = u.Session.Color
	data["userId"] = u.ID

	msg, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal cursor message: %w", err)
	}

	rm.Broadcast(msg, u.Connection)
	return nil
}
