package inbox

import (
	"context"
	"errors"
	"time"

	"github.com/goliatone/go-notifications/internal/inbox"
	"github.com/goliatone/go-notifications/pkg/activity"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
)

// Re-export commonly used types so callers don't depend on the internal package.
type (
	CreateInput = inbox.CreateInput
	ListFilters = inbox.ListFilters
)

// Service exposes inbox management helpers to consumers.
type Service struct {
	internal *inbox.Service
}

// Dependencies wires repositories + realtime hooks.
type Dependencies struct {
	Repository  store.InboxRepository
	Broadcaster broadcaster.Broadcaster
	Logger      logger.Logger
	Activity    activity.Hooks
}

var errServiceNotInitialised = errors.New("inbox: service not initialised")

// New constructs the façade.
func New(deps Dependencies) (*Service, error) {
	internalSvc, err := inbox.NewService(inbox.Dependencies{
		Repository:  deps.Repository,
		Broadcaster: deps.Broadcaster,
		Logger:      deps.Logger,
		Activity:    deps.Activity,
	})
	if err != nil {
		return nil, err
	}
	return &Service{internal: internalSvc}, nil
}

// Create inserts a new inbox entry.
func (s *Service) Create(ctx context.Context, input CreateInput) (*domain.InboxItem, error) {
	if s == nil || s.internal == nil {
		return nil, errServiceNotInitialised
	}
	return s.internal.Create(ctx, input)
}

// List enumerates inbox entries for a user.
func (s *Service) List(ctx context.Context, userID string, opts store.ListOptions, filters ListFilters) (store.ListResult[domain.InboxItem], error) {
	if s == nil || s.internal == nil {
		return store.ListResult[domain.InboxItem]{}, errServiceNotInitialised
	}
	return s.internal.List(ctx, userID, opts, filters)
}

// MarkRead toggles unread flags for the provided IDs.
func (s *Service) MarkRead(ctx context.Context, userID string, ids []string, read bool) error {
	if s == nil || s.internal == nil {
		return errServiceNotInitialised
	}
	uuids, err := parseUUIDs(ids)
	if err != nil {
		return err
	}
	return s.internal.MarkRead(ctx, userID, uuids, read)
}

// Snooze defers an inbox item.
func (s *Service) Snooze(ctx context.Context, userID, id string, until int64) error {
	if s == nil || s.internal == nil {
		return errServiceNotInitialised
	}
	itemID, err := parseUUID(id)
	if err != nil {
		return err
	}
	return s.internal.Snooze(ctx, userID, itemID, unixToTime(until))
}

// Dismiss removes an inbox item from the unread queue.
func (s *Service) Dismiss(ctx context.Context, userID, id string) error {
	if s == nil || s.internal == nil {
		return errServiceNotInitialised
	}
	itemID, err := parseUUID(id)
	if err != nil {
		return err
	}
	return s.internal.Dismiss(ctx, userID, itemID)
}

// BadgeCount returns unread counts.
func (s *Service) BadgeCount(ctx context.Context, userID string) (int, error) {
	if s == nil || s.internal == nil {
		return 0, errServiceNotInitialised
	}
	return s.internal.BadgeCount(ctx, userID)
}

// DeliverFromMessage stores an inbox entry from a rendered message.
func (s *Service) DeliverFromMessage(ctx context.Context, msg *domain.NotificationMessage) error {
	if s == nil || s.internal == nil {
		return errServiceNotInitialised
	}
	return s.internal.DeliverFromMessage(ctx, msg)
}

func parseUUIDs(ids []string) ([]uuid.UUID, error) {
	results := make([]uuid.UUID, 0, len(ids))
	for _, raw := range ids {
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, err
		}
		results = append(results, id)
	}
	return results, nil
}

func parseUUID(id string) (uuid.UUID, error) {
	return uuid.Parse(id)
}

func unixToTime(ts int64) time.Time {
	if ts == 0 {
		return time.Time{}
	}
	return time.Unix(ts, 0).UTC()
}
