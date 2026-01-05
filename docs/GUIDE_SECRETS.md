# Secrets Guide

This guide covers securely managing API keys, tokens, and credentials for notification adapters in `go-notifications`.

---

## Overview

The secrets system provides:

- **Scoped credentials** - System, tenant, and user-level secrets
- **Multiple providers** - Static, encrypted store, and memory backends
- **Resolution priority** - Fallback chains from user → tenant → system
- **Caching** - Reduce external secret manager calls
- **Log masking** - Prevent accidental credential exposure

Adapters like SendGrid, Twilio, and SMTP require API keys or credentials. The secrets system manages these securely without hardcoding them in your application.

---

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      Resolver                           │
│  (batches resolution, supports priority fallbacks)      │
├─────────────────────────────────────────────────────────┤
│                    CachingResolver                      │
│           (optional TTL-based caching)                  │
├─────────────────────────────────────────────────────────┤
│                       Registry                          │
│       (routes to scope-specific providers)              │
├───────────────┬───────────────┬─────────────────────────┤
│    System     │    Tenant     │         User            │
│   Provider    │   Provider    │       Provider          │
└───────────────┴───────────────┴─────────────────────────┘
```

### Key Concepts

| Concept | Description |
|---------|-------------|
| **Reference** | Identifies a specific secret by scope, subject, channel, provider, and key |
| **Provider** | Backend that stores/retrieves secrets (static, encrypted, memory) |
| **Resolver** | Batches reference lookups and returns results |
| **Scope** | Ownership level: `system`, `tenant`, or `user` |

---

## Secret Reference

A `Reference` uniquely identifies a secret:

```go
import "github.com/goliatone/go-notifications/pkg/secrets"

ref := secrets.Reference{
    Scope:     secrets.ScopeSystem,  // system, tenant, or user
    SubjectID: "default",             // who owns this secret
    Channel:   "email",               // delivery channel
    Provider:  "sendgrid",            // adapter name
    Key:       "default",             // secret identifier
    Version:   "",                    // optional: specific version
}
```

### Reference Fields

| Field | Required | Description |
|-------|----------|-------------|
| `Scope` | Yes | `ScopeSystem`, `ScopeTenant`, or `ScopeUser` |
| `SubjectID` | Yes | Owner identifier (e.g., user ID, tenant ID, "default") |
| `Channel` | Yes | Delivery channel type (email, sms, push, etc.) |
| `Provider` | Yes | Adapter name (sendgrid, twilio, smtp, etc.) |
| `Key` | Yes | Secret identifier (often "default" or "api_key") |
| `Version` | No | Specific version; omit for latest |

---

## Scopes

### System Scope

Global secrets shared across all tenants/users:

```go
ref := secrets.Reference{
    Scope:     secrets.ScopeSystem,
    SubjectID: "default",
    Channel:   "email",
    Provider:  "sendgrid",
    Key:       "default",
}
```

Use for: Default API keys, fallback credentials.

### Tenant Scope

Per-tenant secrets for multi-tenant applications:

```go
ref := secrets.Reference{
    Scope:     secrets.ScopeTenant,
    SubjectID: "tenant-abc",
    Channel:   "email",
    Provider:  "sendgrid",
    Key:       "default",
}
```

Use for: Customer-specific API keys, white-label configurations.

### User Scope

Per-user secrets for personalized integrations:

```go
ref := secrets.Reference{
    Scope:     secrets.ScopeUser,
    SubjectID: "user-123",
    Channel:   "email",
    Provider:  "gmail",
    Key:       "oauth_token",
}
```

Use for: OAuth tokens, personal SMTP credentials.

---

## Built-in Providers

### Static Provider

In-memory secrets for testing and development:

```go
import "github.com/goliatone/go-notifications/pkg/secrets"

// Seed with initial secrets
provider := secrets.NewStaticProvider(map[secrets.Reference]secrets.SecretValue{
    {
        Scope:     secrets.ScopeSystem,
        SubjectID: "default",
        Channel:   "email",
        Provider:  "sendgrid",
        Key:       "default",
    }: {
        Data: []byte("SG.your-api-key-here"),
    },
})

// Store a new secret
version, err := provider.Put(secrets.Reference{
    Scope:     secrets.ScopeSystem,
    SubjectID: "default",
    Channel:   "sms",
    Provider:  "twilio",
    Key:       "default",
}, []byte(`{"account_sid":"AC...","auth_token":"..."}`))

// Retrieve a secret
value, err := provider.Get(secrets.Reference{
    Scope:     secrets.ScopeSystem,
    SubjectID: "default",
    Channel:   "email",
    Provider:  "sendgrid",
    Key:       "default",
})
fmt.Println(string(value.Data)) // SG.your-api-key-here
```

### Memory Store

Simple in-memory store implementing the `Store` interface:

```go
import "github.com/goliatone/go-notifications/pkg/secrets"

