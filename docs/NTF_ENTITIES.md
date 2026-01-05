# go-notifications Entities Reference

This guide documents the domain models defined in `pkg/domain` and implemented through the repositories described in `docs/NTF_TDD.md`. Each table is created via Bun models and wired through go-persistence-bun/go-repository-bun adapters.

## Shared Columns

All tables embed the `BaseModel` struct:

- `id UUID PRIMARY KEY`
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`
- `deleted_at TIMESTAMPTZ NULL` (managed via Bun `soft_delete` tag)

## Tables

| Table | Purpose | Key Columns |
|-------|---------|-------------|
| `notification_definitions` | Master list of notification types | `code` (unique), `name`, `severity`, `channels` (JSON array), `template_keys` (JSON array), `policy` (JSON) |
| `notification_templates` | Channel + locale specific templates | `code`, `channel`, `locale`, `format`, `revision`, `subject`, `body`, `source` (JSON), `schema` (JSON), `metadata` (JSON) |
| `notification_events` | Incoming events awaiting fan-out | `definition_code`, `tenant_id`, `actor_id`, `recipients` (JSON array), `context` (JSON), `scheduled_at`, `status` |
| `notification_messages` | Expanded, rendered messages | `event_id` (FK), `channel`, `locale`, `subject`, `body`, `action_url`, `manifest_url`, `url` (deprecated), `receiver`, `status`, `metadata` (JSON) |
| `notification_delivery_attempts` | Adapter executions per message | `message_id` (FK), `adapter`, `status`, `error`, `payload` (JSON) |
| `notification_preferences` | User/tenant overrides | `subject_type`, `subject_id`, `definition_code`, `channel`, `locale`, `enabled`, `quiet_hours` (JSON), `additional_rules` (JSON) |
| `notification_subscription_groups` | Named cohorts | `code` (unique), `name`, `description`, `metadata` (JSON) |
| `notification_inbox_items` | In-app notification center | `user_id`, `message_id`, `title`, `body`, `locale`, `unread`, `pinned`, `action_url`, `metadata` (JSON), `read_at`, `dismissed_at`, `snoozed_until` |

## Migrations

The Bun models double as migration sources. Call `persistence.RegisterModel()` with each struct (Definitions, Templates, Events, Messages, Attempts, Preferences, SubscriptionGroups, InboxItems) before initializing the go-persistence-bun client so schema management stays centralized. Tests under `internal/storage/bun` provision sqlite tables directly via `db.NewCreateTable().Model(...)`.

## Repository Coverage

- **Bun adapters** live under `internal/storage/bun` and operate against sqlite/postgres via go-repository-bun.
- **In-memory adapters** in `internal/storage/memory` support test scenarios and environments without persistence.
- **Factory wiring** (`pkg/storage`) selects the desired backend and exposes repositories via dependency-injection friendly structs.
