package memory

import (
	"context"
	"slices"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
)

type EventRepository struct {
	base baseMemoryRepo[domain.NotificationEvent]
}

func NewEventRepository() *EventRepository {
	return &EventRepository{
		base: newBaseMemoryRepo("event", func(e *domain.NotificationEvent) *domain.RecordMeta { return &e.RecordMeta }),
	}
}

func (r *EventRepository) Create(ctx context.Context, event *domain.NotificationEvent) error {
	if event.Status == "" {
		event.Status = domain.EventStatusPending
	}
	return r.base.create(ctx, event)
}

func (r *EventRepository) Update(ctx context.Context, event *domain.NotificationEvent) error {
	return r.base.update(ctx, event)
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
	result, err := r.base.list(ctx, store.ListOptions{})
	if err != nil {
		return nil, err
	}
	pending := make([]domain.NotificationEvent, 0, len(result.Items))
	for _, event := range result.Items {
		if event.Status == domain.EventStatusPending || event.Status == domain.EventStatusScheduled {
			pending = append(pending, event)
		}
	}
	if limit > 0 && len(pending) > limit {
		pending = slices.Clone(pending[:limit])
	}
	return pending, nil
}

func (r *EventRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	event, err := r.base.getByID(ctx, id, true)
	if err != nil {
		return err
	}
	event.Status = status
	return r.base.update(ctx, event)
}
