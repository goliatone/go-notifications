package bunrepo

import (
	"context"
	"time"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type RetryOperationRepository struct {
	crudRepository[domain.NotificationRetryOperation]
}

func NewRetryOperationRepository(db *bun.DB) *RetryOperationRepository {
	return &RetryOperationRepository{
		crudRepository: newEntityCRUD(db, "id",
			func(operation *domain.NotificationRetryOperation) *domain.RecordMeta { return &operation.RecordMeta }, nil),
	}
}

func (r *RetryOperationRepository) Create(ctx context.Context, value *domain.NotificationRetryOperation) error {
	_, _, err := r.CreateIdempotent(ctx, value)
	return err
}

func (r *RetryOperationRepository) CreateIdempotent(ctx context.Context, value *domain.NotificationRetryOperation) (*domain.NotificationRetryOperation, bool, error) {
	if value.Status == "" {
		value.Status = domain.RetryStatusPending
	}
	if err := r.base.create(ctx, value); err == nil {
		return value, true, nil
	}
	stored := &domain.NotificationRetryOperation{}
	err := r.base.db.NewSelect().
		Model(stored).
		Where("event_id = ?", value.EventID).
		Where("retry_scope = ?", value.RetryScope).
		Where("idempotency_key = ?", value.IdempotencyKey).
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, false, mapError(err)
	}
	return stored, false, nil
}

func (r *RetryOperationRepository) Claim(ctx context.Context, id uuid.UUID, until time.Time) (bool, error) {
	now := time.Now().UTC()
	result, err := r.base.db.NewUpdate().
		Model((*domain.NotificationRetryOperation)(nil)).
		Set("status = ?", domain.RetryStatusProcessing).
		Set("claim_until = ?", until).
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Where("status NOT IN (?, ?)", domain.RetryStatusCompleted, domain.RetryStatusFailed).
		Where("(status <> ? OR claim_until < ? OR claim_until IS NULL)", domain.RetryStatusProcessing, now).
		Exec(ctx)
	if err != nil {
		return false, mapError(err)
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
