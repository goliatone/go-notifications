# LOGIN_TDD

## Summary

We want go-admin + quickstart to support a full user workflow (invite, password reset, signup/registration, and opt-in UX toggles) and to standardize secure links via `go-urlkit/securelink`. Today these flows are partially implemented across the example app, go-users commands, and go-auth password reset records. This document captures the work needed to deliver the full workflow with clear opt-in controls, plus securelink integration as one part of that work.

Note: `go-urlkit/securelink` exposes a `Configurator` interface and a `Manager` interface (`Generate`, `Validate`, `GetAndValidate`, `GetExpiration`). go-auth/go-users do not currently use it, so integration work is required.

## Local Package Paths

- go-admin: `/Users/goliatone/Development/GO/src/github.com/goliatone/go-admin`
- go-auth: `/Users/goliatone/Development/GO/src/github.com/goliatone/go-auth`
- go-users: `/Users/goliatone/Development/GO/src/github.com/goliatone/go-users`
- go-urlkit: `/Users/goliatone/Development/GO/src/github.com/goliatone/go-urlkit`
- go-notifications: `/Users/goliatone/Development/GO/src/github.com/goliatone/go-notifications`

## Full Workflow Scope

- **Login**: sign-in with feature-appropriate UI (password reset link optional).
- **Invite**: issue, verify, and accept invitations.
- **Password reset**: request and confirm reset with replay protection.
- **Signup/registration**: optional self-registration (open/allowlist/closed).
- **Opt-in UX**: enable/disable signup and hide password reset UI when disabled.

## Current State

- **go-admin (example app)**:
  - Onboarding endpoints under `/admin/api/onboarding/*` create UUID tokens and store them in user metadata.
  - Token validation scans all users and checks metadata. This is acceptable for demo data but not production scale.
  - Feature flags and registration modes are wired in `examples/web/setup/onboarding.go` and `examples/web/main.go`.
- **go-users**:
  - `UserInviteCommand` generates a UUID token and stores it in `Metadata["invite"]`.
  - `UserPasswordResetCommand` only applies a new password hash; token issuance/validation is handled outside.
- **go-auth**:
  - Password reset initialization/finalization uses a `password_reset` table. It does not use securelink.
  - No securelink integration; links are surfaced by the transport or host app.

## Gaps / What’s Left for a Full Workflow

- Quickstart does not ship invite acceptance UI or registration UI yet; onboarding API routes are wired directly in `examples/web/main.go`, not in quickstart.
- Default registration UI should be provided by go-admin/quickstart with override hooks (routes/templates), so hosts can replace it when needed.
- No shared secure link manager; token issuance/validation is inconsistent across repos.
- No durable token registry for invite/self-registration flows (demo metadata scan only).
- Email delivery and templates are not provided (host app responsibility).
- Default auth UI only provides a static reset expiry hint; no password policy feedback or per-link expiration data yet (hybrid policy source planned: config defaults with API override).
- Admin UI does not surface invite expiration/remaining time.
- go-auth enforces login-attempt throttling and has an account verification handler, but go-admin/quickstart do not expose lockout/verification/session management flows.
- Rate limit/lockout/verification errors are not translated into user-facing UI messages; use go-errors `ErrorMapper`/`MapToError` to standardize `text_code`.

## Goals

- Provide full workflow wiring with explicit opt-in controls (signup and password reset visibility).
- Use securelink for invite, password reset, and registration links.
- Avoid scanning users to validate tokens.
- Standardize token payloads (user_id, action, scope, jti, issued_at, expires_at).
- Keep transport-agnostic command boundaries (go-users/go-auth stay independent of HTTP).
- Provide quickstart wiring so go-admin apps can opt in with minimal setup.
- Ship a functional default UI while allowing host apps to override routes/templates.
- Provide a unified audit trail for onboarding/auth flows across go-users and go-auth.
- Expose UX feedback for password policy, link expiration, and rate limiting in the default UI (or host overrides).

## Non-goals

- Email delivery and templates (handled by host apps).
- Full public signup UX outside of the admin shell (host app responsibility).
- Changing go-auth JWT token service used for session auth.

