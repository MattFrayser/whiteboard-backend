package tests

import (
	"sync"
	"testing"

	"main/internal/middleware"
)

func TestConnectionTracker_Connect(t *testing.T) {
	tracker := middleware.NewConnectionTracker()

	if !tracker.Connect() {
		t.Error("First connection should succeed")
	}

	if tracker.Count() != 1 {
		t.Errorf("Expected count 1, got %d", tracker.Count())
	}
}

func TestConnectionTracker_Disconnect(t *testing.T) {
	tracker := middleware.NewConnectionTracker()

	tracker.Connect()
	tracker.Disconnect()

	if tracker.Count() != 0 {
		t.Errorf("Expected count 0 after disconnect, got %d", tracker.Count())
	}
}

func TestConnectionTracker_MaxConnections(t *testing.T) {
	tracker := middleware.NewConnectionTracker()
	max := tracker.GetMaxConnections()

	// Fill to capacity
	for i := 0; i < max; i++ {
		if !tracker.Connect() {
			t.Fatalf("Connection %d should succeed (max: %d)", i, max)
		}
	}

	// Next connection should fail
	if tracker.Connect() {
		t.Error("Connection should fail when at capacity")
	}

	if tracker.Count() != max {
		t.Errorf("Expected count %d, got %d", max, tracker.Count())
	}
}

func TestConnectionTracker_CanConnect(t *testing.T) {
	tracker := middleware.NewConnectionTracker()

	if !tracker.CanConnect() {
		t.Error("Should be able to connect initially")
	}

	// Fill to capacity
	max := tracker.GetMaxConnections()
	for i := 0; i < max; i++ {
		tracker.Connect()
	}

	if tracker.CanConnect() {
		t.Error("Should not be able to connect at capacity")
	}
}

func TestConnectionTracker_ConcurrentAccess(t *testing.T) {
	tracker := middleware.NewConnectionTracker()
	iterations := 100

	var wg sync.WaitGroup

	// Concurrent connects
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.Connect()
		}()
	}

	wg.Wait()

	if tracker.Count() != iterations {
		t.Errorf("Expected count %d, got %d", iterations, tracker.Count())
	}

	// Concurrent disconnects
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.Disconnect()
		}()
	}

	wg.Wait()

	if tracker.Count() != 0 {
		t.Errorf("Expected count 0 after all disconnects, got %d", tracker.Count())
	}
}

func TestConnectionTracker_DisconnectWhenEmpty(t *testing.T) {
	tracker := middleware.NewConnectionTracker()

	// Disconnect when count is 0 should not go negative
	tracker.Disconnect()

	if tracker.Count() != 0 {
		t.Errorf("Count should not go negative, got %d", tracker.Count())
	}
}
