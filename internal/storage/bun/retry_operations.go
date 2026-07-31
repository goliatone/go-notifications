package bunrepo

import (
	"context"
	"time"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type RetryOperationRepository struct {
	base baseRepository[domain.NotificationRetryOperation]
}

func NewRetryOperationRepository(db *bun.DB) *RetryOperationRepository {
	handlers := repository.ModelHandlers[*domain.NotificationRetryOperation]{
		NewRecord:          func() *domain.NotificationRetryOperation { return &domain.NotificationRetryOperation{} },
		GetID:              func(value *domain.NotificationRetryOperation) uuid.UUID { return value.ID },
		SetID:              func(value *domain.NotificationRetryOperation, id uuid.UUID) { value.ID = id },
		GetIdentifier:      func() string { return "id" },
		GetIdentifierValue: func(value *domain.NotificationRetryOperation) string { return value.ID.String() },
	}
	return &RetryOperationRepository{
		base: newBaseRepository[domain.NotificationRetryOperation](db, handlers, func(value *domain.NotificationRetryOperation) *domain.RecordMeta {
			return &value.RecordMeta
		}),
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

func (r *RetryOperationRepository) Update(ctx context.Context, value *domain.NotificationRetryOperation) error {
	return r.base.update(ctx, value)
}

func (r *RetryOperationRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.NotificationRetryOperation, error) {
	return r.base.getByID(ctx, id, false)
}

func (r *RetryOperationRepository) List(ctx context.Context, opts store.ListOptions) (store.ListResult[domain.NotificationRetryOperation], error) {
	return r.base.list(ctx, opts)
}

func (r *RetryOperationRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.base.softDelete(ctx, id)
}

func (r *RetryOperationRepository) Claim(ctx context.Context, id uuid.UUID, until time.Time) (bool, error) {
	now := time.Now().UTC()
	result, err := r.base.db.NewUpdate().
		Model((*domain.NotificationRetryOperation)(nil)).
		Set("status = ?", domain.RetryStatusProcessing).
		Set("claim_until = ?", until).
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Where("status <> ?", domain.RetryStatusCompleted).
		Where("(status <> ? OR claim_until < ? OR claim_until IS NULL)", domain.RetryStatusProcessing, now).
		Exec(ctx)
	if err != nil {
		return false, mapError(err)
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
