package inbox

import (
	"context"
	"testing"
	"time"

	"github.com/goliatone/go-notifications/internal/inbox"
	"github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

func TestServiceFacade(t *testing.T) {
	repo := memory.NewInboxRepository()
	internalSvc, err := inbox.NewService(inbox.Dependencies{
		Repository:  repo,
		Broadcaster: &broadcaster.Nop{},
		Logger:      &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("internal service: %v", err)
	}
	// Wrap using public service.
	svc := &Service{internal: internalSvc}

	item, err := svc.Create(context.Background(), CreateInput{
		UserID: "user-a",
		Title:  "Ping",
		Body:   "Body",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.MarkRead(context.Background(), "user-a", []string{item.ID.String()}, true); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if err := svc.Snooze(context.Background(), "user-a", item.ID.String(), time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatalf("snooze: %v", err)
	}
	if err := svc.Dismiss(context.Background(), "user-a", item.ID.String()); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
}
