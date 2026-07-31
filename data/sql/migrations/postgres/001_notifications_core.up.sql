CREATE TABLE IF NOT EXISTS notification_definitions (
  id UUID PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMPTZ,
  code TEXT NOT NULL UNIQUE, name TEXT NOT NULL DEFAULT '', description TEXT,
  severity TEXT, category TEXT, channels JSONB, metadata JSONB, template_keys JSONB, policy JSONB
);
CREATE TABLE IF NOT EXISTS notification_templates (
  id UUID PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMPTZ,
  code TEXT NOT NULL UNIQUE, channel TEXT NOT NULL, description TEXT, body TEXT,
  subject TEXT, locale TEXT, format TEXT, revision INTEGER, source JSONB, schema JSONB, metadata JSONB
);
CREATE TABLE IF NOT EXISTS notification_events (
  id UUID PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMPTZ,
  definition_code TEXT NOT NULL, tenant_id TEXT, actor_id TEXT, recipients JSONB,
  context JSONB, scheduled_at TIMESTAMPTZ, status TEXT
);
CREATE TABLE IF NOT EXISTS notification_messages (
  id UUID PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMPTZ,
  event_id UUID NOT NULL, channel TEXT NOT NULL, locale TEXT, subject TEXT, body TEXT,
  action_url TEXT, manifest_url TEXT, url TEXT, receiver TEXT NOT NULL, status TEXT, metadata JSONB
);
CREATE TABLE IF NOT EXISTS notification_delivery_attempts (
  id UUID PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMPTZ,
  message_id UUID NOT NULL, adapter TEXT NOT NULL, status TEXT, error TEXT, payload JSONB
);
CREATE TABLE IF NOT EXISTS notification_preferences (
  id UUID PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMPTZ,
  subject_id TEXT NOT NULL, subject_type TEXT NOT NULL, definition_code TEXT,
  channel TEXT, locale TEXT, enabled BOOLEAN, quiet_hours JSONB, additional_rules JSONB
);
CREATE TABLE IF NOT EXISTS notification_subscription_groups (
  id UUID PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMPTZ,
  code TEXT NOT NULL UNIQUE, name TEXT NOT NULL, description TEXT, metadata JSONB
);
CREATE TABLE IF NOT EXISTS notification_inbox_items (
  id UUID PRIMARY KEY, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMPTZ,
  user_id TEXT NOT NULL, message_id UUID, title TEXT, body TEXT, locale TEXT,
  unread BOOLEAN, pinned BOOLEAN, action_url TEXT, metadata JSONB,
  read_at TIMESTAMPTZ, dismissed_at TIMESTAMPTZ, snoozed_until TIMESTAMPTZ
);
