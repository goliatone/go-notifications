ALTER TABLE notification_events ADD COLUMN correlation_id TEXT;
ALTER TABLE notification_events ADD COLUMN request_id TEXT;
ALTER TABLE notification_events ADD COLUMN idempotency_scope TEXT;
ALTER TABLE notification_events ADD COLUMN idempotency_key TEXT;
ALTER TABLE notification_events ADD COLUMN request_fingerprint TEXT;
ALTER TABLE notification_events ADD COLUMN transient_dependent BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE notification_events ADD COLUMN publication_id TEXT;
ALTER TABLE notification_events ADD COLUMN digest_key TEXT;
ALTER TABLE notification_events ADD COLUMN retry_claim_until TIMESTAMP;
ALTER TABLE notification_delivery_attempts ADD COLUMN error_code TEXT;
ALTER TABLE notification_messages ADD COLUMN retry_operation_id TEXT;
ALTER TABLE notification_delivery_attempts ADD COLUMN retry_operation_id TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS notification_events_idempotency_uidx
  ON notification_events (idempotency_scope, definition_code, idempotency_key)
  WHERE idempotency_key IS NOT NULL AND idempotency_key <> '' AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS notification_events_publication_idx ON notification_events (publication_id);

CREATE TABLE IF NOT EXISTS notification_publications (
  id TEXT PRIMARY KEY, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMP,
  kind TEXT NOT NULL, digest_key TEXT, queue_key TEXT NOT NULL UNIQUE, run_at TIMESTAMP,
  status TEXT NOT NULL, claim_until TIMESTAMP, attempts INTEGER, error_code TEXT
);
CREATE INDEX IF NOT EXISTS notification_publications_pending_idx
  ON notification_publications (status, run_at);
CREATE INDEX IF NOT EXISTS notification_publications_digest_idx
  ON notification_publications (digest_key, status);

CREATE TABLE IF NOT EXISTS notification_retry_operations (
  id TEXT PRIMARY KEY, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMP,
  event_id TEXT NOT NULL, retry_scope TEXT NOT NULL, idempotency_key TEXT NOT NULL,
  correlation_id TEXT, request_id TEXT, status TEXT NOT NULL, claim_until TIMESTAMP, error_code TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS notification_retry_operations_identity_uidx
  ON notification_retry_operations (event_id, retry_scope, idempotency_key)
  WHERE deleted_at IS NULL;
