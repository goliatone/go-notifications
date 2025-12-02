# go-notifications

`go-notifications` is a self-contained module that handles definitions, templates, rendering, dispatch, preferences, inbox state, and persistence for notification workloads. It stays adapter first so hosts can plug in their own storage, queue, and channel providers.

## What the module provides

- Domain entities for definitions, templates, messages, deliveries, inbox entries, and preferences under `pkg/domain`.
- Repository contracts in `pkg/interfaces` plus Bun and inmemory implementations in `internal/storage`.
- Localization aware template pipeline (`pkg/templates`, `internal/templates`) backed by [go-template](https://github.com/goliatone/go-template) + [go-i18n](https://github.com/goliatone/go-i18n) and cache hooks.
- Dispatcher, channel adapters, and delivery attempt tracking (`pkg/notifier`, `pkg/adapters`, `internal/dispatcher`).
- Preference evaluation with [go-options](https://github.com/goliatone/go-options), inbox services, and realtime broadcaster bridges.
- Command catalog (`pkg/commands`) so transports can call command handlers without touching `internal` packages.
- Optional adapters (`adapters/gocms`, future `adapters/goadmin`) that translate external schemas into notification templates without adding direct dependencies.

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

- `pkg/storage` builds Bun or in memory repositories depending on the configuration.
- `pkg/templates.Service` manages template CRUD and rendering, adapters can call it through the exported interface
- `pkg/commands.Registry` exposes typed command handlers so HTTP, CLI, or queue transports share execution paths
- `adapters/gocms` includes helper structs to convert [go-cms](https://github.com/goliatone/go-cms) snapshots into `templates.TemplateInput` values, see `docs/NTF_ADAPTERS.md`

## Documentation map

- `docs/NTF_TDD.md`: complete technical design.
- `docs/NTF_TEMPLATES.md`: template authoring, schema validation, and go-cms imports.
- `docs/NTF_OPTIONS.md`, `docs/NTF_ENTITIES.md`, `docs/NTF_REALTIME.md`: supporting guides.
- `docs/NTF_TSK.md`: implementation roadmap with progress for each phase.
- `docs/onready.md`: opt-in OnReady helper for “job ready” style notifications (example: `examples/onready`).

### Activity hooks

The module emits activity events (created, delivered/failed, inbox read/snooze/dismiss) through optional hooks. Provide `Activity` hooks in `notifier.ModuleOptions`—for example, bridge to go-users with `activity/usersink.Hook`:

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

`pkg/onready` ships an opt-in definition/template + notifier wrapper for “something is ready” flows (exports, reports, async jobs). It installs via a helper and reuses the main dispatcher/renderer.

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

## Development workflow

The repository includes a shell-based taskfile. Run tasks directly from the repo root:

```bash
./taskfile lint         # golangci-lint run ./...
./taskfile test         # go test ./...
./taskfile docs:lint    # markdownlint or markdownlint-cli2
./taskfile samples      # go run ./cmd/examples/gocms
./taskfile ci           # lint + test + docs + samples
```

`taskfile` sets `GOCACHE` to `tmp/go-cache` so repeated runs stay fast without touching the host-level Go cache. CI uses the same entry point via `.github/workflows/ci.yml`.
