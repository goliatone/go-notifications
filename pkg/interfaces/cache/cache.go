package cache

import (
	"context"
	"time"
)

// Cache exposes the minimal API needed for template + preference caching.
type Cache interface {
	Get(ctx context.Context, key string) (any, bool, error)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// Nop cache returns misses and ignores writes.
type Nop struct{}

var _ Cache = (*Nop)(nil)

func (n *Nop) Get(ctx context.Context, key string) (any, bool, error) { return nil, false, nil }
func (n *Nop) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return nil
}
func (n *Nop) Delete(ctx context.Context, key string) error { return nil }
