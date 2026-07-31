package bunrepo

import (
	"context"
	"strings"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/uptrace/bun"
)

type TemplateRepository struct {
	crudRepository[domain.NotificationTemplate]
}

func NewTemplateRepository(db *bun.DB) *TemplateRepository {
	return &TemplateRepository{
		crudRepository: newEntityCRUD(db, "code",
			func(template *domain.NotificationTemplate) *domain.RecordMeta { return &template.RecordMeta },
			func(template *domain.NotificationTemplate) string { return template.Code }),
	}
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