## Proposed Design

### Ownership split (recommended)

Keep securelink generation with the domain that owns the workflow, and keep notifications link agnostic.

- **go-urlkit/securelink**: signing + validation only (no persistence, no user logic).
- **go-users**: owns invite + self-registration + password reset secure links (issuance, validation, replay protection).
- **go-auth**: provides auth primitives (password hash updates, login throttling); deprecate built-in reset flow/table once go-users takes over.
- **go-notifications**: link consumer only (builds URLs using securelink-compatible interfaces).
- **go-admin/quickstart**: integration wiring (build securelink manager from env and pass into go-users/go-notifications; go-auth only for legacy reset).

This preserves package independence while enabling a seamless combined flow.

### Feature gating and UX controls

- Add explicit feature flags for:
  - `users.invite` (invite issuance + acceptance UI)
  - `users.password_reset` (password reset UI + endpoints)
  - `users.signup` (signup UI + endpoint)
- Update login template to hide the "Forgot password" link when the feature is disabled.
- Add optional signup link to login template, controlled by `users.signup`.
- Ensure quickstart can inject these flags into the auth UI view context.

### Workflow routing

- Quickstart should optionally register onboarding API routes under `/admin/api/onboarding/*`.
- Add a signup UI route (template + handler) and a documented recipe for host apps to implement their own if they need to.
- Keep invite acceptance and password reset confirmation as public routes with CSRF handling as needed.

### Securelink integration

#### Interfaces

Introduce securelink-compatible interfaces locally in go-users and go-auth (mirroring go-urlkit/securelink), so packages do not depend on go-urlkit directly.

```go
type SecureLinkManager interface {
	Generate(route string, payloads ...map[string]any) (string, error)
	Validate(token string) (map[string]any, error)
	GetAndValidate(fn func(string) string) (map[string]any, error)
	GetExpiration() time.Duration
}

type SecureLinkConfigurator interface {
	GetSigningKey() string
	GetExpiration() time.Duration
	GetBaseURL() string
	GetQueryKey() string
	GetRoutes() map[string]string
	GetAsQuery() bool
}
```

Provide thin adapters around `securelink.Manager` so go-urlkit is the default implementation. Accept `SecureLinkConfigurator` in quickstart to build the manager.

Note: go-notifications already mirrors these interfaces under `pkg/links`, so align signatures there.

#### Token payloads

Use securelink payloads with a standard schema across packages. Include `scope` and allow opt-in enforcement strategies.

- `action`: `invite`, `password_reset`, `register`
- `user_id`
- `email` (optional)
- `scope` (`tenant_id`, `org_id`)
- `jti` (unique ID for replay prevention)
- `issued_at` (RFC3339)
- `expires_at` (RFC3339)
- `source` (optional: `go-users`, `go-auth`, `go-admin`)

Additional per-channel keys can be appended by go-notifications (e.g., `message_id`, `template`, `link_key`).

Scope enforcement strategy:
- Provide a hook/option in go-users/go-auth securelink validation that accepts a `ScopeEnforcer` function.
- Default behavior is non-enforcing (payload carries scope for audit only).
- Hosts can opt into `enforce`, `enforce+log`, or `log-only` by supplying their own implementation.

#### Storage and replay protection

We need a server-side record to mark tokens as used and prevent replay.

Password reset already has a durable record in go-users migrations (`password_reset`), so use it for reset replay protection (ID/jti + status) and plan a migration off go-auth's `password_reset` table.
Option A (chosen): extend the go-users `password_reset` schema so securelink lifecycle data is persisted explicitly:
- `jti` (unique token ID; store securelink `jti` or map to `id` if desired)
- `issued_at` (optional if `created_at` is the canonical issued time)
- `expires_at` (explicit expiry per token)
- `used_at` (or `consumed_at`) to enforce single-use
- optional `scope_tenant_id`, `scope_org_id` for validation
- index/unique constraint on `jti` plus an expiry/status index for cleanup

For invite/self-registration tokens:

