package notifier

import (
	"context"
	"errors"

	i18n "github.com/goliatone/go-i18n"
	"github.com/goliatone/go-notifications/internal/di"
	"github.com/goliatone/go-notifications/pkg/activity"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/commands"
	"github.com/goliatone/go-notifications/pkg/config"
	"github.com/goliatone/go-notifications/pkg/definitions"
	"github.com/goliatone/go-notifications/pkg/events"
	"github.com/goliatone/go-notifications/pkg/inbox"
	"github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"
	"github.com/goliatone/go-notifications/pkg/interfaces/cache"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/queue"
	"github.com/goliatone/go-notifications/pkg/links"
	"github.com/goliatone/go-notifications/pkg/persistencepolicy"
	"github.com/goliatone/go-notifications/pkg/preferences"
	"github.com/goliatone/go-notifications/pkg/privacy"
	"github.com/goliatone/go-notifications/pkg/retry"
	"github.com/goliatone/go-notifications/pkg/secrets"
	"github.com/goliatone/go-notifications/pkg/storage"
	"github.com/goliatone/go-notifications/pkg/templates"
)

// ModuleOptions configure the notifier module facade.
type ModuleOptions struct {
	Config            config.Config
	Storage           storage.Providers
	Logger            logger.Logger
	Cache             cache.Cache
	Translator        i18n.Translator
	Fallbacks         i18n.FallbackResolver
	Queue             queue.Queue
	Broadcaster       broadcaster.Broadcaster
	Adapters          []adapters.Messenger
	LinkBuilder       links.LinkBuilder
	LinkStore         links.LinkStore
	LinkObserver      links.LinkObserver
	LinkPolicy        links.FailurePolicy
	Secrets           secrets.Resolver
	Backoff           retry.Backoff
	Activity          activity.Hooks
	Persistence       persistencepolicy.Policy
	Privacy           privacy.Policy
	Diagnostic        privacy.DiagnosticSink
	RecipientResolver adapters.RecipientResolver
}

// Module bundles the container and exposes high-level accessors.
type Module struct {
	container *di.Container
	manager   *Manager
}

// NewModule assembles repositories, services, dispatcher, manager, and commands.
func NewModule(opts ModuleOptions) (*Module, error) {
	container, err := di.New(di.Options{
		Config:            opts.Config,
		Storage:           opts.Storage,
		Logger:            opts.Logger,
		Cache:             opts.Cache,
		Translator:        opts.Translator,
		Fallbacks:         opts.Fallbacks,
		Queue:             opts.Queue,
		Broadcaster:       opts.Broadcaster,
		Adapters:          opts.Adapters,
		LinkBuilder:       opts.LinkBuilder,
		LinkStore:         opts.LinkStore,
		LinkObserver:      opts.LinkObserver,
		LinkPolicy:        opts.LinkPolicy,
		Secrets:           opts.Secrets,
		Backoff:           opts.Backoff,
		Activity:          opts.Activity,
		Persistence:       opts.Persistence,
		Privacy:           opts.Privacy,
		Diagnostic:        opts.Diagnostic,
		RecipientResolver: opts.RecipientResolver,
	})
	if err != nil {
		return nil, err
	}
	manager, err := NewWithDispatcher(Dependencies{
		EventService: container.Events,
		Logger:       opts.Logger,
	}, container.Dispatcher)
	if err != nil {
		return nil, err
	}
	return &Module{container: container, manager: manager}, nil
}

// Manager returns the notifier manager.
func (m *Module) Manager() *Manager {
	if m == nil || m.container == nil {
		return nil
	}
	return m.manager
}

// Templates returns the template service.
func (m *Module) Templates() *templates.Service {
	if m == nil || m.container == nil {
		return nil
	}
	return m.container.Templates
}

// Definitions returns the definition mutation service.
func (m *Module) Definitions() *definitions.Service {
	if m == nil || m.container == nil {
		return nil
	}
	return m.container.Definitions
}

// Preferences returns the preferences service.
func (m *Module) Preferences() *preferences.Service {
	if m == nil || m.container == nil {
		return nil
	}
	return m.container.Preferences
}

// Inbox exposes the inbox service.
func (m *Module) Inbox() *inbox.Service {
	if m == nil || m.container == nil {
		return nil
	}
	return m.container.Inbox
}

// Events returns the event intake service.
func (m *Module) Events() *events.Service {
	if m == nil || m.container == nil {
		return nil
	}
	return m.container.Events
}

// RetryWithReceipt delegates to the canonical event retry service.
func (m *Module) RetryWithReceipt(ctx context.Context, req events.RetryRequest) (events.DispatchReceipt, error) {
	if m == nil || m.container == nil || m.container.Events == nil {
		return events.DispatchReceipt{}, errors.New("notifier: module is not initialized")
	}
	return m.container.Events.RetryWithReceipt(ctx, req)
}

// RecoverPending republishes durable asynchronous work through the event service.
func (m *Module) RecoverPending(ctx context.Context, limit int) error {
	if m == nil || m.container == nil || m.container.Events == nil {
		return errors.New("notifier: module is not initialized")
	}
	return m.container.Events.RecoverPending(ctx, limit)
}

// Commands returns the go-command registry.
func (m *Module) Commands() *commands.Registry {
	if m == nil || m.container == nil {
		return nil
	}
	return m.container.Commands
}

// AdapterRegistry exposes the configured messenger registry.
func (m *Module) AdapterRegistry() *adapters.Registry {
	if m == nil || m.container == nil {
		return nil
	}
	return m.container.Adapters
}

func (m *Module) PersistencePolicy() persistencepolicy.Policy {
	if m == nil || m.container == nil {
		return nil
	}
	return m.container.Persistence
}

func (m *Module) PrivacyPolicy() privacy.Policy {
	if m == nil || m.container == nil {
		return nil
	}
	return m.container.Privacy
}

func (m *Module) DiagnosticSink() privacy.DiagnosticSink {
	if m == nil || m.container == nil {
		return nil
	}
	return m.container.Diagnostic
}

func (m *Module) RecipientResolver() adapters.RecipientResolver {
	if m == nil || m.container == nil {
		return nil
	}
	return m.container.RecipientResolver
}

// Config returns the effective module configuration.
func (m *Module) Config() config.Config {
	if m == nil || m.container == nil {
		return config.Config{}
	}
	return m.container.Config
}

// Container returns the internal DI container.
// This is exposed for advanced use cases like direct storage access.
func (m *Module) Container() *di.Container {
	if m == nil {
		return nil
	}
	return m.container
}
