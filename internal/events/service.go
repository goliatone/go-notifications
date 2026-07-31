package events

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/goliatone/go-notifications/internal/dispatcher"
	"github.com/goliatone/go-notifications/pkg/activity"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/queue"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/privacy"
	"github.com/goliatone/go-notifications/pkg/receipts"
	"github.com/google/uuid"
)

const (
	maxIdempotencyKeyLength = 255
	publicationLease        = 5 * time.Minute
)

// IntakeRequest describes persistable notification intake.
type IntakeRequest struct {
	DefinitionCode   string         `json:"definition_code"`
	Recipients       []string       `json:"recipients"`
	Context          map[string]any `json:"context,omitempty"`
	Locale           string         `json:"locale,omitempty"`
	Channels         []string       `json:"channels,omitempty"`
	TenantID         string         `json:"tenant_id,omitempty"`
	ActorID          string         `json:"actor_id,omitempty"`
	CorrelationID    string         `json:"correlation_id,omitempty"`
	RequestID        string         `json:"request_id,omitempty"`
	IdempotencyKey   string         `json:"idempotency_key,omitempty"`
	IdempotencyScope string         `json:"idempotency_scope,omitempty"`
	ScheduleAt       time.Time      `json:"schedule_at"`
	Digest           *DigestOptions `json:"digest,omitempty"`
}

func (IntakeRequest) Type() string { return "notifications.event.enqueue" }

func (req IntakeRequest) Validate() error {
	if strings.TrimSpace(req.DefinitionCode) == "" {
		return errors.New("events: definition code is required")
	}
	if len(req.Recipients) == 0 {
		return errors.New("events: at least one recipient is required")
	}
	if len(strings.TrimSpace(req.IdempotencyKey)) > maxIdempotencyKeyLength {
		return errors.New("events: idempotency key is too long")
	}
	if len(strings.TrimSpace(req.IdempotencyScope)) > maxIdempotencyKeyLength {
		return errors.New("events: idempotency scope is too long")
	}
	if req.Digest != nil && strings.TrimSpace(req.Digest.Key) == "" {
		return errors.New("events: digest key is required")
	}
	return nil
}

// ImmediateRequest is compile-distinct because transient values may never
// cross scheduling, digest, queue, command, or retry boundaries.
type ImmediateRequest struct {
	DefinitionCode   string         `json:"definition_code"`
	Recipients       []string       `json:"recipients"`
	Context          map[string]any `json:"context,omitempty"`
	Transient        map[string]any `json:"-"`
	Locale           string         `json:"locale,omitempty"`
	Channels         []string       `json:"channels,omitempty"`
	TenantID         string         `json:"tenant_id,omitempty"`
	ActorID          string         `json:"actor_id,omitempty"`
	CorrelationID    string         `json:"correlation_id,omitempty"`
	RequestID        string         `json:"request_id,omitempty"`
	IdempotencyKey   string         `json:"idempotency_key,omitempty"`
	IdempotencyScope string         `json:"idempotency_scope,omitempty"`
}

type DigestOptions struct {
	Key   string        `json:"key"`
	Delay time.Duration `json:"delay"`
}

type RetryRequest struct {
	EventID          uuid.UUID `json:"event_id"`
	IdempotencyKey   string    `json:"idempotency_key"`
	IdempotencyScope string    `json:"idempotency_scope,omitempty"`
	CorrelationID    string    `json:"correlation_id,omitempty"`
	RequestID        string    `json:"request_id,omitempty"`
}

func (RetryRequest) Type() string { return "notifications.event.retry" }

func (req RetryRequest) Validate() error {
	if req.EventID == uuid.Nil {
		return errors.New("events: retry event ID is required")
	}
	if _, err := validateRetryKey(req.IdempotencyKey); err != nil {
		return err
	}
	if len(strings.TrimSpace(req.IdempotencyScope)) > maxIdempotencyKeyLength {
		return errors.New("events: retry scope is too long")
	}
	return nil
}

// PublicationJobPayload is the only new asynchronous queue payload.
type PublicationJobPayload struct {
	PublicationID uuid.UUID
}

// Deprecated compatibility payloads. New producers use PublicationJobPayload.
type ScheduledJobPayload struct{ Request IntakeRequest }
type DigestJobPayload struct {
	Key           string
	PublicationID uuid.UUID
}

type Dependencies struct {
	Definitions     store.NotificationDefinitionRepository
	Events          store.NotificationEventRepository
	Messages        store.NotificationMessageRepository
	Attempts        store.DeliveryAttemptRepository
	Publications    store.NotificationPublicationRepository
	RetryOperations store.NotificationRetryOperationRepository
	Dispatcher      dispatcherInterface
	Queue           queue.Queue
	Logger          logger.Logger
	Activity        activity.Hooks
	Privacy         privacy.Policy
	Diagnostic      privacy.DiagnosticSink
}

type dispatcherInterface interface {
	Dispatch(context.Context, *domain.NotificationEvent, dispatcher.DispatchOptions) error
}

type receiptDispatcher interface {
	DispatchWithReceipt(context.Context, *domain.NotificationEvent, dispatcher.DispatchOptions) (receipts.DispatchReceipt, error)
}

type receiptReconstructor interface {
	ReceiptForEvent(context.Context, *domain.NotificationEvent) (receipts.DispatchReceipt, error)
}

type Service struct {
	definitions     store.NotificationDefinitionRepository
	events          store.NotificationEventRepository
	messages        store.NotificationMessageRepository
	attempts        store.DeliveryAttemptRepository
	publications    store.NotificationPublicationRepository
	retryOperations store.NotificationRetryOperationRepository
	dispatcher      dispatcherInterface
	queue           queue.Queue
	logger          logger.Logger
	activity        activity.Hooks
	privacy         privacy.Policy
	diagnostic      privacy.DiagnosticSink
}

