package memory

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
)

func TestRetentionRepositoryPurgesBoundedTerminalGraphAndPreservesActiveWork(t *testing.T) { //nolint:funlen // Linear dependency graph fixture.
	old := time.Now().UTC().Add(-2 * time.Hour)
	cutoff := time.Now().UTC().Add(-time.Hour)
	events := NewEventRepository()
	messages := NewMessageRepository()
	attempts := NewDeliveryRepository()
	inbox := NewInboxRepository()
	publications := NewPublicationRepository(events)
	retries := NewRetryOperationRepository()
	repo := NewRetentionRepository(events, messages, attempts, inbox, publications, retries)

	terminalPublicationID := uuid.New()
	terminalEventID := uuid.New()
	terminalMessageID := uuid.New()
	terminalAttemptID := uuid.New()
	terminalInboxID := uuid.New()
	terminalRetryID := uuid.New()
	events.base.records[terminalEventID] = domain.NotificationEvent{
		RecordMeta:     domain.RecordMeta{ID: terminalEventID, CreatedAt: old, UpdatedAt: old},
		DefinitionCode: "welcome", IdempotencyScope: "system", IdempotencyKey: "terminal",
		PublicationID: terminalPublicationID, Status: domain.EventStatusProcessed,
	}
	events.identity[eventIdentity("system", "welcome", "terminal")] = terminalEventID
	messages.base.records[terminalMessageID] = domain.NotificationMessage{
		RecordMeta: domain.RecordMeta{ID: terminalMessageID, CreatedAt: old, UpdatedAt: old},
		EventID:    terminalEventID, Status: domain.MessageStatusDelivered,
	}
	attempts.base.records[terminalAttemptID] = domain.DeliveryAttempt{
		RecordMeta: domain.RecordMeta{ID: terminalAttemptID, CreatedAt: old, UpdatedAt: old},
		MessageID:  terminalMessageID, Status: domain.AttemptStatusSucceeded,
	}
	inbox.base.records[terminalInboxID] = domain.InboxItem{
		RecordMeta: domain.RecordMeta{ID: terminalInboxID, CreatedAt: old, UpdatedAt: old},
		MessageID:  terminalMessageID, DismissedAt: old,
	}
	retries.base.records[terminalRetryID] = domain.NotificationRetryOperation{
		RecordMeta: domain.RecordMeta{ID: terminalRetryID, CreatedAt: old, UpdatedAt: old},
		EventID:    terminalEventID, RetryScope: "system", IdempotencyKey: "retry", Status: domain.RetryStatusCompleted,
	}
	retries.identity[retryIdentity(terminalEventID, "system", "retry")] = terminalRetryID
	publications.base.records[terminalPublicationID] = domain.NotificationPublication{
		RecordMeta: domain.RecordMeta{ID: terminalPublicationID, CreatedAt: old, UpdatedAt: old},
		Status:     domain.PublicationStatusCompleted,
	}

	activeEventID := uuid.New()
	activeMessageID := uuid.New()
	activeAttemptID := uuid.New()
	events.base.records[activeEventID] = domain.NotificationEvent{
		RecordMeta: domain.RecordMeta{ID: activeEventID, CreatedAt: old, UpdatedAt: old},
		Status:     domain.EventStatusRetrying,
	}
	messages.base.records[activeMessageID] = domain.NotificationMessage{
		RecordMeta: domain.RecordMeta{ID: activeMessageID, CreatedAt: old, UpdatedAt: old},
		EventID:    activeEventID, Status: domain.MessageStatusFailed,
	}
	attempts.base.records[activeAttemptID] = domain.DeliveryAttempt{
		RecordMeta: domain.RecordMeta{ID: activeAttemptID, CreatedAt: old, UpdatedAt: old},
		MessageID:  activeMessageID, Status: domain.AttemptStatusFailed,
	}

	cutoffs := store.RetentionCutoffs{
		EventsBefore: cutoff, MessagesBefore: cutoff, AttemptsBefore: cutoff,
		InboxBefore: cutoff, PublicationsBefore: cutoff, RetryOperationsBefore: cutoff,
	}
	totals := store.RetentionCounts{}
	hasMore := true
	for run := 0; run < 10 && hasMore; run++ {
		counts, more, err := repo.PurgeTerminal(context.Background(), cutoffs, 2)
		if err != nil {
			t.Fatalf("purge run %d: %v", run, err)
		}
		if counts.Events+counts.Messages+counts.Attempts+counts.Inbox+counts.Publications+counts.RetryOperations > 2 {
			t.Fatalf("run exceeded batch: %+v", counts)
		}
		totals.Events += counts.Events
		totals.Messages += counts.Messages
		totals.Attempts += counts.Attempts
		totals.Inbox += counts.Inbox
		totals.Publications += counts.Publications
		totals.RetryOperations += counts.RetryOperations
		hasMore = more
	}
	if hasMore || totals != (store.RetentionCounts{Events: 1, Messages: 1, Attempts: 1, Inbox: 1, Publications: 1, RetryOperations: 1}) {
		t.Fatalf("terminal graph did not converge: totals=%+v hasMore=%v", totals, hasMore)
	}
	if _, ok := events.base.records[activeEventID]; !ok {
		t.Fatalf("active event was deleted")
	}
	if _, ok := messages.base.records[activeMessageID]; !ok {
		t.Fatalf("retryable message was deleted")
	}
	if _, ok := attempts.base.records[activeAttemptID]; !ok {
		t.Fatalf("failed attempt for retryable work was deleted")
	}
}

func TestRetentionRepositoryConcurrentRunsDoNotDoubleCount(t *testing.T) {
	old := time.Now().UTC().Add(-2 * time.Hour)
	cutoff := time.Now().UTC().Add(-time.Hour)
	events := NewEventRepository()
	messages := NewMessageRepository()
	attempts := NewDeliveryRepository()
	inbox := NewInboxRepository()
	publications := NewPublicationRepository(events)
	retries := NewRetryOperationRepository()
	repo := NewRetentionRepository(events, messages, attempts, inbox, publications, retries)
	for range 20 {
		id := uuid.New()
		events.base.records[id] = domain.NotificationEvent{
			RecordMeta:     domain.RecordMeta{ID: id, CreatedAt: old, UpdatedAt: old},
			DefinitionCode: "terminal", Status: domain.EventStatusProcessed,
		}
	}
	cutoffs := store.RetentionCutoffs{
		EventsBefore: cutoff, MessagesBefore: cutoff, AttemptsBefore: cutoff,
		InboxBefore: cutoff, PublicationsBefore: cutoff, RetryOperationsBefore: cutoff,
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	deleted := 0
	for range 4 {
		wg.Go(func() {
			counts, _, err := repo.PurgeTerminal(context.Background(), cutoffs, 7)
			if err != nil {
				t.Errorf("concurrent purge: %v", err)
				return
			}
			mu.Lock()
			deleted += counts.Events
			mu.Unlock()
		})
	}
	wg.Wait()
	if deleted != 20 || len(events.base.records) != 0 {
		t.Fatalf("concurrent purge deleted=%d remaining=%d", deleted, len(events.base.records))
	}
}