- Decision: use a generic `user_token` table in go-users (type, user_id, jti, status, expires_at, used_at) for invite/registration (and reset when migrated).

#### Audit trail / activity logging

We need a complete audit trail for invites, registration, and password reset that is visible in the admin activity feed:

- Use go-users `ActivitySink`/`ActivityRepository` (user_activity) as the canonical audit store.
- Bridge go-auth `ActivitySink` events into go-users records so login/reset events show up in the same feed.
- Emit activity in all new flows (issue + consume + failure cases), and avoid storing raw tokens; log `jti`, `expires_at`, `action`, and `source` instead.
- Proposed verbs/channels:
  - `user.invite` (existing), `user.invite.consumed` (channel: `invites`)
  - `user.registration.requested`, `user.registration.completed` (channel: `registration`)
  - `user.password.reset.requested`, `user.password.reset` (channel: `password`)
  - `auth.login.success`, `auth.login.failure`, `auth.impersonation.*` (channel: `auth`)

### Error mapping integration (go-errors)

Use go-errors as the canonical error shape and mapping layer for auth/onboarding UX.

- Response shape: API errors should return `{"error": {category, code, text_code, message, ...}}`.
- Mapping entrypoint: `goerrors.MapToError(err, mappers)` (quickstart already uses this for `/admin/api/*`).
- Default mappers: `goerrors.DefaultErrorMappers()` includes `MapHTTPErrors` and `MapAuthErrors` (see `go-errors/response.go`).
- Add auth/onboarding mappers to normalize non-go-errors and legacy errors into `TextCode` values used by UI.
- Quickstart should allow injecting extra mappers (append to defaults) so host apps can extend or override mappings.

Recommended text codes to cover in mappers (reuse existing go-auth/go-users codes where available, add missing):
- `RESET_RATE_LIMIT`, `TOO_MANY_ATTEMPTS`, `ACCOUNT_LOCKED`
- `ACCOUNT_SUSPENDED`, `ACCOUNT_DISABLED`, `ACCOUNT_ARCHIVED`, `ACCOUNT_PENDING`
- `VERIFICATION_REQUIRED`, `VERIFICATION_EXPIRED`
- `TOKEN_EXPIRED`, `TOKEN_MALFORMED`, `INVITE_EXPIRED`, `INVITE_USED`
- `RESET_NOT_ALLOWED`, `FEATURE_DISABLED`

Mapping strategy:
- Prefer returning `*goerrors.Error` with `TextCode` at the source (go-auth/go-users/go-admin).
- Use ErrorMapper functions for non-go-errors (string errors, DB errors, legacy handlers).
- UI reads `error.text_code` and maps to user-facing copy.

## Work Items

### go-users

- Add `SecureLinkManager` (and optional `SecureLinkConfigurator`) interface in `pkg/types` (or `service`).
- Add a storage-backed `user_token` registry with `type` for invite/registration (and reset when migrated), storing `jti` + `used_at` (+ `expires_at`).
- Add a migration to extend `password_reset` with `jti`, `expires_at`, `used_at`, optional `issued_at`, and optional scope IDs; add a unique index on `jti`.
- Update `UserInviteCommand` to:
  - Generate securelink token with payload and store `jti` in the token registry.
  - Return the secure link (or token + expires_at) in `UserInviteResult`.
- Add `UserRegistrationRequestCommand` (or extend existing create flow) to:
  - Issue a securelink token for signup completion when self-registration is enabled.
  - Record the token in the registry with scope.
- Add password reset issuance/validation commands:
  - Issue a securelink token with payload (`action=password_reset`, `user_id`, `jti`, `expires_at`).
  - Persist securelink lifecycle fields in go-users `password_reset` (`jti`, `expires_at`, `used_at`, optional `issued_at`).
  - Validate/consume reset tokens before applying `UserPasswordReset`.
- Add a command to validate and consume invite/registration tokens:
  - Validate token via manager, check `jti` unused, mark used, return payload.
- Emit activity records for invite/registration issue + consume flows (no raw tokens; log `jti` + `expires_at`).
- Coordinate bulk invite/expire capabilities with go-users CRUD/bulk endpoints.