var (
	errDefinitionsRequired = errors.New("events: definition repository is required")
	errEventsRepoRequired  = errors.New("events: event repository is required")
	errDispatcherRequired  = errors.New("events: dispatcher is required")
)

func NewService(deps Dependencies) (*Service, error) {
	if deps.Definitions == nil {
		return nil, errDefinitionsRequired
	}
	if deps.Events == nil {
		return nil, errEventsRepoRequired
	}
	if deps.Dispatcher == nil {
		return nil, errDispatcherRequired
	}
	if deps.Queue == nil {
		deps.Queue = &queue.Nop{}
	}
	if deps.Logger == nil {
		deps.Logger = logger.Default()
	}
	if deps.Privacy == nil {
		deps.Privacy = privacy.DefaultPolicy{}
	}
	if deps.Diagnostic == nil {
		deps.Diagnostic = privacy.NopDiagnosticSink{}
	}
	return &Service{
		definitions: deps.Definitions, events: deps.Events, messages: deps.Messages,
		attempts: deps.Attempts, publications: deps.Publications, retryOperations: deps.RetryOperations, dispatcher: deps.Dispatcher,
		queue: deps.Queue, logger: deps.Logger, activity: deps.Activity,
		privacy: deps.Privacy, diagnostic: deps.Diagnostic,
	}, nil
}

// RetryWithReceipt explicitly redelivers an eligible persisted event while
// preserving the original event ID and recording a unique retry operation.
func (s *Service) RetryWithReceipt(ctx context.Context, req RetryRequest) (receipts.DispatchReceipt, error) {
	event, stored, replay, failureReceipt, err := s.prepareRetryOperation(ctx, req)
	if err != nil {
		return failureReceipt, err
	}
	if stored.Status == domain.RetryStatusCompleted || stored.Status == domain.RetryStatusFailed {
		return s.retryReceipt(ctx, event, stored, replay)
	}
	claimedOperation, err := s.retryOperations.Claim(ctx, stored.ID, time.Now().UTC().Add(publicationLease))
	if err != nil {
		return receipts.DispatchReceipt{}, s.publicError("retry_operation_claim_failed", err)
	}
	if !claimedOperation {
		return s.retryReceipt(ctx, event, stored, true)
	}
	claimed, err := s.events.ClaimRetry(ctx, event.ID, time.Now().UTC().Add(publicationLease))
	if err != nil {
		stored.Status = domain.RetryStatusPending
		stored.ClaimUntil = time.Time{}
		if updateErr := s.retryOperations.Update(ctx, stored); updateErr != nil {
			s.diagnostic.Report(ctx, privacy.DiagnosticEvent{
				Operation: "retry_operation_release_failed",
				EventID:   event.ID.String(),
				Cause:     updateErr,
			})
		}
		return receipts.DispatchReceipt{}, s.publicError("retry_claim_failed", err)
	}
	if !claimed {
		return s.handleUnclaimedRetry(ctx, event, stored, replay)
	}
	return s.dispatchRetry(ctx, event, stored, replay)
}

func (s *Service) prepareRetryOperation(
	ctx context.Context,
	req RetryRequest,
) (*domain.NotificationEvent, *domain.NotificationRetryOperation, bool, receipts.DispatchReceipt, error) {
	if s.retryOperations == nil {
		return nil, nil, false, receipts.DispatchReceipt{}, privacy.SafeError{
			Category: "configuration", Code: "retry_store_required",
			Message: "notification retry is unavailable",
		}
	}
	key, err := validateRetryKey(req.IdempotencyKey)
	if err != nil {
		return nil, nil, false, receipts.DispatchReceipt{}, err
	}
	event, err := s.events.GetByID(ctx, req.EventID)
	if err != nil {
		return nil, nil, false, receipts.DispatchReceipt{}, s.publicError("retry_event_not_found", err)
	}
	definition, err := s.definitions.GetByCode(ctx, event.DefinitionCode)
	if err != nil {
		return nil, nil, false, receipts.DispatchReceipt{}, s.publicError("retry_definition_not_found", err)
	}
	if event.TransientDependent || len(stringSlicePolicy(definition.Policy["transient_required_keys"])) > 0 {
		failureReceipt := receipts.DispatchReceipt{EventID: event.ID, Status: receipts.StatusFailed}
		return nil, nil, false, failureReceipt, privacy.SafeError{
			Category: "validation", Code: "transient_retry_forbidden",
			Message: "transient-dependent notifications cannot be retried",
		}
	}
	scope, err := validateRetryScope(req.IdempotencyScope, event.TenantID)
	if err != nil {
		return nil, nil, false, receipts.DispatchReceipt{}, err
	}
	operation := &domain.NotificationRetryOperation{
		EventID: event.ID, RetryScope: scope, IdempotencyKey: key,
		CorrelationID: req.CorrelationID, RequestID: req.RequestID, Status: domain.RetryStatusPending,
	}
	stored, created, err := s.retryOperations.CreateIdempotent(ctx, operation)
	if err != nil {
		return nil, nil, false, receipts.DispatchReceipt{}, s.publicError("retry_operation_create_failed", err)
	}
	return event, stored, !created, receipts.DispatchReceipt{}, nil
}

