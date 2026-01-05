# Documentation Guides TODO

This document tracks the planned user guides for `go-notifications`. Each guide helps users understand and use specific features of the package.

## Guide Status Legend

- `pending` - Not started
- `in-progress` - Currently being written
- `review` - Draft complete, needs review
- `done` - Published and complete

---

## 1. GUIDE_GETTING_STARTED.md

**Status**: `done`

**Purpose**: Get users sending their first notification in under 5 minutes.

**Sections**:
- Overview of the module
- Installation
- Minimal setup (Module + Console adapter)
- Sending your first notification
- Basic configuration options
- Next steps / where to go from here

**Primary Audience**: New users
**Complexity**: Beginner

---

## 2. GUIDE_ADAPTERS.md

**Status**: `review`

**Purpose**: Comprehensive guide to configuring and using delivery channel adapters.

**Sections**:
- Overview of the adapter architecture
- Adapter registry and selection
- Built-in adapters:
  - Console (development/debugging)
  - SMTP
  - SendGrid
  - Mailgun
  - AWS SES
  - Twilio (SMS)
  - WhatsApp
  - Telegram
  - Slack
  - Firebase (Push)
  - AWS SNS
  - Webhook
- Adapter configuration patterns
- Secrets management for API keys/tokens
- Writing custom adapters (implementing `Messenger` interface)
- Multi-channel fan-out
- Retry policies and error handling

**Primary Audience**: All users
**Complexity**: Intermediate

---

## 3. GUIDE_TEMPLATES.md

**Status**: `review`

**Purpose**: How to create and manage notification templates with localization.

**Sections**:
- Template rendering pipeline overview
- Template syntax (Pongo2/Django-style)
- Creating templates programmatically
- Template schema validation (required/optional fields)
- Localization with go-i18n
  - Translation helper `{{ t(locale, key, args...) }}`
  - Locale fallback chains
- Per-channel template variants
- Template caching
- Referencing go-cms blocks as template sources
- Common patterns and examples (welcome email, password reset, etc.)

**Primary Audience**: Template authors
**Complexity**: Intermediate

---

## 4. GUIDE_PREFERENCES.md

**Status**: `review`

**Purpose**: Managing notification preferences and opt-in/opt-out settings.

**Sections**:
- Preference scopes (global, definition, channel)
- Creating and updating preferences
- Preference evaluation before delivery
- Quiet hours and scheduling rules
- Subscription groups
- Building preference UI (APIs for frontends)
- Inheritance and override patterns

**Primary Audience**: App developers
**Complexity**: Intermediate

---

## 5. GUIDE_INBOX.md

**Status**: `review`

**Purpose**: Implementing in-app notification centers with real-time updates.

**Sections**:
- Inbox architecture overview
- Persisting in-app notifications
- Read/unread tracking
- Pagination and filtering
- Mark as read / snooze / dismiss operations
- Real-time delivery via Broadcaster interface
  - WebSocket integration
  - SSE integration
  - Webhook integration
- Building notification center UI

**Primary Audience**: Frontend/full-stack devs
**Complexity**: Intermediate

---

## 6. GUIDE_EVENTS.md

**Status**: `review`

**Purpose**: How to submit notification events and understand the delivery pipeline.

**Sections**:
- Event structure and payloads
- Sync vs async event submission
- Payload validation against definition schemas
- Recipient resolution
- Scheduled delivery with `ScheduleAt`
- Digest grouping and batching
- Event lifecycle and status tracking
- Delivery attempts and retries

**Primary Audience**: Backend developers
**Complexity**: Intermediate

---

## 7. GUIDE_DEFINITIONS.md

**Status**: `review`

**Purpose**: Creating and managing notification types.

**Sections**:
- What are notification definitions?
- Creating definitions
- Channel enablement per definition
- Throttling policies
- i18n bindings
- Definition metadata contracts
- Archiving and soft deletes
- Guard rails for go-admin editors

**Primary Audience**: Content managers/devs
**Complexity**: Beginner

---

## 8. GUIDE_INTEGRATION.md

**Status**: `review`

**Purpose**: Integrating go-notifications with host applications.

**Sections**:
- Module initialization patterns
- Dependency injection setup
- Integrating with go-admin
- Integrating with go-cms (template sources)
- Command pattern usage (go-command)
- Storage providers (Memory vs Bun/PostgreSQL)
- Configuration via cfgx
- Logging and observability hooks
- Testing with in-memory repositories

**Primary Audience**: Platform engineers
**Complexity**: Advanced

---

## 9. GUIDE_SECRETS.md

**Status**: `review`

**Purpose**: Securely managing API keys, tokens, and credentials for adapters.

**Sections**:
- Secrets architecture overview
- Built-in secret providers:
  - Static secrets
  - Encrypted store
  - Memory store
- Registering secret providers
- Resolving secrets at runtime
- Caching resolved secrets
- Masking secrets in logs
- Best practices for production

**Primary Audience**: DevOps/security
**Complexity**: Intermediate

---

## Summary

| Guide | Audience | Complexity | Status |
|-------|----------|------------|--------|
| GUIDE_GETTING_STARTED | New users | Beginner | `review` |
| GUIDE_ADAPTERS | All users | Intermediate | `review` |
| GUIDE_TEMPLATES | Template authors | Intermediate | `review` |
| GUIDE_PREFERENCES | App developers | Intermediate | `review` |
| GUIDE_INBOX | Frontend/full-stack | Intermediate | `review` |
| GUIDE_EVENTS | Backend developers | Intermediate | `review` |
| GUIDE_DEFINITIONS | Content managers/devs | Beginner | `review` |
| GUIDE_INTEGRATION | Platform engineers | Advanced | `review` |
| GUIDE_SECRETS | DevOps/security | Intermediate | `review` |

---

## Notes

- Guides follow the structure established in `go-formgen/docs/GUIDE_*.md`
- Each guide should include runnable code examples
- Reference existing internal docs (`NTF_TDD.md`, `NTF_TEMPLATES.md`, etc.) for technical details
- Prioritize GUIDE_GETTING_STARTED first to help new users onboard quickly