### go-auth (optional)

- Deprecate/remove go-auth password reset handlers and `password_reset` table once go-users owns reset flows; provide a migration/compat path.
- Provide an adapter so go-auth `ActivitySink` events can be recorded in go-users `user_activity`.
- Add account lockout configuration (threshold + duration) via a storage interface:
  - Default implementation uses code defaults with config-driven overrides.
  - Optional per-tenant implementation stores policy per tenant.
  - Emit lockout events when thresholds are hit.
- Add email verification gating + resend flow metadata (activation requires verification when enabled).
- Add session registry APIs for listing/revoking sessions and force logout via a pluggable service interface (implementations for DB-backed, token-service, or hybrid).
- Define remember-me/trusted-device semantics (cookie/token duration policy).
- Standardize rate-limit/lockout/verification error codes via go-errors:
  - Extend go-errors with auth/onboarding `ErrorMapper` helpers (or add a quickstart hook to inject mappers).
  - Ensure errors carry `TextCode` so UI can map `error.text_code` to messaging.

### go-errors (optional)

- Add auth/onboarding error mappers (rate limit, lockout, verification, invite/reset states).
- Document a canonical `TextCode` list and keep it in sync with go-auth/go-users constants.
- Provide helper to compose `DefaultErrorMappers()` with host-supplied mappers.

### go-notifications (optional)

- Provide payload fields and hooks for lockout, verification, and link-expiration messaging (delivery stays host-owned).
- Define notification event contracts and template variables for:
  - account lockout (reason, lockout_until, unlock_url)
  - email verification (verify_url, expires_at, resend_allowed)
  - invite/reset expiry hints (expires_at, remaining_minutes)

### go-admin (example app)

- Replace metadata token generation in `examples/web/handlers/onboarding.go` with securelink manager usage.
- Stop scanning user metadata for tokens; use token registry lookups.
- Update invite accept and password reset confirm to validate/consume tokens via the new command(s).
- Add a default registration UI route + template with override hooks for host apps.
- Keep the feature flags (`users.invite`, `users.password_reset`, `users.signup`) as-is.
- Ensure onboarding flows emit activity records and surface them in the admin activity feed.
- Add password policy feedback to reset/registration UI (hybrid: config defaults with API override; expose policy hints to host overrides).
- Surface invite expiration/remaining time in admin UI via a token metadata endpoint (expires_at/used_at), and use it in invite list or user detail view.
- Map rate-limit/lockout/verification errors to user-facing messages in auth UI using go-errors `ErrorResponse` (`error.text_code`).

### quickstart

- Add configuration to build a `securelink.Manager`:
  - Env keys: `ADMIN_SECURELINK_KEY`, `ADMIN_SECURELINK_BASE_URL`, `ADMIN_SECURELINK_QUERY_KEY`, `ADMIN_SECURELINK_AS_QUERY`, `ADMIN_SECURELINK_EXPIRATION`.
  - Routes: `invite_accept`, `password_reset`, `register` (app-defined paths).
- Expose a helper (e.g., `quickstart.NewSecureLinkManager(cfg)`) that returns the manager or nil if disabled.
- Pass the manager into go-users and go-auth (and go-notifications link builders) via options or config.
- Wire a shared activity sink that records both go-users and go-auth audit events into the same activity feed.
- Add optional onboarding route registration helpers to quickstart (invite, accept/verify, reset request/confirm, register).
- Add view context helpers to control the visibility of password reset and signup links in auth templates.
- Add registration UI wiring with documented override hooks (routes + templates + view context).
- Add error mapper configuration to quickstart error handling:
  - New option to append custom `ErrorMapper` functions to `goerrors.DefaultErrorMappers()`.
  - Ensure `NewFiberErrorHandler` uses the merged mapper list for API responses.

### Wiring points (concrete)