func (s *Service) handleUnclaimedRetry(
	ctx context.Context,
	event *domain.NotificationEvent,
	stored *domain.NotificationRetryOperation,
	replay bool,
) (receipts.DispatchReceipt, error) {
	current, err := s.events.GetByID(ctx, event.ID)
	if err != nil {
		return receipts.DispatchReceipt{}, s.publicError("retry_event_reload_failed", err)
	}
	if current.Status == domain.EventStatusRetrying && current.RetryClaimUntil.After(time.Now().UTC()) {
		return s.handleActiveRetryClaim(ctx, current, stored, replay)
	}
	stored.Status = domain.RetryStatusFailed
	stored.ErrorCode = "retry_not_eligible"
	if current.Status == domain.EventStatusProcessed && s.retryOperationHasLineage(ctx, current.ID, stored.ID) {
		stored.Status = domain.RetryStatusCompleted
		stored.ErrorCode = ""
	}
	stored.ClaimUntil = time.Time{}
	if updateErr := s.retryOperations.Update(ctx, stored); updateErr != nil {
		return receipts.DispatchReceipt{}, s.publicError("retry_operation_update_failed", updateErr)
	}
	if stored.Status == domain.RetryStatusCompleted {
		return s.retryReceipt(ctx, current, stored, replay)
	}
	return receipts.DispatchReceipt{
			EventID: event.ID, RetryOperationID: stored.ID, Status: receipts.StatusFailed,
		}, privacy.SafeError{
			Category: "conflict", Code: "retry_not_eligible",
			Message: "notification is not eligible for retry",
		}
}

func (s *Service) handleActiveRetryClaim(
	ctx context.Context,
	current *domain.NotificationEvent,
	stored *domain.NotificationRetryOperation,
	replay bool,
) (receipts.DispatchReceipt, error) {
	otherActive, err := s.hasOtherActiveRetryOperation(ctx, current.ID, stored.ID)
	if err != nil {
		return receipts.DispatchReceipt{}, s.publicError("retry_operations_unavailable", err)
	}
	if !otherActive {
		stored.Status = domain.RetryStatusPending
		stored.ClaimUntil = time.Time{}
		if updateErr := s.retryOperations.Update(ctx, stored); updateErr != nil {
			return receipts.DispatchReceipt{}, s.publicError("retry_operation_update_failed", updateErr)
		}
		return s.retryReceipt(ctx, current, stored, replay)
	}
	stored.Status = domain.RetryStatusFailed
	stored.ErrorCode = "retry_not_eligible"
	stored.ClaimUntil = time.Time{}
	if updateErr := s.retryOperations.Update(ctx, stored); updateErr != nil {
		return receipts.DispatchReceipt{}, s.publicError("retry_operation_update_failed", updateErr)
	}
	return receipts.DispatchReceipt{
			EventID: current.ID, RetryOperationID: stored.ID, Status: receipts.StatusRetrying,
			CorrelationID: stored.CorrelationID, RequestID: stored.RequestID,
		},
		privacy.SafeError{
			Category: "conflict", Code: "retry_not_eligible",
			Message: "notification is already being retried",
		}
}

func (s *Service) dispatchRetry(
	ctx context.Context,
	event *domain.NotificationEvent,
	stored *domain.NotificationRetryOperation,
	replay bool,
) (receipts.DispatchReceipt, error) {
	receipt, dispatchErr := s.dispatchWithReceipt(ctx, event, dispatcher.DispatchOptions{
		Channels: event.Channels, Locale: event.Locale,
		RetryFailedOnly: true, RetryOperationID: stored.ID,
	})
	receipt.RetryOperationID = stored.ID
	receipt.Replay = replay
	receipt.CorrelationID = stored.CorrelationID
	receipt.RequestID = stored.RequestID
	if dispatchErr != nil {
		stored.Status = domain.RetryStatusFailed
		stored.ErrorCode = s.privacy.SafeError(dispatchErr).Code
	} else {
		stored.Status = domain.RetryStatusCompleted
		stored.ErrorCode = ""
	}
	stored.ClaimUntil = time.Time{}
	if updateErr := s.retryOperations.Update(ctx, stored); updateErr != nil && dispatchErr == nil {
		return receipt, s.publicError("retry_operation_update_failed", updateErr)
	}
	return receipt, dispatchErr
}

func (s *Service) retryReceipt(
	ctx context.Context,
	event *domain.NotificationEvent,
	operation *domain.NotificationRetryOperation,
	replay bool,
) (receipts.DispatchReceipt, error) {
	current, err := s.events.GetByID(ctx, event.ID)
	if err != nil {
		return receipts.DispatchReceipt{}, s.publicError("retry_event_reload_failed", err)
	}
	receipt, receiptErr := s.receiptForEvent(ctx, current)
	receipt.RetryOperationID = operation.ID
	receipt.Replay = replay
	receipt.CorrelationID = operation.CorrelationID
	receipt.RequestID = operation.RequestID
	return receipt, receiptErr
}

func (s *Service) hasOtherActiveRetryOperation(ctx context.Context, eventID, operationID uuid.UUID) (bool, error) {
	result, err := s.retryOperations.List(ctx, store.ListOptions{})
	if err != nil {
		return false, err
	}
	now := time.Now().UTC()
	for idx := range result.Items {
		candidate := result.Items[idx]
		if candidate.ID != operationID && candidate.EventID == eventID &&
			candidate.Status == domain.RetryStatusProcessing && candidate.ClaimUntil.After(now) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) retryOperationHasLineage(ctx context.Context, eventID, operationID uuid.UUID) bool {
	if s.messages == nil {
		return false
	}
	messages, err := s.messages.ListByEvent(ctx, eventID)
	if err != nil {
		return false
	}
	for idx := range messages {
		if messages[idx].RetryOperationID == operationID {
			return true
		}
	}
	return false
}

func validateRetryKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", privacy.SafeError{Category: "validation", Code: "retry_key_required", Message: "retry key is required"}
	}
	if len(key) > maxIdempotencyKeyLength {
		return "", privacy.SafeError{Category: "validation", Code: "retry_key_too_long", Message: "retry key is too long"}
	}
	return key, nil
}

