package notifications_test

import (
	"context"
	"database/sql"
	"testing"

	notifications "github.com/goliatone/go-notifications"
	persistence "github.com/goliatone/go-persistence-bun"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func TestSQLiteMigrationsApplyBaselineAndUpgrades(t *testing.T) {
	sqldb, err := sql.Open(sqliteshim.DriverName(), "file:migrations?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("close database: %v", closeErr)
		}
	})
	ctx := context.Background()

	manager := persistence.NewMigrations()
	if err := notifications.RegisterMigrations(manager); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := manager.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	for _, table := range []string{
		"notification_definitions", "notification_templates", "notification_events",
		"notification_messages", "notification_delivery_attempts", "notification_preferences",
		"notification_subscription_groups", "notification_inbox_items",
		"notification_publications", "notification_retry_operations",
	} {
		var count int
		if err := sqldb.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
		).Scan(&count); err != nil {
			t.Fatalf("inspect table %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("expected table %s", table)
		}
	}

	assertSQLiteColumn(ctx, t, sqldb, "notification_events", "request_fingerprint")
	assertSQLiteColumn(ctx, t, sqldb, "notification_events", "retry_claim_until")
	assertSQLiteColumn(ctx, t, sqldb, "notification_delivery_attempts", "error_code")
}

func TestOrderedMigrationSourceIdentityIsStable(t *testing.T) {
	source, err := notifications.OrderedMigrationSource()
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	if source.Name != "go-notifications" || source.SourceKey != "go-notifications" || source.Order != 50 {
		t.Fatalf("unexpected source identity: %+v", source)
	}
}

func assertSQLiteColumn(ctx context.Context, t *testing.T, db *sql.DB, table, column string) {
	t.Helper()
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		t.Fatalf("table info %s: %v", table, err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			t.Errorf("close table info rows: %v", closeErr)
		}
	}()
	for rows.Next() {
		var cid int
		var name, kind string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan table info %s: %v", table, err)
		}
		if name == column {
			return
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table info %s: %v", table, err)
	}
	t.Fatalf("expected column %s.%s", table, column)
}
