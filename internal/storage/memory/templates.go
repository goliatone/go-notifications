package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
)

type TemplateRepository struct {
	base  baseMemoryRepo[domain.NotificationTemplate]
	byKey map[string]uuid.UUID
}

func NewTemplateRepository() *TemplateRepository {
	return &TemplateRepository{
		base:  newBaseMemoryRepo("template", func(t *domain.NotificationTemplate) *domain.RecordMeta { return &t.RecordMeta }),
		byKey: make(map[string]uuid.UUID),
	}
}

func templateKey(code, locale, channel string) string {
	return strings.ToLower(fmt.Sprintf("%s|%s|%s", code, locale, channel))
}

func (r *TemplateRepository) Create(ctx context.Context, t *domain.NotificationTemplate) error {
	if t == nil {
		return store.ErrNotFound
	}
	key := templateKey(t.Code, t.Locale, t.Channel)
	if _, ok := r.byKey[key]; ok {
		return fmt.Errorf("template %s already exists for %s/%s", t.Code, t.Locale, t.Channel)
	}
	if err := r.base.create(ctx, t); err != nil {
		return err
	}
	r.byKey[key] = t.ID
	return nil
}

func (r *TemplateRepository) Update(ctx context.Context, t *domain.NotificationTemplate) error {
	return r.base.update(ctx, t)
}

func (r *TemplateRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.NotificationTemplate, error) {
	return r.base.getByID(ctx, id, false)
}

func (r *TemplateRepository) GetByCodeAndLocale(ctx context.Context, code, locale, channel string) (*domain.NotificationTemplate, error) {
	key := templateKey(code, locale, channel)
	id, ok := r.byKey[key]
	if !ok {
		return nil, store.ErrNotFound
	}
	return r.base.getByID(ctx, id, false)
}

func (r *TemplateRepository) List(ctx context.Context, opts store.ListOptions) (store.ListResult[domain.NotificationTemplate], error) {
	return r.base.list(ctx, opts)
}

func (r *TemplateRepository) ListByCode(ctx context.Context, code string, opts store.ListOptions) (store.ListResult[domain.NotificationTemplate], error) {
	all, err := r.base.list(ctx, opts)
	if err != nil {
		return store.ListResult[domain.NotificationTemplate]{}, err
	}
	filtered := make([]domain.NotificationTemplate, 0, len(all.Items))
	for _, item := range all.Items {
		if strings.EqualFold(item.Code, code) {
			filtered = append(filtered, item)
		}
	}
	return store.ListResult[domain.NotificationTemplate]{Items: filtered, Total: len(filtered)}, nil
}

func (r *TemplateRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.base.softDelete(ctx, id)
}
