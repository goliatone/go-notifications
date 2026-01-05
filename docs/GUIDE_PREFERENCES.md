# Preferences Guide

This guide covers how to manage user notification preferences, including opt-in/opt-out settings, quiet hours, channel overrides, and subscription filtering in `go-notifications`.

## Table of Contents

1. [Overview](#overview)
2. [Preference Model](#preference-model)
3. [Preference Scopes](#preference-scopes)
4. [Creating and Managing Preferences](#creating-and-managing-preferences)
5. [Preference Evaluation](#preference-evaluation)
6. [Quiet Hours](#quiet-hours)
7. [Channel Overrides](#channel-overrides)
8. [Subscription Groups](#subscription-groups)
9. [Building Preference UIs](#building-preference-uis)
10. [Inheritance and Override Patterns](#inheritance-and-override-patterns)
11. [Troubleshooting](#troubleshooting)

---

## Overview

The preferences system allows users and tenants to control which notifications they receive. Key features:

- **Scoped preferences**: User, tenant, group, or system-level settings
- **Opt-in/opt-out**: Enable or disable notifications per definition/channel
- **Quiet hours**: Suppress notifications during specified time windows
- **Channel overrides**: Per-channel enable/disable and provider selection
- **Subscription filtering**: Only deliver to users in specific subscription groups
- **Layered resolution**: Higher-priority scopes override lower-priority defaults

---

## Preference Model

### NotificationPreference Entity

```go
type NotificationPreference struct {
    ID              uuid.UUID  // Unique identifier
    SubjectID       string     // Who this preference applies to (user ID, tenant ID, etc.)
    SubjectType     string     // Subject type: "user", "tenant", "group", etc.
    DefinitionCode  string     // Notification type (e.g., "order-shipped")
    Channel         string     // Delivery channel (e.g., "email", "sms")
    Locale          string     // Preferred locale for this notification
    Enabled         bool       // Master on/off switch
    QuietHours      JSONMap    // Time-based suppression rules
    AdditionalRules JSONMap    // Channel overrides, provider, subscriptions
    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

### PreferenceInput

Use this struct for CRUD operations:

```go
type PreferenceInput struct {
    SubjectType    string             // Required: "user", "tenant", "group"
    SubjectID      string             // Required: subject identifier
    DefinitionCode string             // Required: notification type code
    Channel        string             // Required: delivery channel
    Enabled        *bool              // Optional: enable/disable
    Locale         *string            // Optional: preferred locale
    QuietHours     *QuietHoursWindow  // Optional: quiet hours config
    Provider       *string            // Optional: preferred provider
    Rules          domain.JSONMap     // Optional: additional rules
}
```

---

## Preference Scopes

Preferences are evaluated in priority order. Higher-priority scopes override lower ones.

### Scope Priorities

| Scope | Priority | Description |
|-------|----------|-------------|
| `user` | Highest | Individual user preferences |
| `group` | High | Group/team preferences |
| `tenant` | Medium | Organization-wide preferences |
| `system` | Low | System defaults |
| `defaults` | Lowest | Built-in fallback defaults |

### Creating Scope References

```go
import (
    pkgoptions "github.com/goliatone/go-notifications/pkg/options"
    opts "github.com/goliatone/go-options"
)

// User scope (highest priority)
userScope := pkgoptions.PreferenceScopeRef{
    Scope:          opts.NewScope("user", opts.ScopePriorityUser),
    SubjectType:    "user",
    SubjectID:      "user-123",
    DefinitionCode: "order-shipped",
    Channel:        "email",
}

// Tenant scope
tenantScope := pkgoptions.PreferenceScopeRef{
    Scope:          opts.NewScope("tenant", opts.ScopePriorityTenant),
    SubjectType:    "tenant",
    SubjectID:      "tenant-456",
    DefinitionCode: "order-shipped",
    Channel:        "email",
}
```

---

## Creating and Managing Preferences

### Initialize the Service

```go
import (
    "github.com/goliatone/go-notifications/pkg/preferences"
    "github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

prefService, err := preferences.New(preferences.Dependencies{
    Repository: preferenceRepo,
    Logger:     logger,
})
```

### Create a Preference

```go
enabled := true
pref, err := prefService.Create(ctx, preferences.PreferenceInput{
    SubjectType:    "user",
    SubjectID:      "user-123",
    DefinitionCode: "marketing-emails",
    Channel:        "email",
    Enabled:        &enabled,
})
```

### Update a Preference

```go
enabled := false
pref, err := prefService.Update(ctx, preferences.PreferenceInput{
    SubjectType:    "user",
    SubjectID:      "user-123",
    DefinitionCode: "marketing-emails",
    Channel:        "email",
    Enabled:        &enabled,  // User opts out
})
```

### Upsert (Create or Update)

```go
// Creates if doesn't exist, updates if it does
pref, err := prefService.Upsert(ctx, preferences.PreferenceInput{
    SubjectType:    "user",
    SubjectID:      "user-123",
    DefinitionCode: "order-updates",
    Channel:        "sms",
    Enabled:        boolPtr(true),
})

func boolPtr(v bool) *bool { return &v }
```

### Get a Preference

```go
pref, err := prefService.Get(ctx, "user", "user-123", "order-shipped", "email")
if errors.Is(err, store.ErrNotFound) {
    // No preference set, use defaults
}
```

### Delete a Preference

```go
err := prefService.Delete(ctx, "user", "user-123", "marketing-emails", "email")
```

### List Preferences

```go
result, err := prefService.List(ctx, store.ListOptions{
    Limit:  20,
    Offset: 0,
    Filters: map[string]any{
        "subject_type": "user",
        "subject_id":   "user-123",
    },
})

for _, pref := range result.Items {
    fmt.Printf("%s/%s: enabled=%v\n", pref.DefinitionCode, pref.Channel, pref.Enabled)
}
```

---

## Preference Evaluation

Before sending a notification, evaluate preferences to check if delivery is allowed.

### Basic Evaluation

```go
result, err := prefService.Evaluate(ctx, preferences.EvaluationRequest{
    DefinitionCode: "order-shipped",
    Channel:        "email",
    Scopes: []pkgoptions.PreferenceScopeRef{
        {
            Scope:       opts.NewScope("user", opts.ScopePriorityUser),
            SubjectType: "user",
            SubjectID:   "user-123",
        },
        {
            Scope:       opts.NewScope("tenant", opts.ScopePriorityTenant),
            SubjectType: "tenant",
            SubjectID:   "tenant-456",
        },
    },
})

if !result.Allowed {
    fmt.Printf("Notification blocked: %s\n", result.Reason)
    return
}

// Proceed with delivery
```

### EvaluationResult

```go
type EvaluationResult struct {
    Allowed           bool       // Can we send the notification?
    Reason            string     // Why blocked (if not allowed)
    QuietHoursActive  bool       // Is quiet hours window active?
    ChannelOverride   bool       // Was a channel-specific rule applied?
    Provider          string     // Preferred provider (if set)
    Trace             opts.Trace // Resolution trace for debugging
    RequiredSubs      []string   // Required subscription groups
    Resolver          *Resolver  // Access to resolved values
}
```

### Evaluation Reasons

| Reason | Description |
|--------|-------------|
| `default` | No preference set, using defaults (allowed) |
| `opt-out` | User explicitly opted out |
| `quiet-hours` | Blocked by quiet hours window |
| `channel-override` | Channel-specific rule blocked delivery |
| `subscription-filter` | User not in required subscription group |

### Evaluation with Timestamp

For quiet hours evaluation at a specific time:

```go
result, err := prefService.Evaluate(ctx, preferences.EvaluationRequest{
    DefinitionCode: "daily-digest",
    Channel:        "email",
    Scopes:         scopes,
    Timestamp:      time.Date(2024, 1, 15, 23, 30, 0, 0, time.UTC), // 11:30 PM
})
```

---

## Quiet Hours

Suppress notifications during specific time windows.

### Setting Quiet Hours

```go
pref, err := prefService.Upsert(ctx, preferences.PreferenceInput{
    SubjectType:    "user",
    SubjectID:      "user-123",
    DefinitionCode: "all",  // Apply to all notifications
    Channel:        "all",
    Enabled:        boolPtr(true),
    QuietHours: &preferences.QuietHoursWindow{
        Start:    "22:00",  // 10 PM
        End:      "08:00",  // 8 AM (next day)
        Timezone: "America/New_York",
    },
})
```

### Quiet Hours Format

```go
type QuietHoursWindow struct {
    Start    string  // "HH:MM" format (24-hour)
    End      string  // "HH:MM" format (24-hour)
    Timezone string  // IANA timezone (e.g., "America/New_York")
}
```

**Midnight wraparound**: If `End` is before `Start`, the window wraps around midnight.

Example: `Start: "22:00"`, `End: "08:00"` = 10 PM to 8 AM next day.

### Checking Quiet Hours in Evaluation

```go
result, _ := prefService.Evaluate(ctx, preferences.EvaluationRequest{
    DefinitionCode: "order-shipped",
    Channel:        "push",
    Scopes:         scopes,
    Timestamp:      time.Now(),  // Current time for evaluation
})

if result.QuietHoursActive {
    // Schedule for later or skip
    fmt.Println("User is in quiet hours, will retry later")
}
```

---

## Channel Overrides

Override settings for specific channels within a notification type.

### Setting Channel-Specific Rules

```go
pref, err := prefService.Upsert(ctx, preferences.PreferenceInput{
    SubjectType:    "user",
    SubjectID:      "user-123",
    DefinitionCode: "order-updates",
    Channel:        "email",  // Base channel
    Enabled:        boolPtr(true),
    Rules: domain.JSONMap{
        "channels": map[string]any{
            "sms": map[string]any{
                "enabled":  false,     // Disable SMS for this notification
                "provider": "twilio",  // Preferred SMS provider
            },
            "push": map[string]any{
                "enabled": true,
            },
        },
    },
})
```

### Evaluation with Channel Override

```go
result, _ := prefService.Evaluate(ctx, preferences.EvaluationRequest{
    DefinitionCode: "order-updates",
    Channel:        "sms",  // Check SMS channel
    Scopes:         scopes,
})

if result.ChannelOverride {
    fmt.Printf("Channel override applied, allowed: %v\n", result.Allowed)
}

if result.Provider != "" {
    fmt.Printf("Using preferred provider: %s\n", result.Provider)
}
```

### Provider Override

Set a preferred adapter provider:

```go
pref, err := prefService.Upsert(ctx, preferences.PreferenceInput{
    SubjectType:    "tenant",
    SubjectID:      "tenant-456",
    DefinitionCode: "all",
    Channel:        "email",
    Provider:       stringPtr("sendgrid"),  // All emails via SendGrid
})

func stringPtr(v string) *string { return &v }
```

---

## Subscription Groups

Filter notifications to users in specific groups.

### SubscriptionGroup Entity

```go
type SubscriptionGroup struct {
    ID          uuid.UUID
    Code        string  // "beta-users", "premium", "newsletter"
    Name        string  // Human-readable name
    Description string
    Metadata    JSONMap
}
```

### Setting Required Subscriptions

```go
pref, err := prefService.Upsert(ctx, preferences.PreferenceInput{
    SubjectType:    "tenant",
    SubjectID:      "tenant-456",
    DefinitionCode: "beta-features",
    Channel:        "email",
    Rules: domain.JSONMap{
        "subscriptions": []string{"beta-users", "early-adopters"},
    },
})
```

### Evaluation with Subscriptions

```go
result, _ := prefService.Evaluate(ctx, preferences.EvaluationRequest{
    DefinitionCode: "beta-features",
    Channel:        "email",
    Scopes:         scopes,
    Subscriptions:  []string{"newsletter"},  // User's subscriptions
})

if !result.Allowed && result.Reason == preferences.ReasonSubscriptionFilter {
    fmt.Printf("User not in required groups: %v\n", result.RequiredSubs)
}
```

---

## Building Preference UIs

### Get Schema for UI

```go
// Get preference schema for building dynamic UIs
schema, err := prefService.Schema(ctx, preferences.EvaluationRequest{
    DefinitionCode: "order-updates",
    Channel:        "email",
    Scopes:         scopes,
})

// schema.Format - Schema format version
// schema.Properties - Available preference fields
```

### Resolve with Trace

Debug which scope provided a value:

```go
value, trace, err := prefService.ResolveWithTrace(ctx,
    preferences.EvaluationRequest{
        DefinitionCode: "marketing",
        Channel:        "email",
        Scopes:         scopes,
    },
    "enabled",  // Path to resolve
)

fmt.Printf("enabled = %v\n", value)
fmt.Printf("Source scope: %s\n", trace.Layers[0].Scope.Name)
```

### Example: Preference Settings Page

```go
// GET /api/users/:id/preferences
func getPreferences(userID string) ([]PreferenceView, error) {
    result, _ := prefService.List(ctx, store.ListOptions{
        Filters: map[string]any{
            "subject_type": "user",
            "subject_id":   userID,
        },
    })

    views := make([]PreferenceView, len(result.Items))
    for i, pref := range result.Items {
        views[i] = PreferenceView{
            DefinitionCode: pref.DefinitionCode,
            Channel:        pref.Channel,
            Enabled:        pref.Enabled,
            QuietHours:     pref.QuietHours,
        }
    }
    return views, nil
}

// PUT /api/users/:id/preferences/:definition/:channel
func updatePreference(userID, definition, channel string, enabled bool) error {
    _, err := prefService.Upsert(ctx, preferences.PreferenceInput{
        SubjectType:    "user",
        SubjectID:      userID,
        DefinitionCode: definition,
        Channel:        channel,
        Enabled:        &enabled,
    })
    return err
}
```

---

## Inheritance and Override Patterns

### Pattern 1: Tenant Defaults with User Overrides

```go
// Tenant sets default: all marketing emails disabled
prefService.Upsert(ctx, preferences.PreferenceInput{
    SubjectType:    "tenant",
    SubjectID:      "tenant-456",
    DefinitionCode: "marketing",
    Channel:        "email",
    Enabled:        boolPtr(false),
})

// User opts in despite tenant default
prefService.Upsert(ctx, preferences.PreferenceInput{
    SubjectType:    "user",
    SubjectID:      "user-123",
    DefinitionCode: "marketing",
    Channel:        "email",
    Enabled:        boolPtr(true),  // User override wins
})

// Evaluation
result, _ := prefService.Evaluate(ctx, preferences.EvaluationRequest{
    DefinitionCode: "marketing",
    Channel:        "email",
    Scopes: []pkgoptions.PreferenceScopeRef{
        {Scope: opts.NewScope("user", opts.ScopePriorityUser), SubjectType: "user", SubjectID: "user-123"},
        {Scope: opts.NewScope("tenant", opts.ScopePriorityTenant), SubjectType: "tenant", SubjectID: "tenant-456"},
    },
})
// result.Allowed = true (user scope wins)
```

### Pattern 2: Global Quiet Hours

```go
// System-wide quiet hours for all notifications
prefService.Upsert(ctx, preferences.PreferenceInput{
    SubjectType:    "system",
    SubjectID:      "global",
    DefinitionCode: "*",  // All definitions
    Channel:        "*",  // All channels
    QuietHours: &preferences.QuietHoursWindow{
        Start:    "00:00",
        End:      "06:00",
        Timezone: "UTC",
    },
})
```

### Pattern 3: Channel-Specific Provider

```go
// Tenant prefers Twilio for SMS
prefService.Upsert(ctx, preferences.PreferenceInput{
    SubjectType:    "tenant",
    SubjectID:      "tenant-456",
    DefinitionCode: "*",
    Channel:        "sms",
    Rules: domain.JSONMap{
        "provider": "twilio",
    },
})

// Evaluation returns provider preference
result, _ := prefService.Evaluate(ctx, preferences.EvaluationRequest{
    DefinitionCode: "order-shipped",
    Channel:        "sms",
    Scopes:         tenantScopes,
})
fmt.Println(result.Provider)  // "twilio"
```

---

## Troubleshooting

### "preferences: subject type is required"

Both `SubjectType` and `SubjectID` are required:

```go
preferences.PreferenceInput{
    SubjectType: "user",      // Required
    SubjectID:   "user-123",  // Required
    // ...
}
```

### "preferences: at least one scope is required"

Evaluation requires at least one scope reference:

```go
prefService.Evaluate(ctx, preferences.EvaluationRequest{
    Scopes: []pkgoptions.PreferenceScopeRef{
        {
            Scope:       opts.NewScope("user", opts.ScopePriorityUser),
            SubjectType: "user",
            SubjectID:   "user-123",
        },
    },
    // ...
})
```

### Unexpected Evaluation Results

Use tracing to debug:

```go
result, _ := prefService.Evaluate(ctx, request)

fmt.Printf("Allowed: %v\n", result.Allowed)
fmt.Printf("Reason: %s\n", result.Reason)
fmt.Printf("Trace layers:\n")
for _, layer := range result.Trace.Layers {
    fmt.Printf("  - %s: %v\n", layer.Scope.Name, layer.Value)
}
```

### Quiet Hours Not Working

1. Check timezone is valid IANA timezone
2. Ensure time format is "HH:MM" (24-hour)
3. Pass timestamp to evaluation request

```go
result, _ := prefService.Evaluate(ctx, preferences.EvaluationRequest{
    // ...
    Timestamp: time.Now(),  // Required for quiet hours check
})
```

---

## Quick Reference

### Common Operations

```go
// Create preference
prefService.Create(ctx, input)

// Update preference
prefService.Update(ctx, input)

// Create or update
prefService.Upsert(ctx, input)

// Get preference
prefService.Get(ctx, subjectType, subjectID, definitionCode, channel)

// Delete preference
prefService.Delete(ctx, subjectType, subjectID, definitionCode, channel)

// List preferences
prefService.List(ctx, store.ListOptions{})

// Evaluate before sending
prefService.Evaluate(ctx, evaluationRequest)
```

### Evaluation Reasons

```go
const (
    ReasonDefault            = "default"             // No rule matched
    ReasonOptOut             = "opt-out"             // User opted out
    ReasonQuietHours         = "quiet-hours"         // In quiet hours window
    ReasonChannelOverride    = "channel-override"    // Channel-specific block
    ReasonSubscriptionFilter = "subscription-filter" // Not in required group
)
```
