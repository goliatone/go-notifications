# go-notifications Implementation Plan

Roadmap aligned with `docs/NTF_TDD.md` to deliver a standalone, adapter-friendly notification module that integrates localization, templates, preferences, and multi-channel delivery while remaining compatible with go-admin/go-cms via optional adapters.

## Guiding Notes
- Follow the architecture and interface contracts in `docs/NTF_TDD.md`; each task references the relevant section (entities, components, data model).
- Keep the core module free of go-admin/go-cms imports. Optional adapters (e.g., `adapters/gocms`) live outside the core path.
- All configuration decoding uses the `cfgx` helpers from `github.com/goliatone/go-config`; options layering relies on `github.com/goliatone/go-options`.
- Persistence wiring defaults to `github.com/goliatone/go-persistence-bun` + `github.com/goliatone/go-repository-bun`, with interfaces defined under `pkg/interfaces`.
- Vertical slices should land with tests, documentation notes, and example usage mirroring the snippets described in the TDD.

---

## Phase 0. Module Bootstrap & Tooling
**Why**: Establish repository layout, build tooling, and baseline packages before layering services.

**Tasks**
- [x] Task 0.1 – Scaffold module directories (`pkg/`, `internal/`, `docs/`, `adapters/`) and add `go.mod`, `.gitignore`, Taskfile, and README stub outlining scope.
- [x] Task 0.2 – Add lint/test taskfile targets plus CI stubs invoking `go test ./...`, `golangci-lint`, and `task docs:lint` for Markdown.
- [x] Task 0.3 – Create `pkg/interfaces/{logger,cache,queue,broadcaster,store}` with minimal contracts and placeholder implementations for tests.
- [x] Task 0.4 – Add configuration skeleton (`pkg/config`) using `cfgx.Build` with defaults and validation hooks; provide smoke tests verifying decoding from map/struct inputs.
- [x] Task 0.5 – Document contribution guide referencing cfgx/go-options/go-command expectations.

---

## Phase 1. Domain Modeling & Storage Contracts
**Why**: Define core entities, repositories, and shared models so services have stable contracts.

**Tasks**
- [x] Task 1.1 – Formalize domain structs (definitions, templates, events, messages, deliveries, preferences, inbox entries, subscription groups) consistent with `NTF_TDD.md#entity-descriptions`.
- [x] Task 1.2 – Define repository interfaces under `pkg/interfaces/store` for each entity, including pagination/filtering helpers and soft-delete semantics.
- [x] Task 1.3 – Implement Bun-backed adapters (`internal/storage/bun/...`) leveraging `go-persistence-bun` + `go-repository-bun`, plus in-memory repositories for tests.
- [x] Task 1.4 – Add migrations/schema documentation in `docs/NTF_ENTITIES.md` (placeholder if doc not yet created) and connect repository tests to sqlite fixtures.
- [x] Task 1.5 – Set up `pkg/storage` factory wiring DI-friendly constructors (memory vs Bun) and expose metrics hooks.

---

## Phase 2. Templates, Localization, and Rendering
**Why**: Provide the translation-first rendering pipeline and template registry described in the TDD.

**Tasks**
- [x] Task 2.1 – Implement `internal/templates` service wrapping `go-template` with helper registry, schema validation, and locale-aware rendering via `go-i18n`.
- [x] Task 2.2 – Add template definition registry supporting channel-specific variants, revisioning, and references to optional go-cms block payloads.
- [x] Task 2.3 – Expose `pkg/templates` facade with CRUD operations, translation helpers, and caching integration (via `pkg/interfaces/cache`).
- [x] Task 2.4 – Provide tests covering fallback chains, placeholder validation, and integration with sample go-cms-like payloads (without importing go-cms).
- [x] Task 2.5 – Document template authoring guidelines and how to plug in optional adapters (e.g., `adapters/gocms`).

---

## Phase 3. Dispatcher, Channel Adapters, and Message Pipeline
**Why**: Build the core notification manager, channel routing, and delivery attempt tracking.

