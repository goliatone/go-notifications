# Integration Guide

This guide covers integrating `go-notifications` with host applications, including module initialization, dependency injection, storage configuration, and testing patterns.

---

## Overview

`go-notifications` is designed as a composable module that integrates with your application's infrastructure. Key integration points include:

- **Module initialization** - Configure and wire the notification system
- **Storage providers** - Choose between in-memory (testing) or Bun/PostgreSQL (production)
- **Dependency injection** - Connect your logger, cache, queue, and other services
- **Command pattern** - Expose notification operations via `go-command` for HTTP/gRPC
- **Activity hooks** - Integrate with audit logging and observability systems
- **Testing** - Use in-memory repositories for fast, isolated tests

---

## Module Initialization

The `Module` is the main entry point that assembles all services:

```go
import (
    "context"

    i18n "github.com/goliatone/go-i18n"
    "github.com/goliatone/go-notifications/pkg/adapters"
    "github.com/goliatone/go-notifications/pkg/adapters/console"
    "github.com/goliatone/go-notifications/pkg/adapters/sendgrid"
    "github.com/goliatone/go-notifications/pkg/config"
    "github.com/goliatone/go-notifications/pkg/notifier"
    "github.com/goliatone/go-notifications/pkg/storage"
)

func InitNotifications(ctx context.Context, db *bun.DB) (*notifier.Module, error) {
    // Create translator for template localization
    translator, err := i18n.New(i18n.Config{
        DefaultLocale: "en",
        LocalesPath:   "./locales",
    })
    if err != nil {
        return nil, err
    }

    // Configure adapters
    consoleAdapter := console.New(myLogger)
    sendgridAdapter, err := sendgrid.New(sendgrid.Options{
        APIKey: os.Getenv("SENDGRID_API_KEY"),
        From:   "noreply@example.com",
    })
    if err != nil {
        return nil, err
    }

    // Initialize module
    mod, err := notifier.NewModule(notifier.ModuleOptions{
        Config:     config.Defaults(),
        Storage:    storage.NewBunProviders(db),
        Logger:     myLogger,
        Translator: translator,
        Adapters:   []adapters.Messenger{consoleAdapter, sendgridAdapter},
    })
    if err != nil {
        return nil, err
    }

    return mod, nil
}
```

### ModuleOptions

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `Config` | `config.Config` | No | Module configuration (uses defaults if omitted) |
| `Storage` | `storage.Providers` | No | Repository providers (defaults to in-memory) |
| `Logger` | `logger.Logger` | No | Structured logger (defaults to no-op) |
| `Cache` | `cache.Cache` | No | Template cache (defaults to no-op) |
| `Translator` | `i18n.Translator` | **Yes** | Required for template rendering |
| `Fallbacks` | `i18n.FallbackResolver` | No | Locale fallback chain resolver |
| `Queue` | `queue.Queue` | No | Job queue for scheduled delivery |
| `Broadcaster` | `broadcaster.Broadcaster` | No | Real-time event broadcaster |
| `Adapters` | `[]adapters.Messenger` | No | Delivery channel adapters |
| `Secrets` | `secrets.Resolver` | No | Credentials resolver |
| `Activity` | `activity.Hooks` | No | Observability hooks |

---

## Configuration via cfgx

Load configuration from environment or config files using `cfgx`:

```go
import (
    "github.com/goliatone/go-config/cfgx"
    "github.com/goliatone/go-notifications/pkg/config"
)

// From environment variables
cfg, err := config.Load(map[string]any{
    "localization": map[string]any{
        "default_locale": os.Getenv("NOTIFICATIONS_LOCALE"),
    },
    "dispatcher": map[string]any{
        "max_retries": 5,
        "max_workers": 8,
    },
    "templates": map[string]any{
        "cache_ttl": "5m",
    },
})

// Or use defaults
cfg := config.Defaults()
```

### Configuration Structure

```go
type Config struct {
    Localization LocalizationConfig `mapstructure:"localization"`
    Dispatcher   DispatcherConfig   `mapstructure:"dispatcher"`
    Inbox        InboxConfig        `mapstructure:"inbox"`
    Templates    TemplateConfig     `mapstructure:"templates"`
    Realtime     RealtimeConfig     `mapstructure:"realtime"`
    Options      OptionsConfig      `mapstructure:"options"`
}

// Defaults
config.Defaults() == Config{
    Localization: LocalizationConfig{DefaultLocale: "en"},
    Dispatcher: DispatcherConfig{
        Enabled:    true,
        MaxRetries: 3,
        MaxWorkers: 4,
    },
    Inbox:     InboxConfig{Enabled: true},
    Templates: TemplateConfig{CacheTTL: time.Minute},
    Realtime:  RealtimeConfig{Enabled: true},
}
```

