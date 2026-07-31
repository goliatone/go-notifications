# LOGIN Workflow — Implementation Plan

Roadmap aligned with `LOGIN_TDD.md`, read that document first.

## Guiding Notes

- Source of truth: `LOGIN_TDD.md`.
- Feature key is `users.signup` (rename from `users.self_registration`).
- Securelink managers must remain transport-agnostic.
- Do not log or persist raw tokens; store `jti` + lifecycle metadata.
- Option A chosen: add go-users migrations to persist securelink lifecycle fields.
- To run go commands use full path to binary `/Users/goliatone/.g/go/bin/go`.

## Phase 0. Planning & Review
**Why**: Confirm scope, preserve existing behavior, and align on shared interfaces.

**Tasks**
- [x] Task 0.1 – Read `LOGIN_TDD.md` and confirm requirements + ownership split.
- [x] Task 0.2 – Inventory current onboarding flows in `examples/web/handlers/onboarding.go` and note behaviors to preserve.
- [x] Task 0.3 – Confirm feature flag keys in quickstart and templates; align to `users.signup`.
- [x] Task 0.4 – Review go-users `password_reset` schema and go-auth reset flow for migration impact.
- [x] Task 0.5 – Verify go-notifications securelink interfaces match the proposed signatures.

**Notes**
- Task 0.1: Confirmed scope/ownership per `LOGIN_TDD.md`: go-users owns invite/signup/reset issuance + validation; go-auth stays auth primitives; go-notifications consumes securelink only; go-admin/quickstart wires.
- Task 0.2: Onboarding handlers use UUID tokens stored in user metadata; reset requests are rate-limited to 5m and expire in 1h; token validation scans users; accept invite resets password, transitions lifecycle, and marks used tokens in metadata.
- Task 0.3: Quickstart auth UI now checks `users.signup`; login template still keys off `self_registration_enabled`; example feature flag constant updated to `users.signup`.
- Task 0.4: go-users `password_reset` only stores `id/user_id/email/status/reseted_at/created_at` today; go-auth reset uses `password_reset` ID as token + created_at-based expiry (24h) with status gating, so migration needs new lifecycle fields and securelink issuance.
- Task 0.5: go-notifications `pkg/links` SecureLinkManager/Configurator signatures match the proposed `Generate`, `Validate`, `GetAndValidate`, `GetExpiration` shapes (payload is `map[string]any`).

**Acceptance Criteria**
- Requirements confirmed; current behavior documented; naming aligned.

## Phase 1. go-users (Securelink + Token Storage)
**Why**: go-users owns invite/signup/reset issuance, validation, and replay protection.

**Tasks**
- [x] Task 1.1 – Add `SecureLinkManager`/`SecureLinkConfigurator` interfaces in `pkg/types` (mirror go-notifications).
- [x] Task 1.2 – Add securelink adapter (go-urlkit default implementation).
- [x] Task 1.3 – Add `user_token` table + repository for invite/registration tokens.
- [x] Task 1.4 – Add migration to extend `password_reset` with:
  - `jti`, `expires_at`, `used_at`, optional `issued_at`
  - optional `scope_tenant_id`, `scope_org_id`
  - unique index on `jti` + expiry/status index for cleanup
- [x] Task 1.5 – Update `UserInviteCommand` to issue securelink tokens and persist `jti`.
- [x] Task 1.6 – Add `UserRegistrationRequestCommand` (or extend existing create flow) to issue signup tokens.
- [x] Task 1.7 – Add reset issuance + validation commands that:
  - persist lifecycle fields in `password_reset`
  - validate/consume tokens before applying `UserPasswordReset`
- [x] Task 1.8 – Add validate/consume commands for invite/registration tokens with optional `ScopeEnforcer`.
- [x] Task 1.9 – Emit activity with `jti`/`expires_at` only (no raw tokens).
- [x] Task 1.10 – Update service wiring to accept securelink manager + token stores.
- [x] Task 1.11 – Unit tests for payload composition, replay protection, and activity logging.

**Acceptance Criteria**
- Tokens are securelink-based; replay protection persisted; activity logs avoid raw tokens.

## Phase 2. go-auth (Optional Migration + Audit Bridge)
**Why**: Align auth primitives and activity with the new onboarding workflows.

**Tasks**
- [ ] Task 2.1 – Deprecate go-auth reset handlers/table once go-users is authoritative.
- [ ] Task 2.2 – Add adapter to bridge go-auth activity into go-users `user_activity`.
- [ ] Task 2.3 – Standardize rate-limit/lockout/verification error `TextCode`s via go-errors.
- [ ] Task 2.4 – Add lockout configuration interface + emit lockout events.
- [ ] Task 2.5 – Add verification gating + resend metadata.
- [ ] Task 2.6 – Add session registry APIs (list/revoke/force logout).

