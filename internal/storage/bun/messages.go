package bunrepo

import (
	"context"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type MessageRepository struct {
	initializedRepository[domain.NotificationMessage]
}

func NewMessageRepository(db *bun.DB) *MessageRepository {
	return &MessageRepository{
		initializedRepository: newInitializedRepository(db,
			func(message *domain.NotificationMessage) *domain.RecordMeta { return &message.RecordMeta },
			func(message *domain.NotificationMessage) {
				if message.Status == "" {
					message.Status = domain.MessageStatusPending
				}
			}),
	}
}

func (r *MessageRepository) ListByEvent(ctx context.Context, eventID uuid.UUID) ([]domain.NotificationMessage, error) {
	return r.listByUUID(ctx, "event_id", eventID)
}
