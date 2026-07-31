# go-notifications Module - Architecture Design Document

## Table of Contents
1. [Overview](#overview)
2. [Design Philosophy](#design-philosophy)
3. [Key Architectural Decisions](#key-architectural-decisions)
4. [Entity Descriptions](#entity-descriptions)
5. [Core Architecture Components](#core-architecture-components)
6. [Data Model](#data-model)
7. [Go Module Structure](#go-module-structure)

## Overview

This document captures the requirements and technical direction for the `go-notifications` module. The module powers the **go-admin notification center** described in the updated go-admin architecture doc while remaining self-contained and integration-ready for other applications. It extends the initial `coach-platform/src/pkg/notify` implementation with first-class localization, templating, and preference management using `github.com/goliatone/go-i18n` and `github.com/goliatone/go-template`.

### Module Goals

- Self contained notification orchestration functionality
- Minimal external dependencies (only `go-i18n`, `go-template`, and host-provided interfaces)
- Interface based design for pluggable channel, storage, and transport implementations
- Progressive enhancement through vertical slices that deliver value independently
- Integration ready with go-admin, go-cms, and broader Go ecosystem components

### What This Module Provides

- Notification definitions, templates, and translation management
- Multi-channel delivery (SMS, email, push, chat, in-app) with pluggable adapters
- Notification center/inbox storage with read/unread tracking
- User and tenant notification preferences plus subscription groups
- Event ingestion API, batching, and digest scheduling (backed by go-job)
- Real-time delivery hooks (WebSocket/webhook) for in-app notifications
- Template rendering via go-template and localization via go-i18n
- Integration helpers to reuse go-cms blocks as templates when desired
- Persistence adapters backed by `github.com/goliatone/go-persistence-bun` + `github.com/goliatone/go-repository-bun` (same stack as `../go-users`) so relational storage stays consistent across modules
- Configuration layering and option evaluation backed by `github.com/goliatone/go-options` (`../go-options`) so tenants/users inherit sensible defaults while still supporting scoped overrides and rule evaluation

### What This Module Does NOT Provide

- Authentication/Authorization (defer to go-auth or host system)
- User directory and identity resolution (host supplies user metadata)
- WebSocket server implementation (host wires go-router/go-admin UI transports)
- Delivery provider SDKs outside the adapters packaged here
- Full analytics dashboards (module surfaces data so hosts can visualize it)
- Persistence implementations (interfaces fulfilled by go-persistence-bun/go-repository-bun adapters)

## Design Philosophy

### Vertical Slices Approach

1. **Sprint 1**: Core notification definitions, templates, and adapter-backed delivery
2. **Sprint 2**: Localization, go-template integration, and per-channel templating
3. **Sprint 3**: In-app inbox service with read/unread tracking and pagination
4. **Sprint 4**: Preference store, subscription management, and guard rails
5. **Sprint 5**: Real-time delivery (WebSocket/webhook) plus digests and analytics hooks

Each slice ships independently and extends existing services without breaking host integrations.

### Module Independence

Core services (definitions, templates, channels, inbox, preferences) are isolated packages that communicate through interfaces. Hosts can swap storage, queueing, or channel adapters without touching the rest of the module, and go-admin/go-cms integrations layer on top without introducing circular dependencies.

### Interface-Driven Design

- Channel adapters implement the `Messenger` contract (ported from the coach-platform implementation) and expose capability metadata.
- Storage, cache, queue, and logging dependencies are defined in `pkg/interfaces` to keep the module headless.
- Services interact via interfaces so tests can run entirely in-memory while production deployments plug in Bun repositories, Redis caches, or other infrastructure.
- Mutating workflows (definition management, preference updates, inbox maintenance) are exposed as `go-command.Commander[T]` implementations, mirroring the `go-users` (`../go-users`) command catalog so HTTP handlers, CLIs, schedulers, and background jobs can invoke the same logic without depending on internal packages.

## Key Architectural Decisions

1. **Opaque Locale Codes**: Locale identifiers remain string-based with no enforced format so host applications can reuse existing locale catalogs. go-i18n handles fallback chains configured via `Config.Localization`.
2. **Template Payloads as JSON**: Template metadata (channels, bindings, fallbacks) is stored as JSONB so we can represent structured merge data without schema churn.
3. **Interface-Based Channels**: Every delivery provider implements `adapters.Messenger`. The manager selects adapters using a matcher function (default derived from channels list) just like the existing coach-platform package.
4. **Default Configuration**: `Config.DefaultChannel`, `Config.FallbackLocale`, and built-in adapters (console logger) let developers send messages without custom wiring.
5. **Opt-In Complexity**: Features such as digest scheduling, batching, or inbox retention are disabled until explicitly configured to avoid surprising storage or queue usage.
6. **Unified Schema**: Definitions, preferences, and inbox entries share the same schema across simple and advanced deployments. Advanced features add JSON metadata instead of new tables.
7. **Progressive Enhancement**: Vertical slices keep the same contracts. Adding WebSocket delivery does not change how events are enqueued or stored.
8. **Service Layer Architecture**: Business logic (rendering, routing, deduplication) resides in services while repositories stay thin.
9. **Soft Deletes Everywhere**: Definitions, preferences, and inbox entries include `deleted_at` to support compliance and replay.
10. **Scheduled Delivery Support**: `ScheduleAt` fields on events feed go-job-backed queues. Jobs must be idempotent and keyed so reruns are safe.
11. **Translation-First**: go-i18n catalogs power channel-specific templates. All user-visible strings accept locale identifiers, and template resolution enforces fallback chains.
12. **Minimal Dependencies**: Besides go-i18n and go-template, the module depends on standard library plus interfaces satisfied by go-persistence-bun/go-repository-bun/go-repository-cache when hosts choose to use the provided adapters.
13. **Pluggable Logging & Telemetry**: Services log through the shared logger interface; structured events (render failures, delivery retries) include operation names so go-admin dashboards can visualize them.
14. **Command-Oriented Mutations**: All stateful workflows (bootstrapping templates, queuing events, marking inbox entries read) are packaged as `go-command` commands so different transports reuse identical business logic; commands follow the same conventions established in `go-users` (`../go-users`).
15. **Standard Persistence Stack**: Whenever relational persistence is required, repositories use the Bun-based stack (`../go-persistence-bun` + `../go-repository-bun`) so caching/decorators stay compatible with the rest of the ecosystem (matching `go-users` defaults).
16. **Options-Driven Configuration**: All multi-scope configuration (tenants, audiences, users) flows through `go-options` stacks so preference inheritance, overrides, tracing, and evaluation follow a consistent pattern.
17. **CFGX-Based Config Decoding**: Module-level configuration adheres to the cfgx pattern defined in `go-config`, meaning every constructor/DI wire-up decodes inputs via `cfgx.Build[T]` with defaults, strict key enforcement, preprocessors (e.g., `EvalFuncFields`), and validation hooks.
18. **Integration via Adapters**: The core module does not import go-admin or go-cms; instead, optional adapters (e.g., `github.com/goliatone/go-notifications/adapters/gocms`) map external interfaces or schemas into our internal models so we stay compatible without hard dependencies.

## Entity Descriptions

`go-notifications` is composed of the following entities. Detailed schemas and examples will live in `docs/NTF_ENTITIES.md`.

### Notification Definitions
**Role**: Describe a notification type (e.g., `admin.widget.recent_activity`) including channel enablement, throttling policies, and i18n bindings.

### Notification Templates
**Role**: Channel-specific template artifacts rendered via go-template. Each template can reference go-cms blocks and includes translation metadata managed by go-i18n.

### Notification Events
**Role**: Input payload describing who to notify, context metadata, and optional schedule. Events are persisted for auditing before expansion into deliveries.

### Notification Messages
**Role**: Expanded, rendered messages destined for a specific recipient in a single channel. Messages capture the frozen subject/body plus localization info.

### Delivery Attempts
**Role**: Track channel adapter execution, response payloads, retries, and final delivery status.

### Notification Preferences
**Role**: Store per-user or per-audience opt-in/opt-out toggles per notification definition, locale overrides, and channel overrides.

### Inbox Items
**Role**: Store rendered in-app notifications, read/unread status, dismissal timestamps, and action metadata for the go-admin notification center.

### Subscription Groups
**Role**: Named cohorts (e.g., `beta-testers`, `account-owners`) that definitions can target. Each group references membership rules provided by the host application.

### Channel Providers
**Role**: Configuration objects for adapters (API keys, tokens, rate limits) plus capability descriptors so the dispatcher can match events to providers.

## Core Architecture Components

### Definitions Module (`definitions/`)

- Register, update, and archive notification definitions.
- Manage throttling policies, batching rules, and metadata contracts.
- Surface guard rails to go-admin so editors cannot delete definitions in use.

### Templates & Localization Module (`templates/`)

- Wrap go-template for rendering and go-i18n for translation lookup.
- Support per-channel template variants plus locale fallbacks.
- Provide helpers to pull template bodies from go-cms blocks or store inline.
- Validate placeholders against the definition schema to catch missing context keys.
- Document authoring patterns and helper usage in `docs/NTF_TEMPLATES.md`.

### Channel Adapter Module (`adapters/`)

- Port and extend the coach-platform `adapters` package (console, SMTP, Sendgrid, Twilio, Telegram, WhatsApp, etc.).
- Adapters declare capabilities (`channels`, `formats`, `max_attach_size`) so the dispatcher can auto-select the best provider.
- Provide configuration helpers to load provider secrets from env/config providers.
- Default adapters now live under `pkg/adapters/{console,smtp,sendgrid,twilio,telegram,whatsapp}` and share the unified `Messenger` interface plus the registry matcher (`pkg/adapters/registry.go`).

### Dispatcher & Manager Module (`dispatcher/`)

- Reuse the `Manager` abstraction from the existing implementation as the central orchestrator.
- Accept events, expand them into per-recipient messages, render via templates, and route them to adapters asynchronously via errgroup or go-job workers.
- Handle retries with exponential backoff and per-channel limits.
- Emit structured events for delivery success/failure so analytics dashboards can subscribe.
- Implemented via `internal/dispatcher.Service` (renders, persists, retries) and surfaced publicly through `pkg/notifier.Manager`, which persists events then invokes the dispatcher with registry-driven adapters.
- Integration tests (`pkg/notifier/manager_test.go`) exercise multi-channel fan-out, retry failures, and adapter metadata propagation using the memory repositories.

### Command Layer (`commands/`)

- Encapsulate mutating flows (definition CRUD, template publication, preference updates, inbox mutations, enqueue/send operations) as `go-command.Commander[T]` implementations.
- Provide factories that wire dependencies via the module’s DI container so transports can resolve commands without importing internal packages.
- Mirror the `go-users` approach by exposing typed request structs, validation helpers, and decorators (logging, metrics) around each command.
- `internal/commands` packages these handlers while `pkg/commands` exposes a registry so hosts can register every commander with go-command without touching internal paths.

### Preferences Module (`preferences/`)

- Manage scoped preferences (global, definition, channel).
- Enforce opt-out rules before enqueuing deliveries.
- Provide APIs for go-admin UI to fetch/update preferences and to evaluate if a user should receive a notification.

### Inbox & Real-Time Module (`inbox/`)

- Persist in-app notifications, support pagination/filters, and store read/unread timestamps.
- Emit real-time events via a pluggable `Broadcaster` interface (host wires WebSocket, SSE, or webhook transport).
- Provide `MarkRead`, `Snooze`, and `Dismiss` operations for go-admin clients.
- Realtime transport examples live in `docs/NTF_REALTIME.md`.

### Event Intake Module (`events/`)

- Accept sync/async event submissions from host applications (HTTP, go-command handler, or direct API).
- Validate payloads against definition schemas, attach metadata, and hand events off to the dispatcher.
- Support digests by grouping events based on definition policy before rendering.

### Options & Configuration Module (`options/`)

- Wrap tenant/user/system configuration using `go-options` stacks so defaults and overrides resolve deterministically.
- Expose helpers to materialize effective notification settings (channels enabled, throttling, template bindings) plus `ResolveWithTrace` responses for UI explanations.
- Provide schema export so go-admin tooling can display editable forms generated from `go-options` OpenAPI descriptors.
- Use `go-options` evaluators for rule-based gating (e.g., quiet hours, locale-specific overrides) without reimplementing expression parsing.
- See `docs/NTF_OPTIONS.md` for runnable examples and scope layering guidance.

### Storage & Caching Module (`storage/`)

- Repository interfaces for each entity with Bun-based adapters plus in-memory mocks.
- Optional caching via go-repository-cache for template lookups and preference evaluation.
- Transaction helpers to ensure definitions/templates/preferences mutate atomically when editors make changes from go-admin.

### Logging & Observability (cross-cutting)

- All services depend on the shared logger interface; production builds can wire `github.com/goliatone/go-logger`.
- Emit metrics hooks (`DeliveriesSent`, `DeliveryFailures`, `InboxReads`) so host telemetry stacks can consume them.
- Audit trails capture who changed definitions/templates/preferences for compliance.

### Usage Example for Channel Dispatch

```go
import (
    "context"

    "github.com/goliatone/go-notifications/pkg/notifier"
    "github.com/goliatone/go-notifications/pkg/adapters/console"
)

func sendTest(ctx context.Context, lgr core.Log) error {
    consoleAdapter, _ := console.New(lgr)
    mgr := notifier.New(notifier.WithAdapter(consoleAdapter))

    msg := &notifier.Message{
        Subject:  "Admin Test",
        Body:     "Notifications system online",
        To:       []string{"ops@example.com"},
        Channels: []string{"email:console"},
    }

    return mgr.Send(ctx, msg)
}
```

The snippet shows how the existing coach-platform manager is lifted into the new module while remaining configurable via options. Templates, localization, and preference gating are applied before invoking `mgr.Send`.

## Data Model

The data model mirrors the architecture above and supports internationalization, preferences, and auditing out of the box. Detailed schemas will be captured in `NTF_ENTITIES.md`.

### Locales Table
Stores locale metadata and fallback chains consumed by go-i18n.

### Notification Definitions Table
Captures definition code, default severity, throttling policy, and enabled channels.

### Notification Templates Table
Stores template metadata (channel, format, revision) plus references to go-cms blocks when applicable.

### Template Translations Table
Persists translated subject/body per locale and per channel.

### Notification Events Table
Tracks inbound events, context payloads, target audiences, and schedule timestamps.

### Notification Messages Table
Holds rendered content destined for a specific recipient/channel combination and links back to the originating event.

### Delivery Attempts Table
Records adapter invocations, response payloads, retry counters, and failure reasons for diagnostics.

### Notification Preferences Table
Stores user or audience preferences, channel overrides, and locale overrides with soft-delete support.

### Subscription Groups Table
Defines reusable cohorts and rulesets. Hosts can point to their own membership resolvers via foreign keys or JSON expressions.

### Inbox Items Table
Persists in-app notifications per user with read/unread timestamps, pinned state, snooze metadata, and CTA payloads.

### Inbox Delivery State Table
Optional table storing per-user counters and last_seen timestamps to speed up unread badge rendering in go-admin.

## Go Module Structure

### Module Layout

`go-notifications` follows the same layered approach as go-cms: implementation lives under `internal/`, exported facades under `pkg/`, and adapters/helpers in dedicated packages. Proposed layout (subject to refinement once implementations begin):

```
go-notifications/
  docs/
    NTF_TDD.md
  pkg/
    notifier/          // Public facade wrapping dispatcher/manager
    adapters/          // Channel interfaces and built-in adapters
    templates/         // Template helpers + go-template/go-i18n bindings
    preferences/       // Public API for preference evaluation
    inbox/             // Public API for inbox operations
    commands/          // Exported go-command command registries (mirroring go-users)
    options/           // go-options backed configuration helpers and schema/export logic
  internal/
    di/               // Service container wiring config, storage, adapters, and commands
    interfaces/        // Logger, cache, queue, broadcaster contracts
  internal/
    definitions/       // Definition services, registry, bootstrap
    templates/         // Rendering + localization engines
    dispatcher/        // Event expansion, routing, retries
    providers/         // Adapter wiring + capability registry
    storage/           // Repository implementations (Bun, memory)
    events/            // Event ingestion and validation
    inbox/             // Inbox persistence and state reducers
    preferences/       // Preference store services
    realtime/          // Broadcaster hooks (WS/SSE/webhook)
    jobs/              // go-job integration for scheduled delivery
    commands/          // Concrete command implementations, validators, decorators
    options/           // go-options stack assembly, caching, rule evaluation helpers
  adapters/
    gocms/             // Optional go-cms integration (kept out of core build, see docs/NTF_ADAPTERS.md)
    goadmin/           // Optional go-admin integration / helpers
```

### External Dependency Interfaces

- `pkg/interfaces/logger` – minimal logging contract shared with go-cms/go-admin.
- `pkg/interfaces/cache` – optional cache provider used for template and preference lookups.
- `pkg/interfaces/broadcaster` – abstraction over WebSocket/SSE/webhook broadcasters; host applications provide implementations.
- `pkg/interfaces/queue` – schedules jobs via go-job or any compatible task runner.
- `pkg/interfaces/store` – storage provider contract satisfied by go-persistence-bun/go-repository-bun adapters.
- `go-command` – commands exported by `pkg/commands` implement `go-command.Commander[T]` so transports invoke workflows consistently (matching the go-users approach).
- `go-options` – configuration helpers in `pkg/options` use go-options scopes/layers to merge defaults, tenant overrides, and per-user preferences, expose schema/rule evaluation utilities, and ship persistence wrappers that serialize scopes via the Bun-backed repositories so callers do not have to reimplement storage.
- `go-config` (`cfgx`) – `pkg/config` relies on the cfgx helpers from go-config to decode arbitrary inputs (maps, structs, generators) into typed configuration structs with consistent defaults, strict key validation, preprocessors, and evaluation of deferred values.
- `adapters/gocms` (optional) – a separate module translates go-cms block/widget structures into notification templates without requiring the core package to import go-cms directly; similar adapters can target other systems (e.g., go-admin) using the same extension point. These adapters live outside the core module so repositories that do not need go-cms/go-admin can omit them entirely. See `docs/NTF_ADAPTERS.md` for the JSON contract, usage samples, and locale mapping recommendations.

### Configuration Layer

`pkg/config` exposes a declarative struct where hosts opt into features (e.g., `Config.Inbox.Enabled`, `Config.Dispatcher.MaxRetries`) and wire required dependencies. All decoding flows through `cfgx.Build` from `go-config`, ensuring defaults, strict key enforcement, evaluation of function fields, and validation are applied uniformly regardless of whether the source is `go-config` generated structs, maps, or dynamic providers.

### Public API Layer

`pkg/notifier` exposes high-level helpers: `Module`, `Manager`, and `Send` functions mirroring the go-cms façade style. The API hides DI wiring and returns no-op implementations when features are disabled, allowing hosts to conditionally enable inbox or realtime support without defensive checks.

```go
translator := newTranslator()
mod, _ := notifier.NewModule(notifier.ModuleOptions{Translator: translator})

manager := mod.Manager()
_ = manager.Send(ctx, notifier.Event{DefinitionCode: "welcome", Recipients: []string{"user@example.com"}})

for _, cmd := range mod.Commands().Commanders() {
    _ = registry.RegisterCommand(cmd)
}
```

### DI Container Implementation

An internal DI container assembles services, injects adapters, configures go-i18n/go-template, and wires repositories. It also handles bootstrapping (loading definitions, templates, preferences) and exposes handles so go-admin can retrieve services via the module façade. The container mirrors go-cms patterns for consistency: service constructors return errors, dependencies are scoped, and localization catalogs load from embedded FS or host-provided sources.

---

This TDD keeps go-notifications aligned with go-admin goals while building upon the proven design principles established by go-cms: interface-driven architecture, localization-first mindset, and incremental, testable slices.
