package inmemory

import (
	"context"
	"sync"
)

// Repository is an in-memory implementation of system params (for tests / dump-runchecks).
type Repository struct {
	mu   sync.RWMutex
	data map[string]string
}

func NewRepository() *Repository {
	return &Repository{
		data: make(map[string]string),
	}
}

func (r *Repository) SetParam(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[key] = value
	return nil
}

func (r *Repository) GetParam(_ context.Context, key string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.data[key], nil
}