store := secrets.NewMemoryStore()

// Put a record
store.Put(ctx, iface.Record{
    Scope:     "system",
    SubjectID: "default",
    Channel:   "email",
    Provider:  "sendgrid",
    Key:       "default",
    Version:   "v1",
    Cipher:    encryptedData,
    Nonce:     nonce,
})

// Get latest version
record, err := store.GetLatest(ctx, "system", "default", "email", "sendgrid", "default")
```

### Encrypted Store Provider

Production-ready provider with ChaCha20-Poly1305 encryption:

```go
import (
    "github.com/goliatone/go-notifications/pkg/secrets"
    iface "github.com/goliatone/go-notifications/pkg/interfaces/secrets"
)

// 32-byte encryption key (use a proper key management system)
key := []byte("your-32-byte-encryption-key!!") // Must be exactly 32 bytes

// Create store (implement Store interface for your database)
store := NewDatabaseStore(db) // or secrets.NewMemoryStore() for testing

// Create encrypted provider
provider, err := secrets.NewEncryptedStoreProvider(store, key)
if err != nil {
    log.Fatal(err)
}

// Store encrypted secret
version, err := provider.Put(secrets.Reference{
    Scope:     secrets.ScopeSystem,
    SubjectID: "default",
    Channel:   "email",
    Provider:  "sendgrid",
    Key:       "default",
}, []byte("SG.your-api-key"))

// Retrieve and decrypt
value, err := provider.Get(secrets.Reference{
    Scope:     secrets.ScopeSystem,
    SubjectID: "default",
    Channel:   "email",
    Provider:  "sendgrid",
    Key:       "default",
})
```

### NopProvider

No-op provider for optional secret support:

```go
import "github.com/goliatone/go-notifications/pkg/secrets"

provider := secrets.NopProvider{}

// Always returns ErrNotFound
value, err := provider.Get(ref)
// err == secrets.ErrNotFound
```

---

## Registering Providers

Use a `Registry` to route secrets by scope:

```go
import "github.com/goliatone/go-notifications/pkg/secrets"

// Create providers for each scope
systemProvider := secrets.NewStaticProvider(systemSecrets)
tenantProvider := secrets.NewStaticProvider(tenantSecrets)
userProvider := secrets.NopProvider{} // Users don't have custom secrets

// Build registry
registry := secrets.Registry{
    System: systemProvider,
    Tenant: tenantProvider,
    User:   userProvider,
}

// Resolve automatically routes to correct provider
values, err := registry.Resolve(
    secrets.Reference{Scope: secrets.ScopeSystem, ...},
    secrets.Reference{Scope: secrets.ScopeTenant, ...},
)
```

---

## Resolving Secrets at Runtime

### SimpleResolver

Wraps a single provider for basic resolution:

```go
import "github.com/goliatone/go-notifications/pkg/secrets"

resolver := secrets.SimpleResolver{
    Provider: secrets.NewStaticProvider(mySecrets),
}

// Resolve multiple references
values, err := resolver.Resolve(
    secrets.Reference{...},
    secrets.Reference{...},
)

// ErrNotFound entries are skipped (enables fallback chains)
for ref, val := range values {
    fmt.Printf("%s: %s\n", ref.Key, string(val.Data))
}
```

### Priority Fallback

The dispatcher resolves secrets in priority order:

```go
// Resolution order: user → tenant → system
refs := []secrets.Reference{
    {Scope: secrets.ScopeUser, SubjectID: userID, ...},
    {Scope: secrets.ScopeTenant, SubjectID: tenantID, ...},
    {Scope: secrets.ScopeSystem, SubjectID: "default", ...},
}

resolved, err := resolver.Resolve(refs...)
if err != nil {
    return err
}

// Use first found
for _, ref := range refs {
    if val, ok := resolved[ref]; ok {
        return val.Data, nil // Use this credential
    }
}
return nil, secrets.ErrNotFound
```

---

## Caching Resolved Secrets

Reduce external calls with `CachingResolver`:

```go
import (
    "time"
    "github.com/goliatone/go-notifications/pkg/secrets"
)

// Create inner resolver
innerResolver := secrets.SimpleResolver{
    Provider: externalSecretManager,
}

// Wrap with caching (5 minute TTL)
resolver := secrets.NewCachingResolver(innerResolver, 5*time.Minute)

// First call fetches from provider
values, _ := resolver.Resolve(ref)