**Tasks**
- [x] Task 3.1 – Port the coach-platform manager/adapters contracts into `pkg/notifier` and `pkg/adapters`, updating channel metadata and matcher logic per TDD.
- [x] Task 3.2 – Implement dispatcher service that expands events into per-recipient messages, applies rendering, and routes to adapters asynchronously with retry policies.
- [x] Task 3.3 – Add delivery attempt logging + repository writes, including retry/backoff strategy configuration through `pkg/config`.
- [x] Task 3.4 – Provide default adapters (console, SMTP, Sendgrid, Twilio, Telegram, WhatsApp) and ensure they satisfy the unified `Messenger` interface.
- [x] Task 3.5 – Write integration tests that simulate multi-channel send flows with fake adapters to verify concurrency, error handling, and metadata propagation.

---

## Phase 4. Preferences, Options Integration, and Evaluation
**Why**: Deliver scoped notification preferences with deterministic inheritance and rule evaluation.

**Tasks**
- [x] Task 4.1 – Implement `internal/preferences` service for CRUD, evaluation, and opt-out enforcement, backed by repository interfaces.
- [x] Task 4.2 – Build `pkg/options` helpers that wrap `go-options` stacks, including persistence wrappers for loading/saving scopes via the Bun repositories.
- [x] Task 4.3 – Expose `ResolveWithTrace` APIs plus schema export so go-admin/go-cms UIs can present editable forms and show provenance.
- [x] Task 4.4 – Integrate preferences evaluation into dispatcher pre-flight checks to enforce quiet hours, channel overrides, and subscription filters.
- [x] Task 4.5 – Document configuration patterns (system → tenant → user) with runnable examples demonstrating go-options stacks.

---

## Phase 5. Inbox Service, Real-Time Hooks, and Event Intake
**Why**: Provide in-app notification center storage and real-time delivery hooks required by go-admin.

**Tasks**
- [x] Task 5.1 – Implement inbox repository/service supporting pagination, filters, mark-read, snooze, dismiss, and badge count optimizations.
- [x] Task 5.2 – Create `pkg/interfaces/broadcaster` implementations (no-op + pluggable) and integrate with inbox mutations for WebSocket/SSE/webhook pushes.
- [x] Task 5.3 – Develop event intake service (`internal/events`) that validates payloads, schedules jobs (`pkg/interfaces/queue`), and hands work to dispatcher/commands.
- [x] Task 5.4 – Add digest scheduling support (grouping policy, go-job hooks) with idempotent job keys and tests verifying replays.
- [x] Task 5.5 – Provide example WebSocket/SSE handlers in docs (pseudo-code) illustrating how transports consume broadcaster hooks.

---

## Phase 6. Commands, Public Facade, and Module Wiring
**Why**: Expose cohesive APIs consistent with go-users/go-dashboard patterns and finalize DI wiring.

**Tasks**
- [x] Task 6.1 – Implement `internal/commands` package with `go-command.Commander[T]` implementations for definition/template CRUD, preference updates, inbox actions, and event enqueueing.
- [x] Task 6.2 – Build `pkg/commands` registry/factory functions so transports resolve commands without peeking into internal packages.
- [x] Task 6.3 – Assemble DI container wiring (`internal/di`) that stitches config, repositories, services, templates, dispatcher, preferences, inbox, and commands.
- [x] Task 6.4 – Expose `pkg/notifier.Module` façade mirroring go-cms style (lazy initialization, no-op services when disabled) plus helper methods to fetch sub-services.
- [x] Task 6.5 – Add sample usage in docs (CLI snippet, HTTP handler) showing command invocation and manager usage.

---

## Phase 7. Optional Adapters, Docs, and Release Prep
**Why**: Ensure integrations, documentation, and release tooling are ready before GA.

**Tasks**
- [x] Task 7.1 – Build `adapters/gocms` translating go-cms block/widget structures into notification templates without importing go-cms directly (e.g., expect JSON contract).
- [ ] Task 7.2 – Provide skeleton `adapters/goadmin` for UI hooks (routing, broadcaster wiring) referencing interfaces only.
- [x] Task 7.3 – Write comprehensive documentation (`docs/NTF_TDD.md` cross-links, API reference, integration guides) plus examples.
- [x] Task 7.4 – Finalize taskfile/CI updates to run unit/integration tests, linting, and sample commands.
- [ ] Task 7.5 – Prepare release checklist (semantic versioning notes, changelog template, migration guide) and run end-to-end smoke tests covering all phases.

---

This plan should be reviewed after each phase to incorporate learnings (e.g., adapter ergonomics, queue performance). Completion of all tasks yields a production-ready module satisfying the requirements in `docs/NTF_TDD.md`.
