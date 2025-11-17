package notifier

import (
	"context"
	"errors"
	"time"

	"github.com/goliatone/go-notifications/internal/dispatcher"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/config"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	prefsvc "github.com/goliatone/go-notifications/pkg/preferences"
	"github.com/goliatone/go-notifications/pkg/templates"
)

// Event encapsulates host-provided notification payloads.
type Event struct {
	DefinitionCode string
	Recipients     []string
	Context        map[string]any
	Channels       []string
	TenantID       string
	ActorID        string
	Locale         string
	ScheduledAt    time.Time
}

// Manager orchestrates event persistence + dispatcher invocation.
type Manager struct {
	dispatcher *dispatcher.Service
	events     store.NotificationEventRepository
	logger     logger.Logger
}

// Dependencies bundles repositories/adapters required by the manager.
type Dependencies struct {
	Definitions store.NotificationDefinitionRepository
	Events      store.NotificationEventRepository
	Messages    store.NotificationMessageRepository
	Attempts    store.DeliveryAttemptRepository
	Templates   *templates.Service
	Adapters    *adapters.Registry
	Logger      logger.Logger
	Config      config.DispatcherConfig
	Preferences *prefsvc.Service
}

var (
	ErrMissingEventsRepository = errors.New("notifier: events repository is required")
)

// New constructs the notifier manager along with the dispatcher service.
func New(deps Dependencies) (*Manager, error) {
	if deps.Events == nil {
		return nil, ErrMissingEventsRepository
	}
	if deps.Logger == nil {
		deps.Logger = &logger.Nop{}
	}

	dispatcherSvc, err := dispatcher.New(dispatcher.Dependencies{
		Definitions: deps.Definitions,
		Events:      deps.Events,
		Messages:    deps.Messages,
		Attempts:    deps.Attempts,
		Templates:   deps.Templates,
		Registry:    deps.Adapters,
		Logger:      deps.Logger,
		Config:      deps.Config,
		Preferences: deps.Preferences,
	})
	if err != nil {
		return nil, err
	}

	return &Manager{
		dispatcher: dispatcherSvc,
		events:     deps.Events,
		logger:     deps.Logger,
	}, nil
}

// Send persists a notification event and triggers dispatch immediately.
func (m *Manager) Send(ctx context.Context, evt Event) error {
	if err := validateEvent(evt); err != nil {
		return err
	}
	ctxData := evt.Context
	if ctxData == nil {
		ctxData = make(map[string]any)
	}
	record := &domain.NotificationEvent{
		DefinitionCode: evt.DefinitionCode,
		TenantID:       evt.TenantID,
		ActorID:        evt.ActorID,
		Recipients:     domain.StringList(evt.Recipients),
		Context:        domain.JSONMap(ctxData),
		Status:         domain.EventStatusPending,
	}
	if !evt.ScheduledAt.IsZero() {
		record.ScheduledAt = evt.ScheduledAt
	} else {
		record.ScheduledAt = time.Now()
	}
	if err := m.events.Create(ctx, record); err != nil {
		return err
	}
	if err := m.dispatcher.Dispatch(ctx, record, dispatcher.DispatchOptions{
		Channels: evt.Channels,
		Locale:   evt.Locale,
	}); err != nil {
		_ = m.events.UpdateStatus(ctx, record.ID, domain.EventStatusFailed)
		return err
	}
	return nil
}

func validateEvent(evt Event) error {
	if evt.DefinitionCode == "" {
		return errors.New("notifier: definition code is required")
	}
	if len(evt.Recipients) == 0 {
		return errors.New("notifier: at least one recipient is required")
	}
	return nil
}
