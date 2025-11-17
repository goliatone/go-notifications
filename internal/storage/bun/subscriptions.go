package bunrepo

import (
	"context"
	"strings"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type SubscriptionRepository struct {
	base baseRepository[domain.SubscriptionGroup]
}

func NewSubscriptionRepository(db *bun.DB) *SubscriptionRepository {
	handlers := repository.ModelHandlers[*domain.SubscriptionGroup]{
		NewRecord:          func() *domain.SubscriptionGroup { return &domain.SubscriptionGroup{} },
		GetID:              func(s *domain.SubscriptionGroup) uuid.UUID { return s.ID },
		SetID:              func(s *domain.SubscriptionGroup, id uuid.UUID) { s.ID = id },
		GetIdentifier:      func() string { return "code" },
		GetIdentifierValue: func(s *domain.SubscriptionGroup) string { return s.Code },
	}
	return &SubscriptionRepository{
		base: newBaseRepository[domain.SubscriptionGroup](db, handlers, func(s *domain.SubscriptionGroup) *domain.RecordMeta { return &s.RecordMeta }),
	}
}

func (r *SubscriptionRepository) Create(ctx context.Context, g *domain.SubscriptionGroup) error {
	return r.base.create(ctx, g)
}

func (r *SubscriptionRepository) Update(ctx context.Context, g *domain.SubscriptionGroup) error {
	return r.base.update(ctx, g)
}

func (r *SubscriptionRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.SubscriptionGroup, error) {
	return r.base.getByID(ctx, id, false)
}

func (r *SubscriptionRepository) List(ctx context.Context, opts store.ListOptions) (store.ListResult[domain.SubscriptionGroup], error) {
	return r.base.list(ctx, opts)
}

func (r *SubscriptionRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.base.softDelete(ctx, id)
}

func (r *SubscriptionRepository) GetByCode(ctx context.Context, code string) (*domain.SubscriptionGroup, error) {
	record, err := r.base.repo.Get(ctx,
		func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("LOWER(code) = ?", strings.ToLower(code))
		},
		withoutDeleted(),
	)
	if err != nil {
		return nil, mapError(err)
	}
	return record, nil
}
