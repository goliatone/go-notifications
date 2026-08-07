package bunrepo

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func TestBunRetentionRepositoryPurgesTerminalGraphTransactionally(t *testing.T) {
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	sqldb, err := sql.Open(sqliteshim.DriverName(), dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	for _, model := range []any{
		(*domain.NotificationEvent)(nil), (*domain.NotificationMessage)(nil),
		(*domain.DeliveryAttempt)(nil), (*domain.InboxItem)(nil),
		(*domain.NotificationPublication)(nil), (*domain.NotificationRetryOperation)(nil),
	} {
		if _, err := db.NewCreateTable().Model(model).IfNotExists().Exec(ctx); err != nil {
			t.Fatalf("create %T: %v", model, err)
		}
	}

	old := time.Now().UTC().Add(-2 * time.Hour)
	cutoff := time.Now().UTC().Add(-time.Hour)
	publicationID, eventID, messageID := uuid.New(), uuid.New(), uuid.New()
	records := []any{
		&domain.NotificationPublication{
			RecordMeta: domain.RecordMeta{ID: publicationID, CreatedAt: old, UpdatedAt: old},
			Kind:       "scheduled", QueueKey: "terminal", Status: domain.PublicationStatusCompleted,
		},
		&domain.NotificationEvent{
			RecordMeta:     domain.RecordMeta{ID: eventID, CreatedAt: old, UpdatedAt: old},
			DefinitionCode: "welcome", PublicationID: publicationID, Status: domain.EventStatusProcessed,
		},
		&domain.NotificationMessage{
			RecordMeta: domain.RecordMeta{ID: messageID, CreatedAt: old, UpdatedAt: old},
			EventID:    eventID, Channel: "email", Receiver: "opaque", Status: domain.MessageStatusDelivered,
		},
		&domain.DeliveryAttempt{
			RecordMeta: domain.RecordMeta{ID: uuid.New(), CreatedAt: old, UpdatedAt: old},
			MessageID:  messageID, Adapter: "fake", Status: domain.AttemptStatusSucceeded,
		},
		&domain.InboxItem{
			RecordMeta: domain.RecordMeta{ID: uuid.New(), CreatedAt: old, UpdatedAt: old},
			UserID:     "opaque", MessageID: messageID, DismissedAt: old,
		},
		&domain.NotificationRetryOperation{
			RecordMeta: domain.RecordMeta{ID: uuid.New(), CreatedAt: old, UpdatedAt: old},
			EventID:    eventID, RetryScope: "system", IdempotencyKey: "retry", Status: domain.RetryStatusCompleted,
		},
	}
	for _, record := range records {
		if _, err := db.NewInsert().Model(record).Exec(ctx); err != nil {
			t.Fatalf("insert %T: %v", record, err)
		}
	}

	activeEventID, activeMessageID := uuid.New(), uuid.New()
	for _, record := range []any{
		&domain.NotificationEvent{
			RecordMeta:     domain.RecordMeta{ID: activeEventID, CreatedAt: old, UpdatedAt: old},
			DefinitionCode: "welcome", Status: domain.EventStatusRetrying,
		},
		&domain.NotificationMessage{
			RecordMeta: domain.RecordMeta{ID: activeMessageID, CreatedAt: old, UpdatedAt: old},
			EventID:    activeEventID, Channel: "email", Receiver: "opaque", Status: domain.MessageStatusFailed,
		},
		&domain.DeliveryAttempt{
			RecordMeta: domain.RecordMeta{ID: uuid.New(), CreatedAt: old, UpdatedAt: old},
			MessageID:  activeMessageID, Adapter: "fake", Status: domain.AttemptStatusFailed,
		},
	} {
		if _, err := db.NewInsert().Model(record).Exec(ctx); err != nil {
			t.Fatalf("insert active %T: %v", record, err)
		}
	}

	repo := NewRetentionRepository(db)
	cutoffs := store.RetentionCutoffs{
		EventsBefore: cutoff, MessagesBefore: cutoff, AttemptsBefore: cutoff,
		InboxBefore: cutoff, PublicationsBefore: cutoff, RetryOperationsBefore: cutoff,
	}
	totals := store.RetentionCounts{}
	hasMore := true
	for run := 0; run < 10 && hasMore; run++ {
		counts, more, err := repo.PurgeTerminal(ctx, cutoffs, 2)
		if err != nil {
			t.Fatalf("purge run %d: %v", run, err)
		}
		total := counts.Events + counts.Messages + counts.Attempts + counts.Inbox + counts.Publications + counts.RetryOperations
		if total > 2 {
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
	want := store.RetentionCounts{Events: 1, Messages: 1, Attempts: 1, Inbox: 1, Publications: 1, RetryOperations: 1}
	if hasMore || totals != want {
		t.Fatalf("terminal graph did not converge: totals=%+v want=%+v hasMore=%v", totals, want, hasMore)
	}
	for table, wantCount := range map[string]int{
		"notification_events": 1, "notification_messages": 1, "notification_delivery_attempts": 1,
		"notification_inbox_items": 0, "notification_publications": 0, "notification_retry_operations": 0,
	} {
		var count int
		if err := db.NewRaw("SELECT COUNT(*) FROM "+table).Scan(ctx, &count); err != nil || count != wantCount {
			t.Fatalf("%s count=%d want=%d err=%v", table, count, wantCount, err)
		}
	}
}
