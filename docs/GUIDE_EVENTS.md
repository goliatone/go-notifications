# Events Guide

This guide covers how to submit notification events and understand the delivery pipeline in `go-notifications`.

---

## Overview

Events are the entry point for triggering notifications. When you submit an event:

1. The event is validated against a registered definition
2. A `NotificationEvent` record is persisted
3. The dispatcher expands the event into per-recipient messages
4. Templates are rendered for each channel
5. Adapters deliver the rendered messages
6. Delivery attempts are tracked for retry and auditing

The system supports both **synchronous** (immediate) and **asynchronous** (scheduled/queued) event submission.

---

## Event Structure

An event encapsulates all the data needed to trigger a notification:

```go
import "github.com/goliatone/go-notifications/pkg/notifier"

event := notifier.Event{
    // Required: references a NotificationDefinition
    DefinitionCode: "order-shipped",

    // Required: list of recipient identifiers
    Recipients: []string{"user@example.com", "user-456"},

    // Template context data for rendering
    Context: map[string]any{
        "order_id":     "ORD-12345",
        "tracking_url": "https://tracking.example.com/ORD-12345",
        "customer":     "Alice",
    },

    // Optional: override definition channels
    Channels: []string{"email", "sms"},

    // Optional: tenant identifier for multi-tenant apps
    TenantID: "tenant-abc",

    // Optional: actor who triggered the event
    ActorID: "system",

    // Optional: locale override for template rendering
    Locale: "es",

    // Optional: schedule for future delivery
    ScheduledAt: time.Now().Add(24 * time.Hour),
}
```

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `DefinitionCode` | `string` | References a registered `NotificationDefinition` |
| `Recipients` | `[]string` | One or more recipient identifiers (email, phone, user ID, etc.) |

### Optional Fields

| Field | Type | Description |
|-------|------|-------------|
| `Context` | `map[string]any` | Template variables for rendering |
| `Channels` | `[]string` | Override definition's default channels |
| `TenantID` | `string` | Tenant identifier for scoped delivery |
| `ActorID` | `string` | Actor who triggered the notification |
| `Locale` | `string` | Override default locale for rendering |
| `ScheduledAt` | `time.Time` | Delay delivery until this time |

---

## Sending Events

### Via Manager (Recommended)

The `Manager` provides the simplest interface for sending notifications:

```go
import (
    "context"
    "github.com/goliatone/go-notifications/pkg/notifier"
)

func SendOrderShipped(ctx context.Context, manager *notifier.Manager, orderID, customerEmail string) error {
    return manager.Send(ctx, notifier.Event{
        DefinitionCode: "order-shipped",
        Recipients:     []string{customerEmail},
        Context: map[string]any{
            "order_id": orderID,
        },
    })
}
```

The Manager:
1. Validates required fields
2. Persists a `NotificationEvent` record
3. Invokes the dispatcher for immediate delivery
4. Updates event status based on delivery outcome

### Via Events Service

For advanced use cases (digests, scheduled delivery), use the Events service directly:

```go
import (
    "context"
    "time"
    "github.com/goliatone/go-notifications/pkg/events"
)

func EnqueueEvent(ctx context.Context, svc *events.Service) error {
    return svc.Enqueue(ctx, events.IntakeRequest{
        DefinitionCode: "weekly-digest",
        Recipients:     []string{"user@example.com"},
        Context: map[string]any{
            "items": []string{"item1", "item2"},
        },
        // Schedule for next Monday at 9am
        ScheduleAt: nextMonday9AM(),
    })
}
```

---

## Sync vs Async Submission

### Synchronous (Immediate)

By default, events are processed synchronously:

```go
// Blocks until delivery completes or fails
err := manager.Send(ctx, notifier.Event{
    DefinitionCode: "password-reset",
    Recipients:     []string{"user@example.com"},
    Context:        map[string]any{"reset_link": link},
})
if err != nil {
    // Handle delivery failure
}
```

### Asynchronous (Scheduled)

Set `ScheduledAt` to defer delivery:

```go
// Returns immediately; delivery happens later
err := manager.Send(ctx, notifier.Event{
    DefinitionCode: "reminder",
    Recipients:     []string{"user@example.com"},
    ScheduledAt:    time.Now().Add(1 * time.Hour),
})
```

