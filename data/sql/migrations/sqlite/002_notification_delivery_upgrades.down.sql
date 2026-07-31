DROP INDEX IF EXISTS notification_retry_operations_event_idx;
DROP INDEX IF EXISTS notification_retry_operations_identity_uidx;
DROP TABLE IF EXISTS notification_retry_operations;
DROP INDEX IF EXISTS notification_publications_digest_idx;
DROP INDEX IF EXISTS notification_publications_open_digest_uidx;
DROP INDEX IF EXISTS notification_publications_pending_idx;
DROP TABLE IF EXISTS notification_publications;
DROP INDEX IF EXISTS notification_events_publication_idx;
DROP INDEX IF EXISTS notification_events_idempotency_uidx;
DROP INDEX IF EXISTS notification_delivery_attempts_retry_operation_idx;
DROP INDEX IF EXISTS notification_messages_retry_operation_idx;
-- SQLite cannot portably drop upgrade columns without rebuilding the tables.
