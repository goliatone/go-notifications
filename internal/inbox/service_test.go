package inbox

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
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
	svc := newTestService(t, repo, captureBroadcaster())

	item, err := svc.Create(ctx, CreateInput{
		UserID: "user-2",
		Title:  "Alert",
		Body:   "Body",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.MarkRead(ctx, "user-2", []uuid.UUID{item.ID}, true); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if err := svc.Snooze(ctx, "user-2", item.ID, time.Now().Add(2*time.Hour)); err != nil {
		t.Fatalf("snooze: %v", err)
	}
	if err := svc.Dismiss(ctx, "user-2", item.ID); err != nil {
		t.Fatalf("dismiss: %v", err)
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
