package bunrepo

import (
	"context"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type DeliveryRepository struct {
	initializedRepository[domain.DeliveryAttempt]
}

func NewDeliveryRepository(db *bun.DB) *DeliveryRepository {
	return &DeliveryRepository{
		initializedRepository: newInitializedRepository(db,
			func(attempt *domain.DeliveryAttempt) *domain.RecordMeta { return &attempt.RecordMeta },
			func(attempt *domain.DeliveryAttempt) {
				if attempt.Status == "" {
					attempt.Status = domain.AttemptStatusPending
				}
			}),
	}
}

func (r *DeliveryRepository) ListByMessage(ctx context.Context, messageID uuid.UUID) ([]domain.DeliveryAttempt, error) {
	return r.listByUUID(ctx, "message_id", messageID)
}
