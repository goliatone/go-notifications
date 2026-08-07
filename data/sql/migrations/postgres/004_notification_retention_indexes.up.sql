CREATE INDEX IF NOT EXISTS notification_events_retention_idx
  ON notification_events (status, updated_at, id);
CREATE INDEX IF NOT EXISTS notification_messages_retention_idx
  ON notification_messages (status, updated_at, id);
CREATE INDEX IF NOT EXISTS notification_delivery_attempts_retention_idx
  ON notification_delivery_attempts (updated_at, message_id, id);
CREATE INDEX IF NOT EXISTS notification_inbox_items_retention_idx
  ON notification_inbox_items (dismissed_at, updated_at, id);
CREATE INDEX IF NOT EXISTS notification_publications_retention_idx
  ON notification_publications (status, updated_at, id);
CREATE INDEX IF NOT EXISTS notification_retry_operations_retention_idx
  ON notification_retry_operations (status, updated_at, id);
