package memory

import (
	"context"
	"time"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
)

type PublicationRepository struct {
	base baseMemoryRepo[domain.NotificationPublication]
}

func NewPublicationRepository() *PublicationRepository {
	return &PublicationRepository{
		base: newBaseMemoryRepo("publication", func(value *domain.NotificationPublication) *domain.RecordMeta {
			return &value.RecordMeta
		}),
	}
}

func (r *PublicationRepository) Create(ctx context.Context, value *domain.NotificationPublication) error {
	if value.Status == "" {
		value.Status = domain.PublicationStatusPending
	}
	return r.base.create(ctx, value)
}

func (r *PublicationRepository) Update(ctx context.Context, value *domain.NotificationPublication) error {
	return r.base.update(ctx, value)
}

func (r *PublicationRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.NotificationPublication, error) {
	return r.base.getByID(ctx, id, false)
}

func (r *PublicationRepository) List(ctx context.Context, opts store.ListOptions) (store.ListResult[domain.NotificationPublication], error) {
	return r.base.list(ctx, opts)
}

func (r *PublicationRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.base.softDelete(ctx, id)
}

func (r *PublicationRepository) ListPending(ctx context.Context, limit int) ([]domain.NotificationPublication, error) {
	result, err := r.base.list(ctx, store.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]domain.NotificationPublication, 0)
	for _, item := range result.Items {
		if item.Status == domain.PublicationStatusPending {
			out = append(out, item)
			if limit > 0 && len(out) == limit {
				break
			}
		}
	}
	return out, nil
}

func (r *PublicationRepository) FindOpenDigest(ctx context.Context, digestKey string) (*domain.NotificationPublication, error) {
	result, err := r.base.list(ctx, store.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, item := range result.Items {
		if item.DigestKey == digestKey &&
			(item.Status == domain.PublicationStatusPending || item.Status == domain.PublicationStatusPublished) {
			copy := item
			return &copy, nil
		}
	}
	return nil, store.ErrNotFound
}

func (r *PublicationRepository) Claim(_ context.Context, id uuid.UUID, until time.Time) (bool, error) {
	r.base.mu.Lock()
	defer r.base.mu.Unlock()
	item, ok := r.base.records[id]
	if !ok {
		return false, store.ErrNotFound
	}
	now := time.Now().UTC()
	if item.Status == domain.PublicationStatusCompleted {
		return false, nil
	}
	if item.Status == domain.PublicationStatusProcessing && item.ClaimUntil.After(now) {
		return false, nil
	}
	item.Status = domain.PublicationStatusProcessing
	item.ClaimUntil = until
	item.Attempts++
	item.UpdatedAt = now
	r.base.records[id] = item
	return true, nil
}
