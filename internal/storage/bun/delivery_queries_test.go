package bunrepo

import (
	"context"
	"database/sql"
	"errors"
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

func TestBunDeliveryQueryRepositoryScopesAndProjectsSafeMetadata(t *testing.T) { //nolint:gocyclo // Linear SQL projection fixture.
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	sqldb, err := sql.Open(sqliteshim.DriverName(), dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("close database: %v", closeErr)
		}
	})
	ctx := context.Background()
	for _, model := range []any{
		(*domain.NotificationEvent)(nil), (*domain.NotificationMessage)(nil), (*domain.DeliveryAttempt)(nil),
	} {
		if _, createErr := db.NewCreateTable().Model(model).IfNotExists().Exec(ctx); createErr != nil {
			t.Fatalf("create %T: %v", model, createErr)
		}
	}
	created := time.Now().UTC().Add(-time.Hour)
	event := &domain.NotificationEvent{
		RecordMeta:     domain.RecordMeta{ID: uuid.New(), CreatedAt: created, UpdatedAt: created},
		DefinitionCode: "credential.ready", TenantID: "tenant-a", CorrelationID: "corr-safe",
		Recipients: domain.StringList{"private@example.test"}, Context: domain.JSONMap{"token": "secret"},
		Status: domain.EventStatusProcessed,
	}
	message := &domain.NotificationMessage{
		RecordMeta: domain.RecordMeta{ID: uuid.New(), CreatedAt: created, UpdatedAt: created.Add(2 * time.Minute)},
		EventID:    event.ID, Channel: "email", Receiver: "private@example.test",
		Subject: "secret", Body: "token", Status: domain.MessageStatusFailed,
	}
	attempt := &domain.DeliveryAttempt{
		RecordMeta: domain.RecordMeta{ID: uuid.New(), CreatedAt: created.Add(time.Minute), UpdatedAt: created.Add(time.Minute)},
		MessageID:  message.ID, Adapter: "provider-a", Status: domain.AttemptStatusFailed,
		Error: "raw provider response", ErrorCode: "provider_rejected", Payload: domain.JSONMap{"raw": "secret"},
	}
	broadcast := &domain.NotificationMessage{
		RecordMeta: domain.RecordMeta{ID: uuid.New(), CreatedAt: created.Add(3 * time.Minute), UpdatedAt: created.Add(6 * time.Minute)},
		EventID:    event.ID, Channel: "email", Receiver: "private@example.test", Status: domain.MessageStatusDelivered,
	}
	broadcastSuccess := &domain.DeliveryAttempt{
		RecordMeta: domain.RecordMeta{ID: uuid.New(), CreatedAt: created.Add(4 * time.Minute), UpdatedAt: created.Add(4 * time.Minute)},
		MessageID:  broadcast.ID, Adapter: "provider-b", Status: domain.AttemptStatusSucceeded,
	}
	broadcastFailure := &domain.DeliveryAttempt{
		RecordMeta: domain.RecordMeta{ID: uuid.New(), CreatedAt: created.Add(5 * time.Minute), UpdatedAt: created.Add(5 * time.Minute)},
		MessageID:  broadcast.ID, Adapter: "provider-c", Status: domain.AttemptStatusFailed, ErrorCode: "secondary_failure",
	}
	for _, record := range []any{event, message, attempt, broadcast, broadcastSuccess, broadcastFailure} {
		if _, insertErr := db.NewInsert().Model(record).Exec(ctx); insertErr != nil {
			t.Fatalf("insert %T: %v", record, insertErr)
		}
	}
	repo := NewDeliveryQueryRepository(db)
	got, err := repo.GetDelivery(ctx, store.DeliveryQuery{TenantID: "tenant-a", MessageID: message.ID})
	if err != nil || got.EventID != event.ID || got.MessageID != message.ID || got.Provider != "provider-a" ||
		got.ErrorCode != "provider_rejected" || got.AttemptCount != 1 || got.CorrelationID != "corr-safe" ||
		!got.UpdatedAt.Equal(message.UpdatedAt) {
		t.Fatalf("get delivery=%+v err=%v", got, err)
	}
	broadcastGot, err := repo.GetDelivery(ctx, store.DeliveryQuery{TenantID: "tenant-a", MessageID: broadcast.ID})
	if err != nil || broadcastGot.Status != domain.MessageStatusDelivered || broadcastGot.AttemptCount != 2 ||
		broadcastGot.Provider != "" || broadcastGot.ErrorCode != "" || !broadcastGot.UpdatedAt.Equal(broadcast.UpdatedAt) {
		t.Fatalf("get broadcast delivery=%+v err=%v", broadcastGot, err)
	}
	eventGot, err := repo.GetDelivery(ctx, store.DeliveryQuery{TenantID: "tenant-a", EventID: event.ID})
	if err != nil || eventGot.EventID != event.ID || eventGot.MessageID != uuid.Nil || eventGot.AttemptCount != 3 ||
		eventGot.Provider != "" || eventGot.ErrorCode != "" || !eventGot.UpdatedAt.Equal(broadcast.UpdatedAt) {
		t.Fatalf("get event summary=%+v err=%v", eventGot, err)
	}
	if _, getErr := repo.GetDelivery(ctx, store.DeliveryQuery{TenantID: "tenant-b", MessageID: message.ID}); !errors.Is(getErr, store.ErrNotFound) {
		t.Fatalf("cross-scope get error=%v", getErr)
	}
	items, hasMore, err := repo.ListDeliveries(ctx, store.DeliveryQuery{
		TenantID: "tenant-a", DefinitionCode: "credential.ready", Channel: "email",
		Status: "failed", ErrorCode: "provider_rejected", Limit: 100,
	})
	if err != nil || hasMore || len(items) != 1 {
		t.Fatalf("list deliveries=%+v hasMore=%v err=%v", items, hasMore, err)
	}
	ambiguous, hasMore, err := repo.ListDeliveries(ctx, store.DeliveryQuery{
		TenantID: "tenant-a", ErrorCode: "secondary_failure", Limit: 100,
	})
	if err != nil || hasMore || len(ambiguous) != 0 {
		t.Fatalf("ambiguous error filter=%+v hasMore=%v err=%v", ambiguous, hasMore, err)
	}
}
