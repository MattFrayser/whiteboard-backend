package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lucasb-eyer/go-colorful"
)


var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool{ return true },
}

var (
	rooms = make(map[string]*Room)
	roomsMutex sync.RWMutex
	cursorColorCounter int
	cursorColorMutex sync.Mutex
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	http.Handle("/", http.FileServer(http.Dir("./frontend")))
	http.HandleFunc("/ws", handleWebSocket)

	go cleanupRooms(ctx)

	log.Println("WebSocket server started on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatalf("Error starting server: ", err)
	}
}

// handleWebSocket: Upgrades http to websocket then joins room
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error upgrading connection - ", err)
		return
	}
	defer conn.Close()

	u := &User{
		id:         fmt.Sprintf("%d", time.Now().UnixNano()),
		connection: conn,
		color:      getRandomHex(),
	}

	roomCode := r.URL.Query().Get("room")
	room, err := joinRoom(roomCode, u)
	if err != nil {
		fmt.Println("Error: Connection to room (%s) - %v", roomCode, err)
		return
	}

	run(conn, room, u)
}

// joinRoom: Add connection to room based on room code.
func joinRoom(roomCode string, user *User) (*Room, error) {
	if roomCode == "" {
		return nil, errors.New("Error: room code missing")
	}

	roomsMutex.Lock()
	defer roomsMutex.Unlock()

	if rooms[roomCode] == nil {
		rooms[roomCode] = &Room{
			connections: []*User{},
			objects:    make(map[string]*DrawingObject),
			lastActive:  time.Now(),
		}
	}

	room := rooms[roomCode]

	room.join(user)
	return room, nil
}

// run: Message loop for websocket.
func run(conn *websocket.Conn, room *Room, user *User) {

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("Error: Reading message", err)
			room.leave(user)
			break // conn dead
		}

	     	if err := handleMessage(room, user, msg); err != nil {	
			log.Println("error: Converting msg to json -", err)
			continue // Skip msg
		}
	}
}

func handleMessage(room *Room, user *User, msg []byte) error {
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
		return handleObjectAdded(room, user, data)
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

func handleGetUserID(user *User) error {
	response := map[string]interface{}{
		"type":    "userId",
		"userId":  user.id,
	}

	responseMsg, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal user ID response: %w", err)
	}

	return user.connection.WriteMessage(websocket.TextMessage, responseMsg)
}
func handleObjectAdded(room *Room, user *User, data map[string]interface{}) error {
	object, ok := data["object"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing object data")
	}

	id, ok := object["id"].(string)
	if !ok {
		return fmt.Errorf("missing object id")
	}

	// Create obj
	obj := &DrawingObject{
		ID: id,
		Type: object["type"].(string),
		Data: object["data"].(map[string]interface{}),
		UserId: user.id,
		Zindex: int(object["zIndex"].(float64)),
	}

	// add to room 
	room.mu.Lock()
	room.objects[id] = obj
	room.lastActive = time.Now()
	room.mu.Unlock()

	// broadcast
	data["userId"] = user.id
	msg, _ := json.Marshal(data)
	room.broadcast(msg, user.connection)
	return nil
}

func handleObjectUpdated(room *Room, user *User, data map[string]interface{}) error {
	object, ok := data["object"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing object data")
	}

	id, ok := object["id"].(string)
	if !ok {
		return fmt.Errorf("missing object id")
	}

	// update objects in room 
	room.mu.Lock()
	if obj, exists := room.objects[id]; exists {
		obj.Data = object["data"].(map[string]interface{})
	}
	room.lastActive = time.Now()
	room.mu.Unlock()

	// broadcast
	data["userId"] = user.id
	msg, _ := json.Marshal(data)
	room.broadcast(msg, user.connection)
	return nil
}

func handleObjectDeleted(room *Room, user *User, data map[string]interface{}) error {
	objectID, ok := data["objectId"].(string)
	if !ok {
		return fmt.Errorf("missing objectId")
	}

	// update objects in room 
	room.mu.Lock()
	delete(room.objects, objectID)
	room.lastActive = time.Now()
	room.mu.Unlock()

	// broadcast
	data["userId"] = user.id
	msg, _ := json.Marshal(data)
	room.broadcast(msg, user.connection)
	return nil
}


func handleCursor(room *Room, user *User, data map[string]interface{}) error {
	if time.Since(user.lastCursorTime) < 33*time.Millisecond {
		return nil // throttle
	}

	user.lastCursorTime = time.Now()
	data["color"] = user.color
	data["userId"] = user.id

	msg, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal cursor message: %w", err)
	}

	room.broadcast(msg, user.connection)
	return nil
}

// cleanupRooms: Routine to delete expired rooms.
func cleanupRooms(ctx context.Context){
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return 
		case <-ticker.C:
			roomsMutex.Lock()
			now := time.Now()

			for code, room := range rooms {
				room.mu.RLock()
				expired := (now.Sub(room.lastActive) > 1*time.Hour) && len(room.connections) == 0
				room.mu.RUnlock()

				if expired {
					delete(rooms, code)
					log.Printf("Room %s expired", code)
				}
			}
			roomsMutex.Unlock()
		}

	}
}

// getRandomHex: Returns well-distributed cursor colors using golden ratio
func getRandomHex() string {
	cursorColorMutex.Lock()
	defer cursorColorMutex.Unlock()

	const goldenRatio = 0.618033988749895
	hue := float64(cursorColorCounter) * goldenRatio
	hue = hue - float64(int(hue)) // Keep fractional part
	cursorColorCounter++

	color := colorful.Hsl(hue*360, 0.85, 0.55)
	return color.Hex()
}

