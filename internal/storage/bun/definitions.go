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

type DefinitionRepository struct {
	base baseRepository[domain.NotificationDefinition]
}

func NewDefinitionRepository(db *bun.DB) *DefinitionRepository {
	handlers := repository.ModelHandlers[*domain.NotificationDefinition]{
		NewRecord: func() *domain.NotificationDefinition { return &domain.NotificationDefinition{} },
		GetID:     func(d *domain.NotificationDefinition) uuid.UUID { return d.ID },
		SetID: func(d *domain.NotificationDefinition, id uuid.UUID) {
			d.ID = id
		},
		GetIdentifier:      func() string { return "code" },
		GetIdentifierValue: func(d *domain.NotificationDefinition) string { return d.Code },
	}
	return &DefinitionRepository{
		base: newBaseRepository[domain.NotificationDefinition](db, handlers, func(d *domain.NotificationDefinition) *domain.RecordMeta { return &d.RecordMeta }),
	}
}

func (r *DefinitionRepository) Create(ctx context.Context, def *domain.NotificationDefinition) error {
	return r.base.create(ctx, def)
}

func (r *DefinitionRepository) Update(ctx context.Context, def *domain.NotificationDefinition) error {
	return r.base.update(ctx, def)
}

func (r *DefinitionRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.NotificationDefinition, error) {
	return r.base.getByID(ctx, id, false)
}

func (r *DefinitionRepository) List(ctx context.Context, opts store.ListOptions) (store.ListResult[domain.NotificationDefinition], error) {
	return r.base.list(ctx, opts)
}

func (r *DefinitionRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.base.softDelete(ctx, id)
}

func (r *DefinitionRepository) GetByCode(ctx context.Context, code string) (*domain.NotificationDefinition, error) {
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