- **go-users**: extend `InviteCommandConfig` and new registration commands to accept `SecureLinkManager`.
- **go-auth**: extend `InitializePasswordResetHandler` to accept `SecureLinkManager` or adapter; update handler and controller to use it.
- **go-notifications**: use `adapters/securelink` with the same manager instance or a builder wrapper.
- **go-admin/quickstart**: assemble a single manager instance and inject it into:
  - go-users commands (invite/registration)
  - go-auth reset handlers
  - go-notifications link builder (if used)
- **quickstart/error handler**: build mapper list as `DefaultErrorMappers() + custom mappers` and pass into the Fiber error handler for `/admin/api/*` responses.

### Documentation

- Document full workflow wiring (flags, routes, templates, securelink configuration).
- Add notes about replay protection and token storage.
- Document password policy/lockout/session hooks and how to surface them in UI/notifications (including go-errors error mapping).
- Document error response shape, mapper configuration, and the canonical `TextCode` list for auth/onboarding UI.
- Document notification payload contracts and sample template variables.

### Tests

- Unit tests for:
  - securelink payload composition and validation.
  - replay protection (used `jti`).
  - invite and reset commands returning correct link/token + expiration.
  - activity emission for invite/registration/reset issue + consume flows.
  - error mapper coverage for rate-limit/lockout/verification text codes.
- Integration tests:
  - end-to-end invite + accept.
  - password reset request + confirm.
  - optional signup enabled/disabled.
  - activity feed includes auth + onboarding events.
  - UI feedback for password policy, rate-limit messaging, and invite expiration display.

## User Stories (Grouped by Package)

### go-auth

- As security, I can enforce account lockout after N failed login attempts.
- As an admin, I can require email verification before account activation.
- As a user, I can see my active sessions and revoke them.
- As security, I can force logout all sessions for a user.
- As a user, I can choose to stay logged in on trusted devices.
- As a user, I'm informed when I've hit rate limits on reset requests.
- As an admin, I can trigger a password reset and know the link cannot be replayed.
- As security, I can emit audit events for login/reset/impersonation so they appear in the shared activity feed.

### go-users

- As an admin, I can invite a user and share a secure link that expires automatically.
- As an admin, I can resend or reissue invites without leaking previous links.
- As an admin, I can bulk-invite multiple users.
- As an admin, I can bulk-expire pending invites.
- As an invited user, I can accept my invite via a secure link that clearly expires.
- As a user, I can sign up when self-registration is enabled and see no signup option when disabled.
- As support, I can verify whether a link is expired or already used.
- As support, I can reissue a new link without manual cleanup.
- As security, I can query a unified log of auth/onboarding events from the activity store.

### go-urlkit/securelink

- As security, I can enforce short-lived signed links.
- As security, I can disable or rotate signing keys without changing feature code.

### go-notifications

- As a user, I receive notification when my account is locked and how to unlock.
- As a user, I can resend verification email if it didn't arrive.
- As a user, I can see when my invite/reset link expires before clicking.

### go-admin / quickstart

- As an admin, I can enable or disable self-registration and password reset, and the UI reflects those settings.
- As an admin, I can use the default registration UI or point users to a host app-owned flow.
- As a user, I can access a working default registration UI when the host app has not provided a custom one.
- As a developer, I can configure secure links via env or config without writing custom token logic.
- As a developer, I can use the same manager across invite, reset, and registration.
- As a developer, I can override the default registration routes/templates with host app UI.
- As a developer, I can wire onboarding routes via quickstart helpers instead of copying handlers.
- As a user, I see password strength requirements in real time during reset/signup.
- As support, I can see remaining time on a pending invite in the admin UI.
- As security, I can view a log of all authentication events in the admin activity feed.

## Acceptance Criteria

- go-admin onboarding flows use securelink tokens and no longer scan user metadata.
- Token usage is validated and marked used server-side.
- quickstart provides a single place to configure securelink defaults.
- Docs explain configuration and the token lifecycle.
- Admin activity feed shows onboarding/auth events from both go-users and go-auth.
- Default auth UI respects feature flags and surfaces password policy/rate-limit feedback.
- Admin UI exposes invite expiration/remaining time where invites are listed.
- Auth/onboarding API errors return go-errors `ErrorResponse` with `text_code` mapped for UI messaging.
