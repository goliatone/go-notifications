CREATE TABLE IF NOT EXISTS notification_definitions (
  id TEXT PRIMARY KEY, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMP,
  code TEXT NOT NULL UNIQUE, name TEXT NOT NULL DEFAULT '', description TEXT,
  severity TEXT, category TEXT, channels TEXT, metadata TEXT, template_keys TEXT, policy TEXT
);
CREATE TABLE IF NOT EXISTS notification_templates (
  id TEXT PRIMARY KEY, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMP,
  code TEXT NOT NULL UNIQUE, channel TEXT NOT NULL, description TEXT, body TEXT,
  subject TEXT, locale TEXT, format TEXT, revision INTEGER, source TEXT, schema TEXT, metadata TEXT
);
CREATE TABLE IF NOT EXISTS notification_events (
  id TEXT PRIMARY KEY, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMP,
  definition_code TEXT NOT NULL, tenant_id TEXT, actor_id TEXT, recipients TEXT,
  context TEXT, scheduled_at TIMESTAMP, status TEXT
);
CREATE TABLE IF NOT EXISTS notification_messages (
  id TEXT PRIMARY KEY, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMP,
  event_id TEXT NOT NULL, channel TEXT NOT NULL, locale TEXT, subject TEXT, body TEXT,
  action_url TEXT, manifest_url TEXT, url TEXT, receiver TEXT NOT NULL, status TEXT, metadata TEXT
);
CREATE TABLE IF NOT EXISTS notification_delivery_attempts (
  id TEXT PRIMARY KEY, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMP,
  message_id TEXT NOT NULL, adapter TEXT NOT NULL, status TEXT, error TEXT, payload TEXT
);
CREATE TABLE IF NOT EXISTS notification_preferences (
  id TEXT PRIMARY KEY, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMP,
  subject_id TEXT NOT NULL, subject_type TEXT NOT NULL, definition_code TEXT,
  channel TEXT, locale TEXT, enabled BOOLEAN, quiet_hours TEXT, additional_rules TEXT
);
CREATE TABLE IF NOT EXISTS notification_subscription_groups (
  id TEXT PRIMARY KEY, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMP,
  code TEXT NOT NULL UNIQUE, name TEXT NOT NULL, description TEXT, metadata TEXT
);
CREATE TABLE IF NOT EXISTS notification_inbox_items (
  id TEXT PRIMARY KEY, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMP,
  user_id TEXT NOT NULL, message_id TEXT, title TEXT, body TEXT, locale TEXT,
  unread BOOLEAN, pinned BOOLEAN, action_url TEXT, metadata TEXT,
  read_at TIMESTAMP, dismissed_at TIMESTAMP, snoozed_until TIMESTAMP
);
