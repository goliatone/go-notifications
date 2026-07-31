# Secrets & Credential Management Implementation Plan

Roadmap aligned with `docs/SEC_TDD.md` to deliver pluggable, per-recipient secret handling (tokens/chat IDs/from addresses/webhook URLs) with masked logging and storage options.

## Guiding Notes
- Follow the contracts in `docs/SEC_TDD.md` (providers, scope/key model, masking).
- Keep backward compatibility: global env/config remain defaults when a scoped secret is absent.
- Fallback to env/config must be **explicit per subject** (system/admin/test users). Do not broadcast a single global token to arbitrary users or tenants.
- Never log raw secrets; use go-masker helpers for masking.
- Adapters stay stateless; secrets/addresses are injected per send.
- SQLite-backed example should persist messages/preferences/contacts so routing decisions survive restarts.
- if you need to execute go code use the full path to the go binary: "/Users/goliatone/.g/go/bin/go"

---

## Phase 0. Bootstrap & Interfaces
**Why**: Establish the secret abstraction and common helpers.

**Tasks**
- [x] Task 0.1 – Define `pkg/secrets` interfaces (`Reference`, `SecretValue`, `Provider`, `Resolver`) and `Scope` enums (system/tenant/user) per `SEC_TDD`.
- [x] Task 0.2 – Add masking helpers using `../go-masker` (mask map fields; redact known keys like token/api_key/chat_id/webhook_url).
- [x] Task 0.3 – Add minimal errors (`ErrNotFound`, `ErrUnauthorized`, `ErrInvalidScope`) and shared validation helpers for refs.
- [x] Task 0.4 – Document the interface in `pkg/secrets/README.md` (brief) referencing `docs/SEC_TDD.md`.

---

## Phase 1. Providers
**Why**: Implement pluggable backends for secrets.

**Tasks**
- [x] Task 1.1 – StaticProvider (map-based, in-memory) for tests/demo; no encryption.
- [x] Task 1.2 – NopProvider returning `ErrNotFound` to enable clean fallback to env/config.
- [x] Task 1.3 – EncryptedStoreProvider:
  - Store ciphertext/nonce/version metadata via a repository (Bun + in-memory).
  - Use AEAD (XChaCha20-Poly1305) with key from env/KMS; optional envelope decrypt hook.
  - Support `Put/Get/Delete/Describe`, latest-version resolution.
  - Tests covering round-trip and corruption handling.
- [x] Task 1.4 – Provider registry wiring so hosts can register multiple providers and pick by scope or config.
- [x] Task 1.5 – Facade `Resolver` that batches `Get` calls and returns a map keyed by `Reference`.

---

## Phase 2. Storage & Models
**Why**: Persist secrets (encrypted) when using the built-in store.

**Tasks**
- [x] Task 2.1 – Define repository interfaces under `pkg/interfaces/secrets` (CRUD + list by scope/subject/provider/key).
- [x] Task 2.2 – Implement Bun-backed repo with soft delete and versioning; use sqlite fixtures for tests.
- [x] Task 2.3 – Implement in-memory secret repo for unit tests.
- [x] Task 2.4 – Add migrations/schema docs section to `docs/SEC_TDD.md` (if needed) or `docs/SEC_ENTITIES.md` placeholder.

---

## Phase 3. Dispatcher & Adapter Integration
**Why**: Wire secret resolution into delivery without persisting sensitive data.

**Tasks**
- [x] Task 3.1 – Extend notifier/dispatcher deps with optional `SecretsResolver`; when nil, fall back to config/env.
- [x] Task 3.2 – Add per-channel secret resolution before routing: build refs for channel/provider (e.g., chat/slack token, chat_id; email/sendgrid api_key/from; sms/twilio auth token/from).
- [x] Task 3.3 – Inject resolved secrets/addresses into adapter send calls; do not persist them on messages/attempts.
- [x] Task 3.4 – Ensure adapters accept overrides (token/from/chat_id/webhook_url/auth headers) per send; keep existing config as default.
- [x] Task 3.5 – Add masked logging for secret-bearing map fields; ensure delivery logs use masked versions only.
- [x] Task 3.6 – Enforce per-subject fallback policy: only allow env/config fallback for explicit subjects (e.g., admin/system/test users); reject delivery when a user/tenant lacks scoped secrets and is not allowlisted.

---

## Phase 4. Preferences & Provider Selection
**Why**: Allow users to pick providers per channel and enforce preferences.

**Tasks**
- [x] Task 4.1 – Extend preference model to capture provider choice per channel/definition (e.g., `chat:slack`, `chat:telegram`, `email:sendgrid`).
- [x] Task 4.2 – Update dispatcher routing to honor provider preference; fall back to default provider when unset.
- [ ] Task 4.3 – Add schema/export updates so UIs (go-admin/example) can render provider selection controls.

---

## Phase 5. Example (SQLite, UI, and Secrets)
**Why**: Demonstrate the feature end-to-end with persistence.

**Tasks**
- [x] Task 5.1 – Switch `examples/web` to SQLite file DB (not in-memory) for definitions/templates/messages/preferences.
- [x] Task 5.2 – Add a simple contact/credential store (or mock secrets provider backed by SQLite) for demo users: email, phone, Slack user ID, Telegram chat ID, API keys (fake) stored encrypted.
- [x] Task 5.3 – Extend preferences UI to select channel providers per definition (e.g., toggle `chat:slack` vs `chat:telegram`) and persist choices.
- [x] Task 5.4 – Update adapter wiring in the example to resolve per-user addresses/credentials at send time; env vars remain defaults.
- [x] Task 5.5 – Seed data: demo users with contact methods and sample preferences showing mixed providers; add a “last delivery” panel showing which provider was used.
- [x] Task 5.6 – Update ./examples/web/ADAPTERS.md to reflect new setup

---

## Phase 6. Docs & Tooling
**Why**: Ensure guidance and automation are ready.

**Tasks**
- [x] Task 6.1 – Update `docs/SEC_TDD.md` with any schema/migration specifics added in Phase 2 (if not already covered).
- [x] Task 6.2 – Add short how-to for plugging Vault/AWS SM/GCP SM (path/ARN composition) in `pkg/secrets/README.md`.
- [x] Task 6.3 – Add tests for masking helpers and a lint-style check ensuring no logs include secret data.
- [x] Task 6.4 – Optional: CLI/admin snippets for secret rotation/backfill (future go-admin wiring).

---

## Phase 7. Release Prep
**Why**: Finalize for rollout.

**Tasks**
- [x] Task 7.1 – Run end-to-end demo: seed → set per-user provider prefs → send test notification → verify channel/provider selection and masked logs.
- [x] Task 7.2 – Add release notes and migration guide (new tables/config keys and defaults).
- [x] Task 7.3 – Evaluate performance of secret resolution (batching) and, if needed, add caching with short TTL for external managers.

---

This plan should be revisited after foundational phases (0–2) to adjust provider priorities, masking coverage, and example scope based on findings.
