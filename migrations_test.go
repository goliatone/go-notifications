package notifications_test

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	notifications "github.com/goliatone/go-notifications"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/storage"
	persistence "github.com/goliatone/go-persistence-bun"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func TestSQLiteMigrationsApplyBaselineAndUpgrades(t *testing.T) {
	db, sqldb := newSQLiteMigrationDB(t)
	ctx := context.Background()

	manager := persistence.NewMigrations()
	if err := notifications.RegisterMigrations(manager); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := manager.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := manager.Migrate(ctx, db); err != nil {
		t.Fatalf("repeat migration graph: %v", err)
	}

	schema := map[string][]string{
		"notification_definitions":         {"id", "code", "name", "channels", "metadata", "template_keys", "policy"},
		"notification_templates":           {"id", "code", "channel", "locale", "body", "subject", "source", "schema", "metadata"},
		"notification_events":              {"id", "definition_code", "recipients", "channels", "locale", "context", "correlation_id", "request_id", "idempotency_scope", "idempotency_key", "request_fingerprint", "transient_dependent", "publication_id", "digest_key", "retry_claim_until", "scheduled_at", "status"},
		"notification_messages":            {"id", "event_id", "retry_operation_id", "channel", "template_code", "provider_plan", "receiver", "status", "metadata"},
		"notification_delivery_attempts":   {"id", "message_id", "retry_operation_id", "adapter", "status", "error", "error_code", "payload"},
		"notification_preferences":         {"id", "subject_id", "subject_type", "definition_code", "channel", "enabled"},
		"notification_subscription_groups": {"id", "code", "name", "metadata"},
		"notification_inbox_items":         {"id", "user_id", "message_id", "title", "body", "unread", "action_url"},
		"notification_publications":        {"id", "kind", "digest_key", "queue_key", "run_at", "status", "claim_until", "attempts", "error_code"},
		"notification_retry_operations":    {"id", "event_id", "retry_scope", "idempotency_key", "correlation_id", "request_id", "status", "claim_until", "error_code"},
	}
	for table, columns := range schema {
		assertSQLiteTable(ctx, t, sqldb, table)
		for _, column := range columns {
			assertSQLiteColumn(ctx, t, sqldb, table, column)
		}
	}

	for _, index := range []string{
		"notification_templates_lookup_idx",
		"notification_templates_variant_uidx",
		"notification_messages_event_idx",
		"notification_delivery_attempts_message_idx",
		"notification_preferences_subject_idx",
		"notification_inbox_items_user_idx",
		"notification_events_idempotency_uidx",
		"notification_events_publication_idx",
		"notification_publications_pending_idx",
		"notification_publications_digest_idx",
		"notification_publications_open_digest_uidx",
		"notification_retry_operations_identity_uidx",
		"notification_retry_operations_event_idx",
		"notification_events_retention_idx",
		"notification_messages_retention_idx",
		"notification_delivery_attempts_retention_idx",
		"notification_inbox_items_retention_idx",
		"notification_publications_retention_idx",
		"notification_retry_operations_retention_idx",
		"notification_events_scope_created_idx",
		"notification_events_scope_definition_created_idx",
		"notification_messages_inspection_idx",
		"notification_delivery_attempts_inspection_idx",
	} {
		assertSQLiteIndex(ctx, t, sqldb, index)
	}

	assertSQLiteForeignKey(ctx, t, sqldb, "notification_messages", "event_id", "notification_events")
	assertSQLiteForeignKey(ctx, t, sqldb, "notification_delivery_attempts", "message_id", "notification_messages")
	assertSQLiteForeignKey(ctx, t, sqldb, "notification_inbox_items", "message_id", "notification_messages")
}

