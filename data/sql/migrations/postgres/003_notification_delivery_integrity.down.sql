-- The integrity repair is intentionally forward-only. Removing durable plans
-- or restoring UNIQUE(code) would make valid persisted data unrepresentable.
SELECT 1;
