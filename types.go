package main

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/time/rate"
)

//---------------
// USER
//---------------

// userSession persists on disconnects 
type UserSession struct {
	userID 		string
	lastRoom	string
	lastSeen 	time.Time
	rateLimiter 	*rate.Limiter
	color		string
}
type User struct {
	id		string
	session 	*UserSession
	connection 	*websocket.Conn
}



type DrawingObject struct {
	ID 	string			`json:"id"`
	Type 	string 			`json:"type"`
	Data 	map[string]interface{}	`json:"data"`
	UserId  string			`json:"userId"`
	Zindex  int 			`json:"zIndex"`
}

//---------------
// ROOM 
//---------------
type Room struct {
	connections     []*User
	objects         map[string]*DrawingObject
	lastActive	time.Time
	createdAt	time.Time
	mu 		sync.RWMutex
}


// join: adds user to room, sends existing drawings
func (r *Room) join(u *User) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.connections = append(r.connections, u)

	// sync all obj
	objects := make([]map[string]interface{}, 0, len(r.objects))
	for _, obj := range r.objects {
		objects = append(objects, map[string]interface{}{
			"id": obj.ID,
			"type": obj.Type,
			"data": obj.Data, 
			"userId": obj.UserId,
			"zIndex": obj.Zindex,
		})
	}

	syncMsg := map[string]interface{}{
		"type": "sync",
		"objects": objects,
	}

	msgBytes, _ := json.Marshal(syncMsg)
	u.connection.WriteMessage(websocket.TextMessage, msgBytes)
}

func (r *Room) leave(u *User) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, v := range r.connections {
		if v == u {
			r.connections = append(r.connections[:i], r.connections[i+1:]...)
			break
		}
	}

	r.lastActive = time.Now()
}

func (r *Room) broadcast(msg []byte, sender *websocket.Conn) {
	r.mu.RLock()
	connections := make([]*User, len(r.connections))
	copy(connections, r.connections)
	r.mu.RUnlock()
	
	var failed []*User
	for _, u := range connections {
		if u.connection == sender {
			continue
		}

		if err := u .connection.WriteMessage(websocket.TextMessage, msg); err != nil {
			failed = append(failed, u)
		}
	}

	// Remove failed conns
	if len(failed) > 0 {
		r.mu.Lock()
		for _, failedUser := range failed {
			for i, u := range r.connections {
				if u == failedUser {
					r.connections = append(r.connections[:i], r.connections[i+1:]...)
					break
				}
			}
		}
		r.mu.Unlock()
	}
}

//------------------
// RateLimit Struct 
//------------------
type RateLimit struct {
	maxRoomSize 		int
	maxObjects 		int
	maxMessageSize		int
	maxRooms 		int
	messagesPerSecond 	float64
	burstSize 		int
}

type IPRateLimit struct {
	Limiters	map[string]*rate.Limiter
	mu 		sync.RWMutex
}

// CanCreateRoom: Check if server can accept more rooms
func (rl *RateLimit) CanCreateRoom(currentRoomCount int) bool {
	return currentRoomCount < rl.maxRooms
}

// CanAddObject: Check if room has space for more objects
func (rl *RateLimit) CanAddObject(room *Room) bool {
	room.mu.RLock()
	defer room.mu.RUnlock()
	return len(room.objects) < rl.maxObjects
}

// CanJoinRoom: Check if room has space for more users
func (rl *RateLimit) CanJoinRoom(room *Room) bool {
	room.mu.RLock()
	defer room.mu.RUnlock()
	return len(room.connections) < rl.maxRoomSize
}

// ValidateMessageSize: Check if message is within size limit
func (rl *RateLimit) ValidateMessageSize(msgSize int) bool {
	return msgSize <= rl.maxMessageSize
}

//------------------
// IP Rate Limiting
//------------------

// Allow: Check if IP is allowed to make a request
func (iprl *IPRateLimit) Allow(ip string) bool {
	iprl.mu.Lock()
	defer iprl.mu.Unlock()

	limiter, exists := iprl.Limiters[ip]
	if !exists {
		// New IP: 10 connections per minute, burst of 5
		limiter = rate.NewLimiter(rate.Every(6*time.Second), 5)
		iprl.Limiters[ip] = limiter
	}

	return limiter.Allow()
}

// Cleanup: Remove old IP limiters (call periodically)
func (iprl *IPRateLimit) Cleanup() {
	iprl.mu.Lock()
	defer iprl.mu.Unlock()

	// Clear all limiters (they'll be recreated on demand)
	// In production, you'd track last use time and only remove old ones
	iprl.Limiters = make(map[string]*rate.Limiter)
}
