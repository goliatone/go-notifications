# pkg/secrets

Interfaces and helpers for pluggable secret resolution as defined in `docs/SEC_TDD.md`.

- `Reference`/`SecretValue` describe scoped secrets (system/tenant/user → channel/provider/key).
- `Provider`/`Resolver` abstract secret backends (static, encrypted store, external managers).
- Validation helpers ensure scopes/keys/subjects are well formed.
- Masking helpers wrap `github.com/goliatone/go-masker` to safely log secret metadata without leaking payloads.

Default behavior:
- Env/config remain valid fallbacks when a resolver/provider is not configured.
- No secrets are logged; call `MaskValues` for diagnostics.

## External secret managers

Providers for Vault / AWS Secrets Manager / GCP Secret Manager map a `Reference` into a path/ARN and hand back the raw bytes (no local encryption):

- **Vault (KV v2)**: `secret/data/ntf/{scope}/{subject}/{channel}/{provider}/{key}` → store secret payload under `data.value`.
- **AWS SM**: `arn:aws:secretsmanager:{region}:{account}:secret:ntf/{scope}/{subject}/{channel}/{provider}/{key}`; keep the version in the SM version stage if you need explicit lookups.
- **GCP SM**: `projects/{project}/secrets/ntf-{scope}-{channel}-{provider}-{key}-{subject}/versions/latest`.

Each provider should normalize scope/subject IDs (lowercase, no spaces) and preserve `Reference.Version` when the manager supports it. Mask logs with `MaskValues` even when the manager returns already-encrypted material.

Future work (see `docs/SEC_TSK.md`):
- Encrypted store provider + Bun repository.
- External providers (Vault/AWS SM/GCP SM).
- Dispatcher/adapter wiring to inject per-recipient secrets at send time.
