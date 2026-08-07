// Package adoption contains compiling examples for the v0.16 CRM adoption
// APIs. Authorization, retention scheduling, and host migration sources remain
// application concerns.
package adoption

import (
	"context"
	"errors"
	"time"

	notifications "github.com/goliatone/go-notifications"
	"github.com/goliatone/go-notifications/pkg/deliveries"
	"github.com/goliatone/go-notifications/pkg/events"
	"github.com/goliatone/go-notifications/pkg/notifier"
	"github.com/goliatone/go-notifications/pkg/retention"
	persistence "github.com/goliatone/go-persistence-bun"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// RegisterAfterUsers places notifications between a host Users source at
// order 90 and a later host evidence source.
func RegisterAfterUsers(manager *persistence.Migrations) error {
	return notifications.RegisterMigrationsWithOptions(manager, notifications.MigrationSourceOptions{
		Order:        100,
		Dependencies: []string{"nice-guys-delivery-users"},
	})
}

// MigrateAdditiveGraph handles only an explicitly reviewed suffix expansion
// of an existing source-stable graph. It does not repair reordered sources.
func MigrateAdditiveGraph(ctx context.Context, db *bun.DB, manager *persistence.Migrations) error {
	err := manager.Migrate(ctx, db)
	if !errors.Is(err, persistence.ErrOrderedSourceDrift) {
		return err
	}
	if err := notifications.AdoptAdditiveOrderedMigrationGraph(ctx, db, manager); err != nil {
		return err
	}
	return manager.Migrate(ctx, db)
}

// RecoverCredentialReceipt reconstructs durable state without redispatching
// or requiring the original transient credential URL.
func RecoverCredentialReceipt(ctx context.Context, mod *notifier.Module, tenantID, key string) (events.DispatchReceipt, error) {
	return mod.LookupReceipt(ctx, events.ReceiptLookup{
		DefinitionCode:   "credential-issued",
		IdempotencyScope: "tenant:" + tenantID,
		IdempotencyKey:   key,
	})
}

// InspectDelivery returns only the package's fixed metadata projection.
func InspectDelivery(ctx context.Context, mod *notifier.Module, tenantID string, eventID uuid.UUID) (deliveries.View, error) {
	return mod.Deliveries().Get(ctx, deliveries.GetQuery{
		Scope:   "tenant:" + tenantID,
		EventID: eventID,
	})
}

// PurgeTerminalRecords runs one bounded pass through the typed command used by
// both manual and scheduled host entry points.
func PurgeTerminalRecords(ctx context.Context, mod *notifier.Module, cutoff time.Time) (retention.Result, error) {
	return mod.Commands().PurgeRetention.Run(ctx, retention.Request{
		EventsBefore:          cutoff,
		MessagesBefore:        cutoff,
		AttemptsBefore:        cutoff,
		InboxBefore:           cutoff,
		PublicationsBefore:    cutoff,
		RetryOperationsBefore: cutoff,
		BatchSize:             500,
	})
}
