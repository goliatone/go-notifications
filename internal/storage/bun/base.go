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

type baseRepository[T any] struct {
	repo    repository.Repository[*T]
	db      *bun.DB
	extract func(*T) *domain.RecordMeta
}

func newBaseRepository[T any](db *bun.DB, handlers repository.ModelHandlers[*T], extract func(*T) *domain.RecordMeta) baseRepository[T] {
	return baseRepository[T]{
		repo:    repository.MustNewRepository[*T](db, handlers),
		db:      db,
		extract: extract,
	}
}

func (r baseRepository[T]) create(ctx context.Context, record *T) error {
	base := r.extract(record)
	base.EnsureID()
	now := time.Now().UTC()
	if base.CreatedAt.IsZero() {
		base.CreatedAt = now
	}
	base.UpdatedAt = now
	_, err := r.repo.Create(ctx, record)
	return mapError(err)
}

func (r baseRepository[T]) update(ctx context.Context, record *T) error {
	base := r.extract(record)
	base.UpdatedAt = time.Now().UTC()
	_, err := r.repo.Update(ctx, record)
	return mapError(err)
}

func (r baseRepository[T]) getByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*T, error) {
	criteria := []repository.SelectCriteria{withID(id)}
	if !includeDeleted {
		criteria = append(criteria, withoutDeleted())
	}
	record, err := r.repo.Get(ctx, criteria...)
	if err != nil {
		return nil, mapError(err)
	}
	return record, nil
}

func (r baseRepository[T]) list(ctx context.Context, opts store.ListOptions) (store.ListResult[T], error) {
	criteria := []repository.SelectCriteria{withListOptions(opts)}
	records, total, err := r.repo.List(ctx, criteria...)
	if err != nil {
		return store.ListResult[T]{}, mapError(err)
	}
	items := make([]T, len(records))
	for i, rec := range records {
		items[i] = *rec
	}
	return store.ListResult[T]{Items: items, Total: total}, nil
}

func (r baseRepository[T]) softDelete(ctx context.Context, id uuid.UUID) error {
	record, err := r.getByID(ctx, id, true)
	if err != nil {
		return err
	}
	base := r.extract(record)
	base.DeletedAt = time.Now().UTC()
	_, err = r.repo.Update(ctx, record)
	return mapError(err)
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	if repository.IsRecordNotFound(err) {
		return store.ErrNotFound
	}
	return err
}
