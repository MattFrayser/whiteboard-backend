package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"github.com/lucasb-eyer/go-colorful"
	"golang.org/x/time/rate"
)


var upgrader = websocket.Upgrader{
	// CORS 
	CheckOrigin: func(r *http.Request) bool{
		origin := r.Header.Get("origin")

		allowedDomains := strings.Split(os.Getenv("DOMAINS"), ",")

		for _, allowed := range allowedDomains {
			if origin == strings.TrimSpace(allowed) {
				return true
			}
		}

		return false
	},
}

var (
	rooms 	   = make(map[string]*Room)
	roomsMutex sync.RWMutex

	userSessions  = make(map[string]*UserSession)
	sessionsMutex sync.RWMutex

	ipRateLimiter = &IPRateLimit{
		Limiters: make(map[string]*rate.Limiter),
	}

	// Global limits configuration
	config = &RateLimit{
		maxRoomSize:       10,    
		maxObjects:        10000,   
		maxMessageSize:    100000, // 100KB 
		maxRooms:          1000,  
		messagesPerSecond: 30,    
		burstSize:         10,    
	}

	cursorColorCounter int
	cursorColorMutex   sync.Mutex
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	godotenv.Load()

	http.Handle("/", http.FileServer(http.Dir("./frontend")))
	http.HandleFunc("/ws", handleWebSocket)

	// Start periodic cleanups
	go cleanupRooms(ctx)
	go cleanupSessions(ctx)
	go cleanupIPLimiters(ctx)

	// Run server
	log.Println("Server Started on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatalf("Error starting server: ", err)
	}
}

// getClientIP: Extract real client IP from request
func getClientIP(r *http.Request) string {
	// X-Forwarded-For 
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx] // Remove port
	}
	return ip
}

// handleWebSocket: Upgrades http to websocket then joins room
func handleWebSocket(w http.ResponseWriter, r *http.Request) {

	// Check if ratelimited
	clientIP := getClientIP(r)
	if !ipRateLimiter.Allow(clientIP) {
		log.Printf("Rate limit exceeded for IP: %s", clientIP)
		http.Error(w, "Too many connections", http.StatusTooManyRequests)
		return
	}

	// Upgrade conn
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error upgrading connection - ", err)
		return
	}
	defer conn.Close()
	
	// Retrieve roomCode from url
	roomCode := r.URL.Query().Get("room")
	if roomCode == "" {
		log.Println("Error: No room code provided")
		return
	}

	// Wait for authentication message with timeout
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Println("Error: Failed to receive auth message:", err)
		return
	}
	conn.SetReadDeadline(time.Time{}) // Clear timeout

	// Parse authentication message
	var authMsg struct {
		Type   string `json:"type"`
		UserID string `json:"userId"`
	}

	if err := json.Unmarshal(msg, &authMsg); err != nil {
		log.Println("Error: Invalid auth message format:", err)
		return
	}

	if authMsg.Type != "authenticate" {
		log.Println("Error: Expected authenticate message, got:", authMsg.Type)
		return
	}

	// Get or generate userId
	userID := authMsg.UserID
	if userID == "" {
		userID = generateUUID()
	}
	
	// Get session or create a new one
	session := getOrCreateSession(userID)
	session.lastRoom = roomCode // Track last room for resumption

	// Create user with session
	u := &User{
		id:         userID,
		session:    session,
		connection: conn,
	}

	// Send userId back to client (for new users or confirmation)
	response := map[string]interface{}{
		"type":   "authenticated",
		"userId": userID,
		"color":  session.color,
	}
	responseMsg, _ := json.Marshal(response)
	conn.WriteMessage(websocket.TextMessage, responseMsg)

	// Join room
	var room *Room

	// Check if user is rejoining their last room and it still exists
	if session.lastRoom == roomCode {
		if existingRoom, active := getRoomIfActive(roomCode); active {
			room = existingRoom
			room.join(u)
			log.Printf("User %s rejoined room %s", userID, roomCode)
		} else {
			// room expird, make new 
			room, err = joinRoom(roomCode, u)
			if err != nil {
				log.Printf("Error: Connection to room (%s) - %v", roomCode, err)
				return
			}
		}
	} else {
		// Joining a different room or first time
		room, err = joinRoom(roomCode, u)
		if err != nil {
			log.Printf("Error: Connection to room (%s) - %v", roomCode, err)
			return
		}
	}

	run(conn, room, u)
}

