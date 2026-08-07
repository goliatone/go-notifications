CREATE INDEX IF NOT EXISTS notification_events_scope_created_idx
  ON notification_events (tenant_id, created_at, id);
CREATE INDEX IF NOT EXISTS notification_events_scope_definition_created_idx
  ON notification_events (tenant_id, definition_code, created_at, id);
CREATE INDEX IF NOT EXISTS notification_messages_inspection_idx
  ON notification_messages (channel, status, created_at, id);
CREATE INDEX IF NOT EXISTS notification_delivery_attempts_inspection_idx
  ON notification_delivery_attempts (message_id, created_at, id);
