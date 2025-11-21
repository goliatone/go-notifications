package di

import (
	"errors"
	"reflect"

	i18n "github.com/goliatone/go-i18n"
	"github.com/goliatone/go-notifications/internal/dispatcher"
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

// Options configure the DI container.
type Options struct {
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

// Container wires repositories, services, dispatcher, commands, and manager.
type Container struct {
	Config      config.Config
	Storage     storage.Providers
	Templates   *templates.Service
	Preferences *preferences.Service
	Inbox       *inbox.Service
	Events      *events.Service
	Dispatcher  *dispatcher.Service
	Commands    *commands.Registry
	Adapters    *adapters.Registry
	Secrets     secrets.Resolver
}

func isZeroConfig(cfg config.Config) bool {
	return reflect.ValueOf(cfg).IsZero()
}

// New constructs the container using the supplied options.
func New(opts Options) (*Container, error) {
	if opts.Translator == nil {
		return nil, errors.New("di: translator is required")
	}

	cfg := opts.Config
	if isZeroConfig(cfg) {
		cfg = config.Defaults()
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	providers := opts.Storage
	if providers.Definitions == nil {
		providers = storage.NewMemoryProviders()
	}

	lgr := opts.Logger
	if lgr == nil {
		lgr = &logger.Nop{}
	}

	c := opts.Cache
	if c == nil {
		c = &cache.Nop{}
	}

	q := opts.Queue
	if q == nil {
		q = &queue.Nop{}
	}

	b := opts.Broadcaster
	if b == nil {
		b = &broadcaster.Nop{}
	}

	secretsResolver := opts.Secrets
	if secretsResolver == nil {
		secretsResolver = secrets.SimpleResolver{Provider: secrets.NopProvider{}}
	}

	adapterRegistry := adapters.NewRegistry(opts.Adapters...)

	tplSvc, err := templates.New(templates.Dependencies{
		Repository:    providers.Templates,
		Cache:         c,
		Logger:        lgr,
		Translator:    opts.Translator,
		Fallbacks:     opts.Fallbacks,
		DefaultLocale: cfg.Localization.DefaultLocale,
	})
	if err != nil {
		return nil, err
	}

	prefSvc, err := preferences.New(preferences.Dependencies{
		Repository: providers.Preferences,
		Logger:     lgr,
	})
	if err != nil {
		return nil, err
	}

	inboxSvc, err := inbox.New(inbox.Dependencies{
		Repository:  providers.Inbox,
		Broadcaster: b,
		Logger:      lgr,
	})
	if err != nil {
		return nil, err
	}

	dispatcherSvc, err := dispatcher.New(dispatcher.Dependencies{
		Definitions: providers.Definitions,
		Events:      providers.Events,
		Messages:    providers.Messages,
		Attempts:    providers.DeliveryAttempts,
		Templates:   tplSvc,
		Registry:    adapterRegistry,
		Logger:      lgr,
		Config:      cfg.Dispatcher,
		Preferences: prefSvc,
		Inbox:       inboxSvc,
		Secrets:     secretsResolver,
	})
	if err != nil {
		return nil, err
	}

	eventSvc, err := events.New(events.Dependencies{
		Definitions: providers.Definitions,
		Events:      providers.Events,
		Dispatcher:  dispatcherSvc,
		Queue:       q,
		Logger:      lgr,
	})
	if err != nil {
		return nil, err
	}

	cmdRegistry, err := commands.New(commands.Dependencies{
		Definitions: providers.Definitions,
		Templates:   tplSvc,
		Preferences: prefSvc,
		Inbox:       inboxSvc,
		Events:      eventSvc,
		Logger:      lgr,
	})
	if err != nil {
		return nil, err
	}

	return &Container{
		Config:      cfg,
		Storage:     providers,
		Templates:   tplSvc,
		Preferences: prefSvc,
		Inbox:       inboxSvc,
		Events:      eventSvc,
		Dispatcher:  dispatcherSvc,
		Commands:    cmdRegistry,
		Adapters:    adapterRegistry,
		Secrets:     secretsResolver,
	}, nil
}
