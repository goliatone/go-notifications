# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build, Test, and Lint Commands

All development tasks use the `./taskfile` bash script:

```bash
# Run tests
./taskfile test

# Run linters
./taskfile lint

# Lint markdown documentation
./taskfile docs:lint

# Run all checks (lint, test, docs:lint)
./taskfile ci

# Run tests with coverage
./taskfile dev:cover
```

## Architecture Overview

`go-notifications` is a composable notification orchestration module for Go applications, designed to integrate with `go-admin` and `go-cms` without hard dependencies. The architecture follows a strict layered design with interface-driven boundaries.

### Core Design Principles

1. **Vertical Slices**: Features are delivered as independent slices (definitions → templates → inbox → preferences → real-time)
2. **Interface-Driven**: All external dependencies (storage, cache, queue, broadcaster, logger) are defined in `pkg/interfaces/` to keep the module headless
3. **Command-Oriented Mutations**: All stateful workflows are packaged as `go-command.Commander[T]` implementations in `internal/commands/`, exposed via `pkg/commands/Registry`
4. **Options-Driven Configuration**: Multi-scope configuration (tenant/user/system) flows through `go-options` stacks for preference inheritance and rule evaluation
5. **Standard Persistence Stack**: Relational persistence uses Bun-based repositories (`go-persistence-bun` + `go-repository-bun`)
6. **CFGX-Based Config Decoding**: Module configuration uses `cfgx.Build[T]` from `go-config` for defaults, validation, and strict key enforcement

### Module Layout

```
pkg/
  notifier/          # Public facade (Module, Manager)
  adapters/          # Channel adapters (console, smtp, sendgrid, twilio, telegram, whatsapp)
  templates/         # Template rendering with go-template/go-i18n
  preferences/       # Preference evaluation API
  inbox/             # Inbox operations API
  events/            # Event intake API
  commands/          # Command registry (go-command integration)
  options/           # go-options helpers for configuration
  domain/            # Entity definitions and domain types
  interfaces/        # External dependency contracts (logger, cache, queue, broadcaster, store)
  storage/           # Storage provider factory
  config/            # Module configuration

internal/
  di/                # DI container wiring
  dispatcher/        # Event expansion, routing, retries
  templates/         # Rendering engine internals
  preferences/       # Preference store service
  inbox/             # Inbox persistence service
  events/            # Event validation and ingestion
  commands/          # Command implementations
  storage/
    bun/             # Bun-based repositories
    memory/          # In-memory repositories for tests

adapters/
  gocms/             # Optional go-cms integration (kept out of core)
```

### Key Components

**Module Facade (`pkg/notifier/module.go`)**
Entry point that assembles the DI container and exposes high-level accessors for Manager, Templates, Preferences, Inbox, Events, Commands, and AdapterRegistry.

**DI Container (`internal/di/container.go`)**
Wires repositories, services, dispatcher, commands, and adapter registry. Enforces dependency validation and provides defaults for missing optional dependencies.

**Dispatcher (`internal/dispatcher/service.go`)**
Central orchestrator that expands events into per-recipient messages, renders via templates, routes to adapters, handles retries with exponential backoff, and emits delivery events.

**Manager (`pkg/notifier/manager.go`)**
Public API for sending notifications. Persists events via storage, invokes dispatcher with registry-driven adapters, supports preference evaluation and inbox integration.

**Entities (`pkg/domain/entities.go`)**
Domain models with Bun schema annotations. All entities include `RecordMeta` (UUID, timestamps, soft-delete). Key types:
- `NotificationDefinition`: Describes notification types with channel enablement and throttling policies
- `NotificationTemplate`: Channel-specific templates with go-template rendering
- `NotificationEvent`: Input payload before fan-out
- `NotificationMessage`: Rendered message for specific recipient/channel
- `DeliveryAttempt`: Adapter execution tracking
- `NotificationPreference`: Opt-in/out settings per user/definition/channel
- `InboxItem`: In-app notifications with read/unread state

