package inbox

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/goliatone/go-notifications/pkg/activity"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
)

// CreateInput captures the fields required to insert a new inbox item.
type CreateInput struct {
	UserID    string
	MessageID uuid.UUID
	Title     string
	Body      string
	Locale    string
	ActionURL string
	Pinned    bool
	Metadata  domain.JSONMap
}

// ListFilters allow callers to refine mailbox queries.
type ListFilters struct {
	UnreadOnly       bool
	IncludeDismissed bool
	PinnedOnly       bool
	SnoozedOnly      bool
	Before           time.Time
}

// Dependencies wires repositories and realtime hooks into the service.
type Dependencies struct {
	Repository  store.InboxRepository
	Broadcaster broadcaster.Broadcaster
	Logger      logger.Logger
	Activity    activity.Hooks
}

// Service manages inbox CRUD and realtime fan-out.
type Service struct {
	repo        store.InboxRepository
	broadcaster broadcaster.Broadcaster
	logger      logger.Logger
	activity    activity.Hooks
}

var (
	errRepositoryRequired = errors.New("inbox: repository is required")
)

// NewService constructs the inbox service.
func NewService(deps Dependencies) (*Service, error) {
	if deps.Repository == nil {
		return nil, errRepositoryRequired
	}
	if deps.Broadcaster == nil {
		deps.Broadcaster = &broadcaster.Nop{}
	}
	if deps.Logger == nil {
		deps.Logger = &logger.Nop{}
	}
	return &Service{
		repo:        deps.Repository,
		broadcaster: deps.Broadcaster,
		logger:      deps.Logger,
		activity:    deps.Activity,
	}, nil
}

// Create inserts a new inbox entry.
func (s *Service) Create(ctx context.Context, input CreateInput) (*domain.InboxItem, error) {
	if err := validateCreateInput(input); err != nil {
		return nil, err
	}
	item := &domain.InboxItem{
		UserID:       strings.TrimSpace(input.UserID),
		MessageID:    input.MessageID,
		Title:        input.Title,
		Body:         input.Body,
		Locale:       input.Locale,
		ActionURL:    input.ActionURL,
		Metadata:     cloneJSON(input.Metadata),
		Unread:       true,
		Pinned:       input.Pinned,
		SnoozedUntil: time.Time{},
	}
	if err := s.repo.Create(ctx, item); err != nil {
		return nil, err
	}
	s.emit(ctx, "inbox.created", item)
	s.activity.Notify(ctx, activity.Event{
		Verb:       "notification.inbox.created",
		ActorID:    item.UserID,
		UserID:     item.UserID,
		ObjectType: "inbox_item",
		ObjectID:   item.ID.String(),
		Metadata: map[string]any{
			"title":      item.Title,
			"pinned":     item.Pinned,
			"message_id": item.MessageID.String(),
		},
	})
	return item, nil
}

// List returns inbox items for the given user applying the supplied filters.
func (s *Service) List(ctx context.Context, userID string, opts store.ListOptions, filters ListFilters) (store.ListResult[domain.InboxItem], error) {
	result, err := s.repo.ListByUser(ctx, strings.TrimSpace(userID), opts)
	if err != nil {
		return store.ListResult[domain.InboxItem]{}, err
	}
	items := make([]domain.InboxItem, 0, len(result.Items))
	for _, item := range result.Items {
		if !filters.IncludeDismissed && !item.DismissedAt.IsZero() {
			continue
		}
		if filters.UnreadOnly && !item.Unread {
			continue
		}
		if !filters.Before.IsZero() && !item.CreatedAt.Before(filters.Before) {
			continue
		}
		if filters.PinnedOnly && !item.Pinned {
			continue
		}
		if filters.SnoozedOnly && item.SnoozedUntil.IsZero() {
			continue
		}
		items = append(items, item)
	}
	return store.ListResult[domain.InboxItem]{Items: items, Total: len(items)}, nil
}

