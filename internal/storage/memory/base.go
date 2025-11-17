package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
)

type baseMemoryRepo[T any] struct {
	mu        sync.RWMutex
	records   map[uuid.UUID]T
	extract   func(*T) *domain.RecordMeta
	entityStr string
}

func newBaseMemoryRepo[T any](entity string, extract func(*T) *domain.RecordMeta) baseMemoryRepo[T] {
	return baseMemoryRepo[T]{
		records:   make(map[uuid.UUID]T),
		extract:   extract,
		entityStr: entity,
	}
}

func (r *baseMemoryRepo[T]) create(ctx context.Context, record *T) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	base := r.extract(record)
	base.EnsureID()
	now := time.Now().UTC()
	if base.CreatedAt.IsZero() {
		base.CreatedAt = now
	}
	base.UpdatedAt = now
	r.records[base.ID] = *record
	return nil
}

func (r *baseMemoryRepo[T]) update(ctx context.Context, record *T) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	base := r.extract(record)
	if base.ID == uuid.Nil {
		return store.ErrNotFound
	}
	if _, ok := r.records[base.ID]; !ok {
		return store.ErrNotFound
	}
	base.UpdatedAt = time.Now().UTC()
	r.records[base.ID] = *record
	return nil
}

func (r *baseMemoryRepo[T]) getByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*T, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	record, ok := r.records[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	base := r.extract(&record)
	if !includeDeleted && !base.DeletedAt.IsZero() {
		return nil, store.ErrNotFound
	}
	copy := record
	return &copy, nil
}

func (r *baseMemoryRepo[T]) list(ctx context.Context, opts store.ListOptions) (store.ListResult[T], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []T
	for _, record := range r.records {
		base := r.extract(&record)
		if !opts.IncludeSoftDeleted && !base.DeletedAt.IsZero() {
			continue
		}
		if !opts.Since.IsZero() && base.CreatedAt.Before(opts.Since) {
			continue
		}
		if !opts.Until.IsZero() && base.CreatedAt.After(opts.Until) {
			continue
		}
		filtered = append(filtered, record)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return r.extract(&filtered[i]).CreatedAt.Before(r.extract(&filtered[j]).CreatedAt)
	})

	total := len(filtered)
	start := opts.Offset
	if start > total {
		start = total
	}
	end := total
	if opts.Limit > 0 && start+opts.Limit < end {
		end = start + opts.Limit
	}

	result := store.ListResult[T]{
		Items: filtered[start:end],
		Total: total,
	}
	return result, nil
}

func (r *baseMemoryRepo[T]) softDelete(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, ok := r.records[id]
	if !ok {
		return store.ErrNotFound
	}
	base := r.extract(&record)
	if base.DeletedAt.IsZero() {
		base.DeletedAt = time.Now().UTC()
	}
	r.records[id] = record
	return nil
}
