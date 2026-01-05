# Definitions Guide

This guide covers creating and managing notification definitions in `go-notifications`. Definitions describe the types of notifications your application can send.

---

## What Are Notification Definitions?

A **NotificationDefinition** is a blueprint for a notification type. It describes:

- **What** the notification is (code, name, description)
- **How** it should be delivered (channels)
- **Where** the content comes from (template keys)
- **When** special rules apply (policies, throttling)

Think of definitions as notification "types" that you configure once and reference by code when sending events.

```go
import "github.com/goliatone/go-notifications/pkg/domain"

// Example: Order shipped notification definition
definition := domain.NotificationDefinition{
    Code:        "order-shipped",
    Name:        "Order Shipped",
    Description: "Sent when an order is shipped to the customer",
    Severity:    "info",
    Category:    "orders",
    Channels:    domain.StringList{"email", "sms", "push"},
    TemplateKeys: domain.StringList{
        "email:order-shipped-email",
        "sms:order-shipped-sms",
        "push:order-shipped-push",
    },
    Metadata: domain.JSONMap{
        "icon":     "package",
        "priority": "normal",
    },
    Policy: domain.JSONMap{
        "throttle": map[string]any{
            "max":    5,
            "window": "1h",
        },
    },
}
```

---

## Definition Fields

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `Code` | `string` | Unique identifier (e.g., `"order-shipped"`) |
| `Name` | `string` | Human-readable name for admin UIs |

### Optional Fields

| Field | Type | Description |
|-------|------|-------------|
| `Description` | `string` | Detailed explanation of the notification |
| `Severity` | `string` | Urgency level: `"info"`, `"warning"`, `"critical"` |
| `Category` | `string` | Grouping for organization (e.g., `"orders"`, `"auth"`) |
| `Channels` | `StringList` | Delivery channels: `"email"`, `"sms"`, `"push"`, `"inbox"` |
| `TemplateKeys` | `StringList` | Channel-to-template mappings |
| `Metadata` | `JSONMap` | Custom fields for your application |
| `Policy` | `JSONMap` | Throttling, digest, and delivery rules |

---

## Creating Definitions

### Via Repository (Direct)

```go
import (
    "context"
    "github.com/goliatone/go-notifications/pkg/domain"
    "github.com/goliatone/go-notifications/pkg/interfaces/store"
)

func CreateDefinition(ctx context.Context, repo store.NotificationDefinitionRepository) error {
    def := &domain.NotificationDefinition{
        Code:        "welcome",
        Name:        "Welcome Email",
        Description: "Sent to new users after registration",
        Severity:    "info",
        Category:    "onboarding",
        Channels:    domain.StringList{"email", "inbox"},
        TemplateKeys: domain.StringList{
            "email:welcome-email",
            "inbox:welcome-inbox",
        },
    }
    return repo.Create(ctx, def)
}
```

### Via Command Pattern

The command pattern integrates with `go-command` for HTTP/gRPC transports:

```go
import (
    "context"
    "github.com/goliatone/go-notifications/internal/commands"
)

func CreateViaCommand(ctx context.Context, catalog *commands.Catalog) error {
    return catalog.CreateDefinition.Execute(ctx, commands.CreateDefinition{
        Code:        "password-reset",
        Name:        "Password Reset",
        Description: "Sent when user requests password reset",
        Severity:    "warning",
        Category:    "auth",
        Channels:    []string{"email"},
        TemplateIDs: []string{"email:password-reset"},
        AllowUpdate: true, // Update if already exists
    })
}
```

### Via OnReady Helper

For common patterns like file export notifications:

```go
import (
    "context"
    "github.com/goliatone/go-notifications/pkg/onready"
)

func SetupExportNotification(ctx context.Context, deps onready.Dependencies) error {
    result, err := onready.Register(ctx, deps, onready.Options{
        Namespace:             "reports",
        DefinitionName:        "Export Ready",
        DefinitionDescription: "Sent when a report export is ready",
        Channels:              []string{"email", "in-app"},
        EmailSubject:          "Your export is ready",
        EmailBody:             "Click below to download your report.",
        InAppSubject:          "Export Ready",
        InAppBody:             "Your report is ready for download.",
    })
    if err != nil {
        return err
    }
    // result.DefinitionCode = "reports.export-ready"
    return nil
}
```

---

## Channel Enablement

