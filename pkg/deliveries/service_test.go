package deliveries

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/privacy"
	"github.com/goliatone/go-notifications/pkg/storage"
	"github.com/google/uuid"
)

func TestMemoryDeliveryInspectionIsScopedPaginatedAndMetadataOnly(t *testing.T) { //nolint:gocyclo // Linear privacy fixture audit.
	ctx := context.Background()
	providers := storage.NewMemoryProviders()
	service, err := New(providers.DeliveryQueries)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	created := time.Now().UTC().Add(-time.Hour)
	var tenantAMessages []uuid.UUID
	for idx, tenant := range []string{"tenant-a", "tenant-a", "tenant-b"} {
		event := &domain.NotificationEvent{
			RecordMeta:     domain.RecordMeta{CreatedAt: created.Add(time.Duration(idx) * time.Minute)},
			DefinitionCode: "credential.ready", TenantID: tenant,
			CorrelationID: "correlation-" + tenant, Recipients: domain.StringList{"private@example.test"},
			Context: domain.JSONMap{"secret": "credential-token"}, Status: domain.EventStatusProcessed,
		}
		if createErr := providers.Events.Create(ctx, event); createErr != nil {
			t.Fatalf("create event: %v", createErr)
		}
		message := &domain.NotificationMessage{
			RecordMeta: domain.RecordMeta{CreatedAt: event.CreatedAt}, EventID: event.ID,
			Channel: "email", Receiver: "private@example.test", Subject: "Secret subject",
			Body: "credential-token", Status: domain.MessageStatusDelivered,
			Metadata: domain.JSONMap{"provider_response": "raw-response"},
		}
		if createErr := providers.Messages.Create(ctx, message); createErr != nil {
			t.Fatalf("create message: %v", createErr)
		}
		if tenant == "tenant-a" {
			tenantAMessages = append(tenantAMessages, message.ID)
		}
		attempt := &domain.DeliveryAttempt{
			MessageID: message.ID, Adapter: "safe-provider", Status: domain.AttemptStatusSucceeded,
			Error: "raw provider error private@example.test", ErrorCode: "", Payload: domain.JSONMap{"secret": "raw-response"},
		}
		if createErr := providers.DeliveryAttempts.Create(ctx, attempt); createErr != nil {
			t.Fatalf("create attempt: %v", createErr)
		}
	}

	first, err := service.List(ctx, ListQuery{Scope: "tenant:tenant-a", Limit: 1})
	if err != nil || len(first.Items) != 1 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page=%+v err=%v", first, err)
	}
	second, err := service.List(ctx, ListQuery{Scope: "tenant:tenant-a", Limit: 1, Cursor: first.NextCursor})
	if err != nil || len(second.Items) != 1 || second.HasMore || second.NextCursor != "" {
		t.Fatalf("second page=%+v err=%v", second, err)
	}
	if first.Items[0].MessageID == second.Items[0].MessageID {
		t.Fatalf("cursor repeated a delivery")
	}

	view, err := service.Get(ctx, GetQuery{Scope: "tenant:tenant-a", MessageID: tenantAMessages[0]})
	if err != nil || view.MessageID != tenantAMessages[0] || view.Provider != "safe-provider" || view.AttemptCount != 1 {
		t.Fatalf("get view=%+v err=%v", view, err)
	}
	body, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal view: %v", err)
	}
	serialized := string(body)
	for _, forbidden := range []string{"private@example.test", "credential-token", "Secret subject", "raw provider error", "raw-response", "receiver", "body", "subject", "metadata", "payload"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("safe projection leaked %q: %s", forbidden, serialized)
		}
	}
	_, err = service.Get(ctx, GetQuery{Scope: "tenant:tenant-b", MessageID: tenantAMessages[0]})
	var safe privacy.SafeError
	if !errors.As(err, &safe) || safe.Code != "delivery_not_found" || errors.Unwrap(err) != nil {
		t.Fatalf("scope denial was not a safe not-found: %#v", err)
	}
	filtered, err := service.List(ctx, ListQuery{
		Scope: "tenant:tenant-a", DefinitionCode: "credential.ready", Channel: "email",
		Status: "DELIVERED", CreatedAfter: created.Add(-time.Minute), CreatedBefore: created.Add(2 * time.Minute),
	})
	if err != nil || len(filtered.Items) != 2 {
		t.Fatalf("filtered page=%+v err=%v", filtered, err)
	}
}

func TestDeliveryInspectionValidatesIdentityCursorAndPageBounds(t *testing.T) {
	service, err := New(&deliveryRepoStub{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	for _, query := range []GetQuery{{Scope: "system"}, {Scope: "system", EventID: uuid.New(), MessageID: uuid.New()}, {EventID: uuid.New()}} {
		if _, err := service.Get(context.Background(), query); err == nil {
			t.Fatalf("invalid get query passed: %+v", query)
		}
	}
	for _, query := range []ListQuery{{Scope: "system", Limit: -1}, {Scope: "system", Limit: MaxPageSize + 1}, {Scope: "system", Cursor: "not-base64"}, {}} {
		if _, err := service.List(context.Background(), query); err == nil {
			t.Fatalf("invalid list query passed: %+v", query)
		}
	}
}

type deliveryRepoStub struct{}

func (*deliveryRepoStub) GetDelivery(context.Context, store.DeliveryQuery) (store.DeliveryRecord, error) {
	return store.DeliveryRecord{}, store.ErrNotFound
}

func (*deliveryRepoStub) ListDeliveries(context.Context, store.DeliveryQuery) ([]store.DeliveryRecord, bool, error) {
	return nil, false, nil
}
