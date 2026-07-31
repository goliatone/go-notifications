# go-notifications Secrets & Credential Management - Technical Design Document

## Table of Contents
1. [Overview](#overview)
2. [Goals & Non-Goals](#goals--non-goals)
3. [Design Principles](#design-principles)
4. [Core Requirements](#core-requirements)
5. [Architecture](#architecture)
6. [Data Model](#data-model)
7. [Interfaces](#interfaces)
8. [Providers](#providers)
9. [Masking & Logging](#masking--logging)
10. [Lifecycle & Rotations](#lifecycle--rotations)
11. [Integration Points](#integration-points)
12. [Testing & Tooling](#testing--tooling)
13. [Risks & Mitigations](#risks--mitigations)
14. [Open Questions](#open-questions)
15. [Example Updates](#example-updates)

## Overview

This document defines the secret/credential abstraction for the module. The goal is to support **per-user/tenant channel credentials** (e.g., Slack bot tokens, Telegram chat IDs, email/API keys, webhook URLs) while keeping transport-agnostic code and avoiding secret leakage. Secrets must be injectable, encrypted at rest when stored locally, and pluggable for external secret managers.

## Goals & Non-Goals

### Goals
- Provide a `SecretsProvider` abstraction usable by the dispatcher/adapters to resolve per-recipient channel credentials and addresses at send time.
- Support multiple backends: in-memory/static (dev), encrypted DB (default), and external secret managers (Vault/KMS/ASM/GSM).
- Keep adapters stateless w.r.t. secrets; inject secrets per send instead of embedding them in adapter config.
- Mask secrets in logs/errors consistently (reuse `../go-masker`).
- Backward compatibility: global env/config remains as a default when no scoped secret exists.
- **Per-subject fallback only**: Config/env defaults must be opt-in per subject (system/admin/test users). Never broadcast a single global token to all users/tenants by default.

### Non-Goals
- Build a full secret UI. Only the contracts and storage hooks are covered; UI/transport will live in go-admin/example apps.
- Replace host-wide secret management policies; we only integrate with them.

## Design Principles
- **Least privilege**: store only what each channel needs; avoid over-scoped tokens.
- **Pluggable**: drop-in providers; no direct dependency on a specific secret manager at call sites.
- **Ephemeral handling**: secrets are hydrated at dispatch time and not persisted to messages/attempts.
- **Secure by default**: local storage is encrypted; logs are masked.
- **Separation of concerns**: addresses (where to send) are data; secrets (how to auth) are handled by providers.

## Core Requirements
- Resolve secrets by scope (system/tenant/user) + channel + provider.
- Encrypt at rest for DB-backed storage.
- Support binary/opaque blobs (API keys, tokens, certs, JSON credentials).
- Handle rotation (set version, read latest, optionally fetch by version).
- Mask when logging or formatting errors.
- Backward-compatible fallback to global adapter config/env.
- Fallback must be gated per subject (explicit allowlist or feature flag); default behavior is to **not** use global creds for arbitrary users/tenants.

## Architecture

```
Notifier / Dispatcher
    |
    | resolve secrets + addresses
    v
SecretsProvider (interface)
    |--- StaticProvider (dev/demo)
    |--- EncryptedStoreProvider (DB + envelope encryption)
    |--- VaultProvider / AWS SM / GCP SM (external)

Adapters (stateless)
    <- injected secrets (token/from/webhook_url/chat_id) per send
```

Flow:
1. Dispatcher receives event → expands messages per recipient/channel.
2. For each delivery job, it resolves:
   - Address (email/phone/chat_id) from profile/contact store.
   - Secrets via `SecretsProvider.Resolve(scope, channel, provider, keys...)`.
3. Builds adapter message with injected secrets (not persisted).
4. Sends; logs masked fields using go-masker.

## Data Model

Logical secret reference (not persisted as-is):
- `Scope`: `system` | `tenant` | `user`
- `SubjectID`: tenant ID or user ID (or `default`)
- `Channel`: e.g., `chat`, `email`, `sms`, `webhook`
- `Provider`: e.g., `slack`, `telegram`, `sendgrid`, `mailgun`
- `Key`: e.g., `token`, `chat_id`, `from`, `webhook_url`, `api_key`, `client_secret`
- `Version` (optional): for rotation/history

Encrypted storage record (for the DB provider):
- `scope`, `subject_id`, `channel`, `provider`, `key`
- `cipher_text` (AEAD), `nonce`, `version`, `created_at`, `updated_at`
- `metadata` (non-sensitive hints; e.g., last-rotated-at)

## Interfaces

```go
type Scope string // system|tenant|user

type Reference struct {
    Scope     Scope
    SubjectID string
    Channel   string
    Provider  string
    Key       string
    Version   string // optional
}

type SecretValue struct {
    Data      []byte
    Version   string
    Retrieved time.Time
}

type Provider interface {
    Get(ctx context.Context, ref Reference) (SecretValue, error)
    Put(ctx context.Context, ref Reference, value []byte) (string, error) // returns version
    Delete(ctx context.Context, ref Reference) error
    Describe(ctx context.Context, ref Reference) (map[string]any, error) // non-sensitive metadata
}
```

Dispatcher/adapters will receive a resolver:

```go
type Resolver interface {
    Resolve(ctx context.Context, refs ...Reference) (map[string]SecretValue, error)
}
```

## Providers

- **StaticProvider**: map-based, for tests/demo; no encryption.
- **EncryptedStoreProvider (default)**:
  - Persists ciphertext in a repository (Bun adapter).
  - Uses AEAD (e.g., XChaCha20-Poly1305) with a key from env/KMS.
  - Optional envelope decryption via KMS; key IDs stored with the record.
  - Supports versioning by writing new rows; `Get` returns latest when `Version` is empty.
- **VaultProvider / AWS Secrets Manager / GCP Secret Manager**:
  - Translate `Reference` → secret path/ARN; rely on external rotation/audit.
  - Return bytes; no local encryption.
- **NopProvider**: returns `ErrNotFound`, enabling fallback to global config/env.

## Masking & Logging

- Use `../go-masker` to mask known fields before logging/returning errors.
- Add helper: `MaskSecrets(map[string]SecretValue) map[string]any` for safe log fields.
- Ensure adapters never log raw tokens/chat_ids/webhook URLs.
- Repository debug logs must not print ciphertext or plaintext; log only key/ scope/ provider/ version.

## Lifecycle & Rotations

- **Put** writes new version; **Get** without version fetches latest.
- Rotation strategy:
  - Write new version.
  - Optionally keep old version for rollback (configurable retention).
  - Provide `Describe` to expose `last_rotated_at` (non-sensitive).
- Deletion: soft-delete flag for DB provider; hard delete optional via `Delete`.

## Integration Points

- **Notifier/Dispatcher**: add `Secrets Provider` to dependencies; when nil, fall back to config/env.
- **Contact/Address resolution**: separate from secrets; typically from go-users profile or a dedicated contact store.
- **Adapters**: accept per-send overrides (token, from, chat_id, webhook_url) so secrets are not baked into adapter config.
- **Preferences**: can drive provider selection (e.g., `chat:telegram` vs `chat:slack`) while secrets supply credentials.
- **go-masker**: used in logging helpers and any user-facing error propagation.

## Testing & Tooling

- Unit tests for provider behaviors (Get/Put/Delete, versioning, masking).
- Integration tests for EncryptedStoreProvider with AEAD encryption using a test key.
- Fakes for StaticProvider in examples/demo.
- Lint rule/checklist: no logging of `SecretValue.Data`, only masked summaries.

## Risks & Mitigations

- **Secret leakage via logs**: mitigated by go-masker, masked logging helpers, and review of adapter log fields.
- **Key management complexity**: provide AEAD with env key as default; allow KMS envelope to improve security when available.
- **Partial adoption**: keep backward-compatible env/config fallback; emit warnings when routing uses global secrets in multi-tenant mode.
- **Version drift**: encourage rotation metadata and optional validation hooks.
- **Global credential misuse**: ensure env/config fallback is opt-in per subject (system/admin/test accounts). Default behavior should reject delivery when no scoped secret is present for a user/tenant.

## Open Questions

- Should we expose a CLI/admin API for rotations and backfills (likely via go-admin)?
- Do we need RBAC on secret keys per subject/provider (map to go-auth policies)?
- Should we add rate limiting on `Get` when using external managers to avoid throttling?

## Storage Schema Notes

When using the built-in encrypted store provider, secrets are persisted in a `secrets` table with soft deletes:

| column      | type    | notes                                      |
|-------------|---------|--------------------------------------------|
| id          | int PK  | autoincrement primary key                  |
| scope       | text    | system/tenant/user                         |
| subject_id  | text    | tenant ID or user ID                       |
| channel     | text    | logical channel (chat/email/sms/webhook)   |
| provider    | text    | adapter/provider name (slack/sendgrid/etc) |
| key         | text    | token/api_key/client_secret/etc.           |
| version     | text    | version marker (RFC3339 ts recommended)    |
| cipher      | blob    | encrypted payload                          |
| nonce       | blob    | AEAD nonce                                 |
| metadata    | jsonb   | non-sensitive hints (created_at, rotated)  |
| created_at  | timestamptz | default now()                          |
| updated_at  | timestamptz | default now()                          |
| deleted_at  | timestamptz | soft delete                             |

Unique constraint: (scope, subject_id, channel, provider, key, version).

For external secret managers, this schema is not used; the provider maps `Reference` → external path/ARN instead.

### Migrations & fixtures
- Bun schema lives in `internal/storage/bun/secrets.go` with the unique constraint enforced via `secret_identity`. Keep that index aligned with any external migration.
- Tests/fixtures create the table with `db.NewCreateTable().Model((*secretRecord)(nil)).IfNotExists()` (see `internal/storage/bun/secrets_test.go`); reuse that snippet for bootstrap CLIs or migrations.
- SQLite demo runs fine with the same schema; for disk-backed examples, point Bun at `file:tmp/demo.db` (or your path) and call `NewCreateTable` during startup to auto-create the table when missing.
- When adding migrations elsewhere, include the `metadata JSONB` column (nullable), and keep `deleted_at` as a soft delete so historic versions remain listable.

## Example Updates

To demonstrate per-user channel preferences and persistent storage in the demo app:

- Persist everything to SQLite on disk instead of in-memory:
  - Configure `examples/web` to point Bun at `./tmp/demo.db` (or `./tmp/example.sqlite`).
  - Ensure migrations for definitions/templates/messages/preferences/inbox run at startup (or auto-create schemas when missing).
- Channel preferences and provider choice:
  - Extend the example profiles/preferences UI to let a user pick channels/providers per definition (e.g., `chat:slack`, `chat:telegram`, `email:sendgrid`, `sms:twilio`).
  - Store channel enablement and preferred provider in the preferences store (using `go-options`/`go-preferences`) and feed that into dispatcher routing instead of a hardcoded channel list.
- Adapter wiring:
  - Keep global env vars as defaults, but resolve per-user addresses/credentials when sending:
    - Slack: map user → Slack user ID; use bot token from secrets provider.
    - Telegram: map user → chat_id; use token from secrets provider.
    - Email/SMS: map user → address/phone + provider API key from secrets provider.
  - Inject resolved secrets/addresses into adapter calls per message; do not persist them.
- Seed data:
  - Include sample user preferences selecting different providers for `chat` to illustrate `chat:slack` vs `chat:telegram`.
  - Seed a few contact methods (email/phone/chat_id) per demo user so multi-channel delivery works out of the box.
- Observability:
  - Add a simple “last delivery” view in the example showing which provider/channel was used and whether preferences forced a fallback to inbox.

## Admin/CLI hooks (rotation/backfill)

- Minimal rotation helper can wrap `Provider.Put` to write a new version while leaving the old one in place for rollback:

```go
// assumes Bun DB + EncryptedStoreProvider
store := bunrepo.NewSecretStore(db)
provider, _ := secrets.NewEncryptedStoreProvider(store, yourKeyBytes)
ref := secrets.Reference{Scope: secrets.ScopeUser, SubjectID: "u-123", Channel: "chat", Provider: "slack", Key: "token"}
_, _ = provider.Put(ref, []byte(newToken)) // version auto-stamps RFC3339Nano
```

- Backfill task (e.g., wired into go-admin) can list scoped contacts → compose `Reference` → call `Put`/`Delete` accordingly; log using `secrets.MaskValues` to avoid leaking payloads.