// getRoomIfActive: Check if room exists 
func getRoomIfActive(roomCode string) (*Room, bool) {
	roomsMutex.RLock()
	defer roomsMutex.RUnlock()

	room, exists := rooms[roomCode]
	return room, exists
}

// joinRoom: Add connection to room based on room code.
func joinRoom(roomCode string, user *User) (*Room, error) {
	if roomCode == "" {
		return nil, errors.New("Error: room code missing")
	}

	roomsMutex.Lock()
	defer roomsMutex.Unlock()

	if rooms[roomCode] == nil {
		// Check global room limit before creating new room
		if !config.CanCreateRoom(len(rooms)) {
			return nil, errors.New("Server at maximum room capacity")
		}

		rooms[roomCode] = &Room{
			connections: []*User{},
			objects:     make(map[string]*DrawingObject),
			lastActive:  time.Now(),
			createdAt:   time.Now(),
		}
	}

	room := rooms[roomCode]

	// Check room size limit before joining
	if !config.CanJoinRoom(room) {
		return nil, errors.New("Room is full")
	}

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

		// Validate message size
		if !config.ValidateMessageSize(len(msg)) {
			log.Printf("Message too large from user %s: %d bytes", user.id, len(msg))
			continue // Drop oversized message
		}

		// Check rate limit from session
		if !user.session.rateLimiter.Allow() {
			log.Printf("Rate limit exceeded for user: %s", user.id)
			continue // Drop message
		}

	     	if err := handleMessage(room, user, msg); err != nil {
			log.Println("error: Converting msg to json -", err)
			continue // Skip msg
		}
	}
}

// handleMessage: call handlers based on message type
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

// handleGetUserID: return userID
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

// handleObjectAdded: add object to room and broadcast to other users
func handleObjectAdded(room *Room, user *User, data map[string]interface{}) error {
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

// handleObjectUpdated: update object data and broadcast
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

// handleObjectDeleted: Remove object from room and broadcast
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

// handleCursor: update cursor position on screen and broadcast
func handleCursor(room *Room, user *User, data map[string]interface{}) error {
	now := time.Now()
	sessionsMutex.RLock()
	lastTime := user.session.lastSeen
	sessionsMutex.RUnlock()

	if now.Sub(lastTime) < 33*time.Millisecond {
		return nil // ignore to throttle
	}

	sessionsMutex.Lock()
	user.session.lastSeen = now
	sessionsMutex.Unlock()

	data["color"] = user.session.color 
	data["userId"] = user.id

	msg, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal cursor message: %w", err)
	}

	room.broadcast(msg, user.connection)
	return nil
}

// cleanupRooms: Routine to delete expired rooms.
// rooms last 1 hour of inactive and empty before delete
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

			// Room removed if 1 hour empty or 24 hours old
			for code, room := range rooms {
				room.mu.RLock()
				empty := len(room.connections) == 0
				inactive := now.Sub(room.lastActive) > 1*time.Hour	
				expired := now.Sub(room.createdAt) > 24*time.Hour
				room.mu.RUnlock()

				if (inactive && empty) || expired {
					delete(rooms, code)
					log.Printf("Room %s removed", code)
				}
			}
			roomsMutex.Unlock()
		}

	}
}

// cleanupSessions: Routine to delete expired user sessions
func cleanupSessions(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sessionsMutex.Lock()
			now := time.Now()

			for userID, session := range userSessions {
				// Remove sessions inactive for 24 hours
				if now.Sub(session.lastSeen) > 1*time.Hour {
					delete(userSessions, userID)
					log.Printf("Session %s expired", userID)
				}
			}
			sessionsMutex.Unlock()
		}
	}
}

// cleanupIPLimiters: Routine to clear IP rate limiters
func cleanupIPLimiters(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ipRateLimiter.Cleanup()
			log.Println("IP rate limiters cleared")
		}
	}
}

// generateUUID: Generates a random UUID for user identification
func generateUUID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// getOrCreateSession: Gets existing session or creates a new one
func getOrCreateSession(userID string) *UserSession {
	sessionsMutex.Lock()
	defer sessionsMutex.Unlock()

	session, exists := userSessions[userID]
	if exists {
		session.lastSeen = time.Now()
		return session
	}

	// Create new session with persistent color
	session = &UserSession{
		userID:      userID,
		lastSeen:    time.Now(),
		rateLimiter: rate.NewLimiter(30, 10), // 30 msg/sec, burst of 10
		color:       getRandomHex(),
	}
	userSessions[userID] = session
	return session
}

// getRandomHex: Returns well-distributed hex colors using golden ratio
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

