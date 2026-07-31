package notifier

import (
	"context"
	"errors"
	"time"

	"github.com/goliatone/go-notifications/internal/dispatcher"
	"github.com/goliatone/go-notifications/pkg/activity"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/config"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/events"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/queue"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/links"
	"github.com/goliatone/go-notifications/pkg/persistencepolicy"
	prefsvc "github.com/goliatone/go-notifications/pkg/preferences"
	"github.com/goliatone/go-notifications/pkg/privacy"
	"github.com/goliatone/go-notifications/pkg/receipts"
	"github.com/goliatone/go-notifications/pkg/retry"
	"github.com/goliatone/go-notifications/pkg/secrets"
	"github.com/goliatone/go-notifications/pkg/templates"
)

type Event struct {
	DefinitionCode   string
	Recipients       []string
	Context          map[string]any
	Transient        map[string]any `json:"-"`
	Channels         []string
	TenantID         string
	ActorID          string
	Locale           string
	ScheduledAt      time.Time
	CorrelationID    string
	RequestID        string
	IdempotencyKey   string
	IdempotencyScope string
}

type Manager struct {
	eventService *events.Service
	logger       logger.Logger
}

type inboxDeliverer interface {
	DeliverFromMessage(context.Context, *domain.NotificationMessage) error
}

type Dependencies struct {
	Definitions     store.NotificationDefinitionRepository
	Events          store.NotificationEventRepository
	Messages        store.NotificationMessageRepository
	Attempts        store.DeliveryAttemptRepository
	Publications    store.NotificationPublicationRepository
	RetryOperations store.NotificationRetryOperationRepository
	EventService    *events.Service
	Queue           queue.Queue
	Templates       *templates.Service
	Adapters        *adapters.Registry
	Attachments     adapters.AttachmentResolver
	LinkBuilder     links.LinkBuilder
	LinkStore       links.LinkStore
	LinkObserver    links.LinkObserver
	LinkPolicy      links.FailurePolicy
	Logger          logger.Logger
	Config          config.DispatcherConfig
	Preferences     *prefsvc.Service
	Inbox           inboxDeliverer
	Secrets         secrets.Resolver
	Backoff         retry.Backoff
	Activity        activity.Hooks
	Persistence     persistencepolicy.Policy
	Privacy         privacy.Policy
	Diagnostic      privacy.DiagnosticSink
}

var ErrMissingEventsRepository = errors.New("notifier: events repository is required")

func New(deps Dependencies) (*Manager, error) {
	return NewWithDispatcher(deps, nil)
}

func NewWithDispatcher(deps Dependencies, dispatcherSvc *dispatcher.Service) (*Manager, error) {
	if deps.EventService == nil && deps.Events == nil {
		return nil, ErrMissingEventsRepository
	}
	if deps.Logger == nil {
		deps.Logger = logger.Default()
	}
	if deps.EventService != nil {
		return &Manager{eventService: deps.EventService, logger: deps.Logger}, nil
	}
	if dispatcherSvc == nil {
		var err error
		dispatcherSvc, err = dispatcher.New(dispatcher.Dependencies{
			Definitions: deps.Definitions, Events: deps.Events, Messages: deps.Messages,
			Attempts: deps.Attempts, Templates: deps.Templates, Registry: deps.Adapters,
			Attachments: deps.Attachments, LinkBuilder: deps.LinkBuilder, LinkStore: deps.LinkStore,
			LinkObserver: deps.LinkObserver, LinkPolicy: deps.LinkPolicy, Logger: deps.Logger,
			Config: deps.Config, Preferences: deps.Preferences, Inbox: deps.Inbox,
			Secrets: deps.Secrets, Backoff: deps.Backoff, Activity: deps.Activity,
			Persistence: deps.Persistence, Privacy: deps.Privacy, Diagnostic: deps.Diagnostic,
		})
		if err != nil {
			return nil, err
		}
	}
	eventService, err := events.New(events.Dependencies{
		Definitions: deps.Definitions, Events: deps.Events, Messages: deps.Messages,
		Attempts: deps.Attempts, Publications: deps.Publications, RetryOperations: deps.RetryOperations,
		Dispatcher: dispatcherSvc, Queue: deps.Queue, Logger: deps.Logger, Activity: deps.Activity,
		Privacy: deps.Privacy, Diagnostic: deps.Diagnostic,
	})
	if err != nil {
		return nil, err
	}
	return &Manager{eventService: eventService, logger: deps.Logger}, nil
}

func (m *Manager) Send(ctx context.Context, evt Event) error {
	_, err := m.SendWithReceipt(ctx, evt)
	return err
}

func (m *Manager) SendWithReceipt(ctx context.Context, evt Event) (receipts.DispatchReceipt, error) {
	if m == nil || m.eventService == nil {
		return receipts.DispatchReceipt{}, errors.New("notifier: manager is not initialized")
	}
	if len(evt.Transient) > 0 {
		if !evt.ScheduledAt.IsZero() && evt.ScheduledAt.After(time.Now()) {
			return receipts.DispatchReceipt{}, privacy.SafeError{
				Category: "validation", Code: "transient_async_forbidden",
				Message: "transient data is only supported for immediate delivery",
			}
		}
		return m.eventService.DispatchImmediate(ctx, events.ImmediateRequest{
			DefinitionCode: evt.DefinitionCode, Recipients: evt.Recipients, Context: evt.Context,
			Transient: evt.Transient, Channels: evt.Channels, TenantID: evt.TenantID,
			ActorID: evt.ActorID, Locale: evt.Locale, CorrelationID: evt.CorrelationID,
			RequestID: evt.RequestID, IdempotencyKey: evt.IdempotencyKey,
			IdempotencyScope: evt.IdempotencyScope,
		})
	}
	return m.eventService.EnqueueWithReceipt(ctx, events.IntakeRequest{
		DefinitionCode: evt.DefinitionCode, Recipients: evt.Recipients, Context: evt.Context,
		Channels: evt.Channels, TenantID: evt.TenantID, ActorID: evt.ActorID, Locale: evt.Locale,
		ScheduleAt: evt.ScheduledAt, CorrelationID: evt.CorrelationID, RequestID: evt.RequestID,
		IdempotencyKey: evt.IdempotencyKey, IdempotencyScope: evt.IdempotencyScope,
	})
}

func (m *Manager) RetryWithReceipt(ctx context.Context, req events.RetryRequest) (receipts.DispatchReceipt, error) {
	if m == nil || m.eventService == nil {
		return receipts.DispatchReceipt{}, errors.New("notifier: manager is not initialized")
	}
	return m.eventService.RetryWithReceipt(ctx, req)
}

func (m *Manager) RecoverPending(ctx context.Context, limit int) error {
	if m == nil || m.eventService == nil {
		return errors.New("notifier: manager is not initialized")
	}
	return m.eventService.RecoverPending(ctx, limit)
}