// MarkRead toggles the unread flag for the provided items. IDs that do not
// belong to the user are ignored to avoid leaking existence checks.
func (s *Service) MarkRead(ctx context.Context, userID string, ids []uuid.UUID, read bool) error {
	userID = strings.TrimSpace(userID)
	for _, id := range ids {
		item, err := s.repo.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if item.UserID != userID {
			continue
		}
		if err := s.repo.MarkRead(ctx, id, read); err != nil {
			return err
		}
		s.emit(ctx, "inbox.updated", item)
		verb := "notification.unread"
		if read {
			verb = "notification.read"
		}
		s.activity.Notify(ctx, activity.Event{
			Verb:       verb,
			ActorID:    userID,
			UserID:     item.UserID,
			ObjectType: "inbox_item",
			ObjectID:   item.ID.String(),
		})
	}
	return nil
}

// Snooze defers an inbox item until the specified timestamp.
func (s *Service) Snooze(ctx context.Context, userID string, id uuid.UUID, until time.Time) error {
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if item.UserID != strings.TrimSpace(userID) {
		return nil
	}
	if err := s.repo.Snooze(ctx, id, until); err != nil {
		return err
	}
	item.SnoozedUntil = until
	s.emit(ctx, "inbox.updated", item)
	s.activity.Notify(ctx, activity.Event{
		Verb:       "notification.snoozed",
		ActorID:    userID,
		UserID:     item.UserID,
		ObjectType: "inbox_item",
		ObjectID:   item.ID.String(),
		Metadata: map[string]any{
			"until": until,
		},
	})
	return nil
}

// Dismiss marks an inbox item as dismissed and clears the unread flag.
func (s *Service) Dismiss(ctx context.Context, userID string, id uuid.UUID) error {
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if item.UserID != strings.TrimSpace(userID) {
		return nil
	}
	if err := s.repo.Dismiss(ctx, id); err != nil {
		return err
	}
	item.DismissedAt = time.Now().UTC()
	item.Unread = false
	s.emit(ctx, "inbox.updated", item)
	s.activity.Notify(ctx, activity.Event{
		Verb:       "notification.dismissed",
		ActorID:    userID,
		UserID:     item.UserID,
		ObjectType: "inbox_item",
		ObjectID:   item.ID.String(),
	})
	return nil
}

// BadgeCount returns the unread count for the given user.
func (s *Service) BadgeCount(ctx context.Context, userID string) (int, error) {
	return s.repo.CountUnread(ctx, strings.TrimSpace(userID))
}

// DeliverFromMessage converts a rendered notification message into an inbox item.
func (s *Service) DeliverFromMessage(ctx context.Context, msg *domain.NotificationMessage) error {
	if msg == nil {
		return errors.New("inbox: message is required")
	}
	input := CreateInput{
		UserID:    msg.Receiver,
		MessageID: msg.ID,
		Title:     msg.Subject,
		Body:      msg.Body,
		Locale:    msg.Locale,
	}
	if action, ok := msg.Metadata["action_url"].(string); ok {
		input.ActionURL = action
	}
	item, err := s.Create(ctx, input)
	if err != nil {
		return err
	}
	s.logger.Info("inbox delivery created", logger.Field{Key: "user_id", Value: item.UserID})
	return nil
}

func (s *Service) emit(ctx context.Context, topic string, item *domain.InboxItem) {
	if item == nil {
		return
	}
	payload := broadcaster.Event{
		Topic: topic,
		Payload: map[string]any{
			"id":         item.ID.String(),
			"user_id":    item.UserID,
			"title":      item.Title,
			"unread":     item.Unread,
			"dismissed":  !item.DismissedAt.IsZero(),
			"snoozed_at": item.SnoozedUntil,
		},
	}
	if err := s.broadcaster.Broadcast(ctx, payload); err != nil {
		s.logger.Warn("broadcast inbox event failed", logger.Field{Key: "error", Value: err})
	}
}

func validateCreateInput(input CreateInput) error {
	if strings.TrimSpace(input.UserID) == "" {
		return errors.New("inbox: user_id is required")
	}
	if strings.TrimSpace(input.Title) == "" {
		return errors.New("inbox: title is required")
	}
	if strings.TrimSpace(input.Body) == "" {
		return errors.New("inbox: body is required")
	}
	return nil
}

func cloneJSON(src domain.JSONMap) domain.JSONMap {
	if len(src) == 0 {
		return nil
	}
	out := make(domain.JSONMap, len(src))
	for key, value := range src {
		out[key] = value
	}
	return out
}
