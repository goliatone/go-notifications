ALTER TABLE notification_events ADD COLUMN IF NOT EXISTS channels JSONB;
ALTER TABLE notification_events ADD COLUMN IF NOT EXISTS locale TEXT;
ALTER TABLE notification_messages ADD COLUMN IF NOT EXISTS provider_plan JSONB;
ALTER TABLE notification_messages ADD COLUMN IF NOT EXISTS template_code TEXT;

-- Legacy Bun-generated schemas placed a table-level UNIQUE constraint on
-- notification_templates(code). Remove that exact one-column constraint while
-- preserving any host-owned constraints and the intended variant identity.
DO $$
DECLARE
  constraint_name TEXT;
BEGIN
  FOR constraint_name IN
    SELECT c.conname
    FROM pg_constraint c
    JOIN pg_class t ON t.oid = c.conrelid
    JOIN pg_namespace n ON n.oid = t.relnamespace
    WHERE n.nspname = current_schema()
      AND t.relname = 'notification_templates'
      AND c.contype = 'u'
      AND array_length(c.conkey, 1) = 1
      AND (
        SELECT a.attname
        FROM pg_attribute a
        WHERE a.attrelid = c.conrelid
          AND a.attnum = c.conkey[1]
      ) = 'code'
  LOOP
    EXECUTE format(
      'ALTER TABLE %I.%I DROP CONSTRAINT %I',
      current_schema(),
      'notification_templates',
      constraint_name
    );
  END LOOP;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS notification_templates_variant_uidx
  ON notification_templates (code, channel, COALESCE(locale, ''));
