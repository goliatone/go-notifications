package store

import (
	"context"
	"errors"
	"time"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/google/uuid"
)

// ErrNotFound is returned when a record cannot be located.
var ErrNotFound = errors.New("store: not found")

// ListOptions capture pagination and filtering knobs common to repositories.
type ListOptions struct {
	Limit              int
	Offset             int
	Since              time.Time
	Until              time.Time
	IncludeSoftDeleted bool
}

// ListResult bundles records and totals.
type ListResult[T any] struct {
	Items []T
	Total int
}

// Repository defines base CRUD helpers reused by entity-specific interfaces.
type Repository[T any] interface {
	Create(ctx context.Context, record *T) error
	Update(ctx context.Context, record *T) error
	GetByID(ctx context.Context, id uuid.UUID) (*T, error)
	List(ctx context.Context, opts ListOptions) (ListResult[T], error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
}

type NotificationDefinitionRepository interface {
	Repository[domain.NotificationDefinition]
	GetByCode(ctx context.Context, code string) (*domain.NotificationDefinition, error)
}

type NotificationTemplateRepository interface {
	Repository[domain.NotificationTemplate]
	GetByCodeAndLocale(ctx context.Context, code, locale, channel string) (*domain.NotificationTemplate, error)
	ListByCode(ctx context.Context, code string, opts ListOptions) (ListResult[domain.NotificationTemplate], error)
}

type NotificationEventRepository interface {
	Repository[domain.NotificationEvent]
	ListPending(ctx context.Context, limit int) ([]domain.NotificationEvent, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
}

type NotificationMessageRepository interface {
	Repository[domain.NotificationMessage]
	ListByEvent(ctx context.Context, eventID uuid.UUID) ([]domain.NotificationMessage, error)
}

type DeliveryAttemptRepository interface {
	Repository[domain.DeliveryAttempt]
	ListByMessage(ctx context.Context, messageID uuid.UUID) ([]domain.DeliveryAttempt, error)
}

type NotificationPreferenceRepository interface {
	Repository[domain.NotificationPreference]
	GetBySubject(ctx context.Context, subjectType, subjectID string, definitionCode string, channel string) (*domain.NotificationPreference, error)
}

type SubscriptionGroupRepository interface {
	Repository[domain.SubscriptionGroup]
	GetByCode(ctx context.Context, code string) (*domain.SubscriptionGroup, error)
}

type InboxRepository interface {
	Repository[domain.InboxItem]
	ListByUser(ctx context.Context, userID string, opts ListOptions) (ListResult[domain.InboxItem], error)
	MarkRead(ctx context.Context, id uuid.UUID, read bool) error
}