func TestSQLiteMigrationsPreservePreUpgradeRecords(t *testing.T) { //nolint:gocyclo,funlen // Linear legacy fixture audit.
	db, sqldb := newSQLiteMigrationDB(t)
	ctx := context.Background()
	root, err := notifications.GetMigrationsFS()
	if err != nil {
		t.Fatalf("migration filesystem: %v", err)
	}
	baseline, err := fs.ReadFile(root, "sqlite/001_notifications_core.up.sql")
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}
	legacyBaseline := strings.Replace(
		string(baseline),
		"UNIQUE (code, channel, locale)",
		"UNIQUE (code)",
		1,
	)
	if _, err := sqldb.ExecContext(ctx, legacyBaseline); err != nil {
		t.Fatalf("apply pre-upgrade schema: %v", err)
	}

	definitionID := uuid.NewString()
	templateID := uuid.NewString()
	eventID := uuid.NewString()
	messageID := uuid.NewString()
	fixtures := []string{
		`INSERT INTO notification_definitions (id, code, name) VALUES ('` + definitionID + `', 'account.notice', 'Account notice')`,
		`INSERT INTO notification_templates (id, code, channel, locale, body) VALUES ('` + templateID + `', 'account.notice', 'email', 'en', 'hello')`,
		`INSERT INTO notification_events (id, definition_code, recipients, context, status) VALUES ('` + eventID + `', 'account.notice', '["subject-1"]', '{"safe":"value"}', 'pending')`,
		`INSERT INTO notification_messages (id, event_id, channel, body, receiver, status) VALUES ('` + messageID + `', '` + eventID + `', 'email', 'hello', 'subject-1', 'pending')`,
		`INSERT INTO notification_delivery_attempts (id, message_id, adapter, status) VALUES ('` + uuid.NewString() + `', '` + messageID + `', 'fake', 'pending')`,
		`INSERT INTO notification_preferences (id, subject_id, subject_type, enabled) VALUES ('` + uuid.NewString() + `', 'subject-1', 'user', 1)`,
		`INSERT INTO notification_subscription_groups (id, code, name) VALUES ('` + uuid.NewString() + `', 'admins', 'Admins')`,
		`INSERT INTO notification_inbox_items (id, user_id, message_id, title, body, unread) VALUES ('` + uuid.NewString() + `', 'subject-1', '` + messageID + `', 'Title', 'Body', 1)`,
	}
	for _, statement := range fixtures {
		if _, err := sqldb.ExecContext(ctx, statement); err != nil {
			t.Fatalf("insert fixture: %v", err)
		}
	}

	manager := persistence.NewMigrations()
	if err := notifications.RegisterMigrations(manager); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := manager.Migrate(ctx, db); err != nil {
		t.Fatalf("upgrade pre-existing schema: %v", err)
	}

	for _, table := range []string{
		"notification_definitions",
		"notification_templates",
		"notification_events",
		"notification_messages",
		"notification_delivery_attempts",
		"notification_preferences",
		"notification_subscription_groups",
		"notification_inbox_items",
	} {
		var count int
		if err := sqldb.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("expected one preserved row in %s, got %d", table, count)
		}
	}
	var body, fingerprint string
	if err := sqldb.QueryRowContext(ctx,
		`SELECT m.body, COALESCE(e.request_fingerprint, '') FROM notification_messages m JOIN notification_events e ON e.id = m.event_id WHERE m.id = ?`,
		messageID,
	).Scan(&body, &fingerprint); err != nil {
		t.Fatalf("read upgraded fixture: %v", err)
	}
	if body != "hello" || fingerprint != "" {
		t.Fatalf("fixture changed during upgrade: body=%q fingerprint=%q", body, fingerprint)
	}
	if _, err := sqldb.ExecContext(ctx,
		`INSERT INTO notification_templates (id, code, channel, locale, body) VALUES (?, 'account.notice', 'sms', 'en', 'hello by sms')`,
		uuid.NewString(),
	); err != nil {
		t.Fatalf("legacy code-only template uniqueness was not repaired: %v", err)
	}
	if _, err := sqldb.ExecContext(ctx,
		`INSERT INTO notification_templates (id, code, channel, body) VALUES (?, 'default.notice', 'email', 'first')`,
		uuid.NewString(),
	); err != nil {
		t.Fatalf("insert default-locale variant: %v", err)
	}
	if _, err := sqldb.ExecContext(ctx,
		`INSERT INTO notification_templates (id, code, channel, body) VALUES (?, 'default.notice', 'email', 'duplicate')`,
		uuid.NewString(),
	); err == nil {
		t.Fatalf("default-locale template identity permitted a duplicate")
	}
}

