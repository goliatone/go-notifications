package commands

import (
	"context"
	"errors"
	"strings"
	"time"

	command "github.com/goliatone/go-command"
	"github.com/goliatone/go-notifications/pkg/definitions"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/events"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/preferences"
	"github.com/goliatone/go-notifications/pkg/retention"
	"github.com/goliatone/go-notifications/pkg/templates"
	"github.com/google/uuid"
)

const (
	CommandTypeTemplateUpsert = "notifications.template.upsert"
	CommandTypeInboxMarkRead  = "notifications.inbox.mark_read"
	CommandTypeInboxDismiss   = "notifications.inbox.dismiss"
	CommandTypeInboxSnooze    = "notifications.inbox.snooze"
)

type resultCommander[T, R any] interface {
	command.Commander[T]
	Run(context.Context, T) (R, error)
}

// Catalog exposes go-command compatible handlers for host transports.
type Catalog struct {
	CreateDefinition command.Commander[CreateDefinition]
	SaveTemplate     command.Commander[TemplateUpsert]
	UpsertPreference command.Commander[preferences.PreferenceInput]
	InboxMarkRead    command.Commander[InboxMarkRead]
	InboxDismiss     command.Commander[InboxDismiss]
	InboxSnooze      command.Commander[InboxSnooze]
	EnqueueEvent     resultCommander[events.IntakeRequest, events.DispatchReceipt]
	RetryEvent       resultCommander[events.RetryRequest, events.DispatchReceipt]
	PurgeRetention   resultCommander[retention.Request, retention.Result]
}

type definitionService interface {
	Upsert(context.Context, definitions.UpsertInput) (*domain.NotificationDefinition, error)
}

type templateService interface {
	Upsert(context.Context, templates.TemplateInput, bool) (*domain.NotificationTemplate, error)
}

type preferenceService interface {
	Upsert(context.Context, preferences.PreferenceInput) (*domain.NotificationPreference, error)
}

type inboxService interface {
	MarkRead(ctx context.Context, userID string, ids []string, read bool) error
	Dismiss(ctx context.Context, userID, id string) error
	Snooze(ctx context.Context, userID, id string, until int64) error
}

type eventService interface {
	EnqueueWithReceipt(context.Context, events.IntakeRequest) (events.DispatchReceipt, error)
	RetryWithReceipt(context.Context, events.RetryRequest) (events.DispatchReceipt, error)
}

type retentionService interface {
	Purge(context.Context, retention.Request) (retention.Result, error)
}

// Dependencies wires services into the command catalog.
type Dependencies struct {
	Definitions definitionService
	Templates   templateService
	Preferences preferenceService
	Inbox       inboxService
	Events      eventService
	Retention   retentionService
	Logger      logger.Logger
}

// NewCatalog builds the command catalog using the supplied dependencies.
func NewCatalog(deps Dependencies) (*Catalog, error) {
	if deps.Definitions == nil {
		return nil, errors.New("commands: definitions service is required")
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
	if deps.Retention == nil {
		return nil, errors.New("commands: retention service is required")
	}
	if deps.Logger == nil {
		deps.Logger = logger.Default()
	}

	return &Catalog{
		CreateDefinition: definitionUpsertCommand{service: deps.Definitions},
		SaveTemplate:     templateUpsertCommand{service: deps.Templates},
		UpsertPreference: preferenceUpsertCommand{service: deps.Preferences},
		InboxMarkRead:    inboxMarkReadCommand{service: deps.Inbox},
		InboxDismiss:     inboxDismissCommand{service: deps.Inbox},
		InboxSnooze:      inboxSnoozeCommand{service: deps.Inbox},
		EnqueueEvent:     eventEnqueueCommand{service: deps.Events},
		RetryEvent:       eventRetryCommand{service: deps.Events},
		PurgeRetention:   retentionPurgeCommand{service: deps.Retention},
	}, nil
}

// CreateDefinition is retained as the public compatibility name.
type CreateDefinition = definitions.UpsertInput

type definitionUpsertCommand struct {
	service definitionService
}

func (c definitionUpsertCommand) Execute(ctx context.Context, msg CreateDefinition) error {
	if err := msg.Validate(); err != nil {
		return err
	}
	_, err := c.service.Upsert(ctx, msg)
	return err
}

// TemplateUpsert wraps templates.TemplateInput for command invocation.
type TemplateUpsert struct {
	templates.TemplateInput
	AllowUpdate bool `json:"allow_update"`
}

func (TemplateUpsert) Type() string { return CommandTypeTemplateUpsert }

func (msg TemplateUpsert) Validate() error {
	if strings.TrimSpace(msg.Code) == "" {
		return errors.New("commands: template code is required")
	}
	if strings.TrimSpace(msg.Channel) == "" {
		return errors.New("commands: template channel is required")
	}
	if strings.TrimSpace(msg.Locale) == "" {
		return errors.New("commands: template locale is required")
	}
	return nil
}

type templateUpsertCommand struct {
	service templateService
}

func (c templateUpsertCommand) Execute(ctx context.Context, msg TemplateUpsert) error {
	if err := msg.Validate(); err != nil {
		return err
	}
	_, err := c.service.Upsert(ctx, msg.TemplateInput, msg.AllowUpdate)
	return err
}

type preferenceUpsertCommand struct {
	service preferenceService
}

func (c preferenceUpsertCommand) Execute(ctx context.Context, msg preferences.PreferenceInput) error {
	if err := msg.Validate(); err != nil {
		return err
	}
	_, err := c.service.Upsert(ctx, msg)
	return err
}

// InboxMarkRead request payload.
type InboxMarkRead struct {
	UserID string   `json:"user_id"`
	IDs    []string `json:"ids"`
	Read   bool     `json:"read"`
}

func (InboxMarkRead) Type() string { return CommandTypeInboxMarkRead }

func (msg InboxMarkRead) Validate() error {
	if strings.TrimSpace(msg.UserID) == "" {
		return errors.New("commands: inbox user ID is required")
	}
	if len(msg.IDs) == 0 {
		return errors.New("commands: at least one inbox item ID is required")
	}
	for _, id := range msg.IDs {
		if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
			return errors.New("commands: inbox item ID is invalid")
		}
	}
	return nil
}

