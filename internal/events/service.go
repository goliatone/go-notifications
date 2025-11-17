package events

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/goliatone/go-notifications/internal/dispatcher"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/queue"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
)

// IntakeRequest describes an inbound notification request.
type IntakeRequest struct {
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

// DigestOptions groups events before dispatching a batch.
type DigestOptions struct {
	Key   string
	Delay time.Duration
}

// ScheduledJobPayload represents a queued event delivery.
type ScheduledJobPayload struct {
	Request IntakeRequest
}

// DigestJobPayload identifies which digest batch to flush.
type DigestJobPayload struct {
	Key string
}

// Dependencies wires repositories, dispatcher, and queue.
type Dependencies struct {
	Definitions store.NotificationDefinitionRepository
	Events      store.NotificationEventRepository
	Dispatcher  dispatcherInterface
	Queue       queue.Queue
	Logger      logger.Logger
}

type dispatcherInterface interface {
	Dispatch(ctx context.Context, event *domain.NotificationEvent, opts dispatcher.DispatchOptions) error
}

// Service accepts inbound events, validates them, and schedules work.
type Service struct {
	definitions store.NotificationDefinitionRepository
	events      store.NotificationEventRepository
	dispatcher  dispatcherInterface
	queue       queue.Queue
	logger      logger.Logger

	mu      sync.Mutex
	digests map[string]*digestBatch
}

var (
	errDefinitionsRequired = errors.New("events: definition repository is required")
	errEventsRepoRequired  = errors.New("events: event repository is required")
	errDispatcherRequired  = errors.New("events: dispatcher is required")
)

// NewService constructs the intake service.
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
		deps.Logger = &logger.Nop{}
	}
	return &Service{
		definitions: deps.Definitions,
		events:      deps.Events,
		dispatcher:  deps.Dispatcher,
		queue:       deps.Queue,
		logger:      deps.Logger,
		digests:     make(map[string]*digestBatch),
	}, nil
}

// Enqueue validates the request and dispatches, schedules, or digests it.
func (s *Service) Enqueue(ctx context.Context, req IntakeRequest) error {
	if err := s.validateRequest(ctx, req); err != nil {
		return err
	}
	if req.Digest != nil && req.Digest.Key != "" {
		return s.enqueueDigest(ctx, req)
	}
	if !req.ScheduleAt.IsZero() && req.ScheduleAt.After(time.Now().Add(1*time.Second)) {
		payload := ScheduledJobPayload{Request: req}
		job := queue.Job{
			Key:     fmt.Sprintf("event:%s:%d", req.DefinitionCode, req.ScheduleAt.Unix()),
			Payload: payload,
			RunAt:   req.ScheduleAt,
		}
		return s.queue.Enqueue(ctx, job)
	}
	return s.dispatchNow(ctx, req)
}

// ProcessScheduled executes a scheduled request.
func (s *Service) ProcessScheduled(ctx context.Context, payload ScheduledJobPayload) error {
	return s.dispatchNow(ctx, payload.Request)
}

// ProcessDigest flushes the digest batch referenced by the payload.
func (s *Service) ProcessDigest(ctx context.Context, payload DigestJobPayload) error {
	s.mu.Lock()
	batch := s.digests[payload.Key]
	delete(s.digests, payload.Key)
	s.mu.Unlock()

	if batch == nil {
		return nil
	}
	req := batch.merge()
	return s.dispatchNow(ctx, req)
}

func (s *Service) dispatchNow(ctx context.Context, req IntakeRequest) error {
	record := &domain.NotificationEvent{
		DefinitionCode: req.DefinitionCode,
		TenantID:       req.TenantID,
		ActorID:        req.ActorID,
		Recipients:     domain.StringList(req.Recipients),
		Context:        domain.JSONMap(cloneMap(req.Context)),
		ScheduledAt:    time.Now(),
		Status:         domain.EventStatusPending,
	}
	if err := s.events.Create(ctx, record); err != nil {
		return err
	}
	if err := s.dispatcher.Dispatch(ctx, record, dispatcher.DispatchOptions{
		Channels: req.Channels,
		Locale:   req.Locale,
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) validateRequest(ctx context.Context, req IntakeRequest) error {
	if strings.TrimSpace(req.DefinitionCode) == "" {
		return errors.New("events: definition code is required")
	}
	if len(req.Recipients) == 0 {
		return errors.New("events: at least one recipient is required")
	}
	if _, err := s.definitions.GetByCode(ctx, req.DefinitionCode); err != nil {
		return fmt.Errorf("events: definition %s not found: %w", req.DefinitionCode, err)
	}
	return nil
}

func (s *Service) enqueueDigest(ctx context.Context, req IntakeRequest) error {
	key := fmt.Sprintf("%s:%s", req.DefinitionCode, req.Digest.Key)

	s.mu.Lock()
	batch, ok := s.digests[key]
	if !ok {
		batch = newDigestBatch(req)
		s.digests[key] = batch
	} else {
		batch.add(req)
	}
	s.mu.Unlock()

	if ok {
		return nil
	}

	runAt := time.Now().Add(req.Digest.Delay)
	job := queue.Job{
		Key:     fmt.Sprintf("digest:%s", key),
		RunAt:   runAt,
		Payload: DigestJobPayload{Key: key},
	}
	return s.queue.Enqueue(ctx, job)
}

type digestBatch struct {
	request IntakeRequest
	entries []IntakeRequest
}

func newDigestBatch(req IntakeRequest) *digestBatch {
	return &digestBatch{
		request: req,
		entries: []IntakeRequest{req},
	}
}

func (b *digestBatch) add(req IntakeRequest) {
	b.entries = append(b.entries, req)
}

func (b *digestBatch) merge() IntakeRequest {
	if len(b.entries) == 0 {
		return b.request
	}
	recipients := make(map[string]struct{})
	payloads := make([]map[string]any, 0, len(b.entries))

	for _, entry := range b.entries {
		for _, recipient := range entry.Recipients {
			recipients[recipient] = struct{}{}
		}
		payloads = append(payloads, cloneMap(entry.Context))
	}

	mergedRecipients := make([]string, 0, len(recipients))
	for recipient := range recipients {
		mergedRecipients = append(mergedRecipients, recipient)
	}

	base := b.entries[0]
	if base.Context == nil {
		base.Context = make(map[string]any)
	}
	base.Context["digest"] = map[string]any{
		"count":   len(payloads),
		"entries": payloads,
	}
	base.Recipients = mergedRecipients
	base.Digest = nil
	return base
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
