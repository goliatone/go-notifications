# Getting Started Guide

This guide will help you send your first notification in under 5 minutes using `go-notifications`.

## Table of Contents

1. [Overview](#overview)
2. [Installation](#installation)
3. [Minimal Setup](#minimal-setup)
4. [Sending Your First Notification](#sending-your-first-notification)
5. [Understanding the Flow](#understanding-the-flow)
6. [Basic Configuration](#basic-configuration)
7. [Next Steps](#next-steps)

---

## Overview

`go-notifications` is a composable notification orchestration module for Go applications. It provides:

- **Multi-channel delivery**: Email, SMS, push, chat, in-app, and more
- **Template rendering**: Pongo2-based templates with localization support
- **Preference management**: User opt-in/opt-out controls
- **In-app inbox**: Notification center with read/unread tracking
- **Pluggable adapters**: Built-in adapters for SendGrid, Twilio, Slack, and more

The module is designed to integrate with `go-admin` and `go-cms` but works independently in any Go application.

---

## Installation

Add `go-notifications` to your project:

```bash
go get github.com/goliatone/go-notifications
```

---

## Minimal Setup

The fastest way to get started is using the **console adapter** with **in-memory storage**. This setup requires no external services and logs notifications to stdout.

```go
package main

import (
    "context"
    "log"

    i18n "github.com/goliatone/go-i18n"
    "github.com/goliatone/go-notifications/pkg/adapters"
    "github.com/goliatone/go-notifications/pkg/adapters/console"
    "github.com/goliatone/go-notifications/pkg/config"
    "github.com/goliatone/go-notifications/pkg/domain"
    "github.com/goliatone/go-notifications/pkg/interfaces/logger"
    "github.com/goliatone/go-notifications/pkg/notifier"
    "github.com/goliatone/go-notifications/pkg/storage"
    "github.com/goliatone/go-notifications/pkg/templates"
)

func main() {
    ctx := context.Background()

    // 1. Create in-memory storage providers
    providers := storage.NewMemoryProviders()

    // 2. Create a simple translator (required for template rendering)
    translator := newSimpleTranslator()

    // 3. Create a basic logger (defaults to fmt.Printf)
    lgr := logger.Default()

    // 4. Create the console adapter (logs via the logger)
    consoleAdapter := console.New(lgr)

    // 5. Initialize the notification module
    mod, err := notifier.NewModule(notifier.ModuleOptions{
        Config:     config.Defaults(),
        Storage:    providers,
        Logger:     lgr,
        Translator: translator,
        Adapters:   []adapters.Messenger{consoleAdapter},
    })
    if err != nil {
        log.Fatalf("failed to create module: %v", err)
    }

    // 6. Create a notification definition
    def := &domain.NotificationDefinition{
        Code:         "welcome",
        Channels:     domain.StringList{"email:console"},
        TemplateKeys: domain.StringList{"email:welcome-email"},
    }
    if err := providers.Definitions.Create(ctx, def); err != nil {
        log.Fatalf("failed to create definition: %v", err)
    }

    // 7. Create a template
    _, err = mod.Templates().Create(ctx, templates.TemplateInput{
        Code:    "welcome-email",
        Channel: "email",
        Locale:  "en",
        Subject: "Welcome {{ Name }}!",
        Body:    "Hello {{ Name }}, welcome to our platform!",
        Format:  "text/plain",
        Schema:  domain.TemplateSchema{Required: []string{"Name"}},
    })
    if err != nil {
        log.Fatalf("failed to create template: %v", err)
    }

    // 8. Send a notification
    err = mod.Manager().Send(ctx, notifier.Event{
        DefinitionCode: "welcome",
        Recipients:     []string{"user@example.com"},
        Context: map[string]any{
            "Name": "Alice",
        },
        Locale: "en",
    })
    if err != nil {
        log.Fatalf("failed to send notification: %v", err)
    }

    log.Println("Notification sent successfully!")
}

func newSimpleTranslator() i18n.Translator {
    store := i18n.NewStaticStore(i18n.Translations{
        "en": &i18n.TranslationCatalog{
            Locale:   i18n.Locale{Code: "en"},
            Messages: map[string]i18n.Message{},
        },
    })
    translator, _ := i18n.NewSimpleTranslator(store, i18n.WithTranslatorDefaultLocale("en"))
    return translator
}
```

When you run this, you'll see the notification logged to the console:

```
[INFO] [console][email][plain] subject=Welcome Alice! to=user@example.com body=Hello Alice, welcome to our platform!
```

---

## Sending Your First Notification

Notifications flow through three key components:

### 1. Notification Definition

A definition describes a notification type. It specifies:
- **Code**: Unique identifier (e.g., `welcome`, `password-reset`)
- **Channels**: Delivery channels with adapter mappings (e.g., `email:sendgrid`, `sms:twilio`)
- **TemplateKeys**: Which templates to use per channel

```go
definition := &domain.NotificationDefinition{
    Code:         "order-confirmation",
    Channels:     domain.StringList{"email:sendgrid", "sms:twilio"},
    TemplateKeys: domain.StringList{"email:order-email", "sms:order-sms"},
}
```

### 2. Template

Templates define the content rendered for each channel/locale combination:

```go
template := templates.TemplateInput{
    Code:    "order-email",
    Channel: "email",
    Locale:  "en",
    Subject: "Order #{{ OrderID }} Confirmed",
    Body:    "Thank you {{ Name }}! Your order #{{ OrderID }} has been confirmed.",
    Format:  "text/html",
    Schema:  domain.TemplateSchema{Required: []string{"Name", "OrderID"}},
}
```

### 3. Event

An event triggers notification delivery:

```go
err := manager.Send(ctx, notifier.Event{
    DefinitionCode: "order-confirmation",
    Recipients:     []string{"customer@example.com"},
    Context: map[string]any{
        "Name":    "Bob",
        "OrderID": "ORD-12345",
    },
    Locale: "en",
})
```

Each recipient is fanned out to every configured channel. If you need different destinations per channel (email + SMS), send separate events per channel.

---

## Understanding the Flow

When you call `Manager.Send()`, the following happens:

```
Event Submission
       │
       ▼
┌──────────────────┐
│ Event Persisted  │  Events are stored for auditing
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Definition Load  │  Load channels + template keys
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Preference Check │  Skip if user opted out
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Template Render  │  Render subject/body with context
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Message Created  │  One message per recipient/channel
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Adapter Dispatch │  Route to correct adapter
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Delivery Attempt │  Track success/failure + retries
└──────────────────┘
```

---

## Basic Configuration

The module uses sensible defaults, but you can customize behavior (make sure to import `time` if you use durations):

```go
cfg := config.Config{
    Localization: config.LocalizationConfig{
        DefaultLocale: "en",
    },
    Dispatcher: config.DispatcherConfig{
        Enabled:    true,
        MaxRetries: 3,   // Retry failed deliveries up to 3 times
        MaxWorkers: 4,   // Concurrent delivery workers
    },
    Inbox: config.InboxConfig{
        Enabled: true,   // Enable in-app inbox
    },
    Templates: config.TemplateConfig{
        CacheTTL: time.Minute,  // Cache rendered templates
    },
}

mod, err := notifier.NewModule(notifier.ModuleOptions{
    Config:  cfg,
    // ... other options
})
```

### Configuration Reference

| Section | Field | Default | Description |
|---------|-------|---------|-------------|
| `Localization` | `DefaultLocale` | `"en"` | Fallback locale for templates |
| `Dispatcher` | `Enabled` | `true` | Enable/disable delivery |
| `Dispatcher` | `MaxRetries` | `3` | Max retry attempts on failure |
| `Dispatcher` | `MaxWorkers` | `4` | Concurrent delivery workers |
| `Inbox` | `Enabled` | `true` | Enable in-app inbox |
| `Templates` | `CacheTTL` | `1m` | Template cache duration |
| `Realtime` | `Enabled` | `true` | Enable real-time broadcasts |

---

## Next Steps

Now that you've sent your first notification, explore these guides:

### Essential Guides

1. **[GUIDE_ADAPTERS.md](GUIDE_ADAPTERS.md)** - Configure production adapters (SendGrid, Twilio, etc.)
2. **[GUIDE_TEMPLATES.md](GUIDE_TEMPLATES.md)** - Author templates with localization
3. **[GUIDE_DEFINITIONS.md](GUIDE_DEFINITIONS.md)** - Manage notification types

### Advanced Topics

4. **[GUIDE_PREFERENCES.md](GUIDE_PREFERENCES.md)** - User opt-in/opt-out settings
5. **[GUIDE_INBOX.md](GUIDE_INBOX.md)** - In-app notification center
6. **[GUIDE_EVENTS.md](GUIDE_EVENTS.md)** - Event submission and scheduling
7. **[GUIDE_INTEGRATION.md](GUIDE_INTEGRATION.md)** - Production deployment patterns
8. **[GUIDE_SECRETS.md](GUIDE_SECRETS.md)** - Secure credential management

### Reference Documentation

- **[NTF_TDD.md](NTF_TDD.md)** - Technical design document
- **[NTF_ENTITIES.md](NTF_ENTITIES.md)** - Entity schemas and relationships
- **[NTF_ADAPTERS.md](NTF_ADAPTERS.md)** - Adapter implementation details

---

## Quick Reference

### Module Initialization

```go
mod, err := notifier.NewModule(notifier.ModuleOptions{
    Config:      config.Defaults(),
    Storage:     storage.NewMemoryProviders(), // or storage.NewBunProviders(db)
    Logger:      logger.Default(), // or yourLogger
    Translator:  yourTranslator,
    Adapters:    []adapters.Messenger{adapter1, adapter2},
})
```

### Accessing Services

```go
manager := mod.Manager()         // Send notifications
templates := mod.Templates()     // Manage templates
preferences := mod.Preferences() // User preferences
inbox := mod.Inbox()             // In-app notifications
events := mod.Events()           // Event submission
commands := mod.Commands()       // go-command integration
```

### Sending Notifications

```go
err := mod.Manager().Send(ctx, notifier.Event{
    DefinitionCode: "welcome",
    Recipients:     []string{"user@example.com"},
    Context:        map[string]any{"Name": "Alice"},
    Locale:         "en",
    TenantID:       "tenant-123",     // Optional: multi-tenant
    ActorID:        "actor-456",      // Optional: who triggered
    ScheduledAt:    time.Now().Add(time.Hour), // Optional: delay
})
```

---

## Troubleshooting

### "definition code is required"

Ensure you're passing a valid `DefinitionCode` that matches a persisted definition:

```go
err := manager.Send(ctx, notifier.Event{
    DefinitionCode: "welcome",  // Must exist in definitions repository
    // ...
})
```

### "at least one recipient is required"

The `Recipients` slice must contain at least one entry:

```go
err := manager.Send(ctx, notifier.Event{
    Recipients: []string{"user@example.com"},  // Required
    // ...
})
```

### Template not found

Ensure the template code/channel/locale matches what's stored:

```go
// Definition references template key "email:welcome-email"
TemplateKeys: domain.StringList{"email:welcome-email"}

// Template must match
templates.TemplateInput{
    Code:    "welcome-email",  // Matches "welcome-email"
    Channel: "email",          // Matches "email:"
    Locale:  "en",             // Matches requested locale
}
```

### No adapter found for channel

Ensure an adapter is registered that matches the channel:

```go
// Definition uses "email:sendgrid"
Channels: domain.StringList{"email:sendgrid"}

// SendGrid adapter must be registered with name "sendgrid"
sendgridAdapter := sendgrid.New(apiKey, logger.Default())
// Adapter.Name() returns "sendgrid" and Capabilities().Channels includes "email"
```
