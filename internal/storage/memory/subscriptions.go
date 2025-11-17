package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
)

type SubscriptionRepository struct {
	base   baseMemoryRepo[domain.SubscriptionGroup]
	byCode map[string]uuid.UUID
}

func NewSubscriptionRepository() *SubscriptionRepository {
	return &SubscriptionRepository{
		base:   newBaseMemoryRepo("subscription_group", func(s *domain.SubscriptionGroup) *domain.RecordMeta { return &s.RecordMeta }),
		byCode: make(map[string]uuid.UUID),
	}
}

func (r *SubscriptionRepository) Create(ctx context.Context, group *domain.SubscriptionGroup) error {
	key := strings.ToLower(group.Code)
	if _, ok := r.byCode[key]; ok {
		return fmt.Errorf("subscription group %s already exists", group.Code)
	}
	if err := r.base.create(ctx, group); err != nil {
		return err
	}
	r.byCode[key] = group.ID
	return nil
}

func (r *SubscriptionRepository) Update(ctx context.Context, group *domain.SubscriptionGroup) error {
	return r.base.update(ctx, group)
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
	id, ok := r.byCode[strings.ToLower(code)]
	if !ok {
		return nil, store.ErrNotFound
	}
	return r.base.getByID(ctx, id, false)
}
