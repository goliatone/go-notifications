# Link Generation Implementation Plan

Roadmap aligned with `docs/LINKS_TDD.md` to deliver secure, per-channel link generation with optional persistence and securelink-compatible interfaces while keeping the core module dependency-free.

## Guiding Notes
- Follow the contracts and flow in `docs/LINKS_TDD.md` (per-channel builder, `ResolvedLinks`, optional store + observer).
- Do not import `../go-urlkit/securelink` in core packages; define local interfaces compatible with it.
- Keep defaults no-op: if no builder/store/observer is configured, behavior should remain unchanged.
- Update docs and tests alongside code changes.
- Decision defaults for this plan:
  - Failure semantics: configurable per dependency (builder/store/observer strict vs lenient).
  - Invocation order: LinkStore/Observer before persistence/delivery (gating-first).
  - ResolvedLinks merge: explicit precedence table (builder > overrides > original).
  - `url` vs `action_url`: deprecate `url` in favor of `action_url` (track in `CHANGELOG.md`).
  - Template helper: add `secure_link(...)` helper that delegates to builder flow.
  - ResolvedURLs contract: fixed keys (`url`, `action_url`, `manifest_url`) + namespaced extras (e.g. `meta.token`).
  - LinkStore records: dispatcher constructs records when missing.
  - Metadata vs message entity storage: store link fields on the message entity only (breaking change; track in migration doc).

---

## Phase 0. Design & Contracts
**Why**: Establish interfaces and baseline behavior before wiring into dispatcher.

**Tasks**
- [x] Task 0.1 – Add `pkg/links` package with `LinkBuilder`, `LinkRequest`, `ResolvedLinks`, `LinkRecord`, and optional `LinkStore`/`LinkObserver` interfaces.
- [x] Task 0.2 – Add securelink-compatible interfaces (manager/configurator/payload) in `pkg/links` so host apps can plug `securelink` implementations without core dependency.
- [x] Task 0.3 – Document default behavior when no builder is configured (pass-through link fields).
- [x] Task 0.4 – Define failure semantics configuration (strict/lenient per builder/store/observer).
- [x] Task 0.5 – Define `ResolvedURLs` keyset and namespaced extras (e.g. `meta.*`).

---

## Phase 1. Dispatcher Integration (Per-Channel)
**Why**: Ensure link resolution occurs after channel overrides and before template rendering.

**Tasks**
- [x] Task 1.1 – Extend dispatcher dependencies/options to accept a `LinkBuilder`, `LinkStore`, and `LinkObserver`.
- [x] Task 1.2 – Invoke link builder per channel after `applyChannelOverrides` and before template rendering.
- [x] Task 1.3 – Apply `ResolvedLinks` to both payload (template context) and message metadata.
- [x] Task 1.4 – Invoke `LinkStore.Save` and/or `LinkObserver.OnLinksResolved` when configured.
- [x] Task 1.5 – Ensure inbox delivery uses resolved `action_url` from message metadata.
- [x] Task 1.6 – Ensure LinkStore/Observer are executed before persistence/delivery (gating-first).

---

## Phase 2. Data Flow & Compatibility
**Why**: Maintain consistency between template context, metadata, and channel overrides.

**Tasks**
- [x] Task 2.1 – Define and implement explicit merge precedence (builder > overrides > original).
- [x] Task 2.2 – Deprecate `url` in favor of `action_url` (doc + code + `CHANGELOG.md` entry).
- [x] Task 2.3 – Normalize link fields to avoid duplication across payload/metadata (incl. `manifest_url`).
- [x] Task 2.4 – Dispatcher constructs `LinkRecord` entries when builder does not provide them.
- [x] Task 2.5 – Store link fields on the message entity only (remove metadata writes).
- [x] Task 2.6 – Create `docs/LINK_MIGRATION.md` with breaking changes and migration paths introduced in this phase.
- [x] Task 2.7 – Update `docs/LINK_MIGRATION.md` + `CHANGELOG.md` with `url` deprecation and metadata storage change details.

---

## Phase 3. Optional Storage & Observability
**Why**: Enable one-time/gated links with external persistence and analytics hooks.

**Tasks**
- [x] Task 3.1 – Implement no-op defaults for `LinkStore` and `LinkObserver`.
- [x] Task 3.2 – Provide example adapter (outside core) demonstrating securelink + store usage.
- [x] Task 3.3 – Document recommended storage patterns and lifecycle hooks.

---

## Phase 4. Tests
**Why**: Validate behavior with and without builders/stores.

**Tasks**
- [x] Task 4.1 – Unit tests for `ResolvedLinks` application to payload + metadata.
- [x] Task 4.2 – Dispatcher tests verifying per-channel invocation and override ordering.
- [x] Task 4.3 – Tests for LinkStore/Observer invocation and error handling.
- [x] Task 4.4 – Reconcile `docs/LINK_MIGRATION.md` with actual behaviors validated by tests.

---

## Phase 5. Documentation & Examples
**Why**: Ensure feature is discoverable and correctly used.

**Tasks**
- [x] Task 5.1 – Update `docs/GUIDE_ADAPTERS.md` or a new guide to mention secure link workflow.
- [x] Task 5.2 – Add usage snippet to `docs/LINKS_TDD.md` showing securelink integration.
- [x] Task 5.3 – Add release notes entry with the new link builder feature and integration points.
- [x] Task 5.4 – Document `secure_link(...)` template helper and its builder delegation rules.
- [x] Task 5.5 – Final review/update of `docs/LINK_MIGRATION.md` before release notes.

---

Completion of all phases yields a production-ready, securelink-compatible link pipeline with optional persistence and no hard dependency on external packages.
