package bunrepo

import (
	"context"
	"strings"
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

type crudRepository[T any] struct {
	base baseRepository[T]
}

type codedRepository[T any] struct {
	crudRepository[T]
}

type initializedRepository[T any] struct {
	crudRepository[T]
	initialize func(*T)
}

func newInitializedRepository[T any](
	db *bun.DB,
	extract func(*T) *domain.RecordMeta,
	initialize func(*T),
) initializedRepository[T] {
	return initializedRepository[T]{
		crudRepository: newEntityCRUD(db, "id", extract, nil),
		initialize:     initialize,
	}
}

func (r initializedRepository[T]) Create(ctx context.Context, record *T) error {
	if r.initialize != nil {
		r.initialize(record)
	}
	return r.base.create(ctx, record)
}

func newCodedRepository[T any](
	db *bun.DB,
	extract func(*T) *domain.RecordMeta,
	code func(*T) string,
) codedRepository[T] {
	return codedRepository[T]{
		crudRepository: newEntityCRUD(db, "code", extract, code),
	}
}

func (r codedRepository[T]) GetByCode(ctx context.Context, code string) (*T, error) {
	record, err := r.base.repo.Get(ctx,
		func(query *bun.SelectQuery) *bun.SelectQuery {
			return query.Where("LOWER(code) = ?", strings.ToLower(code))
		},
		withoutDeleted(),
	)
	if err != nil {
		return nil, mapError(err)
	}
	return record, nil
}

func newCRUDRepository[T any](db *bun.DB, handlers repository.ModelHandlers[*T], extract func(*T) *domain.RecordMeta) crudRepository[T] {
	return crudRepository[T]{base: newBaseRepository(db, handlers, extract)}
}

func newEntityCRUD[T any](
	db *bun.DB,
	identifier string,
	extract func(*T) *domain.RecordMeta,
	identifierValue func(*T) string,
) crudRepository[T] {
	if identifierValue == nil {
		identifierValue = func(record *T) string { return extract(record).ID.String() }
	}
	handlers := repository.ModelHandlers[*T]{
		NewRecord:          func() *T { return new(T) },
		GetID:              func(record *T) uuid.UUID { return extract(record).ID },
		SetID:              func(record *T, id uuid.UUID) { extract(record).ID = id },
		GetIdentifier:      func() string { return identifier },
		GetIdentifierValue: identifierValue,
	}
	return newCRUDRepository(db, handlers, extract)
}

func (r crudRepository[T]) Create(ctx context.Context, record *T) error {
	return r.base.create(ctx, record)
}

func (r crudRepository[T]) Update(ctx context.Context, record *T) error {
	return r.base.update(ctx, record)
}

func (r crudRepository[T]) GetByID(ctx context.Context, id uuid.UUID) (*T, error) {
	return r.base.getByID(ctx, id, false)
}

func (r crudRepository[T]) List(ctx context.Context, opts store.ListOptions) (store.ListResult[T], error) {
	return r.base.list(ctx, opts)
}

func (r crudRepository[T]) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.base.softDelete(ctx, id)
}

func (r crudRepository[T]) listByUUID(ctx context.Context, column string, id uuid.UUID) ([]T, error) {
	records, _, err := r.base.repo.List(ctx, func(query *bun.SelectQuery) *bun.SelectQuery {
		return query.Where(column+" = ?", id)
	})
	if err != nil {
		return nil, mapError(err)
	}
	items := make([]T, len(records))
	for index, record := range records {
		items[index] = *record
	}
	return items, nil
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
	result, err := r.db.NewUpdate().Model(record).WherePK().Exec(ctx)
	if err != nil {
		return mapError(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return mapError(err)
	}
	if affected != 1 {
		return store.ErrNotFound
	}
	return nil
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
	return r.update(ctx, record)
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
