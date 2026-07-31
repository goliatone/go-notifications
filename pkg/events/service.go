package events

import (
	"context"
	"errors"

	"github.com/goliatone/go-notifications/internal/dispatcher"
	interevents "github.com/goliatone/go-notifications/internal/events"
	"github.com/goliatone/go-notifications/pkg/activity"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/queue"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/privacy"
	"github.com/goliatone/go-notifications/pkg/receipts"
)

// Re-export intake types for callers.
type (
	IntakeRequest         = interevents.IntakeRequest
	ImmediateRequest      = interevents.ImmediateRequest
	RetryRequest          = interevents.RetryRequest
	DigestOptions         = interevents.DigestOptions
	PublicationJobPayload = interevents.PublicationJobPayload
	ScheduledJobPayload   = interevents.ScheduledJobPayload
	DigestJobPayload      = interevents.DigestJobPayload
	DispatchReceipt       = receipts.DispatchReceipt
	DeliveryOutcome       = receipts.DeliveryOutcome
)

// Service exposes the event intake pipeline.
type Service struct {
	internal *interevents.Service
}

// Dependencies wires repositories, dispatcher, and queue.
type Dependencies struct {
	Definitions     store.NotificationDefinitionRepository
	Events          store.NotificationEventRepository
	Messages        store.NotificationMessageRepository
	Attempts        store.DeliveryAttemptRepository
	Publications    store.NotificationPublicationRepository
	RetryOperations store.NotificationRetryOperationRepository
	Dispatcher      *dispatcher.Service
	Queue           queue.Queue
	Logger          logger.Logger
	Activity        activity.Hooks
	Privacy         privacy.Policy
	Diagnostic      privacy.DiagnosticSink
}

// New constructs the public façade.
func New(deps Dependencies) (*Service, error) {
	internalSvc, err := interevents.NewService(interevents.Dependencies{
		Definitions:     deps.Definitions,
		Events:          deps.Events,
		Messages:        deps.Messages,
		Attempts:        deps.Attempts,
		Publications:    deps.Publications,
		RetryOperations: deps.RetryOperations,
		Dispatcher:      deps.Dispatcher,
		Queue:           deps.Queue,
		Logger:          deps.Logger,
		Activity:        deps.Activity,
		Privacy:         deps.Privacy,
		Diagnostic:      deps.Diagnostic,
	})
	if err != nil {
		return nil, err
	}
	return &Service{internal: internalSvc}, nil
}

func (s *Service) RetryWithReceipt(ctx context.Context, req RetryRequest) (DispatchReceipt, error) {
	if s == nil || s.internal == nil {
		return DispatchReceipt{}, errServiceNotInitialised
	}
	return s.internal.RetryWithReceipt(ctx, req)
}

func (s *Service) EnqueueWithReceipt(ctx context.Context, req IntakeRequest) (DispatchReceipt, error) {
	if s == nil || s.internal == nil {
		return DispatchReceipt{}, errServiceNotInitialised
	}
	return s.internal.EnqueueWithReceipt(ctx, req)
}

func (s *Service) DispatchImmediate(ctx context.Context, req ImmediateRequest) (DispatchReceipt, error) {
	if s == nil || s.internal == nil {
		return DispatchReceipt{}, errServiceNotInitialised
	}
	return s.internal.DispatchImmediate(ctx, req)
}

func (s *Service) ProcessPublication(ctx context.Context, payload PublicationJobPayload) (DispatchReceipt, error) {
	if s == nil || s.internal == nil {
		return DispatchReceipt{}, errServiceNotInitialised
	}
	return s.internal.ProcessPublication(ctx, payload)
}

func (s *Service) RecoverPending(ctx context.Context, limit int) error {
	if s == nil || s.internal == nil {
		return errServiceNotInitialised
	}
	return s.internal.RecoverPending(ctx, limit)
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
