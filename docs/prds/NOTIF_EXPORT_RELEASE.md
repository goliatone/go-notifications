# OnReady Notifications – Release Notes (Opt-in)

## What’s new
- Added a reusable `export.ready` definition with default email/in-app templates and localization keys.
- Idempotent registration helper (`onready.Register`) with namespace and CTA/icon overrides.
- DI-friendly `OnReadyNotifier` for sending completion/ready events without importing internal packages.
- Channel metadata support (CTA labels/icons/badge) and manifest/multipart fields (`manifest_url`, `parts`).
- Example bootstrap + send flow under `examples/onready`.

## Compatibility
- Opt-in; no existing definitions/templates are changed unless you register the assets.
- Default definition code: `export.ready` (namespace optional).
- Channels covered: email + in-app. Other channels remain unchanged.

## Migration
1. Call `onready.Register` during service startup to install assets (optionally namespaced).
2. Wire `OnReadyNotifier` via DI and send `OnReadyEvent` payloads when exports complete.
3. If you need CDN/signed links, pass `ChannelOverrides` (`action_url`, `cta_label`, `icon`, `badge`) per channel.
4. Ensure the templates’ placeholders are populated: `file_name`, `format`, `url`, `expires_at`; optional `rows`, `parts`, `manifest_url`, `message`.

## Testing notes
- Run `go test ./pkg/onready` once a Go 1.24+ toolchain is available (network-restricted environments may need an offline toolchain).