**Acceptance Criteria**
- go-auth can be retired for reset flow; auth activity appears in the shared feed.

## Phase 3. go-errors (Auth/Onboarding Mapping)
**Why**: Ensure API errors map consistently for UI messaging.

**Tasks**
- [x] Task 3.1 – Add auth/onboarding error mappers (rate limit, lockout, verification, invite/reset states).
- [x] Task 3.2 – Document canonical `TextCode` list and keep in sync with go-auth/go-users.
- [x] Task 3.3 – Tests for mapper coverage and code mapping behavior.

**Acceptance Criteria**
- `goerrors.MapToError` returns stable `text_code`s for all onboarding/auth scenarios.

## Phase 4. go-notifications (Payload Contracts)
**Why**: Keep notifications link-agnostic but payload-consistent.

**Tasks**
- [x] Task 4.1 – Align securelink interface signatures with go-users/go-auth.
- [x] Task 4.2 – Define payload contracts for lockout/verification/invite/reset expiry data.
- [x] Task 4.3 – Update templates/examples/tests to reflect new payload fields.

**Acceptance Criteria**
- Notifications can build links from securelink data without owning token logic.

## Phase 5. go-admin (Example App + UI)
**Why**: Provide a functional default workflow and UI surface.

**Tasks**
- [ ] Task 5.1 – Replace metadata-based tokens in `examples/web/handlers/onboarding.go` with securelink manager + go-users commands.
- [ ] Task 5.2 – Stop scanning user metadata for tokens; use token registry lookups.
- [ ] Task 5.3 – Add registration UI route + template with host override hooks.
- [ ] Task 5.4 – Add token metadata endpoint (expires/used) and surface in UI.
- [ ] Task 5.5 – Add password policy hints and link expiration feedback to UI.
- [ ] Task 5.6 – Map auth/onboarding errors to UI messages via `text_code`.
- [ ] Task 5.7 – Update OnboardingNotifier to avoid raw tokens in notifications.

**Acceptance Criteria**
- Example app supports invite/reset/signup workflows end-to-end with securelink tokens.

## Phase 6. quickstart (Wiring + Feature Gates)
**Why**: Provide turnkey wiring for go-admin host apps.

**Tasks**
- [ ] Task 6.1 – Add securelink manager builder + env config:
  - `ADMIN_SECURELINK_KEY`, `ADMIN_SECURELINK_BASE_URL`, `ADMIN_SECURELINK_QUERY_KEY`,
    `ADMIN_SECURELINK_AS_QUERY`, `ADMIN_SECURELINK_EXPIRATION`
- [ ] Task 6.2 – Add `quickstart.NewSecureLinkManager(cfg)` helper.
- [ ] Task 6.3 – Inject manager into go-users/go-auth/go-notifications via options.
- [ ] Task 6.4 – Add onboarding route registration helpers.
- [ ] Task 6.5 – Add view context helpers for password reset + signup (`users.signup`).
- [ ] Task 6.6 – Add registration UI wiring + override hooks.
- [ ] Task 6.7 – Add error mapper injection option for `NewFiberErrorHandler`.
- [ ] Task 6.8 – Wire shared activity sink for go-users + go-auth.

**Acceptance Criteria**
- Host apps can enable onboarding flows with minimal configuration.

## Phase 7. Tests (go-admin/quickstart)
**Why**: Validate end-to-end behavior before production rollout.

**Tasks**
- [ ] Task 7.1 – Integration tests: invite + accept, password reset request + confirm.
- [ ] Task 7.2 – Signup enabled/disabled toggle tests (`users.signup`).
- [ ] Task 7.3 – Activity feed integration tests (auth + onboarding events).
- [ ] Task 7.4 – UI feedback tests for password policy + rate limits + token expiry.

**Acceptance Criteria**
- End-to-end flows pass; UI and activity auditing behave as expected.

## Phase 8. Docs & Examples
**Why**: Ensure consumers can configure and operate the workflow.

**Tasks**
- [ ] Task 8.1 – Document securelink config, routes, and feature flags.
- [ ] Task 8.2 – Document token lifecycle + replay protection storage fields.
- [ ] Task 8.3 – Document error response shape + `text_code` mappings.
- [ ] Task 8.4 – Add usage examples for default UI + override hooks.

**Acceptance Criteria**
- Documentation reflects the new workflow, configuration, and troubleshooting paths.