Definitions specify which channels are enabled for delivery:

```go
// Single channel
def := &domain.NotificationDefinition{
    Code:     "sms-verification",
    Name:     "SMS Verification Code",
    Channels: domain.StringList{"sms"},
}

// Multiple channels
def := &domain.NotificationDefinition{
    Code:     "order-update",
    Name:     "Order Update",
    Channels: domain.StringList{"email", "sms", "push", "inbox"},
}
```

### Channel Types

| Channel | Description |
|---------|-------------|
| `email` | Email delivery (SMTP, SendGrid, etc.) |
| `sms` | SMS messages (Twilio, SNS, etc.) |
| `push` | Push notifications (Firebase, APNs) |
| `inbox` / `in-app` | In-app notification center |
| `slack` | Slack messages |
| `telegram` | Telegram messages |
| `whatsapp` | WhatsApp messages |
| `webhook` | HTTP webhook delivery |

### Channel Override at Send Time

Events can override definition channels:

```go
// Definition has: email, sms, push
// Event sends only via email
manager.Send(ctx, notifier.Event{
    DefinitionCode: "order-update",
    Channels:       []string{"email"}, // Override
    Recipients:     []string{"user@example.com"},
})
```

---

## Template Keys

Template keys map channels to template codes:

```go
def := &domain.NotificationDefinition{
    Code: "welcome",
    TemplateKeys: domain.StringList{
        "email:welcome-email",     // channel:template-code
        "sms:welcome-sms",
        "push:welcome-push",
        "inbox:welcome-inbox",
    },
}
```

### Template Key Format

```
channel:template-code
```

Examples:
- `"email:order-shipped-email"` - Use `order-shipped-email` template for email
- `"sms:order-sms"` - Use `order-sms` template for SMS
- `"push:order-push"` - Use `order-push` template for push notifications

### Fallback Behavior

If no channel-specific template is found, the dispatcher uses:
1. First matching template key
2. Definition code as template code

```go
// No specific mapping - uses "order-update" as template code
def := &domain.NotificationDefinition{
    Code: "order-update",
    // TemplateKeys omitted - falls back to Code
}
```

---

## Throttling Policies

The `Policy` field supports throttling to prevent notification spam:

```go
def := &domain.NotificationDefinition{
    Code: "login-alert",
    Name: "Login Alert",
    Policy: domain.JSONMap{
        "throttle": map[string]any{
            "max":    3,      // Maximum notifications
            "window": "1h",   // Time window
            "key":    "user", // Throttle key (per user)
        },
    },
}
```

### Throttle Configuration

| Key | Type | Description |
|-----|------|-------------|
| `max` | `int` | Maximum notifications in window |
| `window` | `string` | Time window: `"1m"`, `"1h"`, `"1d"` |
| `key` | `string` | Throttle scope: `"user"`, `"tenant"`, `"global"` |

### Digest Policies

Group multiple events before sending:

```go
def := &domain.NotificationDefinition{
    Code: "activity-digest",
    Name: "Activity Digest",
    Policy: domain.JSONMap{
        "digest": map[string]any{
            "delay":  "5m",           // Wait before sending
            "max":    100,            // Max events to batch
            "groupBy": "recipient",   // Group events by field
        },
    },
}
```

---

## Localization (i18n) Bindings

Definitions can specify locale-aware templates:

```go
def := &domain.NotificationDefinition{
    Code: "welcome",
    TemplateKeys: domain.StringList{
        "email:welcome-email",      // Default locale
        "email:welcome-email-es",   // Spanish
        "email:welcome-email-fr",   // French
    },
    Metadata: domain.JSONMap{
        "default_locale": "en",
        "supported_locales": []string{"en", "es", "fr", "de"},
    },
}
```

Templates are selected based on event locale:

```go
manager.Send(ctx, notifier.Event{
    DefinitionCode: "welcome",
    Recipients:     []string{"user@example.com"},
    Locale:         "es", // Use Spanish template
})
```

See [GUIDE_TEMPLATES.md](GUIDE_TEMPLATES.md) for detailed localization patterns.

---

## Definition Metadata

Use `Metadata` for custom fields your application needs:

