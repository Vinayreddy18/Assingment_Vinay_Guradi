// Package store provides a concurrency-safe in-memory repository for disputes.
// It is intentionally behind a small interface-shaped surface so it can be
// swapped for DynamoDB or Postgres without touching the engines or handlers.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/pranav/samadhan/internal/domain"
)

// Store is an in-memory dispute repository guarded by a read/write mutex.
type Store struct {
	mu   sync.RWMutex
	data map[string]*domain.Dispute
}

// New builds an empty store.
func New() *Store {
	return &Store{data: make(map[string]*domain.Dispute)}
}

// Create assigns an ID and timestamps and stores the dispute.
func (s *Store) Create(d *domain.Dispute) *domain.Dispute {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	d.ID = newID()
	d.CreatedAt = now
	d.UpdatedAt = now
	if d.Status == "" {
		d.Status = domain.StatusIntake
	}
	s.data[d.ID] = d
	return d
}

// Get returns the dispute by ID or domain.ErrNotFound.
func (s *Store) Get(id string) (*domain.Dispute, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.data[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return d, nil
}

// Update persists changes to an existing dispute.
func (s *Store) Update(d *domain.Dispute) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[d.ID]; !ok {
		return domain.ErrNotFound
	}
	d.UpdatedAt = time.Now().UTC()
	s.data[d.ID] = d
	return nil
}

// List returns all disputes (unordered).
func (s *Store) List() []*domain.Dispute {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*domain.Dispute, 0, len(s.data))
	for _, d := range s.data {
		out = append(out, d)
	}
	return out
}

// newID returns a short, human-friendly case reference like "SAMA-9f3a2b1c".
func newID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fall back to a time-based suffix; collisions are astronomically
		// unlikely for a demo and this avoids a hard failure.
		return "SAMA-" + hex.EncodeToString([]byte(time.Now().Format("150405")))
	}
	return "SAMA-" + hex.EncodeToString(b)
}
