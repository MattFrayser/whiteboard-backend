package transport

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ConnectionRegistry tracks all active WebSocket connections for coordinated shutdown
type ConnectionRegistry struct {
	connections map[*websocket.Conn]context.CancelFunc
	mu          sync.RWMutex
}

// NewConnectionRegistry creates a new connection registry
func NewConnectionRegistry() *ConnectionRegistry {
	return &ConnectionRegistry{
		connections: make(map[*websocket.Conn]context.CancelFunc),
	}
}

// Register adds a connection with its cancel function for shutdown coordination
func (cr *ConnectionRegistry) Register(conn *websocket.Conn, cancel context.CancelFunc) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.connections[conn] = cancel
	log.Printf("Connection registered. Total active: %d", len(cr.connections))
}

// Unregister removes a connection from tracking
func (cr *ConnectionRegistry) Unregister(conn *websocket.Conn) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	delete(cr.connections, conn)
	log.Printf("Connection unregistered. Total active: %d", len(cr.connections))
}

// Count returns the number of currently active connections
func (cr *ConnectionRegistry) Count() int {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return len(cr.connections)
}

// CloseAll gracefully closes all tracked connections during shutdown
// Sends WebSocket close frames and cancels contexts
func (cr *ConnectionRegistry) CloseAll(ctx context.Context) {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	count := len(cr.connections)
	if count == 0 {
		log.Println("No active connections to close")
		return
	}

	log.Printf("Closing %d active WebSocket connection(s)...", count)

	// Create WaitGroup to wait for all close operations
	var wg sync.WaitGroup
	wg.Add(count)

	// Send close message to all connections concurrently
	for conn, cancel := range cr.connections {
		go func(c *websocket.Conn, cancelFunc context.CancelFunc) {
			defer wg.Done()

			// Cancel the connection's context first
			cancelFunc()

			// Send WebSocket close frame (1000 = normal closure)
			closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Server shutting down")
			deadline := time.Now().Add(5 * time.Second)
			if err := c.WriteControl(websocket.CloseMessage, closeMsg, deadline); err != nil {
				log.Printf("Error sending close frame: %v", err)
			}

			// Close the connection
			c.Close()
		}(conn, cancel)
	}

	// Wait for all close operations to complete or context timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All WebSocket connections closed successfully")
	case <-ctx.Done():
		log.Println("WebSocket closure timeout reached, forcing close")
	}

	// Clear the map
	cr.connections = make(map[*websocket.Conn]context.CancelFunc)
}
