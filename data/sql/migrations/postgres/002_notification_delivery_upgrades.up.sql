ALTER TABLE notification_events ADD COLUMN IF NOT EXISTS correlation_id TEXT;
ALTER TABLE notification_events ADD COLUMN IF NOT EXISTS request_id TEXT;
ALTER TABLE notification_events ADD COLUMN IF NOT EXISTS idempotency_scope TEXT;
ALTER TABLE notification_events ADD COLUMN IF NOT EXISTS idempotency_key TEXT;
ALTER TABLE notification_events ADD COLUMN IF NOT EXISTS request_fingerprint TEXT;
ALTER TABLE notification_events ADD COLUMN IF NOT EXISTS transient_dependent BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE notification_events ADD COLUMN IF NOT EXISTS publication_id UUID;
ALTER TABLE notification_events ADD COLUMN IF NOT EXISTS digest_key TEXT;
ALTER TABLE notification_events ADD COLUMN IF NOT EXISTS retry_claim_until TIMESTAMPTZ;
ALTER TABLE notification_delivery_attempts ADD COLUMN IF NOT EXISTS error_code TEXT;
ALTER TABLE notification_messages ADD COLUMN IF NOT EXISTS retry_operation_id UUID;
ALTER TABLE notification_delivery_attempts ADD COLUMN IF NOT EXISTS retry_operation_id UUID;

CREATE UNIQUE INDEX IF NOT EXISTS notification_events_idempotency_uidx
  ON notification_events (idempotency_scope, definition_code, idempotency_key)
  WHERE idempotency_key IS NOT NULL AND idempotency_key <> '' AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS notification_events_publication_idx ON notification_events (publication_id);

CREATE TABLE IF NOT EXISTS notification_publications (
  id UUID PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMPTZ,
  kind TEXT NOT NULL, digest_key TEXT, queue_key TEXT NOT NULL UNIQUE, run_at TIMESTAMPTZ,
  status TEXT NOT NULL, claim_until TIMESTAMPTZ, attempts INTEGER, error_code TEXT
);
CREATE INDEX IF NOT EXISTS notification_publications_pending_idx
  ON notification_publications (status, run_at);
CREATE INDEX IF NOT EXISTS notification_publications_digest_idx
  ON notification_publications (digest_key, status);

CREATE TABLE IF NOT EXISTS notification_retry_operations (
  id UUID PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMPTZ,
  event_id UUID NOT NULL, retry_scope TEXT NOT NULL, idempotency_key TEXT NOT NULL,
  correlation_id TEXT, request_id TEXT, status TEXT NOT NULL, claim_until TIMESTAMPTZ, error_code TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS notification_retry_operations_identity_uidx
  ON notification_retry_operations (event_id, retry_scope, idempotency_key)
  WHERE deleted_at IS NULL;
