package memory

import (
	"context"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
)

type MessageRepository struct {
	base baseMemoryRepo[domain.NotificationMessage]
}

func NewMessageRepository() *MessageRepository {
	return &MessageRepository{
		base: newBaseMemoryRepo("message", func(m *domain.NotificationMessage) *domain.RecordMeta { return &m.RecordMeta }),
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
	res, err := r.base.list(ctx, store.ListOptions{})
	if err != nil {
		return nil, err
	}
	items := make([]domain.NotificationMessage, 0)
	for _, msg := range res.Items {
		if msg.EventID == eventID {
			items = append(items, msg)
		}
	}
	return items, nil
}
