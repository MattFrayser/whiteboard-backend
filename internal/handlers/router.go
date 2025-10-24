package handlers

import (
	"encoding/json"
	"fmt"

	"main/internal/middleware"
	internalObject "main/internal/object"
	internalUser "main/internal/user"
	"main/internal/room"
)

// MessageRouter routes incoming messages to appropriate handlers
type MessageRouter struct {
	objectHandler *ObjectHandler
	cursorHandler *CursorHandler
	userHandler   *UserHandler
}

func NewMessageRouter(
	validator *internalObject.Validator,
	config *middleware.RateLimit,
	sessionMgr SessionProvider,
	broadcaster *room.Broadcaster,
) *MessageRouter {
	return &MessageRouter{
		objectHandler: NewObjectHandler(validator, config, broadcaster),
		cursorHandler: NewCursorHandler(sessionMgr, broadcaster),
		userHandler:   NewUserHandler(),
	}
}

// Route: process a message via appropriate handler
func (mr *MessageRouter) Route(rm *room.Room, u *internalUser.User, msg []byte) error {
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
		return mr.userHandler.HandleGetUserID(u)
	case "objectAdded":
		return mr.objectHandler.HandleAdded(rm, u, data)
	case "objectUpdated":
		return mr.objectHandler.HandleUpdated(rm, u, data)
	case "objectDeleted":
		return mr.objectHandler.HandleDeleted(rm, u, data)
	case "cursor":
		return mr.cursorHandler.Handle(rm, u, data)
	default:
		return fmt.Errorf("unknown message type: %s", messageType)
	}
}
