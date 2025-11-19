package handlers 

import (
	"encoding/json"
	"fmt"

	"main/internal/logger"
	"main/internal/middleware"
	"main/internal/object"
	"main/internal/room"
	"main/internal/user"
)

// ObjectHandler: handles object-related messages (add, update, delete)
type ObjectHandler struct {
	validator   *object.Validator
	config      *middleware.RateLimit
	broadcaster *room.Broadcaster
}

func NewObjectHandler(validator *object.Validator, config *middleware.RateLimit, broadcaster *room.Broadcaster) *ObjectHandler {
	return &ObjectHandler{
		validator:   validator,
		config:      config,
		broadcaster: broadcaster,
	}
}

// sendErrorAck: helper function to send error acknowledgment to user
func (h *ObjectHandler) sendErrorAck(u *user.User, objectID string, errorMsg string) {
	ackMsg := map[string]interface{}{
		"type":     "objectAdded_error",
		"objectId": objectID,
		"success":  false,
		"error":    errorMsg,
	}
	ackData, err := json.Marshal(ackMsg)
	if err != nil {
		logger.Error("Failed to marshal error ACK message").
		Err(err).
		Msg("")
		return
	}
	if err := u.WriteMessage(1, ackData); err != nil {
		logger.Error("Failed to send error ACK to user").
		Str("user_id", u.ID).
		Err(err).
		Msg("")
	}
}

// HandleAdded: objectAdded messages
func (h *ObjectHandler) HandleAdded(rm *room.Room, u *user.User, data map[string]interface{}) error {
	// Extract object ID early so we can use it in error ACKs
	var objectID string
	if objectMsg, ok := data["object"].(map[string]interface{}); ok {
		if id, ok := objectMsg["id"].(string); ok {
			objectID = id
		}
	}

	// Check permissions: Can this user add objects?
	if !rm.CanEdit(u.ID) {
		h.sendErrorAck(u, objectID, "permission denied: room is view-only")
		return fmt.Errorf("permission denied: room is view-only")
	}

	// Check object limit before adding
	if !h.config.CanAddObject(rm) {
		h.sendErrorAck(u, objectID, "room at maximum object capacity")
		return fmt.Errorf("room at maximum object capacity")
	}

	objectMsg, ok := data["object"].(map[string]interface{})
	if !ok {
		h.sendErrorAck(u, objectID, "missing object data")
		return fmt.Errorf("missing object data")
	}

	id, ok := objectMsg["id"].(string)
	if !ok {
		h.sendErrorAck(u, objectID, "missing object id")
		return fmt.Errorf("missing object id")
	}

	objType, ok := objectMsg["type"].(string)
	if !ok {
		h.sendErrorAck(u, id, "missing or invalid object type")
		return fmt.Errorf("missing or invalid object type")
	}

	objData, ok := objectMsg["data"].(map[string]interface{})
	if !ok {
		h.sendErrorAck(u, id, "missing or invalid object data")
		return fmt.Errorf("missing or invalid object data", objectMsg)
	}

	if err := h.config.ValidateObjectComplexity(objData); err != nil {
		h.sendErrorAck(u, id, "object too complex")
		logger.Warn("Object complexity validation failed").
			Str("user_id", u.ID).
			Str("object", id).
			Err(err).
			Msg("")
		return fmt.Errorf("object complexity validation failed: %w", err)
	}

	// Validate and sanitize object data using schema validation
	sanitizedData, err := h.validator.ValidateAndSanitize(objType, objData)
	if err != nil {
		h.sendErrorAck(u, id, fmt.Sprintf("object validation failed: %v", err))
		return fmt.Errorf("object validation failed: %w", err)
	}

	zIndexFloat, ok := objectMsg["zIndex"].(float64)
	if !ok {
		h.sendErrorAck(u, id, "missing or invalid zIndex")
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
	dataWithMeta := make(map[string]interface{})
	for k, v := range sanitizedData {
		dataWithMeta[k] = v
	}
	dataWithMeta["id"] = id
	dataWithMeta["type"] = objType

	// Update the data object with sanitized data for broadcast
	objectMsg["data"] = dataWithMeta
	objectMsg["id"] = id
	objectMsg["type"] = objType
	objectMsg["zIndex"] = int(zIndexFloat)
	data["object"] = objectMsg
	data["userId"] = u.ID

	logger.Debug("Broadcasting objectAdded").
		Str("objectId", id).
		Str("objectType", objType).
		Msg("")

	// Broadcast to other users
	msg, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal broadcast message: %w", err)
	}
	h.broadcaster.Broadcast(rm, msg, u.Connection)

	// Send acknowledgment back to sender
	ackMsg := map[string]interface{}{
		"type":     "objectAdded_ack",
		"objectId": id,
		"success":  true,
	}
	ackData, err := json.Marshal(ackMsg)
	if err != nil {
		// Log error but don't fail the operation since object was added successfully
		logger.Error("Failed to marshal ACK message").
		Err(err).
		Msg("")
		return nil
	}

	if err := u.WriteMessage(1, ackData); err != nil {
		// Log error but don't fail since object was added and broadcast successfully
		logger.Error("Failed to send ACK to user").
		Str("user_id", u.ID).
		Err(err).
		Msg("")
	}

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

	// Check permissions: Can this user edit this specific object?
	if !rm.CanEditObject(u.ID, id) {
		return fmt.Errorf("permission denied: cannot edit this object")
	}

	objData, ok := objectMsg["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing or invalid object data")
	}

	if err := h.config.ValidateObjectComplexity(objData); err != nil {
		h.sendErrorAck(u, id, "object too complex")
		logger.Warn("Object complexity validation failed").
			Str("user_id", u.ID).
			Str("object", id).
			Err(err).
			Msg("")
		return fmt.Errorf("object complexity validation failed: %w", err)
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
	dataWithMeta := make(map[string]interface{})
	for k, v := range sanitizedData {
		dataWithMeta[k] = v
	}
	dataWithMeta["id"] = id
	dataWithMeta["type"] = existingObj.Type
	objectMsg["data"] = dataWithMeta
	objectMsg["id"] = id
	objectMsg["type"] = existingObj.Type
	data["object"] = objectMsg
	data["userId"] = u.ID

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

	// Check permissions: Can this user delete this specific object?
	if !rm.CanEditObject(u.ID, objectID) {
		return fmt.Errorf("permission denied: cannot delete this object")
	}

	// Delete object from room
	rm.DeleteObject(objectID)

	// Broadcast IDs
	data["objectId"] = objectID
	data["userId"] = u.ID
	msg, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal broadcast message: %w", err)
	}
	h.broadcaster.Broadcast(rm, msg, u.Connection)
	return nil
}
