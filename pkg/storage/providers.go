package storage

import (
	"context"
	"database/sql"

	bunrepo "github.com/goliatone/go-notifications/internal/storage/bun"
	"github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	persistence "github.com/goliatone/go-persistence-bun"
	"github.com/uptrace/bun"
)

// MetricsCollector enables downstream observers to record repo timings.
type MetricsCollector interface {
	Record(operation string, labels map[string]string)
}

// Providers exposes all repositories needed by services.
type Providers struct {
	Definitions        store.NotificationDefinitionRepository
	Templates          store.NotificationTemplateRepository
	Events             store.NotificationEventRepository
	Messages           store.NotificationMessageRepository
	DeliveryAttempts   store.DeliveryAttemptRepository
	Preferences        store.NotificationPreferenceRepository
	SubscriptionGroups store.SubscriptionGroupRepository
	Inbox              store.InboxRepository
	Transaction        store.TransactionManager
	Metrics            MetricsCollector
}

type Option func(*Providers)

// WithMetricsCollector registers a metrics collector returned alongside repos.
func WithMetricsCollector(collector MetricsCollector) Option {
	return func(p *Providers) {
		p.Metrics = collector
	}
}

// NewMemoryProviders returns repositories backed by in-memory maps.
func NewMemoryProviders(opts ...Option) Providers {
	providers := Providers{
		Definitions:        memory.NewDefinitionRepository(),
		Templates:          memory.NewTemplateRepository(),
		Events:             memory.NewEventRepository(),
		Messages:           memory.NewMessageRepository(),
		DeliveryAttempts:   memory.NewDeliveryRepository(),
		Preferences:        memory.NewPreferenceRepository(),
		SubscriptionGroups: memory.NewSubscriptionRepository(),
		Inbox:              memory.NewInboxRepository(),
		Transaction:        &store.NopTransactionManager{},
	}
	for _, opt := range opts {
		opt(&providers)
	}
	return providers
}

// NewBunProviders wires Bun-backed repositories using go-repository-bun.
// The caller is responsible for creating the *bun.DB instance (potentially
// via go-persistence-bun) and managing its lifecycle.
func NewBunProviders(db *bun.DB, opts ...Option) Providers {
	if db == nil {
		panic("storage: bun DB is required")
	}

	// Register models so go-persistence-bun migrations can pick them up.
	persistence.RegisterModel(
		(*domain.NotificationDefinition)(nil),
		(*domain.NotificationTemplate)(nil),
		(*domain.NotificationEvent)(nil),
		(*domain.NotificationMessage)(nil),
		(*domain.DeliveryAttempt)(nil),
		(*domain.NotificationPreference)(nil),
		(*domain.SubscriptionGroup)(nil),
		(*domain.InboxItem)(nil),
	)

	txManager := &bunTxManager{db: db}

	providers := Providers{
		Definitions:        bunrepo.NewDefinitionRepository(db),
		Templates:          bunrepo.NewTemplateRepository(db),
		Events:             bunrepo.NewEventRepository(db),
		Messages:           bunrepo.NewMessageRepository(db),
		DeliveryAttempts:   bunrepo.NewDeliveryRepository(db),
		Preferences:        bunrepo.NewPreferenceRepository(db),
		SubscriptionGroups: bunrepo.NewSubscriptionRepository(db),
		Inbox:              bunrepo.NewInboxRepository(db),
		Transaction:        txManager,
	}

	for _, opt := range opts {
		opt(&providers)
	}
	return providers
}

type bunTxManager struct {
	db *bun.DB
}

func (m *bunTxManager) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	return m.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		return fn(ctx)
	})
}
