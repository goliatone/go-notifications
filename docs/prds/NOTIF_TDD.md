# go-notifications – Export Integration TDD

This TDD captures improvements to go-notifications to smooth integration with go-export (and similar modules) by providing a reusable “export ready” notification flow and a clean adapter surface.

## Background and Goals
- **Current state**: go-notifications provides definitions/templates, rendering, dispatch, preferences, commands, and adapters. Exporting modules can call the manager/commands but must define their own export-ready templates/metadata.
- **Desired state**: A first-class, opt-in “export ready” notification package (definition + template + command helper) plus a minimal interface for DI so exporters can send notifications without bespoke setup. Support per-channel metadata and expiry/link fields.
- **Objectives**:
  1. Provide a reusable export-ready notification definition/template with standard fields (link, filename, format, expires, row/part counts).
  2. Offer a simple helper/command to send export-ready events via DI without touching internals.
  3. Allow channel-specific metadata (email subject/body/CTA, in-app title/body, optional manifest link).
  4. Keep integration adapter-first; no hard coupling to go-export.

## Scope
### In Scope
- Standard export-ready definition/template scaffolding (optional/opt-in).
- Helper to register the definition/template at startup (DI-friendly).
- Interface for sending export-ready notifications (wrapping manager/commands).
- Per-channel metadata support for the export template (email/in-app).
- Preference/localization alignment with existing go-notifications capabilities.

### Out of Scope
- New channels beyond what go-notifications already supports.
- Overhauling template engine; reuse existing go-template/go-i18n pipeline.

## Requirements & Constraints
- Keep core adapter-first; no direct dependency on go-export.
- Definition/template registration should be optional and non-breaking.
- Fields should cover: `FileName`, `Format`, `URL`, `ExpiresAt`, `Rows` (optional), `Parts` (optional), `ManifestURL` (optional), `Message` (optional).
- Channel metadata: email subject/CTA text/URL; in-app title/body; optional icon/badge.
- Localization: allow templates to localize strings; reuse existing i18n pipeline.
- Preferences: respect recipient preferences and rules already provided by go-notifications.

## Proposed Changes
1. **Export-Ready Definition & Template**
   - Provide a prebuilt definition/template (YAML/Go helper) for “export ready” events.
   - Include default localization keys/placeholders; allow overrides.
2. **Registration Helper**
   - Add a helper to register the export definition/template during module init (optionally namespaced), returning the definition code for callers.
3. **Sender Interface/Adapter**
   - Define a small interface (e.g., `OnReadyNotifier`) with a `Send(ctx, OnReadyEvent)` method; default implementation wraps the existing manager/commands.
   - Expose in a package that does not require importing `internal/` packages.
4. **Channel Metadata**
   - Ensure the template supports per-channel metadata fields: email subject/body/cta, in-app title/body/icon/badge; map placeholders accordingly.
5. **Manifest Support**
   - Include optional `ManifestURL`/`Parts` fields so multipart exports can present multiple links or a manifest CTA.
6. **Docs & Examples**
   - Document how to register the export definition/template and send events via DI; include a minimal example.

## Detailed Design
### ExportReady Definition/Template
- Definition code: e.g., `export.ready`.
- Template placeholders: `FileName`, `Format`, `URL`, `ExpiresAt`, `Rows` (optional), `Parts` (optional), `ManifestURL` (optional), `Message` (optional).
- Provide default email/in-app variants using existing template engine; allow localization via go-i18n keys.

### Registration Helper
- Function to install the definition/template into the manager/registry if not present; accept overrides (strings, CTA labels, icons).
- Return definition code/IDs for later send calls.

### Sender Interface
- Define `type OnReadyNotifier interface { Send(ctx context.Context, evt OnReadyEvent) error }`.
- `OnReadyEvent` carries recipient(s), locale, fields above, and optional channel overrides.
- Default implementation delegates to manager/commands; respects preferences/rules.

