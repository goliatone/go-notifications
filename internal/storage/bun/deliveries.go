package bunrepo

import (
	"context"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type DeliveryRepository struct {
	base baseRepository[domain.DeliveryAttempt]
}

func NewDeliveryRepository(db *bun.DB) *DeliveryRepository {
	handlers := repository.ModelHandlers[*domain.DeliveryAttempt]{
		NewRecord:          func() *domain.DeliveryAttempt { return &domain.DeliveryAttempt{} },
		GetID:              func(d *domain.DeliveryAttempt) uuid.UUID { return d.ID },
		SetID:              func(d *domain.DeliveryAttempt, id uuid.UUID) { d.ID = id },
		GetIdentifier:      func() string { return "id" },
		GetIdentifierValue: func(d *domain.DeliveryAttempt) string { return d.ID.String() },
	}
	return &DeliveryRepository{
		base: newBaseRepository[domain.DeliveryAttempt](db, handlers, func(d *domain.DeliveryAttempt) *domain.RecordMeta { return &d.RecordMeta }),
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
	records, _, err := r.base.repo.List(ctx, func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("message_id = ?", messageID)
	})
	if err != nil {
		return nil, mapError(err)
	}
	items := make([]domain.DeliveryAttempt, len(records))
	for i, rec := range records {
		items[i] = *rec
	}
	return items, nil
}
