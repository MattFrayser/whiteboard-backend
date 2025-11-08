package transport

import (
	"context"
	"sync"
	"time"

	"main/internal/logger"

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
}

// Unregister removes a connection from tracking
func (cr *ConnectionRegistry) Unregister(conn *websocket.Conn) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	delete(cr.connections, conn)
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
			return
	}

	logger.Info("Closing active WebSocket connections").
		Int("count", count).
		Msg("")

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
				logger.Warn("Error sending close frame").
					Err(err).
					Msg("")
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
		logger.Info("All WebSocket connections closed successfully").Msg("")
	case <-ctx.Done():
		logger.Warn("WebSocket closure timeout reached, forcing close").Msg("")
	}

	// Clear the map
	cr.connections = make(map[*websocket.Conn]context.CancelFunc)
}
