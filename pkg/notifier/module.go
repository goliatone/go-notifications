package notifier

import (
	i18n "github.com/goliatone/go-i18n"
	"github.com/goliatone/go-notifications/internal/di"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/commands"
	"github.com/goliatone/go-notifications/pkg/config"
	"github.com/goliatone/go-notifications/pkg/events"
	"github.com/goliatone/go-notifications/pkg/inbox"
	"github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"
	"github.com/goliatone/go-notifications/pkg/interfaces/cache"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/queue"
	"github.com/goliatone/go-notifications/pkg/preferences"
	"github.com/goliatone/go-notifications/pkg/secrets"
	"github.com/goliatone/go-notifications/pkg/storage"
	"github.com/goliatone/go-notifications/pkg/templates"
)

// ModuleOptions configure the notifier module facade.
type ModuleOptions struct {
	Config      config.Config
	Storage     storage.Providers
	Logger      logger.Logger
	Cache       cache.Cache
	Translator  i18n.Translator
	Fallbacks   i18n.FallbackResolver
	Queue       queue.Queue
	Broadcaster broadcaster.Broadcaster
	Adapters    []adapters.Messenger
	Secrets     secrets.Resolver
}

// Module bundles the container and exposes high-level accessors.
type Module struct {
	container *di.Container
	manager   *Manager
}

// NewModule assembles repositories, services, dispatcher, manager, and commands.
func NewModule(opts ModuleOptions) (*Module, error) {
	container, err := di.New(di.Options{
		Config:      opts.Config,
		Storage:     opts.Storage,
		Logger:      opts.Logger,
		Cache:       opts.Cache,
		Translator:  opts.Translator,
		Fallbacks:   opts.Fallbacks,
		Queue:       opts.Queue,
		Broadcaster: opts.Broadcaster,
		Adapters:    opts.Adapters,
		Secrets:     opts.Secrets,
	})
	if err != nil {
		return nil, err
	}
	manager, err := NewWithDispatcher(Dependencies{
		Definitions: container.Storage.Definitions,
		Events:      container.Storage.Events,
		Messages:    container.Storage.Messages,
		Attempts:    container.Storage.DeliveryAttempts,
		Templates:   container.Templates,
		Adapters:    container.Adapters,
		Logger:      opts.Logger,
		Config:      container.Config.Dispatcher,
		Preferences: container.Preferences,
		Inbox:       container.Inbox,
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