---

## Storage Providers

### In-Memory (Development/Testing)

```go
import "github.com/goliatone/go-notifications/pkg/storage"

// Auto-selected when Storage is nil
mod, _ := notifier.NewModule(notifier.ModuleOptions{
    Translator: translator,
    // Storage omitted = in-memory
})

// Or explicitly
providers := storage.NewMemoryProviders()
mod, _ := notifier.NewModule(notifier.ModuleOptions{
    Storage:    providers,
    Translator: translator,
})
```

### Bun/PostgreSQL (Production)

```go
import (
    "database/sql"

    "github.com/goliatone/go-notifications/pkg/storage"
    persistence "github.com/goliatone/go-persistence-bun"
    "github.com/uptrace/bun"
    "github.com/uptrace/bun/dialect/pgdialect"
    "github.com/uptrace/bun/driver/pgdriver"
)

func NewDatabase() (*bun.DB, error) {
    dsn := os.Getenv("DATABASE_URL")
    sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
    db := bun.NewDB(sqldb, pgdialect.New())
    return db, nil
}

func InitWithPostgres(db *bun.DB) (*notifier.Module, error) {
    providers := storage.NewBunProviders(db)

    return notifier.NewModule(notifier.ModuleOptions{
        Storage:    providers,
        Translator: translator,
        Adapters:   adapters,
    })
}
```

### Running Migrations

```go
import persistence "github.com/goliatone/go-persistence-bun"

func RunMigrations(ctx context.Context, db *bun.DB) error {
    // Models are auto-registered by NewBunProviders
    migrator := persistence.NewMigrator(db)
    return migrator.Migrate(ctx)
}
```

### Available Repositories

```go
type Providers struct {
    Definitions        store.NotificationDefinitionRepository
    Templates          store.NotificationTemplateRepository
    Events             store.NotificationEventRepository
    Messages           store.NotificationMessageRepository
    DeliveryAttempts   store.DeliveryAttemptRepository
    Preferences        store.NotificationPreferenceRepository
    SubscriptionGroups store.SubscriptionGroupRepository
    Inbox              store.InboxRepository
    Transaction        store.TransactionManager
}
```

---

## Dependency Injection Setup

### Logger Interface

Implement the `Logger` interface to integrate with your logging system:

```go
import "github.com/goliatone/go-notifications/pkg/interfaces/logger"

type MyLogger struct {
    // Your logger (zap, logrus, etc.)
}

func (l *MyLogger) With(fields ...logger.Field) logger.Logger {
    // Return logger with fields attached
}

func (l *MyLogger) Debug(msg string, fields ...logger.Field) {
    // Log at debug level
}

func (l *MyLogger) Info(msg string, fields ...logger.Field) {
    // Log at info level
}

func (l *MyLogger) Warn(msg string, fields ...logger.Field) {
    // Log at warn level
}

func (l *MyLogger) Error(msg string, fields ...logger.Field) {
    // Log at error level
}
```

### Cache Interface

For template caching:

```go
import "github.com/goliatone/go-notifications/pkg/interfaces/cache"

type RedisCache struct {
    client *redis.Client
}

func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
    return c.client.Get(ctx, key).Bytes()
}

func (c *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
    return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *RedisCache) Delete(ctx context.Context, key string) error {
    return c.client.Del(ctx, key).Err()
}
```

### Queue Interface

For scheduled/async delivery:

```go
import "github.com/goliatone/go-notifications/pkg/interfaces/queue"

type SQSQueue struct {
    client *sqs.Client
    url    string
}

func (q *SQSQueue) Enqueue(ctx context.Context, job queue.Job) error {
    payload, _ := json.Marshal(job)
    _, err := q.client.SendMessage(ctx, &sqs.SendMessageInput{
        QueueUrl:     &q.url,
        MessageBody:  aws.String(string(payload)),
        DelaySeconds: int32(time.Until(job.RunAt).Seconds()),
    })
    return err
}
```

