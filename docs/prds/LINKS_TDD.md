# Link Generation - Technical Design Document

## Table of Contents
1. [Overview](#overview)
2. [Goals](#goals)
3. [Non-Goals](#non-goals)
4. [Key Decisions](#key-decisions)
5. [Proposed Interfaces](#proposed-interfaces)
6. [Flow & Integration Points](#flow--integration-points)
7. [Securelink Compatibility](#securelink-compatibility)
8. [Storage & Observability](#storage--observability)
9. [Security Considerations](#security-considerations)
10. [Testing Strategy](#testing-strategy)
11. [Open Questions](#open-questions)

---

## Overview

Notifications frequently include action links (e.g., password resets, export downloads). We need a consistent way to generate secure, user-gated, and optionally one-time links while avoiding hard dependencies on `../go-urlkit/securelink`. This document defines the link generation flow and interfaces for per-channel link building, link resolution outputs, and optional persistence/analytics hooks.

---

## Goals

- Provide secure link generation for notification payloads (`action_url`, `manifest_url`, etc.).
- Run link generation per channel (after channel overrides) to allow channel-specific URLs.
- Avoid hard dependency on `go-urlkit/securelink` while remaining compatible.
- Enable consumer applications to store or audit generated links.
- Keep templates and message metadata consistent with resolved links.

---

## Non-Goals

- Implementing link landing endpoints or auth gates (belongs to host app).
- Enforcing storage requirements inside this module.
- Building a new secure link algorithm (delegate to `securelink`-compatible implementations).

---

## Key Decisions

1. **Per-channel execution**: link building occurs after channel overrides are applied and before template render.
2. **ResolvedLinks output**: builders return a `ResolvedLinks` struct; link fields are applied to payload + message entity, and `ResolvedLinks.Metadata` is applied to message metadata.
3. **Storage via optional interfaces**: consumers can inject a `LinkStore` and/or observer hook; default behavior is no persistence.
4. **Securelink compatibility**: define local interfaces that match `securelink.Manager` and `securelink.Configurator` so adapters can wrap the real package without a direct dependency.
5. **Failure semantics**: configurable per dependency (builder/store/observer strict vs lenient).
6. **Invocation order**: LinkStore/Observer run before persistence/delivery (gating-first).
7. **Merge precedence**: explicit precedence table (builder > overrides > original).
8. **`url` deprecation**: normalize to `action_url` and track as a breaking change.
9. **Template helper**: `secure_link(...)` delegates to the builder flow.
10. **ResolvedURLs contract**: fixed keys (`url`, `action_url`, `manifest_url`) plus namespaced extras (e.g. `meta.token`).
11. **LinkRecord construction**: dispatcher constructs records when builder does not provide them.
12. **Message storage**: link fields are stored on the message entity only (breaking change; metadata is not the source of truth).

---

## Merge Precedence

| Priority | Source |
| --- | --- |
| 1 | LinkBuilder output |
| 2 | Channel overrides |
| 3 | Original event context |

When only `url` is provided, the resolver treats it as the `action_url` source during migration.

---

## Proposed Interfaces

### Link Builder

```go
type LinkBuilder interface {
	Build(ctx context.Context, req LinkRequest) (ResolvedLinks, error)
}
```

### Link Request

```go
type LinkRequest struct {
	EventID       string
	Definition    string
	Recipient     string
	Channel       string
	Provider      string
	TemplateCode  string
	MessageID     string
	Locale        string
	Payload       map[string]any // channel-aware payload (after overrides)
	Metadata      map[string]any // message metadata (current state)
	ResolvedURLs  map[string]string // fixed keys + namespaced extras (e.g. meta.token)
}
```

### Resolved Links

```go
type ResolvedLinks struct {
	ActionURL    string
	ManifestURL  string
	URL          string
	Metadata     map[string]any // builder-provided metadata (tokens, expiry, ids)
	Records      []LinkRecord    // optional, for storage/analytics
}
```

### Link Records (Optional Storage)

```go
type LinkRecord struct {
	ID           string
	URL          string
	Channel      string
	Recipient    string
	MessageID    string
	Definition   string
	ExpiresAt    time.Time
	Metadata     map[string]any
}
```

### Link Store (Optional)

```go
type LinkStore interface {
	Save(ctx context.Context, records []LinkRecord) error
}
```

### Link Observer (Optional Hook)

```go
type LinkObserver interface {
	OnLinksResolved(ctx context.Context, info LinkResolution)
}

type LinkResolution struct {
	Request  LinkRequest
	Resolved ResolvedLinks
}
```

### Failure Semantics Configuration

```go
type FailureMode string

const (
	FailureStrict  FailureMode = "strict"  // abort on error
	FailureLenient FailureMode = "lenient" // log and continue
)

type FailurePolicy struct {
	Builder  FailureMode
	Store    FailureMode
	Observer FailureMode
}
```

### Template Helper

Templates can use `secure_link(...)` to access resolved links. The helper delegates to the builder flow (it does not bypass per-channel overrides) and should read from the already-resolved payload/message fields.

Helper rules:

- Never invokes link generation on its own; it reads `action_url`, `manifest_url`, and `url` from the resolved payload/message.
- Assumes the dispatcher already ran the builder per channel; overrides remain applied.
- Treats `action_url` as canonical (uses deprecated `url` only for migration/back-compat).

Usage:

```text
{{ secure_link(action_url, url) }}     // defaults to action_url, fallback to url
{{ secure_link(manifest_url) }}        // manifest link
```

The templates service registers `secure_link` by default.

---

## Flow & Integration Points

1. **Event intake** builds payload (context + overrides).
2. **Channel overrides** are applied (per channel).
3. **Link builder invoked** with `LinkRequest` (per channel).
4. **Resolved links applied**:
   - `payload["action_url"]`, `payload["manifest_url"]` (and deprecated `payload["url"]` during migration)
   - message entity link fields (source of truth); metadata holds only `ResolvedLinks.Metadata`
5. **Template render** uses updated payload and helper accessors.
6. **Observer/LinkStore** invoked if configured (before persistence/delivery).
7. **Message persisted and delivered**.

If no builder is configured, the current behavior remains (pass-through from payload/overrides) and LinkStore/Observer are not invoked.
If the builder fails in lenient mode, pass-through links are applied, the observer runs, and the store is skipped.

---

## Securelink Compatibility

We mirror `securelink` interfaces locally so implementers can wrap `../go-urlkit/securelink` without importing it in the core module.

```go
type SecureLinkManager interface {
	Generate(route string, payloads ...SecureLinkPayload) (string, error)
	Validate(token string) (map[string]any, error)
	GetAndValidate(fn func(string) string) (SecureLinkPayload, error)
	GetExpiration() time.Duration
}

type SecureLinkPayload map[string]any

type SecureLinkConfigurator interface {
	GetSigningKey() string
	GetExpiration() time.Duration
	GetBaseURL() string
	GetQueryKey() string
	GetRoutes() map[string]string
	GetAsQuery() bool
}
```

An adapter package can implement `LinkBuilder` using a `SecureLinkManager` backed by `go-urlkit/securelink`, while keeping this module dependency-free. See `adapters/securelink` for an example builder + store and manager adapter.

Example integration (host app):

```go
import (
	linksecure "github.com/goliatone/go-notifications/adapters/securelink"
	"github.com/goliatone/go-notifications/pkg/notifier"
	urlsecure "github.com/goliatone/go-urlkit/securelink"
)

cfg := urlsecure.Config{
	// SigningKey, BaseURL, Routes, QueryKey...
}
rawManager, _ := urlsecure.NewManager(cfg)
manager := linksecure.WrapManager(rawManager)
builder := linksecure.NewBuilder(manager,
	linksecure.WithActionRoute("reset-password"),
	linksecure.WithManifestRoute("export-download"),
)
store := linksecure.NewMemoryStore()

module, _ := notifier.NewModule(notifier.ModuleOptions{
	LinkBuilder: builder,
	LinkStore:   store,
})
_ = module
```

If your host app prefers passing config instead of constructing `urlsecure.Manager` directly, `linksecure.NewManager(cfg)` is equivalent.

---

## Storage & Observability

We support two integration mechanisms:

- **LinkStore**: synchronous persistence for one-time or auditable links.
- **LinkObserver**: fire-and-forget hook for analytics or logging.

Consumers can choose either or both; defaults are no-op (`links.NopStore`, `links.NopObserver`). If the builder does not provide `Records`, the dispatcher derives them from `ResolvedLinks` and `LinkRequest`.

### Recommended Storage Patterns

- Use idempotent keys (message ID + link key or URL) to avoid duplicate records.
- Persist expiration/usage state to enforce one-time or gated links in the host app.
- Keep `LinkRecord.Metadata` small and sanitized; avoid storing raw secrets.

### Lifecycle Hooks

- `LinkStore.Save` runs after link resolution and before message persistence/delivery.
- `LinkObserver.OnLinksResolved` should be fast; queue heavy analytics work.
- The dispatcher adds `link_key` metadata when missing to map records to fields.

---

## Security Considerations

- Link payloads should include recipient identity, channel, and message IDs to support gating.
- For one-time usage, the consumer should persist `LinkRecord` with usage state and expiration.
- Avoid leaking secrets into logs; link metadata should be sanitized before logging.

---

## Testing Strategy

- Unit tests for `LinkBuilder` contract behavior (empty, partial, full).
- Dispatcher tests:
  - Builder invoked per channel after overrides.
  - ResolvedLinks applied to payload + message entity (metadata only receives `ResolvedLinks.Metadata`).
  - Hook/store invoked with correct records.
- Securelink adapter tests in integration package (outside core module).

---

## Open Questions

- None at this time.
