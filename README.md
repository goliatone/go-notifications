# go-notifications

`go-notifications` is a self-contained module that handles definitions,
templates, rendering, dispatch, preferences, inbox state, and persistence for
notification workloads. It stays adapter first so hosts can plug in their own
storage, queue, and channel providers.

## What the module provides

- Domain entities for definitions, templates, messages, deliveries, inbox
  entries, and preferences under `pkg/domain`.
- Repository contracts in `pkg/interfaces` plus Bun and in-memory
  implementations in `internal/storage`.
- Localization-aware template pipeline (`pkg/templates`, `internal/templates`)
  backed by [go-template](https://github.com/goliatone/go-template) and
  [go-i18n](https://github.com/goliatone/go-i18n), with cache hooks.
- Dispatcher, channel adapters, and delivery attempt tracking
  (`pkg/notifier`, `pkg/adapters`, `internal/dispatcher`).
- Preference evaluation with
  [go-options](https://github.com/goliatone/go-options), inbox services, and
  realtime broadcaster bridges.
- Command catalog (`pkg/commands`) so transports can call command handlers
  without touching `internal` packages.
- Embedded, ordered SQLite/PostgreSQL migrations plus durable publications,
  scoped idempotency, typed receipts, and explicit retry operations.
- Immediate-only transient rendering, per-definition persistence projections,
  recipient resolution, privacy-safe errors, and opt-in raw diagnostics.
- Reminder cadence primitives (`pkg/reminders`) for deterministic scheduling,
  cooldown windows, and no-spam evaluation in host-managed sweep jobs.
- Optional adapters (`adapters/gocms`, future `adapters/goadmin`) that translate
  external schemas into notification templates without direct dependencies.

## Using the module

```go
import (
    "context"

    "github.com/goliatone/go-notifications/pkg/notifier"
)

func send(ctx context.Context, mod *notifier.Module) error {
    return mod.Manager().Send(ctx, notifier.Event{
        DefinitionCode: "welcome",
        Recipients:     []string{"user-123"},
        Context: map[string]any{
            "Name": "Rosa",
        },
    })
}
```

- `pkg/storage` builds Bun or in-memory repositories depending on the
  configuration.
- `pkg/templates.Service` manages template CRUD and rendering; adapters can
  call it through the exported interface.
- `pkg/commands.Registry` exposes typed command handlers so HTTP, CLI, and queue
  transports share execution paths.
- `pkg/events.Service` exposes receipt-returning intake, immediate transient
  dispatch, publication recovery, and explicit retry.
- `pkg/reminders` provides pure policy/state evaluation helpers for recurring
  reminder workflows owned by the host app.
- `adapters/gocms` converts
  [go-cms](https://github.com/goliatone/go-cms) snapshots into
  `templates.TemplateInput` values; see `docs/NTF_ADAPTERS.md`.

## Durable pipeline adoption

Production hosts should register the package migration source in their shared
`go-persistence-bun` graph before constructing Bun providers:

```go
manager := persistence.NewMigrations()
if err := notifications.RegisterMigrations(manager); err != nil {
    return err
}
if err := manager.Migrate(ctx, db); err != nil {
    return err
}
providers := storage.NewBunProviders(db)
```

Module construction never mutates the schema. Scheduled and digest intake also
requires a durable queue; the built-in no-op queue fails closed and leaves the
persisted publication recoverable through `mod.RecoverPending`.

For sensitive values, call `mod.Events().DispatchImmediate` with
`events.ImmediateRequest.Transient`. Do not put credentials, destinations, or
one-time URLs in persistent `Context`. Configure `RecipientResolver`,
`Persistence`, `Privacy`, and `Diagnostic` through `notifier.ModuleOptions`.

The upgrade is planned as `v0.15.0`. After the release tag is authorized and
published, hosts should adopt it with:

```bash
go get github.com/goliatone/go-notifications@v0.15.0
```

Pin the released module version; do not use a local `replace` for production
adoption.

## Documentation map

- `docs/GUIDE_GETTING_STARTED.md`: module setup and first delivery.
- `docs/GUIDE_EVENTS.md`: receipts, idempotency, transient data, publications,
  digest behavior, and retry.
- `docs/GUIDE_INTEGRATION.md`: migrations, module policies, resolver,
  diagnostics, and commands.
- `docs/GUIDE_REMINDERS.md`: reminder cadence primitives (`pkg/reminders`) and
  host sweep integration pattern.
- `docs/NTF_TEMPLATES.md`: template authoring, schema validation, and go-cms imports.
- `docs/NTF_OPTIONS.md`, `docs/NTF_ENTITIES.md`, and `docs/NTF_REALTIME.md`:
  supporting guides.
- `docs/onready.md`: opt-in OnReady helper for “job ready” style notifications
  (example: `examples/onready`).

### Activity hooks

The module emits activity events (created, delivered/failed, inbox
read/snooze/dismiss) through optional hooks. Provide `Activity` hooks in
`notifier.ModuleOptions`—for example, bridge to go-users with
`activity/usersink.Hook`:

```go
import (
    "github.com/goliatone/go-notifications/pkg/activity"
    "github.com/goliatone/go-notifications/pkg/activity/usersink"
    "github.com/goliatone/go-notifications/pkg/notifier"
)

module, _ := notifier.NewModule(notifier.ModuleOptions{
    // ...
    Activity: activity.Hooks{usersink.Hook{Sink: myGoUsersSink}},
})
```

## OnReady helper

`pkg/onready` ships an opt-in definition/template and notifier wrapper for
“something is ready” flows such as exports, reports, and async jobs. It
installs via a helper and reuses the main dispatcher and renderer.

```go
import "github.com/goliatone/go-notifications/pkg/onready"

result, _ := onready.Register(ctx, onready.Dependencies{
    Definitions: defRepo,
    Templates:   tplSvc,
}, onready.Options{})

ready, _ := onready.NewNotifier(manager, result.DefinitionCode)
_ = ready.Send(ctx, onready.OnReadyEvent{
    Recipients: []string{"user-1"},
    FileName:   "orders.csv",
    Format:     "csv",
    URL:        "https://example.com/orders.csv",
    ExpiresAt:  "2025-01-01T00:00:00Z",
})
```

Attachments are supported via `OnReadyEvent.Attachments` (raw bytes for email
adapters and URLs for chat/SMS providers), with optional per-channel overrides.
See `docs/onready.md`.

## Development workflow

The repository uses the same taskfile-driven quality model as `go-admin`.
Run tasks from the repository root; each Go task covers both the root module and
the nested `examples/web` module:

```bash
./taskfile go:quality:pr   # format, tests, and changed-code lint
./taskfile go:quality:all  # format, tests, race, full lint, and security
./taskfile go:lint         # full GolangCI lint
./taskfile go:lint:report  # report findings without failing
./taskfile go:security:all # govulncheck and gosec
./taskfile go:cover        # coverage report for every module
./taskfile ci              # full quality, docs, samples, and OnReady checks
```

GolangCI is pinned in `.golangci-lint-version`; the taskfile verifies installed
tool versions and installs the pinned releases when necessary. Repository-local
Go caches live under `.tmp/`. CI invokes the same `./taskfile ci` entry point
through `.github/workflows/ci.yml`, so local and CI policy stay aligned.
