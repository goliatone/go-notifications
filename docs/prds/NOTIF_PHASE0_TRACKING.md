# OnReady Notifications – Phase 0 Tracking

Umbrella issue drafts for the OnReady notification work described in `NOTIF_TSK.md` and `NOTIF_TDD.md`. These are ready to file in go-notifications (and cross-link from go-export/go-admin adapters) once implementation starts.

| ID | Title | Owner | Status | Notes |
| -- | ----- | ----- | ------ | ----- |
| NTF-EXP-01 | Export-ready definition & templates | go-notifications/core | Open | Ship `export.ready` definition plus email/in-app templates with required placeholders and localization keys. |
| NTF-EXP-02 | Registration helper (idempotent) | go-notifications/core | Open | Public helper to install definition/templates with optional namespace/overrides, returning code/IDs. |
| NTF-EXP-03 | OnReadyNotifier interface & default impl | go-notifications/core | Open | DI-friendly `OnReadyNotifier` and `OnReadyEvent` that wrap manager/commands and respect preferences. |
| NTF-EXP-04 | Channel metadata & overrides | go-notifications/core | Open | Ensure per-channel metadata (email subject/CTA, in-app title/body/icon/badge) and overrides flow through templates. |
| NTF-EXP-05 | Manifest/multipart support | go-notifications/core | Open | Handle optional `ManifestURL`/`Parts` fields for multipart exports; render manifest CTA when present. |
| NTF-EXP-06 | Docs/examples & release notes | docs/examples | Open | Author registration + send docs, minimal example, and opt-in release notes; include go-export integration note. |

## Issue Drafts

- **NTF-EXP-01 – Ready definition & templates**
  - Scope: Add `export.ready` definition with placeholders (`file_name`, `format`, `url`, `expires_at`, optional `rows`, `parts`, `manifest_url`, `message`) and default email/in-app variants with localization keys.
  - Deliverables: Definition artifacts (YAML/Go helper), template strings keyed by locale, placeholder validation tests, and docs linkback to `NOTIF_TDD.md`.

- **NTF-EXP-02 – Registration helper (idempotent)**
  - Scope: Helper that installs the definition/template at startup (namespaced option) and accepts override strings/labels/icons without duplicating existing assets.
  - Deliverables: Public API returning definition code/IDs, idempotency tests, override-behavior tests, and wiring note for hosts/go-export adapters.

- **NTF-EXP-03 – OnReadyNotifier interface & default impl**
  - Scope: Define `OnReadyNotifier` + `OnReadyEvent` payload (recipients, locale, fields, channel overrides) and a default implementation that delegates to commands/manager while enforcing preferences/rules.
  - Deliverables: Interface in a stable package (no internal imports), helper constructor, send-path tests for email/in-app, and DI usage snippet for go-export.

- **NTF-EXP-04 – Channel metadata & overrides**
  - Scope: Ensure templates support per-channel metadata (email subject/body/CTA; in-app title/body/icon/badge) and allow override payloads to flow through rendering.
  - Deliverables: Template updates, override handling in renderer/commands, and tests asserting per-channel metadata is honored.

- **NTF-EXP-05 – Manifest/multipart support**
  - Scope: Support optional `ManifestURL` and `Parts` fields so multipart exports can render manifest CTA plus part counts when provided.
  - Deliverables: Template branches for manifest vs single file, tests covering multipart payloads, and compatibility note for go-export manifest generation.

- **NTF-EXP-06 – Docs/examples & release notes**
  - Scope: Document registration helper + `OnReadyNotifier` usage, add a minimal send example, and include release/migration guidance (opt-in).
  - Deliverables: Docs pages, example code wired into samples/tests, and release-note entry summarizing new assets and defaults.
