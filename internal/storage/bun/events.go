package bunrepo

import (
	"context"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type EventRepository struct {
	base baseRepository[domain.NotificationEvent]
}

func NewEventRepository(db *bun.DB) *EventRepository {
	handlers := repository.ModelHandlers[*domain.NotificationEvent]{
		NewRecord:          func() *domain.NotificationEvent { return &domain.NotificationEvent{} },
		GetID:              func(e *domain.NotificationEvent) uuid.UUID { return e.ID },
		SetID:              func(e *domain.NotificationEvent, id uuid.UUID) { e.ID = id },
		GetIdentifier:      func() string { return "id" },
		GetIdentifierValue: func(e *domain.NotificationEvent) string { return e.ID.String() },
	}
	return &EventRepository{
		base: newBaseRepository[domain.NotificationEvent](db, handlers, func(e *domain.NotificationEvent) *domain.RecordMeta { return &e.RecordMeta }),
	}
}

func (r *EventRepository) Create(ctx context.Context, e *domain.NotificationEvent) error {
	if e.Status == "" {
		e.Status = domain.EventStatusPending
	}
	return r.base.create(ctx, e)
}

func (r *EventRepository) Update(ctx context.Context, e *domain.NotificationEvent) error {
	return r.base.update(ctx, e)
}

func (r *EventRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.NotificationEvent, error) {
	return r.base.getByID(ctx, id, false)
}

func (r *EventRepository) List(ctx context.Context, opts store.ListOptions) (store.ListResult[domain.NotificationEvent], error) {
	return r.base.list(ctx, opts)
}

func (r *EventRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.base.softDelete(ctx, id)
}

func (r *EventRepository) ListPending(ctx context.Context, limit int) ([]domain.NotificationEvent, error) {
	criteria := []repository.SelectCriteria{
		func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("status IN (?, ?)", domain.EventStatusPending, domain.EventStatusScheduled).
				Order("created_at ASC")
		},
	}
	if limit > 0 {
		criteria = append(criteria, func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Limit(limit)
		})
	}
	records, _, err := r.base.repo.List(ctx, criteria...)
	if err != nil {
		return nil, mapError(err)
	}
	items := make([]domain.NotificationEvent, len(records))
	for i, rec := range records {
		items[i] = *rec
	}
	return items, nil
}

func (r *EventRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	event, err := r.base.getByID(ctx, id, true)
	if err != nil {
		return err
	}
	event.Status = status
	return r.base.update(ctx, event)
}