func validateRetryScope(configured, tenantID string) (string, error) {
	if scope := strings.TrimSpace(configured); scope != "" {
		if len(scope) > maxIdempotencyKeyLength {
			return "", privacy.SafeError{Category: "validation", Code: "retry_scope_too_long", Message: "retry scope is too long"}
		}
		return scope, nil
	}
	if tenantID != "" {
		scope := "tenant:" + tenantID
		if len(scope) > maxIdempotencyKeyLength {
			return "", privacy.SafeError{Category: "validation", Code: "retry_scope_too_long", Message: "retry scope is too long"}
		}
		return scope, nil
	}
	return "system", nil
}

func (s *Service) Enqueue(ctx context.Context, req IntakeRequest) error {
	_, err := s.EnqueueWithReceipt(ctx, req)
	return err
}

func (s *Service) EnqueueWithReceipt(ctx context.Context, req IntakeRequest) (receipts.DispatchReceipt, error) {
	if err := s.validateRequest(ctx, req, nil); err != nil {
		return receipts.DispatchReceipt{}, err
	}
	event, created, err := s.persistEvent(ctx, req, false)
	if err != nil {
		return receipts.DispatchReceipt{}, err
	}
	if !created {
		return s.replayReceipt(ctx, event, req)
	}
	s.notifyCreated(ctx, event, req.Channels, req.Locale, req.Digest)

	if req.Digest != nil && strings.TrimSpace(req.Digest.Key) != "" {
		return s.publish(ctx, event, "digest", digestIdentity(req), time.Now().UTC().Add(req.Digest.Delay))
	}
	if !req.ScheduleAt.IsZero() && req.ScheduleAt.After(time.Now()) {
		return s.publish(ctx, event, "scheduled", "", req.ScheduleAt)
	}
	return s.dispatchWithReceipt(ctx, event, dispatcher.DispatchOptions{Channels: event.Channels, Locale: event.Locale})
}

func (s *Service) DispatchImmediate(ctx context.Context, req ImmediateRequest) (receipts.DispatchReceipt, error) {
	persistable := IntakeRequest{
		DefinitionCode: req.DefinitionCode, Recipients: req.Recipients, Context: req.Context,
		Locale: req.Locale, Channels: req.Channels, TenantID: req.TenantID, ActorID: req.ActorID,
		CorrelationID: req.CorrelationID, RequestID: req.RequestID,
		IdempotencyKey: req.IdempotencyKey, IdempotencyScope: req.IdempotencyScope,
	}
	if err := s.validateRequest(ctx, persistable, req.Transient); err != nil {
		return receipts.DispatchReceipt{}, err
	}
	event, created, err := s.persistEvent(ctx, persistable, len(req.Transient) > 0)
	if err != nil {
		return receipts.DispatchReceipt{}, err
	}
	if !created {
		return s.replayReceipt(ctx, event, persistable)
	}
	s.notifyCreated(ctx, event, req.Channels, req.Locale, nil)
	return s.dispatchWithReceipt(ctx, event, dispatcher.DispatchOptions{
		Channels: event.Channels, Locale: event.Locale, Transient: cloneMap(req.Transient),
	})
}

func (s *Service) ProcessPublication(ctx context.Context, payload PublicationJobPayload) (receipts.DispatchReceipt, error) {
	if s.publications == nil {
		return receipts.DispatchReceipt{}, privacy.SafeError{Category: "configuration", Code: "publication_store_required", Message: "asynchronous notifications are unavailable"}
	}
	publication, err := s.publications.GetByID(ctx, payload.PublicationID)
	if err != nil {
		return receipts.DispatchReceipt{}, s.publicError("publication_not_found", err)
	}
	if publication.Status == domain.PublicationStatusCompleted {
		return s.replayPublication(ctx, publication)
	}
	if publication.Status == domain.PublicationStatusFailed {
		return s.replayFailedPublication(ctx, publication)
	}
	claimed, err := s.publications.Claim(ctx, publication.ID, time.Now().UTC().Add(publicationLease))
	if err != nil {
		return receipts.DispatchReceipt{}, s.publicError("publication_claim_failed", err)
	}
	if !claimed {
		return s.handleUnclaimedPublication(ctx, publication.ID)
	}
	return s.dispatchPublication(ctx, publication)
}

func (s *Service) handleUnclaimedPublication(ctx context.Context, publicationID uuid.UUID) (receipts.DispatchReceipt, error) {
	current, err := s.publications.GetByID(ctx, publicationID)
	if err != nil {
		return receipts.DispatchReceipt{}, s.publicError("publication_reload_failed", err)
	}
	switch current.Status {
	case domain.PublicationStatusCompleted:
		return s.replayPublication(ctx, current)
	case domain.PublicationStatusFailed:
		return s.replayFailedPublication(ctx, current)
	}
	events, err := s.events.ListByPublication(ctx, current.ID)
	if err != nil {
		return receipts.DispatchReceipt{}, s.publicError("publication_members_unavailable", err)
	}
	receipt := receipts.DispatchReceipt{
		PublicationID: current.ID,
		Status:        receipts.StatusDispatching,
		Replay:        true,
	}
	if len(events) == 0 {
		return receipt, nil
	}
	receipt, err = s.receiptForEvent(ctx, &events[0])
	if err != nil {
		return receipts.DispatchReceipt{}, err
	}
	receipt.PublicationID = current.ID
	receipt.Status = receipts.StatusDispatching
	receipt.Replay = true
	return receipt, nil
}

