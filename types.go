package main

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)
type User struct {
	id		string
	connection 	*websocket.Conn
	color		string
	lastCursorTime 	time.Time
}

type DrawingObject struct {
	ID 	string			`json:"id"`
	Type 	string 			`json:"type"`
	Data 	map[string]interface{}	`json:"data"`
	UserId  string			`json:"userId"`
	Zindex  int 			`json:"zIndex"`
}

type Room struct {
	connections     []*User
	objects         map[string]*DrawingObject
	lastActive	time.Time
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

