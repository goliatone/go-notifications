# go-notifications Entities Reference

This guide documents the domain models defined in `pkg/domain` and implemented
through the repositories described in `docs/NTF_TDD.md`. Each table is created
via Bun models and wired through go-persistence-bun/go-repository-bun adapters.

## Shared Columns

All tables embed the `BaseModel` struct:

- `id UUID PRIMARY KEY`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `deleted_at TIMESTAMPTZ NULL` (managed via Bun `soft_delete` tag)

## Tables

<!-- markdownlint-disable MD013 -->

| Table | Purpose | Key Columns |
| ------- | ------- | ------------- |
| `notification_definitions` | Master list of notification types | `code` (unique), `name`, `severity`, `channels` (JSON array), `template_keys` (JSON array), `policy` (JSON) |
| `notification_templates` | Channel + locale specific templates | `code`, `channel`, `locale`, `format`, `revision`, `subject`, `body`, `source` (JSON), `schema` (JSON), `metadata` (JSON) |
| `notification_events` | Incoming events awaiting fan-out | `definition_code`, `tenant_id`, `actor_id`, `recipients` and resolved `channels` (JSON arrays), requested `locale`, `context` (JSON), correlation/request IDs, scoped idempotency identity and fingerprint, transient marker, publication/digest references, retry lease, `scheduled_at`, `status` |
| `notification_messages` | Expanded, rendered or redacted message state | `event_id` (FK), `retry_operation_id`, `channel`, durable `template_code` and ordered `provider_plan`, resolved `locale`, content/URL projection, opaque `receiver`, `status`, safe `metadata` (JSON) |
| `notification_delivery_attempts` | Adapter executions per message | `message_id` (FK), `retry_operation_id`, `adapter`, `status`, legacy sanitized `error`, stable `error_code`, safe `payload` (JSON) |
| `notification_preferences` | User/tenant overrides | `subject_type`, `subject_id`, `definition_code`, `channel`, `locale`, `enabled`, `quiet_hours` (JSON), `additional_rules` (JSON) |
| `notification_subscription_groups` | Named cohorts | `code` (unique), `name`, `description`, `metadata` (JSON) |
| `notification_inbox_items` | In-app notification center | `user_id`, `message_id`, `title`, `body`, `locale`, `unread`, `pinned`, `action_url`, `metadata` (JSON), `read_at`, `dismissed_at`, `snoozed_until` |
| `notification_publications` | Durable scheduled/digest queue authority | stable `queue_key`, `kind`, `digest_key`, `run_at`, status, claim lease, attempts, safe error code |
| `notification_retry_operations` | Same-event explicit retry ledger | `event_id`, retry scope/key, correlation/request IDs, status, claim lease, safe error code |

<!-- markdownlint-enable MD013 -->

## Migrations

Production hosts should register the package-owned SQL profile instead of
auto-creating Bun models:

```go
manager := persistence.NewMigrations()
if err := notifications.RegisterMigrations(manager); err != nil {
    return err
}
if err := manager.Migrate(ctx, db); err != nil {
    return err
}
```

`GetMigrationsFS()` returns the embedded tree rooted at
`data/sql/migrations`, with `sqlite/` and `postgres/` dialect directories.
`OrderedMigrationSource()` exposes the source-stable identity
`go-notifications`, default order `50`, package version namespace `001`,
`002`, `003`, and mandatory validation targets `sqlite` and `postgres`. A host
that needs graph dependencies can pass source keys, for example
`OrderedMigrationSource("go-users")`, and register the returned source in its
shared migration graph.

Registration does not execute migrations, connect to a database, or change
module-construction behavior. Re-running the graph through
`go-persistence-bun` is safe because applied versions are recorded. Raw SQL
replay is a different contract: PostgreSQL upgrade statements are guarded
where supported, but SQLite `ALTER TABLE ... ADD COLUMN` statements are not
directly replay-safe.

Migration `003` is intentionally forward-only on PostgreSQL and SQLite.
Removing its durable delivery-plan fields or restoring legacy
`notification_templates.code` uniqueness could make valid rows
unrepresentable. Earlier SQLite upgrade downs also retain added columns
because portable column removal requires table reconstruction. Hosts should
restore from backup when a complete schema rollback is required.

## Repository Coverage

- **Bun adapters** live under `internal/storage/bun` and operate against
  SQLite/PostgreSQL via go-repository-bun.
- **In-memory adapters** in `internal/storage/memory` support test scenarios
  and environments without persistence.
- **Factory wiring** (`pkg/storage`) selects the desired backend and exposes
  repositories through dependency-injection-friendly structs.
- Tests under `internal/storage/bun` may create model tables directly for
  isolated repository checks; that test helper is not the production
  migration contract.
