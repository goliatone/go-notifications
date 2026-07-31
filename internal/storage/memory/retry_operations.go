package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
)

type RetryOperationRepository struct {
	base     baseMemoryRepo[domain.NotificationRetryOperation]
	identity map[string]uuid.UUID
}

func NewRetryOperationRepository() *RetryOperationRepository {
	return &RetryOperationRepository{
		base: newBaseMemoryRepo("retry_operation", func(value *domain.NotificationRetryOperation) *domain.RecordMeta {
			return &value.RecordMeta
		}),
		identity: make(map[string]uuid.UUID),
	}
}

func (r *RetryOperationRepository) Create(ctx context.Context, value *domain.NotificationRetryOperation) error {
	_, _, err := r.CreateIdempotent(ctx, value)
	return err
}

func (r *RetryOperationRepository) CreateIdempotent(_ context.Context, value *domain.NotificationRetryOperation) (*domain.NotificationRetryOperation, bool, error) {
	r.base.mu.Lock()
	defer r.base.mu.Unlock()
	key := retryIdentity(value.EventID, value.RetryScope, value.IdempotencyKey)
	if id, ok := r.identity[key]; ok {
		stored := r.base.records[id]
		return &stored, false, nil
	}
	value.EnsureID()
	now := time.Now().UTC()
	if value.CreatedAt.IsZero() {
		value.CreatedAt = now
	}
	value.UpdatedAt = now
	if value.Status == "" {
		value.Status = domain.RetryStatusPending
	}
	r.base.records[value.ID] = *value
	r.identity[key] = value.ID
	copy := *value
	return &copy, true, nil
}

func (r *RetryOperationRepository) Update(ctx context.Context, value *domain.NotificationRetryOperation) error {
	return r.base.update(ctx, value)
}

func (r *RetryOperationRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.NotificationRetryOperation, error) {
	return r.base.getByID(ctx, id, false)
}

func (r *RetryOperationRepository) List(ctx context.Context, opts store.ListOptions) (store.ListResult[domain.NotificationRetryOperation], error) {
	return r.base.list(ctx, opts)
}

func (r *RetryOperationRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.base.softDelete(ctx, id)
}

func (r *RetryOperationRepository) Claim(_ context.Context, id uuid.UUID, until time.Time) (bool, error) {
	r.base.mu.Lock()
	defer r.base.mu.Unlock()
	item, ok := r.base.records[id]
	if !ok {
		return false, store.ErrNotFound
	}
	now := time.Now().UTC()
	if item.Status == domain.RetryStatusCompleted {
		return false, nil
	}
	if item.Status == domain.RetryStatusProcessing && item.ClaimUntil.After(now) {
		return false, nil
	}
	item.Status = domain.RetryStatusProcessing
	item.ClaimUntil = until
	item.UpdatedAt = now
	r.base.records[id] = item
	return true, nil
}

func retryIdentity(eventID uuid.UUID, scope, key string) string {
	return fmt.Sprintf("%s\x00%s\x00%s", eventID, scope, key)
}