```go
def := &domain.NotificationDefinition{
    Code: "payment-failed",
    Name: "Payment Failed",
    Metadata: domain.JSONMap{
        // UI customization
        "icon":       "credit-card-x",
        "color":      "#dc3545",

        // Categorization
        "tags":       []string{"billing", "urgent"},
        "priority":   "high",

        // Feature flags
        "require_confirmation": true,
        "allow_unsubscribe":    true,

        // Analytics
        "tracking_category": "billing",
    },
}
```

### Common Metadata Fields

| Field | Purpose |
|-------|---------|
| `icon` | Icon identifier for UI display |
| `color` | Accent color for notification cards |
| `priority` | Delivery priority hint |
| `tags` | Categorization labels |
| `require_confirmation` | User must acknowledge |
| `allow_unsubscribe` | Show opt-out option |

---

## Archiving and Soft Deletes

Definitions support soft deletion for audit trails:

```go
import "github.com/google/uuid"

func ArchiveDefinition(ctx context.Context, repo store.NotificationDefinitionRepository, id uuid.UUID) error {
    return repo.SoftDelete(ctx, id)
}
```

### Querying with Soft Deletes

```go
// Normal query - excludes soft-deleted
result, _ := repo.List(ctx, store.ListOptions{
    Limit: 100,
})

// Include soft-deleted for admin views
result, _ := repo.List(ctx, store.ListOptions{
    Limit:              100,
    IncludeSoftDeleted: true,
})
```

### Restoring Deleted Definitions

```go
func RestoreDefinition(ctx context.Context, repo store.NotificationDefinitionRepository, id uuid.UUID) error {
    def, err := repo.GetByID(ctx, id)
    if err != nil {
        return err
    }
    def.DeletedAt = time.Time{} // Clear deletion timestamp
    return repo.Update(ctx, def)
}
```

---

## Admin UI Integration

When building admin interfaces with `go-admin`:

### List Definitions

```go
func ListDefinitions(ctx context.Context, repo store.NotificationDefinitionRepository) ([]domain.NotificationDefinition, error) {
    result, err := repo.List(ctx, store.ListOptions{
        Limit: 100,
    })
    if err != nil {
        return nil, err
    }
    return result.Items, nil
}
```

### Definition Editor Form

```go
type DefinitionForm struct {
    Code        string   `json:"code" form:"code" validate:"required"`
    Name        string   `json:"name" form:"name" validate:"required"`
    Description string   `json:"description" form:"description"`
    Severity    string   `json:"severity" form:"severity"`
    Category    string   `json:"category" form:"category"`
    Channels    []string `json:"channels" form:"channels"`
}

func HandleDefinitionUpdate(ctx context.Context, repo store.NotificationDefinitionRepository, form DefinitionForm) error {
    def, err := repo.GetByCode(ctx, form.Code)
    if err != nil {
        return err
    }

    def.Name = form.Name
    def.Description = form.Description
    def.Severity = form.Severity
    def.Category = form.Category
    def.Channels = domain.StringList(form.Channels)

    return repo.Update(ctx, def)
}
```

### Guard Rails

Prevent accidental changes to critical definitions:

```go
func UpdateWithGuards(ctx context.Context, repo store.NotificationDefinitionRepository, def *domain.NotificationDefinition) error {
    // Prevent code changes
    existing, err := repo.GetByID(ctx, def.ID)
    if err != nil {
        return err
    }
    if existing.Code != def.Code {
        return errors.New("cannot change definition code")
    }

    // Require at least one channel
    if len(def.Channels) == 0 {
        return errors.New("at least one channel is required")
    }

    // Validate channels
    validChannels := map[string]bool{
        "email": true, "sms": true, "push": true,
        "inbox": true, "slack": true, "webhook": true,
    }
    for _, ch := range def.Channels {
        if !validChannels[strings.ToLower(ch)] {
            return fmt.Errorf("invalid channel: %s", ch)
        }
    }

    return repo.Update(ctx, def)
}
```

---

## Complete Example

