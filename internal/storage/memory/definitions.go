package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
)

type DefinitionRepository struct {
	base   baseMemoryRepo[domain.NotificationDefinition]
	byCode map[string]uuid.UUID
}

func NewDefinitionRepository() *DefinitionRepository {
	return &DefinitionRepository{
		base:   newBaseMemoryRepo("definition", func(d *domain.NotificationDefinition) *domain.RecordMeta { return &d.RecordMeta }),
		byCode: make(map[string]uuid.UUID),
	}
}

func (r *DefinitionRepository) Create(ctx context.Context, record *domain.NotificationDefinition) error {
	if record == nil {
		return store.ErrNotFound
	}
	if record.Code == "" {
		return errors.New("definition code is required")
	}
	codeKey := strings.ToLower(record.Code)
	if _, exists := r.byCode[codeKey]; exists {
		return fmt.Errorf("definition %s already exists", record.Code)
	}
	if err := r.base.create(ctx, record); err != nil {
		return err
	}
	r.byCode[codeKey] = record.ID
	return nil
}

func (r *DefinitionRepository) Update(ctx context.Context, record *domain.NotificationDefinition) error {
	return r.base.update(ctx, record)
}

func (r *DefinitionRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.NotificationDefinition, error) {
	return r.base.getByID(ctx, id, false)
}

func (r *DefinitionRepository) GetByCode(ctx context.Context, code string) (*domain.NotificationDefinition, error) {
	id, ok := r.byCode[strings.ToLower(code)]
	if !ok {
		return nil, store.ErrNotFound
	}
	return r.base.getByID(ctx, id, false)
}

func (r *DefinitionRepository) List(ctx context.Context, opts store.ListOptions) (store.ListResult[domain.NotificationDefinition], error) {
	return r.base.list(ctx, opts)
}

func (r *DefinitionRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.base.softDelete(ctx, id)
}