Scheduled events require a Queue implementation:

```go
import (
    "github.com/goliatone/go-notifications/pkg/interfaces/queue"
)

type MyQueue struct {
    // Your queue backend (Redis, SQS, etc.)
}

func (q *MyQueue) Enqueue(ctx context.Context, job queue.Job) error {
    // Persist job.Key, job.Payload, job.RunAt
    // Worker polls and calls ProcessScheduled when RunAt is reached
    return nil
}
```

---

## Payload Validation

Events are validated against their definition before processing:

```go
// This will fail if "order-shipped" definition doesn't exist
err := manager.Send(ctx, notifier.Event{
    DefinitionCode: "order-shipped", // Must exist in definitions repository
    Recipients:     []string{},       // Will fail: at least one recipient required
})
// Error: "notifier: at least one recipient is required"
```

### Template Schema Validation

When templates define a schema, the Context is validated:

```go
// Template with schema: {"required": ["order_id"], "optional": ["tracking_url"]}

err := manager.Send(ctx, notifier.Event{
    DefinitionCode: "order-shipped",
    Recipients:     []string{"user@example.com"},
    Context: map[string]any{
        // Missing required "order_id" - will cause rendering to fail
        "tracking_url": "https://...",
    },
})
```

---

## Recipient Resolution

Recipients are identifiers that adapters know how to route:

```go
// Email adapter expects email addresses
event := notifier.Event{
    DefinitionCode: "welcome",
    Recipients:     []string{"alice@example.com", "bob@example.com"},
}

// SMS adapter expects phone numbers
event := notifier.Event{
    DefinitionCode: "verification-code",
    Recipients:     []string{"+1234567890"},
    Channels:       []string{"sms"},
}

// Multi-channel with different identifier types
event := notifier.Event{
    DefinitionCode: "order-update",
    Recipients:     []string{"user-123"}, // User ID looked up by adapter
    Channels:       []string{"email", "push", "inbox"},
}
```

### Recipient in Context

You can pass additional recipient metadata via Context:

```go
event := notifier.Event{
    DefinitionCode: "welcome",
    Recipients:     []string{"user-123"},
    Context: map[string]any{
        "recipient_email": "alice@example.com",
        "recipient_phone": "+1234567890",
        "recipient_name":  "Alice",
    },
}
```

The dispatcher automatically adds `recipient` to the Context for template rendering:

```django
Hello {{ recipient_name }}, your order is on the way!
Delivery to: {{ recipient }}
```

---

## Scheduled Delivery

Use `ScheduledAt` to delay notification delivery:

```go
import "time"

// Send reminder 24 hours before appointment
err := manager.Send(ctx, notifier.Event{
    DefinitionCode: "appointment-reminder",
    Recipients:     []string{"patient@example.com"},
    Context: map[string]any{
        "appointment_time": appointmentTime.Format(time.RFC1123),
        "doctor_name":      "Dr. Smith",
    },
    ScheduledAt: appointmentTime.Add(-24 * time.Hour),
})
```

### How Scheduling Works

1. Event is validated and persisted
2. A job is enqueued with `RunAt` set to `ScheduledAt`
3. Queue worker picks up the job when time arrives
4. `ProcessScheduled` is called to dispatch the event

```go
// Worker code (runs in background)
func (w *Worker) ProcessJob(ctx context.Context, job queue.Job) error {
    switch payload := job.Payload.(type) {
    case events.ScheduledJobPayload:
        return w.eventsService.ProcessScheduled(ctx, payload)
    case events.DigestJobPayload:
        return w.eventsService.ProcessDigest(ctx, payload)
    }
    return nil
}
```

---

## Digest Grouping

Digests batch multiple events before sending a single notification:

