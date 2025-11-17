package events

import (
	"context"
	"errors"

	"github.com/goliatone/go-notifications/internal/dispatcher"
	interevents "github.com/goliatone/go-notifications/internal/events"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/queue"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
)

// Re-export intake types for callers.
type (
	IntakeRequest       = interevents.IntakeRequest
	DigestOptions       = interevents.DigestOptions
	ScheduledJobPayload = interevents.ScheduledJobPayload
	DigestJobPayload    = interevents.DigestJobPayload
)

// Service exposes the event intake pipeline.
type Service struct {
	internal *interevents.Service
}

// Dependencies wires repositories, dispatcher, and queue.
type Dependencies struct {
	Definitions store.NotificationDefinitionRepository
	Events      store.NotificationEventRepository
	Dispatcher  *dispatcher.Service
	Queue       queue.Queue
	Logger      logger.Logger
}

// New constructs the public façade.
func New(deps Dependencies) (*Service, error) {
	internalSvc, err := interevents.NewService(interevents.Dependencies{
		Definitions: deps.Definitions,
		Events:      deps.Events,
		Dispatcher:  deps.Dispatcher,
		Queue:       deps.Queue,
		Logger:      deps.Logger,
	})
	if err != nil {
		return nil, err
	}
	return &Service{internal: internalSvc}, nil
}

// Enqueue validates and routes the intake request.
func (s *Service) Enqueue(ctx context.Context, req IntakeRequest) error {
	if s == nil || s.internal == nil {
		return errServiceNotInitialised
	}
	return s.internal.Enqueue(ctx, req)
}

// ProcessScheduled runs a scheduled payload (invoked by queue workers).
func (s *Service) ProcessScheduled(ctx context.Context, payload ScheduledJobPayload) error {
	if s == nil || s.internal == nil {
		return errServiceNotInitialised
	}
	return s.internal.ProcessScheduled(ctx, payload)
}

// ProcessDigest flushes a digest batch.
func (s *Service) ProcessDigest(ctx context.Context, payload DigestJobPayload) error {
	if s == nil || s.internal == nil {
		return errServiceNotInitialised
	}
	return s.internal.ProcessDigest(ctx, payload)
}

var errServiceNotInitialised = errors.New("events: service not initialised")
