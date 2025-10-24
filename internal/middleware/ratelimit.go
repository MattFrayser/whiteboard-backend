package middleware

import (
	"fmt"
)

// ObjectCounter interface for counting objects (avoids import cycle with room)
type ObjectCounter interface {
	ObjectCount() int
}

//  configuration for rate limiting
type RateLimit struct {
	MaxRoomSize       int
	MaxObjects        int
	MaxMessageSize    int
	MaxRooms          int
	MaxObjectDepth    int
	MaxObjectElements int
	MessagesPerSecond float64
	BurstSize         int
}

// NewRateLimit: creates a new RateLimit configuration
func NewRateLimit(maxRoomSize, maxObjects, maxMessageSize, maxRooms, maxObjectDepth, maxObjectElements int, messagesPerSecond float64, burstSize int) *RateLimit {
	return &RateLimit{
		MaxRoomSize:       maxRoomSize,
		MaxObjects:        maxObjects,
		MaxMessageSize:    maxMessageSize,
		MaxRooms:          maxRooms,
		MaxObjectDepth:    maxObjectDepth,
		MaxObjectElements: maxObjectElements,
		MessagesPerSecond: messagesPerSecond,
		BurstSize:         burstSize,
	}
}

// CanAddObject: checks if a room has space for more objects
func (rl *RateLimit) CanAddObject(counter ObjectCounter) bool {
	return counter.ObjectCount() < rl.MaxObjects
}

// ValidateMessageSize: checks if a message is within the size limit
func (rl *RateLimit) ValidateMessageSize(msgSize int) bool {
	return msgSize <= rl.MaxMessageSize
}

// ValidateObjectComplexity: validates object data complexity
// Checks nesting depth and unique key count (not array lengths)
func (rl *RateLimit) ValidateObjectComplexity(data map[string]interface{}) error {
	depth, keys := validateComplexity(data, 0)

	if depth > rl.MaxObjectDepth {
		return fmt.Errorf("object nesting too deep: %d levels (max %d)", depth, rl.MaxObjectDepth)
	}

	if keys > rl.MaxObjectElements {
		return fmt.Errorf("object too complex: %d keys (max %d)", keys, rl.MaxObjectElements)
	}

	return nil
}

// validateComplexity: recursively checks depth and counts unique keys
func validateComplexity(data interface{}, currentDepth int) (int, int) {
	maxDepth := currentDepth
	keyCount := 0

	switch v := data.(type) {
	case map[string]interface{}:
		keyCount = len(v) 
		for _, val := range v {
			subDepth, subKeys := validateComplexity(val, currentDepth+1)
			if subDepth > maxDepth {
				maxDepth = subDepth
			}
			keyCount += subKeys
		}
	case []interface{}:
		// Don't count array length
		for _, val := range v {
			subDepth, subKeys := validateComplexity(val, currentDepth+1)
			if subDepth > maxDepth {
				maxDepth = subDepth
			}
			keyCount += subKeys
		}
	}

	return maxDepth, keyCount
}