// Subsequent calls use cache until TTL expires
values, _ = resolver.Resolve(ref) // Cached!
```

### Cache Behavior

| Scenario | Behavior |
|----------|----------|
| Cache hit (fresh) | Return cached value |
| Cache miss | Fetch from inner resolver, cache result |
| Cache expired | Fetch fresh value, update cache |
| Fetch error | Propagate error, don't cache failures |
| TTL <= 0 | Caching disabled, passthrough to inner |

---

## Masking Secrets in Logs

Prevent accidental exposure with `MaskValues`:

```go
import "github.com/goliatone/go-notifications/pkg/secrets"

// Resolved secrets
values := map[secrets.Reference]secrets.SecretValue{
    {Key: "api_key"}: {Data: []byte("sk_live_abc123xyz789")},
    {Key: "token"}:   {Data: []byte("ghp_1234567890abcdef")},
}

// Mask for safe logging
masked := secrets.MaskValues(values)
log.Printf("Secrets: %+v", masked)
// Output: Secrets: map[api_key:map[value:sk**************89 version:] token:map[value:gh**************ef version:]]
```

### Auto-Masked Fields

These field names are automatically masked:

- `token`, `access_token`, `refresh_token`
- `api_key`, `apikey`, `apiKey`
- `client_secret`, `secret`, `signing_key`
- `chat_id`, `webhook_url`, `from`, `from_email`

### Custom Masking

```go
import masker "github.com/goliatone/go-masker"

// Register additional fields to mask
masker.Default.RegisterMaskField("custom_token", "preserveEnds(2,2)")
```

---

## Integration with Module

Pass the resolver during module initialization:

```go
import (
    "github.com/goliatone/go-notifications/pkg/notifier"
    "github.com/goliatone/go-notifications/pkg/secrets"
)

// Setup providers
systemProvider := secrets.NewStaticProvider(map[secrets.Reference]secrets.SecretValue{
    {Scope: secrets.ScopeSystem, SubjectID: "default", Channel: "email", Provider: "sendgrid", Key: "default"}: {
        Data: []byte(os.Getenv("SENDGRID_API_KEY")),
    },
})

resolver := secrets.SimpleResolver{Provider: systemProvider}

// Or with caching for external secret managers
cachedResolver := secrets.NewCachingResolver(resolver, 5*time.Minute)

// Initialize module
mod, err := notifier.NewModule(notifier.ModuleOptions{
    Translator: translator,
    Adapters:   adapters,
    Secrets:    cachedResolver,
})
```

---

## Implementing Custom Store

For production, implement the `Store` interface with your database:

```go
import (
    "context"
    iface "github.com/goliatone/go-notifications/pkg/interfaces/secrets"
)

type PostgresStore struct {
    db *sql.DB
}

func (s *PostgresStore) Put(ctx context.Context, rec iface.Record) error {
    _, err := s.db.ExecContext(ctx, `
        INSERT INTO secrets (scope, subject_id, channel, provider, key, version, cipher, nonce, metadata)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        ON CONFLICT (scope, subject_id, channel, provider, key, version)
        DO UPDATE SET cipher = $7, nonce = $8, metadata = $9
    `, rec.Scope, rec.SubjectID, rec.Channel, rec.Provider, rec.Key, rec.Version, rec.Cipher, rec.Nonce, rec.Metadata)
    return err
}

func (s *PostgresStore) GetLatest(ctx context.Context, scope, subjectID, channel, provider, key string) (iface.Record, error) {
    var rec iface.Record
    err := s.db.QueryRowContext(ctx, `
        SELECT scope, subject_id, channel, provider, key, version, cipher, nonce, metadata
        FROM secrets
        WHERE scope = $1 AND subject_id = $2 AND channel = $3 AND provider = $4 AND key = $5
        ORDER BY version DESC LIMIT 1
    `, scope, subjectID, channel, provider, key).Scan(
        &rec.Scope, &rec.SubjectID, &rec.Channel, &rec.Provider, &rec.Key,
        &rec.Version, &rec.Cipher, &rec.Nonce, &rec.Metadata,
    )
    return rec, err
}

func (s *PostgresStore) GetVersion(ctx context.Context, scope, subjectID, channel, provider, key, version string) (iface.Record, error) {
    // Similar query with version filter
}

func (s *PostgresStore) Delete(ctx context.Context, scope, subjectID, channel, provider, key string) error {
    _, err := s.db.ExecContext(ctx, `
        DELETE FROM secrets
        WHERE scope = $1 AND subject_id = $2 AND channel = $3 AND provider = $4 AND key = $5
    `, scope, subjectID, channel, provider, key)
    return err
}

func (s *PostgresStore) List(ctx context.Context, scope, subjectID, channel, provider, key string) ([]iface.Record, error) {
    // Query with optional filters
}
```

---

## Best Practices

### 1. Never Hardcode Secrets

```go
// Bad
apiKey := "SG.hardcoded-key"

