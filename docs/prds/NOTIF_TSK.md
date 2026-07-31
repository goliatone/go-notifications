# go-notifications OnReady Integration Implementation Plan

Roadmap aligned with `NOTIF_TDD.md`, following the phased structure used in other modules.

## Phase 0. Planning & Scaffolding
**Why**: Confirm scope, dependencies, and ownership.

**Tasks**
- [x] Task 0.1 – Cross-reference `NOTIF_TDD.md` with `EXPORT_TDD.md` and related docs; document ownership split in `docs/prds/NOTIF_OVERVIEW.md`.
- [x] Task 0.2 – Create umbrella issues for export-ready definition/template, registration helper, sender interface, channel metadata, manifest support, docs/examples.
- [x] Task 0.3 – Add CI/test placeholders for new assets/helpers.

**Acceptance Criteria**
- Ownership and scope documented; tracking issues opened.
- CI placeholders ready.

## Phase 1. Definition & Templates
**Why**: Provide ready/completion defaults.

**Tasks**
- [x] Task 1.1 – Add export-ready definition/template (code e.g., `export.ready`) with placeholders (`FileName`, `Format`, `URL`, `ExpiresAt`, optional `Rows`, `Parts`, `ManifestURL`, `Message`).
- [x] Task 1.2 – Provide default email and in-app template variants with localization keys.
- [x] Task 1.3 – Tests for rendering with sample payload (email/in-app).

**Acceptance Criteria**
- Definition/template added and render correctly; tests pass; localization keys and preference checks respected.

## Phase 2. Registration Helper
**Why**: Make onboarding easy and idempotent.

**Tasks**
- [x] Task 2.1 – Implement helper to register the export definition/template at startup (idempotent), accepting overrides (strings, CTA labels, icons).
- [x] Task 2.2 – Return definition code/IDs for callers; namespaced option.
- [x] Task 2.3 – Tests for registration (no duplication, override behavior).

**Acceptance Criteria**
- Registration helper works idempotently; tests cover overrides.

## Phase 3. Sender Interface
**Why**: Clean DI surface for exporters.

**Tasks**
- [x] Task 3.1 – Define `OnReadyNotifier` interface and `OnReadyEvent` payload (recipients, locale, fields, channel overrides).
- [x] Task 3.2 – Default implementation wrapping manager/commands; respects preferences/rules; no internal imports required.
- [x] Task 3.3 – Tests for send behavior (email/in-app) with sample payloads.

**Acceptance Criteria**
- Sender interface usable via DI; default implementation works; tests green.

## Phase 4. Channel Metadata & Manifest Support
**Why**: Support multi-channel and multipart exports.

**Tasks**
- [x] Task 4.1 – Ensure template supports per-channel metadata (email subject/body/CTA; in-app title/body/icon/badge).
- [x] Task 4.2 – Handle optional `ManifestURL`/`Parts` fields for multipart exports.
- [x] Task 4.3 – Tests for channel overrides and manifest inclusion.

**Acceptance Criteria**
- Channel metadata and manifest fields render correctly; tests pass.

## Phase 5. Docs & Examples
**Why**: Guide integrators.

**Tasks**
- [x] Task 5.1 – Add documentation on registering the ready definition/template and sending via `OnReadyNotifier`; integration note for go-export.
- [x] Task 5.2 – Add minimal example demonstrating registration + send with sample payload.
- [x] Task 5.3 – Release notes and migration guidance (opt-in).

**Acceptance Criteria**
- Docs/examples published; release/migration notes ready.

## Phase 6. Attachment Support
**Why**: Enable provider-specific file/media delivery with an injectable upload service.

**Tasks**
- [x] Task 6.1 – Define `Attachment` model and event payload fields (`Attachments`, optional `ChannelAttachments`), plus normalization helpers.
- [x] Task 6.2 – Add an attachment resolver/uploader interface; inject via `dispatcher.Dependencies`.
- [x] Task 6.3 – Wire attachment resolution into dispatcher delivery flow; sanitize logging/metrics payloads.
- [x] Task 6.4 – Implement email attachments (SMTP + at least one API provider) using `Message.Attachments`; keep metadata compatibility.
- [x] Task 6.5 – Implement URL-only/provider-specific attachments (Slack, SMS/MMS, WhatsApp, Telegram) using URLs or upload APIs.
- [x] Task 6.6 – Tests: attachment passthrough, resolver upload behavior, provider payload assertions.
- [x] Task 6.7 – Docs: update OnReady guide and adapter READMEs to document attachment support and limitations.

**Acceptance Criteria**
- Attachments flow end-to-end for at least one email adapter and one URL-only provider.
- Channels that do not support attachments remain unchanged.
- Upload service can be swapped without changing adapters or callers.
