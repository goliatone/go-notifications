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
- Reminder cadence primitives (`pkg/reminders`) for deterministic scheduling,
  cooldown windows, and no-spam evaluation in host-managed sweep jobs.
- Optional adapters (`adapters/gocms`, future `adapters/goadmin`) that translate
  external schemas into notification templates without direct dependencies.

## Using the module

```go
import (
    "context"

    "github.com/goliatone/go-notifications/pkg/notifier"
    "github.com/goliatone/go-notifications/pkg/config"
)

func send(ctx context.Context) error {
    cfg := config.Default()
    mod, err := notifier.NewModule(cfg)
    if err != nil {
        return err
    }
    manager := mod.Manager()
    return manager.Send(ctx, notifier.Event{
        DefinitionCode: "welcome",
        Recipients:     []string{"user@example.com"},
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
- `pkg/reminders` provides pure policy/state evaluation helpers for recurring
  reminder workflows owned by the host app.
- `adapters/gocms` converts
  [go-cms](https://github.com/goliatone/go-cms) snapshots into
  `templates.TemplateInput` values; see `docs/NTF_ADAPTERS.md`.

## Documentation map

- `docs/NTF_TDD.md`: complete technical design.
- `docs/GUIDE_REMINDERS.md`: reminder cadence primitives (`pkg/reminders`) and
  host sweep integration pattern.
- `docs/NTF_TEMPLATES.md`: template authoring, schema validation, and go-cms imports.
- `docs/NTF_OPTIONS.md`, `docs/NTF_ENTITIES.md`, and `docs/NTF_REALTIME.md`:
  supporting guides.
- `docs/NTF_TSK.md`: implementation roadmap with progress for each phase.
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
