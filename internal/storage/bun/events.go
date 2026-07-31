package bunrepo

import (
	"context"
	"time"

	"github.com/goliatone/go-notifications/pkg/domain"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type EventRepository struct {
	crudRepository[domain.NotificationEvent]
}

func NewEventRepository(db *bun.DB) *EventRepository {
	return &EventRepository{
		crudRepository: newEntityCRUD(db, "id",
			func(event *domain.NotificationEvent) *domain.RecordMeta { return &event.RecordMeta }, nil),
	}
}

func (r *EventRepository) Create(ctx context.Context, e *domain.NotificationEvent) error {
	if e.Status == "" {
		e.Status = domain.EventStatusPending
	}
	_, _, err := r.CreateIdempotent(ctx, e)
	return err
}

func (r *EventRepository) CreateIdempotent(ctx context.Context, event *domain.NotificationEvent) (*domain.NotificationEvent, bool, error) {
	if event.Status == "" {
		event.Status = domain.EventStatusPending
	}
	if event.IdempotencyKey == "" {
		if err := r.base.create(ctx, event); err != nil {
			return nil, false, err
		}
		return event, true, nil
	}
	createErr := r.base.create(ctx, event)
	if createErr == nil {
		return event, true, nil
	}
	stored, lookupErr := r.GetByIdempotency(ctx, event.IdempotencyScope, event.DefinitionCode, event.IdempotencyKey)
	if lookupErr != nil {
		return nil, false, createErr
	}
	return stored, false, nil
}

func (r *EventRepository) ClaimRetry(ctx context.Context, id uuid.UUID, until time.Time) (bool, error) {
	now := time.Now().UTC()
	result, err := r.base.db.NewUpdate().
		Model((*domain.NotificationEvent)(nil)).
		Set("status = ?", domain.EventStatusRetrying).
		Set("retry_claim_until = ?", until).
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Where("(status IN (?, ?) OR (status = ? AND retry_claim_until <= ?))",
			domain.EventStatusFailed, domain.EventStatusPartial, domain.EventStatusRetrying, now).
		Exec(ctx)
	if err != nil {
		return false, mapError(err)
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func (r *EventRepository) GetByIdempotency(ctx context.Context, scope, definitionCode, key string) (*domain.NotificationEvent, error) {
	event := &domain.NotificationEvent{}
	err := r.base.db.NewSelect().
		Model(event).
		WhereAllWithDeleted().
		Where("idempotency_scope = ?", scope).
		Where("definition_code = ?", definitionCode).
		Where("idempotency_key = ?", key).
		Limit(1).
		Scan(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	return event, nil
}

func (r *EventRepository) ListPending(ctx context.Context, limit int) ([]domain.NotificationEvent, error) {
	criteria := []repository.SelectCriteria{
		func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Where("status IN (?, ?)", domain.EventStatusPending, domain.EventStatusScheduled).
				Order("created_at ASC")
		},
	}
	if limit > 0 {
		criteria = append(criteria, func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.Limit(limit)
		})
	}
	records, _, err := r.base.repo.List(ctx, criteria...)
	if err != nil {
		return nil, mapError(err)
	}
	items := make([]domain.NotificationEvent, len(records))
	for i, rec := range records {
		items[i] = *rec
	}
	return items, nil
}

func (r *EventRepository) ListByPublication(ctx context.Context, publicationID uuid.UUID) ([]domain.NotificationEvent, error) {
	records, _, err := r.base.repo.List(ctx, func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("publication_id = ?", publicationID).Order("created_at ASC")
	})
	if err != nil {
		return nil, mapError(err)
	}
	items := make([]domain.NotificationEvent, len(records))
	for i, record := range records {
		items[i] = *record
	}
	return items, nil
}

func (r *EventRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	event, err := r.base.getByID(ctx, id, true)
	if err != nil {
		return err
	}
	event.Status = status
	return r.base.update(ctx, event)
}