### Broadcaster Interface

For real-time inbox updates:

```go
import "github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"

type WebSocketBroadcaster struct {
    hub *websocket.Hub
}

func (b *WebSocketBroadcaster) Broadcast(ctx context.Context, channel string, event any) error {
    payload, _ := json.Marshal(event)
    return b.hub.Broadcast(channel, payload)
}
```

---

## Command Pattern (go-command)

Register notification commands with your application's command registry:

```go
import (
    command "github.com/goliatone/go-command"
    "github.com/goliatone/go-notifications/pkg/commands"
)

func RegisterCommands(registry *command.Registry, mod *notifier.Module) error {
    cmds := mod.Commands()

    // Register all commands
    for _, cmd := range cmds.Commanders() {
        if err := registry.RegisterCommand(cmd); err != nil {
            return err
        }
    }

    return nil
}
```

### Available Commands

| Command | Input Type | Description |
|---------|------------|-------------|
| `CreateDefinition` | `commands.CreateDefinition` | Create/update notification definitions |
| `SaveTemplate` | `commands.TemplateUpsert` | Create/update templates |
| `UpsertPreference` | `preferences.PreferenceInput` | Set user preferences |
| `InboxMarkRead` | `commands.InboxMarkRead` | Mark items read/unread |
| `InboxDismiss` | `commands.InboxDismiss` | Dismiss inbox items |
| `InboxSnooze` | `commands.InboxSnooze` | Snooze items |
| `EnqueueEvent` | `events.IntakeRequest` | Queue notification events |

### HTTP Handler Example

```go
import (
    "encoding/json"
    "net/http"

    "github.com/goliatone/go-notifications/pkg/commands"
)

func CreateDefinitionHandler(mod *notifier.Module) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var input commands.CreateDefinition
        if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
            http.Error(w, err.Error(), http.StatusBadRequest)
            return
        }

        err := mod.Commands().CreateDefinition.Execute(r.Context(), input)
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        w.WriteHeader(http.StatusCreated)
    }
}
```

---

## Integrating with go-admin

Expose notification management in admin interfaces:

```go
import (
    "github.com/goliatone/go-notifications/pkg/domain"
    "github.com/goliatone/go-notifications/pkg/interfaces/store"
)

// List definitions for admin table
func ListDefinitionsHandler(repo store.NotificationDefinitionRepository) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        result, err := repo.List(r.Context(), store.ListOptions{
            Limit:  100,
            Offset: 0,
        })
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        json.NewEncoder(w).Encode(result)
    }
}

// Get definition for edit form
func GetDefinitionHandler(repo store.NotificationDefinitionRepository) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        code := r.URL.Query().Get("code")
        def, err := repo.GetByCode(r.Context(), code)
        if err != nil {
            http.Error(w, err.Error(), http.StatusNotFound)
            return
        }
        json.NewEncoder(w).Encode(def)
    }
}
```

---

## Integrating with go-cms

Use go-cms blocks as template sources:

```go
import (
    "github.com/goliatone/go-notifications/adapters/gocms"
    "github.com/goliatone/go-notifications/pkg/templates"
)

// Convert CMS block to notification templates
func ImportBlockAsTemplate(
    ctx context.Context,
    tplSvc *templates.Service,
    blockSnapshot gocms.BlockVersionSnapshot,
) error {
    spec := gocms.TemplateSpec{
        Code:           "welcome-email",
        Channel:        "email",
        SubjectKey:     "email_subject",
        BodyKey:        "email_body",
        DescriptionKey: "description",
    }

    inputs, err := gocms.TemplatesFromBlockSnapshot(spec, blockSnapshot)
    if err != nil {
        return err
    }

    for _, input := range inputs {
        if _, err := tplSvc.Create(ctx, input); err != nil {
            return err
        }
    }

    return nil
}
```

### Template Source Reference

Templates can reference CMS blocks via `Source` metadata:

```go
template := &domain.NotificationTemplate{
    Code:    "welcome",
    Channel: "email",
    Source: domain.TemplateSource{
        Type:      "gocms",
        Reference: "block:welcome-notification:v1",
        Payload: domain.JSONMap{
            "subject_key": "email_subject",
            "body_key":    "email_body",
        },
    },
}
```

---

## Logging and Observability

### Activity Hooks

