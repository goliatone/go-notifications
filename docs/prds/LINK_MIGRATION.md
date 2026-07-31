# Link Generation Migration Guide

This document tracks breaking changes introduced by the link generation refactor and how to migrate dependent repos. Update this file as tasks land so consumers can follow a clear path.

---

## Status Checklist (Update As Tasks Progress)

- [x] Confirm final merge precedence and helper behavior match implementation.
- [x] Verify storage behavior (message entity vs metadata) is live in dispatcher.
- [x] Validate deprecation behavior for `url` vs `action_url`.
- [x] Update examples/snippets to align with new fields and helpers.

---

## Breaking Changes

### 1) Link fields stored on the message entity only

**What changed**
Link fields (`action_url`, `manifest_url`, deprecated `url`) are stored on the message entity as the source of truth. Message metadata no longer carries these link fields; metadata only contains `ResolvedLinks.Metadata`. These fields now live directly on `NotificationMessage`.

**Impact**
Templates, adapters, or downstream code that read link fields from `message.Metadata` will no longer find them.

**Migration path**
- Update code to read link fields from the message entity.
- If you must keep metadata access (e.g., legacy templates), add a compatibility shim in the consuming app or update templates to use the new fields or `secure_link(...)`.

---

### 2) `url` deprecated in favor of `action_url`

**What changed**
`action_url` is now the canonical CTA link. `url` is deprecated and will be removed after the migration window. When only `url` is provided, the dispatcher mirrors it into `action_url` during resolution.

**Impact**
Templates or code that rely on `url` may render empty links once deprecation is enforced.

**Migration path**
- Update templates and adapters to prefer `action_url`.
- If you still pass `url`, ensure it is mirrored to `action_url` during the transition.

---

## Notes for This Repo

You are the only consumer right now, so updates can be applied directly in dependent repos as the refactor lands. Keep this file in sync with any additional breaking changes introduced by Phase 2 tasks.

Templates should read `action_url` directly or via `secure_link(...)`; the helper reads resolved payload/message fields and does not trigger link generation (registered by default in the templates service).

## Validated Behavior (Phase 4 Tests)

- Link fields are applied to payload + message entity; message metadata only receives `ResolvedLinks.Metadata`.
- Channel overrides are applied before builder execution; builder output remains highest precedence.
- `LinkStore` runs before message persistence/delivery; strict store failures abort delivery.