// Good
ref := secrets.Reference{
    Scope:     secrets.ScopeSystem,
    SubjectID: "default",
    Channel:   "email",
    Provider:  "sendgrid",
    Key:       "default",
}
values, _ := resolver.Resolve(ref)
apiKey := string(values[ref].Data)
```

### 2. Use Appropriate Scopes

```go
// System: shared credentials
{Scope: secrets.ScopeSystem, SubjectID: "default"}

// Tenant: customer-specific keys
{Scope: secrets.ScopeTenant, SubjectID: customerID}

// User: personal integrations
{Scope: secrets.ScopeUser, SubjectID: userID}
```

### 3. Enable Caching for External Managers

```go
// AWS Secrets Manager, HashiCorp Vault, etc.
resolver := secrets.NewCachingResolver(
    awsSecretsResolver,
    5*time.Minute,
)
```

### 4. Use Encrypted Storage in Production

```go
// Generate key securely (use KMS in production)
key := make([]byte, 32)
rand.Read(key)

provider, _ := secrets.NewEncryptedStoreProvider(dbStore, key)
```

### 5. Always Mask Before Logging

```go
// Before logging any secret values
masked := secrets.MaskValues(resolvedSecrets)
logger.Info("using credentials", "secrets", masked)
```

### 6. Implement Fallback Chains

```go
// Try user-specific, then tenant, then system
for _, scope := range []secrets.Scope{
    secrets.ScopeUser,
    secrets.ScopeTenant,
    secrets.ScopeSystem,
} {
    if val, ok := resolved[secrets.Reference{Scope: scope, ...}]; ok {
        return val.Data
    }
}
```

---

## Complete Example

```go
package main

import (
    "context"
    "log"
    "os"
    "time"

    i18n "github.com/goliatone/go-i18n"
    "github.com/goliatone/go-notifications/pkg/adapters"
    "github.com/goliatone/go-notifications/pkg/adapters/sendgrid"
    "github.com/goliatone/go-notifications/pkg/notifier"
    "github.com/goliatone/go-notifications/pkg/secrets"
)

func main() {
    ctx := context.Background()

    // Create static provider with secrets from environment
    systemProvider := secrets.NewStaticProvider(map[secrets.Reference]secrets.SecretValue{
        {
            Scope:     secrets.ScopeSystem,
            SubjectID: "default",
            Channel:   "email",
            Provider:  "sendgrid",
            Key:       "default",
        }: {
            Data: []byte(os.Getenv("SENDGRID_API_KEY")),
        },
        {
            Scope:     secrets.ScopeSystem,
            SubjectID: "default",
            Channel:   "sms",
            Provider:  "twilio",
            Key:       "default",
        }: {
            Data: []byte(os.Getenv("TWILIO_AUTH_TOKEN")),
        },
    })

    // Create resolver with caching
    resolver := secrets.NewCachingResolver(
        secrets.SimpleResolver{Provider: systemProvider},
        5*time.Minute,
    )

    // Setup translator
    translator, _ := i18n.New()

    // Setup adapters
    sg, _ := sendgrid.New(sendgrid.Options{
        From: "notifications@example.com",
        // API key will be resolved from secrets
    })

    // Initialize module with secrets resolver
    mod, err := notifier.NewModule(notifier.ModuleOptions{
        Translator: translator,
        Adapters:   []adapters.Messenger{sg},
        Secrets:    resolver,
    })
    if err != nil {
        log.Fatal(err)
    }

    // Send notification - secrets resolved automatically
    err = mod.Manager().Send(ctx, notifier.Event{
        DefinitionCode: "welcome",
        Recipients:     []string{"user@example.com"},
    })
    if err != nil {
        log.Printf("Send failed: %v", err)
    }
}
```

---

## Troubleshooting

### Secret Not Found

1. **Check reference fields**: All required fields must be non-empty
2. **Verify scope**: Ensure provider is registered for the scope
3. **Check SubjectID**: Must match exactly (case-sensitive)

```go
if err := secrets.ValidateReference(ref); err != nil {
    log.Printf("Invalid reference: %v", err)
}
```

### Encryption Errors

1. **Key length**: Must be exactly 32 bytes for ChaCha20-Poly1305
2. **Nonce corruption**: Ensure nonce is stored/retrieved correctly
3. **Data integrity**: Cipher includes authentication tag

### Cache Issues

1. **Stale data**: Reduce TTL or clear cache on secret rotation
2. **Memory growth**: Monitor cache size for long-running services

---

## Next Steps

- [GUIDE_ADAPTERS.md](GUIDE_ADAPTERS.md) - Configure adapters that use secrets
- [GUIDE_INTEGRATION.md](GUIDE_INTEGRATION.md) - Full module setup
- [GUIDE_GETTING_STARTED.md](GUIDE_GETTING_STARTED.md) - Quick start guide