func (s *Service) replayFailedPublication(
	ctx context.Context,
	publication *domain.NotificationPublication,
) (receipts.DispatchReceipt, error) {
	events, err := s.events.ListByPublication(ctx, publication.ID)
	if err != nil {
		return receipts.DispatchReceipt{}, s.publicError("publication_members_unavailable", err)
	}
	receipt := receipts.DispatchReceipt{
		PublicationID: publication.ID,
		Status:        receipts.StatusFailed,
		Replay:        true,
	}
	if len(events) > 0 {
		receipt, err = s.receiptForEvent(ctx, &events[0])
		if err != nil {
			return receipts.DispatchReceipt{}, err
		}
		receipt.PublicationID = publication.ID
		receipt.Status = receipts.StatusFailed
		receipt.Replay = true
	}
	code := publication.ErrorCode
	if code == "" {
		code = "publication_failed"
	}
	return receipt, privacy.SafeError{
		Category: "notification",
		Code:     code,
		Message:  "notification publication failed",
	}
}

func (s *Service) dispatchPublication(
	ctx context.Context,
	publication *domain.NotificationPublication,
) (receipts.DispatchReceipt, error) {
	members, err := s.events.ListByPublication(ctx, publication.ID)
	if err != nil || len(members) == 0 {
		return receipts.DispatchReceipt{}, s.failPublication(ctx, publication, "publication_members_unavailable", err)
	}
	event := &members[0]
	if publication.Kind == "digest" && len(members) > 1 {
		event = mergeDigestMembers(members)
	}
	receipt, dispatchErr := s.dispatchWithReceipt(ctx, event, dispatcher.DispatchOptions{
		Channels: event.Channels,
		Locale:   event.Locale,
	})
	receipt.PublicationID = publication.ID
	if dispatchErr != nil {
		publication.Status = domain.PublicationStatusFailed
		publication.ErrorCode = s.privacy.SafeError(dispatchErr).Code
	} else {
		publication.Status = domain.PublicationStatusCompleted
		publication.ErrorCode = ""
	}
	memberStatus := eventStatusForReceipt(receipt.Status)
	if dispatchErr != nil && memberStatus == "" {
		memberStatus = domain.EventStatusFailed
	}
	if memberStatus != "" {
		for idx := range members {
			if updateErr := s.events.UpdateStatus(ctx, members[idx].ID, memberStatus); updateErr != nil {
				return receipt, s.publicError("event_update_failed", updateErr)
			}
		}
	}
	publication.ClaimUntil = time.Time{}
	if updateErr := s.publications.Update(ctx, publication); updateErr != nil && dispatchErr == nil {
		return receipt, s.publicError("publication_update_failed", updateErr)
	}
	return receipt, dispatchErr
}

func (s *Service) replayPublication(
	ctx context.Context,
	publication *domain.NotificationPublication,
) (receipts.DispatchReceipt, error) {
	events, err := s.events.ListByPublication(ctx, publication.ID)
	if err != nil {
		return receipts.DispatchReceipt{}, s.publicError("publication_members_unavailable", err)
	}
	if len(events) == 0 {
		return receipts.DispatchReceipt{PublicationID: publication.ID, Status: receipts.StatusProcessed, Replay: true}, nil
	}
	receipt, err := s.receiptForEvent(ctx, &events[0])
	if err != nil {
		return receipts.DispatchReceipt{}, err
	}
	receipt.PublicationID, receipt.Replay = publication.ID, true
	return receipt, nil
}

// RecoverPending republishes durable pending ledger entries after enqueue
// failure or process restart. Stable keys make repeated recovery safe.
func (s *Service) RecoverPending(ctx context.Context, limit int) error {
	if s.publications == nil {
		return nil
	}
	if _, ok := s.queue.(*queue.Nop); ok {
		return privacy.SafeError{
			Category: "configuration", Code: "durable_queue_required",
			Message: "asynchronous notifications are unavailable",
		}
	}
	pending, err := s.publications.ListPending(ctx, limit)
	if err != nil {
		return s.publicError("publication_recovery_failed", err)
	}
	for idx := range pending {
		if err := s.enqueuePublication(ctx, &pending[idx]); err != nil {
			return err
		}
	}
	return nil
}

// ProcessScheduled remains for existing workers and immediately persists and
// dispatches the legacy payload.
func (s *Service) ProcessScheduled(ctx context.Context, payload ScheduledJobPayload) error {
	payload.Request.ScheduleAt = time.Time{}
	_, err := s.EnqueueWithReceipt(ctx, payload.Request)
	return err
}

// ProcessDigest remains a compatibility shim for already queued payloads.
func (s *Service) ProcessDigest(ctx context.Context, payload DigestJobPayload) error {
	if payload.PublicationID == uuid.Nil {
		return privacy.SafeError{Category: "validation", Code: "legacy_digest_payload_expired", Message: "digest payload cannot be processed"}
	}
	_, err := s.ProcessPublication(ctx, PublicationJobPayload{PublicationID: payload.PublicationID})
	return err
}

