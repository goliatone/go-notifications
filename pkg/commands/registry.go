package commands

import (
	command "github.com/goliatone/go-command"
	internalcommands "github.com/goliatone/go-notifications/internal/commands"
	"github.com/goliatone/go-notifications/pkg/events"
	"github.com/goliatone/go-notifications/pkg/inbox"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/preferences"
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

// Registry exposes go-command compatible handlers backed by the module services.
type Registry struct {
	Catalog          *internalcommands.Catalog
	CreateDefinition command.Commander[CreateDefinition]
	SaveTemplate     command.Commander[TemplateUpsert]
	UpsertPreference command.Commander[preferences.PreferenceInput]
	InboxMarkRead    command.Commander[InboxMarkRead]
	InboxDismiss     command.Commander[InboxDismiss]
	InboxSnooze      command.Commander[InboxSnooze]
	EnqueueEvent     command.Commander[events.IntakeRequest]
}

// Dependencies mirror the internal command dependencies but keep them public.
type Dependencies struct {
	Definitions store.NotificationDefinitionRepository
	Templates   *templates.Service
	Preferences *preferences.Service
	Inbox       *inbox.Service
	Events      *events.Service
	Logger      logger.Logger
}

// New builds the registry using the provided dependencies.
func New(deps Dependencies) (*Registry, error) {
	catalog, err := internalcommands.NewCatalog(internalcommands.Dependencies{
		Definitions: deps.Definitions,
		Templates:   deps.Templates,
		Preferences: deps.Preferences,
		Inbox:       deps.Inbox,
		Events:      deps.Events,
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
	}
}