Capture notification lifecycle events for audit logging:

```go
import "github.com/goliatone/go-notifications/pkg/activity"

type AuditLogger struct {
    db *sql.DB
}

func (l *AuditLogger) Notify(ctx context.Context, evt activity.Event) {
    l.db.ExecContext(ctx, `
        INSERT INTO audit_log (verb, actor_id, object_type, object_id, metadata, occurred_at)
        VALUES ($1, $2, $3, $4, $5, $6)
    `, evt.Verb, evt.ActorID, evt.ObjectType, evt.ObjectID, evt.Metadata, evt.OccurredAt)
}

// Register during module initialization
mod, _ := notifier.NewModule(notifier.ModuleOptions{
    Activity: activity.Hooks{&AuditLogger{db: db}},
    // ...
})
```

### Activity Event Structure

```go
type Event struct {
    Verb           string         // "notification.created", "notification.delivered", etc.
    ActorID        string         // Who triggered the action
    UserID         string         // Target user
    TenantID       string         // Tenant context
    ObjectType     string         // "notification_event", "notification_message"
    ObjectID       string         // Entity UUID
    Channel        string         // Delivery channel
    DefinitionCode string         // Notification type
    Recipients     []string       // Target recipients
    Metadata       map[string]any // Additional context
    OccurredAt     time.Time      // Event timestamp
}
```

### Metrics Collection

```go
import "github.com/goliatone/go-notifications/pkg/storage"

type PrometheusCollector struct {
    histogram *prometheus.HistogramVec
}

func (c *PrometheusCollector) Record(operation string, labels map[string]string) {
    c.histogram.With(prometheus.Labels{
        "operation": operation,
        "entity":    labels["entity"],
    }).Observe(labels["duration_ms"])
}

// Use with storage providers
providers := storage.NewBunProviders(db, storage.WithMetricsCollector(&PrometheusCollector{}))
```

---

## Testing with In-Memory Repositories

### Unit Test Setup

```go
import (
    "context"
    "testing"

    i18n "github.com/goliatone/go-i18n"
    "github.com/goliatone/go-notifications/internal/storage/memory"
    "github.com/goliatone/go-notifications/pkg/adapters"
    "github.com/goliatone/go-notifications/pkg/adapters/console"
    "github.com/goliatone/go-notifications/pkg/domain"
    "github.com/goliatone/go-notifications/pkg/interfaces/cache"
    "github.com/goliatone/go-notifications/pkg/interfaces/logger"
    "github.com/goliatone/go-notifications/pkg/notifier"
    "github.com/goliatone/go-notifications/pkg/storage"
    "github.com/goliatone/go-notifications/pkg/templates"
)

func TestNotificationDelivery(t *testing.T) {
    ctx := context.Background()

    // Create in-memory repositories
    providers := storage.NewMemoryProviders()

    // Create mock translator
    translator, _ := i18n.New()

    // Create test adapter that captures sends
    var sentMessages []adapters.Message
    testAdapter := &mockAdapter{
        onSend: func(msg adapters.Message) {
            sentMessages = append(sentMessages, msg)
        },
    }

    // Initialize module
    mod, err := notifier.NewModule(notifier.ModuleOptions{
        Storage:    providers,
        Translator: translator,
        Adapters:   []adapters.Messenger{testAdapter},
    })
    if err != nil {
        t.Fatal(err)
    }

    // Setup test data
    providers.Definitions.Create(ctx, &domain.NotificationDefinition{
        Code:     "test",
        Channels: domain.StringList{"test"},
    })
    providers.Templates.Create(ctx, &domain.NotificationTemplate{
        Code:    "test",
        Channel: "test",
        Subject: "Hello {{ name }}",
        Body:    "Test body",
    })

    // Send notification
    err = mod.Manager().Send(ctx, notifier.Event{
        DefinitionCode: "test",
        Recipients:     []string{"user@test.com"},
        Context:        map[string]any{"name": "Test"},
    })
    if err != nil {
        t.Fatal(err)
    }

    // Assert
    if len(sentMessages) != 1 {
        t.Fatalf("expected 1 message, got %d", len(sentMessages))
    }
    if sentMessages[0].Subject != "Hello Test" {
        t.Errorf("unexpected subject: %s", sentMessages[0].Subject)
    }
}
```

### Mock Adapter

