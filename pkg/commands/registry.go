package commands

import (
	"context"

	command "github.com/goliatone/go-command"
	internalcommands "github.com/goliatone/go-notifications/internal/commands"
	"github.com/goliatone/go-notifications/pkg/definitions"
	"github.com/goliatone/go-notifications/pkg/events"
	"github.com/goliatone/go-notifications/pkg/inbox"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/preferences"
	"github.com/goliatone/go-notifications/pkg/retention"
	"github.com/goliatone/go-notifications/pkg/templates"
)

// Re-export request types so consumers need not import internal packages.
type (
	CreateDefinition = internalcommands.CreateDefinition
	TemplateUpsert   = internalcommands.TemplateUpsert
	InboxMarkRead    = internalcommands.InboxMarkRead
	InboxDismiss     = internalcommands.InboxDismiss
	InboxSnooze      = internalcommands.InboxSnooze
)

// ResultCommander preserves Commander compatibility while exposing a typed result.
type ResultCommander[T, R any] interface {
	command.Commander[T]
	Run(context.Context, T) (R, error)
}

// Registry exposes go-command compatible handlers backed by the module services.
type Registry struct {
	Catalog          *internalcommands.Catalog
	CreateDefinition command.Commander[CreateDefinition]
	SaveTemplate     command.Commander[TemplateUpsert]
	UpsertPreference command.Commander[preferences.PreferenceInput]
	InboxMarkRead    command.Commander[InboxMarkRead]
	InboxDismiss     command.Commander[InboxDismiss]
	InboxSnooze      command.Commander[InboxSnooze]
	EnqueueEvent     ResultCommander[events.IntakeRequest, events.DispatchReceipt]
	RetryEvent       ResultCommander[events.RetryRequest, events.DispatchReceipt]
	PurgeRetention   ResultCommander[retention.Request, retention.Result]
}

// Dependencies mirror the internal command dependencies but keep them public.
type Dependencies struct {
	// Definitions is retained for compatibility; DefinitionService takes precedence.
	Definitions       store.NotificationDefinitionRepository
	DefinitionService *definitions.Service
	Templates         *templates.Service
	Preferences       *preferences.Service
	Inbox             *inbox.Service
	Events            *events.Service
	Retention         *retention.Service
	Logger            logger.Logger
}

// New builds the registry using the provided dependencies.
func New(deps Dependencies) (*Registry, error) {
	definitionService := deps.DefinitionService
	if definitionService == nil {
		var err error
		definitionService, err = definitions.New(deps.Definitions)
		if err != nil {
			return nil, err
		}
	}
	retentionService := deps.Retention
	if retentionService == nil {
		var err error
		retentionService, err = retention.New(nil)
		if err != nil {
			return nil, err
		}
	}
	catalog, err := internalcommands.NewCatalog(internalcommands.Dependencies{
		Definitions: definitionService,
		Templates:   deps.Templates,
		Preferences: deps.Preferences,
		Inbox:       deps.Inbox,
		Events:      deps.Events,
		Retention:   retentionService,
		Logger:      deps.Logger,
	})
	if err != nil {
		return nil, err
	}
	return &Registry{
		Catalog:          catalog,
		CreateDefinition: catalog.CreateDefinition,
		SaveTemplate:     catalog.SaveTemplate,
		UpsertPreference: catalog.UpsertPreference,
		InboxMarkRead:    catalog.InboxMarkRead,
		InboxDismiss:     catalog.InboxDismiss,
		InboxSnooze:      catalog.InboxSnooze,
		EnqueueEvent:     catalog.EnqueueEvent,
		RetryEvent:       catalog.RetryEvent,
		PurgeRetention:   catalog.PurgeRetention,
	}, nil
}

// Commanders returns every handler so callers can register them with go-command registries.
func (r *Registry) Commanders() []any {
	if r == nil {
		return nil
	}
	return []any{
		r.CreateDefinition,
		r.SaveTemplate,
		r.UpsertPreference,
		r.InboxMarkRead,
		r.InboxDismiss,
		r.InboxSnooze,
		r.EnqueueEvent,
		r.RetryEvent,
		r.PurgeRetention,
	}
}
