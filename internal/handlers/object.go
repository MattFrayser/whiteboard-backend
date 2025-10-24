package handlers 

import (
	"encoding/json"
	"fmt"

	"main/internal/middleware"
	"main/internal/object"
	"main/internal/room"
	"main/internal/user"
)

// ObjectHandler: handles object-related messages (add, update, delete)
type ObjectHandler struct {
	validator   *object.Validator
	config      *middleware.RateLimit
	broadcaster Broadcaster
}

func NewObjectHandler(validator *object.Validator, config *middleware.RateLimit, broadcaster Broadcaster) *ObjectHandler {
	return &ObjectHandler{
		validator:   validator,
		config:      config,
		broadcaster: broadcaster,
	}
}

// HandleAdded: objectAdded messages
func (h *ObjectHandler) HandleAdded(rm *room.Room, u *user.User, data map[string]interface{}) error {
	// Check object limit before adding
	if !h.config.CanAddObject(rm) {
		return fmt.Errorf("room at maximum object capacity")
	}

	objectMsg, ok := data["object"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing object data")
	}

	id, ok := objectMsg["id"].(string)
	if !ok {
		return fmt.Errorf("missing object id")
	}

	objType, ok := objectMsg["type"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid object type")
	}

	objData, ok := objectMsg["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing or invalid object data")
	}

	// Validate and sanitize object data using schema validation
	sanitizedData, err := h.validator.ValidateAndSanitize(objType, objData)
	if err != nil {
		return fmt.Errorf("object validation failed: %w", err)
	}

	zIndexFloat, ok := objectMsg["zIndex"].(float64)
	if !ok {
		return fmt.Errorf("missing or invalid zIndex")
	}

	// Create object with sanitized data
	obj := &object.Drawing{
		ID:     id,
		Type:   objType,
		Data:   sanitizedData,
		UserID: u.ID,
		ZIndex: int(zIndexFloat),
	}

	// Add to room
	rm.AddObject(obj)

	// Update the data object with sanitized data for broadcast
	objectMsg["data"] = sanitizedData
	objectMsg["id"] = object.SanitizeString(id)
	data["object"] = objectMsg
	data["userId"] = object.SanitizeString(u.ID)

	// Broadcast
	msg, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal broadcast message: %w", err)
	}
	h.broadcaster.Broadcast(rm, msg, u.Connection)
	return nil
}

// HandleUpdated: objectUpdated messages
func (h *ObjectHandler) HandleUpdated(rm *room.Room, u *user.User, data map[string]interface{}) error {
	objectMsg, ok := data["object"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing object data")
	}

	id, ok := objectMsg["id"].(string)
	if !ok {
		return fmt.Errorf("missing object id")
	}

	objData, ok := objectMsg["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing or invalid object data")
	}

	// Get the existing object to determine its type
	existingObj := rm.GetObject(id)
	if existingObj == nil {
		return fmt.Errorf("object not found: %s", id)
	}

	// Validate and sanitize object data using schema validation
	sanitizedData, err := h.validator.ValidateAndSanitize(existingObj.Type, objData)
	if err != nil {
		return fmt.Errorf("object validation failed: %w", err)
	}

	// Update object in room with sanitized data
	rm.UpdateObject(id, sanitizedData)

	// Update the data object with sanitized data for broadcast
	objectMsg["data"] = sanitizedData
	objectMsg["id"] = object.SanitizeString(id)
	data["object"] = objectMsg
	data["userId"] = object.SanitizeString(u.ID)

	// Broadcast
	msg, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal broadcast message: %w", err)
	}
	h.broadcaster.Broadcast(rm, msg, u.Connection)
	return nil
}

// HandleDeleted: objectDeleted messages
func (h *ObjectHandler) HandleDeleted(rm *room.Room, u *user.User, data map[string]interface{}) error {
	objectID, ok := data["objectId"].(string)
	if !ok {
		return fmt.Errorf("missing objectId")
	}

	// Delete object from room
	rm.DeleteObject(objectID)

	// Broadcast with sanitized IDs
	data["objectId"] = object.SanitizeString(objectID)
	data["userId"] = object.SanitizeString(u.ID)
	msg, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal broadcast message: %w", err)
	}
	h.broadcaster.Broadcast(rm, msg, u.Connection)
	return nil
}
