package memory

import (
	"context"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
)

type DeliveryRepository struct {
	base baseMemoryRepo[domain.DeliveryAttempt]
}

func NewDeliveryRepository() *DeliveryRepository {
	return &DeliveryRepository{
		base: newBaseMemoryRepo("delivery_attempt", func(d *domain.DeliveryAttempt) *domain.RecordMeta { return &d.RecordMeta }),
	}
}

func (r *DeliveryRepository) Create(ctx context.Context, attempt *domain.DeliveryAttempt) error {
	if attempt.Status == "" {
		attempt.Status = domain.AttemptStatusPending
	}
	return r.base.create(ctx, attempt)
}

func (r *DeliveryRepository) Update(ctx context.Context, attempt *domain.DeliveryAttempt) error {
	return r.base.update(ctx, attempt)
}

func (r *DeliveryRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.DeliveryAttempt, error) {
	return r.base.getByID(ctx, id, false)
}

func (r *DeliveryRepository) List(ctx context.Context, opts store.ListOptions) (store.ListResult[domain.DeliveryAttempt], error) {
	return r.base.list(ctx, opts)
}

func (r *DeliveryRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.base.softDelete(ctx, id)
}

func (r *DeliveryRepository) ListByMessage(ctx context.Context, messageID uuid.UUID) ([]domain.DeliveryAttempt, error) {
	result, err := r.base.list(ctx, store.ListOptions{})
	if err != nil {
		return nil, err
	}
	items := make([]domain.DeliveryAttempt, 0)
	for _, attempt := range result.Items {
		if attempt.MessageID == messageID {
			items = append(items, attempt)
		}
	}
	return items, nil
}