```go
package main

import (
    "context"
    "log"

    i18n "github.com/goliatone/go-i18n"
    "github.com/goliatone/go-notifications/pkg/adapters"
    "github.com/goliatone/go-notifications/pkg/adapters/console"
    "github.com/goliatone/go-notifications/pkg/domain"
    "github.com/goliatone/go-notifications/pkg/notifier"
)

func main() {
    ctx := context.Background()

    // Initialize module
    translator, _ := i18n.New()
    consoleAdapter, _ := console.New(nil)

    mod, err := notifier.NewModule(notifier.ModuleOptions{
        Translator: translator,
        Adapters:   []adapters.Messenger{consoleAdapter},
    })
    if err != nil {
        log.Fatal(err)
    }

    defRepo := mod.Repositories().Definitions
    tplRepo := mod.Repositories().Templates

    // Create definitions
    definitions := []*domain.NotificationDefinition{
        {
            Code:        "welcome",
            Name:        "Welcome",
            Description: "Sent to new users",
            Severity:    "info",
            Category:    "onboarding",
            Channels:    domain.StringList{"console"},
            TemplateKeys: domain.StringList{"console:welcome"},
        },
        {
            Code:        "password-reset",
            Name:        "Password Reset",
            Description: "Sent when user requests password reset",
            Severity:    "warning",
            Category:    "auth",
            Channels:    domain.StringList{"console"},
            TemplateKeys: domain.StringList{"console:password-reset"},
            Policy: domain.JSONMap{
                "throttle": map[string]any{
                    "max":    3,
                    "window": "1h",
                },
            },
        },
        {
            Code:        "order-shipped",
            Name:        "Order Shipped",
            Description: "Sent when order ships",
            Severity:    "info",
            Category:    "orders",
            Channels:    domain.StringList{"console"},
            TemplateKeys: domain.StringList{"console:order-shipped"},
            Metadata: domain.JSONMap{
                "icon":     "package",
                "priority": "high",
            },
        },
    }

    for _, def := range definitions {
        if err := defRepo.Create(ctx, def); err != nil {
            log.Printf("Failed to create definition %s: %v", def.Code, err)
            continue
        }
        log.Printf("Created definition: %s", def.Code)
    }

    // Create matching templates
    templates := []*domain.NotificationTemplate{
        {
            Code:    "welcome",
            Channel: "console",
            Subject: "Welcome, {{ name }}!",
            Body:    "Thanks for joining. Your account is ready.",
        },
        {
            Code:    "password-reset",
            Channel: "console",
            Subject: "Password Reset Request",
            Body:    "Click here to reset: {{ reset_link }}",
        },
        {
            Code:    "order-shipped",
            Channel: "console",
            Subject: "Order {{ order_id }} Shipped!",
            Body:    "Track your package: {{ tracking_url }}",
        },
    }

    for _, tpl := range templates {
        if err := tplRepo.Create(ctx, tpl); err != nil {
            log.Printf("Failed to create template %s: %v", tpl.Code, err)
            continue
        }
        log.Printf("Created template: %s", tpl.Code)
    }

    // List all definitions
    result, _ := defRepo.List(ctx, store.ListOptions{Limit: 100})
    log.Printf("Total definitions: %d", result.Total)
    for _, def := range result.Items {
        log.Printf("  - %s (%s): %v", def.Code, def.Category, def.Channels)
    }
}
```

---

## Best Practices

### 1. Use Descriptive Codes

```go
// Good - clear and namespaced
"orders.order-shipped"
"auth.password-reset"
"billing.payment-failed"

// Avoid - too generic
"notification1"
"email"
"update"
```

### 2. Group by Category

```go
// Organize definitions by business domain
categories := []string{
    "auth",       // Login, password, 2FA
    "orders",     // Order lifecycle
    "billing",    // Payments, invoices
    "marketing",  // Promotions, newsletters
    "system",     // Maintenance, alerts
}
```

### 3. Set Appropriate Severity

```go
// info - Normal notifications
// warning - Requires attention
// critical - Immediate action needed

def.Severity = "critical" // Payment failed, security alert
def.Severity = "warning"  // Password reset, unusual activity
def.Severity = "info"     // Order shipped, welcome email
```

### 4. Plan Template Keys

```go
// Consistent naming convention
TemplateKeys: domain.StringList{
    "email:order-shipped-email",     // {channel}:{definition}-{channel}
    "sms:order-shipped-sms",
    "push:order-shipped-push",
}
```

---

## Next Steps

- [GUIDE_TEMPLATES.md](GUIDE_TEMPLATES.md) - Create templates for definitions
- [GUIDE_EVENTS.md](GUIDE_EVENTS.md) - Send events using definitions
- [GUIDE_PREFERENCES.md](GUIDE_PREFERENCES.md) - User opt-in/opt-out per definition
- [GUIDE_ADAPTERS.md](GUIDE_ADAPTERS.md) - Configure delivery channels
