package memory

import (
	"context"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/google/uuid"
)

type MessageRepository struct {
	memoryCRUDRepository[domain.NotificationMessage]
}

func NewMessageRepository() *MessageRepository {
	return &MessageRepository{memoryCRUDRepository: newMessageCRUDRepository()}
}

func (r *MessageRepository) ListByEvent(ctx context.Context, eventID uuid.UUID) ([]domain.NotificationMessage, error) {
	return r.listMatching(ctx, func(message domain.NotificationMessage) bool {
		return message.EventID == eventID
	})
}
