package events

import (
	"context"
	"testing"
	"time"

	"github.com/goliatone/go-notifications/internal/dispatcher"
	"github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/queue"
)

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
	payload := job.Payload.(DigestJobPayload)
	if err := service.ProcessDigest(ctx, payload); err != nil {
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
		Definitions: defRepo,
		Events:      evtRepo,
		Dispatcher:  disp,
		Queue:       q,
		Logger:      &logger.Nop{},
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