**Adapters (`pkg/adapters/`)**
All delivery providers implement the `Messenger` interface. Registry (`pkg/adapters/registry.go`) auto-selects adapters using matcher functions. Built-in adapters: console, SMTP, Sendgrid, Twilio, Telegram, WhatsApp.

**Templates (`pkg/templates/service.go`, `internal/templates/service.go`)**
Wraps go-template for rendering and go-i18n for translation. Supports per-channel template variants, locale fallbacks, and placeholder validation against definition schemas.

**Preferences (`pkg/preferences/service.go`, `internal/preferences/service.go`)**
Manages scoped preferences (global, definition, channel). Enforces opt-out rules before delivery. Provides APIs to evaluate if a user should receive a notification.

**Inbox (`pkg/inbox/service.go`, `internal/inbox/service.go`)**
Persists in-app notifications with pagination, read/unread tracking, and real-time events via pluggable `Broadcaster` interface (WebSocket/SSE/webhook).

**Events (`pkg/events/service.go`, `internal/events/service.go`)**
Accepts sync/async event submissions, validates payloads against definition schemas, hands events to dispatcher. Supports digest grouping based on definition policies.

### Integration Patterns

**Module Initialization**:
```go
import (
    i18n "github.com/goliatone/go-i18n"
    "github.com/goliatone/go-notifications/pkg/notifier"
    "github.com/goliatone/go-notifications/pkg/adapters/console"
)

translator, _ := i18n.New(/* config */)
consoleAdapter, _ := console.New(lgr)

mod, err := notifier.NewModule(notifier.ModuleOptions{
    Translator: translator,
    Adapters:   []adapters.Messenger{consoleAdapter},
})

manager := mod.Manager()
```

**Sending a Notification**:
```go
err := manager.Send(ctx, notifier.Event{
    DefinitionCode: "welcome",
    Recipients:     []string{"user@example.com"},
    Context:        map[string]any{"name": "Alice"},
})
```

**Registering Commands**:
```go
for _, cmd := range mod.Commands().Commanders() {
    _ = registry.RegisterCommand(cmd)
}
```

## Storage Layers

The module supports two storage implementations:

1. **Memory** (`internal/storage/memory/`): In-memory repositories for tests and demos
2. **Bun** (`internal/storage/bun/`): Production repositories using `uptrace/bun` with SQLite/PostgreSQL support

Storage providers are injected via `storage.Providers` and implement interfaces defined in `pkg/interfaces/store/repositories.go`.

## Localization and Templates

- **Localization**: Uses `github.com/goliatone/go-i18n` with opaque locale codes and fallback chains
- **Template Rendering**: Uses `github.com/goliatone/go-template` with go-template syntax
- **Template Sources**: Templates can be stored inline or reference go-cms blocks via `TemplateSource` metadata
- **Placeholder Validation**: Templates validate placeholders against `TemplateSchema` (required/optional fields)

## Testing

All services include comprehensive tests using in-memory repositories. Test files follow `*_test.go` naming convention and use table-driven tests where applicable.

Integration tests for the Manager (`pkg/notifier/manager_test.go`) exercise multi-channel fan-out, retry failures, and adapter metadata propagation.

## External Dependencies

The module depends on:
- `github.com/goliatone/go-i18n` - Localization
- `github.com/goliatone/go-template` - Template rendering
- `github.com/goliatone/go-options` - Configuration layering
- `github.com/goliatone/go-config` - Config decoding (cfgx)
- `github.com/goliatone/go-command` - Command pattern implementation
- `github.com/goliatone/go-persistence-bun` - Bun persistence adapters
- `github.com/goliatone/go-repository-bun` - Bun repository patterns
- `github.com/uptrace/bun` - SQL toolkit
- `github.com/google/uuid` - UUID generation

## Technical Design Document

Refer to [docs/NTF_TDD.md](docs/NTF_TDD.md) for the complete technical design document covering:
- Design philosophy and vertical slices approach
- Key architectural decisions
- Entity descriptions and data model
- Core architecture components
- Go module structure
- Usage examples
