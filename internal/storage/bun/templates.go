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

type TemplateRepository struct {
	base baseRepository[domain.NotificationTemplate]
}

func NewTemplateRepository(db *bun.DB) *TemplateRepository {
	handlers := repository.ModelHandlers[*domain.NotificationTemplate]{
		NewRecord:          func() *domain.NotificationTemplate { return &domain.NotificationTemplate{} },
		GetID:              func(t *domain.NotificationTemplate) uuid.UUID { return t.ID },
		SetID:              func(t *domain.NotificationTemplate, id uuid.UUID) { t.ID = id },
		GetIdentifier:      func() string { return "code" },
		GetIdentifierValue: func(t *domain.NotificationTemplate) string { return t.Code },
	}
	return &TemplateRepository{
		base: newBaseRepository[domain.NotificationTemplate](db, handlers, func(t *domain.NotificationTemplate) *domain.RecordMeta { return &t.RecordMeta }),
	}
}

func (r *TemplateRepository) Create(ctx context.Context, tpl *domain.NotificationTemplate) error {
	return r.base.create(ctx, tpl)
}

func (r *TemplateRepository) Update(ctx context.Context, tpl *domain.NotificationTemplate) error {
	return r.base.update(ctx, tpl)
}

func (r *TemplateRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.NotificationTemplate, error) {
	return r.base.getByID(ctx, id, false)
}

func (r *TemplateRepository) List(ctx context.Context, opts store.ListOptions) (store.ListResult[domain.NotificationTemplate], error) {
	return r.base.list(ctx, opts)
}

func (r *TemplateRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.base.softDelete(ctx, id)
}

func (r *TemplateRepository) GetByCodeAndLocale(ctx context.Context, code, locale, channel string) (*domain.NotificationTemplate, error) {
	record, err := r.base.repo.Get(ctx,
		func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("LOWER(code) = ?", strings.ToLower(code)).
				Where("LOWER(locale) = ?", strings.ToLower(locale)).
				Where("LOWER(channel) = ?", strings.ToLower(channel))
		},
		withoutDeleted(),
	)
	if err != nil {
		return nil, mapError(err)
	}
	return record, nil
}

func (r *TemplateRepository) ListByCode(ctx context.Context, code string, opts store.ListOptions) (store.ListResult[domain.NotificationTemplate], error) {
	criteria := []repository.SelectCriteria{
		func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("LOWER(code) = ?", strings.ToLower(code))
		},
		withListOptions(opts),
	}
	records, total, err := r.base.repo.List(ctx, criteria...)
	if err != nil {
		return store.ListResult[domain.NotificationTemplate]{}, mapError(err)
	}
	items := make([]domain.NotificationTemplate, len(records))
	for i, rec := range records {
		items[i] = *rec
	}
	return store.ListResult[domain.NotificationTemplate]{Items: items, Total: total}, nil
}