func (s *Service) persistEvent(ctx context.Context, req IntakeRequest, transientDependent bool) (*domain.NotificationEvent, bool, error) {
	scope, key, err := normalizeIdempotency(req.IdempotencyScope, req.TenantID, req.IdempotencyKey)
	if err != nil {
		return nil, false, err
	}
	fingerprint, err := requestFingerprint(req)
	if err != nil {
		return nil, false, s.publicError("request_fingerprint_failed", err)
	}
	channels := slices.Clone(req.Channels)
	if len(channels) == 0 {
		definition, lookupErr := s.definitions.GetByCode(ctx, req.DefinitionCode)
		if lookupErr != nil {
			return nil, false, s.publicError("definition_not_found", lookupErr)
		}
		channels = slices.Clone([]string(definition.Channels))
	}
	record := &domain.NotificationEvent{
		DefinitionCode: req.DefinitionCode, TenantID: req.TenantID, ActorID: req.ActorID,
		Recipients: domain.StringList(slices.Clone(req.Recipients)),
		Channels:   domain.StringList(channels), Locale: req.Locale,
		Context:       domain.JSONMap(cloneMap(req.Context)),
		CorrelationID: req.CorrelationID, RequestID: req.RequestID,
		IdempotencyScope: scope, IdempotencyKey: key, RequestFingerprint: fingerprint,
		TransientDependent: transientDependent,
		ScheduledAt:        req.ScheduleAt, Status: domain.EventStatusPending,
	}
	stored, created, err := s.events.CreateIdempotent(ctx, record)
	if err != nil {
		return nil, false, s.publicError("event_create_failed", err)
	}
	return stored, created, nil
}

func (s *Service) publish(ctx context.Context, event *domain.NotificationEvent, kind, digestKey string, runAt time.Time) (receipts.DispatchReceipt, error) {
	if s.publications == nil {
		return receipts.DispatchReceipt{EventID: event.ID, Status: receipts.StatusFailed},
			privacy.SafeError{Category: "configuration", Code: "publication_store_required", Message: "asynchronous notifications are unavailable"}
	}
	if _, ok := s.queue.(*queue.Nop); ok {
		return receipts.DispatchReceipt{EventID: event.ID, Status: receipts.StatusFailed},
			privacy.SafeError{Category: "configuration", Code: "durable_queue_required", Message: "asynchronous notifications are unavailable"}
	}
	var publication *domain.NotificationPublication
	newPublication := true
	if kind == "digest" {
		candidate := &domain.NotificationPublication{
			Kind: kind, DigestKey: digestKey, RunAt: runAt, Status: domain.PublicationStatusPending,
		}
		candidate.EnsureID()
		candidate.QueueKey = "notification-publication:" + candidate.ID.String()
		var createErr error
		publication, newPublication, createErr = s.publications.CreateOrAttachOpenDigest(ctx, candidate, event)
		if createErr != nil {
			return receipts.DispatchReceipt{EventID: event.ID, Status: receipts.StatusFailed},
				s.publicError("publication_create_failed", createErr)
		}
	}
	if publication == nil {
		publication = &domain.NotificationPublication{
			Kind: kind, DigestKey: digestKey, RunAt: runAt, Status: domain.PublicationStatusPending,
		}
		publication.EnsureID()
		publication.QueueKey = "notification-publication:" + publication.ID.String()
		if err := s.publications.Create(ctx, publication); err != nil {
			return receipts.DispatchReceipt{EventID: event.ID, Status: receipts.StatusFailed}, s.publicError("publication_create_failed", err)
		}
	}
	if kind != "digest" {
		event.PublicationID = publication.ID
		event.DigestKey = digestKey
		event.Status = domain.EventStatusScheduled
		if err := s.events.Update(ctx, event); err != nil {
			return receipts.DispatchReceipt{EventID: event.ID, PublicationID: publication.ID, Status: receipts.StatusFailed}, s.publicError("event_update_failed", err)
		}
	}
	if newPublication {
		if err := s.enqueuePublication(ctx, publication); err != nil {
			return receipts.DispatchReceipt{EventID: event.ID, PublicationID: publication.ID, Status: receipts.StatusFailed}, err
		}
	}
	return receipts.DispatchReceipt{
		EventID: event.ID, PublicationID: publication.ID, Status: receipts.StatusScheduled,
		CorrelationID: event.CorrelationID, RequestID: event.RequestID,
	}, nil
}

func (s *Service) enqueuePublication(ctx context.Context, publication *domain.NotificationPublication) error {
	job := queue.Job{
		Key: publication.QueueKey, RunAt: publication.RunAt,
		Payload: PublicationJobPayload{PublicationID: publication.ID},
	}
	if err := s.queue.Enqueue(ctx, job); err != nil {
		publication.Status = domain.PublicationStatusPending
		publication.ErrorCode = "publication_enqueue_failed"
		updateErr := s.publications.Update(ctx, publication)
		return s.publicError("publication_enqueue_failed", errors.Join(err, updateErr))
	}
	publication.Status = domain.PublicationStatusPublished
	publication.ErrorCode = ""
	if err := s.publications.Update(ctx, publication); err != nil {
		return s.publicError("publication_update_failed", err)
	}
	return nil
}

func (s *Service) replayReceipt(ctx context.Context, event *domain.NotificationEvent, req IntakeRequest) (receipts.DispatchReceipt, error) {
	fingerprint, err := requestFingerprint(req)
	if err != nil {
		return receipts.DispatchReceipt{}, s.publicError("request_fingerprint_failed", err)
	}
	if event.RequestFingerprint != fingerprint {
		return receipts.DispatchReceipt{EventID: event.ID, Status: receipts.StatusFailed, Replay: true},
			privacy.SafeError{Category: "conflict", Code: "idempotency_conflict", Message: "idempotency key was already used for a different notification"}
	}
	receipt, err := s.receiptForEvent(ctx, event)
	receipt.Replay = true
	return receipt, err
}

