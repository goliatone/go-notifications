package events

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/goliatone/go-notifications/internal/dispatcher"
	"github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/queue"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/privacy"
)

func TestScopedIdempotencyReturnsReplayWithoutRedispatch(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, q := setupDeps(t)
	service := newTestService(t, defRepo, evtRepo, disp, q)
	req := IntakeRequest{
		DefinitionCode: "welcome", Recipients: []string{"user@example.com"},
		IdempotencyScope: "tenant:one", IdempotencyKey: "  Key-1  ",
	}
	first, err := service.EnqueueWithReceipt(ctx, req)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	second, err := service.EnqueueWithReceipt(ctx, req)
	if err != nil {
		t.Fatalf("replay enqueue: %v", err)
	}
	if first.EventID != second.EventID || !second.Replay || len(disp.events) != 1 {
		t.Fatalf("unexpected replay: first=%+v second=%+v dispatches=%d", first, second, len(disp.events))
	}
}

func TestConcurrentScopedIdempotencyCreatesOneEvent(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, q := setupDeps(t)
	service := newTestService(t, defRepo, evtRepo, disp, q)
	req := IntakeRequest{
		DefinitionCode: "welcome", Recipients: []string{"subject-1"},
		IdempotencyScope: "system", IdempotencyKey: "same",
	}
	var wg sync.WaitGroup
	errCh := make(chan error, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, enqueueErr := service.EnqueueWithReceipt(ctx, req); enqueueErr != nil {
				errCh <- enqueueErr
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for enqueueErr := range errCh {
		t.Errorf("enqueue concurrently: %v", enqueueErr)
	}
	result, err := evtRepo.List(ctx, store.ListOptions{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected one event, got %d", result.Total)
	}
}

func TestConcurrentDigestIntakeCreatesOneDurableWindow(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, _ := setupDeps(t)
	publications := memory.NewPublicationRepository()
	q := &lockedQueue{}
	service, err := NewService(Dependencies{
		Definitions: defRepo, Events: evtRepo, Publications: publications,
		Dispatcher: disp, Queue: q, Logger: &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	var wg sync.WaitGroup
	errCh := make(chan error, 20)
	for idx := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, enqueueErr := service.EnqueueWithReceipt(ctx, IntakeRequest{
				DefinitionCode: "welcome",
				Recipients:     []string{fmt.Sprintf("subject-%d", idx)},
				Digest:         &DigestOptions{Key: "daily", Delay: time.Hour},
			}); enqueueErr != nil {
				errCh <- enqueueErr
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for enqueueErr := range errCh {
		t.Errorf("enqueue digest member: %v", enqueueErr)
	}
	publicationResult, err := publications.List(ctx, store.ListOptions{})
	if err != nil {
		t.Fatalf("list publications: %v", err)
	}
	if publicationResult.Total != 1 || q.count() != 1 {
		t.Fatalf("expected one publication/job, got publications=%d jobs=%d", publicationResult.Total, q.count())
	}
	events, err := evtRepo.ListByPublication(ctx, publicationResult.Items[0].ID)
	if err != nil || len(events) != 20 {
		t.Fatalf("expected 20 durable members, got %d (err=%v)", len(events), err)
	}
}

func TestIdempotencyFingerprintConflict(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, q := setupDeps(t)
	service := newTestService(t, defRepo, evtRepo, disp, q)
	req := IntakeRequest{
		DefinitionCode: "welcome", Recipients: []string{"subject-1"},
		IdempotencyScope: "system", IdempotencyKey: "same",
	}
	if _, err := service.EnqueueWithReceipt(ctx, req); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	req.Recipients = []string{"subject-2"}
	_, err := service.EnqueueWithReceipt(ctx, req)
	var safe privacy.SafeError
	if !errors.As(err, &safe) || safe.Code != "idempotency_conflict" {
		t.Fatalf("expected safe idempotency conflict, got %v", err)
	}
}

func TestScheduledIntakeRejectsNopQueue(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, _ := setupDeps(t)
	service, err := NewService(Dependencies{
		Definitions: defRepo, Events: evtRepo, Publications: memory.NewPublicationRepository(),
		Dispatcher: disp, Queue: &queue.Nop{}, Logger: &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = service.EnqueueWithReceipt(ctx, IntakeRequest{
		DefinitionCode: "welcome", Recipients: []string{"subject-1"},
		ScheduleAt: time.Now().Add(time.Hour),
	})
	var safe privacy.SafeError
	if !errors.As(err, &safe) || safe.Code != "durable_queue_required" {
		t.Fatalf("expected durable queue error, got %v", err)
	}
}

func TestPublicationRecoveryUsesStableQueueKey(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, _ := setupDeps(t)
	publications := memory.NewPublicationRepository()
	failing := &failingQueue{err: errors.New("queue unavailable")}
	service, err := NewService(Dependencies{
		Definitions: defRepo, Events: evtRepo, Publications: publications,
		Dispatcher: disp, Queue: failing, Logger: &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = service.EnqueueWithReceipt(ctx, IntakeRequest{
		DefinitionCode: "welcome", Recipients: []string{"subject-1"},
		ScheduleAt: time.Now().Add(time.Hour),
	})
	if err == nil {
		t.Fatalf("expected enqueue failure")
	}
	success := &stubQueue{}
	service.queue = success
	if err := service.RecoverPending(ctx, 10); err != nil {
		t.Fatalf("recover: %v", err)
	}
	if len(failing.jobs) != 1 || len(success.jobs) != 1 || failing.jobs[0].Key != success.jobs[0].Key {
		t.Fatalf("expected stable recovery key: failed=%v recovered=%v", failing.jobs, success.jobs)
	}
}

func TestRetryIsDurableAndIdempotent(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, q := setupDeps(t)
	event := &domain.NotificationEvent{
		DefinitionCode: "welcome", Recipients: domain.StringList{"subject-1"},
		Status: domain.EventStatusFailed,
	}
	if err := evtRepo.Create(ctx, event); err != nil {
		t.Fatalf("create event: %v", err)
	}
	service, err := NewService(Dependencies{
		Definitions: defRepo, Events: evtRepo, Publications: memory.NewPublicationRepository(),
		RetryOperations: memory.NewRetryOperationRepository(), Dispatcher: disp,
		Queue: q, Logger: &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	req := RetryRequest{EventID: event.ID, IdempotencyKey: "retry-1"}
	first, err := service.RetryWithReceipt(ctx, req)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	second, err := service.RetryWithReceipt(ctx, req)
	if err != nil {
		t.Fatalf("retry replay: %v", err)
	}
	if first.RetryOperationID != second.RetryOperationID || !second.Replay || len(disp.events) != 1 {
		t.Fatalf("unexpected retry receipts: first=%+v second=%+v dispatches=%d", first, second, len(disp.events))
	}
}

func TestEnqueueImmediateDispatch(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, q := setupDeps(t)
	service := newTestService(t, defRepo, evtRepo, disp, q)

	request := IntakeRequest{
		DefinitionCode: "welcome",
		Recipients:     []string{"user@example.com"},
		Context:        map[string]any{"name": "Rosa"},
	}
	if err := service.Enqueue(ctx, request); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if len(disp.events) != 1 {
		t.Fatalf("expected dispatcher call, got %d", len(disp.events))
	}
}

func TestEnqueueSchedulesFutureJob(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, q := setupDeps(t)
	service := newTestService(t, defRepo, evtRepo, disp, q)

	schedule := time.Now().Add(10 * time.Minute)
	err := service.Enqueue(ctx, IntakeRequest{
		DefinitionCode: "welcome",
		Recipients:     []string{"user@example.com"},
		ScheduleAt:     schedule,
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if len(q.jobs) != 1 {
		t.Fatalf("expected job scheduled, got %d", len(q.jobs))
	}
}

func TestDigestProcessingMergesEntries(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, q := setupDeps(t)
	service := newTestService(t, defRepo, evtRepo, disp, q)

	req := IntakeRequest{
		DefinitionCode: "welcome",
		Recipients:     []string{"user1"},
		Context:        map[string]any{"id": 1},
		Digest: &DigestOptions{
			Key:   "daily",
			Delay: time.Minute,
		},
	}
	if err := service.Enqueue(ctx, req); err != nil {
		t.Fatalf("enqueue digest: %v", err)
	}
	req2 := req
	req2.Recipients = []string{"user2"}
	req2.Context = map[string]any{"id": 2}
	if err := service.Enqueue(ctx, req2); err != nil {
		t.Fatalf("enqueue digest second: %v", err)
	}
	if len(q.jobs) != 1 {
		t.Fatalf("expected single digest job, got %d", len(q.jobs))
	}
	job := q.jobs[0]
	payload, ok := job.Payload.(PublicationJobPayload)
	if !ok {
		t.Fatalf("expected publication job payload, got %T", job.Payload)
	}
	if _, err := service.ProcessPublication(ctx, payload); err != nil {
		t.Fatalf("process digest: %v", err)
	}
	if len(disp.events) != 1 {
		t.Fatalf("expected single dispatch after digest, got %d", len(disp.events))
	}
	event := disp.events[0]
	if len(event.Recipients) != 2 {
		t.Fatalf("expected merged recipients, got %v", event.Recipients)
	}
}

func setupDeps(t *testing.T) (*memory.DefinitionRepository, *memory.EventRepository, *stubDispatcher, *stubQueue) {
	t.Helper()
	defRepo := memory.NewDefinitionRepository()
	if err := defRepo.Create(context.Background(), &domain.NotificationDefinition{
		Code: "welcome",
	}); err != nil {
		t.Fatalf("seed definition: %v", err)
	}
	evtRepo := memory.NewEventRepository()
	disp := &stubDispatcher{}
	q := &stubQueue{}
	return defRepo, evtRepo, disp, q
}

func newTestService(t *testing.T, defRepo *memory.DefinitionRepository, evtRepo *memory.EventRepository, disp *stubDispatcher, q *stubQueue) *Service {
	t.Helper()
	service, err := NewService(Dependencies{
		Definitions:  defRepo,
		Events:       evtRepo,
		Publications: memory.NewPublicationRepository(),
		Dispatcher:   disp,
		Queue:        q,
		Logger:       &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

type stubDispatcher struct {
	events []*domain.NotificationEvent
}

func (s *stubDispatcher) Dispatch(ctx context.Context, event *domain.NotificationEvent, opts dispatcher.DispatchOptions) error {
	s.events = append(s.events, event)
	return nil
}

type stubQueue struct {
	jobs []queue.Job
}

func (s *stubQueue) Enqueue(ctx context.Context, job queue.Job) error {
	s.jobs = append(s.jobs, job)
	return nil
}

type failingQueue struct {
	jobs []queue.Job
	err  error
}

func (q *failingQueue) Enqueue(_ context.Context, job queue.Job) error {
	q.jobs = append(q.jobs, job)
	return q.err
}

type lockedQueue struct {
	mu   sync.Mutex
	jobs []queue.Job
}

func (q *lockedQueue) Enqueue(_ context.Context, job queue.Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = append(q.jobs, job)
	return nil
}

func (q *lockedQueue) count() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.jobs)
}