```go
type mockAdapter struct {
    onSend func(msg adapters.Message)
}

func (m *mockAdapter) Name() string { return "test" }

func (m *mockAdapter) Channels() []string { return []string{"test"} }

func (m *mockAdapter) Matches(channel string) bool {
    return channel == "test"
}

func (m *mockAdapter) Send(ctx context.Context, msg adapters.Message) error {
    if m.onSend != nil {
        m.onSend(msg)
    }
    return nil
}
```

### Integration Test with Database

```go
func TestWithDatabase(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping database test")
    }

    ctx := context.Background()

    // Use test database
    db := setupTestDB(t)
    defer db.Close()

    providers := storage.NewBunProviders(db)

    mod, err := notifier.NewModule(notifier.ModuleOptions{
        Storage:    providers,
        Translator: translator,
        Adapters:   []adapters.Messenger{console.New(nil)},
    })
    if err != nil {
        t.Fatal(err)
    }

    // Test with real database
    // ...
}
```

---

## Complete Integration Example

```go
package main

import (
    "context"
    "database/sql"
    "log"
    "net/http"
    "os"

    i18n "github.com/goliatone/go-i18n"
    "github.com/goliatone/go-notifications/pkg/activity"
    "github.com/goliatone/go-notifications/pkg/adapters"
    "github.com/goliatone/go-notifications/pkg/adapters/sendgrid"
    "github.com/goliatone/go-notifications/pkg/config"
    "github.com/goliatone/go-notifications/pkg/notifier"
    "github.com/goliatone/go-notifications/pkg/storage"
    "github.com/uptrace/bun"
    "github.com/uptrace/bun/dialect/pgdialect"
    "github.com/uptrace/bun/driver/pgdriver"
)

func main() {
    ctx := context.Background()

    // Database
    sqldb := sql.OpenDB(pgdriver.NewConnector(
        pgdriver.WithDSN(os.Getenv("DATABASE_URL")),
    ))
    db := bun.NewDB(sqldb, pgdialect.New())
    defer db.Close()

    // Translator
    translator, err := i18n.New(i18n.Config{
        DefaultLocale: "en",
        LocalesPath:   "./locales",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Adapters
    sg, err := sendgrid.New(sendgrid.Options{
        APIKey: os.Getenv("SENDGRID_API_KEY"),
        From:   "notifications@example.com",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Activity hooks
    hooks := activity.Hooks{
        &LoggingHook{},
    }

    // Configuration
    cfg, err := config.Load(map[string]any{
        "dispatcher": map[string]any{
            "max_workers": 8,
            "max_retries": 5,
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    // Initialize module
    mod, err := notifier.NewModule(notifier.ModuleOptions{
        Config:     cfg,
        Storage:    storage.NewBunProviders(db),
        Translator: translator,
        Adapters:   []adapters.Messenger{sg},
        Activity:   hooks,
    })
    if err != nil {
        log.Fatal(err)
    }

    // Access services
    manager := mod.Manager()
    templates := mod.Templates()
    preferences := mod.Preferences()
    inbox := mod.Inbox()
    commands := mod.Commands()

    // Example: Send notification
    err = manager.Send(ctx, notifier.Event{
        DefinitionCode: "welcome",
        Recipients:     []string{"user@example.com"},
        Context:        map[string]any{"name": "Alice"},
    })
    if err != nil {
        log.Printf("Failed to send: %v", err)
    }

    // Start HTTP server with notification endpoints
    http.HandleFunc("/api/notifications/send", SendHandler(manager))
    log.Fatal(http.ListenAndServe(":8080", nil))
}

type LoggingHook struct{}

func (h *LoggingHook) Notify(ctx context.Context, evt activity.Event) {
    log.Printf("[%s] %s %s/%s", evt.Verb, evt.DefinitionCode, evt.ObjectType, evt.ObjectID)
}

func SendHandler(manager *notifier.Manager) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Handle notification send requests
    }
}
```

---

## Next Steps

- [GUIDE_GETTING_STARTED.md](GUIDE_GETTING_STARTED.md) - Quick start guide
- [GUIDE_ADAPTERS.md](GUIDE_ADAPTERS.md) - Configure delivery channels
- [GUIDE_TEMPLATES.md](GUIDE_TEMPLATES.md) - Template rendering
- [GUIDE_SECRETS.md](GUIDE_SECRETS.md) - Credentials management
