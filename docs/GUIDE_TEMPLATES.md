# Templates Guide

This guide covers how to create, manage, and render notification templates with localization support in `go-notifications`.

## Table of Contents

1. [Overview](#overview)
2. [Template Structure](#template-structure)
3. [Template Syntax](#template-syntax)
4. [Creating Templates](#creating-templates)
5. [Schema Validation](#schema-validation)
6. [Localization](#localization)
7. [Per-Channel Variants](#per-channel-variants)
8. [Template Caching](#template-caching)
9. [Template Sources](#template-sources)
10. [Common Patterns](#common-patterns)
11. [Troubleshooting](#troubleshooting)

---

## Overview

Templates define the content rendered for each notification. The template system provides:

- **Pongo2/Django-style syntax**: Familiar `{{ variable }}` and `{% tag %}` patterns
- **Localization**: Built-in translation helpers via `go-i18n`
- **Per-channel variants**: Different content for email, SMS, push, etc.
- **Schema validation**: Ensure required data is provided before rendering
- **Caching**: Reduce database lookups for frequently used templates
- **Versioning**: Revision tracking for audit trails

---

## Template Structure

### NotificationTemplate Entity

```go
type NotificationTemplate struct {
    ID          uuid.UUID       // Unique identifier
    Code        string          // Template code (e.g., "welcome-email")
    Channel     string          // Target channel (email, sms, push, etc.)
    Locale      string          // Locale code (en, es, fr, etc.)
    Subject     string          // Subject line template
    Body        string          // Body content template
    Description string          // Human-readable description
    Format      string          // Content type (text/plain, text/html)
    Revision    int             // Version number
    Schema      TemplateSchema  // Required/optional field definitions
    Source      TemplateSource  // External source reference (go-cms)
    Metadata    map[string]any  // Custom metadata
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

### TemplateInput

Use this struct when creating or updating templates:

```go
type TemplateInput struct {
    Code        string               // Required: unique template code
    Channel     string               // Required: target channel
    Locale      string               // Required: locale code
    Subject     string               // Subject template (required if no Source)
    Body        string               // Body template (required if no Source)
    Description string               // Optional description
    Format      string               // Default: "text/plain"
    Schema      domain.TemplateSchema
    Source      domain.TemplateSource
    Metadata    domain.JSONMap
}
```

---

## Template Syntax

Templates use [Pongo2](https://github.com/flosch/pongo2), a Django-style template engine for Go.

### Helper Functions

Built-in helpers include:

- `t(locale, key, args...)` for translations
- `secure_link(data, key)` for resolved links (`action_url` by default)

Example:

```text
{{ t(locale, "welcome.subject", Name) }}
{{ secure_link(action_url, url) }}
{{ secure_link(manifest_url) }}
```

### Variable Interpolation

```django
Hello {{ Name }}!

Your order #{{ OrderID }} has been confirmed.
Total: ${{ Amount }}
```

### Filters

```django
{{ name|title }}                    {# Capitalize: "john" -> "John" #}
{{ email|lower }}                   {# Lowercase #}
{{ description|truncatewords:10 }}  {# Limit to 10 words #}
{{ price|floatformat:2 }}           {# Format number: 99.9 -> 99.90 #}
{{ created_at|date:"Jan 2, 2006" }} {# Format date #}
```

### Conditionals

```django
{% if user.is_premium %}
    Thank you for being a premium member!
{% else %}
    Upgrade to premium for exclusive benefits.
{% endif %}
```

### Loops

```django
Your items:
{% for item in items %}
  - {{ item.name }}: ${{ item.price }}
{% endfor %}
```

### Default Values

```django
Hello {{ name|default:"there" }}!
```

### Nested Data

```django
{{ user.profile.avatar_url }}
{{ order.items.0.name }}
```

---

## Creating Templates

### Basic Template Creation

```go
import (
    "github.com/goliatone/go-notifications/pkg/templates"
    "github.com/goliatone/go-notifications/pkg/domain"
)

// Create a welcome email template
tpl, err := templateService.Create(ctx, templates.TemplateInput{
    Code:        "welcome-email",
    Channel:     "email",
    Locale:      "en",
    Subject:     "Welcome to {{ AppName }}, {{ Name }}!",
    Body:        `
        <h1>Welcome, {{ Name }}!</h1>
        <p>Thanks for joining {{ AppName }}. We're excited to have you.</p>
        <p>Get started by <a href="{{ DashboardURL }}">visiting your dashboard</a>.</p>
    `,
    Format:      "text/html",
    Description: "Welcome email for new users",
    Schema: domain.TemplateSchema{
        Required: []string{"Name", "AppName", "DashboardURL"},
    },
})
```

### Creating Multi-Locale Templates

```go
// English version
templateService.Create(ctx, templates.TemplateInput{
    Code:    "order-confirmation",
    Channel: "email",
    Locale:  "en",
    Subject: "Order #{{ OrderID }} Confirmed",
    Body:    "Thank you for your order, {{ Name }}!",
    Format:  "text/html",
})

// Spanish version
templateService.Create(ctx, templates.TemplateInput{
    Code:    "order-confirmation",
    Channel: "email",
    Locale:  "es",
    Subject: "Pedido #{{ OrderID }} Confirmado",
    Body:    "Gracias por tu pedido, {{ Name }}!",
    Format:  "text/html",
})
```

### Updating Templates

```go
updated, err := templateService.Update(ctx, templates.TemplateInput{
    Code:    "welcome-email",
    Channel: "email",
    Locale:  "en",
    Subject: "Welcome aboard, {{ Name }}!",  // Updated subject
    Body:    "...",
})
// Revision is automatically incremented
```

---

## Schema Validation

Schemas ensure required data is present before rendering, preventing runtime errors.

### Defining Schemas

```go
templates.TemplateInput{
    // ...
    Schema: domain.TemplateSchema{
        Required: []string{"Name", "OrderID", "Items"},
        Optional: []string{"PromoCode", "Notes"},
    },
}
```

### Nested Field Validation

Use dot notation for nested fields:

```go
Schema: domain.TemplateSchema{
    Required: []string{
        "user.name",
        "user.email",
        "order.id",
        "order.items",
    },
}
```

### Validation Behavior

When rendering, if required fields are missing:

```go
result, err := templateService.Render(ctx, templates.RenderRequest{
    Code:    "order-confirmation",
    Channel: "email",
    Locale:  "en",
    Data: map[string]any{
        "Name": "Alice",
        // Missing: OrderID
    },
})
// err is a SchemaError indicating missing "OrderID"
```

---

## Localization

### Translation Helper

Use the `t()` helper to access translations from your `go-i18n` catalog:

```django
{{ t(locale, "welcome.greeting", Name) }}
```

**Arguments**:
1. `locale` - The current locale (auto-injected as `locale` key)
2. Translation key (e.g., `"welcome.greeting"`)
3. Additional arguments for placeholders

### Translation Catalog Example

In your i18n catalog (`en.yaml`):

```yaml
welcome:
  greeting: "Hello, %s!"
  subject: "Welcome to Our Platform"
  body: "Thanks for joining us, %s. We're glad you're here."
```

### Template Using Translations

```django
Subject: {{ t(locale, "welcome.subject") }}
Body: {{ t(locale, "welcome.body", Name) }}
```

### Locale Fallback Chains

Configure fallbacks so `es-MX` can fall back to `es`, then `en`:

```go
import i18n "github.com/goliatone/go-i18n"

resolver := i18n.NewStaticFallbackResolver()
resolver.Set("es-mx", "es", "en")
resolver.Set("pt-br", "pt", "en")

templateService, _ := templates.New(templates.Dependencies{
    Repository: repo,
    Translator: translator,
    Fallbacks:  resolver,
    DefaultLocale: "en",
})
```

**Resolution order for `es-MX`**:
1. Look for `es-mx` template
2. Fall back to `es` template
3. Fall back to `en` template

### Render Result Indicates Fallback

```go
result, _ := templateService.Render(ctx, templates.RenderRequest{
    Code:    "welcome",
    Channel: "email",
    Locale:  "es-mx",  // Requested locale
    Data:    data,
})

fmt.Println(result.Locale)       // "es" (actual locale used)
fmt.Println(result.UsedFallback) // true
```

---

## Per-Channel Variants

Create different templates for each channel:

### Email Template

```go
templateService.Create(ctx, templates.TemplateInput{
    Code:    "order-shipped",
    Channel: "email",
    Locale:  "en",
    Subject: "Your Order #{{ OrderID }} Has Shipped!",
    Body: `
        <h1>Great news, {{ Name }}!</h1>
        <p>Your order has shipped and is on its way.</p>
        <p>Tracking: <a href="{{ TrackingURL }}">{{ TrackingNumber }}</a></p>
    `,
    Format: "text/html",
})
```

### SMS Template

```go
templateService.Create(ctx, templates.TemplateInput{
    Code:    "order-shipped",
    Channel: "sms",
    Locale:  "en",
    Subject: "Order Shipped",
    Body:    "{{ Name }}, your order #{{ OrderID }} shipped! Track: {{ TrackingURL }}",
    Format:  "text/plain",
})
```

### Push Template

```go
templateService.Create(ctx, templates.TemplateInput{
    Code:    "order-shipped",
    Channel: "push",
    Locale:  "en",
    Subject: "Order Shipped",
    Body:    "Your order is on the way!",
    Format:  "text/plain",
    Metadata: domain.JSONMap{
        "click_action": "/orders/{{ OrderID }}",
        "icon":         "shipping_icon",
    },
})
```

### Linking Templates to Definitions

```go
definition := &domain.NotificationDefinition{
    Code:     "order-shipped",
    Channels: domain.StringList{"email:sendgrid", "sms:twilio", "push:firebase"},
    TemplateKeys: domain.StringList{
        "email:order-shipped",  // channel:template-code
        "sms:order-shipped",
        "push:order-shipped",
    },
}
```

---

## Template Caching

Templates are cached to reduce database lookups.

### Cache Configuration

```go
templateService, _ := templates.New(templates.Dependencies{
    Repository: repo,
    Cache:      redisCache,  // Implement cache.Cache interface
    CacheTTL:   5 * time.Minute,
    // ...
})
```

### Cache Key Format

```
templates:<code>:<channel>:<locale>
```

Example: `templates:welcome-email:email:en`

### Cache Invalidation

Templates are automatically cached on:
- `Create()` - New template is cached
- `Update()` - Updated template replaces cache entry
- `Get()` / `Render()` - Template is cached after database fetch

For manual invalidation, implement cache clearing in your `cache.Cache` implementation.

---

## Template Sources

Templates can reference external content sources like `go-cms`.

### TemplateSource Structure

```go
type TemplateSource struct {
    Type      string  // Source type (e.g., "gocms-block")
    Reference string  // External reference ID
    Payload   JSONMap // Source-specific data
}
```

### go-cms Integration

```go
templateService.Create(ctx, templates.TemplateInput{
    Code:    "newsletter",
    Channel: "email",
    Locale:  "en",
    Source: domain.TemplateSource{
        Type:      "gocms-block",
        Reference: "newsletter-template-v2",
        Payload: domain.JSONMap{
            "subject": "Weekly Newsletter - {{ Title }}",
            "blocks": []any{
                map[string]any{
                    "body": `<div>{{ t(locale, "newsletter.intro", Name) }}</div>`,
                },
            },
        },
    },
    Format: "text/html",
    Schema: domain.TemplateSchema{Required: []string{"Name", "Title"}},
})
```

When `Source` is set, the template engine extracts `subject` and `body` from the payload rather than using the direct `Subject`/`Body` fields.

---

## Common Patterns

### Welcome Email

```go
templates.TemplateInput{
    Code:    "welcome-email",
    Channel: "email",
    Locale:  "en",
    Subject: "Welcome to {{ AppName }}!",
    Body: `
<!DOCTYPE html>
<html>
<head><title>Welcome</title></head>
<body>
    <h1>Welcome, {{ Name }}!</h1>
    <p>Thanks for signing up for {{ AppName }}.</p>

    <h2>Getting Started</h2>
    <ul>
        <li><a href="{{ ProfileURL }}">Complete your profile</a></li>
        <li><a href="{{ DocsURL }}">Read our docs</a></li>
    </ul>

    <p>If you have questions, reply to this email.</p>
</body>
</html>
    `,
    Format: "text/html",
    Schema: domain.TemplateSchema{
        Required: []string{"Name", "AppName", "ProfileURL", "DocsURL"},
    },
}
```

### Password Reset

```go
templates.TemplateInput{
    Code:    "password-reset",
    Channel: "email",
    Locale:  "en",
    Subject: "Reset Your Password",
    Body: `
Hi {{ name }},

We received a request to reset your password.

Click the link below to set a new password{% if remaining_minutes %} (expires at {{ expires_at }}, about {{ remaining_minutes }} minutes from now){% else %} (expires at {{ expires_at }}){% endif %}:
{{ action_url }}

If you didn't request this, you can ignore this email.

- The {{ app_name }} Team
    `,
    Format: "text/plain",
    Schema: domain.TemplateSchema{
        Required: []string{"name", "action_url", "expires_at", "app_name"},
        Optional: []string{"remaining_minutes"},
    },
}
```

### Invite

```go
templates.TemplateInput{
    Code:    "invite",
    Channel: "email",
    Locale:  "en",
    Subject: "You're invited",
    Body: `
Hi {{ name }},

You've been invited to join {{ app_name }}.

Accept your invite here{% if remaining_minutes %} (expires at {{ expires_at }}, about {{ remaining_minutes }} minutes from now){% else %} (expires at {{ expires_at }}){% endif %}:
{{ action_url }}

- The {{ app_name }} Team
    `,
    Format: "text/plain",
    Schema: domain.TemplateSchema{
        Required: []string{"name", "app_name", "action_url", "expires_at"},
        Optional: []string{"remaining_minutes"},
    },
}
```

### Account Lockout

```go
templates.TemplateInput{
    Code:    "account-lockout",
    Channel: "email",
    Locale:  "en",
    Subject: "Account Locked",
    Body: `
Hi {{ name }},

Your account was locked due to {{ reason }} until {{ lockout_until }}.

Unlock your account here:
{{ unlock_url }}

- The {{ app_name }} Team
    `,
    Format: "text/plain",
    Schema: domain.TemplateSchema{
        Required: []string{"name", "reason", "lockout_until", "unlock_url", "app_name"},
    },
}
```

### Email Verification

```go
templates.TemplateInput{
    Code:    "email-verification",
    Channel: "email",
    Locale:  "en",
    Subject: "Verify Your Email",
    Body: `
Hi {{ name }},

Please verify your email address:
{{ verify_url }}

This link expires at {{ expires_at }}.
Resend allowed: {{ resend_allowed }}

- The {{ app_name }} Team
    `,
    Format: "text/plain",
    Schema: domain.TemplateSchema{
        Required: []string{"name", "verify_url", "expires_at", "resend_allowed", "app_name"},
    },
}
```

### Order Confirmation with Items

```go
templates.TemplateInput{
    Code:    "order-confirmation",
    Channel: "email",
    Locale:  "en",
    Subject: "Order #{{ Order.ID }} Confirmed",
    Body: `
<h1>Thank you for your order, {{ Customer.Name }}!</h1>

<h2>Order Details</h2>
<table>
    <tr><th>Item</th><th>Qty</th><th>Price</th></tr>
    {% for item in Order.Items %}
    <tr>
        <td>{{ item.Name }}</td>
        <td>{{ item.Quantity }}</td>
        <td>${{ item.Price|floatformat:2 }}</td>
    </tr>
    {% endfor %}
</table>

<p><strong>Total:</strong> ${{ Order.Total|floatformat:2 }}</p>

{% if Order.PromoCode %}
<p>Promo code applied: {{ Order.PromoCode }}</p>
{% endif %}
    `,
    Format: "text/html",
    Schema: domain.TemplateSchema{
        Required: []string{"Customer.Name", "Order.ID", "Order.Items", "Order.Total"},
        Optional: []string{"Order.PromoCode"},
    },
}
```

### SMS Alert

```go
templates.TemplateInput{
    Code:    "security-alert-sms",
    Channel: "sms",
    Locale:  "en",
    Subject: "Security Alert",
    Body:    "{{ AppName }} Security: New login from {{ Location }}. If not you, secure your account: {{ SecurityURL }}",
    Format:  "text/plain",
    Schema: domain.TemplateSchema{
        Required: []string{"AppName", "Location", "SecurityURL"},
    },
}
```

---

## Troubleshooting

### "templates: code is required"

The `Code` field must be non-empty:

```go
templates.TemplateInput{
    Code: "my-template",  // Required
    // ...
}
```

### "templates: subject is required when source is empty"

Either provide `Subject`/`Body` or a `Source`:

```go
// Option 1: Direct content
templates.TemplateInput{
    Subject: "Hello",
    Body:    "World",
}

// Option 2: External source
templates.TemplateInput{
    Source: domain.TemplateSource{
        Type: "gocms-block",
        Payload: domain.JSONMap{
            "subject": "...",
            "blocks": []any{...},
        },
    },
}
```

### Schema Validation Errors

When required fields are missing:

```go
result, err := svc.Render(ctx, templates.RenderRequest{
    Data: map[string]any{}, // Missing required fields
})
// err: SchemaError{Missing: []string{"Name", "OrderID"}}
```

**Fix**: Ensure all required fields are in `Data`:

```go
result, err := svc.Render(ctx, templates.RenderRequest{
    Data: map[string]any{
        "Name":    "Alice",
        "OrderID": "12345",
    },
})
```

### Template Not Found

Ensure the template exists for the requested code/channel/locale:

```go
// This will fail if no "welcome" template exists for "email" channel
result, err := svc.Render(ctx, templates.RenderRequest{
    Code:    "welcome",
    Channel: "email",
    Locale:  "en",
})
```

Check the fallback chain - if `en` is missing, ensure there's a default locale template.

### Locale Fallback Not Working

Configure the fallback resolver:

```go
resolver := i18n.NewStaticFallbackResolver()
resolver.Set("es-mx", "es", "en")

svc, _ := templates.New(templates.Dependencies{
    Fallbacks: resolver,  // Required for fallback chains
    // ...
})
```

---

## Quick Reference

### Rendering Templates

```go
result, err := templateService.Render(ctx, templates.RenderRequest{
    Code:    "welcome-email",
    Channel: "email",
    Locale:  "en",
    Data: map[string]any{
        "Name":    "Alice",
        "AppName": "MyApp",
    },
})

// result.Subject - Rendered subject
// result.Body    - Rendered body
// result.Locale  - Actual locale used
// result.UsedFallback - true if fallback was used
```

### Registering Custom Helpers

```go
templateService.RegisterHelpers(map[string]any{
    "formatCurrency": func(amount float64, currency string) string {
        return fmt.Sprintf("%s%.2f", currency, amount)
    },
})
```

Use in templates:

```django
Total: {{ formatCurrency(order.total, "$") }}
```

### Template Service Dependencies

```go
svc, err := templates.New(templates.Dependencies{
    Repository:    repo,           // Required
    Translator:    translator,     // Required
    Cache:         cache,          // Optional (defaults to Nop)
    Logger:        logger,         // Optional
    Fallbacks:     fallbackResolver,
    DefaultLocale: "en",
    CacheTTL:      time.Minute,
})
```
