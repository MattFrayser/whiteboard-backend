package middleware

import (
	"sync"
)

// ConnectionTracker tracks the total number of active WebSocket connections globally
type ConnectionTracker struct {
	activeConnections int
	maxConnections    int
	mu                sync.RWMutex
}

// NewConnectionTracker creates a new connection tracker with the specified maximum
func NewConnectionTracker(maxConnections int) *ConnectionTracker {
	return &ConnectionTracker{
		maxConnections:    maxConnections,
		activeConnections: 0,
	}
}

// CanConnect checks if a new connection can be accepted without exceeding the limit
func (ct *ConnectionTracker) CanConnect() bool {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.activeConnections < ct.maxConnections
}

// Connect attempts to acquire a connection slot
// Returns true if successful, false if at capacity
func (ct *ConnectionTracker) Connect() bool {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if ct.activeConnections >= ct.maxConnections {
		return false
	}
	ct.activeConnections++
	return true
}

// Disconnect releases a connection slot
func (ct *ConnectionTracker) Disconnect() {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if ct.activeConnections > 0 {
		ct.activeConnections--
	}
}

// Count returns the current number of active connections
func (ct *ConnectionTracker) Count() int {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.activeConnections
}

// GetMaxConnections returns the maximum allowed connections
func (ct *ConnectionTracker) GetMaxConnections() int {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.maxConnections
}
