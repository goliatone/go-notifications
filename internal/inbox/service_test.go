package inbox

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/privacy"
	"github.com/google/uuid"
)

func TestServiceCreateAndList(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewInboxRepository()
	events := captureBroadcaster()
	svc := newTestService(t, repo, events)

	item, err := svc.Create(ctx, CreateInput{
		UserID: "user-1",
		Title:  "Welcome",
		Body:   "Body",
		Locale: "en",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if item.ID == uuid.Nil {
		t.Fatalf("expected persisted inbox item")
	}
	if len(events.events) != 1 || events.events[0].Topic != "inbox.created" {
		t.Fatalf("expected broadcast on create, got %+v", events.events)
	}

	result, err := svc.List(ctx, "user-1", storeOpts(), ListFilters{UnreadOnly: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 item, got %d", result.Total)
	}
}

func TestServiceMarkReadSnoozeDismiss(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewInboxRepository()
	events := captureBroadcaster()
	svc := newTestService(t, repo, events)

	item, err := svc.Create(ctx, CreateInput{
		UserID: "user-2",
		Title:  "Alert",
		Body:   "Body",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if actionErr := svc.MarkRead(ctx, "user-2", []uuid.UUID{item.ID}, true); actionErr != nil {
		t.Fatalf("mark read: %v", actionErr)
	}
	payload, ok := events.events[len(events.events)-1].Payload.(map[string]any)
	if !ok || payload["unread"] != false {
		t.Fatalf("mark-read broadcast contained stale state: %+v", events.events)
	}
	if actionErr := svc.Snooze(ctx, "user-2", item.ID, time.Now().Add(2*time.Hour)); actionErr != nil {
		t.Fatalf("snooze: %v", actionErr)
	}
	if actionErr := svc.Dismiss(ctx, "user-2", item.ID); actionErr != nil {
		t.Fatalf("dismiss: %v", actionErr)
	}
	count, err := svc.BadgeCount(ctx, "user-2")
	if err != nil {
		t.Fatalf("badge count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected dismissed item to reduce badge count")
	}
}

func TestDeliverFromMessage(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewInboxRepository()
	svc := newTestService(t, repo, captureBroadcaster())

	msg := &domain.NotificationMessage{
		RecordMeta: domain.RecordMeta{ID: uuid.New()},
		Receiver:   "user-3",
		Subject:    "Subject",
		Body:       "Body",
		Locale:     "en",
		ActionURL:  "https://example.com",
	}
	if err := svc.DeliverFromMessage(ctx, msg); err != nil {
		t.Fatalf("deliver: %v", err)
	}
}

func TestRepositoryFailureReturnsSafeErrorAndReportsRawDiagnostic(t *testing.T) {
	raw := errors.New("database leaked person@example.com token-123")
	diagnostic := &captureDiagnostic{}
	svc, err := NewService(Dependencies{
		Repository: &failingInboxRepository{
			InboxRepository: memory.NewInboxRepository(),
			err:             raw,
		},
		Broadcaster: &broadcaster.Nop{},
		Logger:      &logger.Nop{},
		Diagnostic:  diagnostic,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.Create(context.Background(), CreateInput{
		UserID: "subject-1", Title: "Title", Body: "Body",
	})
	var safe privacy.SafeError
	if !errors.As(err, &safe) || errors.Is(err, raw) ||
		strings.Contains(err.Error(), "person@example.com") ||
		strings.Contains(err.Error(), "token-123") {
		t.Fatalf("expected non-wrapping safe inbox error, got %v", err)
	}
	if !errors.Is(diagnostic.event.Cause, raw) || diagnostic.event.Operation != "inbox_create_failed" {
		t.Fatalf("unexpected diagnostic event: %+v", diagnostic.event)
	}
}

func TestBroadcasterFailureIsDiagnosticOnly(t *testing.T) {
	raw := errors.New("broadcaster leaked private body")
	diagnostic := &captureDiagnostic{}
	svc, err := NewService(Dependencies{
		Repository:  memory.NewInboxRepository(),
		Broadcaster: failingBroadcaster{err: raw},
		Logger:      &logger.Nop{},
		Diagnostic:  diagnostic,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.Create(context.Background(), CreateInput{
		UserID: "subject-1", Title: "Title", Body: "Body",
	}); err != nil {
		t.Fatalf("broadcast failure should not fail inbox persistence: %v", err)
	}
	if !errors.Is(diagnostic.event.Cause, raw) || diagnostic.event.Operation != "inbox_broadcast_failed" {
		t.Fatalf("unexpected broadcast diagnostic: %+v", diagnostic.event)
	}
}

type failingInboxRepository struct {
	*memory.InboxRepository
	err error
}

func (r *failingInboxRepository) Create(context.Context, *domain.InboxItem) error {
	return r.err
}

type failingBroadcaster struct {
	err error
}

func (b failingBroadcaster) Broadcast(context.Context, broadcaster.Event) error {
	return b.err
}

type captureDiagnostic struct {
	event privacy.DiagnosticEvent
}

func (d *captureDiagnostic) Report(_ context.Context, event privacy.DiagnosticEvent) {
	d.event = event
}

type capturedEvents struct {
	mu     sync.Mutex
	events []broadcaster.Event
}

func captureBroadcaster() *capturedEvents {
	sink := &capturedEvents{}
	return sink
}

func (c *capturedEvents) Broadcast(ctx context.Context, event broadcaster.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
	return nil
}

func newTestService(t *testing.T, repo *memory.InboxRepository, br broadcaster.Broadcaster) *Service {
	t.Helper()
	svc, err := NewService(Dependencies{
		Repository:  repo,
		Broadcaster: br,
		Logger:      &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func storeOpts() store.ListOptions {
	return store.ListOptions{
		Limit:  50,
		Offset: 0,
	}
}