```go
import (
    "time"
    "github.com/goliatone/go-notifications/pkg/events"
)

// First activity event - starts the batch
err := svc.Enqueue(ctx, events.IntakeRequest{
    DefinitionCode: "activity-digest",
    Recipients:     []string{"user@example.com"},
    Context: map[string]any{
        "action": "liked your post",
        "actor":  "Alice",
    },
    Digest: &events.DigestOptions{
        Key:   "user@example.com:activity", // Unique batch identifier
        Delay: 5 * time.Minute,             // Wait before sending
    },
})

// Second activity event - added to same batch
err = svc.Enqueue(ctx, events.IntakeRequest{
    DefinitionCode: "activity-digest",
    Recipients:     []string{"user@example.com"},
    Context: map[string]any{
        "action": "commented on your post",
        "actor":  "Bob",
    },
    Digest: &events.DigestOptions{
        Key:   "user@example.com:activity", // Same key = same batch
        Delay: 5 * time.Minute,
    },
})
```

After the delay, a single notification is sent with merged context:

```go
// Merged context available in template:
{
    "digest": {
        "count": 2,
        "entries": [
            {"action": "liked your post", "actor": "Alice"},
            {"action": "commented on your post", "actor": "Bob"}
        ]
    }
}
```

### Digest Template Example

```django
You have {{ digest.count }} new activities:

{% for entry in digest.entries %}
- {{ entry.actor }} {{ entry.action }}
{% endfor %}
```

---

## Event Lifecycle

Events progress through these statuses:

| Status | Description |
|--------|-------------|
| `pending` | Event created, awaiting dispatch |
| `scheduled` | Queued for future delivery |
| `processed` | All deliveries completed successfully |
| `failed` | One or more deliveries failed |

```go
import "github.com/goliatone/go-notifications/pkg/domain"

const (
    EventStatusPending   = "pending"
    EventStatusScheduled = "scheduled"
    EventStatusProcessed = "processed"
    EventStatusFailed    = "failed"
)
```

### Tracking Event Status

```go
import (
    "github.com/goliatone/go-notifications/pkg/interfaces/store"
    "github.com/google/uuid"
)

func GetEventStatus(ctx context.Context, repo store.NotificationEventRepository, eventID uuid.UUID) (string, error) {
    event, err := repo.Get(ctx, eventID)
    if err != nil {
        return "", err
    }
    return event.Status, nil
}
```

---

## Delivery Attempts and Retries

The dispatcher automatically retries failed deliveries with exponential backoff:

```go
import "github.com/goliatone/go-notifications/pkg/config"

dispatcherConfig := config.DispatcherConfig{
    MaxRetries: 3,         // Retry up to 3 times
    MaxWorkers: 4,         // Parallel delivery workers
}
```

### Retry Behavior

1. First attempt: immediate
2. Second attempt: 100ms delay
3. Third attempt: 200ms delay
4. After max retries: message marked as `failed`

### Delivery Attempt Records

Each adapter execution is logged:

```go
import "github.com/goliatone/go-notifications/pkg/domain"

type DeliveryAttempt struct {
    ID        uuid.UUID
    MessageID uuid.UUID // Links to NotificationMessage
    Adapter   string    // e.g., "sendgrid", "twilio"
    Status    string    // "pending", "succeeded", "failed"
    Error     string    // Error message if failed
    Payload   JSONMap   // Attempt metadata
    CreatedAt time.Time
}
```

### Querying Delivery History

```go
func GetDeliveryHistory(ctx context.Context, repo store.DeliveryAttemptRepository, messageID uuid.UUID) ([]*domain.DeliveryAttempt, error) {
    return repo.ListByMessage(ctx, messageID)
}
```

---

## Multi-Channel Fan-Out

Events can target multiple channels simultaneously:

```go
err := manager.Send(ctx, notifier.Event{
    DefinitionCode: "order-shipped",
    Recipients:     []string{"user@example.com"},
    Channels:       []string{"email", "sms", "push", "inbox"},
    Context: map[string]any{
        "order_id": "ORD-12345",
        // Phone number for SMS
        "phone": "+1234567890",
        // Device token for push
        "device_token": "fcm-token-xxx",
    },
})
```

### Channel-Specific Overrides

Customize content per channel via `channel_overrides`:

```go
event := notifier.Event{
    DefinitionCode: "promo",
    Recipients:     []string{"user@example.com"},
    Channels:       []string{"email", "sms", "push"},
    Context: map[string]any{
        "promo_code": "SAVE20",
        "channel_overrides": map[string]any{
            "email": map[string]any{
                "subject": "Exclusive Email Offer!",
            },
            "sms": map[string]any{
                "body": "Use code SAVE20 for 20% off!",
            },
            "push": map[string]any{
                "body": "Tap for your exclusive offer",
                "action_url": "myapp://promo/SAVE20",
            },
        },
    },
}
```

