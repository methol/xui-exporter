package store

import (
	"sync"

	"github.com/methol/xui-exporter/internal/compute"
)

// Store holds the current snapshot of subscription metrics
// It provides thread-safe atomic snapshot replacement
type Store struct {
	mu       sync.RWMutex
	snapshot map[string]compute.SubscriptionMetrics
}

// New creates a new Store with an empty snapshot
func New() *Store {
	return &Store{
		snapshot: make(map[string]compute.SubscriptionMetrics),
	}
}

// GetSnapshot returns a copy of the current snapshot
// Safe for concurrent reads
func (s *Store) GetSnapshot() map[string]compute.SubscriptionMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent external modification
	snapshot := make(map[string]compute.SubscriptionMetrics, len(s.snapshot))
	for k, v := range s.snapshot {
		snapshot[k] = v
	}
	return snapshot
}

// SetSnapshot atomically replaces the entire snapshot
// This is called after a full refresh cycle completes
func (s *Store) SetSnapshot(newSnapshot map[string]compute.SubscriptionMetrics) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.snapshot = newSnapshot
}
