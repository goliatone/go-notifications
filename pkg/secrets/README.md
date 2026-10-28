# Notification adapter credentials

`pkg/secrets` defines scoped `Reference`/`SecretValue`, providers, registry and
resolver contracts for notification adapter credentials. It supplies static
providers, a memory Store, XChaCha20-Poly1305 `EncryptedStoreProvider`, a
`SimpleResolver`, TTL `CachingResolver` and masking helpers. Encrypted providers
require a 32-byte key and a host-supplied `pkg/interfaces/secrets.Store`.

The dispatcher resolves per-recipient credential references before adapter send.
Global environment/config fallback is controlled by the configured subject/tenant
allowlist. Hosts own provider policy, key lifecycle and persistent secret tables;
module notification migrations do not create a general secret-manager service.
External Vault/AWS/GCP integrations implement these provider ports.

See [the maintained secrets guide](../../docs/GUIDE_SECRETS.md) for reference
shapes, fallback ordering, encryption, caching and logging policy. This package's
credential interfaces are distinct from the standalone go-secrets package;
integrate the two through an explicit host adapter when needed.
