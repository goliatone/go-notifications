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

type PublicationRepository struct {
	base baseRepository[domain.NotificationPublication]
}

func NewPublicationRepository(db *bun.DB) *PublicationRepository {
	handlers := repository.ModelHandlers[*domain.NotificationPublication]{
		NewRecord:          func() *domain.NotificationPublication { return &domain.NotificationPublication{} },
		GetID:              func(value *domain.NotificationPublication) uuid.UUID { return value.ID },
		SetID:              func(value *domain.NotificationPublication, id uuid.UUID) { value.ID = id },
		GetIdentifier:      func() string { return "id" },
		GetIdentifierValue: func(value *domain.NotificationPublication) string { return value.ID.String() },
	}
	return &PublicationRepository{
		base: newBaseRepository[domain.NotificationPublication](db, handlers, func(value *domain.NotificationPublication) *domain.RecordMeta {
			return &value.RecordMeta
		}),
	}
}

func (r *PublicationRepository) Create(ctx context.Context, value *domain.NotificationPublication) error {
	if value.Status == "" {
		value.Status = domain.PublicationStatusPending
	}
	return r.base.create(ctx, value)
}

func (r *PublicationRepository) Update(ctx context.Context, value *domain.NotificationPublication) error {
	return r.base.update(ctx, value)
}

func (r *PublicationRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.NotificationPublication, error) {
	return r.base.getByID(ctx, id, false)
}

func (r *PublicationRepository) List(ctx context.Context, opts store.ListOptions) (store.ListResult[domain.NotificationPublication], error) {
	return r.base.list(ctx, opts)
}

func (r *PublicationRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.base.softDelete(ctx, id)
}

func (r *PublicationRepository) ListPending(ctx context.Context, limit int) ([]domain.NotificationPublication, error) {
	query := r.base.db.NewSelect().
		Model((*domain.NotificationPublication)(nil)).
		Where("status = ?", domain.PublicationStatusPending).
		Order("created_at ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var items []domain.NotificationPublication
	if err := query.Scan(ctx, &items); err != nil {
		return nil, mapError(err)
	}
	return items, nil
}

func (r *PublicationRepository) FindOpenDigest(ctx context.Context, digestKey string) (*domain.NotificationPublication, error) {
	value := &domain.NotificationPublication{}
	err := r.base.db.NewSelect().
		Model(value).
		Where("digest_key = ?", digestKey).
		Where("status IN (?, ?)", domain.PublicationStatusPending, domain.PublicationStatusPublished).
		Order("created_at ASC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	return value, nil
}

func (r *PublicationRepository) Claim(ctx context.Context, id uuid.UUID, until time.Time) (bool, error) {
	now := time.Now().UTC()
	result, err := r.base.db.NewUpdate().
		Model((*domain.NotificationPublication)(nil)).
		Set("status = ?", domain.PublicationStatusProcessing).
		Set("claim_until = ?", until).
		Set("attempts = attempts + 1").
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Where("status <> ?", domain.PublicationStatusCompleted).
		Where("(status <> ? OR claim_until < ? OR claim_until IS NULL)", domain.PublicationStatusProcessing, now).
		Exec(ctx)
	if err != nil {
		return false, mapError(err)
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
