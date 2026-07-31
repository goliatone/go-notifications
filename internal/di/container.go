package di

import (
	"errors"
	"reflect"

	i18n "github.com/goliatone/go-i18n"
	"github.com/goliatone/go-notifications/internal/dispatcher"
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

// Options configure the DI container.
type Options struct {
	Config            config.Config
	Storage           storage.Providers
	Logger            logger.Logger
	Cache             cache.Cache
	Translator        i18n.Translator
	Fallbacks         i18n.FallbackResolver
	Queue             queue.Queue
	Broadcaster       broadcaster.Broadcaster
	Adapters          []adapters.Messenger
	Attachments       adapters.AttachmentResolver
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

// Container wires repositories, services, dispatcher, commands, and manager.
type Container struct {
	Config            config.Config
	Storage           storage.Providers
	Templates         *templates.Service
	Definitions       *definitions.Service
	Preferences       *preferences.Service
	Inbox             *inbox.Service
	Events            *events.Service
	Dispatcher        *dispatcher.Service
	Commands          *commands.Registry
	Adapters          *adapters.Registry
	Secrets           secrets.Resolver
	Activity          activity.Hooks
	Persistence       persistencepolicy.Policy
	Privacy           privacy.Policy
	Diagnostic        privacy.DiagnosticSink
	RecipientResolver adapters.RecipientResolver
}

func isZeroConfig(cfg config.Config) bool {
	return reflect.ValueOf(cfg).IsZero()
}

// New constructs the container using the supplied options.
func New(opts Options) (*Container, error) {
	if opts.Translator == nil {
		return nil, errors.New("di: translator is required")
	}
	opts = withDefaults(opts)

	hooks := opts.Activity

	cfg := opts.Config
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	providers := opts.Storage
	lgr := opts.Logger
	c := opts.Cache
	q := opts.Queue
	b := opts.Broadcaster
	secretsResolver := opts.Secrets

	adapterRegistry := adapters.NewRegistry(resolveAdapters(opts)...)
	core, err := buildCoreServices(opts, providers, c, b, lgr, hooks)
	if err != nil {
		return nil, err
	}
	delivery, err := buildDeliveryServices(opts, providers, core, adapterRegistry, q, lgr, secretsResolver, hooks, cfg)
	if err != nil {
		return nil, err
	}

	return &Container{
		Config:            cfg,
		Storage:           providers,
		Templates:         core.templates,
		Definitions:       core.definitions,
		Preferences:       core.preferences,
		Inbox:             core.inbox,
		Events:            delivery.events,
		Dispatcher:        delivery.dispatcher,
		Commands:          delivery.commands,
		Adapters:          adapterRegistry,
		Secrets:           secretsResolver,
		Activity:          hooks,
		Persistence:       opts.Persistence,
		Privacy:           opts.Privacy,
		Diagnostic:        opts.Diagnostic,
		RecipientResolver: opts.RecipientResolver,
	}, nil
}

type coreServices struct {
	definitions *definitions.Service
	templates   *templates.Service
	preferences *preferences.Service
	inbox       *inbox.Service
}

func buildCoreServices(
	opts Options,
	providers storage.Providers,
	c cache.Cache,
	b broadcaster.Broadcaster,
	lgr logger.Logger,
	hooks activity.Hooks,
) (coreServices, error) {
	definitionService, err := definitions.New(providers.Definitions)
	if err != nil {
		return coreServices{}, err
	}
	templateService, err := templates.New(templates.Dependencies{
		Repository: providers.Templates, Cache: c, Logger: lgr,
		Translator: opts.Translator, Fallbacks: opts.Fallbacks,
		DefaultLocale: opts.Config.Localization.DefaultLocale,
	})
	if err != nil {
		return coreServices{}, err
	}
	preferenceService, err := preferences.New(preferences.Dependencies{
		Repository: providers.Preferences, Logger: lgr,
	})
	if err != nil {
		return coreServices{}, err
	}
	inboxService, err := inbox.New(inbox.Dependencies{
		Repository: providers.Inbox, Broadcaster: b, Logger: lgr,
		Activity: hooks, Privacy: opts.Privacy, Diagnostic: opts.Diagnostic,
	})
	if err != nil {
		return coreServices{}, err
	}
	return coreServices{
		definitions: definitionService,
		templates:   templateService,
		preferences: preferenceService,
		inbox:       inboxService,
	}, nil
}

type deliveryServices struct {
	dispatcher *dispatcher.Service
	events     *events.Service
	commands   *commands.Registry
}

func buildDeliveryServices(
	opts Options,
	providers storage.Providers,
	core coreServices,
	registry *adapters.Registry,
	q queue.Queue,
	lgr logger.Logger,
	secretsResolver secrets.Resolver,
	hooks activity.Hooks,
	cfg config.Config,
) (deliveryServices, error) {
	dispatcherService, err := dispatcher.New(dispatcher.Dependencies{
		Definitions: providers.Definitions, Events: providers.Events,
		Messages: providers.Messages, Attempts: providers.DeliveryAttempts,
		Templates: core.templates, Registry: registry, Attachments: opts.Attachments,
		LinkBuilder: opts.LinkBuilder, LinkStore: opts.LinkStore,
		LinkObserver: opts.LinkObserver, LinkPolicy: opts.LinkPolicy,
		Logger: lgr, Config: cfg.Dispatcher, Preferences: core.preferences,
		Inbox: core.inbox, Secrets: secretsResolver, Backoff: opts.Backoff,
		Activity: hooks, Persistence: opts.Persistence, Privacy: opts.Privacy,
		Diagnostic: opts.Diagnostic,
	})
	if err != nil {
		return deliveryServices{}, err
	}
	eventService, err := events.New(events.Dependencies{
		Definitions: providers.Definitions, Events: providers.Events,
		Messages: providers.Messages, Attempts: providers.DeliveryAttempts,
		Publications: providers.Publications, RetryOperations: providers.RetryOperations,
		Dispatcher: dispatcherService, Queue: q, Logger: lgr, Activity: hooks,
		Privacy: opts.Privacy, Diagnostic: opts.Diagnostic,
	})
	if err != nil {
		return deliveryServices{}, err
	}
	commandRegistry, err := commands.New(commands.Dependencies{
		DefinitionService: core.definitions, Templates: core.templates,
		Preferences: core.preferences, Inbox: core.inbox, Events: eventService, Logger: lgr,
	})
	if err != nil {
		return deliveryServices{}, err
	}
	return deliveryServices{
		dispatcher: dispatcherService, events: eventService, commands: commandRegistry,
	}, nil
}

func withDefaults(opts Options) Options {
	if isZeroConfig(opts.Config) {
		opts.Config = config.Defaults()
	}
	if opts.Storage.Definitions == nil {
		opts.Storage = storage.NewMemoryProviders()
	}
	if opts.Logger == nil {
		opts.Logger = logger.Default()
	}
	if opts.Cache == nil {
		opts.Cache = &cache.Nop{}
	}
	if opts.Queue == nil {
		opts.Queue = &queue.Nop{}
	}
	if opts.Broadcaster == nil {
		opts.Broadcaster = &broadcaster.Nop{}
	}
	if opts.Secrets == nil {
		opts.Secrets = secrets.SimpleResolver{Provider: secrets.NopProvider{}}
	}
	if opts.Persistence == nil {
		opts.Persistence = persistencepolicy.DefinitionPolicy{}
	}
	if opts.Privacy == nil {
		opts.Privacy = privacy.DefaultPolicy{}
	}
	if opts.Diagnostic == nil {
		opts.Diagnostic = privacy.NopDiagnosticSink{}
	}
	return opts
}

func resolveAdapters(opts Options) []adapters.Messenger {
	if opts.RecipientResolver == nil {
		return opts.Adapters
	}
	resolved := make([]adapters.Messenger, 0, len(opts.Adapters))
	for _, messenger := range opts.Adapters {
		if messenger == nil {
			continue
		}
		switch messenger.(type) {
		case adapters.ResolvingMessenger, *adapters.ResolvingMessenger:
			resolved = append(resolved, messenger)
		default:
			resolved = append(resolved, adapters.ResolvingMessenger{
				Inner:      messenger,
				Resolver:   opts.RecipientResolver,
				Privacy:    opts.Privacy,
				Diagnostic: opts.Diagnostic,
			})
		}
	}
	return resolved
}
