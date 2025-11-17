package memory

import (
	"context"
	"time"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
)

type InboxRepository struct {
	base baseMemoryRepo[domain.InboxItem]
}

func NewInboxRepository() *InboxRepository {
	return &InboxRepository{
		base: newBaseMemoryRepo("inbox_item", func(i *domain.InboxItem) *domain.RecordMeta { return &i.RecordMeta }),
	}
}

func (r *InboxRepository) Create(ctx context.Context, item *domain.InboxItem) error {
	return r.base.create(ctx, item)
}

func (r *InboxRepository) Update(ctx context.Context, item *domain.InboxItem) error {
	return r.base.update(ctx, item)
}

func (r *InboxRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.InboxItem, error) {
	return r.base.getByID(ctx, id, false)
}

func (r *InboxRepository) List(ctx context.Context, opts store.ListOptions) (store.ListResult[domain.InboxItem], error) {
	return r.base.list(ctx, opts)
}

func (r *InboxRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.base.softDelete(ctx, id)
}

func (r *InboxRepository) ListByUser(ctx context.Context, userID string, opts store.ListOptions) (store.ListResult[domain.InboxItem], error) {
	result, err := r.base.list(ctx, opts)
	if err != nil {
		return store.ListResult[domain.InboxItem]{}, err
	}
	filtered := make([]domain.InboxItem, 0, len(result.Items))
	for _, item := range result.Items {
		if item.UserID == userID {
			filtered = append(filtered, item)
		}
	}
	return store.ListResult[domain.InboxItem]{Items: filtered, Total: len(filtered)}, nil
}

func (r *InboxRepository) MarkRead(ctx context.Context, id uuid.UUID, read bool) error {
	item, err := r.base.getByID(ctx, id, false)
	if err != nil {
		return err
	}
	item.Unread = !read
	if read {
		item.ReadAt = time.Now().UTC()
	} else {
		item.ReadAt = time.Time{}
	}
	return r.base.update(ctx, item)
}

func (r *InboxRepository) Snooze(ctx context.Context, id uuid.UUID, until time.Time) error {
	item, err := r.base.getByID(ctx, id, false)
	if err != nil {
		return err
	}
	item.SnoozedUntil = until.UTC()
	return r.base.update(ctx, item)
}

func (r *InboxRepository) Dismiss(ctx context.Context, id uuid.UUID) error {
	item, err := r.base.getByID(ctx, id, false)
	if err != nil {
		return err
	}
	item.DismissedAt = time.Now().UTC()
	item.Unread = false
	return r.base.update(ctx, item)
}

func (r *InboxRepository) CountUnread(ctx context.Context, userID string) (int, error) {
	r.base.mu.RLock()
	defer r.base.mu.RUnlock()

	count := 0
	for _, item := range r.base.records {
		if item.UserID == userID && item.Unread && item.DismissedAt.IsZero() {
			count++
		}
	}
	return count, nil
}