func TestPostgresLegacyTemplateConstraintUpgrade(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("NOTIFICATIONS_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("NOTIFICATIONS_POSTGRES_DSN is not configured")
	}
	ctx := context.Background()
	sqldb, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	sqldb.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if closeErr := sqldb.Close(); closeErr != nil {
			t.Errorf("close postgres: %v", closeErr)
		}
	})
	schema := "notifications_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, schemaErr := sqldb.ExecContext(ctx, `CREATE SCHEMA `+schema); schemaErr != nil {
		t.Fatalf("create schema: %v", schemaErr)
	}
	t.Cleanup(func() {
		if _, dropErr := sqldb.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); dropErr != nil {
			t.Errorf("drop schema: %v", dropErr)
		}
	})
	if _, searchPathErr := sqldb.ExecContext(ctx, `SET search_path TO `+schema); searchPathErr != nil {
		t.Fatalf("set search path: %v", searchPathErr)
	}
	legacy := `
CREATE TABLE notification_templates (
  id UUID PRIMARY KEY, code TEXT NOT NULL UNIQUE, channel TEXT NOT NULL,
  locale TEXT, body TEXT
);
CREATE TABLE notification_events (id UUID PRIMARY KEY);
CREATE TABLE notification_messages (id UUID PRIMARY KEY);
CREATE TABLE notification_publications (
  id UUID PRIMARY KEY, digest_key TEXT, kind TEXT NOT NULL, queue_key TEXT NOT NULL,
  status TEXT NOT NULL
);
INSERT INTO notification_templates (id, code, channel, locale, body)
VALUES ('` + uuid.NewString() + `', 'account.notice', 'email', 'en', 'hello');`
	if _, legacyErr := sqldb.ExecContext(ctx, legacy); legacyErr != nil {
		t.Fatalf("create legacy schema: %v", legacyErr)
	}
	root, err := notifications.GetMigrationsFS()
	if err != nil {
		t.Fatalf("migration filesystem: %v", err)
	}
	upgrade, err := fs.ReadFile(root, "postgres/003_notification_delivery_integrity.up.sql")
	if err != nil {
		t.Fatalf("read integrity migration: %v", err)
	}
	if _, upgradeErr := sqldb.ExecContext(ctx, string(upgrade)); upgradeErr != nil {
		t.Fatalf("apply integrity migration: %v", upgradeErr)
	}
	if _, err := sqldb.ExecContext(ctx,
		`INSERT INTO notification_templates (id, code, channel, locale, body) VALUES ($1, 'account.notice', 'sms', 'en', 'hello by sms')`,
		uuid.NewString(),
	); err != nil {
		t.Fatalf("legacy code-only uniqueness was not repaired: %v", err)
	}
	if _, err := sqldb.ExecContext(ctx,
		`INSERT INTO notification_publications (id, digest_key, kind, queue_key, status) VALUES ($1, $2, 'digest', $3, 'pending')`,
		uuid.NewString(), strings.Repeat("a", 64), "notification-publication:"+uuid.NewString(),
	); err != nil {
		t.Fatalf("PostgreSQL-safe digest identity was not persisted: %v", err)
	}
	var count int
	if err := sqldb.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notification_templates WHERE code = 'account.notice'`,
	).Scan(&count); err != nil || count != 2 {
		t.Fatalf("legacy templates were not preserved: count=%d err=%v", count, err)
	}
}

func TestOrderedMigrationSourceIdentityIsStable(t *testing.T) {
	source, err := notifications.OrderedMigrationSource("go-users", "host-core")
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	if source.Name != "go-notifications" || source.SourceKey != "go-notifications" || source.Order != 50 {
		t.Fatalf("unexpected source identity: %+v", source)
	}
	if strings.Join(source.DependsOn, ",") != "go-users,host-core" {
		t.Fatalf("unexpected dependencies: %v", source.DependsOn)
	}
	if source.IdentityMode != persistence.OrderedMigrationIdentitySourceStable {
		t.Fatalf("unexpected identity mode: %s", source.IdentityMode)
	}
}

func TestOrderedMigrationSourceSupportsHostSelectedPlacement(t *testing.T) {
	source, err := notifications.OrderedMigrationSourceWithOptions(notifications.MigrationSourceOptions{
		Order: 100, Dependencies: []string{"nice-guys-delivery-users"},
	})
	if err != nil {
		t.Fatalf("configured source: %v", err)
	}
	if source.Name != notifications.MigrationSourceName || source.SourceKey != notifications.MigrationSourceKey ||
		source.Order != 100 || strings.Join(source.DependsOn, ",") != "nice-guys-delivery-users" {
		t.Fatalf("configured source changed stable identity: %+v", source)
	}
	for _, order := range []int{-1, persistence.MaxOrderedMigrationSourceOrder + 1} {
		if _, orderErr := notifications.OrderedMigrationSourceWithOptions(notifications.MigrationSourceOptions{Order: order}); orderErr == nil {
			t.Fatalf("invalid order %d was accepted", order)
		}
	}
	users := persistence.NewStableOrderedMigrationSource(
		"nice-guys-delivery-users", fstest.MapFS{"001_users.up.sql": {Data: []byte("SELECT 1;")}},
		"nice-guys-delivery-users", 90,
	)
	inverted, err := notifications.OrderedMigrationSourceWithOptions(notifications.MigrationSourceOptions{
		Order: 80, Dependencies: []string{"nice-guys-delivery-users"},
	})
	if err != nil {
		t.Fatalf("build inverted source: %v", err)
	}
	manager := persistence.NewMigrations()
	if registerErr := manager.RegisterOrderedMigrationSources(users, inverted); !errors.Is(registerErr, persistence.ErrOrderedSourceOrderInversion) {
		t.Fatalf("expected framework order inversion, got %v", registerErr)
	}
	unknown, err := notifications.OrderedMigrationSourceWithOptions(notifications.MigrationSourceOptions{
		Order: 100, Dependencies: []string{"missing-source"},
	})
	if err != nil {
		t.Fatalf("build unknown-dependency source: %v", err)
	}
	manager = persistence.NewMigrations()
	if registerErr := manager.RegisterOrderedMigrationSources(unknown); !errors.Is(registerErr, persistence.ErrOrderedSourceUnknownDep) {
		t.Fatalf("expected framework unknown dependency, got %v", registerErr)
	}
}

func TestSQLiteCRMOrderedMigrationCompositionIsRepeatableAndDetectsDrift(t *testing.T) {
	db, sqldb := newSQLiteMigrationDB(t)
	ctx := context.Background()
	users := persistence.NewStableOrderedMigrationSource(
		"nice-guys-delivery-users",
		fstest.MapFS{"001_users.up.sql": {Data: []byte("CREATE TABLE crm_users (id TEXT PRIMARY KEY);")}},
		"nice-guys-delivery-users", 90,
	)
	notificationSource, err := notifications.OrderedMigrationSourceWithOptions(notifications.MigrationSourceOptions{
		Order: 100, Dependencies: []string{"nice-guys-delivery-users"},
	})
	if err != nil {
		t.Fatalf("notifications source: %v", err)
	}
	evidence := persistence.NewStableOrderedMigrationSource(
		"crm-notification-evidence",
		fstest.MapFS{"001_evidence.up.sql": {Data: []byte("CREATE TABLE crm_notification_evidence (id TEXT PRIMARY KEY);")}},
		"crm-notification-evidence", 110,
		persistence.WithOrderedMigrationDependencies(notifications.MigrationSourceKey),
	)
	manager := persistence.NewMigrations()
	if registerErr := manager.RegisterOrderedMigrationSources(users, notificationSource, evidence); registerErr != nil {
		t.Fatalf("register CRM graph: %v", registerErr)
	}
	if migrateErr := manager.Migrate(ctx, db); migrateErr != nil {
		t.Fatalf("migrate CRM graph: %v", migrateErr)
	}
	if migrateErr := manager.Migrate(ctx, db); migrateErr != nil {
		t.Fatalf("repeat CRM graph: %v", migrateErr)
	}
	for _, table := range []string{"crm_users", "notification_events", "crm_notification_evidence"} {
		assertSQLiteTable(ctx, t, sqldb, table)
	}

	driftedNotifications, err := notifications.OrderedMigrationSourceWithOptions(notifications.MigrationSourceOptions{
		Order: 101, Dependencies: []string{"nice-guys-delivery-users"},
	})
	if err != nil {
		t.Fatalf("drifted source: %v", err)
	}
	drifted := persistence.NewMigrations()
	if err := drifted.RegisterOrderedMigrationSources(users, driftedNotifications, evidence); err != nil {
		t.Fatalf("register drifted graph: %v", err)
	}
	if err := drifted.Migrate(ctx, db); !errors.Is(err, persistence.ErrOrderedSourceDrift) {
		t.Fatalf("expected graph drift, got %v", err)
	}
}

func TestSQLiteCRMOrderedMigrationCompositionUpgradesExistingUsersJournal(t *testing.T) { //nolint:gocyclo,funlen // Linear upgrade and safety fixture.
	db, sqldb := newSQLiteMigrationDB(t)
	ctx := context.Background()
	users := persistence.NewStableOrderedMigrationSource(
		"nice-guys-delivery-users",
		fstest.MapFS{"001_users.up.sql": {Data: []byte("CREATE TABLE crm_users (id TEXT PRIMARY KEY);")}},
		"nice-guys-delivery-users", 90,
	)
	baseline := persistence.NewMigrations()
	if registerErr := baseline.RegisterOrderedMigrationSources(users); registerErr != nil {
		t.Fatalf("register existing Users graph: %v", registerErr)
	}
	if migrateErr := baseline.Migrate(ctx, db); migrateErr != nil {
		t.Fatalf("migrate existing Users graph: %v", migrateErr)
	}
	var before int
	if err := sqldb.QueryRowContext(ctx, "SELECT COUNT(*) FROM bun_migrations").Scan(&before); err != nil {
		t.Fatalf("inspect existing journal: %v", err)
	}

	notificationSource, err := notifications.OrderedMigrationSourceWithOptions(notifications.MigrationSourceOptions{
		Order: 100, Dependencies: []string{"nice-guys-delivery-users"},
	})
	if err != nil {
		t.Fatalf("notifications source: %v", err)
	}
	evidence := persistence.NewStableOrderedMigrationSource(
		"crm-notification-evidence",
		fstest.MapFS{"001_evidence.up.sql": {Data: []byte("CREATE TABLE crm_notification_evidence (id TEXT PRIMARY KEY);")}},
		"crm-notification-evidence", 110,
		persistence.WithOrderedMigrationDependencies(notifications.MigrationSourceKey),
	)
	upgraded := persistence.NewMigrations()
	if registerErr := upgraded.RegisterOrderedMigrationSources(users, notificationSource, evidence); registerErr != nil {
		t.Fatalf("register upgraded graph: %v", registerErr)
	}
	if migrateErr := upgraded.Migrate(ctx, db); !errors.Is(migrateErr, persistence.ErrOrderedSourceDrift) {
		t.Fatalf("expected explicit graph repair gate, got %v", migrateErr)
	}
	if adoptionErr := notifications.AdoptAdditiveOrderedMigrationGraph(ctx, db, baseline); !errors.Is(adoptionErr, notifications.ErrUnsafeMigrationGraphAdoption) {
		t.Fatalf("expected no-op graph adoption rejection, got %v", adoptionErr)
	}
	unsafePrefix := persistence.NewStableOrderedMigrationSource(
		"unsafe-prefix",
		fstest.MapFS{"001_prefix.up.sql": {Data: []byte("CREATE TABLE unsafe_prefix (id TEXT PRIMARY KEY);")}},
		"unsafe-prefix", 80,
	)
	unsafeGraph := persistence.NewMigrations()
	if registerErr := unsafeGraph.RegisterOrderedMigrationSources(unsafePrefix, users); registerErr != nil {
		t.Fatalf("register unsafe reordered graph: %v", registerErr)
	}
	if adoptionErr := notifications.AdoptAdditiveOrderedMigrationGraph(ctx, db, unsafeGraph); !errors.Is(adoptionErr, notifications.ErrUnsafeMigrationGraphAdoption) {
		t.Fatalf("expected reordered graph adoption rejection, got %v", adoptionErr)
	}
	if repairErr := notifications.AdoptAdditiveOrderedMigrationGraph(ctx, db, upgraded); repairErr != nil {
		t.Fatalf("repair existing graph identity: %v", repairErr)
	}
	var afterAdoption int
	if err := sqldb.QueryRowContext(ctx, "SELECT COUNT(*) FROM bun_migrations").Scan(&afterAdoption); err != nil {
		t.Fatalf("inspect adopted journal: %v", err)
	}
	if afterAdoption != before {
		t.Fatalf("graph adoption rewrote migration journal: before=%d after=%d", before, afterAdoption)
	}
	if migrateErr := upgraded.Migrate(ctx, db); migrateErr != nil {
		t.Fatalf("upgrade existing graph: %v", migrateErr)
	}
	if migrateErr := upgraded.Migrate(ctx, db); migrateErr != nil {
		t.Fatalf("repeat upgraded graph: %v", migrateErr)
	}
	for _, table := range []string{"crm_users", "notification_events", "crm_notification_evidence"} {
		assertSQLiteTable(ctx, t, sqldb, table)
	}
	var after int
	if err := sqldb.QueryRowContext(ctx, "SELECT COUNT(*) FROM bun_migrations").Scan(&after); err != nil {
		t.Fatalf("inspect upgraded journal: %v", err)
	}
	if after <= before {
		t.Fatalf("upgrade did not preserve and extend journal: before=%d after=%d", before, after)
	}
}

func TestPostgresCRMOrderedMigrationCompositionUpgradesExistingUsersJournal(t *testing.T) { //nolint:gocyclo // Linear optional PostgreSQL graph fixture.
	dsn := strings.TrimSpace(os.Getenv("NOTIFICATIONS_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("NOTIFICATIONS_POSTGRES_DSN is not configured")
	}
	ctx := context.Background()
	adminDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	schema := "notifications_crm_graph_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, schemaErr := adminDB.ExecContext(ctx, `CREATE SCHEMA `+schema); schemaErr != nil {
		t.Fatalf("create schema: %v", schemaErr)
	}
	t.Cleanup(func() {
		if _, dropErr := adminDB.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); dropErr != nil {
			t.Errorf("drop schema: %v", dropErr)
		}
		if closeErr := adminDB.Close(); closeErr != nil {
			t.Errorf("close postgres: %v", closeErr)
		}
	})
	sqldb, err := sql.Open("postgres", postgresDSNWithSearchPath(dsn, schema))
	if err != nil {
		t.Fatalf("open schema database: %v", err)
	}
	sqldb.SetMaxOpenConns(1)
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("close schema database: %v", closeErr)
		}
	})
	users := persistence.NewStableOrderedMigrationSource(
		"nice-guys-delivery-users",
		fstest.MapFS{"001_users.up.sql": {Data: []byte("CREATE TABLE crm_users (id UUID PRIMARY KEY);")}},
		"nice-guys-delivery-users", 90,
	)
	baseline := persistence.NewMigrations()
	if registerErr := baseline.RegisterOrderedMigrationSources(users); registerErr != nil {
		t.Fatalf("register Users graph: %v", registerErr)
	}
	if migrateErr := baseline.Migrate(ctx, db); migrateErr != nil {
		t.Fatalf("migrate Users graph: %v", migrateErr)
	}
	notificationSource, err := notifications.OrderedMigrationSourceWithOptions(notifications.MigrationSourceOptions{
		Order: 100, Dependencies: []string{"nice-guys-delivery-users"},
	})
	if err != nil {
		t.Fatalf("notifications source: %v", err)
	}
	evidence := persistence.NewStableOrderedMigrationSource(
		"crm-notification-evidence",
		fstest.MapFS{"001_evidence.up.sql": {Data: []byte("CREATE TABLE crm_notification_evidence (id UUID PRIMARY KEY);")}},
		"crm-notification-evidence", 110,
		persistence.WithOrderedMigrationDependencies(notifications.MigrationSourceKey),
	)
	upgraded := persistence.NewMigrations()
	if registerErr := upgraded.RegisterOrderedMigrationSources(users, notificationSource, evidence); registerErr != nil {
		t.Fatalf("register upgraded graph: %v", registerErr)
	}
	if migrateErr := upgraded.Migrate(ctx, db); !errors.Is(migrateErr, persistence.ErrOrderedSourceDrift) {
		t.Fatalf("expected explicit graph adoption gate, got %v", migrateErr)
	}
	if adoptionErr := notifications.AdoptAdditiveOrderedMigrationGraph(ctx, db, upgraded); adoptionErr != nil {
		t.Fatalf("adopt upgraded graph: %v", adoptionErr)
	}
	if migrateErr := upgraded.Migrate(ctx, db); migrateErr != nil {
		t.Fatalf("migrate upgraded graph: %v", migrateErr)
	}
	if migrateErr := upgraded.Migrate(ctx, db); migrateErr != nil {
		t.Fatalf("repeat upgraded graph: %v", migrateErr)
	}
	for _, table := range []string{"crm_users", "notification_events", "crm_notification_evidence"} {
		var name sql.NullString
		if err := sqldb.QueryRowContext(ctx, "SELECT to_regclass($1)", table).Scan(&name); err != nil || !name.Valid {
			t.Fatalf("expected table %s: name=%v err=%v", table, name, err)
		}
	}
}

func TestMigrationDialectsHaveVersionParity(t *testing.T) {
	root, err := notifications.GetMigrationsFS()
	if err != nil {
		t.Fatalf("migration filesystem: %v", err)
	}
	versions := make(map[string][]string)
	for _, dialect := range []string{"postgres", "sqlite"} {
		entries, err := fs.ReadDir(root, dialect)
		if err != nil {
			t.Fatalf("read %s migrations: %v", dialect, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
				continue
			}
			version, _, _ := strings.Cut(entry.Name(), "_")
			versions[dialect] = append(versions[dialect], version)
		}
		sort.Strings(versions[dialect])
	}
	if strings.Join(versions["postgres"], ",") != strings.Join(versions["sqlite"], ",") {
		t.Fatalf("migration version mismatch: postgres=%v sqlite=%v", versions["postgres"], versions["sqlite"])
	}
	if strings.Join(versions["sqlite"], ",") != "001,002,003,004,005" {
		t.Fatalf("unexpected released migration versions: %v", versions["sqlite"])
	}
}

func TestSQLiteBunScopedIdempotencyIsAtomic(t *testing.T) {
	db, sqldb := newSQLiteMigrationDB(t)
	sqldb.SetMaxOpenConns(1)
	ctx := context.Background()
	manager := persistence.NewMigrations()
	if err := notifications.RegisterMigrations(manager); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := manager.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := storage.NewBunProviders(db).Events

	const callers = 20
	var wg sync.WaitGroup
	var mu sync.Mutex
	createdCount := 0
	ids := make(map[uuid.UUID]struct{})
	errs := make(chan error, callers)
	for range callers {
		wg.Go(func() {
			stored, created, err := repo.CreateIdempotent(ctx, &domain.NotificationEvent{
				DefinitionCode:     "account.notice",
				IdempotencyScope:   "tenant:one",
				IdempotencyKey:     "Key-A",
				RequestFingerprint: "fingerprint",
			})
			if err != nil {
				errs <- err
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if created {
				createdCount++
			}
			ids[stored.ID] = struct{}{}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		root := err
		for unwrapped := errors.Unwrap(root); unwrapped != nil; unwrapped = errors.Unwrap(root) {
			root = unwrapped
		}
		t.Errorf("concurrent create: %v (root: %T %v)", err, root, root)
	}
	if createdCount != 1 || len(ids) != 1 {
		t.Fatalf("expected one created event ID, created=%d ids=%v", createdCount, ids)
	}

	var eventID uuid.UUID
	for id := range ids {
		eventID = id
	}
	if err := repo.SoftDelete(ctx, eventID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	replayed, created, err := repo.CreateIdempotent(ctx, &domain.NotificationEvent{
		DefinitionCode:     "account.notice",
		IdempotencyScope:   "tenant:one",
		IdempotencyKey:     "Key-A",
		RequestFingerprint: "fingerprint",
	})
	if err != nil || created || replayed.ID != eventID {
		t.Fatalf("soft-deleted Bun identity was reused: replayed=%+v created=%v err=%v", replayed, created, err)
	}
}

func newSQLiteMigrationDB(t *testing.T) (*bun.DB, *sql.DB) {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	sqldb, err := sql.Open(sqliteshim.DriverName(), dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := bun.NewDB(sqldb, sqlitedialect.New())
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("close database: %v", closeErr)
		}
	})
	return db, sqldb
}

func assertSQLiteTable(ctx context.Context, t *testing.T, db *sql.DB, table string) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
	).Scan(&count); err != nil {
		t.Fatalf("inspect table %s: %v", table, err)
	}
	if count != 1 {
		t.Fatalf("expected table %s", table)
	}
}

func assertSQLiteIndex(ctx context.Context, t *testing.T, db *sql.DB, index string) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, index,
	).Scan(&count); err != nil {
		t.Fatalf("inspect index %s: %v", index, err)
	}
	if count != 1 {
		t.Fatalf("expected index %s", index)
	}
}

func assertSQLiteForeignKey(ctx context.Context, t *testing.T, db *sql.DB, table, column, target string) {
	t.Helper()
	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_list(`+table+`)`)
	if err != nil {
		t.Fatalf("foreign keys for %s: %v", table, err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			t.Errorf("close foreign key rows: %v", closeErr)
		}
	}()
	for rows.Next() {
		var id, seq int
		var referencedTable, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &referencedTable, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			t.Fatalf("scan foreign key for %s: %v", table, err)
		}
		if from == column && referencedTable == target {
			return
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate foreign keys for %s: %v", table, err)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate foreign keys for %s: %v", table, err)
	}
	t.Fatalf("expected %s.%s to reference %s", table, column, target)
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