func (s *Service) dispatchWithReceipt(ctx context.Context, event *domain.NotificationEvent, opts dispatcher.DispatchOptions) (receipts.DispatchReceipt, error) {
	if rich, ok := s.dispatcher.(receiptDispatcher); ok {
		receipt, err := rich.DispatchWithReceipt(ctx, event, opts)
		return s.persistDispatchReceipt(ctx, event, receipt, err)
	}
	err := s.dispatcher.Dispatch(ctx, event, opts)
	status := receipts.StatusProcessed
	if err != nil {
		status = receipts.StatusFailed
		err = s.publicError("notification_delivery_failed", err)
	}
	receipt := receipts.DispatchReceipt{
		EventID: event.ID, Status: status, CorrelationID: event.CorrelationID, RequestID: event.RequestID,
	}
	return s.persistDispatchReceipt(ctx, event, receipt, err)
}

func (s *Service) persistDispatchReceipt(
	ctx context.Context,
	event *domain.NotificationEvent,
	receipt receipts.DispatchReceipt,
	dispatchErr error,
) (receipts.DispatchReceipt, error) {
	status := eventStatusForReceipt(receipt.Status)
	if dispatchErr != nil && status == "" {
		status = domain.EventStatusFailed
	}
	if status == "" || event == nil {
		return receipt, dispatchErr
	}
	event.Status = status
	if err := s.events.UpdateStatus(ctx, event.ID, status); err != nil {
		if dispatchErr == nil {
			return receipt, s.publicError("event_update_failed", err)
		}
		s.diagnostic.Report(ctx, privacy.DiagnosticEvent{
			Operation: "event_update_failed",
			EventID:   event.ID.String(),
			Cause:     err,
		})
	}
	return receipt, dispatchErr
}

func eventStatusForReceipt(status receipts.Status) string {
	switch status {
	case receipts.StatusProcessed:
		return domain.EventStatusProcessed
	case receipts.StatusPartial:
		return domain.EventStatusPartial
	case receipts.StatusFailed:
		return domain.EventStatusFailed
	case receipts.StatusDispatching:
		return domain.EventStatusDispatching
	case receipts.StatusRetrying:
		return domain.EventStatusRetrying
	default:
		return ""
	}
}

func (s *Service) receiptForEvent(ctx context.Context, event *domain.NotificationEvent) (receipts.DispatchReceipt, error) {
	if reconstructor, ok := s.dispatcher.(receiptReconstructor); ok {
		return s.reconstructedReceipt(ctx, event, reconstructor)
	}
	return receiptFromEvent(event), nil
}

func (s *Service) reconstructedReceipt(
	ctx context.Context,
	event *domain.NotificationEvent,
	reconstructor receiptReconstructor,
) (receipts.DispatchReceipt, error) {
	receipt, err := reconstructor.ReceiptForEvent(ctx, event)
	if err != nil || event.PublicationID == uuid.Nil || hasTerminalOutcome(receipt.Outcomes) {
		return receipt, err
	}
	members, err := s.events.ListByPublication(ctx, event.PublicationID)
	if err != nil {
		return receipts.DispatchReceipt{}, s.publicError("publication_members_unavailable", err)
	}
	for idx := range members {
		if members[idx].ID == event.ID {
			continue
		}
		candidate, candidateErr := reconstructor.ReceiptForEvent(ctx, &members[idx])
		if candidateErr != nil {
			return receipts.DispatchReceipt{}, candidateErr
		}
		if hasTerminalOutcome(candidate.Outcomes) {
			receipt.Outcomes = candidate.Outcomes
			return receipt, nil
		}
	}
	return receipt, nil
}

func receiptFromEvent(event *domain.NotificationEvent) receipts.DispatchReceipt {
	status := receipts.StatusAccepted
	switch event.Status {
	case domain.EventStatusScheduled:
		status = receipts.StatusScheduled
	case domain.EventStatusDispatching:
		status = receipts.StatusDispatching
	case domain.EventStatusRetrying:
		status = receipts.StatusRetrying
	case domain.EventStatusPartial:
		status = receipts.StatusPartial
	case domain.EventStatusProcessed:
		status = receipts.StatusProcessed
	case domain.EventStatusFailed:
		status = receipts.StatusFailed
	}
	return receipts.DispatchReceipt{
		EventID: event.ID, PublicationID: event.PublicationID, Status: status,
		CorrelationID: event.CorrelationID, RequestID: event.RequestID,
	}
}

func hasTerminalOutcome(outcomes []receipts.DeliveryOutcome) bool {
	for _, outcome := range outcomes {
		if outcome.Status == receipts.OutcomeDelivered ||
			outcome.Status == receipts.OutcomeFailed ||
			outcome.Status == receipts.OutcomeSkipped {
			return true
		}
	}
	return false
}

func (s *Service) validateRequest(ctx context.Context, req IntakeRequest, transient map[string]any) error {
	if strings.TrimSpace(req.DefinitionCode) == "" {
		return privacy.SafeError{Category: "validation", Code: "definition_required", Message: "definition code is required"}
	}
	if len(req.Recipients) == 0 {
		return privacy.SafeError{Category: "validation", Code: "recipient_required", Message: "at least one recipient is required"}
	}
	definition, err := s.definitions.GetByCode(ctx, req.DefinitionCode)
	if err != nil {
		return s.publicError("definition_not_found", err)
	}
	if err := validateTransientSchema(definition, transient); err != nil {
		return err
	}
	if req.Digest != nil && strings.TrimSpace(req.Digest.Key) == "" {
		return privacy.SafeError{Category: "validation", Code: "digest_key_required", Message: "digest key is required"}
	}
	return nil
}

