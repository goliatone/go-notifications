ALTER TABLE notification_events ADD COLUMN channels TEXT;
ALTER TABLE notification_events ADD COLUMN locale TEXT;
ALTER TABLE notification_messages ADD COLUMN provider_plan TEXT;
ALTER TABLE notification_messages ADD COLUMN template_code TEXT;

-- SQLite cannot drop a table-level UNIQUE(code) constraint in place. Rebuild
-- the table so installations created by the legacy Bun schema can store one
-- template variant per channel and locale.
CREATE TABLE notification_templates_v003 (
  id TEXT PRIMARY KEY, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at TIMESTAMP,
  code TEXT NOT NULL, channel TEXT NOT NULL, description TEXT, body TEXT,
  subject TEXT, locale TEXT, format TEXT, revision INTEGER, source TEXT, schema TEXT, metadata TEXT,
  UNIQUE (code, channel, locale)
);
INSERT INTO notification_templates_v003 (
  id, created_at, updated_at, deleted_at, code, channel, description, body,
  subject, locale, format, revision, source, schema, metadata
)
SELECT
  id, created_at, updated_at, deleted_at, code, channel, description, body,
  subject, locale, format, revision, source, schema, metadata
FROM notification_templates;
DROP TABLE notification_templates;
ALTER TABLE notification_templates_v003 RENAME TO notification_templates;
CREATE INDEX notification_templates_lookup_idx
  ON notification_templates (code, channel, locale);
CREATE UNIQUE INDEX notification_templates_variant_uidx
  ON notification_templates (code, channel, IFNULL(locale, ''));
