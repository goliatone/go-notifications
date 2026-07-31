package memory

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
)

type EventRepository struct {
	base     baseMemoryRepo[domain.NotificationEvent]
	identity map[string]uuid.UUID
}

func NewEventRepository() *EventRepository {
	return &EventRepository{
		base:     newBaseMemoryRepo("event", func(e *domain.NotificationEvent) *domain.RecordMeta { return &e.RecordMeta }),
		identity: make(map[string]uuid.UUID),
	}
}

func (r *EventRepository) Create(ctx context.Context, event *domain.NotificationEvent) error {
	if event.Status == "" {
		event.Status = domain.EventStatusPending
	}
	_, _, err := r.CreateIdempotent(ctx, event)
	return err
}

func (r *EventRepository) CreateIdempotent(_ context.Context, event *domain.NotificationEvent) (*domain.NotificationEvent, bool, error) {
	r.base.mu.Lock()
	defer r.base.mu.Unlock()

	if event.Status == "" {
		event.Status = domain.EventStatusPending
	}
	key := eventIdentity(event.IdempotencyScope, event.DefinitionCode, event.IdempotencyKey)
	if event.IdempotencyKey != "" {
		if id, ok := r.identity[key]; ok {
			stored := r.base.records[id]
			copy := stored
			return &copy, false, nil
		}
	}
	event.EnsureID()
	now := time.Now().UTC()
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	}
	event.UpdatedAt = now
	r.base.records[event.ID] = *event
	if event.IdempotencyKey != "" {
		r.identity[key] = event.ID
	}
	copy := *event
	return &copy, true, nil
}

func (r *EventRepository) GetByIdempotency(_ context.Context, scope, definitionCode, key string) (*domain.NotificationEvent, error) {
	r.base.mu.RLock()
	defer r.base.mu.RUnlock()
	id, ok := r.identity[eventIdentity(scope, definitionCode, key)]
	if !ok {
		return nil, store.ErrNotFound
	}
	event := r.base.records[id]
	return &event, nil
}

func (r *EventRepository) Update(ctx context.Context, event *domain.NotificationEvent) error {
	return r.base.update(ctx, event)
}

func (r *EventRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.NotificationEvent, error) {
	return r.base.getByID(ctx, id, false)
}

func (r *EventRepository) List(ctx context.Context, opts store.ListOptions) (store.ListResult[domain.NotificationEvent], error) {
	return r.base.list(ctx, opts)
}

func (r *EventRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.base.softDelete(ctx, id)
}

func (r *EventRepository) ListPending(ctx context.Context, limit int) ([]domain.NotificationEvent, error) {
	result, err := r.base.list(ctx, store.ListOptions{})
	if err != nil {
		return nil, err
	}
	pending := make([]domain.NotificationEvent, 0, len(result.Items))
	for _, event := range result.Items {
		if event.Status == domain.EventStatusPending || event.Status == domain.EventStatusScheduled {
			pending = append(pending, event)
		}
	}
	if limit > 0 && len(pending) > limit {
		pending = slices.Clone(pending[:limit])
	}
	return pending, nil
}

func (r *EventRepository) ListByPublication(ctx context.Context, publicationID uuid.UUID) ([]domain.NotificationEvent, error) {
	result, err := r.base.list(ctx, store.ListOptions{})
	if err != nil {
		return nil, err
	}
	items := make([]domain.NotificationEvent, 0)
	for _, event := range result.Items {
		if event.PublicationID == publicationID {
			items = append(items, event)
		}
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

func (r *EventRepository) ClaimRetry(_ context.Context, id uuid.UUID, until time.Time) (bool, error) {
	r.base.mu.Lock()
	defer r.base.mu.Unlock()
	event, ok := r.base.records[id]
	if !ok {
		return false, store.ErrNotFound
	}
	now := time.Now().UTC()
	if event.Status == domain.EventStatusRetrying && event.RetryClaimUntil.After(now) {
		return false, nil
	}
	if event.Status != domain.EventStatusFailed && event.Status != domain.EventStatusPartial &&
		(event.Status != domain.EventStatusRetrying || event.RetryClaimUntil.After(now)) {
		return false, nil
	}
	event.Status = domain.EventStatusRetrying
	event.RetryClaimUntil = until
	event.UpdatedAt = now
	r.base.records[id] = event
	return true, nil
}

// attachPublication updates the digest membership while the caller holds the
// publication repository lock. Keeping this operation private preserves one
// lock order (publication, then event) for digest membership and claims.
func (r *EventRepository) attachPublication(event *domain.NotificationEvent, publicationID uuid.UUID, digestKey string) error {
	r.base.mu.Lock()
	defer r.base.mu.Unlock()
	stored, ok := r.base.records[event.ID]
	if !ok {
		return store.ErrNotFound
	}
	stored.PublicationID = publicationID
	stored.DigestKey = digestKey
	stored.Status = domain.EventStatusScheduled
	stored.UpdatedAt = time.Now().UTC()
	r.base.records[event.ID] = stored
	*event = stored
	return nil
}

func eventIdentity(scope, definitionCode, key string) string {
	return fmt.Sprintf("%s\x00%s\x00%s", scope, definitionCode, key)
}
