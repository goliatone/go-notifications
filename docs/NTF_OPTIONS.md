# Notification Options & Preference Layering

Phase 4 introduces deterministic, scope-aware configuration derived from `github.com/goliatone/go-options`. The preferences stack mirrors the canonical `system → tenant → user` precedence and powers quiet hours, channel overrides, and subscription filters.

## Scope Layout

| Scope  | Priority | Subject Type | Purpose                                   |
|--------|----------|--------------|-------------------------------------------|
| User   | 500      | `user`       | Per-recipient opt-in/out + quiet hours    |
| Tenant | 200      | `tenant`     | Workspace-wide defaults                   |
| System | 100      | `system`     | Safety net defaults shipped with the app  |

The dispatcher assembles these scopes automatically by combining the event tenant ID plus the recipient ID. Custom transports can append additional scopes (org/team) with their own priorities when building evaluation requests.

## Raw go-options Usage

The `pkg/options` package wraps go-options stacks and provides persistence helpers around the Bun repositories:

```go
import (
    pkgoptions "github.com/goliatone/go-notifications/pkg/options"
    opts "github.com/goliatone/go-options"
)

scopes := []pkgoptions.Snapshot{
    {
        Scope: opts.NewScope("system", opts.ScopePrioritySystem),
        Data: map[string]any{"enabled": true},
    },
    {
        Scope: opts.NewScope("tenant", opts.ScopePriorityTenant),
        Data: map[string]any{
            "enabled": true,
            "quiet_hours": map[string]any{
                "start": "21:00",
                "end":   "06:00",
                "timezone": "America/New_York",
            },
        },
    },
    {
        Scope: opts.NewScope("user", opts.ScopePriorityUser),
        Data: map[string]any{"enabled": false},
    },
}

resolver, _ := pkgoptions.NewResolver(scopes...)
allowed, trace, _ := resolver.ResolveBool("enabled")
schema, _ := resolver.Schema()
```

The schema output is JSON-serialisable and can be sent to go-admin/go-cms to generate forms that match the effective configuration.

## Preferences Service Example

`pkg/preferences.Service` persists per-subject settings and exposes evaluation + trace helpers. The dispatcher uses the same API before fan-out to skip deliveries that violate quiet hours or explicit opt-outs.

```go
import (
    "context"
    prefsvc "github.com/goliatone/go-notifications/pkg/preferences"
    pkgoptions "github.com/goliatone/go-notifications/pkg/options"
    opts "github.com/goliatone/go-options"
)

service, _ := prefsvc.New(prefsvc.Dependencies{
    Repository: storageProviders.Preferences,
    Logger:     lgr,
})

req := prefsvc.EvaluationRequest{
    DefinitionCode: "billing.alert",
    Channel:        "email",
    Scopes: []pkgoptions.PreferenceScopeRef{
        {
            Scope:       opts.NewScope("user", opts.ScopePriorityUser),
            SubjectType: "user",
            SubjectID:   "user-42",
        },
        {
            Scope:       opts.NewScope("tenant", opts.ScopePriorityTenant),
            SubjectType: "tenant",
            SubjectID:   "tenant-1",
        },
    },
}

result, _ := service.Evaluate(context.Background(), req)
if !result.Allowed && result.Reason == prefsvc.ReasonQuietHours {
    // Retry later or route to inbox.
}

value, trace, _ := service.ResolveWithTrace(context.Background(), req, "quiet_hours")
schema, _ := service.Schema(context.Background(), req)
```

The resolver exposes `ResolveWithTrace` so UI clients can highlight the originating scope for each effective field, and `Schema` returns descriptors suitable for rendering editable forms.

## Subscription Filters & Channel Overrides

Additional rules stored under `rules.subscriptions` and `rules.channels` are merged through the same go-options stack:

```json
{
  "rules": {
    "subscriptions": ["ops", "billing"],
    "channels": {
      "sms": { "enabled": false }
    }
  }
}
```

- `rules.subscriptions` enforces membership before dispatch. The dispatcher automatically passes the `subscriptions` slice found in the event context into the evaluation request.
- `rules.channels.<channel>.enabled` allows per-channel overrides so tenants can disable SMS while leaving email untouched.

Quiet hours use the dedicated `quiet_hours` object (`start`, `end`, `timezone`) and the evaluation result surfaces a `QuietHoursActive` flag that integrations can log or display back to the user.