type inboxMarkReadCommand struct {
	service inboxService
}

func (c inboxMarkReadCommand) Execute(ctx context.Context, msg InboxMarkRead) error {
	if err := msg.Validate(); err != nil {
		return err
	}
	return c.service.MarkRead(ctx, msg.UserID, msg.IDs, msg.Read)
}

// InboxDismiss dismisses a notification.
type InboxDismiss struct {
	UserID string `json:"user_id"`
	ID     string `json:"id"`
}

func (InboxDismiss) Type() string { return CommandTypeInboxDismiss }

func (msg InboxDismiss) Validate() error {
	if strings.TrimSpace(msg.UserID) == "" {
		return errors.New("commands: inbox user ID is required")
	}
	if _, err := uuid.Parse(strings.TrimSpace(msg.ID)); err != nil {
		return errors.New("commands: inbox item ID is invalid")
	}
	return nil
}

type inboxDismissCommand struct {
	service inboxService
}

func (c inboxDismissCommand) Execute(ctx context.Context, msg InboxDismiss) error {
	if err := msg.Validate(); err != nil {
		return err
	}
	return c.service.Dismiss(ctx, msg.UserID, msg.ID)
}

// InboxSnooze defers delivery until a timestamp.
type InboxSnooze struct {
	UserID string    `json:"user_id"`
	ID     string    `json:"id"`
	Until  time.Time `json:"until"`
}

func (InboxSnooze) Type() string { return CommandTypeInboxSnooze }

func (msg InboxSnooze) Validate() error {
	if strings.TrimSpace(msg.UserID) == "" {
		return errors.New("commands: inbox user ID is required")
	}
	if _, err := uuid.Parse(strings.TrimSpace(msg.ID)); err != nil {
		return errors.New("commands: inbox item ID is invalid")
	}
	if msg.Until.IsZero() {
		return errors.New("commands: inbox snooze time is required")
	}
	return nil
}

type inboxSnoozeCommand struct {
	service inboxService
}

func (c inboxSnoozeCommand) Execute(ctx context.Context, msg InboxSnooze) error {
	if err := msg.Validate(); err != nil {
		return err
	}
	return c.service.Snooze(ctx, msg.UserID, msg.ID, msg.Until.Unix())
}

type eventEnqueueCommand struct {
	service eventService
}

func (c eventEnqueueCommand) Run(ctx context.Context, msg events.IntakeRequest) (events.DispatchReceipt, error) {
	if err := msg.Validate(); err != nil {
		return events.DispatchReceipt{}, err
	}
	return c.service.EnqueueWithReceipt(ctx, msg)
}

func (c eventEnqueueCommand) Execute(ctx context.Context, msg events.IntakeRequest) error {
	_, err := c.Run(ctx, msg)
	return err
}

type eventRetryCommand struct {
	service eventService
}

func (c eventRetryCommand) Run(ctx context.Context, msg events.RetryRequest) (events.DispatchReceipt, error) {
	if err := msg.Validate(); err != nil {
		return events.DispatchReceipt{}, err
	}
	return c.service.RetryWithReceipt(ctx, msg)
}

func (c eventRetryCommand) Execute(ctx context.Context, msg events.RetryRequest) error {
	_, err := c.Run(ctx, msg)
	return err
}

type retentionPurgeCommand struct{ service retentionService }

func (c retentionPurgeCommand) Run(ctx context.Context, msg retention.Request) (retention.Result, error) {
	if err := msg.Validate(); err != nil {
		return retention.Result{}, err
	}
	return c.service.Purge(ctx, msg)
}

func (c retentionPurgeCommand) Execute(ctx context.Context, msg retention.Request) error {
	_, err := c.Run(ctx, msg)
	return err
}
