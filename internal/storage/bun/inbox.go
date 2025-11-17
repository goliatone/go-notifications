package bunrepo

import (
	"context"
	"time"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type InboxRepository struct {
	base baseRepository[domain.InboxItem]
}

func NewInboxRepository(db *bun.DB) *InboxRepository {
	handlers := repository.ModelHandlers[*domain.InboxItem]{
		NewRecord:          func() *domain.InboxItem { return &domain.InboxItem{} },
		GetID:              func(i *domain.InboxItem) uuid.UUID { return i.ID },
		SetID:              func(i *domain.InboxItem, id uuid.UUID) { i.ID = id },
		GetIdentifier:      func() string { return "id" },
		GetIdentifierValue: func(i *domain.InboxItem) string { return i.ID.String() },
	}
	return &InboxRepository{
		base: newBaseRepository[domain.InboxItem](db, handlers, func(i *domain.InboxItem) *domain.RecordMeta { return &i.RecordMeta }),
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
	criteria := []repository.SelectCriteria{
		func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("user_id = ?", userID)
		},
		withListOptions(opts),
	}
	records, total, err := r.base.repo.List(ctx, criteria...)
	if err != nil {
		return store.ListResult[domain.InboxItem]{}, mapError(err)
	}
	items := make([]domain.InboxItem, len(records))
	for i, rec := range records {
		items[i] = *rec
	}
	return store.ListResult[domain.InboxItem]{Items: items, Total: total}, nil
}

func (r *InboxRepository) MarkRead(ctx context.Context, id uuid.UUID, read bool) error {
	record, err := r.base.getByID(ctx, id, false)
	if err != nil {
		return err
	}
	record.Unread = !read
	if read {
		record.ReadAt = time.Now().UTC()
	} else {
		record.ReadAt = time.Time{}
	}
	return r.base.update(ctx, record)
}

func (r *InboxRepository) Snooze(ctx context.Context, id uuid.UUID, until time.Time) error {
	_, err := r.base.db.
		NewUpdate().
		Model((*domain.InboxItem)(nil)).
		Set("snoozed_until = ?", until.UTC()).
		Where("id = ?", id).
		Exec(ctx)
	return mapError(err)
}

func (r *InboxRepository) Dismiss(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	_, err := r.base.db.
		NewUpdate().
		Model((*domain.InboxItem)(nil)).
		Set("dismissed_at = ?", now).
		Set("unread = ?", false).
		Where("id = ?", id).
		Exec(ctx)
	return mapError(err)
}

func (r *InboxRepository) CountUnread(ctx context.Context, userID string) (int, error) {
	count, err := r.base.db.
		NewSelect().
		Model((*domain.InboxItem)(nil)).
		Where("user_id = ?", userID).
		Where("unread = TRUE").
		Where("dismissed_at IS NULL").
		Count(ctx)
	return count, mapError(err)
}
