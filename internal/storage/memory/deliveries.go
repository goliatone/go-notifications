package memory

import (
	"context"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/google/uuid"
)

type DeliveryRepository struct {
	memoryCRUDRepository[domain.DeliveryAttempt]
}

func NewDeliveryRepository() *DeliveryRepository {
	return &DeliveryRepository{memoryCRUDRepository: newDeliveryCRUDRepository()}
}

func (r *DeliveryRepository) ListByMessage(ctx context.Context, messageID uuid.UUID) ([]domain.DeliveryAttempt, error) {
	return r.listMatching(ctx, func(attempt domain.DeliveryAttempt) bool {
		return attempt.MessageID == messageID
	})
}