func validateTransientSchema(definition *domain.NotificationDefinition, transient map[string]any) error {
	if definition == nil || len(definition.Policy) == 0 {
		return nil
	}
	required := stringSlicePolicy(definition.Policy["transient_required_keys"])
	allowed := stringSlicePolicy(definition.Policy["transient_allowed_keys"])
	for _, key := range required {
		if _, ok := transient[key]; !ok {
			return privacy.SafeError{
				Category: "validation",
				Code:     "transient_key_required",
				Message:  "required transient render data is missing",
			}
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	for key := range transient {
		if _, ok := allowedSet[key]; !ok {
			return privacy.SafeError{
				Category: "validation",
				Code:     "transient_key_not_allowed",
				Message:  "transient render data contains an unsupported field",
			}
		}
	}
	return nil
}

func stringSlicePolicy(value any) []string {
	var values []string
	switch typed := value.(type) {
	case []string:
		values = append(values, typed...)
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				values = append(values, text)
			}
		}
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func (s *Service) notifyCreated(ctx context.Context, event *domain.NotificationEvent, channels []string, locale string, digest *DigestOptions) {
	s.activity.Notify(ctx, activity.Event{
		Verb: "notification.created", ActorID: event.ActorID, TenantID: event.TenantID,
		ObjectType: "notification_event", ObjectID: event.ID.String(),
		DefinitionCode: event.DefinitionCode, Recipients: safeRecipients(s.privacy, event.Recipients),
		Metadata: s.privacy.SafeMetadata(map[string]any{"channels": channels, "locale": locale, "digest": digest}),
	})
}

func (s *Service) publicError(code string, cause error) error {
	if cause == nil {
		cause = errors.New(code)
	}
	s.diagnostic.Report(context.Background(), privacy.DiagnosticEvent{Operation: code, Cause: cause})
	return privacy.SafeError{Category: "notification", Code: code, Message: "notification operation failed"}
}

func (s *Service) failPublication(ctx context.Context, publication *domain.NotificationPublication, code string, cause error) error {
	publication.Status, publication.ErrorCode, publication.ClaimUntil = domain.PublicationStatusFailed, code, time.Time{}
	updateErr := s.publications.Update(ctx, publication)
	return s.publicError(code, errors.Join(cause, updateErr))
}

func normalizeIdempotency(scope, tenantID, key string) (string, string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", nil
	}
	if len(key) > maxIdempotencyKeyLength {
		return "", "", privacy.SafeError{Category: "validation", Code: "idempotency_key_too_long", Message: "idempotency key is too long"}
	}
	scope = strings.TrimSpace(scope)
	if scope == "" {
		if tenantID != "" {
			scope = "tenant:" + tenantID
		} else {
			scope = "system"
		}
	}
	if len(scope) > maxIdempotencyKeyLength {
		return "", "", privacy.SafeError{Category: "validation", Code: "idempotency_scope_too_long", Message: "idempotency scope is too long"}
	}
	return scope, key, nil
}

func requestFingerprint(req IntakeRequest) (string, error) {
	type material struct {
		DefinitionCode string
		Recipients     []string
		Context        map[string]any
		Locale         string
		Channels       []string
		TenantID       string
		ActorID        string
		ScheduleAt     time.Time
		Digest         *DigestOptions
	}
	recipients, channels := slices.Clone(req.Recipients), slices.Clone(req.Channels)
	slices.Sort(recipients)
	slices.Sort(channels)
	payload, err := json.Marshal(material{
		DefinitionCode: strings.TrimSpace(req.DefinitionCode), Recipients: recipients,
		Context: req.Context, Locale: req.Locale, Channels: channels, TenantID: req.TenantID,
		ActorID: req.ActorID, ScheduleAt: req.ScheduleAt.UTC(), Digest: req.Digest,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func digestIdentity(req IntakeRequest) string {
	scope := strings.TrimSpace(req.IdempotencyScope)
	if scope == "" {
		if req.TenantID != "" {
			scope = "tenant:" + req.TenantID
		} else {
			scope = "system"
		}
	}
	definition := strings.TrimSpace(req.DefinitionCode)
	key := strings.TrimSpace(req.Digest.Key)
	material := strconv.Itoa(len(scope)) + ":" + scope +
		strconv.Itoa(len(definition)) + ":" + definition +
		strconv.Itoa(len(key)) + ":" + key
	sum := sha256.Sum256([]byte(material))
	return hex.EncodeToString(sum[:])
}

func mergeDigestMembers(members []domain.NotificationEvent) *domain.NotificationEvent {
	merged := members[0]
	recipients := make(map[string]struct{})
	entries := make([]map[string]any, 0, len(members))
	for idx := range members {
		for _, recipient := range members[idx].Recipients {
			recipients[recipient] = struct{}{}
		}
		entries = append(entries, cloneMap(members[idx].Context))
	}
	merged.Recipients = make(domain.StringList, 0, len(recipients))
	for recipient := range recipients {
		merged.Recipients = append(merged.Recipients, recipient)
	}
	slices.Sort(merged.Recipients)
	merged.Context = domain.JSONMap{"digest": map[string]any{"count": len(entries), "entries": entries}}
	return &merged
}

func safeRecipients(policy privacy.Policy, recipients []string) []string {
	out := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		out = append(out, policy.SafeSubjectID(recipient))
	}
	return out
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	maps.Copy(out, src)
	return out
}
