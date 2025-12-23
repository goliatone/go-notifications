package commands

import (
	"context"
	"errors"
	"strings"
	"time"

	command "github.com/goliatone/go-command"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/events"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/preferences"
	"github.com/goliatone/go-notifications/pkg/templates"
)

// Catalog exposes go-command compatible handlers for host transports.
type Catalog struct {
	CreateDefinition command.Commander[CreateDefinition]
	SaveTemplate     command.Commander[TemplateUpsert]
	UpsertPreference command.Commander[preferences.PreferenceInput]
	InboxMarkRead    command.Commander[InboxMarkRead]
	InboxDismiss     command.Commander[InboxDismiss]
	InboxSnooze      command.Commander[InboxSnooze]
	EnqueueEvent     command.Commander[events.IntakeRequest]
}

type templateService interface {
	Get(ctx context.Context, code, channel, locale string) (*domain.NotificationTemplate, error)
	Create(ctx context.Context, input templates.TemplateInput) (*domain.NotificationTemplate, error)
	Update(ctx context.Context, input templates.TemplateInput) (*domain.NotificationTemplate, error)
}

type preferenceService interface {
	Upsert(ctx context.Context, input preferences.PreferenceInput) (*domain.NotificationPreference, error)
}

type inboxService interface {
	MarkRead(ctx context.Context, userID string, ids []string, read bool) error
	Dismiss(ctx context.Context, userID, id string) error
	Snooze(ctx context.Context, userID, id string, until int64) error
}

type eventService interface {
	Enqueue(ctx context.Context, req events.IntakeRequest) error
}

// Dependencies wires repositories and services into the command catalog.
type Dependencies struct {
	Definitions store.NotificationDefinitionRepository
	Templates   templateService
	Preferences preferenceService
	Inbox       inboxService
	Events      eventService
	Logger      logger.Logger
}

// NewCatalog builds the command catalog using the supplied dependencies.
func NewCatalog(deps Dependencies) (*Catalog, error) {
	if deps.Definitions == nil {
		return nil, errors.New("commands: definition repository is required")
	}
	if deps.Templates == nil {
		return nil, errors.New("commands: templates service is required")
	}
	if deps.Preferences == nil {
		return nil, errors.New("commands: preferences service is required")
	}
	if deps.Inbox == nil {
		return nil, errors.New("commands: inbox service is required")
	}
	if deps.Events == nil {
		return nil, errors.New("commands: events service is required")
	}
	if deps.Logger == nil {
		deps.Logger = &logger.Nop{}
	}

	return &Catalog{
		CreateDefinition: definitionCreateCommand{repo: deps.Definitions},
		SaveTemplate:     templateUpsertCommand{templates: deps.Templates},
		UpsertPreference: preferenceUpsertCommand{svc: deps.Preferences},
		InboxMarkRead:    inboxMarkReadCommand{svc: deps.Inbox},
		InboxDismiss:     inboxDismissCommand{svc: deps.Inbox},
		InboxSnooze:      inboxSnoozeCommand{svc: deps.Inbox},
		EnqueueEvent:     eventEnqueueCommand{svc: deps.Events},
	}, nil
}

// CreateDefinition represents the payload for creating or updating definitions.
type CreateDefinition struct {
	Code        string         `json:"code"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Severity    string         `json:"severity"`
	Category    string         `json:"category"`
	Channels    []string       `json:"channels"`
	TemplateIDs []string       `json:"template_keys"`
	Metadata    map[string]any `json:"metadata"`
	AllowUpdate bool           `json:"allow_update"`
	Policy      map[string]any `json:"policy"`
}

type definitionCreateCommand struct {
	repo store.NotificationDefinitionRepository
}

func (c definitionCreateCommand) Execute(ctx context.Context, msg CreateDefinition) error {
	msg.Code = strings.TrimSpace(msg.Code)
	if msg.Code == "" {
		return errors.New("commands: definition code is required")
	}
	def := &domain.NotificationDefinition{
		Code:        msg.Code,
		Name:        msg.Name,
		Description: msg.Description,
		Severity:    msg.Severity,
		Category:    msg.Category,
		Channels:    domain.StringList(msg.Channels),
		TemplateKeys: func() domain.StringList {
			return domain.StringList(msg.TemplateIDs)
		}(),
		Metadata: domain.JSONMap(msg.Metadata),
		Policy:   domain.JSONMap(msg.Policy),
	}
	if existing, err := c.repo.GetByCode(ctx, msg.Code); err == nil {
		if !msg.AllowUpdate {
			return errors.New("commands: definition already exists")
		}
		existing.Name = def.Name
		existing.Description = def.Description
		existing.Severity = def.Severity
		existing.Category = def.Category
		existing.Channels = def.Channels
		existing.TemplateKeys = def.TemplateKeys
		existing.Metadata = def.Metadata
		existing.Policy = def.Policy
		return c.repo.Update(ctx, existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	return c.repo.Create(ctx, def)
}

// TemplateUpsert wraps templates.TemplateInput for command invocation.
type TemplateUpsert struct {
	templates.TemplateInput
	AllowUpdate bool `json:"allow_update"`
}

type templateUpsertCommand struct {
	templates templateService
}

func (c templateUpsertCommand) Execute(ctx context.Context, msg TemplateUpsert) error {
	input := msg.TemplateInput
	_, err := c.templates.Get(ctx, input.Code, input.Channel, input.Locale)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			_, err := c.templates.Create(ctx, input)
			return err
		}
		return err
	}
	if !msg.AllowUpdate {
		return errors.New("commands: template already exists")
	}
	_, err = c.templates.Update(ctx, input)
	return err
}

type preferenceUpsertCommand struct {
	svc preferenceService
}

func (c preferenceUpsertCommand) Execute(ctx context.Context, msg preferences.PreferenceInput) error {
	_, err := c.svc.Upsert(ctx, msg)
	return err
}

// InboxMarkRead request payload.
type InboxMarkRead struct {
	UserID string   `json:"user_id"`
	IDs    []string `json:"ids"`
	Read   bool     `json:"read"`
}

type inboxMarkReadCommand struct {
	svc inboxService
}

func (c inboxMarkReadCommand) Execute(ctx context.Context, msg InboxMarkRead) error {
	return c.svc.MarkRead(ctx, msg.UserID, msg.IDs, msg.Read)
}

// InboxDismiss dismisses a notification.
type InboxDismiss struct {
	UserID string `json:"user_id"`
	ID     string `json:"id"`
}

type inboxDismissCommand struct {
	svc inboxService
}

func (c inboxDismissCommand) Execute(ctx context.Context, msg InboxDismiss) error {
	return c.svc.Dismiss(ctx, msg.UserID, msg.ID)
}

// InboxSnooze defers delivery until a timestamp.
type InboxSnooze struct {
	UserID string    `json:"user_id"`
	ID     string    `json:"id"`
	Until  time.Time `json:"until"`
}

type inboxSnoozeCommand struct {
	svc inboxService
}

func (c inboxSnoozeCommand) Execute(ctx context.Context, msg InboxSnooze) error {
	return c.svc.Snooze(ctx, msg.UserID, msg.ID, msg.Until.Unix())
}

type eventEnqueueCommand struct {
	svc eventService
}

func (c eventEnqueueCommand) Execute(ctx context.Context, msg events.IntakeRequest) error {
	return c.svc.Enqueue(ctx, msg)
}
