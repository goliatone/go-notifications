package bunrepo

import (
	"context"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type MessageRepository struct {
	base baseRepository[domain.NotificationMessage]
}

func NewMessageRepository(db *bun.DB) *MessageRepository {
	handlers := repository.ModelHandlers[*domain.NotificationMessage]{
		NewRecord:          func() *domain.NotificationMessage { return &domain.NotificationMessage{} },
		GetID:              func(m *domain.NotificationMessage) uuid.UUID { return m.ID },
		SetID:              func(m *domain.NotificationMessage, id uuid.UUID) { m.ID = id },
		GetIdentifier:      func() string { return "id" },
		GetIdentifierValue: func(m *domain.NotificationMessage) string { return m.ID.String() },
	}
	return &MessageRepository{
		base: newBaseRepository[domain.NotificationMessage](db, handlers, func(m *domain.NotificationMessage) *domain.RecordMeta { return &m.RecordMeta }),
	}
}

func (r *MessageRepository) Create(ctx context.Context, msg *domain.NotificationMessage) error {
	if msg.Status == "" {
		msg.Status = domain.MessageStatusPending
	}
	return r.base.create(ctx, msg)
}

func (r *MessageRepository) Update(ctx context.Context, msg *domain.NotificationMessage) error {
	return r.base.update(ctx, msg)
}

func (r *MessageRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.NotificationMessage, error) {
	return r.base.getByID(ctx, id, false)
}

func (r *MessageRepository) List(ctx context.Context, opts store.ListOptions) (store.ListResult[domain.NotificationMessage], error) {
	return r.base.list(ctx, opts)
}

func (r *MessageRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.base.softDelete(ctx, id)
}

func (r *MessageRepository) ListByEvent(ctx context.Context, eventID uuid.UUID) ([]domain.NotificationMessage, error) {
	records, _, err := r.base.repo.List(ctx, func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("event_id = ?", eventID)
	})
	if err != nil {
		return nil, mapError(err)
	}
	items := make([]domain.NotificationMessage, len(records))
	for i, rec := range records {
		items[i] = *rec
	}
	return items, nil
}