### Channel Metadata
- Map placeholders to email subject/body/CTA and in-app title/body/icon/badge.
- Allow channel-specific overrides in the event payload.

### Manifest Support
- If `ManifestURL` present, templates include a CTA to the manifest; if `Parts` present, optionally list counts.

## Attachment Support for Providers
### Context
- Many providers treat "attachments" differently: email supports raw bytes; SMS/MMS and WhatsApp require media URLs; Slack/Telegram support upload APIs.
- We need a consistent, minimal API for callers while allowing provider-specific handling and optional uploads.
- Attachment payloads can be large or sensitive; avoid persisting raw bytes in `notification_events` unless explicitly required.

### Goals
- Provide a single attachment shape for callers (`Attachment`) with optional per-channel overrides.
- Allow adapters to consume attachments when supported and ignore them when not.
- Support URL-only channels by optionally uploading bytes via an injected uploader service.
- Keep backward compatibility with metadata-based attachments in existing adapters.

### Attachment Model
- `Attachment` fields:
  - `Filename` (required)
  - `ContentType` (optional, defaults to `application/octet-stream`)
  - `Content []byte` (optional, for email or upload-capable providers)
  - `Size` (optional, derived when missing)
  - `URL` (optional, for URL-only providers like SMS/MMS)
- Event payload:
  - `Attachments []Attachment` (default)
  - `ChannelAttachments map[string][]Attachment` (per-channel override; optional)

### Upload Service Injection
- Introduce an attachment resolver that can be injected into the dispatcher:
  - If attachments include `Content` but the channel requires `URL`, use the injected uploader to store the bytes and return URLs.
  - If no uploader is configured, either drop unsupported attachments or return a clear error (policy-driven).
- Suggested interface:
  - `Uploader.Upload(ctx, UploadRequest) (UploadedAttachment, error)`
  - `Resolver.Resolve(ctx, job, attachments) ([]Attachment, error)`
- Wire the resolver through `dispatcher.Dependencies` so it can be swapped in per module/runtime.

### Provider Mapping (Behavior)
- **Email (SMTP, SES, SendGrid, Mailgun)**: use `Attachment.Content` to build multipart messages; ignore `URL` unless the provider supports it.
- **Slack**: upload bytes (`files.upload`) or attach URLs (`files.remote.add`); include message text in the main payload.
- **SMS/MMS (Twilio)**: only accept `URL` (media URLs).
- **WhatsApp/Telegram**: accept URLs or multipart uploads; prefer URLs for simplicity.
- **In-app/Push**: ignore attachments; include action URLs in metadata if needed.

### Data & Persistence
- Prefer pre-uploading attachments before persisting events, storing only `URL` + metadata in event context.
- If raw `Content` must be passed at runtime, ensure it is scrubbed from activity logs and/or metrics.

### Compatibility Notes
- Preserve existing adapter metadata keys (e.g., Mailgun `metadata["attachments"]`) by normalizing them into `Attachment` at dispatch time.
- Attachments are additive; channels without support should continue to behave unchanged.

## Implementation Phases & Acceptance Criteria
1. **Definition & Template**: Add export-ready definition/template (email + in-app) with placeholders/localization keys. Tests for rendering with sample data.
2. **Registration Helper**: Public helper to register export-ready assets; idempotent; tests ensure no duplication.
3. **Sender Interface**: Add `OnReadyNotifier` and default implementation wrapping manager/commands; tests cover send with preferences/i18n.
4. **Channel Metadata**: Ensure template supports per-channel overrides; tests for email/in-app render.
5. **Docs/Example**: Add doc/example showing registration + send; integration note for go-export.

## Risks & Mitigations
- **Template collisions**: namespaced codes and idempotent registration.
- **Channel mismatches**: clearly document required fields per channel; defaults provided.
- **Localization gaps**: ship default keys; allow overrides.

## Open Questions
- Default definition code/name? (`export.ready` vs. namespaced)
- Include SMS/push variants out of the box, or email+in-app only?
- Should registration helper accept a custom template FS, or rely on built-ins only?