---

## Activity Hooks

Events emit activity signals for observability:

```go
import "github.com/goliatone/go-notifications/pkg/activity"

type MyActivityLogger struct{}

func (l *MyActivityLogger) Notify(ctx context.Context, evt activity.Event) {
    log.Printf("[%s] %s %s/%s to %v",
        evt.Verb,           // e.g., "notification.created", "notification.delivered"
        evt.DefinitionCode,
        evt.ObjectType,
        evt.ObjectID,
        evt.Recipients,
    )
}

// Register during module setup
mod, _ := notifier.NewModule(notifier.ModuleOptions{
    Activity: &MyActivityLogger{},
})
```

### Activity Event Verbs

| Verb | Trigger |
|------|---------|
| `notification.created` | Event persisted |
| `notification.delivered` | Delivery succeeded |
| `notification.failed` | Delivery failed after retries |

---

## Complete Example

```go
package main

import (
    "context"
    "log"
    "time"

    i18n "github.com/goliatone/go-i18n"
    "github.com/goliatone/go-notifications/pkg/adapters"
    "github.com/goliatone/go-notifications/pkg/adapters/console"
    "github.com/goliatone/go-notifications/pkg/domain"
    "github.com/goliatone/go-notifications/pkg/notifier"
)

func main() {
    ctx := context.Background()

    // Setup module
    translator, _ := i18n.New()
    consoleAdapter, _ := console.New(nil)

    mod, err := notifier.NewModule(notifier.ModuleOptions{
        Translator: translator,
        Adapters:   []adapters.Messenger{consoleAdapter},
    })
    if err != nil {
        log.Fatal(err)
    }

    // Create definition
    defRepo := mod.Repositories().Definitions
    defRepo.Create(ctx, &domain.NotificationDefinition{
        Code:     "welcome",
        Name:     "Welcome Email",
        Channels: domain.StringList{"console"},
    })

    // Create template
    tplRepo := mod.Repositories().Templates
    tplRepo.Create(ctx, &domain.NotificationTemplate{
        Code:    "welcome",
        Channel: "console",
        Subject: "Welcome, {{ name }}!",
        Body:    "Thanks for joining us. Your account is ready.",
    })

    // Send notification
    manager := mod.Manager()

    err = manager.Send(ctx, notifier.Event{
        DefinitionCode: "welcome",
        Recipients:     []string{"alice@example.com"},
        Context: map[string]any{
            "name": "Alice",
        },
    })
    if err != nil {
        log.Printf("Failed to send: %v", err)
        return
    }

    log.Println("Notification sent successfully!")
}
```

---

## Troubleshooting

### Event Not Delivered

1. **Check definition exists**:
   ```go
   def, err := defRepo.GetByCode(ctx, "my-definition")
   ```

2. **Check channels configured**:
   ```go
   if len(def.Channels) == 0 {
       // No channels = no delivery
   }
   ```

3. **Check adapters registered**:
   ```go
   adapters := registry.List("email")
   if len(adapters) == 0 {
       // No adapter for channel
   }
   ```

### Scheduled Event Not Firing

1. **Verify queue implementation**: Ensure your Queue correctly persists and polls jobs
2. **Check worker is running**: ProcessScheduled must be called when job time arrives
3. **Verify ScheduleAt is in future**: Events scheduled in the past are sent immediately

### Digest Not Merging

1. **Use consistent Key**: All events in a batch must share the same `Digest.Key`
2. **Check Delay duration**: Ensure delay is long enough for events to accumulate
3. **Verify worker calls ProcessDigest**: Digest jobs require explicit processing

---

## Next Steps

- [GUIDE_DEFINITIONS.md](GUIDE_DEFINITIONS.md) - Creating notification types
- [GUIDE_TEMPLATES.md](GUIDE_TEMPLATES.md) - Template rendering and localization
- [GUIDE_PREFERENCES.md](GUIDE_PREFERENCES.md) - User opt-in/opt-out settings
- [GUIDE_ADAPTERS.md](GUIDE_ADAPTERS.md) - Configuring delivery channels
